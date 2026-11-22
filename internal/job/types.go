package job

import (
	"os"
	"sync"
	"time"

	"github.com/ALiwoto/codex-supervisor/internal/processTree"
)

type Request struct {
	ID              string            `json:"id,omitempty"`
	Command         []string          `json:"command"`
	Directory       string            `json:"directory"`
	Environment     map[string]string `json:"environment,omitempty"`
	TimeoutSeconds  int               `json:"timeout_seconds,omitempty"`
	StallSeconds    int               `json:"stall_seconds,omitempty"`
	Heartbeat       string            `json:"heartbeat,omitempty"`
	WaitForChildren bool              `json:"wait_for_children,omitempty"`
}

type Snapshot struct {
	ID                     string            `json:"id"`
	State                  string            `json:"state"`
	PID                    int               `json:"pid,omitempty"`
	Started                time.Time         `json:"started"`
	Finished               *time.Time        `json:"finished,omitempty"`
	CommandFinished        *time.Time        `json:"command_finished,omitempty"`
	ProcessesAtCommandExit uint32            `json:"processes_at_command_exit,omitempty"`
	ElapsedSeconds         float64           `json:"elapsed_seconds"`
	ExitCode               *int              `json:"exit_code,omitempty"`
	LogPath                string            `json:"log_path"`
	PreviousLogPath        string            `json:"previous_log_path,omitempty"`
	LogTruncated           bool              `json:"log_truncated,omitempty"`
	Usage                  processTree.Usage `json:"usage"`
	Warning                string            `json:"warning,omitempty"`
	Error                  string            `json:"error,omitempty"`
	Diagnostics            []string          `json:"diagnostics,omitempty"`
}

type Connection struct {
	Address string `json:"address"`
	Token   string `json:"token"`
}

type Manager struct {
	Root string
}

type worker struct {
	mu         sync.Mutex
	snapshot   Snapshot
	changed    chan struct{}
	cancel     chan struct{}
	cancelOnce sync.Once
	connection Connection
}

type logWriter struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	size    int64
	total   int64
	rotated bool
}

type completion struct {
	code int
	err  error
}

type outcome struct {
	state   string
	code    int
	message string
}
