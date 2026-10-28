// Command app is the Xegesis desktop app. Its interface calls the same command registry as the CLI;
// it has no features of its own.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

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
	ctx  context.Context
	reg  *command.Registry
	env  *command.Env
	stop context.CancelFunc
	done chan struct{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	bg, stop := context.WithCancel(context.Background())
	a.stop, a.done = stop, make(chan struct{})
	go a.autoBackup(bg)
}

func (a *App) shutdown(context.Context) {
	a.stop()
	<-a.done
}

// autoBackup runs the scheduled backup at start-up and then hourly; the command decides whether one is due.
func (a *App) autoBackup(ctx context.Context) {
	defer close(a.done)
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if _, err := a.reg.Run(ctx, a.env, "backup.auto", nil); err != nil && ctx.Err() == nil {
			log.Printf("automatic backup: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

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

// libraryPath is library.db in the user's config folder, or XEGESIS_LIBRARY when set, as for the CLI.
func libraryPath() string {
	if p := os.Getenv("XEGESIS_LIBRARY"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	return filepath.Join(dir, "Xegesis", "library.db")
}

func main() {
	lib := libraryPath()
	s, err := store.Open(lib)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	reg := all.Registry(filepath.Dir(lib), secrets.OS())
	app := &App{reg: reg, env: &command.Env{Store: s}}

	err = wails.Run(&options.App{
		Title:       "Xegesis",
		Width:       1200,
		Height:      800,
		MinWidth:    420,
		MinHeight:   360,
		AssetServer: &assetserver.Options{Assets: frontend.Dist},
		OnStartup:   app.startup,
		OnShutdown:  app.shutdown,
		Bind:        []any{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
