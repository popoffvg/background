package tui

import (
	"os/exec"
	"runtime"
	"strings"
)

// clipboardCommand returns the clipboard tool argv for the given OS, choosing
// the first available tool. Returns nil if none found.
func clipboardCommand(goos string, lookPath func(string) (string, error)) []string {
	switch goos {
	case "darwin":
		if _, err := lookPath("pbcopy"); err == nil {
			return []string{"pbcopy"}
		}
	default: // linux and others
		candidates := [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
		}
		for _, c := range candidates {
			if _, err := lookPath(c[0]); err == nil {
				return c
			}
		}
	}
	return nil
}

// copyToClipboard pipes text to the platform clipboard tool. Returns the tool
// name used, or an error if no tool is available / the command failed.
func copyToClipboard(text string) (string, error) {
	argv := clipboardCommand(runtime.GOOS, exec.LookPath)
	if argv == nil {
		return "", errNoClipboard
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return argv[0], nil
}

var errNoClipboard = clipboardError("no clipboard tool found")

type clipboardError string

func (e clipboardError) Error() string { return string(e) }
