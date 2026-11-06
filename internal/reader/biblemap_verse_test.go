package reader

import (
	"fmt"
	"strings"
	"testing"
)

// bibleEPUB builds a Psalms book: one file per chapter, or all in one file when shared.
func bibleEPUB(t *testing.T, title string, chapters map[int]string, shared bool) []byte {
	order := []int{83, 84}
	files := [][2]string{
		{"mimetype", "application/epub+zip"},
		{"META-INF/container.xml", `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`},
	}
	var manifest, spine, nav string
	body := func(n int) string { return chapters[n] }
	if shared {
		var all strings.Builder
		for _, n := range order {
			fmt.Fprintf(&all, `<h2 id="ps%d">Psalm %d</h2>%s`, n, n, body(n))
			nav += fmt.Sprintf(`<li><a href="all.xhtml#ps%d">Psalm %d</a></li>`, n, n)
		}
		files = append(files, [2]string{"OEBPS/all.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>The Book of Psalms</h1>` + all.String() + `</body></html>`})
		manifest, spine = `<item id="a" href="all.xhtml" media-type="application/xhtml+xml"/>`, `<itemref idref="a"/>`
	} else {
		for _, n := range order {
			name := fmt.Sprintf("p%d.xhtml", n)
			files = append(files, [2]string{"OEBPS/" + name, fmt.Sprintf(`<html xmlns="http://www.w3.org/1999/xhtml"><body><h2>Psalm %d</h2>%s</body></html>`, n, body(n))})
			manifest += fmt.Sprintf(`<item id="p%d" href="%s" media-type="application/xhtml+xml"/>`, n, name)
			spine += fmt.Sprintf(`<itemref idref="p%d"/>`, n)
			nav += fmt.Sprintf(`<li><a href="%s">Psalm %d</a></li>`, name, n)
		}
	}
	files = append(files,
		[2]string{"OEBPS/content.opf", `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="uid">x</dc:identifier><dc:title>` + title + `</dc:title></metadata><manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>` + manifest + `</manifest><spine>` + spine + `</spine></package>`},
		[2]string{"OEBPS/nav.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body><nav epub:type="toc"><ol>` + nav + `</ol></nav></body></html>`})
	return zipBytes(t, files)
}

func TestAtReferenceVerses(t *testing.T) {
	sup := map[int]string{
		83: `<p><sup>1</sup>O God, do not keep silence.</p>`,
		84: `<p><sup>1</sup>How lovely is your dwelling place.<sup><a href="#n1">1</a></sup></p><p><sup>2</sup>My soul longs.</p>` +
			`<p>Even the sparrow finds a home, <sup>3</sup>at your altars, O Lord.</p><p><sup>4</sup>Blessed are those.</p>`,
	}
	lead := map[int]string{
		83: `<p>1 O God, do not keep silence.</p>`,
		84: `<p>1 How lovely is your dwelling place.</p><p>2 My soul longs.</p><p><span class="v">3</span> The sparrow finds a home.</p><p><span id="v4">4</span>Blessed are those.</p>`,
	}
	plain := map[int]string{83: `<p>O God, do not keep silence.</p>`, 84: `<p>How lovely.</p><p>My soul longs.</p>`}
	e := setupWith(t, map[string][]byte{
		"sup.epub": bibleEPUB(t, "Sup Psalms", sup, false), "lead.epub": bibleEPUB(t, "Lead Psalms", lead, false),
		"plain.epub": bibleEPUB(t, "Plain Psalms", plain, false), "shared.epub": bibleEPUB(t, "Shared Psalms", sup, true),
		"sharedlead.epub": bibleEPUB(t, "Shared Lead", lead, true)})
	at := func(title, ref string) AtRef {
		var a AtRef
		e.must("book.at_reference", map[string]any{"item_id": e.titles[title], "ref": ref}, &a)
		return a
	}
	for _, c := range []struct {
		title, ref, found string
		para, off         int
	}{
		{"Sup Psalms", "Psalm 84:2", "verse", 3, 0},
		{"Sup Psalms", "Psalm 84:3", "verse", 4, len("Even the sparrow finds a home, ")},
		{"Sup Psalms", "Psalm 84:4", "verse", 5, 0},
		{"Sup Psalms", "Psalm 84:9", "chapter", 0, 0},
		{"Sup Psalms", "Psalm 84", "chapter", 0, 0},
		{"Lead Psalms", "Psalm 84:3", "verse", 4, 0},
		{"Lead Psalms", "Psalm 84:4", "verse", 5, 0},
		{"Plain Psalms", "Psalm 84:2", "chapter", 0, 0},
		{"Shared Psalms", "Psalm 83:1", "verse", 3, 0},
		{"Shared Psalms", "Psalm 84:3", "verse", 7, len("Even the sparrow finds a home, ")},
		{"Shared Lead", "Psalm 84:1", "verse", 5, 0},
	} {
		a := at(c.title, c.ref)
		if a.Found != c.found || a.Para != c.para || a.Offset != c.off {
			t.Errorf("%s %s = %+v, want %s para %d offset %d", c.title, c.ref, a, c.found, c.para, c.off)
		}
		if c.found == "verse" && a.Anchor != fmt.Sprintf("bp-%d", c.para) {
			t.Errorf("%s %s anchor = %q", c.title, c.ref, a.Anchor)
		}
	}
}

func TestPickVerseSkipsStrayNumbers(t *testing.T) {
	marks := []verseMark{{N: 1, Para: 1}, {N: 7, Para: 1, Offset: 5}, {N: 2, Para: 2}, {N: 3, Para: 2, Offset: 9}}
	if m, ok := pickVerse(marks, 0, 0, 3); !ok || m.Offset != 9 {
		t.Errorf("got %+v %v", m, ok)
	}
	if _, ok := pickVerse(marks, 0, 0, 7); ok {
		t.Error("a jump from 1 to 7 is not a verse")
	}
}
