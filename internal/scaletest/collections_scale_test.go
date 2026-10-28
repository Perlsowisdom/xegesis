//go:build scale

package scaletest

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// TestCollectionsScale times collection counts and rule matching on a catalogue of
// XEGESIS_SCALE_BOOKS books (default 10000). Only catalogue rows are generated (books, authors,
// types, traditions, years, licences), not book files: counts never read files. Run with
//
//	go test -tags scale -run TestCollectionsScale -v ./internal/scaletest
func TestCollectionsScale(t *testing.T) {
	n := envInt("XEGESIS_SCALE_BOOKS", 10000)
	l := openLib(t, t.TempDir())
	var vocab library.VocabularyResult
	l.must("facet.vocabulary", nil, &vocab)
	r := rand.New(rand.NewPCG(5, 5))
	authors := make([]string, n/3)
	for i := range authors {
		authors[i] = fmt.Sprintf("%s %s", titleCase(words(r, 1)), titleCase(words(r, 1)))
	}
	ids := make([]string, n)
	start := time.Now()
	err := l.st.Write(t.Context(), "scale.seed", func(tx *store.Tx) error {
		pids := make([]string, len(authors))
		for i, a := range authors {
			pids[i] = store.NewID()
			if err := tx.Create("people", store.Row{"id": pids[i], "name": a, "sort_name": a}); err != nil {
				return err
			}
		}
		for i := range ids {
			ids[i] = store.NewID()
			year := 1500 + r.IntN(520)
			err := tx.Create("items", store.Row{"id": ids[i], "kind": "book", "title": titleCase(words(r, 2+r.IntN(3))),
				"language": []string{"en", "en", "en", "de", "la"}[r.IntN(5)], "year": year, "source": "import", "created_at": 1, "updated_at": 1})
			if err != nil {
				return err
			}
			if err := tx.Create("item_people", store.Row{"item_id": ids[i], "person_id": pids[r.IntN(len(pids))], "role": "author"}); err != nil {
				return err
			}
			facets := map[string]string{"type": vocab.Types[r.IntN(len(vocab.Types))], "tradition": vocab.Traditions[r.IntN(len(vocab.Traditions))]}
			for f, v := range facets {
				if err := tx.Create("item_facets", store.Row{"item_id": ids[i], "facet": f, "value": v, "origin": "user"}); err != nil {
					return err
				}
			}
			lic := []string{"public_domain", "creative_commons", "personal_purchase", "own_work"}[r.IntN(4)]
			if err := tx.Create("licences", store.Row{"id": store.NewID(), "item_id": ids[i], "type": lic, "updated_at": 1}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("seeded %d books, %d authors in %.1fs (not app work)", n, len(authors), time.Since(start).Seconds())

	var tree library.ListResult
	var bestWithin func(string, time.Duration, func())
	best := func(name string, f func()) { bestWithin(name, 200*time.Millisecond, f) }
	bestWithin = func(name string, limit time.Duration, f func()) {
		var ds []time.Duration
		for range 5 {
			s := time.Now()
			f()
			ds = append(ds, time.Since(s))
		}
		med, worst := median(ds)
		t.Logf("%-52s median %7.1f ms  worst %7.1f ms", name, med.Seconds()*1000, worst.Seconds()*1000)
		if med > limit {
			t.Errorf("%s: median %v is over %v", name, med, limit)
		}
	}
	best("collection.list, groupings only", func() { l.must("collection.list", nil, &tree) })
	bestWithin("collection.list, groupings with every author", 400*time.Millisecond, func() { l.must("collection.list", map[string]any{"authors": true}, &tree) })
	// A hand-picked collection of 500 books and ten rule collections, nested.
	var parent library.CollectionResult
	l.must("collection.create", map[string]any{"name": "Picked", "item_ids": ids[:min(500, n)]}, &parent)
	rules := []map[string]any{
		{"type": "Commentary (New Testament)", "tradition": "Reformed"}, {"author": "ber"}, {"text": "e"},
		{"year_from": 1600, "year_to": 1700}, {"language": "en", "licence": "public_domain"}, {"tradition": "Baptist"},
		{"type": "Bible"}, {"text": "the", "year_from": 1800}, {"author": "ton", "tradition": "Lutheran"}, {"language": "la"},
	}
	for i, rule := range rules {
		l.must("collection.create", map[string]any{"name": fmt.Sprintf("Rule %d", i+1), "parent_id": parent.ID, "rule": rule}, nil)
	}

	best("collection.list (groupings + 11 collections, counts)", func() { l.must("collection.list", nil, &tree) })
	bestWithin("collection.list with every author (3,333 of them)", 400*time.Millisecond, func() { l.must("collection.list", map[string]any{"authors": true}, &tree) })
	best("collection.preview (rule: type, tradition)", func() { l.must("collection.preview", map[string]any{"rule": rules[0]}, nil) })
	best("collection.preview (rule: text in title)", func() { l.must("collection.preview", map[string]any{"rule": rules[2]}, nil) })
	best("collection.preview (rule: language, licence)", func() { l.must("collection.preview", map[string]any{"rule": rules[4]}, nil) })
	for _, r := range []map[string]any{{"language": "en"}, {"licence": "public_domain"}, {"year_from": 1600, "year_to": 1700}, {"author": "ber"}, {"author": "son"}, {"tradition": "Baptist"}, {"text": "the"}} {
		r := r
		best(fmt.Sprintf("preview %v", r), func() { l.must("collection.preview", map[string]any{"rule": r}, nil) })
	}
	var items library.ItemsResult
	best("collection.items, rule collection (first 50)", func() {
		l.must("collection.items", map[string]any{"collection_id": tree.Tree[len(tree.Tree)-1].Children[2].ID, "limit": 50}, &items)
	})
	best("library.items in a built-in grouping (first 50)", func() {
		l.must("library.items", map[string]any{"collection_id": "b:type:Bible", "limit": 50}, &items)
	})
	_ = filepath.Join
	t.Logf("total books in the tree: %d", tree.Tree[0].Count)
}
