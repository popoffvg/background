//go:build linux || darwin

package runner

import (
	"os/exec"
	"syscall"
)

// SetSetsid sets the Setsid flag on the command's SysProcAttr.
func SetSetsid(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}

// SetPgid sets the Pgid flag so kill can target the process group.
func SetPgid(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
	cmd.SysProcAttr.Pgid = 0
}
