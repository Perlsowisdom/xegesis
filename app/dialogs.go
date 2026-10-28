package main

import "github.com/wailsapp/wails/v2/pkg/runtime"

// PickFile shows the system's open-file dialog and returns the chosen path, or "" if cancelled.
// It only returns a path; what is done with it is up to a command.
func (a *App) PickFile(title string) (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: title})
}

// PickFiles shows the open-file dialog for several book files and returns the chosen paths.
func (a *App) PickFiles(title string) ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: title, Filters: []runtime.FileFilter{
		{DisplayName: "Books (EPUB, PDF, Word, HTML, Markdown, text)", Pattern: "*.epub;*.pdf;*.docx;*.html;*.htm;*.xhtml;*.md;*.markdown;*.txt"},
		{DisplayName: "All files", Pattern: "*.*"},
	}})
}

// PickFolder shows the system's choose-folder dialog and returns the chosen path, or "".
func (a *App) PickFolder(title string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title, CanCreateDirectories: false})
}
