package core

import (
	"context"
	"database/sql"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

type Note struct {
	ID        string     `json:"id"`
	Title     string     `json:"title,omitempty"`
	Body      string     `json:"body" desc:"Markdown text"`
	Ref       *RefResult `json:"ref,omitempty" desc:"The verses the note is attached to"`
	CreatedAt int64      `json:"created_at"`
	UpdatedAt int64      `json:"updated_at"`
}

type NoteCreateParams struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body" desc:"Markdown text"`
	Ref   string `json:"ref,omitempty" desc:"Bible reference to attach the note to, e.g. \"John 3:16\""`
}

func noteCreate(ctx context.Context, env *command.Env, p NoteCreateParams) (Note, error) {
	var anchor any
	var r verse.Range
	if p.Ref != "" {
		var err error
		if r, err = verse.Parse(p.Ref); err != nil {
			return Note{}, command.Errorf("bad_params", "%v", err)
		}
	}
	id, t := store.NewID(), now()
	err := env.Store.Write(ctx, "note.create", func(tx *store.Tx) error {
		if p.Ref != "" {
			aid := store.NewID()
			if err := tx.Create("anchors", store.Row{"id": aid, "verse_start": int64(r.Start),
				"verse_end": int64(r.End), "versification": string(verse.English)}); err != nil {
				return err
			}
			anchor = aid
		}
		return tx.Create("notes", store.Row{"id": id, "anchor_id": anchor, "title": nullable(p.Title),
			"body_md": p.Body, "created_at": t, "updated_at": t})
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
	Ref   string `json:"ref,omitempty" desc:"Only notes on verses overlapping this reference"`
	Limit int    `json:"limit,omitempty" desc:"Most notes to return (default 100)"`
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
	q += ` ORDER BY n.updated_at DESC LIMIT ?`
	rows, err := env.Store.Query(ctx, q, append(args, p.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

type NoteUpdateParams struct {
	ID    string  `json:"id" required:"true"`
	Title *string `json:"title,omitempty" desc:"New title; omit to keep"`
	Body  *string `json:"body,omitempty" desc:"New Markdown text; omit to keep"`
}

func noteUpdate(ctx context.Context, env *command.Env, p NoteUpdateParams) (Note, error) {
	fields := store.Row{}
	if p.Title != nil {
		fields["title"] = nullable(*p.Title)
	}
	if p.Body != nil {
		fields["body_md"] = *p.Body
	}
	if len(fields) == 0 {
		return getNote(ctx, env, p.ID)
	}
	err := env.Store.Write(ctx, "note.update", func(tx *store.Tx) error {
		cur, err := tx.Get("notes", store.ByID(p.ID))
		if err != nil {
			return err
		}
		if cur["deleted_at"] != nil {
			return command.Errorf("not_found", "note %s is deleted", p.ID)
		}
		fields["updated_at"] = now()
		return tx.Update("notes", store.ByID(p.ID), fields)
	})
	if err != nil {
		return Note{}, err
	}
	return getNote(ctx, env, p.ID)
}

func noteDelete(ctx context.Context, env *command.Env, p IDParams) (struct{}, error) {
	return struct{}{}, env.Store.Write(ctx, "note.delete", func(tx *store.Tx) error {
		return tx.Delete("notes", store.ByID(p.ID))
	})
}

const noteSelect = `SELECT n.id, n.title, n.body_md, n.created_at, n.updated_at, a.verse_start, a.verse_end
	FROM notes n LEFT JOIN anchors a ON a.id = n.anchor_id`

func getNote(ctx context.Context, env *command.Env, id string) (Note, error) {
	rows, err := env.Store.Query(ctx, noteSelect+` WHERE n.id = ? AND n.deleted_at IS NULL`, id)
	if err != nil {
		return Note{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return Note{}, command.Errorf("not_found", "no note %s", id)
	}
	return scanNote(rows)
}

func scanNote(rows *sql.Rows) (Note, error) {
	var n Note
	var title sql.NullString
	var start, end sql.NullInt64
	if err := rows.Scan(&n.ID, &title, &n.Body, &n.CreatedAt, &n.UpdatedAt, &start, &end); err != nil {
		return n, err
	}
	n.Title = title.String
	if start.Valid && end.Valid {
		r := refResult(verse.Range{Start: verse.ID(start.Int64), End: verse.ID(end.Int64)})
		n.Ref = &r
	}
	return n, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
