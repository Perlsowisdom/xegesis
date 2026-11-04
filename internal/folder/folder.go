// Package folder holds the folders that organise the user's own writing. A folder holds notes and
// documents (sermons, lessons and studies) together, and can sit inside one other folder. Deleting a
// folder puts it in the bin with everything in it; restoring it brings them back together. The bin
// (bin.go) lists deleted notes, documents, lists and folders, and deletes them for good.
//
// Source: Phase 1 survey, rows "Notes organised in notebooks or folders" and "Sermon builder";
// docs/ui-requirements.md, "Library and My work".
package folder

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

const maxName = 120

// Register adds the folder and bin commands to r.
func Register(r *command.Registry) {
	r.Add(
		command.Define("folder.create", "Make a folder for notes and sermons, at the top or inside another folder.", true, create),
		command.Define("folder.rename", "Rename a folder.", true, rename),
		command.Define("folder.move", "Move a folder to the top or inside another top-level folder, optionally to a position among its neighbours.", true, move),
		command.Define("folder.delete", "Delete a folder with its subfolders, notes and sermons. They wait in the bin together until restored.", true, del),
		command.Define("folder.list", "List the folders, top-level ones first in their order, each with how many notes and documents it holds.", false, list),
		command.Define("bin.list", "List what is in the bin: deleted folders, notes, sermons and lists, most recently deleted first.", false, binList),
		command.Define("bin.restore", "Bring something back from the bin. A folder comes back with what was deleted with it.", true, binRestore),
		command.Define("bin.purge", "Delete something in the bin for good. Its text is also removed from the change history. This cannot be undone.", true, binPurge),
	)
}

func now() int64 { return time.Now().UnixMilli() }

func needStore(env *command.Env) error {
	if env.Store == nil {
		return command.Errorf("unavailable", "no library is open")
	}
	return nil
}

