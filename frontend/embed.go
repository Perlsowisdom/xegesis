// Package frontend holds the desktop interface's built web files.
package frontend

import "embed"

//go:embed all:dist
var Dist embed.FS
