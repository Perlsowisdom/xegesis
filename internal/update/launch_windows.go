//go:build !store && windows

package update

import (
	"os/exec"
	"syscall"
)

// launchInstaller starts the verified setup silently (/S) in update mode, detached so it outlives
// the app, which quits right after. The setup waits for the app to let go of its files, installs
// over it and starts the app again.
func launchInstaller(installer string) error {
	cmd := exec.Command(installer, "/S", "/UPDATE")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200} // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
