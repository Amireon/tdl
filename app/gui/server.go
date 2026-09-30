package gui

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-faster/errors"
	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/iyear/tdl/core/util/netutil"
)

//go:embed web
var webFS embed.FS

type Server struct {
	engine *Engine
	router *mux.Router
	webDir string // dev override, empty = embed
	http   *http.Server

	// allowedHosts/allowedOrigins are populated once the listener is
	// bound. They guard against DNS rebinding (Host) and cross-site
	// request forgery against the unauthenticated local API (Origin).
	allowedHosts   map[string]struct{}
	allowedOrigins map[string]struct{}
}

func NewServer(engine *Engine, webDir string) *Server {
	s := &Server{engine: engine, webDir: webDir}
	r := mux.NewRouter()

	// API
	r.HandleFunc("/api/tasks", s.listTasks).Methods(http.MethodGet)
	r.HandleFunc("/api/tasks", s.createTask).Methods(http.MethodPost)
	r.HandleFunc("/api/tasks/start-all", s.startAll).Methods(http.MethodPost)
	r.HandleFunc("/api/tasks/clear-finished", s.clearFinished).Methods(http.MethodPost)
	r.HandleFunc("/api/tasks/{id}", s.deleteTask).Methods(http.MethodDelete)
	r.HandleFunc("/api/tasks/{id}/start", s.startTask).Methods(http.MethodPost)
	r.HandleFunc("/api/tasks/{id}/cancel", s.cancelTask).Methods(http.MethodPost)
	r.HandleFunc("/api/tasks/{id}/restart", s.restartTask).Methods(http.MethodPost)
	r.HandleFunc("/api/tasks/{id}/open-dir", s.openTaskDir).Methods(http.MethodPost)
	r.HandleFunc("/api/settings", s.getSettings).Methods(http.MethodGet)
	r.HandleFunc("/api/settings", s.updateSettings).Methods(http.MethodPut)
	r.HandleFunc("/api/auth/status", s.authStatus).Methods(http.MethodGet)
	r.HandleFunc("/api/auth/phone", s.authPhone).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/code", s.authCode).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/password", s.authPassword).Methods(http.MethodPost)

	// static
	if webDir != "" {
		r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir(webDir))))
		r.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
			http.ServeFile(w, req, webDir+"/index.html")
		})
	} else {
		sub, err := fs.Sub(webFS, "web")
		if err != nil {
			panic(err)
		}
		r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
		r.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
			b, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(b)
		})
	}

	// local-only request guard (DNS rebinding / CSRF)
	r.Use(s.guard)

	s.router = r
	return s
}

// guard rejects requests whose Host is not the loopback listener (DNS
// rebinding) and requires a same-origin Origin/Referer for state-changing
// requests, so other websites cannot drive the local API via simple
// cross-site forms or fetch(text/plain) requests.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(s.allowedHosts) > 0 {
			if _, ok := s.allowedHosts[r.Host]; !ok {
				writeError(w, http.StatusForbidden, "unexpected host")
				return
			}
		}

		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			// safe methods: reading is only possible via same host
		default:
			if !s.originAllowed(r) {
				writeError(w, http.StatusForbidden, "cross-origin request rejected")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) originAllowed(r *http.Request) bool {
	if o := r.Header.Get("Origin"); o != "" {
		_, ok := s.allowedOrigins[o]
		return ok
	}
	// some same-origin navigations omit Origin; fall back to Referer
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Scheme != "" && u.Host != "" {
			_, ok := s.allowedOrigins[u.Scheme+"://"+u.Host]
			return ok
		}
	}
	return false
}

// ListenAndServe tries ports starting at port, increments on conflict.
func (s *Server) ListenAndServe(port int) (string, error) {
	var ln net.Listener
	var err error
	addr := ""
	for i := 0; i < 4; i++ {
		addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(port+i))
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", errors.Wrap(err, "listen")
	}

	// addr is 127.0.0.1:port; the page may also be opened via localhost
	hostPort := ln.Addr().String()
	if _, port, perr := net.SplitHostPort(hostPort); perr == nil {
		s.allowedHosts = map[string]struct{}{
			net.JoinHostPort("127.0.0.1", port): {},
			net.JoinHostPort("localhost", port): {},
		}
		s.allowedOrigins = map[string]struct{}{
			"http://" + net.JoinHostPort("127.0.0.1", port): {},
			"http://" + net.JoinHostPort("localhost", port): {},
		}
	}

	s.http = &http.Server{Handler: s.router, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		_ = s.http.Serve(ln)
	}()
	return "http://" + addr, nil
}

// Shutdown gracefully stops the http server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(msg))
}

