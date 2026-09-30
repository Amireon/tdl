package gui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flytam/filenamify"
	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/iyear/tdl/app/dl"
	"github.com/iyear/tdl/core/logctx"
	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/pkg/consts"
	"github.com/iyear/tdl/pkg/key"
	"github.com/iyear/tdl/pkg/tclient"
)

type Engine struct {
	store *Store
	kvd   storage.Storage

	// fallbackProxy is used when settings.proxy is empty (from --proxy flag)
	fallbackProxy string

	rootCtx context.Context

	auth *authMachine

	mu        sync.Mutex
	client    *telegram.Client
	runCancel context.CancelFunc
	readyCh   chan struct{}
	runDone   chan struct{} // closed when the current client run exits

	selfMu sync.RWMutex
	self   *AuthStatus

	// active holds ids of tasks that are queued, waiting for login or
	// downloading. It is set in StartTask and only cleared once the task
	// leaves the executor, preventing the same task from running twice.
	queueCh    chan string
	active     map[string]struct{}
	restartSet map[string]struct{}
	cancels    map[string]context.CancelFunc
	running    atomic.Int32

	// pendingReconnect means the current client should be torn down once
	// all running tasks finish (e.g. proxy changed mid-download).
	pendingReconnect bool

	// wg covers the client loop, executor, flush loop and every running
	// task goroutine, so Shutdown can wait for them to wind down.
	wg sync.WaitGroup
}

func NewEngine(ctx context.Context, store *Store, kvd storage.Storage, fallbackProxy string) *Engine {
	return &Engine{
		store:         store,
		kvd:           kvd,
		fallbackProxy: fallbackProxy,
		rootCtx:       ctx,
		auth:          newAuthMachine(),
		queueCh:       make(chan string, 256),
		active:        make(map[string]struct{}),
		restartSet:    make(map[string]struct{}),
		cancels:       make(map[string]context.CancelFunc),
	}
}

func (e *Engine) log() *zap.Logger {
	return logctx.From(e.rootCtx)
}

// Start launches the client loop and the task executor.
func (e *Engine) Start() {
	e.wg.Add(3)
	go func() { defer e.wg.Done(); e.clientLoop() }()
	go func() { defer e.wg.Done(); e.executor() }()
	go func() { defer e.wg.Done(); e.flushLoop() }()
}

// Shutdown flushes pending task changes to disk and waits (up to ctx
// deadline) for the client loop, executor and running tasks to stop after
// the root context has been canceled.
func (e *Engine) Shutdown(ctx context.Context) error {
	if err := e.store.Flush(); err != nil {
		e.log().Warn("flush tasks on shutdown", zap.Error(err))
	}

	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Engine) flushLoop() {
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-e.rootCtx.Done():
			return
		case <-t.C:
			if err := e.store.Flush(); err != nil {
				e.log().Warn("flush tasks", zap.Error(err))
			}
		}
	}
}

func (e *Engine) clientLoop() {
	for {
		if e.rootCtx.Err() != nil {
			return
		}

		e.auth.reset()
		e.setSelf(&AuthStatus{State: LoginStateConnecting})

		proxy := e.store.Settings().Proxy
		if proxy == "" {
			proxy = e.fallbackProxy
		}

		// ensure app key is set for login (only when absent)
		if _, err := e.kvd.Get(e.rootCtx, key.App()); err != nil {
			if serr := e.kvd.Set(e.rootCtx, key.App(), []byte(tclient.AppDesktop)); serr != nil {
				e.setAuthError(errors.Wrap(serr, "set app"))
				if !e.sleep(3 * time.Second) {
					return
				}
				continue
			}
		}

		c, err := tclient.New(e.rootCtx, tclient.Options{
			KV:               e.kvd,
			Proxy:            proxy,
			NTP:              viper.GetString(consts.FlagNTP),
			ReconnectTimeout: viper.GetDuration(consts.FlagReconnectTimeout),
		}, false)
		if err != nil {
			e.setAuthError(errors.Wrap(err, "create client"))
			if !e.sleep(3 * time.Second) {
				return
			}
			continue
		}

		runCtx, cancel := context.WithCancel(e.rootCtx)
		readyCh := make(chan struct{})
		runDone := make(chan struct{})
		e.mu.Lock()
		e.client = c
		e.runCancel = cancel
		e.readyCh = readyCh
		e.runDone = runDone
		e.mu.Unlock()

		runErr := c.Run(runCtx, func(rctx context.Context) error {
			if err := c.Ping(rctx); err != nil {
				return err
			}

			flow := auth.NewFlow(e.auth, auth.SendCodeOptions{})
			if err := c.Auth().IfNecessary(rctx, flow); err != nil {
				return err
			}

			self, err := c.Self(rctx)
			if err != nil {
				return err
			}

			e.auth.setState(LoginStateReady, "")
			e.auth.clearPendingPhone()
			e.setSelf(&AuthStatus{
				State:     LoginStateReady,
				UserID:    self.ID,
				Username:  self.Username,
				FirstName: self.FirstName,
			})
			close(readyCh)

			<-rctx.Done()
			return nil
		})
		cancel()
		close(runDone)

		e.mu.Lock()
		e.client = nil
		e.runCancel = nil
		e.readyCh = nil
		e.runDone = nil
		e.mu.Unlock()

		if e.rootCtx.Err() != nil {
			return
		}

		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			e.setAuthError(runErr)
		}

		if !e.sleep(3 * time.Second) {
			return
		}
	}
}

