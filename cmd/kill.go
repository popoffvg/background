package cmd

import (
	"fmt"

	"github.com/popoffvg/background/internal/runner"
	"github.com/popoffvg/background/internal/store"
	"github.com/spf13/cobra"
)

var killCmd = &cobra.Command{
	Use:   "kill <id>",
	Short: "Terminate a running task",
	Args:  cobra.ExactArgs(1),
	RunE:  runKill,
}

func init() {
	rootCmd.AddCommand(killCmd)
}

func runKill(cmd *cobra.Command, args []string) error {
	task, err := store.FindTask(args[0])
	if err != nil {
		return err
	}

	if task.State() != "running" {
		return fmt.Errorf("task %s is not running (state: %s)", task.Meta.ID, task.State())
	}

	if err := runner.Kill(task.Meta.Pid); err != nil {
		return fmt.Errorf("kill: %w", err)
	}

	fmt.Printf("Sent kill signal to task %s (pid %d)\n", task.Meta.ID, task.Meta.Pid)
	return nil
}
