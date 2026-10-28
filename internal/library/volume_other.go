//go:build !windows

package library

import (
	"os"
	"path/filepath"
	"strings"
)

// mountParents are the directories removable volumes are mounted under, with how many levels
// below each the mount point sits (/media/<user>/<volume> is two).
var mountParents = []struct {
	dir   string
	depth int
}{{"/Volumes", 1}, {"/run/media", 2}, {"/media", 2}, {"/media", 1}, {"/mnt", 1}}

// volumeOf returns the mount point holding path, judged from its prefix so that it also works
// for a volume that is no longer mounted.
var volumeOf = func(path string) string {
	for _, m := range mountParents {
		rest, ok := strings.CutPrefix(path, m.dir+"/")
		if !ok {
			continue
		}
		parts := strings.Split(rest, "/")
		if len(parts) > m.depth {
			return filepath.Join(append([]string{m.dir}, parts[:m.depth]...)...)
		}
	}
	return "/"
}

// mountedRoots lists the mount points of the volumes mounted now. Tests replace it.
var mountedRoots = func() []string {
	out := []string{"/"}
	for _, m := range mountParents {
		pattern := m.dir + strings.Repeat("/*", m.depth)
		matches, _ := filepath.Glob(pattern)
		out = append(out, matches...)
	}
	return out
}

// documentsDir is ~/Documents.
func documentsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Documents")
}
