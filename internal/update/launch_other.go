//go:build !store && !windows

package update

import "errors"

// launchInstaller: automatic updates are Windows-only for now.
func launchInstaller(string) error {
	return errors.New("automatic updates are only available on Windows for now")
}
