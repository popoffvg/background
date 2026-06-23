package cmd

import (
	"fmt"

	"github.com/popoffvg/background/internal/store"
	"github.com/spf13/cobra"
)

var (
	gcMaxAge   string
	gcMaxSize  string
	gcMaxTasks int
	gcHardAge  string
	gcDryRun   bool
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Purge old or oversized task logs",
	Long: `Garbage-collect finished tasks. Running tasks are never touched.

Rules apply in order: age purge (delete out.log), size budget (delete oldest
out.logs until under the limit), hard delete (remove whole dirs past hard-age),
and max-tasks (remove oldest dirs beyond the limit). Omitted flags use defaults
(also overridable via BG_GC_MAX_AGE / BG_GC_MAX_SIZE / BG_GC_HARD_AGE /
BG_GC_MAX_TASKS).`,
	RunE: runGC,
}

func init() {
	gcCmd.Flags().StringVar(&gcMaxAge, "max-age", "", "purge out.log older than this (e.g. 14d, 72h)")
	gcCmd.Flags().StringVar(&gcMaxSize, "max-size", "", "total out.log budget (e.g. 500MB, 1GB)")
	gcCmd.Flags().IntVar(&gcMaxTasks, "max-tasks", -1, "hard-delete oldest finished tasks beyond this count")
	gcCmd.Flags().StringVar(&gcHardAge, "hard-age", "", "remove whole task dir older than this (e.g. 30d)")
	gcCmd.Flags().BoolVar(&gcDryRun, "dry-run", false, "report what would be purged without changing anything")
	rootCmd.AddCommand(gcCmd)
}

func runGC(cmd *cobra.Command, args []string) error {
	opts := store.GCDefaults()
	opts.DryRun = gcDryRun

	if gcMaxAge != "" {
		d, err := store.ParseDuration(gcMaxAge)
		if err != nil {
			return fmt.Errorf("--max-age: %w", err)
		}
		opts.MaxAge = d
	}
	if gcMaxSize != "" {
		b, err := store.ParseBytes(gcMaxSize)
		if err != nil {
			return fmt.Errorf("--max-size: %w", err)
		}
		opts.MaxTotalBytes = b
	}
	if gcHardAge != "" {
		d, err := store.ParseDuration(gcHardAge)
		if err != nil {
			return fmt.Errorf("--hard-age: %w", err)
		}
		opts.HardAge = d
	}
	if gcMaxTasks >= 0 {
		opts.MaxTasks = gcMaxTasks
	}

	res, err := store.GC(opts)
	if err != nil {
		return err
	}

	prefix := ""
	if opts.DryRun {
		prefix = "[dry-run] would purge"
	} else {
		prefix = "purged"
	}
	fmt.Printf("%s %d logs, deleted %d task dirs, reclaimed %.1f MB\n",
		prefix, res.LogsPurged, res.DirsDeleted,
		float64(res.BytesReclaimed)/(1024*1024))
	return nil
}
