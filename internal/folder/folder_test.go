package folder

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/document"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

type env struct {
	t   *testing.T
	r   *command.Registry
	env *command.Env
	st  *store.Store
}

func setup(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	r := command.NewRegistry()
	core.Register(r)
	document.Register(r)
	Register(r)
	return &env{t: t, r: r, env: &command.Env{Store: st}, st: st}
}

func (e *env) run(id string, params any, out any) error {
	e.t.Helper()
	raw, _ := json.Marshal(params)
	res, err := e.r.Run(context.Background(), e.env, id, raw)
	if err != nil {
		return err
	}
	if out != nil {
		b, _ := json.Marshal(res)
		if err := json.Unmarshal(b, out); err != nil {
			e.t.Fatal(err)
		}
	}
	return nil
}

func (e *env) must(id string, params any, out any) {
	e.t.Helper()
	if err := e.run(id, params, out); err != nil {
		e.t.Fatalf("%s: %v", id, err)
	}
}

func code(err error) string {
	var ce *command.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func (e *env) folders() []Folder {
	e.t.Helper()
	var fs []Folder
	e.must("folder.list", map[string]any{}, &fs)
	return fs
}

func names(fs []Folder) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, f.Name)
	}
	return out
}

func TestCreateRenameNestAndOrder(t *testing.T) {
	e := setup(t)
	var a, b, c, sub Folder
	e.must("folder.create", map[string]any{"name": "  Sermon   series "}, &a)
	e.must("folder.create", map[string]any{"name": "Romans"}, &b)
	e.must("folder.create", map[string]any{"name": "Advent", "parent_id": a.ID}, &sub)
	if a.Name != "Sermon series" || sub.ParentID != a.ID || b.Sort != 1 {
		t.Fatalf("created %+v %+v %+v", a, b, sub)
	}
	// Folders nest one level.
	if err := e.run("folder.create", map[string]any{"name": "Too deep", "parent_id": sub.ID}, nil); code(err) != "bad_params" {
		t.Fatalf("nested two levels: %v", err)
	}
	if err := e.run("folder.move", map[string]any{"id": a.ID, "parent_id": b.ID}, nil); code(err) != "bad_params" {
		t.Fatalf("moved a folder with subfolders inside another: %v", err)
	}
	if err := e.run("folder.create", map[string]any{"name": "   "}, nil); code(err) != "bad_params" {
		t.Fatalf("empty name: %v", err)
	}
	e.must("folder.rename", map[string]any{"id": b.ID, "name": "Romans 2026"}, &b)
	if b.Name != "Romans 2026" {
		t.Fatalf("renamed = %+v", b)
	}
	// The list is in order, each top-level folder followed by its own.
	if got := names(e.folders()); len(got) != 3 || got[0] != "Sermon series" || got[1] != "Advent" || got[2] != "Romans 2026" {
		t.Fatalf("list = %v", got)
	}
	zero := 0
	e.must("folder.move", map[string]any{"id": b.ID, "position": zero}, nil)
	if got := names(e.folders()); got[0] != "Romans 2026" || got[1] != "Sermon series" || got[2] != "Advent" {
		t.Fatalf("after reorder = %v", got)
	}
	// A subfolder moves to the top, and a top folder without subfolders moves into another.
	var subMoved, cMoved Folder
	e.must("folder.move", map[string]any{"id": sub.ID}, &subMoved)
	e.must("folder.create", map[string]any{"name": "Lent"}, &c)
	e.must("folder.move", map[string]any{"id": c.ID, "parent_id": b.ID}, &cMoved)
	if subMoved.ParentID != "" || cMoved.ParentID != b.ID {
		t.Fatalf("moved %+v %+v", subMoved, cMoved)
	}
	var n int
	e.st.QueryRow(context.Background(), `SELECT COUNT(*) FROM changes WHERE object_type = 'folders' AND command LIKE 'folder.%'`).Scan(&n)
	if n < 8 {
		t.Fatalf("changes logged = %d", n)
	}
}

