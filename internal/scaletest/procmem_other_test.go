//go:build !windows

package scaletest

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// processMemory returns the process's resident set and its peak so far, in bytes (Linux; zero
// elsewhere).
func processMemory() (cur, peak uint64) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		n, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
		switch k {
		case "VmRSS":
			cur = n * 1024
		case "VmHWM":
			peak = n * 1024
		}
	}
	return cur, peak
}