// Folder is one folder as callers see it.
type Folder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parent_id,omitempty" desc:"The folder it sits in; empty at the top"`
	Sort      int    `json:"sort" desc:"Order among the folders beside it"`
	Notes     int    `json:"notes" desc:"Notes directly in this folder"`
	Documents int    `json:"documents" desc:"Sermons, lessons and studies directly in this folder"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

func cleanName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || utf8.RuneCountInString(s) > maxName {
		return "", command.Errorf("bad_params", "give the folder a name of up to %d characters", maxName)
	}
	return s, nil
}

// live reads a folder that is not in the bin.
func live(tx *store.Tx, id string) (store.Row, error) {
	cur, err := tx.Get("folders", store.ByID(id))
	if err != nil {
		return nil, command.Errorf("not_found", "no folder %s", id)
	}
	if cur["deleted_at"] != nil {
		return nil, command.Errorf("not_found", "folder %s is in the bin", id)
	}
	return cur, nil
}

// parentFor checks that id can hold another folder: it exists, is not in the bin and is at the top,
// because folders nest one level.
func parentFor(tx *store.Tx, id string) error {
	cur, err := live(tx, id)
	if err != nil {
		return err
	}
	if cur["parent_id"] != nil {
		return command.Errorf("bad_params", "folders nest one level: %q is already inside another folder", cur["name"])
	}
	return nil
}

// nextSort is one more than the highest sort among the folders in parent.
func nextSort(ctx context.Context, env *command.Env, parent string) (int64, error) {
	var n sql.NullInt64
	err := env.Store.QueryRow(ctx, `SELECT MAX(sort) FROM folders WHERE deleted_at IS NULL AND COALESCE(parent_id, '') = ?`, parent).Scan(&n)
	if err != nil {
		return 0, err
	}
	if !n.Valid {
		return 0, nil
	}
	return n.Int64 + 1, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type CreateParams struct {
	Name     string `json:"name" required:"true"`
	ParentID string `json:"parent_id,omitempty" desc:"Put the new folder inside this top-level folder"`
}

func create(ctx context.Context, env *command.Env, p CreateParams) (Folder, error) {
	name, err := cleanName(p.Name)
	if err != nil {
		return Folder{}, err
	}
	sort, err := nextSort(ctx, env, p.ParentID)
	if err != nil {
		return Folder{}, err
	}
	id, t := store.NewID(), now()
	err = env.Store.Write(ctx, "folder.create", func(tx *store.Tx) error {
		if p.ParentID != "" {
			if err := parentFor(tx, p.ParentID); err != nil {
				return err
			}
		}
		return tx.Create("folders", store.Row{"id": id, "name": name, "parent_id": nullable(p.ParentID), "sort": sort,
			"created_at": t, "updated_at": t})
	})
	if err != nil {
		return Folder{}, err
	}
	return get(ctx, env, id)
}

type RenameParams struct {
	ID   string `json:"id" required:"true"`
	Name string `json:"name" required:"true"`
}

func rename(ctx context.Context, env *command.Env, p RenameParams) (Folder, error) {
	name, err := cleanName(p.Name)
	if err != nil {
		return Folder{}, err
	}
	err = env.Store.Write(ctx, "folder.rename", func(tx *store.Tx) error {
		cur, err := live(tx, p.ID)
		if err != nil {
			return err
		}
		if cur["name"] == name {
			return nil
		}
		return tx.Update("folders", store.ByID(p.ID), store.Row{"name": name, "updated_at": now()})
	})
	if err != nil {
		return Folder{}, err
	}
	return get(ctx, env, p.ID)
}

type MoveParams struct {
	ID       string `json:"id" required:"true"`
	ParentID string `json:"parent_id,omitempty" desc:"The top-level folder to move it into; empty for the top"`
	Position *int   `json:"position,omitempty" desc:"Where among the folders beside it, from 0; omit to put it last"`
}

func move(ctx context.Context, env *command.Env, p MoveParams) (Folder, error) {
	if err := needStore(env); err != nil {
		return Folder{}, err
	}
	if p.ParentID == p.ID {
		return Folder{}, command.Errorf("bad_params", "a folder cannot go inside itself")
	}
	var children int
	if err := env.Store.QueryRow(ctx, `SELECT COUNT(*) FROM folders WHERE parent_id = ? AND deleted_at IS NULL`, p.ID).Scan(&children); err != nil {
		return Folder{}, err
	}
	if children > 0 && p.ParentID != "" {
		return Folder{}, command.Errorf("bad_params", "folders nest one level: move the folders inside this one out first")
	}
	siblings, err := siblingIDs(ctx, env, p.ParentID, p.ID)
	if err != nil {
		return Folder{}, err
	}
	at := len(siblings)
	if p.Position != nil && *p.Position >= 0 && *p.Position < at {
		at = *p.Position
	}
	order := append(append(append([]string{}, siblings[:at]...), p.ID), siblings[at:]...)
	err = env.Store.Write(ctx, "folder.move", func(tx *store.Tx) error {
		cur, err := live(tx, p.ID)
		if err != nil {
			return err
		}
		if p.ParentID != "" {
			if err := parentFor(tx, p.ParentID); err != nil {
				return err
			}
		}
		if parent, _ := cur["parent_id"].(string); parent != p.ParentID {
			if err := tx.Update("folders", store.ByID(p.ID), store.Row{"parent_id": nullable(p.ParentID), "updated_at": now()}); err != nil {
				return err
			}
		}
		for i, id := range order {
			if err := tx.Update("folders", store.ByID(id), store.Row{"sort": int64(i)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Folder{}, err
	}
	return get(ctx, env, p.ID)
}

// siblingIDs lists the live folders in parent in order, leaving out skip.
func siblingIDs(ctx context.Context, env *command.Env, parent, skip string) ([]string, error) {
	return ids(ctx, env, `SELECT id FROM folders WHERE deleted_at IS NULL AND COALESCE(parent_id, '') = ? AND id <> ?
		ORDER BY sort, name COLLATE NOCASE, id`, parent, skip)
}

func ids(ctx context.Context, env *command.Env, q string, args ...any) ([]string, error) {
	rows, err := env.Store.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

type IDParams struct {
	ID string `json:"id" required:"true"`
}

// Deleted counts what a folder.delete put in the bin.
type Deleted struct {
	Folders   int `json:"folders" desc:"The folder and its subfolders"`
	Notes     int `json:"notes"`
	Documents int `json:"documents"`
}

func del(ctx context.Context, env *command.Env, p IDParams) (Deleted, error) {
	if err := needStore(env); err != nil {
		return Deleted{}, err
	}
	subs, err := ids(ctx, env, `SELECT id FROM folders WHERE parent_id = ? AND deleted_at IS NULL`, p.ID)
	if err != nil {
		return Deleted{}, err
	}
	all := append([]string{p.ID}, subs...)
	notes, err := ids(ctx, env, `SELECT id FROM notes WHERE deleted_at IS NULL AND folder_id IN (`+marks(all)+`)`, anys(all)...)
	if err != nil {
		return Deleted{}, err
	}
	docs, err := ids(ctx, env, `SELECT id FROM documents WHERE deleted_at IS NULL AND folder_id IN (`+marks(all)+`)`, anys(all)...)
	if err != nil {
		return Deleted{}, err
	}
	// The folder goes first, so everything deleted with it has a deleted_at no earlier than its own:
	// that is how a restore knows what to bring back.
	err = env.Store.Write(ctx, "folder.delete", func(tx *store.Tx) error {
		if _, err := live(tx, p.ID); err != nil {
			return err
		}
		for _, x := range []struct {
			table string
			ids   []string
		}{{"folders", all}, {"notes", notes}, {"documents", docs}} {
			for _, id := range x.ids {
				if err := tx.Delete(x.table, store.ByID(id)); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return Deleted{}, err
	}
	return Deleted{Folders: len(all), Notes: len(notes), Documents: len(docs)}, nil
}

func marks(ids []string) string {
	return strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
}

func anys(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

const folderSelect = `SELECT f.id, f.name, COALESCE(f.parent_id, ''), f.sort, f.created_at, f.updated_at,
	(SELECT COUNT(*) FROM notes n WHERE n.folder_id = f.id AND n.deleted_at IS NULL),
	(SELECT COUNT(*) FROM documents d WHERE d.folder_id = f.id AND d.deleted_at IS NULL)
	FROM folders f`

func query(ctx context.Context, env *command.Env, where string, args ...any) ([]Folder, error) {
	rows, err := env.Store.Query(ctx, folderSelect+" "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Folder{}
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.Name, &f.ParentID, &f.Sort, &f.CreatedAt, &f.UpdatedAt, &f.Notes, &f.Documents); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func get(ctx context.Context, env *command.Env, id string) (Folder, error) {
	got, err := query(ctx, env, `WHERE f.id = ? AND f.deleted_at IS NULL`, id)
	if err != nil {
		return Folder{}, err
	}
	if len(got) == 0 {
		return Folder{}, command.Errorf("not_found", "no folder %s", id)
	}
	return got[0], nil
}

func list(ctx context.Context, env *command.Env, _ struct{}) ([]Folder, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	// Top-level folders in order, each followed by its own subfolders in order.
	return query(ctx, env, `LEFT JOIN folders p ON p.id = f.parent_id WHERE f.deleted_at IS NULL
		ORDER BY COALESCE(p.sort, f.sort), COALESCE(p.name, f.name) COLLATE NOCASE, COALESCE(p.id, f.id),
		f.parent_id IS NOT NULL, f.sort, f.name COLLATE NOCASE, f.id`)
}
