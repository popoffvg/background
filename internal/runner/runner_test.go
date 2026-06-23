package runner_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/popoffvg/background/internal/runner"
	"github.com/popoffvg/background/internal/store"
)

// A command that cannot start (e.g. the shell builtin `exit`, which is not a
// real binary) must still record a non-zero status instead of leaving the task
// hanging in "unknown".
func TestSuperviseStartFailureWritesStatus(t *testing.T) {
	t.Setenv("BG_HOME", t.TempDir())

	id := store.NewID()
	dir, err := store.CreateTaskDir(id)
	if err != nil {
		t.Fatalf("CreateTaskDir: %v", err)
	}
	meta := store.Meta{ID: id, Cmd: "exit", Args: []string{"1"}, StartedAt: time.Now().Format(time.RFC3339)}
	if err := store.WriteMeta(dir, meta); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	// Supervise returns an error (start failed) but must have written status.json.
	if err := runner.Supervise(dir, "exit", []string{"1"}); err == nil {
		t.Fatalf("expected start error for non-binary %q", "exit")
	}

	st, exists, err := store.ReadStatus(dir)
	if err != nil {
		t.Fatalf("reading status.json: %v", err)
	}
	if !exists {
		t.Fatalf("status.json not written after start failure")
	}
	if st.ExitCode == 0 {
		t.Errorf("ExitCode: got 0, want non-zero for failed start")
	}

	// The task is no longer "running"/"unknown-hanging": it has a finished status.
	task := store.Task{Dir: dir, Meta: meta}
	if state := task.State(); state == "running" {
		t.Errorf("State: got %q, want a finished/exited state", state)
	}

	// The log explains why it failed.
	logBytes, _ := os.ReadFile(filepath.Join(dir, "out.log"))
	if len(logBytes) == 0 {
		t.Errorf("expected an explanatory line in out.log, got empty")
	}
}
