package document

import (
	"context"
	"database/sql"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

type TurnParams struct {
	ID   string `json:"id" required:"true" desc:"The note"`
	Kind string `json:"kind" required:"true" enum:"sermon,lesson,study" desc:"What to turn it into"`
}

// turnInto makes a document from a note and deletes the note, in one logged change set. The note's
// verses become the document's passage, its tags and folder come along, and the note stays in the
// bin (note.restore brings it back; its history is untouched).
func (s *service) turnInto(ctx context.Context, env *command.Env, p TurnParams) (Document, error) {
	kind, err := cleanKind(p.Kind)
	if err != nil {
		return Document{}, err
	}
	var title, body string
	var created int64
	var folder sql.NullString
	var vs, ve sql.NullInt64
	err = env.Store.QueryRow(ctx, `SELECT COALESCE(n.title, ''), n.body_md, n.created_at, n.folder_id, a.verse_start, a.verse_end
		FROM notes n LEFT JOIN anchors a ON a.id = n.anchor_id WHERE n.id = ? AND n.deleted_at IS NULL`, p.ID).
		Scan(&title, &body, &created, &folder, &vs, &ve)
	if err == sql.ErrNoRows {
		return Document{}, command.Errorf("not_found", "no note %s", p.ID)
	}
	if err != nil {
		return Document{}, err
	}
	tags, err := noteTags(ctx, env, p.ID)
	if err != nil {
		return Document{}, err
	}
	var passages []string
	if vs.Valid && ve.Valid {
		passages = []string{verse.Range{Start: verse.ID(vs.Int64), End: verse.ID(ve.Int64)}.String()}
	}
	if title, err = cleanTitle(title); err != nil {
		return Document{}, err
	}
	if title == "" {
		title = untitled(kind)
	}
	ol, err := cleanOutline(passages, nil, outline{Tags: tags, FromNote: p.ID})
	if err != nil {
		return Document{}, err
	}
	id, t := store.NewID(), now()
	err = env.Store.Write(ctx, "note.turn_into", func(tx *store.Tx) error {
		cur, err := tx.Get("notes", store.ByID(p.ID))
		if err != nil {
			return err
		}
		if cur["deleted_at"] != nil {
			return command.Errorf("not_found", "note %s is deleted", p.ID)
		}
		var f any
		if folder.Valid {
			f = folder.String
		}
		if err := tx.Create("documents", store.Row{"id": id, "kind": kind, "title": title, "body_md": body,
			"outline_json": ol, "created_at": created, "updated_at": t, "folder_id": f}); err != nil {
			return err
		}
		return tx.Delete("notes", store.ByID(p.ID))
	})
	if err != nil {
		return Document{}, err
	}
	return s.get(ctx, env, IDParams{ID: id})
}

func noteTags(ctx context.Context, env *command.Env, id string) ([]string, error) {
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