func (e *Engine) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-e.rootCtx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (e *Engine) setSelf(s *AuthStatus) {
	e.selfMu.Lock()
	defer e.selfMu.Unlock()
	e.self = s
}

func (e *Engine) setAuthError(err error) {
	e.log().Warn("auth", zap.Error(err))
	e.auth.setState(LoginStateError, err.Error())
	e.setSelf(&AuthStatus{State: LoginStateError, Error: err.Error()})
}

// AuthStatus returns the current login state.
func (e *Engine) AuthStatus() AuthStatus {
	st, serr := e.auth.State()
	e.selfMu.RLock()
	defer e.selfMu.RUnlock()
	if st == LoginStateReady && e.self != nil {
		return *e.self
	}
	return AuthStatus{State: st, Error: serr}
}

func (e *Engine) SubmitPhone(v string) error    { return e.auth.SubmitPhone(v) }
func (e *Engine) SubmitCode(v string) error     { return e.auth.SubmitCode(v) }
func (e *Engine) SubmitPassword(v string) error { return e.auth.SubmitPassword(v) }

// UpdateSettings persists new settings and invalidates the client when the
// proxy changed. Global viper values are updated here as well.
func (e *Engine) UpdateSettings(s Settings) error {
	if s.MaxConcurrentTasks < 1 {
		s.MaxConcurrentTasks = 1
	}
	if s.Threads < 1 {
		s.Threads = 4
	}
	if s.Limit < 1 {
		s.Limit = 2
	}

	oldProxy := e.store.Settings().Proxy
	if err := e.store.UpdateSettings(func(cur *Settings) { *cur = s }); err != nil {
		return err
	}

	viper.Set(consts.FlagThreads, s.Threads)
	viper.Set(consts.FlagLimit, s.Limit)

	if s.Proxy != oldProxy {
		e.invalidateClient()
	}
	return nil
}

// invalidateClient tears down the current connection so the client loop
// reconnects with fresh settings (e.g. new proxy). If a download is in
// flight, the teardown is deferred until all running tasks finish, so the
// ongoing download is not interrupted.
func (e *Engine) invalidateClient() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runCancel == nil {
		return
	}

	// If the client is not ready yet, any "running" tasks are only waiting
	// for login, so reconnect immediately.
	ready := false
	if e.readyCh != nil {
		select {
		case <-e.readyCh:
			ready = true
		default:
		}
	}

	if ready && e.running.Load() > 0 {
		// don't interrupt an active download; reconnect once it finishes
		e.pendingReconnect = true
		return
	}
	e.runCancel()
}

// maybeReconnect tears down the client when a reconnect was requested while
// the previous task was running.
func (e *Engine) maybeReconnect() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.pendingReconnect || e.running.Load() != 0 {
		return
	}
	e.pendingReconnect = false
	if e.runCancel != nil {
		e.runCancel()
	}
}

