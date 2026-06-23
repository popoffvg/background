package launcher_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/popoffvg/background/internal/launcher"
	"github.com/popoffvg/background/internal/store"
)

func TestLaunchWithEnvFileVar(t *testing.T) {
	self := os.Getenv("BG_SELF")
	if self == "" {
		t.Skip("set BG_SELF to bin/bg path to run end-to-end launch test")
	}

	tmp := t.TempDir()
	t.Setenv("BG_HOME", tmp)

	// source log file with 3 lines
	srcLog := filepath.Join(tmp, "source.log")
	if err := os.WriteFile(srcLog, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// transform: wc -l "$FILE" → should print 3
	command := `wc -l "$FILE" > "` + filepath.Join(tmp, "result.txt") + `"`
	id, err := launcher.LaunchWithEnv("sh", []string{"-c", command}, []string{"-c", command}, []string{"FILE=" + srcLog}, "src-1")
	if err != nil {
		t.Fatalf("LaunchWithEnv: %v", err)
	}
	if id == "" {
		t.Fatal("empty id")
	}

	// wait for completion
	task, err := store.FindTask(id)
	if err != nil {
		t.Fatalf("FindTask: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, exists, _ := store.ReadStatus(task.Dir); exists {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	out, err := os.ReadFile(filepath.Join(tmp, "result.txt"))
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	got := string(out)
	if !contains(got, "3") {
		t.Fatalf("expected wc to report 3 lines, got %q", got)
	}
	// SourceID recorded
	m, _ := store.ReadMeta(task.Dir)
	if m.SourceID != "src-1" {
		t.Errorf("SourceID = %q want src-1", m.SourceID)
	}
}

func TestRelaunch(t *testing.T) {
	self := os.Getenv("BG_SELF")
	if self == "" {
		t.Skip("set BG_SELF to bin/bg path to run end-to-end launch test")
	}

	tmp := t.TempDir()
	t.Setenv("BG_HOME", tmp)

	// Source task: write known output to a result file in its cwd so we can
	// confirm the relaunch reuses src.Cwd.
	srcCwd := t.TempDir()
	command := `printf 'repeated-ok\n' > out.txt`
	srcID, err := launcher.LaunchWithEnv("sh", []string{"-c", command}, []string{"-c", command}, nil, "")
	if err != nil {
		t.Fatalf("LaunchWithEnv: %v", err)
	}
	srcTask, err := store.FindTask(srcID)
	if err != nil {
		t.Fatalf("FindTask src: %v", err)
	}
	srcMeta, _ := store.ReadMeta(srcTask.Dir)
	// Force a known cwd to prove Relaunch reuses it.
	srcMeta.Cwd = srcCwd
	if err := store.WriteMeta(srcTask.Dir, srcMeta); err != nil {
		t.Fatalf("rewrite src meta: %v", err)
	}

	// Relaunch.
	newID, err := launcher.Relaunch(srcMeta)
	if err != nil {
		t.Fatalf("Relaunch: %v", err)
	}
	if newID == srcID {
		t.Fatal("Relaunch should produce a new id")
	}

	newTask, err := store.FindTask(newID)
	if err != nil {
		t.Fatalf("FindTask new: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, exists, _ := store.ReadStatus(newTask.Dir); exists {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	newMeta, _ := store.ReadMeta(newTask.Dir)
	if newMeta.Cmd != srcMeta.Cmd {
		t.Errorf("Cmd = %q want %q", newMeta.Cmd, srcMeta.Cmd)
	}
	if len(newMeta.DisplayArgs) != len(srcMeta.DisplayArgs) {
		t.Errorf("DisplayArgs = %v want %v", newMeta.DisplayArgs, srcMeta.DisplayArgs)
	} else {
		for i := range newMeta.DisplayArgs {
			if newMeta.DisplayArgs[i] != srcMeta.DisplayArgs[i] {
				t.Errorf("DisplayArgs[%d] = %q want %q", i, newMeta.DisplayArgs[i], srcMeta.DisplayArgs[i])
			}
		}
	}
	if newMeta.Cwd != srcCwd {
		t.Errorf("Cwd = %q want %q (src.Cwd)", newMeta.Cwd, srcCwd)
	}
	if newMeta.SourceID != srcID {
		t.Errorf("SourceID = %q want %q", newMeta.SourceID, srcID)
	}

	// The relaunched command wrote out.txt in srcCwd.
	out, err := os.ReadFile(filepath.Join(srcCwd, "out.txt"))
	if err != nil {
		t.Fatalf("read relaunched output: %v", err)
	}
	if !contains(string(out), "repeated-ok") {
		t.Fatalf("relaunched output = %q want contains repeated-ok", string(out))
	}
}

func contains(s, sub string) bool {
	return len(s) > 0 && (len(sub) == 0 || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
