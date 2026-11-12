//go:build store

package main

import "context"

// The Microsoft Store build has no updater: the Store installs updates.
func (a *App) startUpdates(context.Context, context.Context) {}

func (a *App) afterRun(any) {}
