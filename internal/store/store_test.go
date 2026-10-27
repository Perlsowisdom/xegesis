package store

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"testing"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenMigratesAndKeepsDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.SchemaVersion()
	if err != nil || v != 2 {
		t.Fatalf("schema version = %d, %v; want 2", v, err)
	}
	dev := s.Device()
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Device() != dev {
		t.Fatalf("device changed on reopen: %s then %s", dev, s.Device())
	}
}

func TestIDsSortByTime(t *testing.T) {
	ids := make([]string, 50)
	for i := range ids {
		ids[i] = NewID()
		if len(ids[i]) != 26 {
			t.Fatalf("id %q has length %d", ids[i], len(ids[i]))
		}
	}
	if !sort.StringsAreSorted([]string{ids[0], NewIDAfter(t, ids[0])}) {
		t.Fatal("later id sorts first")
	}
}

func NewIDAfter(t *testing.T, prev string) string {
	t.Helper()
	for {
		if id := NewID(); id[:10] != prev[:10] {
			return id
		}
	}
}

func TestClockIsMonotonic(t *testing.T) {
	c := NewClock("dev")
	c.now = func() int64 { return 1000 }
	a, b := c.Now(), c.Now()
	if !(a < b) {
		t.Fatalf("%s not before %s", a, b)
	}
	if err := c.Observe("0000000005000.00003.other"); err != nil {
		t.Fatal(err)
	}
	if n := c.Now(); n != "0000000005000.00004.dev" {
		t.Fatalf("after observe got %s", n)
	}
}

func TestWriteLogsAndRestores(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	id := NewID()
	err := s.Write(ctx, "note.create", func(tx *Tx) error {
		return tx.Create("notes", Row{"id": id, "body_md": "first", "created_at": int64(1), "updated_at": int64(1)})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"second", "third", "third"} {
		err := s.Write(ctx, "note.update", func(tx *Tx) error {
			return tx.Update("notes", ByID(id), Row{"body_md": body})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	h, err := s.History(ctx, "notes", id)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 3 {
		t.Fatalf("history has %d changes, want 3 (unchanged values are not logged)", len(h))
	}
	if h[1].Before != "first" || h[1].After != "second" || h[1].Field != "body_md" {
		t.Fatalf("unexpected change %+v", h[1])
	}

	if err := s.Restore(ctx, "history.restore", "notes", ByID(id), h[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := body(t, s, id); got != "first" {
		t.Fatalf("after restore body = %q, want first", got)
	}

	if err := s.Write(ctx, "note.delete", func(tx *Tx) error { return tx.Delete("notes", ByID(id)) }); err != nil {
		t.Fatal(err)
	}
	var deleted any
	s.QueryRow(ctx, "SELECT deleted_at FROM notes WHERE id = ?", id).Scan(&deleted)
	if deleted == nil {
		t.Fatal("delete did not set deleted_at")
	}
	if err := s.Restore(ctx, "history.restore", "notes", ByID(id), h[2].ID); err != nil {
		t.Fatal(err)
	}
	s.QueryRow(ctx, "SELECT deleted_at FROM notes WHERE id = ?", id).Scan(&deleted)
	if deleted != nil || body(t, s, id) != "third" {
		t.Fatal("restore did not undelete the note")
	}
}

func TestCompositeKeysAndHardDelete(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	item, coll := NewID(), NewID()
	err := s.Write(ctx, "test.setup", func(tx *Tx) error {
		if err := tx.Create("items", Row{"id": item, "kind": "monograph", "title": "T", "source": "own",
			"created_at": int64(1), "updated_at": int64(1)}); err != nil {
			return err
		}
		if err := tx.Create("collections", Row{"id": coll, "name": "C", "kind": "manual",
			"created_at": int64(1), "updated_at": int64(1)}); err != nil {
			return err
		}
		return tx.Create("collection_items", Row{"collection_id": coll, "item_id": item})
	})
	if err != nil {
		t.Fatal(err)
	}
	key := Row{"collection_id": coll, "item_id": item}
	if err := s.Write(ctx, "collection.remove", func(tx *Tx) error { return tx.Delete("collection_items", key) }); err != nil {
		t.Fatal(err)
	}
	err = s.Write(ctx, "x", func(tx *Tx) error { _, err := tx.Get("collection_items", key); return err })
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("row still present: %v", err)
	}
	h, err := s.History(ctx, "collection_items", `["`+coll+`","`+item+`"]`)
	if err != nil || len(h) != 2 {
		t.Fatalf("history = %v, %v", h, err)
	}
	if err := s.Restore(ctx, "history.restore", "collection_items", key, h[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsUnknownColumnsAndTables(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	cases := map[string]func(*Tx) error{
		"column":  func(tx *Tx) error { return tx.Create("notes", Row{"id": "x", "evil; DROP": 1}) },
		"table":   func(tx *Tx) error { return tx.Create("nope", Row{"id": "x"}) },
		"changes": func(tx *Tx) error { return tx.Create("changes", Row{"id": "x"}) },
		"command": nil,
	}
	for name, fn := range cases {
		cmd := "test"
		if fn == nil {
			cmd, fn = "", func(*Tx) error { return nil }
		}
		if err := s.Write(ctx, cmd, fn); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func body(t *testing.T, s *Store, id string) string {
	t.Helper()
	var b string
	if err := s.QueryRow(context.Background(), "SELECT body_md FROM notes WHERE id = ?", id).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}
