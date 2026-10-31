// Package launch runs a command for another Go program and records it as a bg task, so
// `bg ls`, `bg logs`, and the TUI show it. The caller waits for the exit and reads the
// output itself; no supervisor process is started.
package launch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/popoffvg/background/internal/runner"
	"github.com/popoffvg/background/internal/store"
)

// Options configures one Run. Every field is optional.
type Options struct {
	// Dir is the working directory; empty uses the caller's.
	Dir string
	// Env holds KEY=VAL pairs added to the caller's environment.
	Env []string
	// Stdin feeds the command; nil gives it no input.
	Stdin io.Reader
	// Stdout and Stderr receive the output as well as the task log.
	Stdout io.Writer
	Stderr io.Writer
	// SourceID names the task this run comes from, as `bg ls` shows it.
	SourceID string
}

// Result names the bg task and the command's exit code.
type Result struct {
	TaskID   string
	ExitCode int
}

// Run starts `command args...`, waits for it, and returns its exit code. A non-zero exit
// is not an error. When ctx ends, Run sends SIGTERM to the process group and returns ctx.Err().
func Run(ctx context.Context, command string, args []string, opts Options) (Result, error) {
	cwd := opts.Dir
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	meta := store.Meta{
		ID:        store.NewID(),
		Cmd:       command,
		Args:      args,
		Cwd:       cwd,
		SourceID:  opts.SourceID,
		StartedAt: time.Now().Format(time.RFC3339),
	}
	result := Result{TaskID: meta.ID}
	dir, err := store.CreateTaskDir(meta.ID)
	if err != nil {
		return result, fmt.Errorf("launch: create task dir: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(dir, "out.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return result, fmt.Errorf("launch: open log: %w", err)
	}
	defer logFile.Close()
	taskLog := runner.NewTaskLogWriter(logFile)

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = cwd
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}
	cmd.Stdin = opts.Stdin
	cmd.Stdout = teeTaskLog(taskLog, opts.Stdout)
	cmd.Stderr = teeTaskLog(taskLog, opts.Stderr)
	runner.SetPgid(cmd)
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }

	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(logFile, "bg: failed to start command %q: %v\n", command, err)
		_ = store.WriteStatus(dir, store.Status{ExitCode: 127, FinishedAt: time.Now().Format(time.RFC3339)})
		return result, fmt.Errorf("launch: start %s: %w", command, err)
	}
	meta.Pid = cmd.Process.Pid
	if err := store.WriteMeta(dir, meta); err != nil {
		return result, fmt.Errorf("launch: write meta: %w", err)
	}

	waitErr := cmd.Wait()
	status := runner.ExitStatus(waitErr)
	result.ExitCode = status.ExitCode
	if err := store.WriteStatus(dir, status); err != nil {
		return result, fmt.Errorf("launch: write status: %w", err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, ctxErr
	}
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return result, fmt.Errorf("launch: wait %s: %w", command, waitErr)
	}
	return result, nil
}

func teeTaskLog(taskLog io.Writer, caller io.Writer) io.Writer {
	if caller == nil {
		return taskLog
	}
	return io.MultiWriter(caller, taskLog)
}
