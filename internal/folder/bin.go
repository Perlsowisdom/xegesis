package folder

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Kinds of thing the bin holds, and the table each lives in.
var binTables = map[string]string{"folder": "folders", "note": "notes", "document": "documents", "list": "lists"}

// BinEntry is one thing in the bin.
type BinEntry struct {
	Kind      string `json:"kind" enum:"folder,note,document,list"`
	ID        string `json:"id"`
	Title     string `json:"title" desc:"Its name or title; empty for an untitled note"`
	Sort      string `json:"sort,omitempty" desc:"A document's kind (sermon, lesson or study) or a list's kind (verses, words or excerpts)"`
	DeletedAt int64  `json:"deleted_at"`
	Folders   int    `json:"folders,omitempty" desc:"A folder: subfolders deleted with it"`
	Notes     int    `json:"notes,omitempty" desc:"A folder: notes deleted with it"`
	Documents int    `json:"documents,omitempty" desc:"A folder: documents deleted with it"`
}

// Things inside a deleted folder are listed with the folder, not on their own.
const binQuery = `
SELECT 'folder', f.id, f.name, '', f.deleted_at,
	(SELECT COUNT(*) FROM folders s WHERE s.parent_id = f.id AND s.deleted_at >= f.deleted_at),
	(SELECT COUNT(*) FROM notes n WHERE n.deleted_at >= f.deleted_at AND n.folder_id IN
		(SELECT f.id UNION SELECT s.id FROM folders s WHERE s.parent_id = f.id AND s.deleted_at >= f.deleted_at)),
	(SELECT COUNT(*) FROM documents d WHERE d.deleted_at >= f.deleted_at AND d.folder_id IN
		(SELECT f.id UNION SELECT s.id FROM folders s WHERE s.parent_id = f.id AND s.deleted_at >= f.deleted_at))
	FROM folders f LEFT JOIN folders p ON p.id = f.parent_id
	WHERE f.deleted_at IS NOT NULL AND (p.id IS NULL OR p.deleted_at IS NULL)
UNION ALL
SELECT 'note', n.id, COALESCE(n.title, ''), '', n.deleted_at, 0, 0, 0
	FROM notes n LEFT JOIN folders f ON f.id = n.folder_id
	WHERE n.deleted_at IS NOT NULL AND (f.id IS NULL OR f.deleted_at IS NULL)
UNION ALL
SELECT 'document', d.id, d.title, d.kind, d.deleted_at, 0, 0, 0
	FROM documents d LEFT JOIN folders f ON f.id = d.folder_id
	WHERE d.deleted_at IS NOT NULL AND (f.id IS NULL OR f.deleted_at IS NULL)
UNION ALL
SELECT 'list', l.id, l.name, l.kind, l.deleted_at, 0, 0, 0 FROM lists l WHERE l.deleted_at IS NOT NULL
ORDER BY 5 DESC, 2 LIMIT ?`

type BinListParams struct {
	Limit int `json:"limit,omitempty" desc:"Most entries to return (default 500)"`
}

