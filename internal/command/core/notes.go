package core

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

type Note struct {
	ID        string         `json:"id"`
	Title     string         `json:"title,omitempty"`
	Body      string         `json:"body" desc:"Markdown text"`
	Ref       *RefResult     `json:"ref,omitempty" desc:"The verses the note is attached to"`
	ItemID    string         `json:"item_id,omitempty" desc:"The book the note is attached to"`
	ItemTitle string         `json:"item_title,omitempty" desc:"That book's title"`
	Locator   map[string]any `json:"locator,omitempty" desc:"Where in the book, as highlight.create's locator"`
	Tags      []string       `json:"tags" desc:"The note's tags, in lower case"`
	CreatedAt int64          `json:"created_at"`
	UpdatedAt int64          `json:"updated_at"`
	DeletedAt int64          `json:"deleted_at,omitempty" desc:"When the note was deleted; only in note.deleted"`
}

type NoteCreateParams struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body" desc:"Markdown text"`
	Ref   string `json:"ref,omitempty" desc:"Bible reference to attach the note to, e.g. \"John 3:16\""`
	// A book anchor: the same locator highlight.create takes.
	ItemID  string         `json:"item_id,omitempty" desc:"Library book to attach the note to; give locator too"`
	Locator map[string]any `json:"locator,omitempty" desc:"Where in the book: {section, para, offset, end_para, end_offset, text} or {section, page, rects, text}"`
	Tags    []string       `json:"tags,omitempty" desc:"Free-text tags; note.tags lists the ones already in use"`
}

// bookAnchor checks a note's book anchor and returns its locator as JSON.
func bookAnchor(ctx context.Context, env *command.Env, cmd, itemID string, loc map[string]any) (string, error) {
	if itemID == "" || loc == nil {
		return "", command.Errorf("bad_params", "%s: give item_id and locator together", cmd)
	}
	if n, ok := loc["section"].(float64); !ok || n < 1 {
		return "", command.Errorf("bad_params", "%s: the locator needs a section of 1 or more", cmd)
	}
	raw, err := json.Marshal(loc)
	if err != nil || len(raw) > 64<<10 {
		return "", command.Errorf("bad_params", "%s: the locator is not valid", cmd)
	}
	var n int
	if err := env.Store.QueryRow(ctx, `SELECT COUNT(*) FROM items WHERE id = ? AND deleted_at IS NULL`, itemID).Scan(&n); err != nil {
		return "", err
	}
	if n == 0 {
		return "", command.Errorf("not_found", "no item %s", itemID)
	}
	return string(raw), nil
}

// newAnchor creates an anchor for verses or a book place and returns its ID.
func newAnchor(tx *store.Tx, r *verse.Range, itemID, locator string) (string, error) {
	aid := store.NewID()
	row := store.Row{"id": aid}
	if r != nil {
		row["verse_start"], row["verse_end"], row["versification"] = int64(r.Start), int64(r.End), string(verse.English)
	} else {
		row["item_id"], row["locator_json"] = itemID, locator
	}
	return aid, tx.Create("anchors", row)
}

func noteCreate(ctx context.Context, env *command.Env, p NoteCreateParams) (Note, error) {
	var r *verse.Range
	if p.Ref != "" {
		parsed, err := verse.Parse(p.Ref)
		if err != nil {
			return Note{}, command.Errorf("bad_params", "%v", err)
		}
		r = &parsed
	}
	var locator string
	if p.ItemID != "" || p.Locator != nil {
		if p.Ref != "" {
			return Note{}, command.Errorf("bad_params", "note.create: attach a note to a book or to verses, not both")
		}
		var err error
		if locator, err = bookAnchor(ctx, env, "note.create", p.ItemID, p.Locator); err != nil {
			return Note{}, err
		}
	}
	tags, err := cleanTags(p.Tags)
	if err != nil {
		return Note{}, err
	}
	id, t := store.NewID(), now()
	err = env.Store.Write(ctx, "note.create", func(tx *store.Tx) error {
		var anchor any
		if r != nil || locator != "" {
			aid, err := newAnchor(tx, r, p.ItemID, locator)
			if err != nil {
				return err
			}
			anchor = aid
		}
		if err := tx.Create("notes", store.Row{"id": id, "anchor_id": anchor, "title": nullable(p.Title),
			"body_md": p.Body, "created_at": t, "updated_at": t}); err != nil {
			return err
		}
		_, err := replaceTags(ctx, tx, id, tags)
		return err
	})
	if err != nil {
		return Note{}, err
	}
	return getNote(ctx, env, id)
}

type IDParams struct {
	ID string `json:"id" required:"true"`
}

func noteGet(ctx context.Context, env *command.Env, p IDParams) (Note, error) {
	if err := needStore(env); err != nil {
		return Note{}, err
	}
	return getNote(ctx, env, p.ID)
}

type NoteListParams struct {
	Ref     string `json:"ref,omitempty" desc:"Only notes on verses overlapping this reference"`
	ItemID  string `json:"item_id,omitempty" desc:"Only notes on this book"`
	Section int    `json:"section,omitempty" desc:"With item_id, only notes on this section or PDF page"`
	Limit   int    `json:"limit,omitempty" desc:"Most notes to return (default 100)"`
}

