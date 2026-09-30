package gui

import (
	"context"
	"sync"

	"github.com/go-faster/errors"

	"github.com/iyear/tdl/app/dl"
)

// guiHook implements dl.Hook and mirrors per-element progress into the task
// stored in Store. Progress updates are kept in memory (persist=false) and
// flushed to disk by the engine's flush ticker.
type guiHook struct {
	store *Store
	id    string

	mu        sync.Mutex
	elems     map[int]*elemState // elem id -> state
	failCount int
	firstErr  string
}

type elemState struct {
	size       int64
	downloaded int64
	name       string
}

func newGUIHook(store *Store, taskID string) *guiHook {
	return &guiHook{
		store: store,
		id:    taskID,
		elems: make(map[int]*elemState),
	}
}

// FailCount reports how many elements finished with an error, plus the
// first error message. tdl's downloader intentionally swallows per-element
// errors, so the GUI uses this to detect partial failures.
func (h *guiHook) FailCount() (int, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failCount, h.firstErr
}

func (h *guiHook) recalc() (downloaded, total int64, name string) {
	for _, e := range h.elems {
		downloaded += e.downloaded
		total += e.size
		if name == "" {
			name = e.name
		}
	}
	return
}

func (h *guiHook) OnAdd(elem dl.ElemInfo) {
	h.mu.Lock()
	h.elems[elem.ID] = &elemState{size: elem.Size, name: elem.Name}
	d, t, name := h.recalc()
	h.mu.Unlock()

	_ = h.store.UpdateTask(h.id, false, func(task *Task) {
		task.Progress = d
		task.Total = t
		task.Filename = name
	})
}

func (h *guiHook) OnDownload(elem dl.ElemInfo, downloaded, total int64) {
	h.mu.Lock()
	st, ok := h.elems[elem.ID]
	if !ok {
		st = &elemState{name: elem.Name}
		h.elems[elem.ID] = st
	}
	st.downloaded = downloaded
	if total > 0 {
		st.size = total
	}
	d, t, name := h.recalc()
	h.mu.Unlock()

	_ = h.store.UpdateTask(h.id, false, func(task *Task) {
		task.Progress = d
		task.Total = t
		if name != "" {
			task.Filename = name
		}
	})
}

func (h *guiHook) OnDone(elem dl.ElemInfo, err error) {
	h.mu.Lock()
	if st, ok := h.elems[elem.ID]; ok {
		if err == nil {
			st.downloaded = st.size
		}
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		h.failCount++
		if h.firstErr == "" {
			h.firstErr = err.Error()
		}
	}
	d, t, name := h.recalc()
	h.mu.Unlock()

	_ = h.store.UpdateTask(h.id, false, func(task *Task) {
		task.Progress = d
		task.Total = t
		if name != "" {
			task.Filename = name
		}
		// record the first real per-element error
		if err != nil && !errors.Is(err, context.Canceled) && task.Error == "" {
			task.Error = err.Error()
		}
		if err == nil && elem.FinalPath != "" {
			task.FinalPath = elem.FinalPath
		}
	})

	_ = h.store.Flush()
}