// StartTask queues a task. restart=true forces a full re-download.
func (e *Engine) StartTask(id string, restart bool) error {
	if _, ok := e.store.GetTask(id); !ok {
		return errors.New("task not found")
	}
	e.mu.Lock()
	if _, dup := e.active[id]; dup {
		e.mu.Unlock()
		return errors.New("task is already queued or running")
	}
	// active is held until the executor finishes with this id (including
	// the wait-for-login window), so the task can never be enqueued twice.
	e.active[id] = struct{}{}
	if restart {
		e.restartSet[id] = struct{}{}
	} else {
		delete(e.restartSet, id)
	}
	e.mu.Unlock()

	now := time.Now()
	err := e.store.UpdateTask(id, true, func(t *Task) {
		t.Status = TaskStatusPending
		t.Error = ""
		t.StartedAt = &now
		t.FinishedAt = nil
	})
	if err != nil {
		e.mu.Lock()
		delete(e.active, id)
		delete(e.restartSet, id)
		e.mu.Unlock()
		return err
	}

	select {
	case e.queueCh <- id:
	default:
		go func() { e.queueCh <- id }()
	}
	return nil
}

// StartAll queues all pending tasks. Returns queued count.
func (e *Engine) StartAll() int {
	n := 0
	for _, t := range e.store.Tasks() {
		if t.Status == TaskStatusPending {
			if err := e.StartTask(t.ID, false); err == nil {
				n++
			}
		}
	}
	return n
}

// ForgetTask drops in-memory markers for a task. Call after store deletion.
func (e *Engine) ForgetTask(id string) {
	e.mu.Lock()
	delete(e.active, id)
	delete(e.restartSet, id)
	e.mu.Unlock()
}

// clearActive releases the active marker for a task that has left the
// executor (finished, skipped or no longer exists).
func (e *Engine) clearActive(id string) {
	e.mu.Lock()
	delete(e.active, id)
	e.mu.Unlock()
}

// CancelTask cancels a running or queued task.
func (e *Engine) CancelTask(id string) error {
	e.mu.Lock()
	cancel, running := e.cancels[id]
	e.mu.Unlock()

	if running {
		cancel()
		return nil
	}

	t, ok := e.store.GetTask(id)
	if !ok {
		return errors.New("task not found")
	}
	if t.Status == TaskStatusPending {
		return e.store.UpdateTask(id, true, func(t *Task) {
			t.Status = TaskStatusCanceled
			now := time.Now()
			t.FinishedAt = &now
		})
	}
	return errors.New("task is not cancellable")
}

func (e *Engine) executor() {
	for {
		select {
		case <-e.rootCtx.Done():
			return
		case id := <-e.queueCh:
			maxC := e.store.Settings().MaxConcurrentTasks
			for int(e.running.Load()) >= maxC {
				if !e.sleep(200 * time.Millisecond) {
					return
				}
				maxC = e.store.Settings().MaxConcurrentTasks
			}

			// skip if no longer pending (e.g. canceled or deleted
			// while queued); release its active marker
			t, ok := e.store.GetTask(id)
			if !ok || t.Status != TaskStatusPending {
				e.clearActive(id)
				continue
			}

			e.running.Add(1)
			e.wg.Add(1)
			go func() {
				defer e.wg.Done()
				defer func() {
					if e.running.Add(-1) == 0 {
						e.maybeReconnect()
					}
					// only release after runTask fully returns, so the
					// wait-for-login window is also protected
					e.clearActive(id)
				}()
				e.runTask(id)
			}()
		}
	}
}

