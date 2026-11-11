package search

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

type env struct {
	t    *testing.T
	data string
	reg  *command.Registry
	svc  *service
	env  *command.Env
}

func setup(t *testing.T) *env {
	t.Helper()
	data := t.TempDir()
	s, err := store.Open(filepath.Join(data, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	e := &env{t: t, data: data, env: &command.Env{Store: s}}
	e.restart()
	return e
}

// restart builds a fresh registry and search service over the same data folder, as after the app
// is restarted.
func (e *env) restart() {
	if e.svc != nil {
		e.svc.closeAll()
	}
	e.reg = command.NewRegistry()
	library.Register(e.reg)
	e.svc = newService(e.reg, e.data)
	register(e.reg, e.svc)
	e.t.Cleanup(e.svc.closeAll)
}

func (e *env) run(ctx context.Context, id string, params, out any) error {
	e.t.Helper()
	raw, _ := json.Marshal(params)
	res, err := e.reg.Run(ctx, e.env, id, raw)
	if err != nil {
		return err
	}
	if out != nil {
		reflect.ValueOf(out).Elem().SetZero() // decode each result afresh
		b, _ := json.Marshal(res)
		if err := json.Unmarshal(b, out); err != nil {
			e.t.Fatal(err)
		}
	}
	return nil
}

func (e *env) must(id string, params, out any) {
	e.t.Helper()
	if err := e.run(context.Background(), id, params, out); err != nil {
		e.t.Fatalf("%s: %v", id, err)
	}
}

// folder adds dir as a library folder and scans it.
func (e *env) folder(dir string) string {
	e.t.Helper()
	var f library.Folder
	e.must("library.add_folder", map[string]any{"path": dir}, &f)
	e.must("library.scan", map[string]any{"folder_id": f.ID}, nil)
	return f.ID
}

func (e *env) index(folderID string) Progress {
	e.t.Helper()
	var res IndexResult
	e.must("search.index", map[string]any{"folder_id": folderID, "wait": true}, &res)
	if len(res.Jobs) != 1 || res.Jobs[0].State != "complete" {
		e.t.Fatalf("index: %+v", res)
	}
	return res.Jobs[0]
}

func (e *env) query(params map[string]any) QueryResult {
	e.t.Helper()
	var res QueryResult
	e.must("search.query", params, &res)
	return res
}

func (e *env) status(folderID string) FolderStatus {
	e.t.Helper()
	var st []FolderStatus
	e.must("search.status", map[string]any{"folder_id": folderID}, &st)
	return st[0]
}

func titles(hits []Hit) string {
	var out []string
	for _, h := range hits {
		out = append(out, h.Title)
	}
	return strings.Join(out, "|")
}

func TestIndexAndQuery(t *testing.T) {
	e := setup(t)
	dir := t.TempDir()
	books(t, dir)
	id := e.folder(dir)
	p := e.index(id)
	if p.Files != 6 || p.Indexed != 6 || p.Failed != 0 || p.JobID == "" {
		t.Fatalf("progress %+v", p)
	}
	if _, err := os.Stat(filepath.Join(e.data, "index", id+".db")); err != nil {
		t.Fatalf("index file: %v", err)
	}
	st := e.status(id)
	if st.Files != 6 || st.Indexed != 6 || st.State != "complete" || st.Running || st.Passages == 0 {
		t.Errorf("status %+v", st)
	}

	res := e.query(map[string]any{"query": "righteous faith"})
	if len(res.Hits) != 1 {
		t.Fatalf("hits %+v", res)
	}
	h := res.Hits[0]
	if h.Title != "Lectures on Romans" || h.RelPath != "luther.epub" || h.Format != "epub" || !h.Online ||
		h.Locator.Spine != 1 || h.Heading != "Chapter One" || h.Authors[0] != "Martin Luther" ||
		!strings.Contains(h.Snippet, "<mark>righteous</mark>") || h.Score <= 0 {
		t.Errorf("hit %+v", h)
	}

	// grace: html (title and twice in text), pdf (once). The HTML page ranks first.
	res = e.query(map[string]any{"query": "grace"})
	if len(res.Hits) < 2 || res.Hits[0].Format != "html" || res.More || len(res.Incomplete) != 0 {
		t.Errorf("grace: %+v", res)
	}
	page := e.query(map[string]any{"query": "grace", "limit": 1})
	if len(page.Hits) != 1 || !page.More {
		t.Errorf("page 1: %+v", page)
	}
	page = e.query(map[string]any{"query": "grace", "limit": 1, "offset": 1})
	if len(page.Hits) != 1 || page.Hits[0].PassageID == res.Hits[0].PassageID {
		t.Errorf("page 2: %+v", page)
	}

	// Phrase versus words.
	if r := e.query(map[string]any{"query": "faith alone", "mode": "phrase"}); titles(r.Hits) != "commentary" {
		t.Errorf("phrase: %s", titles(r.Hits))
	}
	if r := e.query(map[string]any{"query": "alone faith", "mode": "phrase"}); len(r.Hits) != 0 {
		t.Errorf("reversed phrase: %s", titles(r.Hits))
	}
	if r := e.query(map[string]any{"query": "pilgrim mercy"}); len(r.Hits) != 0 {
		t.Errorf("all words: %s", titles(r.Hits))
	}
	if r := e.query(map[string]any{"query": "pilgrim mercy", "mode": "any"}); len(r.Hits) != 2 {
		t.Errorf("any word: %s", titles(r.Hits))
	}

	// Filters.
	if r := e.query(map[string]any{"query": "pilgrim mercy", "mode": "any", "author": "bunyan"}); titles(r.Hits) != "Study Notes" {
		t.Errorf("author filter: %s", titles(r.Hits))
	}
	if r := e.query(map[string]any{"query": "grace", "kind": "bible"}); len(r.Hits) != 0 {
		t.Errorf("kind filter: %s", titles(r.Hits))
	}
	if r := e.query(map[string]any{"query": "grace", "item_ids": []string{h.ItemID}}); len(r.Hits) != 0 {
		t.Errorf("item filter: %s", titles(r.Hits))
	}

	var ce *command.Error
	err := e.run(context.Background(), "search.query", map[string]any{"query": "(", "mode": "fts"}, nil)
	if !errors.As(err, &ce) || ce.Code != "bad_params" {
		t.Errorf("bad fts: %v", err)
	}
	err = e.run(context.Background(), "search.query", map[string]any{"query": "\"\""}, nil)
	if !errors.As(err, &ce) || ce.Code != "bad_params" {
		t.Errorf("empty query: %v", err)
	}
}

func TestIncrementalReindex(t *testing.T) {
	e := setup(t)
	dir := t.TempDir()
	books(t, dir)
	id := e.folder(dir)
	e.index(id)
	if p := e.index(id); p.ToIndex != 0 || p.Unchanged != 6 {
		t.Errorf("second run: %+v", p)
	}
	writeFile(t, filepath.Join(dir, "plain.txt"), []byte("Entirely new words about covenant.\n"))
	os.Remove(filepath.Join(dir, "page.html"))
	e.must("library.scan", map[string]any{"folder_id": id}, nil)
	p := e.index(id)
	if p.ToIndex != 1 || p.Indexed != 1 || p.Removed != 1 || p.Unchanged != 4 {
		t.Errorf("after change: %+v", p)
	}
	if r := e.query(map[string]any{"query": "mercy"}); len(r.Hits) != 0 {
		t.Errorf("old text still found: %+v", r.Hits)
	}
	if r := e.query(map[string]any{"query": "covenant"}); titles(r.Hits) != "Plain Text" {
		t.Errorf("new text: %+v", r.Hits)
	}
	if r := e.query(map[string]any{"query": "amazing"}); len(r.Hits) != 0 {
		t.Errorf("removed file still found: %+v", r.Hits)
	}
}

func TestResumeAfterInterruption(t *testing.T) {
	e := setup(t)
	dir := t.TempDir()
	books(t, dir)
	id := e.folder(dir)
	e.svc.workers = 1
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	e.svc.onFile = func(string) {
		if n++; n == 2 {
			cancel()
		}
	}
	var res IndexResult
	if err := e.run(ctx, "search.index", map[string]any{"wait": true}, &res); err != nil {
		t.Fatal(err)
	}
	if res.Jobs[0].State != "cancelled" || res.Jobs[0].Indexed != 2 {
		t.Fatalf("interrupted run: %+v", res.Jobs[0])
	}
	// Pretend the app died mid-run.
	ix, _ := e.svc.index(id, false)
	ix.setState(context.Background(), map[string]string{"status": "running"})

	e.restart()
	st := e.status(id)
	if st.State != "interrupted" || st.Indexed != 2 || st.Files != 6 || st.Running {
		t.Errorf("status after restart: %+v", st)
	}
	p := e.index(id)
	if p.ToIndex != 4 || p.Indexed != 4 || p.Unchanged != 2 {
		t.Errorf("resumed run: %+v", p)
	}
	if st := e.status(id); st.Indexed != 6 || st.State != "complete" {
		t.Errorf("final status: %+v", st)
	}
}

func TestBackgroundIndexAndCancel(t *testing.T) {
	e := setup(t)
	dir := t.TempDir()
	books(t, dir)
	id := e.folder(dir)
	var res IndexResult
	e.must("search.index", map[string]any{}, &res)
	if len(res.Jobs) != 1 || res.Jobs[0].JobID == "" {
		t.Fatalf("start: %+v", res)
	}
	deadline := time.Now().Add(10 * time.Second)
	for e.status(id).Running {
		if time.Now().After(deadline) {
			t.Fatal("indexing did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st := e.status(id); st.Indexed != 6 || st.Job == nil || st.Job.JobID != res.Jobs[0].JobID {
		t.Errorf("status: %+v", st)
	}
	e.must("search.cancel", map[string]any{}, &res)
	if len(res.Jobs) != 1 || res.Jobs[0].State != "complete" {
		t.Errorf("cancel after the end: %+v", res)
	}
}

func TestMergedSearchWithOfflineFolder(t *testing.T) {
	e := setup(t)
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	writeFile(t, filepath.Join(a, "one.txt"), []byte("Title: Alpha\n\nGrace and peace to you."))
	writeFile(t, filepath.Join(b, "two.txt"), []byte("Title: Beta\n\nGrace, grace, grace upon grace."))
	writeFile(t, filepath.Join(b, "three.md"), []byte("# Gamma\n\nNothing to see."))
	idA, idB := e.folder(a), e.folder(b)
	var res IndexResult
	e.must("search.index", map[string]any{"wait": true}, &res)
	if len(res.Jobs) != 2 {
		t.Fatalf("jobs %+v", res)
	}
	if err := os.Rename(b, filepath.Join(root, "unplugged")); err != nil {
		t.Fatal(err)
	}
	r := e.query(map[string]any{"query": "grace"})
	if titles(r.Hits) != "Beta|Alpha" {
		t.Fatalf("merged: %+v", r.Hits)
	}
	if r.Hits[0].FolderID != idB || r.Hits[0].Online || r.Hits[1].FolderID != idA || !r.Hits[1].Online {
		t.Errorf("online flags: %+v", r.Hits)
	}
	if r := e.query(map[string]any{"query": "grace", "folder_ids": []string{idA}}); titles(r.Hits) != "Alpha" {
		t.Errorf("folder filter: %s", titles(r.Hits))
	}
	e.must("search.index", map[string]any{"wait": true}, &res)
	for _, j := range res.Jobs {
		if j.FolderID == idB && j.State != "skipped" {
			t.Errorf("offline folder indexed: %+v", j)
		}
	}
	err := e.run(context.Background(), "search.rebuild", map[string]any{"folder_id": idB}, nil)
	var ce *command.Error
	if !errors.As(err, &ce) || ce.Code != "unavailable" {
		t.Errorf("rebuild offline: %v", err)
	}
	if st := e.status(idB); st.Online || st.Indexed != 2 {
		t.Errorf("offline status: %+v", st)
	}
}

func TestRefs(t *testing.T) {
	e := setup(t)
	dir := t.TempDir()
	books(t, dir)
	writeFile(t, filepath.Join(dir, "chapter.txt"), []byte("Title: Chapter Study\n\nAll of Romans 8:1-39 is about life in the Spirit."))
	id := e.folder(dir)
	e.index(id)
	var res QueryResult
	e.must("search.refs", map[string]any{"ref": "Romans 8:28"}, &res)
	if titles(res.Hits) != "Lectures on Romans|Chapter Study" {
		t.Fatalf("refs: %+v", res.Hits)
	}
	h := res.Hits[0]
	if h.Cites != "Romans 8:28-30" || h.Locator.Spine != 2 || !strings.Contains(h.Snippet, "<mark>Romans 8:28–30</mark>") {
		t.Errorf("hit %+v", h)
	}
	e.must("search.refs", map[string]any{"ref": "John 4"}, &res)
	if titles(res.Hits) != "Sermon on Hope" || res.Hits[0].Cites != "John 4:2" {
		t.Errorf("John 4: %+v", res.Hits)
	}
	e.must("search.refs", map[string]any{"ref": "1 Cor 13"}, &res)
	if titles(res.Hits) != "commentary" || res.Hits[0].Locator.Page != 2 {
		t.Errorf("1 Cor 13: %+v", res.Hits)
	}
	e.must("search.refs", map[string]any{"ref": "Ps 23:4", "author": "doe"}, &res)
	if titles(res.Hits) != "Sermon on Hope" {
		t.Errorf("Ps 23:4: %+v", res.Hits)
	}
	e.must("search.refs", map[string]any{"ref": "Gen 1:1"}, &res)
	if len(res.Hits) != 0 {
		t.Errorf("Gen 1:1: %+v", res.Hits)
	}
}

func TestRebuild(t *testing.T) {
	e := setup(t)
	dir := t.TempDir()
	books(t, dir)
	id := e.folder(dir)
	e.index(id)
	before := e.status(id)
	var p Progress
	e.must("search.rebuild", map[string]any{"folder_id": id, "wait": true}, &p)
	if p.State != "complete" || p.Indexed != 6 || p.Unchanged != 0 {
		t.Errorf("rebuild: %+v", p)
	}
	if after := e.status(id); after.Passages != before.Passages || after.Indexed != 6 {
		t.Errorf("after rebuild %+v, before %+v", after, before)
	}
	if r := e.query(map[string]any{"query": "grace"}); len(r.Hits) == 0 {
		t.Error("nothing found after rebuild")
	}
}

func TestCollectionFilter(t *testing.T) {
	e := setup(t)
	dir := t.TempDir()
	books(t, dir)
	e.index(e.folder(dir))

	all := e.query(map[string]any{"query": "pilgrim mercy", "mode": "any"})
	if len(all.Hits) != 2 {
		t.Fatalf("unfiltered: %s", titles(all.Hits))
	}
	pick := all.Hits[0].ItemID
	var c library.CollectionResult
	e.must("collection.create", map[string]any{"name": "Mine", "item_ids": []string{pick}}, &c)
	res := e.query(map[string]any{"query": "pilgrim mercy", "mode": "any", "collection_id": c.ID})
	if len(res.Hits) != 1 || res.Hits[0].ItemID != pick {
		t.Errorf("hand-picked collection: %s", titles(res.Hits))
	}

	e.must("collection.create", map[string]any{"name": "Bunyan", "rule": map[string]any{"author": "bunyan"}}, &c)
	if r := e.query(map[string]any{"query": "pilgrim mercy", "mode": "any", "collection_id": c.ID}); titles(r.Hits) != "Study Notes" {
		t.Errorf("rule collection: %s", titles(r.Hits))
	}
	if r := e.query(map[string]any{"query": "pilgrim mercy", "mode": "any", "collection_id": "b:author:John Bunyan"}); titles(r.Hits) != "Study Notes" {
		t.Errorf("built-in grouping: %s", titles(r.Hits))
	}
	e.must("collection.create", map[string]any{"name": "Empty"}, &c)
	if r := e.query(map[string]any{"query": "pilgrim mercy", "mode": "any", "collection_id": c.ID}); len(r.Hits) != 0 {
		t.Errorf("empty collection: %s", titles(r.Hits))
	}
	var out QueryResult
	if err := e.run(context.Background(), "search.query", map[string]any{"query": "grace", "collection_id": "nope"}, &out); err == nil {
		t.Error("an unknown collection was accepted")
	}
}

// Closing the registry (as the app does at shutdown) closes the indexes that are open.
func TestRegistryCloseClosesIndexes(t *testing.T) {
	e := setup(t)
	if _, err := e.svc.index("f1", true); err != nil {
		t.Fatal(err)
	}
	e.reg.Close()
	e.svc.mu.Lock()
	defer e.svc.mu.Unlock()
	if len(e.svc.indexes) != 0 {
		t.Fatalf("%d indexes still open", len(e.svc.indexes))
	}
}
