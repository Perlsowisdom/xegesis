package library

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyType(t *testing.T) {
	cases := []struct {
		name string
		in   ClassifyInput
		want string
		why  string
	}{
		{"nt commentary", ClassifyInput{Title: "Commentary on Romans"}, TypeCommentaryN, "commentary on Romans"},
		{"author is not a book", ClassifyInput{Title: "John Calvin's Commentary on Genesis"}, TypeCommentaryO, "Genesis"},
		{"book before the word", ClassifyInput{Title: "Philippians: A Commentary"}, TypeCommentaryN, "Philippians"},
		{"series says testament", ClassifyInput{Title: "The Gospel Accounts", Series: "Commentary on the New Testament"}, TypeCommentaryN, "series"},
		{"whole bible", ClassifyInput{Title: "One Volume Bible Commentary"}, TypeCommentaryW, "commentary"},
		{"lcsh commentary", ClassifyInput{Title: "Exegesis", Subjects: []string{"Bible. Romans -- Commentaries"}}, TypeCommentaryN, "Romans"},
		{"lcsh ot", ClassifyInput{Title: "Notes", Subjects: []string{"Bible. O.T. -- Commentaries"}}, TypeCommentaryO, "Old Testament"},
		{"bible text", ClassifyInput{Title: "The Holy Bible, English Standard Version"}, TypeBible, "Bible"},
		{"bible study is not a bible", ClassifyInput{Title: "Bible Study Guide for Small Groups"}, "", ""},
		{"dictionary", ClassifyInput{Title: "New Bible Dictionary"}, TypeDictionary, "dictionary"},
		{"lexicon", ClassifyInput{Title: "A Greek-English Lexicon"}, TypeLexicon, "lexicon"},
		{"sermons by subject", ClassifyInput{Title: "Spurgeon on Faith", Subjects: []string{"Sermons"}}, TypeSermons, "Sermons"},
		{"biography code", ClassifyInput{Title: "Augustine", Subjects: []string{"BIO018000"}}, TypeBiography, "BIO018000"},
		{"fiction", ClassifyInput{Title: "Pilgrim", Subjects: []string{"Christian fiction"}}, TypeFiction, "fiction"},
		{"theology folder", ClassifyInput{Title: "Institutes", Folders: []string{"Theology"}}, TypeTheology, "folder"},
		{"folder commentaries and a book", ClassifyInput{Title: "Notes on Hebrews", Folders: []string{"Commentaries"}}, TypeCommentaryN, "Hebrews"},
		{"church history", ClassifyInput{Title: "A History of the Church", Subjects: []string{"Church history"}}, TypeHistory, "Church history"},
		{"installed dictionary", ClassifyInput{Kind: "dictionary", Title: "Strong"}, TypeDictionary, ""},
		{"nothing", ClassifyInput{Title: "Hope in Hard Times"}, "", ""},
	}
	for _, c := range cases {
		got, ok := ClassifyType(c.in)
		if c.want == "" {
			if ok {
				t.Errorf("%s: got %+v, want no suggestion", c.name, got)
			}
			continue
		}
		if !ok || got.Value != c.want || !strings.Contains(got.Reason, c.why) || got.Confidence <= 0 || got.Confidence > 1 {
			t.Errorf("%s: got %+v, want %s with reason containing %q", c.name, got, c.want, c.why)
		}
	}
	if s, _ := ClassifyType(ClassifyInput{Title: "Notes", Subjects: []string{"FIC027000"}}); s.Origin != "bisac" {
		t.Errorf("subject code origin = %q", s.Origin)
	}
}

