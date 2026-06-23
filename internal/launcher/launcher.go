// Package launcher provides a high-level entry point for launching detached
// tasks with extra environment variables and provenance metadata. It lives in
// its own package so both cmd and internal/tui can use it without an import
// cycle.
package launcher

import (
	"os"
	"time"

	"github.com/popoffvg/background/internal/runner"
	"github.com/popoffvg/background/internal/store"
)

// LaunchWithEnv launches `command args...` as a detached task carrying extra
// env vars (KEY=VAL strings) and an optional source task id for traceability.
// Returns the new task id. displayArgs sets the user-facing command.
//
// The supervisor re-exec self-path is os.Executable() by default; set the
// BG_SELF env var to override it (used by tests that build bin/bg separately).
func LaunchWithEnv(command string, args []string, displayArgs []string, extraEnv []string, sourceID string) (string, error) {
	id := store.NewID()
	dir, err := store.CreateTaskDir(id)
	if err != nil {
		return "", err
	}
	cwd, _ := os.Getwd()
	meta := store.Meta{
		ID:          id,
		Cmd:         command,
		Args:        args,
		DisplayArgs: displayArgs,
		Cwd:         cwd,
		SourceID:    sourceID,
		StartedAt:   time.Now().Format(time.RFC3339),
	}
	if err := store.WriteMeta(dir, meta); err != nil {
		return "", err
	}
	selfPath := os.Getenv("BG_SELF")
	if selfPath == "" {
		selfPath, err = os.Executable()
		if err != nil {
			return "", err
		}
	}
	pid, err := runner.Launch(selfPath, dir, command, args, extraEnv)
	if err != nil {
		return "", err
	}
	meta.Pid = pid
	if err := store.WriteMeta(dir, meta); err != nil {
		return "", err
	}
	return id, nil
}
