package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/popoffvg/background/internal/store"
)

// makeTask fabricates a finished task in BG_HOME with controlled StartedAt /
// FinishedAt and an out.log of the given size. Returns the task dir.
func makeTask(t *testing.T, id string, started, finished time.Time, logSize int) string {
	t.Helper()
	dir, err := store.CreateTaskDir(id)
	if err != nil {
		t.Fatalf("CreateTaskDir: %v", err)
	}
	meta := store.Meta{ID: id, Cmd: "bash", StartedAt: started.Format(time.RFC3339)}
	if err := store.WriteMeta(dir, meta); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	if err := store.WriteStatus(dir, store.Status{ExitCode: 0, FinishedAt: finished.Format(time.RFC3339)}); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	if logSize >= 0 {
		data := make([]byte, logSize)
		if err := os.WriteFile(filepath.Join(dir, "out.log"), data, 0o644); err != nil {
			t.Fatalf("write out.log: %v", err)
		}
	}
	return dir
}

func TestStateUnknownWhenLogMissing(t *testing.T) {
	t.Setenv("BG_HOME", t.TempDir())
	id := store.NewID()
	dir, err := store.CreateTaskDir(id)
	if err != nil {
		t.Fatalf("CreateTaskDir: %v", err)
	}
	meta := store.Meta{ID: id, StartedAt: time.Now().Format(time.RFC3339)}
	if err := store.WriteMeta(dir, meta); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	// status.json exists (exit 0) but NO out.log.
	if err := store.WriteStatus(dir, store.Status{ExitCode: 0, FinishedAt: time.Now().Format(time.RFC3339)}); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	task := store.Task{Dir: dir, Meta: meta}
	if task.LogExists() {
		t.Errorf("LogExists() = true, want false")
	}
	if got := task.State(); got != "unknown" {
		t.Errorf("State() = %q, want unknown", got)
	}
	if got := task.StateCategory(); got != "unknown" {
		t.Errorf("StateCategory() = %q, want unknown", got)
	}
}