func TestFolderHoldsNotesAndSermonsAndItemsMove(t *testing.T) {
	e := setup(t)
	var f, g Folder
	e.must("folder.create", map[string]any{"name": "Romans"}, &f)
	e.must("folder.create", map[string]any{"name": "Other"}, &g)
	var n core.Note
	e.must("note.create", map[string]any{"body": "Justified by faith", "ref": "Rom 5:1", "folder_id": f.ID}, &n)
	var d document.Document
	e.must("document.create", map[string]any{"title": "Peace with God", "folder_id": f.ID, "preached_on": "2026-10-04", "theme": " peace "}, &d)
	if n.FolderID != f.ID || d.FolderID != f.ID || d.PreachedOn != "2026-10-04" || d.Theme != "peace" {
		t.Fatalf("filed %+v %+v", n, d)
	}
	got := e.folders()
	if got[0].Name != "Romans" || got[0].Notes != 1 || got[0].Documents != 1 {
		t.Fatalf("counts = %+v", got)
	}
	// Moving into another folder keeps the note's passage and its edit time.
	var moved core.Note
	e.must("note.move", map[string]any{"id": n.ID, "folder_id": g.ID}, &moved)
	if moved.FolderID != g.ID || moved.Ref == nil || moved.UpdatedAt != n.UpdatedAt {
		t.Fatalf("moved note = %+v", moved)
	}
	var notes []core.Note
	e.must("note.search", map[string]any{"folder_id": g.ID}, &notes)
	if len(notes) != 1 {
		t.Fatalf("note.search by folder = %d", len(notes))
	}
	var unfiled core.Note
	e.must("note.move", map[string]any{"id": n.ID, "folder_id": ""}, &unfiled)
	e.must("note.search", map[string]any{"folder_id": "none"}, &notes)
	if unfiled.FolderID != "" || len(notes) != 1 {
		t.Fatalf("unfiled = %+v, %d", unfiled, len(notes))
	}
	var dm document.Document
	e.must("document.move", map[string]any{"id": d.ID, "folder_id": g.ID}, &dm)
	var docs []document.Document
	e.must("document.list", map[string]any{"folder_id": g.ID}, &docs)
	if dm.FolderID != g.ID || len(docs) != 1 {
		t.Fatalf("moved document = %+v, %d", dm, len(docs))
	}
	if err := e.run("document.move", map[string]any{"id": d.ID, "folder_id": "nope"}, nil); code(err) != "not_found" {
		t.Fatalf("move to a missing folder: %v", err)
	}
	if err := e.run("note.move", map[string]any{"id": n.ID, "folder_id": g.ID, "free": true}, nil); code(err) != "bad_params" {
		t.Fatalf("two moves at once: %v", err)
	}
	if err := e.run("document.update", map[string]any{"id": d.ID, "preached_on": "4 Oct"}, nil); code(err) != "bad_params" {
		t.Fatalf("bad date: %v", err)
	}
	var cleared document.Document
	e.must("document.update", map[string]any{"id": d.ID, "preached_on": "", "theme": ""}, &cleared)
	if cleared.PreachedOn != "" || cleared.Theme != "" {
		t.Fatalf("cleared = %+v", cleared)
	}
}

func binOf(e *env) []BinEntry {
	e.t.Helper()
	var b []BinEntry
	e.must("bin.list", map[string]any{}, &b)
	return b
}

func TestDeleteFolderGoesToBinWithItsContents(t *testing.T) {
	e := setup(t)
	var top, sub Folder
	e.must("folder.create", map[string]any{"name": "Series"}, &top)
	e.must("folder.create", map[string]any{"name": "Week 1", "parent_id": top.ID}, &sub)
	var early, n1, n2 core.Note
	e.must("note.create", map[string]any{"body": "deleted before", "folder_id": top.ID}, &early)
	e.must("note.delete", map[string]any{"id": early.ID}, nil)
	e.must("note.create", map[string]any{"body": "one", "folder_id": top.ID}, &n1)
	e.must("note.create", map[string]any{"body": "two", "folder_id": sub.ID, "tags": []string{"grace"}}, &n2)
	var d document.Document
	e.must("document.create", map[string]any{"title": "Sermon", "folder_id": sub.ID}, &d)

	var del Deleted
	e.must("folder.delete", map[string]any{"id": top.ID}, &del)
	if del != (Deleted{Folders: 2, Notes: 2, Documents: 1}) {
		t.Fatalf("deleted = %+v", del)
	}
	if fs := e.folders(); len(fs) != 0 {
		t.Fatalf("folders left = %v", names(fs))
	}
	if err := e.run("note.get", map[string]any{"id": n2.ID}, nil); code(err) != "not_found" {
		t.Fatalf("note in a deleted folder is still live: %v", err)
	}
	// The bin shows the folder with what went with it; the note deleted earlier is with the folder,
	// not on its own.
	bin := binOf(e)
	if len(bin) != 1 || bin[0].Kind != "folder" || bin[0].Folders != 1 || bin[0].Notes != 2 || bin[0].Documents != 1 {
		t.Fatalf("bin = %+v", bin)
	}

	var back Restored
	e.must("bin.restore", map[string]any{"kind": "folder", "id": top.ID}, &back)
	if back != (Restored{Folders: 2, Notes: 2, Documents: 1}) {
		t.Fatalf("restored = %+v", back)
	}
	if fs := e.folders(); len(fs) != 2 || fs[1].ParentID != top.ID || fs[1].Documents != 1 {
		t.Fatalf("folders back = %+v", fs)
	}
	var got core.Note
	e.must("note.get", map[string]any{"id": n2.ID}, &got)
	if got.FolderID != sub.ID || len(got.Tags) != 1 {
		t.Fatalf("restored note = %+v", got)
	}
	// The note deleted on its own before stays in the bin, now listed by itself.
	bin = binOf(e)
	if len(bin) != 1 || bin[0].Kind != "note" || bin[0].ID != early.ID {
		t.Fatalf("bin after restore = %+v", bin)
	}
}

