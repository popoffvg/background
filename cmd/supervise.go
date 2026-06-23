package cmd

import (
	"fmt"

	"github.com/popoffvg/background/internal/runner"
	"github.com/spf13/cobra"
)

var superviseCmd = &cobra.Command{
	Use:                "__supervise <taskDir> -- <command> [args...]",
	Hidden:             true,
	Short:              "Internal: supervise a background task",
	DisableFlagParsing: true,
	RunE:               runSupervise,
}

func init() {
	rootCmd.AddCommand(superviseCmd)
}

func runSupervise(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: __supervise <taskDir> -- <command> [args...]")
	}

	taskDir := args[0]
	// Find the -- separator
	cmdArgs := args[1:]
	start := 0
	for i, a := range cmdArgs {
		if a == "--" {
			start = i + 1
			break
		}
	}

	if start >= len(cmdArgs) || len(cmdArgs[start:]) == 0 {
		return fmt.Errorf("command required after --")
	}

	command := cmdArgs[start]
	commandArgs := cmdArgs[start+1:]

	return runner.Supervise(taskDir, command, commandArgs)
}
