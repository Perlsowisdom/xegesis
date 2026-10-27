// Package all builds the registry with every Xegesis command, so the CLI, the MCP server and the
// desktop app offer exactly the same features.
package all

import (
	"path/filepath"

	"github.com/Perlsowisdom/xegesis/internal/backup"
	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/integrations/apibible"
	"github.com/Perlsowisdom/xegesis/internal/integrations/dbl"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/reader"
	"github.com/Perlsowisdom/xegesis/internal/search"
)

// Registry returns every command. dataDir is the folder holding library.db; Bible modules live in
// its modules subfolder and search indexes in its index subfolder.
func Registry(dataDir string, s secrets.Store) *command.Registry {
	r := command.NewRegistry()
	core.Register(r)
	library.Register(r)
	bible.Register(r, filepath.Join(dataDir, "modules"))
	apibible.Register(r, s)
	dbl.Register(r, s)
	search.Register(r, dataDir)
	backup.Register(r, dataDir)
	reader.Register(r)
	return r
}