func TestScanSuggestsAndUserChoicesSurvive(t *testing.T) {
	r, env := setup(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Commentaries", "romans.txt"), "Title: Commentary on Romans\nAuthor: John Calvin\n\nx")
	writeFile(t, filepath.Join(dir, "genesis.txt"), "Title: Commentary on Genesis\nAuthor: Gordon Wenham\n\nx")
	writeFile(t, filepath.Join(dir, "hope.txt"), "Title: Hope in Hard Times\nAuthor: Someone\n\nx")
	f, res := addAndScan(t, r, env, dir)
	if res.Typed != 2 {
		t.Fatalf("typed = %d, want 2", res.Typed)
	}
	var list ItemsResult
	must(t, r, env, "library.items", obj{}, &list)
	by := map[string]ItemSummary{}
	for _, it := range list.Items {
		by[it.Title] = it
	}
	rom := by["Commentary on Romans"]
	if rom.Type != TypeCommentaryN || rom.TypeBy != "auto" || !strings.Contains(rom.TypeReason, "Romans") {
		t.Fatalf("romans: %+v", rom)
	}
	if by["Commentary on Genesis"].Type != TypeCommentaryO || by["Hope in Hard Times"].Type != "" {
		t.Fatalf("others: %+v", by)
	}

	// Filters: by type, by "not set", by who set it.
	var some ItemsResult
	must(t, r, env, "library.items", obj{"type": TypeCommentaryN}, &some)
	if some.Total != 1 {
		t.Errorf("type filter total = %d", some.Total)
	}
	must(t, r, env, "library.items", obj{"type": "none"}, &some)
	if some.Total != 1 || some.Items[0].Title != "Hope in Hard Times" {
		t.Errorf("none filter: %+v", some)
	}
	must(t, r, env, "library.items", obj{"set_by": "auto"}, &some)
	if some.Total != 2 {
		t.Errorf("auto filter total = %d", some.Total)
	}

	// A bulk correction by the user wins, now and after a rescan and a redo.
	var set SetResult
	must(t, r, env, "facet.set", obj{"item_ids": []string{rom.ID, by["Hope in Hard Times"].ID}, "type": TypeSermons, "tradition": "Moravian"}, &set)
	if set.Updated != 2 {
		t.Fatalf("updated = %d", set.Updated)
	}
	writeFile(t, filepath.Join(dir, "another.txt"), "Title: Commentary on Mark\n\nx")
	must(t, r, env, "library.scan", obj{"folder_id": f.ID}, nil)
	must(t, r, env, "facet.suggest", obj{"redo": true}, nil)
	must(t, r, env, "library.items", obj{"type": TypeSermons, "set_by": "user"}, &some)
	if some.Total != 2 || some.Items[0].Tradition != "Moravian" || some.Items[0].TraditionBy != "user" {
		t.Fatalf("user choice lost: %+v", some)
	}
	var voc VocabularyResult
	must(t, r, env, "facet.vocabulary", nil, &voc)
	if !contains(voc.Traditions, "Moravian") || !contains(voc.Traditions, "Reformed") || len(voc.Types) != 14 {
		t.Errorf("vocabulary: %+v", voc)
	}

	// Back to automatic: the guess returns.
	must(t, r, env, "facet.set", obj{"item_ids": []string{rom.ID}, "type": ""}, nil)
	var it Item
	must(t, r, env, "library.item", obj{"id": rom.ID}, &it)
	var got Facet
	for _, fc := range it.Facets {
		if fc.Facet == "type" {
			got = fc
		}
	}
	if got.Value != TypeCommentaryN || got.Origin != "metadata" || got.Reason == "" || got.Confidence == 0 {
		t.Errorf("after reset: %+v", got)
	}
	if code(run(t, r, env, "facet.set", obj{"item_ids": []string{rom.ID}, "type": "Nonsense"}, nil)) != "bad_params" {
		t.Error("bad type accepted")
	}
}

func TestTraditionRules(t *testing.T) {
	r, env := setup(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "calvin.txt"), "Title: Institutes\nAuthor: John Calvin\n\nx")
	writeFile(t, filepath.Join(dir, "Anglican Texts", "prayer.txt"), "Title: Prayer Book\nAuthor: Cranmer\n\nx")
	writeFile(t, filepath.Join(dir, "other.txt"), "Title: Other\nAuthor: Nobody\n\nx")
	f, _ := addAndScan(t, r, env, dir)

	var rr RulesResult
	must(t, r, env, "facet.rules", obj{"action": "add", "field": "author", "text": "calvin", "value": "Reformed"}, &rr)
	must(t, r, env, "facet.rules", obj{"action": "add", "field": "folder", "text": "anglican", "value": "Anglican"}, &rr)
	if rr.Applied != 2 || len(rr.Rules) != 2 {
		t.Fatalf("rules: %+v", rr)
	}
	var list ItemsResult
	must(t, r, env, "library.items", obj{"tradition": "Reformed"}, &list)
	if list.Total != 1 || list.Items[0].TraditionBy != "auto" || !strings.Contains(list.Items[0].TraditionReason, "calvin") {
		t.Fatalf("reformed: %+v", list)
	}
	must(t, r, env, "library.items", obj{"tradition": "none"}, &list)
	if list.Total != 1 {
		t.Fatalf("untagged: %+v", list)
	}

	// Rules apply on scan to new books.
	writeFile(t, filepath.Join(dir, "calvin2.txt"), "Title: Commentaries\nAuthor: Jean Calvin\n\nx")
	must(t, r, env, "library.scan", obj{"folder_id": f.ID}, nil)
	must(t, r, env, "library.items", obj{"tradition": "Reformed"}, &list)
	if list.Total != 2 {
		t.Fatalf("after scan: %+v", list)
	}

	// A user's own tradition beats a rule; removing a rule takes its tags back.
	must(t, r, env, "facet.set", obj{"item_ids": []string{list.Items[0].ID}, "tradition": "Baptist"}, nil)
	must(t, r, env, "facet.rules", obj{"action": "apply"}, nil)
	must(t, r, env, "library.items", obj{"tradition": "Baptist"}, &list)
	if list.Total != 1 || list.Items[0].TraditionBy != "user" {
		t.Fatalf("user tradition: %+v", list)
	}
	must(t, r, env, "facet.rules", obj{"action": "remove", "id": rr.Rules[0].ID}, &rr)
	must(t, r, env, "library.items", obj{"tradition": "Reformed"}, &list)
	if list.Total != 0 {
		t.Fatalf("removed rule kept tags: %+v", list)
	}
	if code(run(t, r, env, "facet.rules", obj{"action": "remove", "id": "nope"}, nil)) != "not_found" {
		t.Error("removing a missing rule")
	}
	if code(run(t, r, env, "facet.rules", obj{"action": "add", "field": "colour", "text": "x", "value": "y"}, nil)) != "bad_params" {
		t.Error("bad rule accepted")
	}

	var tr TraditionsResult
	must(t, r, env, "facet.traditions", obj{"action": "remove", "name": "Methodist"}, &tr)
	if contains(tr.Traditions, "Methodist") || !contains(tr.Traditions, "Anglican") {
		t.Errorf("traditions: %v", tr.Traditions)
	}
}
