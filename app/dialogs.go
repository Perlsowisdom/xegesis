package main

import "github.com/wailsapp/wails/v2/pkg/runtime"

// PickFile shows the system's open-file dialog and returns the chosen path, or "" if cancelled.
// It only returns a path; what is done with it is up to a command.
func (a *App) PickFile(title string) (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: title})
}

// PickFolder shows the system's choose-folder dialog and returns the chosen path, or "".
func (a *App) PickFolder(title string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title, CanCreateDirectories: false})
}
