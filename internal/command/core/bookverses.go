package core

import (
	"context"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// BookVerses is set by the reader. For a library book typed Bible it gives the verses a place in the
// book (a note or highlight locator, as JSON) falls in, so the note shows with those verses.
var BookVerses func(ctx context.Context, env *command.Env, itemID, locatorJSON string) (verse.Range, bool)

func linkBook(ctx context.Context, env *command.Env, itemID, locator string) *verse.Range {
	if BookVerses == nil {
		return nil
	}
	if r, ok := BookVerses(ctx, env, itemID, locator); ok {
		return &r
	}
	return nil
}
