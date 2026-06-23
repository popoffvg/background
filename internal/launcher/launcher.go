// Package launcher provides a high-level entry point for launching detached
// tasks with extra environment variables and provenance metadata. It lives in
// its own package so both cmd and internal/tui can use it without an import
// cycle.
package launcher

import (
	"os"
	"time"

	"github.com/popoffvg/background/internal/adapter"
	"github.com/popoffvg/background/internal/runner"
	"github.com/popoffvg/background/internal/store"
)

// selfPath returns the binary to re-exec for the supervisor. BG_SELF overrides
// os.Executable() (used by tests that build bin/bg separately).
func selfPath() (string, error) {
	if p := os.Getenv("BG_SELF"); p != "" {
		return p, nil
	}
	return os.Executable()
}

// launch writes meta, spawns the detached supervisor, and records the pid.
// It is the shared tail used by LaunchWithEnv and Relaunch.
func launch(meta store.Meta, extraEnv []string) (string, error) {
	dir, err := store.CreateTaskDir(meta.ID)
	if err != nil {
		return "", err
	}
	if err := store.WriteMeta(dir, meta); err != nil {
		return "", err
	}
	self, err := selfPath()
	if err != nil {
		return "", err
	}
	pid, err := runner.Launch(self, dir, meta.Cmd, meta.Args, extraEnv)
	if err != nil {
		return "", err
	}
	meta.Pid = pid
	if err := store.WriteMeta(dir, meta); err != nil {
		return "", err
	}
	return meta.ID, nil
}

// LaunchWithEnv launches `command args...` as a detached task carrying extra
// env vars (KEY=VAL strings) and an optional source task id for traceability.
// Returns the new task id. displayArgs sets the user-facing command. Cwd is the
// caller's current working directory.
//
// The supervisor re-exec self-path is os.Executable() by default; set the
// BG_SELF env var to override it (used by tests that build bin/bg separately).
func LaunchWithEnv(command string, args []string, displayArgs []string, extraEnv []string, sourceID string) (string, error) {
	cwd, _ := os.Getwd()
	meta := store.Meta{
		ID:          store.NewID(),
		Cmd:         command,
		Args:        args,
		DisplayArgs: displayArgs,
		Cwd:         cwd,
		SourceID:    sourceID,
		StartedAt:   time.Now().Format(time.RFC3339),
	}
	return launch(meta, extraEnv)
}

// Relaunch re-runs a previous task's command as a brand-new task. It recomputes
// adapter injection (so the repeat reflects current adapter config) and reuses
// the source task's working directory. The new task records SourceID = src.ID
// for provenance. Returns the new task id.
func Relaunch(src store.Meta) (string, error) {
	displayArgs := src.DisplayArgs
	if displayArgs == nil {
		displayArgs = src.Args
	}
	runArgs := displayArgs
	renderer := ""
	if a := adapter.Match(src.Cmd, displayArgs); a != nil {
		runArgs = append(append([]string{}, displayArgs...), a.Inject...)
		renderer = a.Renderer
	}
	meta := store.Meta{
		ID:          store.NewID(),
		Cmd:         src.Cmd,
		Args:        runArgs,
		DisplayArgs: displayArgs,
		Renderer:    renderer,
		Cwd:         src.Cwd,
		SourceID:    src.ID,
		StartedAt:   time.Now().Format(time.RFC3339),
	}
	return launch(meta, nil)
}
