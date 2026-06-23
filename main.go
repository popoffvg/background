package main

import (
	"fmt"
	"os"

	"github.com/popoffvg/background/cmd"
)

func main() {
	// Handle 'bg -- cmd args...' directly before cobra routing.
	// cobra treats '--' as flag terminator; we intercept it here.
	// Skip if first arg is an internal subcommand like __supervise.
	if len(os.Args) > 1 && os.Args[1] != "__supervise" {
		for i, arg := range os.Args[1:] {
			if arg == "--" {
				rest := os.Args[i+2:]
				if err := cmd.RunLaunch(rest); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				return
			}
		}
	}

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
