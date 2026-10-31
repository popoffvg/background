package launch_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/popoffvg/background/internal/store"
	"github.com/popoffvg/background/launch"
)

func TestRunRecordsTask(t *testing.T) {
	t.Setenv("BG_HOME", t.TempDir())
	var stdout bytes.Buffer
	result, err := launch.Run(t.Context(), "sh", []string{"-c", "cat; echo err >&2; exit 3"}, launch.Options{
		Stdin:  strings.NewReader("hello\n"),
		Stdout: &stdout,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", result.ExitCode)
	}
	if stdout.String() != "hello\n" {
		t.Fatalf("stdout = %q, want %q", stdout.String(), "hello\n")
	}
	task, err := store.FindTask(result.TaskID)
	if err != nil {
		t.Fatalf("FindTask: %v", err)
	}
	log, err := os.ReadFile(filepath.Join(task.Dir, "out.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "hello") || !strings.Contains(string(log), "err") {
		t.Fatalf("out.log = %q, want stdout and stderr", log)
	}
	status, exists, err := store.ReadStatus(task.Dir)
	if err != nil || !exists || status.ExitCode != 3 {
		t.Fatalf("status = %+v exists=%v err=%v, want exit 3", status, exists, err)
	}
	if task.Meta.Pid == 0 {
		t.Fatal("meta pid not recorded")
	}
}

func TestRunCanceled(t *testing.T) {
	t.Setenv("BG_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := launch.Run(ctx, "sleep", []string{"5"}, launch.Options{}); err == nil {
		t.Fatal("Run with a canceled context returned no error")
	}
}
