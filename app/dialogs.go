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

// SaveFile shows the system's save-file dialog for an export and returns the chosen path, or "".
// format is the file extension, such as pdf or docx; name is the suggested file name.
func (a *App) SaveFile(title, name, format string) (string, error) {
	label := map[string]string{"pdf": "PDF", "docx": "Word document", "html": "Web page", "md": "Markdown"}[format]
	if label == "" {
		label = format
	}
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: title, DefaultFilename: name,
		Filters: []runtime.FileFilter{{DisplayName: label + " (*." + format + ")", Pattern: "*." + format}}})
}
