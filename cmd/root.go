package cmd

import (
	"github.com/popoffvg/background/internal/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "bg",
	Short: "Run commands as detached background tasks",
	Long:  `bg runs commands as detached background tasks and provides a TUI to monitor them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run()
	},
}

func Execute() error {
	return rootCmd.Execute()
}
