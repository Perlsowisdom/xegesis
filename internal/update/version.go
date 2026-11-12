//go:build !store

// Package update is the direct download's own auto-update (ADR 0009: off until the user turns it on).
// It reads latest.json from the repository's public GitHub Releases, downloads a newer installer,
// checks its SHA-256 and our Ed25519 signature (internal/update/updatesig), and only then runs it.
// The Microsoft Store build is built with the tag "store" and has none of this (see store.go).
package update

import (
	"strconv"
	"strings"
)

// Available is true in the direct-download build.
const Available = true

// ver is a version split into numbers and an optional pre-release tag.
type ver struct {
	nums [3]int
	pre  string
	ok   bool
}

func parseVersion(s string) ver {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v ver
	core, pre, _ := strings.Cut(s, "-")
	v.pre = pre
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return ver{}
		}
		v.nums[i] = n
	}
	v.ok = true
	return v
}

// Newer reports whether version a is newer than b. A release is newer than a pre-release of the same
// number; a version that cannot be read (including "0.0.0-dev" builds) is older than any release.
func Newer(a, b string) bool {
	x, y := parseVersion(a), parseVersion(b)
	switch {
	case !x.ok:
		return false
	case !y.ok:
		return true
	}
	for i := range x.nums {
		if x.nums[i] != y.nums[i] {
			return x.nums[i] > y.nums[i]
		}
	}
	switch {
	case x.pre == y.pre:
		return false
	case x.pre == "":
		return true
	case y.pre == "":
		return false
	}
	return x.pre > y.pre
}