func binList(ctx context.Context, env *command.Env, p BinListParams) ([]BinEntry, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	if p.Limit <= 0 {
		p.Limit = 500
	}
	rows, err := env.Store.Query(ctx, binQuery, p.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BinEntry{}
	for rows.Next() {
		var e BinEntry
		if err := rows.Scan(&e.Kind, &e.ID, &e.Title, &e.Sort, &e.DeletedAt, &e.Folders, &e.Notes, &e.Documents); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type BinParams struct {
	Kind string `json:"kind" required:"true" enum:"folder,note,document,list"`
	ID   string `json:"id" required:"true"`
}

// Restored counts what a bin.restore brought back.
type Restored struct {
	Folders   int `json:"folders"`
	Notes     int `json:"notes"`
	Documents int `json:"documents"`
	Lists     int `json:"lists"`
}

// deletedRow reads a row in the bin and its deleted_at.
func deletedRow(ctx context.Context, env *command.Env, p BinParams) (int64, error) {
	table, ok := binTables[p.Kind]
	if !ok {
		return 0, command.Errorf("bad_params", "kind must be folder, note, document or list")
	}
	var at sql.NullInt64
	err := env.Store.QueryRow(ctx, `SELECT deleted_at FROM `+table+` WHERE id = ?`, p.ID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, command.Errorf("not_found", "no %s %s", p.Kind, p.ID)
	} else if err != nil {
		return 0, err
	}
	if !at.Valid {
		return 0, command.Errorf("bad_params", "%s %s is not in the bin", p.Kind, p.ID)
	}
	return at.Int64, nil
}

// folderContents finds what was deleted with a folder at or after since: its subfolders, and the
// notes and documents in it or them.
func folderContents(ctx context.Context, env *command.Env, id string, since int64) (subs, notes, docs []string, err error) {
	if subs, err = ids(ctx, env, `SELECT id FROM folders WHERE parent_id = ? AND deleted_at >= ?`, id, since); err != nil {
		return
	}
	all := append([]string{id}, subs...)
	args := append(anys(all), since)
	if notes, err = ids(ctx, env, `SELECT id FROM notes WHERE folder_id IN (`+marks(all)+`) AND deleted_at >= ?`, args...); err != nil {
		return
	}
	docs, err = ids(ctx, env, `SELECT id FROM documents WHERE folder_id IN (`+marks(all)+`) AND deleted_at >= ?`, args...)
	return
}

func binRestore(ctx context.Context, env *command.Env, p BinParams) (Restored, error) {
	if err := needStore(env); err != nil {
		return Restored{}, err
	}
	since, err := deletedRow(ctx, env, p)
	if err != nil {
		return Restored{}, err
	}
	var subs, notes, docs []string
	if p.Kind == "folder" {
		if subs, notes, docs, err = folderContents(ctx, env, p.ID, since); err != nil {
			return Restored{}, err
		}
	}
	var got Restored
	err = env.Store.Write(ctx, "bin.restore", func(tx *store.Tx) error {
		table := binTables[p.Kind]
		cur, err := tx.Get(table, store.ByID(p.ID))
		if err != nil {
			return err
		}
		fields := store.Row{"deleted_at": nil}
		// Something whose folder is still in the bin comes back at the top.
		switch p.Kind {
		case "folder":
			if parent, ok := cur["parent_id"].(string); ok && !folderLive(tx, parent) {
				fields["parent_id"] = nil
			}
		case "note", "document":
			if f, ok := cur["folder_id"].(string); ok && !folderLive(tx, f) {
				fields["folder_id"] = nil
			}
		}
		if err := tx.Update(table, store.ByID(p.ID), fields); err != nil {
			return err
		}
		for _, x := range []struct {
			table string
			ids   []string
		}{{"folders", subs}, {"notes", notes}, {"documents", docs}} {
			for _, id := range x.ids {
				if err := tx.Update(x.table, store.ByID(id), store.Row{"deleted_at": nil}); err != nil {
					return err
				}
			}
		}
		got = Restored{Folders: len(subs), Notes: len(notes), Documents: len(docs)}
		switch p.Kind {
		case "folder":
			got.Folders++
		case "note":
			got.Notes++
		case "document":
			got.Documents++
		case "list":
			got.Lists++
		}
		return nil
	})
	return got, err
}

func folderLive(tx *store.Tx, id string) bool {
	row, err := tx.Get("folders", store.ByID(id))
	return err == nil && row["deleted_at"] == nil
}

// Purged counts what a bin.purge removed for good.
type Purged = Restored

func binPurge(ctx context.Context, env *command.Env, p BinParams) (Purged, error) {
	if err := needStore(env); err != nil {
		return Purged{}, err
	}
	since, err := deletedRow(ctx, env, p)
	if err != nil {
		return Purged{}, err
	}
	var out Purged
	var folders, notes, docs, entries, lists []string
	switch p.Kind {
	case "folder":
		var subs []string
		if subs, notes, docs, err = folderContents(ctx, env, p.ID, since); err != nil {
			return Purged{}, err
		}
		folders = append(subs, p.ID) // subfolders before the folder that holds them
	case "note":
		notes = []string{p.ID}
	case "document":
		docs = []string{p.ID}
	case "list":
		if entries, err = ids(ctx, env, `SELECT id FROM list_entries WHERE list_id = ?`, p.ID); err != nil {
			return Purged{}, err
		}
		lists = []string{p.ID}
	}
	out = Purged{Folders: len(folders), Notes: len(notes), Documents: len(docs), Lists: len(lists)}

	// A note's tags and every place it was ever attached go with it, as do a list entry's places.
	tags := map[string][]string{}
	places := map[string]bool{}
	for _, n := range notes {
		if tags[n], err = ids(ctx, env, `SELECT tag FROM note_tags WHERE note_id = ?`, n); err != nil {
			return Purged{}, err
		}
		if err := pastAnchors(ctx, env, "notes", n, places); err != nil {
			return Purged{}, err
		}
	}
	for _, e := range entries {
		if err := pastAnchors(ctx, env, "list_entries", e, places); err != nil {
			return Purged{}, err
		}
	}
	anchors, err := unusedAnchors(ctx, env, places, notes, entries)
	if err != nil {
		return Purged{}, err
	}
	// Anything else still filed in a purged folder (deleted on its own before the folder was)
	// stays, at the top.
	var strayNotes, strayDocs []string
	if len(folders) > 0 {
		args := append(anys(folders), since)
		q := ` WHERE folder_id IN (` + marks(folders) + `) AND (deleted_at IS NULL OR deleted_at < ?)`
		if strayNotes, err = ids(ctx, env, `SELECT id FROM notes`+q, args...); err != nil {
			return Purged{}, err
		}
		if strayDocs, err = ids(ctx, env, `SELECT id FROM documents`+q, args...); err != nil {
			return Purged{}, err
		}
	}

	err = env.Store.Write(ctx, "bin.purge", func(tx *store.Tx) error {
		for _, x := range []struct {
			table string
			ids   []string
		}{{"notes", strayNotes}, {"documents", strayDocs}} {
			for _, id := range x.ids {
				if err := tx.Update(x.table, store.ByID(id), store.Row{"folder_id": nil}); err != nil {
					return err
				}
			}
		}
		for _, n := range notes {
			for _, t := range tags[n] {
				if err := tx.Purge("note_tags", store.Row{"note_id": n, "tag": t}); err != nil {
					return err
				}
			}
		}
		// Children first, so no foreign key ever points at a removed row.
		for _, x := range []struct {
			table string
			ids   []string
		}{{"notes", notes}, {"documents", docs}, {"list_entries", entries}, {"lists", lists}, {"folders", folders}, {"anchors", anchors}} {
			for _, id := range x.ids {
				if err := tx.Purge(x.table, store.ByID(id)); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return Purged{}, err
	}
	return out, nil
}

// pastAnchors adds to found every anchor a row has pointed to, from its history and its current row.
func pastAnchors(ctx context.Context, env *command.Env, table, id string, found map[string]bool) error {
	rows, err := env.Store.Query(ctx, `SELECT op, before_json, after_json FROM changes WHERE object_type = ? AND object_id = ?
		AND (field = 'anchor_id' OR op = 'create')`, table, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var op string
		var before, after sql.NullString
		if err := rows.Scan(&op, &before, &after); err != nil {
			return err
		}
		for _, raw := range []sql.NullString{before, after} {
			if !raw.Valid {
				continue
			}
			if op == "create" {
				var row struct {
					Anchor *string `json:"anchor_id"`
				}
				if json.Unmarshal([]byte(raw.String), &row) == nil && row.Anchor != nil {
					found[*row.Anchor] = true
				}
			} else {
				var a string
				if json.Unmarshal([]byte(raw.String), &a) == nil && a != "" {
					found[a] = true
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var cur sql.NullString
	err = env.Store.QueryRow(ctx, `SELECT anchor_id FROM `+table+` WHERE id = ?`, id).Scan(&cur)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if cur.Valid {
		found[cur.String] = true
	}
	return nil
}

// unusedAnchors keeps the anchors that still exist and that nothing outside the purge points to.
func unusedAnchors(ctx context.Context, env *command.Env, places map[string]bool, notes, entries []string) ([]string, error) {
	skipNotes, skipEntries := append([]string{""}, notes...), append([]string{""}, entries...)
	out := []string{}
	for a := range places {
		var exists, used int
		args := append([]any{a, a}, anys(skipNotes)...)
		args = append(append(args, a, a), anys(skipEntries)...)
		err := env.Store.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM anchors WHERE id = ?),
			(SELECT COUNT(*) FROM notes WHERE anchor_id = ? AND id NOT IN (`+marks(skipNotes)+`)) +
			(SELECT COUNT(*) FROM highlights WHERE anchor_id = ?) +
			(SELECT COUNT(*) FROM list_entries WHERE anchor_id = ? AND id NOT IN (`+marks(skipEntries)+`))`,
			args...).Scan(&exists, &used)
		if err != nil {
			return nil, err
		}
		if exists == 1 && used == 0 {
			out = append(out, a)
		}
	}
	return out, nil
}
