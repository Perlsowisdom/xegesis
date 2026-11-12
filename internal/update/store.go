//go:build store

// Package update is the direct download's own auto-update. The Microsoft Store build is built with
// the tag "store" and has no updater at all: this file is all it contains, so no update code, command
// or setting exists in it (the Store installs updates itself).
package update

import "github.com/Perlsowisdom/xegesis/internal/command"

// Available is false in the Store build.
const Available = false

// Register adds nothing in the Store build.
func Register(*command.Registry) {}
