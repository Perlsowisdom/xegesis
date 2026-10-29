package repos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/bible/sword/swordtest"
)

// addReferenceFiles serves a tiny SWORD commentary and Strong's dictionary, STEPBible files and
// an OpenBible.info archive, in the published layouts.
func addReferenceFiles(t *testing.T, f *fake) {
	t.Helper()
	com := t.TempDir()
	if err := swordtest.ZText(com, map[swordtest.Key]string{swordtest.K(43, 3, 16): "God's love for the world."}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"nt.bzs", "nt.bzv", "nt.bzz"} {
		data, _ := os.ReadFile(filepath.Join(com, n))
		f.files["/sword/raw/modules/comments/zcom/tinycom/"+n] = data
	}
	dict := t.TempDir()
	if err := swordtest.ZLD(dict, "dict", [][2]string{{"00000", "<entryFree>Introduction</entryFree>"},
		{"00026", `<entryFree n="26"><orth>ἀγάπη</orth> <def>love</def></entryFree>`}}); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".idx", ".dat", ".zdx", ".zdt"} {
		data, _ := os.ReadFile(filepath.Join(dict, "dict"+ext))
		f.files["/sword/raw/modules/lexdict/zld/dict/dict"+ext] = data
	}
	lexicon := "header\nG0026\tG0026 =\tG0026\tἀγάπη\tagapē\tG:N-F\tlove\t<b>ἀγάπη</b>, love\n"
	text := "Jhn.3.16#01=NKO\tἠγάπησεν (ēgapēsen)\tloved\tG0025=V-AAI-3S\tἀγαπάω=to love\n"
	for _, d := range datasets {
		for i, name := range d.files {
			body := ""
			switch {
			case d.id == "TBESG" || d.id == "TBESH":
				body = lexicon
			case d.id == "TAGNT" && i == 0:
				body = text
			case d.id == "cross-references":
				f.files["/ob/"+name] = zipOf(t, map[string]string{"cross_references.txt": "From Verse\tTo Verse\tVotes\nJohn.3.16\tRom.5.8\t10\n"})
				continue
			}
			f.files["/step/"+name] = []byte(body)
		}
	}
}

func TestInstallReferenceWorks(t *testing.T) {
	e := setup(t)
	e.enable()
	for _, tc := range []struct{ source, item, kind string }{
		{"crosswire", "TinyCom", bible.KindCommentary},
		{"crosswire", "Dict", bible.KindLexicon},
		{"stepbible", "TAGNT", bible.KindBible},
		{"stepbible", "TBESG", bible.KindLexicon},
		{"openbible", "cross-references", bible.KindCrossRef},
	} {
		var res GetResult
		e.must("bible.get", map[string]any{"source": tc.source, "item": tc.item, "accept_licence": true}, &res)
		if res.Module == nil || res.Module.Kind != tc.kind {
			t.Fatalf("%s: %+v", tc.item, res.Module)
		}
		if it := e.item(tc.source, tc.item); !it.Installed {
			t.Errorf("%s not marked installed", tc.item)
		}
	}
	var refs bible.ForPassageResult
	e.must("reference.for_passage", map[string]any{"ref": "John 3:16"}, &refs)
	if len(refs.Commentaries) != 1 || len(refs.CrossRefs) != 1 || refs.CrossRefs[0].To != "Rom.5.8" {
		t.Errorf("for passage: %+v", refs)
	}
	var entries []bible.LookupEntry
	e.must("lexicon.lookup", map[string]any{"strong": "G26"}, &entries)
	if len(entries) != 2 || entries[0].Lemma != "ἀγάπη" {
		t.Errorf("lookup: %+v", entries)
	}
	var study bible.WordStudyResult
	e.must("word.study", map[string]any{"strong": "G25"}, &study)
	if study.Occurrences != 1 || study.Bibles[0].Module != "tagnt" {
		t.Errorf("study: %+v", study)
	}
	tags := e.item("stepbible", "TAGNT")
	if tags.Licence.SPDX != "CC-BY-4.0" || !strings.Contains(tags.Licence.Attribution, "STEPBible") {
		t.Errorf("licence: %+v", tags.Licence)
	}
	if left, _ := filepath.Glob(filepath.Join(os.Getenv("TMP"), "xegesis-*")); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
}
