package bible

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

// Tiny fixtures in the formats of STEPBible's TAGNT and TBESG (CC BY 4.0) and OpenBible.info's
// cross-references (CC BY 4.0), typed by hand with a few public-domain words and references.
const tagntSample = "TAGNT test\n" +
	"Word & Type\tGreek\tEnglish translation\tdStrongs = Grammar\n" +
	"Jhn.3.16#01=NKO\tοὕτως (houtōs)\tThus\tG3779=ADV\tοὕτω=thus\n" +
	"Jhn.3.16#02=NKO\tγὰρ (gar)\tfor\tG1063=CONJ\tγάρ=for\n" +
	"Jhn.3.16#03=NKO\tἠγάπησεν (ēgapēsen)\tloved\tG0025=V-AAI-3S\tἀγαπάω=to love\n" +
	"Jhn.3.16#04=K\tμόνον (monon)\tonly\tG3441=A-ASM\tμόνος=only\n" +
	"Jhn.3.16#05=NKO\tὁ (ho)\t<the>\tG3588=T-NSM\tὁ=the\n" +
	"Jhn.3.16#06=NKO\tθεὸς (theos)\tGod\tG2316G=N-NSM-T\tθεός=God\n"

const tahotSample = "Eng (Heb) Ref & Type\tHebrew\tTransliteration\tTranslation\tdStrongs\tGrammar\n" +
	"Gen.1.1#01=L\tבְּ/רֵאשִׁ֖ית\tbe./re.Shit\tin/ [the] beginning\tH9003/{H7225G}\tHR/Ncfsa\n" +
	"Gen.1.1#02=L\tבָּרָ֣א\tba.Ra'\the created\t{H1254A}\tHVqp3ms\n" +
	"Psa.3.0(3.1)#01=L\tמִזְמ֥וֹר\tmiz.Mor\ta psalm\t{H4210}\tHNcmsa\n"

const tbesgSample = "TBESG test header\n" +
	"eStrong#\tdStrong\tuStrong\tGreek\tTransliteration\tMorph\tGloss\tMeaning\n" +
	"G0025\tG0025 =\tG0025\tἀγαπάω\tagapaō\tG:V\tto love\t<b>ἀγαπάω</b>, to love: <ref='Jhn.3.16'>Jhn.3:16</ref><BR />__1. of men.\n" +
	"G0026\tG0026 =\tG0026\tἀγάπη\tagapē\tG:N-F\tlove\t<b>ἀγάπη</b>, love, <ref='Rom.5.8'>Rom.5:8</ref>\n"

const xrefSample = "From Verse\tTo Verse\tVotes\t#www.openbible.info CC-BY\n" +
	"John.3.16\tRom.5.8\t300\n" +
	"John.3.16\t1John.4.9-1John.4.10\t250\n" +
	"John.3.16\tGen.1.1\t-3\n" +
	"Gen.1.1\tJohn.1.1-John.1.3\t384\n"

type refEnv struct {
	*env
	lib *Library
}