func TestGCAgePurgeKeepsStatus(t *testing.T) {
	t.Setenv("BG_HOME", t.TempDir())
	now := time.Now()
	oldDir := makeTask(t, "20200101-000000-old1", now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour), 100)
	newDir := makeTask(t, "20990101-000000-new1", now, now, 100)

	res, err := store.GC(store.GCOptions{MaxAge: 14 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if res.LogsPurged != 1 {
		t.Errorf("LogsPurged = %d, want 1", res.LogsPurged)
	}
	// old out.log gone, status.json kept
	if _, err := os.Stat(filepath.Join(oldDir, "out.log")); !os.IsNotExist(err) {
		t.Errorf("old out.log should be removed")
	}
	if _, err := os.Stat(filepath.Join(oldDir, "status.json")); err != nil {
		t.Errorf("old status.json should be kept: %v", err)
	}
	// new out.log untouched
	if _, err := os.Stat(filepath.Join(newDir, "out.log")); err != nil {
		t.Errorf("new out.log should be kept: %v", err)
	}
}

func TestGCSizeBudgetRemovesOldestFirst(t *testing.T) {
	t.Setenv("BG_HOME", t.TempDir())
	now := time.Now()
	oldDir := makeTask(t, "20990101-000001-a", now.Add(-3*time.Hour), now.Add(-3*time.Hour), 1000)
	midDir := makeTask(t, "20990101-000002-b", now.Add(-2*time.Hour), now.Add(-2*time.Hour), 1000)
	newDir := makeTask(t, "20990101-000003-c", now.Add(-1*time.Hour), now.Add(-1*time.Hour), 1000)

	// Budget 1500 bytes; 3×1000=3000 → must purge the two oldest to get to 1000.
	res, err := store.GC(store.GCOptions{MaxTotalBytes: 1500})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if res.LogsPurged != 2 {
		t.Errorf("LogsPurged = %d, want 2", res.LogsPurged)
	}
	if _, err := os.Stat(filepath.Join(oldDir, "out.log")); !os.IsNotExist(err) {
		t.Errorf("oldest out.log should be removed")
	}
	if _, err := os.Stat(filepath.Join(midDir, "out.log")); !os.IsNotExist(err) {
		t.Errorf("middle out.log should be removed")
	}
	if _, err := os.Stat(filepath.Join(newDir, "out.log")); err != nil {
		t.Errorf("newest out.log should be kept: %v", err)
	}
}

func TestGCHardAgeRemovesWholeDir(t *testing.T) {
	t.Setenv("BG_HOME", t.TempDir())
	now := time.Now()
	oldDir := makeTask(t, "20200101-000000-h", now.Add(-40*24*time.Hour), now.Add(-40*24*time.Hour), 100)
	newDir := makeTask(t, "20990101-000000-h", now, now, 100)

	res, err := store.GC(store.GCOptions{HardAge: 30 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if res.DirsDeleted != 1 {
		t.Errorf("DirsDeleted = %d, want 1", res.DirsDeleted)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("old dir should be removed entirely")
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Errorf("new dir should be kept: %v", err)
	}
}

func TestGCSkipsRunningTask(t *testing.T) {
	t.Setenv("BG_HOME", t.TempDir())
	now := time.Now()
	// A "running" task: NO status.json, pid = our own process (alive), with a log.
	id := store.NewID()
	dir, err := store.CreateTaskDir(id)
	if err != nil {
		t.Fatalf("CreateTaskDir: %v", err)
	}
	meta := store.Meta{ID: id, Pid: os.Getpid(), StartedAt: now.Add(-100 * 24 * time.Hour).Format(time.RFC3339)}
	if err := store.WriteMeta(dir, meta); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out.log"), make([]byte, 100), 0o644); err != nil {
		t.Fatalf("write out.log: %v", err)
	}
	task := store.Task{Dir: dir, Meta: meta}
	if task.State() != "running" {
		t.Fatalf("precondition: task should be running, got %q", task.State())
	}

	// Aggressive GC that would otherwise nuke an ancient task.
	res, err := store.GC(store.GCOptions{MaxAge: time.Hour, HardAge: time.Hour, MaxTotalBytes: 1})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if res.LogsPurged != 0 || res.DirsDeleted != 0 {
		t.Errorf("running task touched: purged=%d deleted=%d", res.LogsPurged, res.DirsDeleted)
	}
	if _, err := os.Stat(filepath.Join(dir, "out.log")); err != nil {
		t.Errorf("running task's out.log should be kept: %v", err)
	}
}

func TestGCDryRunChangesNothing(t *testing.T) {
	t.Setenv("BG_HOME", t.TempDir())
	now := time.Now()
	oldDir := makeTask(t, "20200101-000000-d", now.Add(-40*24*time.Hour), now.Add(-40*24*time.Hour), 100)

	res, err := store.GC(store.GCOptions{MaxAge: 14 * 24 * time.Hour, HardAge: 30 * 24 * time.Hour, DryRun: true})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	// Reports a non-zero plan...
	if res.LogsPurged == 0 && res.DirsDeleted == 0 {
		t.Errorf("dry-run should report a non-empty plan")
	}
	// ...but changes nothing on disk.
	if _, err := os.Stat(filepath.Join(oldDir, "out.log")); err != nil {
		t.Errorf("dry-run removed out.log: %v", err)
	}
	if _, err := os.Stat(oldDir); err != nil {
		t.Errorf("dry-run removed dir: %v", err)
	}
}

func TestGCMaxTasks(t *testing.T) {
	t.Setenv("BG_HOME", t.TempDir())
	now := time.Now()
	makeTask(t, "20990101-000001-t", now.Add(-3*time.Hour), now.Add(-3*time.Hour), 10)
	makeTask(t, "20990101-000002-t", now.Add(-2*time.Hour), now.Add(-2*time.Hour), 10)
	keep := makeTask(t, "20990101-000003-t", now.Add(-1*time.Hour), now.Add(-1*time.Hour), 10)

	res, err := store.GC(store.GCOptions{MaxTasks: 1})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if res.DirsDeleted != 2 {
		t.Errorf("DirsDeleted = %d, want 2", res.DirsDeleted)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("newest task should survive MaxTasks: %v", err)
	}
}