func noteList(ctx context.Context, env *command.Env, p NoteListParams) ([]Note, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	if p.Limit <= 0 {
		p.Limit = 100
	}
	q := noteSelect + ` WHERE n.deleted_at IS NULL`
	args := []any{}
	if p.Ref != "" {
		r, err := verse.Parse(p.Ref)
		if err != nil {
			return nil, command.Errorf("bad_params", "%v", err)
		}
		q += ` AND a.verse_start <= ? AND a.verse_end >= ?`
		args = append(args, int64(r.End), int64(r.Start))
	}
	if p.ItemID != "" {
		q += ` AND a.item_id = ?`
		args = append(args, p.ItemID)
		if p.Section > 0 {
			q += ` AND json_extract(a.locator_json, '$.section') = ?`
			args = append(args, p.Section)
		}
	}
	q += ` ORDER BY n.updated_at DESC, n.id LIMIT ?`
	return queryNotes(ctx, env, q, append(args, p.Limit)...)
}

type NoteUpdateParams struct {
	ID    string    `json:"id" required:"true"`
	Title *string   `json:"title,omitempty" desc:"New title; omit to keep"`
	Body  *string   `json:"body,omitempty" desc:"New Markdown text; omit to keep"`
	Tags  *[]string `json:"tags,omitempty" desc:"The note's complete set of tags; omit to keep"`
}

// bumped is an updated_at that moves forward even for two edits within one millisecond.
func bumped(cur store.Row) int64 {
	t := now()
	if prev, ok := cur["updated_at"].(int64); ok && t <= prev {
		t = prev + 1
	}
	return t
}

func noteUpdate(ctx context.Context, env *command.Env, p NoteUpdateParams) (Note, error) {
	fields := store.Row{}
	if p.Title != nil {
		fields["title"] = nullable(*p.Title)
	}
	if p.Body != nil {
		fields["body_md"] = *p.Body
	}
	var tags []string
	if p.Tags != nil {
		var err error
		if tags, err = cleanTags(*p.Tags); err != nil {
			return Note{}, err
		}
	}
	var old []string
	if p.Tags != nil {
		var err error
		if old, err = tagsOf(ctx, env, p.ID); err != nil {
			return Note{}, err
		}
	}
	if len(fields) == 0 && p.Tags == nil {
		return getNote(ctx, env, p.ID)
	}
	err := env.Store.Write(ctx, "note.update", func(tx *store.Tx) error {
		cur, err := liveNote(tx, p.ID)
		if err != nil {
			return err
		}
		if p.Tags != nil {
			changed, err := changeTags(ctx, tx, p.ID, old, tags)
			if err != nil {
				return err
			}
			if !changed && len(fields) == 0 {
				return nil
			}
		}
		fields["updated_at"] = bumped(cur)
		return tx.Update("notes", store.ByID(p.ID), fields)
	})
	if err != nil {
		return Note{}, err
	}
	return getNote(ctx, env, p.ID)
}

// liveNote reads a note that has not been deleted.
func liveNote(tx *store.Tx, id string) (store.Row, error) {
	cur, err := tx.Get("notes", store.ByID(id))
	if err != nil {
		return nil, err
	}
	if cur["deleted_at"] != nil {
		return nil, command.Errorf("not_found", "note %s is deleted", id)
	}
	return cur, nil
}

func noteDelete(ctx context.Context, env *command.Env, p IDParams) (struct{}, error) {
	return struct{}{}, env.Store.Write(ctx, "note.delete", func(tx *store.Tx) error {
		if _, err := liveNote(tx, p.ID); err != nil {
			return err
		}
		return tx.Delete("notes", store.ByID(p.ID))
	})
}

const noteSelect = `SELECT n.id, n.title, n.body_md, n.created_at, n.updated_at, a.verse_start, a.verse_end,
	a.item_id, a.locator_json, COALESCE(n.deleted_at, 0), it.title FROM notes n LEFT JOIN anchors a ON a.id = n.anchor_id
	LEFT JOIN items it ON it.id = a.item_id`

// queryNotes runs a noteSelect query and fills in each note's tags.
func queryNotes(ctx context.Context, env *command.Env, q string, args ...any) ([]Note, error) {
	rows, err := env.Store.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := []Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, n)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, loadTags(ctx, env, out)
}

func getNote(ctx context.Context, env *command.Env, id string) (Note, error) {
	list, err := queryNotes(ctx, env, noteSelect+` WHERE n.id = ? AND n.deleted_at IS NULL`, id)
	if err != nil {
		return Note{}, err
	}
	if len(list) == 0 {
		return Note{}, command.Errorf("not_found", "no note %s", id)
	}
	return list[0], nil
}

func scanNote(rows *sql.Rows) (Note, error) {
	var n Note
	var title sql.NullString
	var start, end sql.NullInt64
	var item, locator, itemTitle sql.NullString
	if err := rows.Scan(&n.ID, &title, &n.Body, &n.CreatedAt, &n.UpdatedAt, &start, &end, &item, &locator, &n.DeletedAt, &itemTitle); err != nil {
		return n, err
	}
	n.ItemID, n.ItemTitle = item.String, itemTitle.String
	if locator.Valid {
		json.Unmarshal([]byte(locator.String), &n.Locator)
	}
	n.Title = title.String
	if start.Valid && end.Valid {
		r := refResult(verse.Range{Start: verse.ID(start.Int64), End: verse.ID(end.Int64)})
		n.Ref = &r
	}
	n.Tags = []string{}
	return n, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
