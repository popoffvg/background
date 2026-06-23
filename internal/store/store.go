package store

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const letters = "abcdefghijklmnopqrstuvwxyz0123456789"

func randSuffix(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// BaseDir returns the root tasks directory, respecting BG_HOME env var.
func BaseDir() (string, error) {
	base := os.Getenv("BG_HOME")
	if base == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("user cache dir: %w", err)
		}
		base = filepath.Join(cacheDir, "bg", "tasks")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", fmt.Errorf("mkdir base dir: %w", err)
	}
	return base, nil
}

// NewID generates a chronologically-sortable task id.
func NewID() string {
	return time.Now().Format("20060102-150405") + "-" + randSuffix(4)
}

// Meta holds task metadata stored in meta.json.
type Meta struct {
	ID          string   `json:"id"`
	Cmd         string   `json:"cmd"`
	Args        []string `json:"args"`
	DisplayArgs []string `json:"displayArgs,omitempty"`
	Renderer    string   `json:"renderer,omitempty"`
	Cwd         string   `json:"cwd"`
	Pid         int      `json:"pid"`
	SourceID    string   `json:"sourceId,omitempty"`
	Alias       string   `json:"alias,omitempty"`
	StartedAt   string   `json:"startedAt"`
}

// Status holds completion info stored in status.json.
type Status struct {
	ExitCode   int    `json:"exitCode"`
	FinishedAt string `json:"finishedAt"`
	Signaled   bool   `json:"signaled"`
}

// Task represents a task with its directory.
type Task struct {
	Dir  string
	Meta Meta
}

// WriteMeta writes meta.json to the task directory.
func WriteMeta(dir string, m Meta) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "meta.json"), data, 0o644)
}

// ReadMeta reads meta.json from the task directory.
func ReadMeta(dir string) (Meta, error) {
	var m Meta
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

// WriteStatus writes status.json to the task directory.
func WriteStatus(dir string, s Status) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "status.json"), data, 0o644)
}

// ReadStatus reads status.json from the task directory.
func ReadStatus(dir string) (Status, bool, error) {
	var s Status
	data, err := os.ReadFile(filepath.Join(dir, "status.json"))
	if os.IsNotExist(err) {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	return s, true, json.Unmarshal(data, &s)
}

// CreateTaskDir creates a new task directory and returns its path.
func CreateTaskDir(id string) (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir task dir: %w", err)
	}
	return dir, nil
}

// LogExists reports whether the task's out.log file is present on disk.
func (t Task) LogExists() bool {
	_, err := os.Stat(t.LogPath())
	return err == nil
}

// State returns human-readable state of the task. A missing out.log forces
// "unknown" regardless of status (the log was removed by GC or manually).
func (t Task) State() string {
	if !t.LogExists() {
		return "unknown"
	}
	st, exists, err := ReadStatus(t.Dir)
	if err == nil && exists {
		if st.Signaled {
			return "killed"
		}
		return fmt.Sprintf("exited(%d)", st.ExitCode)
	}
	// Check if process is alive
	if t.Meta.Pid > 0 {
		err := syscall.Kill(t.Meta.Pid, 0)
		if err == nil {
			return "running"
		}
	}
	return "dead"
}

// StateCategory returns one of: "ok", "fail", "running", "unknown". A missing
// out.log forces "unknown" (grey) regardless of status.
func (t Task) StateCategory() string {
	if !t.LogExists() {
		return "unknown"
	}
	st, exists, err := ReadStatus(t.Dir)
	if err == nil && exists {
		if st.Signaled {
			return "fail"
		}
		if st.ExitCode == 0 {
			return "ok"
		}
		return "fail"
	}
	if t.Meta.Pid > 0 {
		if syscall.Kill(t.Meta.Pid, 0) == nil {
			return "running"
		}
	}
	return "unknown"
}

// DisplayCommand returns the user-facing command string (original args).
func (t Task) DisplayCommand() string {
	args := t.Meta.DisplayArgs
	if args == nil {
		args = t.Meta.Args
	}
	cmd := t.Meta.Cmd
	if len(args) > 0 {
		cmd += " " + strings.Join(args, " ")
	}
	return cmd
}

// Label returns the task's display label: its alias if set, else the command.
func (t Task) Label() string {
	if t.Meta.Alias != "" {
		return t.Meta.Alias
	}
	return t.DisplayCommand()
}

// SetAlias persists a user-friendly alias onto the task's meta.json.
func SetAlias(dir, alias string) error {
	m, err := ReadMeta(dir)
	if err != nil {
		return err
	}
	m.Alias = alias
	return WriteMeta(dir, m)
}

// LogPath returns the path to the task's log file.
func (t Task) LogPath() string {
	return filepath.Join(t.Dir, "out.log")
}

// ListTasks returns all tasks sorted newest first.
func ListTasks() ([]Task, error) {
	base, err := BaseDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var tasks []Task
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(base, e.Name())
		m, err := ReadMeta(dir)
		if err != nil {
			continue
		}
		tasks = append(tasks, Task{Dir: dir, Meta: m})
	}
	// Sort newest first (IDs are time-based strings, reverse lexicographic)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Meta.ID > tasks[j].Meta.ID
	})
	return tasks, nil
}

// FindTask finds a task by id prefix or full id.
func FindTask(id string) (Task, error) {
	base, err := BaseDir()
	if err != nil {
		return Task{}, err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return Task{}, err
	}
	var matches []Task
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == id || (len(id) >= 4 && len(name) >= len(id) && name[:len(id)] == id) {
			dir := filepath.Join(base, name)
			m, err := ReadMeta(dir)
			if err != nil {
				continue
			}
			matches = append(matches, Task{Dir: dir, Meta: m})
		}
	}
	if len(matches) == 0 {
		return Task{}, fmt.Errorf("task %q not found", id)
	}
	if len(matches) > 1 {
		return Task{}, fmt.Errorf("ambiguous id %q matches multiple tasks", id)
	}
	return matches[0], nil
}
