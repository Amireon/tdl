package gui

import (
	"path/filepath"
	"time"

	"github.com/iyear/tdl/pkg/consts"
)

// TaskStatus is the lifecycle state of a download task.
type TaskStatus string

const (
	TaskStatusPending     TaskStatus = "pending"
	TaskStatusDownloading TaskStatus = "downloading"
	TaskStatusDone        TaskStatus = "done"
	TaskStatusError       TaskStatus = "error"
	TaskStatusCanceled    TaskStatus = "canceled"
)

// Task is a single download job created from a Telegram message URL.
type Task struct {
	ID        string     `json:"id"`
	URL       string     `json:"url"`
	Author    string     `json:"author"`
	Dir       string     `json:"dir"`
	Status    TaskStatus `json:"status"`
	Progress  int64      `json:"progress"` // downloaded bytes
	Total     int64      `json:"total"`    // total bytes
	Filename  string     `json:"filename,omitempty"`
	FinalPath string     `json:"final_path,omitempty"`
	// EffectiveDir is the actual on-disk folder (dir + author subdir),
	// computed on read by the server. It is never persisted.
	EffectiveDir string     `json:"effective_dir,omitempty"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// Settings are GUI-level options persisted to settings.json.
type Settings struct {
	Proxy              string `json:"proxy"` // protocol://username:password@host:port
	DefaultDir         string `json:"default_dir"`
	DefaultAuthor      string `json:"default_author"`
	MaxConcurrentTasks int    `json:"max_concurrent_tasks"`
	Threads            int    `json:"threads"` // max threads for one item
	Limit              int    `json:"limit"`   // max concurrent items within one task
	RewriteExt         bool   `json:"rewrite_ext"`
	SkipSame           bool   `json:"skip_same"`
}

func DefaultSettings() Settings {
	return Settings{
		Proxy:              "",
		DefaultDir:         filepath.Join(consts.HomeDir, "Downloads", "tdl"),
		DefaultAuthor:      "",
		MaxConcurrentTasks: 1,
		Threads:            4,
		Limit:              2,
		RewriteExt:         true,
		SkipSame:           true,
	}
}

// LoginState is the state machine of Telegram authorization.
type LoginState string

const (
	LoginStateConnecting   LoginState = "connecting"
	LoginStateReady        LoginState = "ready"
	LoginStateWaitPhone    LoginState = "wait_phone"
	LoginStateWaitCode     LoginState = "wait_code"
	LoginStateWaitPassword LoginState = "wait_password"
	LoginStateError        LoginState = "error"
)

// AuthStatus reports current authorization state to the web UI.
type AuthStatus struct {
	State     LoginState `json:"state"`
	UserID    int64      `json:"user_id,omitempty"`
	Username  string     `json:"username,omitempty"`
	FirstName string     `json:"first_name,omitempty"`
	Error     string     `json:"error,omitempty"`
}
