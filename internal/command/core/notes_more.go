package core

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

const (
	maxTags   = 30
	maxTagLen = 40
)

// cleanTags makes tags comparable: trimmed, single-spaced, lower case, no duplicates, no commas.
func cleanTags(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(t, ",", " ")), " "))
		if t == "" || seen[t] {
			continue
		}
		if utf8.RuneCountInString(t) > maxTagLen {
			return nil, command.Errorf("bad_params", "a tag can have at most %d characters", maxTagLen)
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > maxTags {
		return nil, command.Errorf("bad_params", "a note can have at most %d tags", maxTags)
	}
	sort.Strings(out)
	return out, nil
}

// replaceTags makes tags the note's tag set, creating and deleting only what differs.
func replaceTags(ctx context.Context, tx *store.Tx, id string, tags []string) (bool, error) {
	return changeTags(ctx, tx, id, nil, tags)
}

func changeTags(_ context.Context, tx *store.Tx, id string, old, tags []string) (bool, error) {
	have, want := map[string]bool{}, map[string]bool{}
	for _, t := range old {
		have[t] = true
	}
	changed := false
	for _, t := range tags {
		want[t] = true
		if !have[t] {
			changed = true
			if err := tx.Create("note_tags", store.Row{"note_id": id, "tag": t}); err != nil {
				return false, err
			}
		}
	}
	for _, t := range old {
		if !want[t] {
			changed = true
			if err := tx.Delete("note_tags", store.Row{"note_id": id, "tag": t}); err != nil {
				return false, err
			}
		}
	}
	return changed, nil
}

