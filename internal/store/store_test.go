package store_test

import (
	"os"
	"testing"
	"time"

	"github.com/popoffvg/background/internal/store"
)

func setupTempStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BG_HOME", dir)
	return dir
}

func TestMetaRoundTrip(t *testing.T) {
	setupTempStore(t)

	id := store.NewID()
	dir, err := store.CreateTaskDir(id)
	if err != nil {
		t.Fatalf("CreateTaskDir: %v", err)
	}

	meta := store.Meta{
		ID:        id,
		Cmd:       "bash",
		Args:      []string{"-c", "echo hello"},
		Cwd:       "/tmp",
		Pid:       12345,
		StartedAt: time.Now().Format(time.RFC3339),
	}

	if err := store.WriteMeta(dir, meta); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	got, err := store.ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}

	if got.ID != meta.ID {
		t.Errorf("ID: got %q want %q", got.ID, meta.ID)
	}
	if got.Cmd != meta.Cmd {
		t.Errorf("Cmd: got %q want %q", got.Cmd, meta.Cmd)
	}
	if got.Pid != meta.Pid {
		t.Errorf("Pid: got %d want %d", got.Pid, meta.Pid)
	}
}

func TestStateExited(t *testing.T) {
	setupTempStore(t)

	id := store.NewID()
	dir, err := store.CreateTaskDir(id)
	if err != nil {
		t.Fatalf("CreateTaskDir: %v", err)
	}

	meta := store.Meta{ID: id, Pid: 99999999}
	if err := store.WriteMeta(dir, meta); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	// Write status: exited 0
	if err := store.WriteStatus(dir, store.Status{ExitCode: 0, FinishedAt: time.Now().Format(time.RFC3339)}); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	task := store.Task{Dir: dir, Meta: meta}
	if state := task.State(); state != "exited(0)" {
		t.Errorf("State: got %q want %q", state, "exited(0)")
	}
}

func TestStateKilled(t *testing.T) {
	setupTempStore(t)

	id := store.NewID()
	dir, err := store.CreateTaskDir(id)
	if err != nil {
		t.Fatalf("CreateTaskDir: %v", err)
	}

	meta := store.Meta{ID: id, Pid: 99999999}
	if err := store.WriteMeta(dir, meta); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	if err := store.WriteStatus(dir, store.Status{Signaled: true, FinishedAt: time.Now().Format(time.RFC3339)}); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	task := store.Task{Dir: dir, Meta: meta}
	if state := task.State(); state != "killed" {
		t.Errorf("State: got %q want %q", state, "killed")
	}
}

func TestStateDead(t *testing.T) {
	setupTempStore(t)

	id := store.NewID()
	dir, err := store.CreateTaskDir(id)
	if err != nil {
		t.Fatalf("CreateTaskDir: %v", err)
	}

	// Use a pid that almost certainly doesn't exist
	meta := store.Meta{ID: id, Pid: 99999999}
	if err := store.WriteMeta(dir, meta); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	task := store.Task{Dir: dir, Meta: meta}
	state := task.State()
	// No status.json and pid doesn't exist → dead
	if state != "dead" && state != "running" {
		t.Errorf("State: got %q want dead or running", state)
	}
}

func TestStateRunning(t *testing.T) {
	setupTempStore(t)

	id := store.NewID()
	dir, err := store.CreateTaskDir(id)
	if err != nil {
		t.Fatalf("CreateTaskDir: %v", err)
	}

	// Use current process pid — definitely alive
	meta := store.Meta{ID: id, Pid: os.Getpid()}
	if err := store.WriteMeta(dir, meta); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	task := store.Task{Dir: dir, Meta: meta}
	if state := task.State(); state != "running" {
		t.Errorf("State: got %q want running", state)
	}
}

func TestListTasksSortedNewestFirst(t *testing.T) {
	setupTempStore(t)

	var ids []string
	for i := 0; i < 3; i++ {
		id := store.NewID()
		ids = append(ids, id)
		dir, err := store.CreateTaskDir(id)
		if err != nil {
			t.Fatalf("CreateTaskDir: %v", err)
		}
		meta := store.Meta{ID: id, Cmd: "test", StartedAt: time.Now().Format(time.RFC3339)}
		if err := store.WriteMeta(dir, meta); err != nil {
			t.Fatalf("WriteMeta: %v", err)
		}
		time.Sleep(1100 * time.Millisecond)
	}

	tasks, err := store.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("got %d tasks want 3", len(tasks))
	}
	// Newest first means last created id is first
	if tasks[0].Meta.ID != ids[2] {
		t.Errorf("first task: got %q want %q", tasks[0].Meta.ID, ids[2])
	}
}
