package core

import (
	"context"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// linkBook gives the verses a place in a library book typed Bible falls in (a note or highlight
// locator, as JSON), so the note shows with those verses. The lookup comes from env.Books.
func linkBook(ctx context.Context, env *command.Env, itemID, locator string) *verse.Range {
	if env.Books == nil {
		return nil
	}
	if r, ok := env.Books.BookVerses(ctx, env, itemID, locator); ok {
		return &r
	}
	return nil
}
