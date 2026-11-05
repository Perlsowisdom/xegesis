//go:build windows

package assistant

import (
	"os/exec"
	"strconv"
	"syscall"
)

// prepare makes cancelling the command's context end the whole process tree: the tool's
// launcher script and everything it started.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		k := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		k.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = k.Run()
		return cmd.Process.Kill()
	}
}
