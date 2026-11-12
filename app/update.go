//go:build !store

package main

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Perlsowisdom/xegesis/internal/update"
)

// startUpdates looks for a newer version now and then, if the user turned that on (update.auto does
// nothing otherwise). The Store build has no updater; see update_store.go.
func (a *App) startUpdates(ctx, bg context.Context) {
	a.every(bg, time.Hour, "update.auto", func(res any) {
		if r, ok := res.(update.CheckResult); ok && r.Newer {
			runtime.EventsEmit(ctx, "update:available", r)
		}
	})
}

// afterRun quits the app once an update's installer is running: it waits for this app to close,
// installs over it and starts it again.
func (a *App) afterRun(res any) {
	if r, ok := res.(update.InstallResult); ok && r.Launched {
		go runtime.Quit(a.ctx)
	}
}
