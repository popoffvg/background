package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/popoffvg/background/internal/adapter"
	"github.com/popoffvg/background/internal/store"
)

// Supervise runs the target command, writing logs and status to taskDir.
// This is called by the hidden __supervise subcommand.
func Supervise(taskDir string, command string, args []string) error {
	logFile, err := os.OpenFile(filepath.Join(taskDir, "out.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()

	// Read meta first to learn the renderer (set at launch time).
	meta, err := store.ReadMeta(taskDir)
	if err != nil {
		return fmt.Errorf("read meta: %w", err)
	}
	rendererName := meta.Renderer
	useRenderer := rendererName != "" && rendererName != "raw"

	// One shared capped writer guards the per-task disk budget. Both stdout and
	// stderr (and the renderer output) feed it, so the cap is a total. It is
	// mutex-guarded for concurrent writers (stderr + renderer goroutine).
	capped := newCappedWriter(logFile, maxLogBytes())

	cmd := exec.Command(command, args...)
	cmd.Stdin = nil
	// Run in the task's recorded working directory when set (empty = inherit the
	// supervisor's cwd, preserving prior behavior).
	if meta.Cwd != "" {
		cmd.Dir = meta.Cwd
	}

	var pr, pw *os.File
	done := make(chan struct{})
	if useRenderer {
		pr, pw, err = os.Pipe()
		if err != nil {
			return fmt.Errorf("pipe: %w", err)
		}
		cmd.Stdout = pw
		cmd.Stderr = capped // raw stderr straight to the capped log
	} else {
		cmd.Stdout = capped
		cmd.Stderr = capped
	}
	SetPgid(cmd)

	if err := cmd.Start(); err != nil {
		if pw != nil {
			pw.Close()
		}
		if pr != nil {
			pr.Close()
		}
		// The command never started (e.g. not a real binary — `exit` is a shell
		// builtin, not an executable). Record a failure status so the task shows
		// a non-zero exit instead of hanging forever in "unknown". 127 is the
		// conventional shell "command not found / cannot execute" code.
		fmt.Fprintf(logFile, "bg: failed to start command %q: %v\n", command, err)
		_ = store.WriteStatus(taskDir, store.Status{
			ExitCode:   127,
			FinishedAt: time.Now().Format(time.RFC3339),
		})
		return fmt.Errorf("start command: %w", err)
	}

	if useRenderer {
		// Parent closes its copy of the write end so the reader sees EOF
		// when the child exits; the child keeps its inherited fd.
		pw.Close()
		go func() {
			defer close(done)
			_ = adapter.GetRenderer(rendererName).Render(pr, capped)
			pr.Close()
		}()
	}

	// Update meta.json with child's real pid
	meta.Pid = cmd.Process.Pid
	if err := store.WriteMeta(taskDir, meta); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}

	waitErr := cmd.Wait()
	if useRenderer {
		<-done
	}

	return store.WriteStatus(taskDir, ExitStatus(waitErr))
}

// ExitStatus turns the error of exec.Cmd.Wait into the status.json record.
// An error that is not an exit error counts as exit code 1.
func ExitStatus(waitErr error) store.Status {
	status := store.Status{
		FinishedAt: time.Now().Format(time.RFC3339),
	}
	var exitErr *exec.ExitError
	if waitErr != nil {
		if errors.As(waitErr, &exitErr) {
			status.ExitCode = exitErr.ExitCode()
			status.Signaled = exitErr.ExitCode() == -1
		} else {
			status.ExitCode = 1
		}
	}
	return status
}

// Launch starts the supervisor as a detached process.
// It re-execs the current binary with __supervise subcommand.
// Returns the supervisor's pid.
// extraEnv (KEY=VAL strings), when non-empty, is appended to the supervisor's
// environment; the supervised child inherits it (Supervise leaves child Env nil
// → inherits the supervisor process env). nil keeps the prior behavior.
func Launch(selfPath string, taskDir string, command string, args []string, extraEnv []string) (int, error) {
	supervisorArgs := append([]string{"__supervise", taskDir, "--"}, append([]string{command}, args...)...)
	cmd := exec.Command(selfPath, supervisorArgs...)
	cmd.Stdin, _ = os.Open(os.DevNull)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	SetSetsid(cmd)

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start supervisor: %w", err)
	}

	return cmd.Process.Pid, nil
}

// Kill sends SIGTERM then SIGKILL to the process group.
func Kill(pid int) error {
	pgid := -pid
	// Send SIGTERM to process group
	if err := syscall.Kill(pgid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("SIGTERM: %w", err)
	}
	// Wait up to 2 seconds
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if err := syscall.Kill(pgid, 0); err != nil {
			// Process gone
			return nil
		}
	}
	// SIGKILL
	if err := syscall.Kill(pgid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("SIGKILL: %w", err)
	}
	return nil
}

// UpdatePid updates the pid field in meta.json.
func UpdatePid(taskDir string, pid int) error {
	data, err := os.ReadFile(filepath.Join(taskDir, "meta.json"))
	if err != nil {
		return err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	m["pid"] = pid
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(taskDir, "meta.json"), out, 0o644)
}