// tagsOf reads one note's tags.
func tagsOf(ctx context.Context, env *command.Env, id string) ([]string, error) {
	rows, err := env.Store.Query(ctx, `SELECT tag FROM note_tags WHERE note_id = ? ORDER BY tag`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// loadTags fills in the tags of notes.
func loadTags(ctx context.Context, env *command.Env, notes []Note) error {
	if len(notes) == 0 {
		return nil
	}
	at := map[string]int{}
	args := make([]any, len(notes))
	marks := make([]string, len(notes))
	for i, n := range notes {
		at[n.ID], args[i], marks[i] = i, n.ID, "?"
	}
	rows, err := env.Store.Query(ctx, `SELECT note_id, tag FROM note_tags WHERE note_id IN (`+strings.Join(marks, ",")+`) ORDER BY tag`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, tag string
		if err := rows.Scan(&id, &tag); err != nil {
			return err
		}
		notes[at[id]].Tags = append(notes[at[id]].Tags, tag)
	}
	return rows.Err()
}

// Tags -----------------------------------------------------------------------------------------

type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count" desc:"Notes with this tag"`
}

type NoteTagsParams struct {
	Prefix string `json:"prefix,omitempty" desc:"Only tags starting with this, to suggest completions"`
}

func noteTags(ctx context.Context, env *command.Env, p NoteTagsParams) ([]TagCount, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	rows, err := env.Store.Query(ctx, `SELECT t.tag, COUNT(*) FROM note_tags t JOIN notes n ON n.id = t.note_id
		WHERE n.deleted_at IS NULL AND t.tag LIKE ? ESCAPE '\' GROUP BY t.tag ORDER BY COUNT(*) DESC, t.tag`,
		likeEscape(strings.ToLower(strings.TrimSpace(p.Prefix)))+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TagCount{}
	for rows.Next() {
		var c TagCount
		if err := rows.Scan(&c.Tag, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Search ---------------------------------------------------------------------------------------

type NoteSearchParams struct {
	Query  string `json:"query,omitempty" desc:"Words to find in the note's title, text or tags; every word must match"`
	Tag    string `json:"tag,omitempty" desc:"Only notes with this tag"`
	Book   string `json:"book,omitempty" desc:"Only notes on verses in this book of the Bible, e.g. \"Romans\""`
	ItemID string `json:"item_id,omitempty" desc:"Only notes on this library book"`
	On     string `json:"on,omitempty" enum:"verses,books,nothing" desc:"Only notes attached to verses, to library books, or to neither"`
	Limit  int    `json:"limit,omitempty" desc:"Most notes to return (default 100)"`
	Offset int    `json:"offset,omitempty" desc:"Notes to skip, for the next page"`
}

func noteSearch(ctx context.Context, env *command.Env, p NoteSearchParams) ([]Note, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	if p.Limit <= 0 {
		p.Limit = 100
	}
	q := noteSelect + ` WHERE n.deleted_at IS NULL`
	args := []any{}
	for _, w := range strings.Fields(strings.ToLower(p.Query)) {
		like := "%" + likeEscape(w) + "%"
		q += ` AND (LOWER(n.title) LIKE ? ESCAPE '\' OR LOWER(n.body_md) LIKE ? ESCAPE '\'
			OR EXISTS (SELECT 1 FROM note_tags t WHERE t.note_id = n.id AND t.tag LIKE ? ESCAPE '\'))`
		args = append(args, like, like, like)
	}
	if tag := strings.ToLower(strings.Join(strings.Fields(p.Tag), " ")); tag != "" {
		q += ` AND EXISTS (SELECT 1 FROM note_tags t WHERE t.note_id = n.id AND t.tag = ?)`
		args = append(args, tag)
	}
	if p.Book != "" {
		r, err := verse.Parse(p.Book)
		if err != nil {
			return nil, command.Errorf("bad_params", "%v", err)
		}
		q += ` AND a.verse_start >= ? AND a.verse_start < ?`
		args = append(args, int64(r.Start.Book())*1_000_000, int64(r.Start.Book()+1)*1_000_000)
	}
	if p.ItemID != "" {
		q += ` AND a.item_id = ?`
		args = append(args, p.ItemID)
	}
	switch p.On {
	case "verses":
		q += ` AND a.verse_start IS NOT NULL`
	case "books":
		q += ` AND a.item_id IS NOT NULL`
	case "nothing":
		q += ` AND n.anchor_id IS NULL`
	case "":
	default:
		return nil, command.Errorf("bad_params", "note.search: on must be verses, books or nothing")
	}
	q += ` ORDER BY n.updated_at DESC, n.id LIMIT ? OFFSET ?`
	return queryNotes(ctx, env, q, append(args, p.Limit, max(p.Offset, 0))...)
}

// Move -----------------------------------------------------------------------------------------

type NoteMoveParams struct {
	ID      string         `json:"id" required:"true"`
	Ref     string         `json:"ref,omitempty" desc:"Attach the note to these Bible verses"`
	ItemID  string         `json:"item_id,omitempty" desc:"Attach the note to this library book; give locator too"`
	Locator map[string]any `json:"locator,omitempty" desc:"Where in the book, as note.create's locator"`
	Free    bool           `json:"free,omitempty" desc:"Detach the note so it belongs to no passage"`
}

func noteMove(ctx context.Context, env *command.Env, p NoteMoveParams) (Note, error) {
	given := 0
	for _, b := range []bool{p.Ref != "", p.ItemID != "" || p.Locator != nil, p.Free} {
		if b {
			given++
		}
	}
	if given != 1 {
		return Note{}, command.Errorf("bad_params", "note.move: give one of ref, item_id with locator, or free")
	}
	var r *verse.Range
	var locator string
	switch {
	case p.Ref != "":
		parsed, err := verse.Parse(p.Ref)
		if err != nil {
			return Note{}, command.Errorf("bad_params", "%v", err)
		}
		r = &parsed
	case !p.Free:
		var err error
		if locator, err = bookAnchor(ctx, env, "note.move", p.ItemID, p.Locator); err != nil {
			return Note{}, err
		}
	}
	err := env.Store.Write(ctx, "note.move", func(tx *store.Tx) error {
		cur, err := liveNote(tx, p.ID)
		if err != nil {
			return err
		}
		var anchor any
		if !p.Free {
			aid, err := newAnchor(tx, r, p.ItemID, locator)
			if err != nil {
				return err
			}
			anchor = aid
		}
		return tx.Update("notes", store.ByID(p.ID), store.Row{"anchor_id": anchor, "updated_at": bumped(cur)})
	})
	if err != nil {
		return Note{}, err
	}
	return getNote(ctx, env, p.ID)
}

// Deleted notes --------------------------------------------------------------------------------

type NoteDeletedParams struct {
	Limit int `json:"limit,omitempty" desc:"Most notes to return (default 100)"`
}

func noteDeleted(ctx context.Context, env *command.Env, p NoteDeletedParams) ([]Note, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	if p.Limit <= 0 {
		p.Limit = 100
	}
	return queryNotes(ctx, env, noteSelect+` WHERE n.deleted_at IS NOT NULL ORDER BY n.deleted_at DESC, n.id LIMIT ?`, p.Limit)
}

func noteRestore(ctx context.Context, env *command.Env, p IDParams) (Note, error) {
	err := env.Store.Write(ctx, "note.restore", func(tx *store.Tx) error {
		cur, err := tx.Get("notes", store.ByID(p.ID))
		if err != nil {
			return err
		}
		if cur["deleted_at"] == nil {
			return command.Errorf("bad_params", "note %s is not deleted", p.ID)
		}
		return tx.Update("notes", store.ByID(p.ID), store.Row{"deleted_at": nil})
	})
	if err != nil {
		return Note{}, err
	}
	return getNote(ctx, env, p.ID)
}
