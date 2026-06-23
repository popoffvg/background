package cmd

import (
	"fmt"
	"os"

	"github.com/popoffvg/background/internal/store"
	"github.com/spf13/cobra"
)

var rmForce bool

var rmCmd = &cobra.Command{
	Use:   "rm <id>",
	Short: "Delete a finished task directory",
	Args:  cobra.ExactArgs(1),
	RunE:  runRm,
}

func init() {
	rmCmd.Flags().BoolVar(&rmForce, "force", false, "Force delete even if running")
	rootCmd.AddCommand(rmCmd)
}

func runRm(cmd *cobra.Command, args []string) error {
	task, err := store.FindTask(args[0])
	if err != nil {
		return err
	}

	if task.State() == "running" && !rmForce {
		return fmt.Errorf("task %s is still running; use --force to delete anyway", task.Meta.ID)
	}

	if err := os.RemoveAll(task.Dir); err != nil {
		return fmt.Errorf("remove: %w", err)
	}

	fmt.Printf("Deleted task %s\n", task.Meta.ID)
	return nil
}