func setupRef(t *testing.T) refEnv {
	t.Helper()
	e := &env{t: t, r: command.NewRegistry(), dir: filepath.Join(t.TempDir(), "modules"), src: t.TempDir()}
	lib := NewLibrary(e.dir)
	t.Cleanup(lib.Close)
	RegisterLibrary(e.r, lib)
	ctx := context.Background()
	if _, _, err := lib.InstallSTEP(ctx, InstallOptions{ID: "tagnt", Name: "Greek NT", Language: "grc"},
		[]io.Reader{strings.NewReader(tahotSample), strings.NewReader(tagntSample)}); err != nil {
		t.Fatal(err)
	}
	heads, err := ReadSTEPLexicon(strings.NewReader(tbesgSample), true)
	if err != nil || len(heads) != 2 {
		t.Fatalf("lexicon: %v %v", heads, err)
	}
	if _, err := lib.InstallHeadwords(ctx, InstallOptions{ID: "tbesg", Name: "Greek lexicon", Kind: KindLexicon}, heads); err != nil {
		t.Fatal(err)
	}
	dict := DictMarkup(`<entryFree n="Aaron"><orth>Aaron</orth><def>a teacher; lofty</def><lb/>See <ref osisRef="Exod.4.14">Ex 4:14</ref></entryFree>`, "TEI")
	dict.Key = "AARON"
	if _, err := lib.InstallHeadwords(ctx, InstallOptions{ID: "names", Name: "Bible names"}, []Headword{dict}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lib.InstallCrossRefs(ctx, InstallOptions{ID: "xref", Name: "Cross-references"}, strings.NewReader(xrefSample), 25); err != nil {
		t.Fatal(err)
	}
	com := []Entry{{ID: 43003016, Markup: `God's love is <reference osisRef="Rom.5.8">shown</reference> here.`},
		{ID: 43003017, Markup: "The Son was sent to save."}, {ID: 43003018, Markup: "The Son was sent to save."}}
	_, _, err = lib.InstallEntries(ctx, InstallOptions{ID: "notes", Name: "Short notes", Kind: KindCommentary}, "osis",
		func(emit func(Entry) error) error {
			for _, c := range com {
				if err := emit(c); err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	e.install(e.file("kjv.xml", osisKJV), map[string]any{"abbreviation": "KJV"})
	return refEnv{e, lib}
}

func TestSTEPTaggedText(t *testing.T) {
	e := setupRef(t)
	var p Passage
	e.must("bible.read", map[string]any{"module": "tagnt", "ref": "John 3:16", "markup": true}, &p)
	if len(p.Verses) != 1 || p.Verses[0].Text != "οὕτως γὰρ ἠγάπησεν ὁ θεὸς" {
		t.Fatalf("John 3:16 = %+v", p.Verses)
	}
	if m := p.Verses[0].Markup; !strings.Contains(m, `<w s="G25" m="V-AAI-3S" g="loved">ἠγάπησεν</w>`) || !strings.Contains(m, `s="G2316"`) {
		t.Errorf("markup %s", m)
	}
	e.must("bible.read", map[string]any{"module": "tagnt", "ref": "Gen 1:1", "markup": true}, &p)
	if p.Verses[0].Text != "בְּרֵאשִׁ֖ית בָּרָ֣א" || !strings.Contains(p.Verses[0].Markup, `s="H7225"`) {
		t.Errorf("Gen 1:1 = %+v", p.Verses[0])
	}
	e.must("bible.read", map[string]any{"module": "tagnt", "ref": "Ps 3:0"}, &p)
	if len(p.Verses) != 1 || p.Verses[0].ID != 19003000 {
		t.Errorf("psalm title: %+v", p.Verses)
	}
}

func TestModulesByKind(t *testing.T) {
	e := setupRef(t)
	var ms []Meta
	e.must("bible.modules", map[string]any{}, &ms)
	if len(ms) != 2 {
		t.Errorf("Bibles: %+v", ms)
	}
	e.must("bible.modules", map[string]any{"kind": "all"}, &ms)
	if len(ms) != 6 {
		t.Errorf("all modules: %d", len(ms))
	}
	e.must("bible.modules", map[string]any{"kind": "lexicon"}, &ms)
	if len(ms) != 1 || ms[0].ID != "tbesg" || ms[0].VerseCount != 2 {
		t.Errorf("lexicons: %+v", ms)
	}
	var hits []Hit
	e.must("bible.search", map[string]any{"query": "Son"}, &hits)
	for _, h := range hits {
		if h.Module == "notes" {
			t.Errorf("search found a commentary: %+v", h)
		}
	}
}

func TestForPassage(t *testing.T) {
	e := setupRef(t)
	var res ForPassageResult
	e.must("reference.for_passage", map[string]any{"ref": "John 3:16-18"}, &res)
	if len(res.Commentaries) != 1 || len(res.Commentaries[0].Entries) != 2 {
		t.Fatalf("commentaries: %+v", res.Commentaries)
	}
	c := res.Commentaries[0].Entries
	if !strings.Contains(c[0].Markup, `<r o="Rom.5.8">shown</r>`) || c[1].Ref != "John 3:17-18" {
		t.Errorf("entries: %+v", c)
	}
	// OpenBible: Rom 5:8 (votes 300) and 1 John 4:9-10; the KJV note also gives Rom 5:8.
	if len(res.CrossRefs) != 2 {
		t.Fatalf("cross-references: %+v", res.CrossRefs)
	}
	if r := res.CrossRefs[0]; r.To != "Rom.5.8" || r.FromRef != "John 3:16" || len(r.Sources) != 2 {
		t.Errorf("first cross-reference: %+v", r)
	}
	if r := res.CrossRefs[1]; r.ToRef != "1 John 4:9-10" {
		t.Errorf("range cross-reference: %+v", r)
	}
	e.must("reference.for_passage", map[string]any{"ref": "Rev 1"}, &res)
	if len(res.Commentaries) != 0 || len(res.CrossRefs) != 0 {
		t.Errorf("empty passage: %+v", res)
	}
}

func TestLexiconLookup(t *testing.T) {
	e := setupRef(t)
	var es []LookupEntry
	e.must("lexicon.lookup", map[string]any{"strong": "G0026"}, &es)
	if len(es) != 1 || es[0].Lemma != "ἀγάπη" || es[0].Gloss != "love" || !strings.Contains(es[0].Markup, `<r o="Rom.5.8">Rom.5:8</r>`) {
		t.Fatalf("G26: %+v", es)
	}
	if es[0].Attribution == "" && es[0].Module != "tbesg" {
		t.Errorf("entry: %+v", es[0])
	}
	e.must("lexicon.lookup", map[string]any{"word": "αγαπαω"}, &es)
	if len(es) != 1 || es[0].Strong != "G25" || !strings.Contains(es[0].Markup, "<p/>") {
		t.Errorf("by word: %+v", es)
	}
	e.must("lexicon.lookup", map[string]any{"word": "aaron"}, &es)
	if len(es) != 1 || es[0].Module != "names" || es[0].Lemma != "Aaron" || !strings.Contains(es[0].Markup, `<r o="Exod.4.14">`) {
		t.Errorf("dictionary: %+v", es)
	}
	if es[0].Kind != KindDictionary {
		t.Errorf("kind %q", es[0].Kind)
	}
	e.must("lexicon.lookup", map[string]any{"word": "aar"}, &es)
	if len(es) != 0 {
		t.Errorf("exact lookup found a prefix: %+v", es)
	}
	e.must("lexicon.lookup", map[string]any{"word": "aar", "prefix": true}, &es)
	if len(es) != 1 || es[0].Key != "AARON" {
		t.Errorf("prefix: %+v", es)
	}
	if err := e.run("lexicon.lookup", map[string]any{"strong": "X1"}, nil); code(err) != "bad_params" {
		t.Errorf("bad number: %v", err)
	}
}

func TestWordStudy(t *testing.T) {
	e := setupRef(t)
	var res WordStudyResult
	e.must("word.study", map[string]any{"strong": "G2316"}, &res)
	if res.Occurrences != 1 || len(res.Bibles) != 1 || res.Bibles[0].Renderings[0].Text != "θεὸς" {
		t.Errorf("G2316: %+v", res)
	}
	e.must("word.study", map[string]any{"strong": "H430"}, &res) // KJV Gen 1:1 "God"
	if len(res.Bibles) != 1 || res.Bibles[0].Module != "kjv" || res.Bibles[0].Renderings[0].Text != "god" ||
		res.Bibles[0].Renderings[0].Refs[0] != "Genesis 1:1" {
		t.Errorf("H430: %+v", res)
	}
	e.must("word.study", map[string]any{"strong": "G26"}, &res)
	if res.Occurrences != 0 || len(res.Bibles) != 0 {
		t.Errorf("absent number: %+v", res)
	}
}
