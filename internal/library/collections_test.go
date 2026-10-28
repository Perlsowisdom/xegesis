package library

import (
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

type seedBook struct {
	title, author, typ, tradition, lang string
	year                                int
}

// seed adds books straight to the catalogue and returns their IDs in order.
func seed(t *testing.T, env *command.Env, books []seedBook) []string {
	t.Helper()
	var ids []string
	people := map[string]string{}
	err := env.Store.Write(t.Context(), "test.seed", func(tx *store.Tx) error {
		for _, b := range books {
			id := store.NewID()
			ids = append(ids, id)
			row := store.Row{"id": id, "kind": "book", "title": b.title, "source": "import", "created_at": now(), "updated_at": now()}
			if b.year > 0 {
				row["year"] = b.year
			}
			if b.lang != "" {
				row["language"] = b.lang
			}
			if err := tx.Create("items", row); err != nil {
				return err
			}
			if b.author != "" {
				pid, ok := people[b.author]
				if !ok {
					pid = store.NewID()
					people[b.author] = pid
					if err := tx.Create("people", store.Row{"id": pid, "name": b.author, "sort_name": b.author}); err != nil {
						return err
					}
				}
				if err := tx.Create("item_people", store.Row{"item_id": id, "person_id": pid, "role": "author"}); err != nil {
					return err
				}
			}
			for facet, v := range map[string]string{facetType: b.typ, facetTradition: b.tradition} {
				if v != "" {
					if err := tx.Create("item_facets", store.Row{"item_id": id, "facet": facet, "value": v, "origin": "user"}); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

var library5 = []seedBook{
	{"Commentary on Romans", "John Calvin", TypeCommentaryN, "Reformed", "en", 1540},
	{"Commentary on Genesis", "John Calvin", TypeCommentaryO, "Reformed", "en", 1554},
	{"Notes on Hebrews", "Matthew Henry", TypeCommentaryN, "Presbyterian", "en", 1708},
	{"The Holy Bible", "", TypeBible, "", "en-GB", 1611},
	{"Hope in Hard Times", "Someone Else", "", "", "fr", 0},
}

func find(nodes []*Node, id string) *Node {
	for _, n := range nodes {
		if n.ID == id {
			return n
		}
		if f := find(n.Children, id); f != nil {
			return f
		}
	}
	return nil
}

func listTree(t *testing.T, r *command.Registry, env *command.Env, authors bool) []*Node {
	t.Helper()
	var res ListResult
	must(t, r, env, "collection.list", obj{"authors": authors}, &res)
	return res.Tree
}

func wantCount(t *testing.T, tree []*Node, id string, want int) {
	t.Helper()
	n := find(tree, id)
	if n == nil {
		t.Fatalf("no entry %s", id)
	}
	if n.Count != want {
		t.Errorf("%s has %d books, want %d", id, n.Count, want)
	}
}

func TestBuiltinGroupingsAppearByThemselves(t *testing.T) {
	r, env := setup(t)
	if tree := listTree(t, r, env, false); find(tree, builtinAll).Count != 0 {
		t.Fatal("an empty library has books")
	}
	seed(t, env, library5)
	tree := listTree(t, r, env, true)
	wantCount(t, tree, builtinAll, 5)
	wantCount(t, tree, "b:type", 4)
	wantCount(t, tree, "b:type:"+TypeCommentaryN, 2)
	wantCount(t, tree, "b:type:"+TypeBible, 1)
	wantCount(t, tree, "b:type:none", 1)
	wantCount(t, tree, "b:tradition:Reformed", 2)
	wantCount(t, tree, "b:tradition:none", 2)
	wantCount(t, tree, "b:author", 4)
	wantCount(t, tree, "b:author:John Calvin", 2)
	wantCount(t, tree, "b:year", 4)
	wantCount(t, tree, "b:year:c16", 2)
	wantCount(t, tree, "b:year:c17", 1)
	wantCount(t, tree, "b:year:c18", 1)
	wantCount(t, tree, "b:year:d1540", 1)
	wantCount(t, tree, "b:year:d1550", 1)
	if find(tree, "b:type:"+TypeDictionary) != nil {
		t.Error("a type with no books is listed")
	}
	if c := find(tree, "b:year:c16"); c.Name != "16th century" || len(c.Children) != 2 {
		t.Errorf("century = %+v", c)
	}
	// Authors are only listed when asked for.
	if a := find(listTree(t, r, env, false), "b:author"); len(a.Children) != 0 || !a.HasChildren {
		t.Errorf("author group = %+v", a)
	}
}

func TestBuiltinGroupingsFilterTheBookList(t *testing.T) {
	r, env := setup(t)
	seed(t, env, library5)
	cases := map[string]int{
		builtinAll: 5, "b:type": 4, "b:type:" + TypeCommentaryN: 2, "b:type:none": 1, "b:tradition:Reformed": 2,
		"b:tradition": 3, "b:author:John Calvin": 2, "b:author": 4, "b:year:c16": 2, "b:year:c17": 1, "b:year:d1540": 1, "b:year": 4,
	}
	for id, want := range cases {
		var res ItemsResult
		must(t, r, env, "collection.items", obj{"collection_id": id}, &res)
		var viaItems ItemsResult
		must(t, r, env, "library.items", obj{"collection_id": id}, &viaItems)
		if res.Total != want || len(res.Items) != want || viaItems.Total != want {
			t.Errorf("%s: %d books (%d via library.items), want %d", id, res.Total, viaItems.Total, want)
		}
	}
	if err := run(t, r, env, "collection.items", obj{"collection_id": "b:nonsense"}, nil); code(err) != "not_found" {
		t.Errorf("unknown grouping: %v", err)
	}
	if err := run(t, r, env, "library.items", obj{"collection_id": "nope"}, nil); code(err) != "not_found" {
		t.Errorf("unknown collection: %v", err)
	}
	if got := countOfFilter(t, r, env, obj{"language": "en"}); got != 4 {
		t.Errorf("language en: %d, want 4 (en and en-GB)", got)
	}
	if got := countOfFilter(t, r, env, obj{"year_from": 1550, "year_to": 1650}); got != 2 {
		t.Errorf("years 1550 to 1650: %d, want 2", got)
	}
}

func countOfFilter(t *testing.T, r *command.Registry, env *command.Env, p obj) int {
	t.Helper()
	var res ItemsResult
	must(t, r, env, "library.items", p, &res)
	return res.Total
}

func TestHandPickedCollection(t *testing.T) {
	r, env := setup(t)
	ids := seed(t, env, library5)
	var c CollectionResult
	must(t, r, env, "collection.create", obj{"name": "  For Sunday  ", "item_ids": ids[:2]}, &c)
	if c.Kind != "manual" || c.Name != "For Sunday" || c.Count != 2 {
		t.Fatalf("created %+v", c)
	}
	var sub CollectionResult
	must(t, r, env, "collection.create", obj{"name": "Romans", "parent_id": c.ID, "item_ids": []string{ids[0]}}, &sub)

	var m MembersResult
	must(t, r, env, "collection.add", obj{"collection_id": c.ID, "item_ids": []string{ids[1], ids[2]}}, &m)
	if m.Changed != 1 || m.Count != 3 {
		t.Errorf("add = %+v (one was already there)", m)
	}
	must(t, r, env, "collection.remove", obj{"collection_id": c.ID, "item_ids": []string{ids[0], ids[4]}}, &m)
	if m.Changed != 1 || m.Count != 2 {
		t.Errorf("remove = %+v", m)
	}
	if err := run(t, r, env, "collection.add", obj{"collection_id": c.ID, "item_ids": []string{"missing"}}, nil); code(err) != "not_found" {
		t.Errorf("add a missing book: %v", err)
	}

	tree := listTree(t, r, env, false)
	wantCount(t, tree, c.ID, 2)
	wantCount(t, tree, sub.ID, 1)
	if n := find(tree, c.ID); len(n.Children) != 1 || n.Children[0].ID != sub.ID || n.Children[0].ParentID != c.ID {
		t.Errorf("not nested: %+v", n)
	}
	var items ItemsResult
	must(t, r, env, "collection.items", obj{"collection_id": c.ID}, &items)
	if items.Total != 2 || items.Items[0].Title != "Commentary on Genesis" {
		t.Errorf("items = %+v", items)
	}

	must(t, r, env, "collection.update", obj{"id": c.ID, "name": "Sunday"}, &c)
	if c.Name != "Sunday" {
		t.Errorf("rename: %+v", c)
	}
	if err := run(t, r, env, "collection.update", obj{"id": c.ID, "parent_id": sub.ID}, nil); code(err) != "bad_params" {
		t.Errorf("a collection inside its own child: %v", err)
	}
	moved := CollectionResult{}
	must(t, r, env, "collection.update", obj{"id": sub.ID, "parent_id": ""}, &moved)
	if moved.ParentID != "" {
		t.Errorf("not moved to the top: %+v", moved)
	}
	if err := run(t, r, env, "collection.update", obj{"id": c.ID, "rule": obj{"type": TypeBible}}, nil); code(err) != "bad_params" {
		t.Errorf("rule on a hand-picked collection: %v", err)
	}
}

func TestRuleCollection(t *testing.T) {
	r, env := setup(t)
	seed(t, env, library5)
	rule := obj{"type": TypeCommentaryN, "tradition": "Reformed"}
	var pre PreviewResult
	must(t, r, env, "collection.preview", obj{"rule": rule}, &pre)
	if pre.Count != 1 || len(pre.Examples) != 1 || pre.Examples[0].Title != "Commentary on Romans" || pre.Examples[0].Authors[0] != "John Calvin" {
		t.Fatalf("preview = %+v", pre)
	}
	var c CollectionResult
	must(t, r, env, "collection.create", obj{"name": "Reformed NT", "rule": rule}, &c)
	if c.Kind != "rule" || c.Count != 1 || c.Rule.Version != 1 {
		t.Fatalf("created %+v", c)
	}
	var raw string
	if err := env.Store.QueryRow(t.Context(), `SELECT rule_json FROM collections WHERE id = ?`, c.ID).Scan(&raw); err != nil ||
		!strings.Contains(raw, `"version":1`) || !strings.Contains(raw, `"tradition":"Reformed"`) {
		t.Errorf("stored rule %q (%v)", raw, err)
	}
	// A rule follows the books: change a book's tradition and the count follows.
	var items ItemsResult
	must(t, r, env, "collection.items", obj{"collection_id": c.ID}, &items)
	must(t, r, env, "facet.set", obj{"item_ids": []string{items.Items[0].ID}, "tradition": "Baptist"}, nil)
	wantCount(t, listTree(t, r, env, false), c.ID, 0)

	must(t, r, env, "collection.update", obj{"id": c.ID, "rule": obj{"type": TypeCommentaryN, "year_from": 1700, "year_to": 1800}}, &c)
	if c.Count != 1 || c.Rule.YearFrom != 1700 {
		t.Errorf("new rule: %+v", c)
	}
	must(t, r, env, "collection.items", obj{"collection_id": c.ID}, &items)
	if items.Items[0].Title != "Notes on Hebrews" {
		t.Errorf("items = %+v", items.Items)
	}
	if err := run(t, r, env, "collection.add", obj{"collection_id": c.ID, "item_ids": []string{items.Items[0].ID}}, nil); code(err) != "bad_params" {
		t.Errorf("adding to a rule collection: %v", err)
	}
	for name, bad := range map[string]obj{
		"no condition": {}, "unknown type": {"type": "Poetry"}, "years back to front": {"year_from": 1900, "year_to": 1800},
		"newer version": {"version": 2, "type": TypeBible}, "unknown licence": {"licence": "free"},
	} {
		if err := run(t, r, env, "collection.preview", obj{"rule": bad}, nil); code(err) != "bad_params" {
			t.Errorf("%s: %v", name, err)
		}
	}
	must(t, r, env, "collection.preview", obj{"rule": obj{"author": "calvin", "text": "genesis"}}, &pre)
	if pre.Count != 1 {
		t.Errorf("author and text: %+v", pre)
	}
	must(t, r, env, "collection.preview", obj{"rule": obj{"type": "none"}}, &pre)
	if pre.Count != 1 {
		t.Errorf("no type: %+v", pre)
	}
}

func TestDeleteRestoreAndReorder(t *testing.T) {
	r, env := setup(t)
	ids := seed(t, env, library5)
	var a, b, c, sub CollectionResult
	must(t, r, env, "collection.create", obj{"name": "A", "item_ids": ids[:1]}, &a)
	must(t, r, env, "collection.create", obj{"name": "B"}, &b)
	must(t, r, env, "collection.create", obj{"name": "C"}, &c)
	must(t, r, env, "collection.create", obj{"name": "Inside A", "parent_id": a.ID}, &sub)
	order := func() string {
		var names []string
		for _, n := range listTree(t, r, env, false) {
			if n.Kind != "builtin" {
				names = append(names, n.Name)
			}
		}
		return strings.Join(names, ",")
	}
	if order() != "A,B,C" {
		t.Fatalf("order %s", order())
	}
	must(t, r, env, "collection.update", obj{"id": c.ID, "move": "up"}, nil)
	if order() != "A,C,B" {
		t.Errorf("order after moving C up: %s", order())
	}
	must(t, r, env, "collection.update", obj{"id": a.ID, "move": "down"}, nil)
	if order() != "C,A,B" {
		t.Errorf("order after moving A down: %s", order())
	}
	must(t, r, env, "collection.update", obj{"id": c.ID, "move": "up"}, nil) // already first
	if order() != "C,A,B" {
		t.Errorf("order after moving the first up: %s", order())
	}

	var d DeleteResult
	must(t, r, env, "collection.delete", obj{"id": a.ID}, &d)
	if d.Deleted != 2 {
		t.Errorf("deleted %d, want 2 (it and the one inside)", d.Deleted)
	}
	tree := listTree(t, r, env, false)
	if find(tree, a.ID) != nil || find(tree, sub.ID) != nil {
		t.Error("deleted collections still listed")
	}
	if err := run(t, r, env, "collection.items", obj{"collection_id": a.ID}, nil); code(err) != "not_found" {
		t.Errorf("items of a deleted collection: %v", err)
	}

	// Restore from history: back to how it was just after it was created.
	hist, err := env.Store.History(t.Context(), "collections", a.ID)
	if err != nil || len(hist) < 2 || hist[len(hist)-1].Op != "delete" {
		t.Fatalf("history of the deleted collection: %+v (%v)", hist, err)
	}
	if err := env.Store.Restore(t.Context(), "history.restore", "collections", store.ByID(a.ID), hist[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := env.Store.Restore(t.Context(), "history.restore", "collections", store.ByID(sub.ID), mustFirst(t, env, sub.ID)); err != nil {
		t.Fatal(err)
	}
	tree = listTree(t, r, env, false)
	wantCount(t, tree, a.ID, 1)
	if n := find(tree, a.ID); len(n.Children) != 1 || n.Children[0].ID != sub.ID {
		t.Errorf("restored collection: %+v", n)
	}
}

func mustFirst(t *testing.T, env *command.Env, id string) string {
	t.Helper()
	h, err := env.Store.History(t.Context(), "collections", id)
	if err != nil || len(h) == 0 {
		t.Fatalf("no history for %s (%v)", id, err)
	}
	return h[0].ID
}

func TestCollectionNeedsAName(t *testing.T) {
	r, env := setup(t)
	if err := run(t, r, env, "collection.create", obj{"name": "  "}, nil); code(err) != "bad_params" {
		t.Errorf("blank name: %v", err)
	}
	if err := run(t, r, env, "collection.create", obj{"name": "x", "parent_id": "nope"}, nil); code(err) != "not_found" {
		t.Errorf("missing parent: %v", err)
	}
	if err := run(t, r, env, "collection.create", obj{"name": "x", "rule": obj{"type": TypeBible}, "item_ids": []string{"a"}}, nil); code(err) != "bad_params" {
		t.Errorf("both kinds: %v", err)
	}
}
