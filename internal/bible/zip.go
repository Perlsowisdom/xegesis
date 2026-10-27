package bible

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxUnzipped caps how much a Bible archive may expand to, so a hostile zip cannot fill the disk.
const maxUnzipped = 2 << 30

// unzipTemp extracts a zip archive into a new temporary folder and returns it. The caller removes it.
// Entries that would land outside the folder are refused.
func unzipTemp(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	dir, err := os.MkdirTemp("", "xegesis-bible-*")
	if err != nil {
		return "", err
	}
	var total int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(target, dir+string(os.PathSeparator)) {
			os.RemoveAll(dir)
			return "", fmt.Errorf("archive entry %q points outside the archive", f.Name)
		}
		n, err := extract(f, target, maxUnzipped-total)
		total += n
		if err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}

// contentDir returns the folder inside dir that holds the files, going down through folders that
// contain nothing but one subfolder, as many archives wrap their files that way.
func contentDir(dir string) string {
	for {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 || !entries[0].IsDir() {
			return dir
		}
		dir = filepath.Join(dir, entries[0].Name())
	}
}

func extract(f *zip.File, target string, limit int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	in, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	n, err := io.Copy(out, io.LimitReader(in, limit+1))
	if err == nil && n > limit {
		err = fmt.Errorf("archive expands to more than %d bytes", int64(maxUnzipped))
	}
	return n, err
}
