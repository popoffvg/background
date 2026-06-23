package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/popoffvg/background/internal/adapter"
	"github.com/popoffvg/background/internal/runner"
	"github.com/popoffvg/background/internal/store"
)

// RunLaunch is the exported entry point for launching a task (called from main.go).
func RunLaunch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("command required after --")
	}

	command := args[0]
	cmdArgs := args[1:]

	// Best-effort: drop a starter adapters.yaml so users can discover overrides.
	_ = adapter.WriteDefaultUserConfig()

	// Detect a known AI CLI and inject structured-output flags.
	runArgs := cmdArgs
	renderer := ""
	if a := adapter.Match(command, cmdArgs); a != nil {
		runArgs = append(append([]string{}, cmdArgs...), a.Inject...)
		renderer = a.Renderer
	}

	id := store.NewID()
	dir, err := store.CreateTaskDir(id)
	if err != nil {
		return fmt.Errorf("create task dir: %w", err)
	}

	cwd, _ := os.Getwd()
	meta := store.Meta{
		ID:          id,
		Cmd:         command,
		Args:        runArgs,
		DisplayArgs: cmdArgs,
		Renderer:    renderer,
		Cwd:         cwd,
		Pid:         0,
		StartedAt:   time.Now().Format(time.RFC3339),
	}
	if err := store.WriteMeta(dir, meta); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}

	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	pid, err := runner.Launch(selfPath, dir, command, runArgs, nil)
	if err != nil {
		return fmt.Errorf("launch: %w", err)
	}

	// Update meta with supervisor pid
	meta.Pid = pid
	if err := store.WriteMeta(dir, meta); err != nil {
		return fmt.Errorf("update meta: %w", err)
	}

	fmt.Println(id)

	// Best-effort retention sweep with defaults. Synchronous (the CLI exits now,
	// so a goroutine would be killed) and bounded — GC only stats/acts on tasks
	// already over thresholds. Errors are ignored and nothing is printed.
	_, _ = store.GC(store.GCDefaults())

	return nil
}
