package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/popoffvg/background/internal/store"
	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List background tasks (non-interactive)",
	RunE:  runLs,
}

func init() {
	rootCmd.AddCommand(lsCmd)
}

func runLs(cmd *cobra.Command, args []string) error {
	tasks, err := store.ListTasks()
	if err != nil {
		return err
	}

	fi, _ := os.Stdout.Stat()
	isTTY := fi != nil && fi.Mode()&os.ModeCharDevice != 0

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tSTARTED\tAGE\tCOMMAND")

	for _, t := range tasks {
		status := colorStatus(t.State(), t.StateCategory(), isTTY)
		started := startedLabel(t.Meta.StartedAt)
		age := relAge(t.Meta.StartedAt)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.Meta.ID, status, started, age, t.Label())
	}

	return w.Flush()
}

func colorStatus(state, category string, isTTY bool) string {
	if !isTTY {
		return state
	}
	var code string
	switch category {
	case "ok":
		code = "32" // green
	case "fail":
		code = "31" // red
	case "running":
		code = "34" // blue
	default:
		code = "90" // grey
	}
	return "\x1b[" + code + "m" + state + "\x1b[0m"
}

func startedLabel(startedAt string) string {
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return "?"
	}
	t = t.Local()
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04:05")
	}
	return t.Format("Jan 02 15:04")
}

func relAge(startedAt string) string {
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return startedAt
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}