type createTaskReq struct {
	URL    string `json:"url"`
	Author string `json:"author"`
	Dir    string `json:"dir"`
}

// normalizeMessageURL trims the input and prepends https:// when no scheme
// is present, so bare links like "t.me/foo/123" work too.
func normalizeMessageURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "://") {
		return raw
	}
	return "https://" + raw
}

// isValidMessageURL accepts the same link shapes tdl itself parses:
//   - t.me/username/123
//   - t.me/username/123?comment=456 (discussion comment)
//   - t.me/username/topic/123 (forum topic, 3 segments)
//   - t.me/c/channel/123 (private)
//   - t.me/c/channel/topic/123 (private forum topic, 4 segments)
func isValidMessageURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "t.me", "www.t.me", "telegram.me", "www.telegram.me":
	default:
		return false
	}
	p := strings.Trim(u.Path, "/")
	if p == "" {
		return false
	}
	parts := strings.Split(p, "/")
	isNum := func(s string) bool {
		_, err := strconv.Atoi(s)
		return err == nil
	}
	switch {
	case len(parts) == 2:
		// username/123 (comment query also lands here)
		return isNum(parts[1])
	case len(parts) == 3:
		// c/channel/123 or username/topic/123
		return isNum(parts[2])
	case len(parts) == 4 && parts[0] == "c":
		// c/channel/topic/123
		return isNum(parts[3])
	}
	return false
}

func (s *Server) listTasks(w http.ResponseWriter, _ *http.Request) {
	tasks := s.engine.store.Tasks()
	def := s.engine.store.Settings().DefaultDir
	for i := range tasks {
		tasks[i].EffectiveDir = effectiveDir(&tasks[i], def)
	}
	writeJSON(w, tasks)
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.URL = normalizeMessageURL(req.URL)
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	if !isValidMessageURL(req.URL) {
		writeError(w, http.StatusBadRequest, "url must be a t.me message link, e.g. https://t.me/xxx/123")
		return
	}

	settings := s.engine.store.Settings()
	author := strings.TrimSpace(req.Author)
	if author == "" {
		author = settings.DefaultAuthor
	}
	dir := strings.TrimSpace(req.Dir)
	if dir == "" {
		dir = settings.DefaultDir
	}

	t := &Task{
		ID:        uuid.NewString(),
		URL:       req.URL,
		Author:    author,
		Dir:       dir,
		Status:    TaskStatusPending,
		CreatedAt: time.Now(),
	}
	if err := s.engine.store.AddTask(t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, t)
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.engine.CancelTask(id) // cancel first if running (best effort)
	s.engine.ForgetTask(id)
	if err := s.engine.store.DeleteTask(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) startTask(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := s.engine.StartTask(id, false); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) restartTask(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := s.engine.StartTask(id, true); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) openTaskDir(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	t, ok := s.engine.store.GetTask(id)
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	// open the real download folder: dir + author subdir
	dir := effectiveDir(&t, s.engine.store.Settings().DefaultDir)
	if dir == "" || dir == "." {
		writeError(w, http.StatusBadRequest, "task has no directory")
		return
	}
	// Ensure it exists so a pending task (whose folder tdl creates only
	// when the download starts) can still be opened up-front.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "create directory: "+err.Error())
		return
	}
	if err := OpenInFileManager(dir); err != nil {
		writeError(w, http.StatusInternalServerError, "open directory: "+err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) cancelTask(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := s.engine.CancelTask(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) startAll(w http.ResponseWriter, _ *http.Request) {
	n := s.engine.StartAll()
	writeJSON(w, map[string]int{"started": n})
}

func (s *Server) clearFinished(w http.ResponseWriter, _ *http.Request) {
	if err := s.engine.store.ClearFinished(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.engine.store.Settings())
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req Settings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Proxy = strings.TrimSpace(req.Proxy)
	if req.Proxy != "" {
		if _, err := netutil.NewProxy(req.Proxy); err != nil {
			writeError(w, http.StatusBadRequest, "invalid proxy: "+err.Error())
			return
		}
	}
	if err := s.engine.UpdateSettings(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) authStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.engine.AuthStatus())
}

type authReq struct {
	Phone    string `json:"phone"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

func (s *Server) authPhone(w http.ResponseWriter, r *http.Request) {
	var req authReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.engine.SubmitPhone(req.Phone); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) authCode(w http.ResponseWriter, r *http.Request) {
	var req authReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.engine.SubmitCode(req.Code); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) authPassword(w http.ResponseWriter, r *http.Request) {
	var req authReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.engine.SubmitPassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
