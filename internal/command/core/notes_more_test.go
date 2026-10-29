package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/store"
)

func TestNoteTagsSearchMoveAndRecentlyDeleted(t *testing.T) {
	r, env := setup(t)
	mk := func(params string) Note {
		var n Note
		if err := run(t, r, env, "note.create", params, &n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	a := mk(`{"title":"Grace","body":"Saved by grace alone","ref":"Eph 2:8","tags":["Grace "," salvation","grace"]}`)
	b := mk(`{"title":"Hope","body":"100% sure_thing","ref":"Rom 8:28","tags":["hope"]}`)
	c := mk(`{"body":"Loose thought","tags":["salvation"]}`)
	if strings.Join(a.Tags, ",") != "grace,salvation" {
		t.Fatalf("tags = %v", a.Tags)
	}

	search := func(params string) []string {
		var got []Note
		if err := run(t, r, env, "note.search", params, &got); err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, n := range got {
			ids = append(ids, n.ID)
		}
		return ids
	}
	if got := search(`{"query":"GRACE"}`); len(got) != 1 || got[0] != a.ID {
		t.Errorf("query grace = %v", got)
	}
	if got := search(`{"query":"100%"}`); len(got) != 1 || got[0] != b.ID { // % is not a wildcard
		t.Errorf("query 100%% = %v", got)
	}
	if got := search(`{"query":"sure_t"}`); len(got) != 1 {
		t.Errorf("query sure_t = %v", got)
	}
	if got := search(`{"query":"sure xyz"}`); len(got) != 0 {
		t.Errorf("every word must match: %v", got)
	}
	if got := search(`{"tag":"salvation"}`); len(got) != 2 {
		t.Errorf("tag salvation = %v", got)
	}
	if got := search(`{"book":"Romans"}`); len(got) != 1 || got[0] != b.ID {
		t.Errorf("book Romans = %v", got)
	}
	if got := search(`{"on":"nothing"}`); len(got) != 1 || got[0] != c.ID {
		t.Errorf("free notes = %v", got)
	}
	if err := run(t, r, env, "note.search", `{"on":"sideways"}`, nil); code(err) != "bad_params" {
		t.Errorf("bad on: %v", err)
	}

	var tags []TagCount
	run(t, r, env, "note.tags", `{}`, &tags)
	if len(tags) != 3 || tags[0].Tag != "salvation" || tags[0].Count != 2 {
		t.Errorf("tags = %+v", tags)
	}
	run(t, r, env, "note.tags", `{"prefix":"ho"}`, &tags)
	if len(tags) != 1 || tags[0].Tag != "hope" {
		t.Errorf("tags by prefix = %+v", tags)
	}

	var moved Note
	// Edit the tags, then move the free note onto a verse and a verse note off its verse.
	if err := run(t, r, env, "note.update", fmt.Sprintf(`{"id":%q,"tags":["mercy","grace"]}`, a.ID), &a); err != nil || strings.Join(a.Tags, ",") != "grace,mercy" {
		t.Fatalf("update tags = %v, %v", a.Tags, err)
	}
	if err := run(t, r, env, "note.move", fmt.Sprintf(`{"id":%q,"ref":"John 3:16"}`, c.ID), &c); err != nil || c.Ref == nil || c.Ref.Start != 43003016 {
		t.Fatalf("move onto verse = %+v, %v", c.Ref, err)
	}
	if err := run(t, r, env, "note.move", fmt.Sprintf(`{"id":%q,"free":true}`, b.ID), &moved); err != nil || moved.Ref != nil {
		t.Fatalf("detach = %+v, %v", moved.Ref, err)
	}
	if err := run(t, r, env, "note.move", fmt.Sprintf(`{"id":%q}`, b.ID), nil); code(err) != "bad_params" {
		t.Errorf("move with nowhere to go: %v", err)
	}

	// Delete, find under Recently deleted, restore.
	if err := run(t, r, env, "note.delete", fmt.Sprintf(`{"id":%q}`, a.ID), nil); err != nil {
		t.Fatal(err)
	}
	var gone []Note
	run(t, r, env, "note.deleted", `{}`, &gone)
	if len(gone) != 1 || gone[0].ID != a.ID || gone[0].DeletedAt == 0 || len(gone[0].Tags) != 2 {
		t.Fatalf("deleted = %+v", gone)
	}
	if got := search(`{"query":"grace"}`); len(got) != 0 {
		t.Errorf("a deleted note is found: %v", got)
	}
	if err := run(t, r, env, "note.restore", fmt.Sprintf(`{"id":%q}`, a.ID), &a); err != nil || a.Body != "Saved by grace alone" {
		t.Fatalf("restore = %+v, %v", a, err)
	}
	if err := run(t, r, env, "note.restore", fmt.Sprintf(`{"id":%q}`, a.ID), nil); code(err) != "bad_params" {
		t.Errorf("restoring a live note: %v", err)
	}
	run(t, r, env, "note.deleted", `{}`, &gone)
	if len(gone) != 0 {
		t.Errorf("deleted after restore = %d", len(gone))
	}
}

// A restore from history is itself recorded, so it can be undone.
func TestRestoringAVersionCanBeUndone(t *testing.T) {
	r, env := setup(t)
	var n Note
	run(t, r, env, "note.create", `{"body":"one"}`, &n)
	run(t, r, env, "note.update", fmt.Sprintf(`{"id":%q,"body":"two"}`, n.ID), &n)
	var h []store.Change
	run(t, r, env, "history.list", fmt.Sprintf(`{"object_type":"notes","object_id":%q}`, n.ID), &h)
	restore := func(id string) {
		t.Helper()
		if err := run(t, r, env, "history.restore", fmt.Sprintf(`{"object_type":"notes","object_id":%q,"change_id":%q}`, n.ID, id), nil); err != nil {
			t.Fatal(err)
		}
		run(t, r, env, "note.get", fmt.Sprintf(`{"id":%q}`, n.ID), &n)
	}
	restore(h[0].ID)
	if n.Body != "one" {
		t.Fatalf("body = %q", n.Body)
	}
	run(t, r, env, "history.list", fmt.Sprintf(`{"object_type":"notes","object_id":%q}`, n.ID), &h)
	// The change before the restore's own changes brings "two" back.
	for i := len(h) - 1; i >= 0; i-- {
		if h[i].Command == "note.update" && h[i].Field == "updated_at" {
			restore(h[i].ID)
			break
		}
	}
	if n.Body != "two" {
		t.Fatalf("after undoing the restore, body = %q", n.Body)
	}
}

func TestNoteSources(t *testing.T) {
	r, env := setup(t)
	for _, p := range []string{`{"body":"a","ref":"Rom 1:1"}`, `{"body":"b","ref":"Rom 2:1"}`, `{"body":"c","ref":"Gen 1:1"}`, `{"body":"d"}`} {
		run(t, r, env, "note.create", p, nil)
	}
	var n Note
	run(t, r, env, "note.create", `{"body":"gone"}`, &n)
	run(t, r, env, "note.delete", `{"id":"`+n.ID+`"}`, nil)
	var s NoteSources
	if err := run(t, r, env, "note.sources", `{}`, &s); err != nil {
		t.Fatal(err)
	}
	if len(s.BibleBooks) != 2 || s.BibleBooks[0].Name != "Genesis" || s.BibleBooks[1].Count != 2 || s.Free != 1 || s.Deleted != 1 || len(s.LibraryBooks) != 0 {
		t.Errorf("sources = %+v", s)
	}
}
