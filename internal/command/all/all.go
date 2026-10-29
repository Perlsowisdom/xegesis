// Package all builds the registry with every Xegesis command, so the CLI, the MCP server and the
// desktop app offer exactly the same features.
package all

import (
	"path/filepath"

	"github.com/Perlsowisdom/xegesis/internal/backup"
	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/bible/repos"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/integrations/apibible"
	"github.com/Perlsowisdom/xegesis/internal/integrations/dbl"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/lists"
	"github.com/Perlsowisdom/xegesis/internal/plan"
	"github.com/Perlsowisdom/xegesis/internal/reader"
	"github.com/Perlsowisdom/xegesis/internal/search"
)

// Registry returns every command. dataDir is the folder holding library.db; Bible modules live in
// its modules subfolder, Bible catalogues in catalogues and search indexes in index.
func Registry(dataDir string, s secrets.Store) *command.Registry {
	r := command.NewRegistry()
	core.Register(r)
	library.Register(r)
	bibles := bible.NewLibrary(filepath.Join(dataDir, "modules"))
	bible.RegisterLibrary(r, bibles)
	repos.Register(r, filepath.Join(dataDir, "catalogues"), bibles)
	apibible.Register(r, s)
	dbl.Register(r, s)
	search.Register(r, dataDir)
	backup.Register(r, dataDir)
	reader.Register(r)
	lists.Register(r)
	plan.Register(r)
	return r
}
