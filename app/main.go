// Command app is the Xegesis desktop app. Its interface calls the same command registry as the CLI;
// it has no features of its own.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Perlsowisdom/xegesis/frontend"
	"github.com/Perlsowisdom/xegesis/internal/assistant"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/all"
	"github.com/Perlsowisdom/xegesis/internal/integrations/secrets"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// App is bound to the interface. The interface reaches every feature through Commands and Run.
type App struct {
	ctx   context.Context
	bg    context.Context // ends at shutdown
	reg   *command.Registry
	env   *command.Env
	stop  context.CancelFunc
	wg    sync.WaitGroup
	typed sync.Once // fills in missing book types once per run
	shot  string    // --screenshot: save the window's picture here, then quit
}

// rescanEvery is how often library folders are rescanned. Unchanged files are skipped, so it is cheap.
const rescanEvery = 10 * time.Minute

func (a *App) startup(ctx context.Context) {
	// The assistant streams its replies to the interface as events.
	a.ctx = assistant.WithEmitter(ctx, func(name string, data any) { runtime.EventsEmit(ctx, name, data) })
	bg, stop := context.WithCancel(context.Background())
	a.bg, a.stop = bg, stop
	a.every(bg, time.Hour, "backup.auto", nil)
	a.every(bg, rescanEvery, "library.rescan", func(res any) {
		if r, ok := res.(library.RescanResult); ok && r.Added+r.Changed+r.Removed > 0 {
			runtime.EventsEmit(ctx, "library:changed")
		}
		a.index()
		// Libraries from before book types existed get them once, without the user asking.
		a.typed.Do(func() {
			if _, err := a.reg.Run(bg, a.env, "facet.suggest", nil); err != nil && bg.Err() == nil {
				log.Printf("facet.suggest: %v", err)
			}
		})
	})
	// Files and folders dropped on the window are added to the library by the interface.
	runtime.OnFileDrop(ctx, func(_, _ int, paths []string) { runtime.EventsEmit(ctx, "files:dropped", paths) })
	if a.shot != "" {
		takeScreenshot(ctx, a.shot)
	}
}

func (a *App) shutdown(context.Context) {
	a.stop()
	a.wg.Wait()
	if _, err := a.reg.Run(context.Background(), a.env, "search.cancel", nil); err != nil {
		log.Printf("search.cancel: %v", err)
	}
}

// index starts indexing new and changed books in the background; books become searchable one by
// one. It is cheap when nothing has changed, and it rebuilds index files of an older format.
func (a *App) index() {
	if a.bg == nil || a.bg.Err() != nil {
		return
	}
	if _, err := a.reg.Run(a.bg, a.env, "search.index", nil); err != nil {
		log.Printf("search.index: %v", err)
	}
}

// addsBooks reports whether a command can bring new books into the library.
func addsBooks(id string) bool {
	return strings.HasPrefix(id, "library.add") || id == "library.scan" || id == "library.relocate_folder"
}

// every runs a command at start-up and then at each interval until ctx ends: the scheduled backup
// (the command decides whether one is due) and the library rescan.
func (a *App) every(ctx context.Context, interval time.Duration, id string, done func(any)) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		tick := time.NewTicker(interval)
		defer tick.Stop()
		for {
			res, err := a.reg.Run(ctx, a.env, id, nil)
			if err != nil && ctx.Err() == nil {
				log.Printf("%s: %v", id, err)
			}
			if err == nil && done != nil {
				done(res)
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}

// Commands describes every command.
func (a *App) Commands() []command.Description { return a.reg.List() }

// Run runs a command with JSON parameters and returns its JSON result.
func (a *App) Run(id, params string) (string, error) {
	res, err := a.reg.Run(a.ctx, a.env, id, json.RawMessage(params))
	if err != nil {
		return "", err
	}
	if addsBooks(id) {
		a.index()
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

// memoryLimit is the Go heap's soft limit (ADR 0008) unless GOMEMLIMIT is set. The 10,000-book
// test peaks far below it (docs/performance.md); it makes the collector work harder before the
// app grows past its memory budget.
const memoryLimit = 300 << 20

func setMemoryLimit() int64 {
	if os.Getenv("GOMEMLIMIT") != "" {
		return debug.SetMemoryLimit(-1)
	}
	debug.SetMemoryLimit(memoryLimit)
	return memoryLimit
}

func main() {
	setMemoryLimit()
	lib := libraryPath()
	s, err := store.Open(lib)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	reg := all.Registry(filepath.Dir(lib), secrets.OS())
	app := &App{reg: reg, env: &command.Env{Store: s}, shot: screenshotArg(os.Args[1:])}

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
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true},
	})
	if err != nil {
		log.Fatal(err)
	}
}
