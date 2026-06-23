package cmd

import (
	"io"
	"os"
	"time"

	"github.com/popoffvg/background/internal/store"
	"github.com/spf13/cobra"
)

var logsFollow bool

var logsCmd = &cobra.Command{
	Use:   "logs <id>",
	Short: "Print logs of a task",
	Args:  cobra.ExactArgs(1),
	RunE:  runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	task, err := store.FindTask(args[0])
	if err != nil {
		return err
	}

	if !logsFollow {
		data, err := os.ReadFile(task.LogPath())
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	}

	// Follow mode: poll and print new data
	var offset int64
	for {
		f, err := os.Open(task.LogPath())
		if err != nil {
			if os.IsNotExist(err) {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			return err
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return err
		}
		written, err := io.Copy(os.Stdout, f)
		offset += written
		f.Close()
		if err != nil {
			return err
		}

		state := task.State()
		if state != "running" && written == 0 {
			// Task done and no new data
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}
