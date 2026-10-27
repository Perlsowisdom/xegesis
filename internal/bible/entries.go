package bible

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Entry is one entry of a verse-keyed source such as a SWORD module: a verse's markup, or an
// introduction (module, testament, book or chapter) that comes before the next verse.
type Entry struct {
	ID     verse.ID // the verse; for an introduction, the verse it comes before
	Intro  bool
	Markup string
}

// EntryMarkup names the markup of entries: osis, thml, gbf or plaintext.
type EntryMarkup string

// InstallEntries makes a module from verse entries. walk calls emit for every entry in canonical
// order. Headings in introductions are kept before the next verse; other introduction text is
// dropped.
func (l *Library) InstallEntries(ctx context.Context, o InstallOptions, markup EntryMarkup,
	walk func(emit func(Entry) error) error) (Meta, []string, error) {
	read, err := entryReader(markup)
	if err != nil {
		return Meta{}, nil, err
	}
	b := newBuilder()
	err = walk(func(e Entry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.Intro {
			b.endVerse()
			b.intro = true
			defer func() { b.intro = false }()
		} else {
			b.startVerse(e.ID, e.ID)
		}
		if err := read(e.Markup, b); err != nil {
			b.warn("%s: markup could not be read, kept as plain text (%v)", e.ID, err)
			b.addText(stripTags(e.Markup))
		}
		return nil
	})
	if err != nil {
		return Meta{}, nil, err
	}
	b.endVerse()
	b.closeAll()
	return l.finish(ctx, o, Doc{}, b)
}

func entryReader(m EntryMarkup) (func(string, *builder) error, error) {
	switch strings.ToLower(string(m)) {
	case "osis":
		return readOSISFragment, nil
	case "thml":
		return readThMLFragment, nil
	case "gbf":
		return readGBFFragment, nil
	case "plaintext", "":
		return func(s string, b *builder) error { b.addText(s); return nil }, nil
	}
	return nil, fmt.Errorf("bible: entries in %q markup cannot be read", m)
}

// ThML (Theological Markup Language, CCEL) as used in verse entries: notes, scripture
// references, Strong's numbers as <sync type="Strongs" value="H1234"/>, section headings and
// paragraphs. Other elements keep their text.
func readThMLFragment(s string, b *builder) error {
	d := xml.NewDecoder(strings.NewReader("<entry>" + s + "</entry>"))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	var stack []string
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, thmlStart(t, b))
		case xml.EndElement:
			if n := len(stack); n > 0 {
				if tag := stack[n-1]; tag != "" {
					b.closeSpan(tag)
				}
				stack = stack[:n-1]
			}
		case xml.CharData:
			b.addText(string(t))
		}
	}
}

func thmlStart(t xml.StartElement, b *builder) string {
	switch strings.ToLower(t.Name.Local) {
	case "note":
		b.openSpan("f", "", true, false)
		return "f"
	case "scripref":
		ref := ""
		if r, err := verse.Parse(get(t, "passage")); err == nil {
			ref = r.Start.OSIS()
		}
		b.openSpan("r", attr("o", ref), false, false)
		return "r"
	case "sync":
		if strings.EqualFold(get(t, "type"), "Strongs") {
			if n := normStrong(get(t, "value")); n != "" {
				b.strong([]string{n})
			}
		}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		b.openSpan("h", "", true, false)
		return "h"
	case "div":
		if c := get(t, "class"); c == "sechead" || c == "title" {
			b.openSpan("h", "", true, false)
			return "h"
		}
	case "p", "br":
		b.paragraph()
	}
	return ""
}

// GBF (General Bible Format) is a legacy markup of angle-bracket tokens such as <FR>…<Fr> and
// <WH1234>. Strong's numbers are kept; other tokens are dropped and their text kept.
var gbfToken = regexp.MustCompile(`<([A-Z][A-Za-z]?)([^>]*)>`)

func readGBFFragment(s string, b *builder) error {
	last := 0
	for _, m := range gbfToken.FindAllStringSubmatchIndex(s, -1) {
		b.addText(s[last:m[0]])
		last = m[1]
		tag, arg := s[m[2]:m[3]], s[m[4]:m[5]]
		switch {
		case tag == "WH" || tag == "WG":
			if n := normStrong(tag[1:] + arg); n != "" {
				b.strong([]string{n})
			}
		case tag == "RF":
			b.openSpan("f", "", true, false)
		case tag == "Rf":
			b.closeSpan("f")
		case tag == "CM" || tag == "CL":
			b.paragraph()
		}
	}
	b.addText(s[last:])
	return nil
}
