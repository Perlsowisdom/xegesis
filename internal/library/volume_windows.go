package library

import (
	"os"
	"path/filepath"
)

// volumeOf returns the root of the volume holding path, such as a drive letter or a network share.
var volumeOf = func(path string) string { return filepath.VolumeName(path) + `\` }

// mountedRoots lists the roots of the drives mounted now. Tests replace it.
var mountedRoots = func() []string {
	var out []string
	for c := 'A'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		if _, err := os.Stat(root); err == nil {
			out = append(out, root)
		}
	}
	return out
}
