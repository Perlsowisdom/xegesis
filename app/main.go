// Command app is the Xegesis desktop app. Its interface calls the same command registry as the CLI;
// it has no features of its own.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/Perlsowisdom/xegesis/frontend"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/all"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// App is bound to the interface. The interface reaches every feature through Commands and Run.
type App struct {
	ctx context.Context
	reg *command.Registry
	env *command.Env
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// Commands describes every command.
func (a *App) Commands() []command.Description { return a.reg.List() }

// Run runs a command with JSON parameters and returns its JSON result.
func (a *App) Run(id, params string) (string, error) {
	res, err := a.reg.Run(a.ctx, a.env, id, json.RawMessage(params))
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(res)
	return string(b), err
}

func main() {
	dir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	s, err := store.Open(filepath.Join(dir, "Xegesis", "library.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	reg := all.Registry(filepath.Join(dir, "Xegesis"), secrets.OS())
	app := &App{reg: reg, env: &command.Env{Store: s}}

	err = wails.Run(&options.App{
		Title:       "Xegesis",
		Width:       1200,
		Height:      800,
		AssetServer: &assetserver.Options{Assets: frontend.Dist},
		OnStartup:   app.startup,
		Bind:        []any{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
