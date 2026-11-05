//go:build !windows

package assistant

import (
	"os/exec"
	"syscall"
)

// prepare puts the command in its own process group and makes cancelling its context kill the
// whole group.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
