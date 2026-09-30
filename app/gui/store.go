package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-faster/errors"
)

// Store persists tasks and settings to disk with atomic writes.
type Store struct {
	mu       sync.RWMutex
	dir      string
	tasks    []*Task
	settings Settings
	dirty    bool // tasks changed but not flushed to disk
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create gui dir")
	}

	s := &Store{
		dir:      dir,
		tasks:    []*Task{},
		settings: DefaultSettings(),
	}

	if err := s.loadTasks(); err != nil {
		return nil, err
	}
	if err := s.loadSettings(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Store) tasksPath() string    { return filepath.Join(s.dir, "tasks.json") }
func (s *Store) settingsPath() string { return filepath.Join(s.dir, "settings.json") }

func (s *Store) loadTasks() error {
	b, err := os.ReadFile(s.tasksPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "read tasks")
	}
	if len(b) == 0 {
		return nil
	}
	if err = json.Unmarshal(b, &s.tasks); err != nil {
		return errors.Wrap(err, "unmarshal tasks")
	}
	// reset interrupted tasks (e.g. process killed mid-download) and
	// drop stale counters so the UI shows 0 until OnAdd re-reports them
	for _, t := range s.tasks {
		if t.Status == TaskStatusDownloading {
			t.Status = TaskStatusPending
			t.Error = ""
			t.Progress = 0
			t.Total = 0
			t.Filename = ""
		}
	}
	return nil
}

func (s *Store) loadSettings() error {
	b, err := os.ReadFile(s.settingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "read settings")
	}
	if len(b) == 0 {
		return nil
	}
	// merge over defaults so newly-added fields keep their defaults
	cur := DefaultSettings()
	if err = json.Unmarshal(b, &cur); err != nil {
		return errors.Wrap(err, "unmarshal settings")
	}
	s.settings = cur
	return nil
}

// atomicWrite writes data to a temp file then renames it.
func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) saveTasksLocked() error {
	b, err := json.MarshalIndent(s.tasks, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(s.tasksPath(), b)
}

func (s *Store) saveSettingsLocked() error {
	b, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(s.settingsPath(), b)
}

// Flush writes tasks to disk if dirty. Called periodically.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	if err := s.saveTasksLocked(); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

// Tasks returns a snapshot of all tasks.
func (s *Store) Tasks() []Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Task, len(s.tasks))
	for i, t := range s.tasks {
		out[i] = *t
	}
	return out
}

// GetTask returns a copy of the task with the given id.
func (s *Store) GetTask(id string) (Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.tasks {
		if t.ID == id {
			return *t, true
		}
	}
	return Task{}, false
}

func (s *Store) AddTask(t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// keep the pre-append slice so an atomic-write failure rolls back
	// cleanly instead of leaving a task that was never persisted
	prev := s.tasks
	s.tasks = append(s.tasks, t)
	if err := s.saveTasksLocked(); err != nil {
		s.tasks = prev
		return err
	}
	return nil
}

func (s *Store) DeleteTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.tasks {
		if t.ID == id {
			s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
			return s.saveTasksLocked()
		}
	}
	return nil
}

// UpdateTask mutates a task under lock. If persist is false the change is
// kept in memory only and flushed later via Flush.
func (s *Store) UpdateTask(id string, persist bool, fn func(*Task)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tasks {
		if t.ID == id {
			fn(t)
			if persist {
				s.dirty = false
				return s.saveTasksLocked()
			}
			s.dirty = true
			return nil
		}
	}
	return os.ErrNotExist
}

// ClearFinished removes done/error/canceled tasks.
func (s *Store) ClearFinished() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.tasks[:0]
	for _, t := range s.tasks {
		if t.Status == TaskStatusPending || t.Status == TaskStatusDownloading {
			kept = append(kept, t)
		}
	}
	s.tasks = kept
	return s.saveTasksLocked()
}

func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *Store) UpdateSettings(fn func(*Settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.settings)
	return s.saveSettingsLocked()
}