func TestRestoreItemFromDeletedFolderComesBackAtTop(t *testing.T) {
	e := setup(t)
	var f Folder
	e.must("folder.create", map[string]any{"name": "Gone"}, &f)
	var n core.Note
	e.must("note.create", map[string]any{"body": "x", "folder_id": f.ID}, &n)
	e.must("folder.delete", map[string]any{"id": f.ID}, nil)
	var back core.Note
	e.must("note.restore", map[string]any{"id": n.ID}, &back)
	if back.FolderID != "" {
		t.Fatalf("restored into a deleted folder: %+v", back)
	}
}

func TestPurgeFolderNoteDocumentAndList(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var f Folder
	e.must("folder.create", map[string]any{"name": "Private"}, &f)
	var n core.Note
	e.must("note.create", map[string]any{"body": "secret words", "ref": "John 3:16", "folder_id": f.ID, "tags": []string{"mine"}}, &n)
	e.must("note.move", map[string]any{"id": n.ID, "ref": "John 3:17"}, nil)
	var d document.Document
	e.must("document.create", map[string]any{"title": "Draft", "body": "secret sermon"}, &d)
	var listID string
	err := e.st.Write(ctx, "list.create", func(tx *store.Tx) error {
		listID = store.NewID()
		return tx.Create("lists", store.Row{"id": listID, "kind": "verses", "name": "Old", "created_at": int64(1), "updated_at": int64(1)})
	})
	if err != nil {
		t.Fatal(err)
	}
	e.st.Write(ctx, "list.delete", func(tx *store.Tx) error { return tx.Delete("lists", store.ByID(listID)) })
	e.must("folder.delete", map[string]any{"id": f.ID}, nil)
	e.must("document.delete", map[string]any{"id": d.ID}, nil)

	if err := e.run("bin.purge", map[string]any{"kind": "document", "id": n.ID}, nil); code(err) != "not_found" {
		t.Fatalf("purge with the wrong kind: %v", err)
	}
	var p Purged
	e.must("bin.purge", map[string]any{"kind": "folder", "id": f.ID}, &p)
	if p.Folders != 1 || p.Notes != 1 {
		t.Fatalf("purged = %+v", p)
	}
	e.must("bin.purge", map[string]any{"kind": "document", "id": d.ID}, nil)
	e.must("bin.purge", map[string]any{"kind": "list", "id": listID}, nil)
	if bin := binOf(e); len(bin) != 0 {
		t.Fatalf("bin = %+v", bin)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM notes`, `SELECT COUNT(*) FROM note_tags`, `SELECT COUNT(*) FROM anchors`,
		`SELECT COUNT(*) FROM folders`, `SELECT COUNT(*) FROM documents`, `SELECT COUNT(*) FROM lists`,
		`SELECT COUNT(*) FROM changes WHERE before_json LIKE '%secret%' OR after_json LIKE '%secret%'`,
	} {
		var c int
		if err := e.st.QueryRow(ctx, q).Scan(&c); err != nil || c != 0 {
			t.Fatalf("%s = %d, %v", q, c, err)
		}
	}
	// Something not in the bin cannot be purged.
	var live document.Document
	e.must("document.create", map[string]any{"title": "Keep"}, &live)
	if err := e.run("bin.purge", map[string]any{"kind": "document", "id": live.ID}, nil); code(err) != "bad_params" {
		t.Fatalf("purge a live document: %v", err)
	}
}