func (e *Engine) runTask(id string) {
	if _, ok := e.store.GetTask(id); !ok {
		return
	}

	// wait until an authorized client is available. readyCh only closes on
	// successful login and runDone closes when the client run exits (e.g.
	// network failure before login, or a proxy switch), so both must be
	// watched: waiting on a stale readyCh alone would block forever and
	// freeze the whole queue.
	var c *telegram.Client
	for {
		if e.rootCtx.Err() != nil {
			_ = e.store.UpdateTask(id, true, func(t *Task) { t.Status = TaskStatusPending })
			return
		}

		e.mu.Lock()
		readyCh := e.readyCh
		doneCh := e.runDone
		c = e.client
		e.mu.Unlock()

		if readyCh == nil || doneCh == nil || c == nil {
			if !e.sleep(300 * time.Millisecond) {
				return
			}
			continue
		}

		select {
		case <-e.rootCtx.Done():
			_ = e.store.UpdateTask(id, true, func(t *Task) { t.Status = TaskStatusPending })
			return
		case <-doneCh:
			// client torn down (possibly before login); retry with the
			// next client run instead of blocking on its readyCh
			if !e.sleep(100 * time.Millisecond) {
				return
			}
			continue
		case <-readyCh:
		}

		// readyCh fired: make sure the run we waited on is still the
		// live, authorized client
		e.mu.Lock()
		cur := e.client
		curDone := e.runDone
		e.mu.Unlock()
		if cur == nil || cur != c || curDone != doneCh {
			if !e.sleep(100 * time.Millisecond) {
				return
			}
			continue
		}
		break
	}

	// could have been canceled while waiting
	t, ok := e.store.GetTask(id)
	if !ok || t.Status != TaskStatusPending {
		return
	}

	ctx, cancel := context.WithCancel(e.rootCtx)
	e.mu.Lock()
	e.cancels[id] = cancel
	e.mu.Unlock()
	defer func() {
		cancel()
		e.mu.Lock()
		delete(e.cancels, id)
		e.mu.Unlock()
	}()

	settings := e.store.Settings()
	dir := effectiveDir(&t, settings.DefaultDir)

	restart := false
	e.mu.Lock()
	if _, ok := e.restartSet[id]; ok {
		restart = true
		delete(e.restartSet, id)
	}
	e.mu.Unlock()

	hook := newGUIHook(e.store, id)

	now := time.Now()
	_ = e.store.UpdateTask(id, true, func(t *Task) {
		t.Status = TaskStatusDownloading
		t.StartedAt = &now
		t.Error = ""
		if restart {
			t.Progress = 0
			t.Total = 0
			t.Filename = ""
			t.FinalPath = ""
		}
	})

	opts := dl.Options{
		URLs:       []string{t.URL},
		Dir:        dir,
		RewriteExt: settings.RewriteExt,
		SkipSame:   settings.SkipSame,
		Template:   `{{ .DialogID }}_{{ .MessageID }}_{{ filenamify .FileName }}`,
		Group:      true,
		Continue:   !restart,
		Restart:    restart,
		Hook:       hook,
	}

	e.log().Info("start task", zap.String("id", id), zap.String("url", t.URL), zap.String("dir", dir))
	err := dl.Run(ctx, c, e.kvd, opts)

	finished := time.Now()
	switch {
	case err == nil:
		failed, firstErr := hook.FailCount()
		_ = e.store.UpdateTask(id, true, func(t *Task) {
			t.FinishedAt = &finished
			if failed > 0 {
				t.Status = TaskStatusError
				t.Error = fmt.Sprintf("%d file(s) failed: %s", failed, firstErr)
			} else {
				t.Status = TaskStatusDone
				t.Error = ""
			}
		})
	case errors.Is(err, context.Canceled):
		_ = e.store.UpdateTask(id, true, func(t *Task) {
			t.Status = TaskStatusCanceled
			t.FinishedAt = &finished
		})
	default:
		e.log().Warn("task failed", zap.String("id", id), zap.Error(err))
		_ = e.store.UpdateTask(id, true, func(t *Task) {
			t.Status = TaskStatusError
			t.Error = err.Error()
			t.FinishedAt = &finished
		})
	}
}

// effectiveDir returns the actual on-disk folder tdl downloads a task
// into: <task dir or default dir>/<sanitized author>, honoring "~".
// This is the single source of truth shared by the executor, the
// list API and the "open directory" action.
func effectiveDir(t *Task, fallbackDefault string) string {
	dir := strings.TrimSpace(t.Dir)
	if dir == "" {
		dir = fallbackDefault
	}
	dir = expandHome(dir)
	if author := strings.TrimSpace(t.Author); author != "" {
		dir = filepath.Join(dir, sanitizeFilename(author))
	}
	return filepath.Clean(dir)
}

func expandHome(p string) string {
	switch {
	case p == "~":
		return consts.HomeDir
	case strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`):
		return filepath.Join(consts.HomeDir, p[2:])
	default:
		return p
	}
}

var invalidFileChars = strings.NewReplacer(
	"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
	"\"", "_", "<", "_", ">", "_", "|", "_",
)

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	s = invalidFileChars.Replace(s)
	s = strings.Trim(s, ". ")
	if s == "" || s == ".." {
		return "_"
	}
	if r, err := filenamify.FilenamifyV2(s); err == nil && r != "" {
		return r
	}
	return s
}
