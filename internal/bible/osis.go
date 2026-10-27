package bible

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// OSIS import, following the OSIS 2.1.1 user manual. Verses may be containers or sID/eID
// milestones; an osisID listing several verses ("Gen.1.1 Gen.1.2") is a combined verse.

// osisPass lists elements whose text is kept without markup of their own.
var osisPass = map[string]bool{
	"osis": true, "osisText": true, "div": true, "chapter": true, "lg": true, "l": true, "hi": true,
	"foreign": true, "seg": true, "divineName": true, "name": true, "inscription": true, "abbr": true,
	"catchWord": true, "rdg": true, "list": true, "item": true, "label": true, "milestone": true,
	"lb": true, "speech": true, "speaker": true, "mentioned": true, "date": true, "index": true,
	"table": true, "row": true, "cell": true, "figure": true, "transChange": true, "w": true,
	"q": true, "note": true, "title": true, "reference": true, "p": true, "verse": true,
}

type osisReader struct {
	b      *builder
	stack  []string // what each open element must undo when it ends: "" or an action
	header int      // depth inside <header>
	skip   int      // depth inside elements whose text is dropped
	title  *strings.Builder
	doc    Doc
}

func readOSIS(r io.Reader, b *builder) (Doc, error) {
	o := &osisReader{b: b}
	d := xml.NewDecoder(r)
	d.Strict = false
	d.Entity = xml.HTMLEntity
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			line, _ := d.InputPos()
			return o.doc, fmt.Errorf("osis: line %d: %w", line, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			o.stack = append(o.stack, o.start(t))
		case xml.EndElement:
			o.end(t.Name.Local)
		case xml.CharData:
			o.chars(string(t))
		}
	}
	b.endVerse()
	b.closeAll()
	return o.doc, nil
}

func get(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// start handles an opening tag and returns the action its closing tag must take.
func (o *osisReader) start(t xml.StartElement) string {
	name := t.Name.Local
	if o.header > 0 || name == "header" {
		o.header++
		if name == "title" && o.doc.Name == "" {
			o.title = &strings.Builder{}
		}
		return "header"
	}
	b := o.b
	switch name {
	case "osisText":
		o.doc.Language = get(t, "lang")
	case "verse":
		return o.verse(t)
	case "w":
		return o.word(t)
	case "note":
		tag := "f"
		if get(t, "type") == "crossReference" {
			tag = "x"
		}
		b.openSpan(tag, "", true, false)
		return "span:" + tag
	case "title":
		if typ := get(t, "type"); typ == "x-gen" || typ == "runningHead" {
			o.skip++
			return "skip"
		}
		b.openSpan("h", attr("k", headingKind(get(t, "type"), get(t, "canonical"))), true, false)
		return "span:h"
	case "q":
		return o.quote(t)
	case "transChange":
		b.openSpan("add", "", false, false)
		return "span:add"
	case "reference":
		b.openSpan("r", attr("o", get(t, "osisRef")), false, false)
		return "span:r"
	case "p", "l":
		if get(t, "eID") == "" {
			b.paragraph()
		}
	case "lb":
		b.addText(" ")
	case "div":
		if get(t, "type") == "book" {
			return "div"
		}
	default:
		if !osisPass[name] {
			b.warn("unknown OSIS element <%s> kept as plain text", name)
		}
	}
	return ""
}

func headingKind(typ, canonical string) string {
	if typ == "psalm" || canonical == "true" {
		return "psalm"
	}
	return ""
}

func (o *osisReader) verse(t xml.StartElement) string {
	if get(t, "eID") != "" {
		o.b.endVerse()
		return ""
	}
	ids := strings.Fields(get(t, "osisID"))
	if len(ids) == 0 {
		o.b.warn("verse without an osisID ignored")
		return ""
	}
	first, err1 := osisVerse(ids[0])
	last, err2 := osisVerse(ids[len(ids)-1])
	if err := errors.Join(err1, err2); err != nil {
		o.b.warn("verse %q skipped: %v", get(t, "osisID"), err)
		o.b.endVerse()
		return ""
	}
	o.b.startVerse(first, last)
	if get(t, "sID") == "" {
		return "verse"
	}
	return ""
}

// osisVerse reads "Gen.1.1", ignoring a work prefix such as "KJV:".
func osisVerse(ref string) (verse.ID, error) {
	if _, after, ok := strings.Cut(ref, ":"); ok {
		ref = after
	}
	p := strings.Split(ref, ".")
	if len(p) < 3 {
		return 0, fmt.Errorf("not a verse reference")
	}
	b, ok := verse.LookupBook(p[0])
	if !ok {
		return 0, fmt.Errorf("unknown book %q", p[0])
	}
	c, err1 := strconv.Atoi(p[1])
	v, err2 := strconv.Atoi(strings.TrimRight(p[2], "abcdefgh!"))
	if err1 != nil || err2 != nil {
		return 0, fmt.Errorf("bad numbers")
	}
	return verse.Make(b.Number, c, v)
}

func (o *osisReader) word(t xml.StartElement) string {
	var nums []string
	for _, l := range strings.Fields(get(t, "lemma")) {
		if n := normStrong(l); n != "" {
			nums = append(nums, n)
		}
	}
	o.b.strong(nums)
	o.b.openSpan("w", attr("s", strings.Join(nums, " "))+attr("m", get(t, "morph")), false, false)
	return "span:w"
}

// quote handles words of Christ, as a container or as sID/eID milestones.
func (o *osisReader) quote(t xml.StartElement) string {
	jesus := get(t, "who") == "Jesus"
	switch {
	case get(t, "eID") != "":
		o.b.closeSpan("j")
	case jesus && get(t, "sID") != "":
		o.b.openSpan("j", "", false, false)
	case jesus:
		o.b.openSpan("j", "", false, false)
		return "span:j"
	}
	return ""
}

func (o *osisReader) end(name string) {
	action := ""
	if n := len(o.stack); n > 0 {
		action = o.stack[n-1]
		o.stack = o.stack[:n-1]
	}
	switch {
	case action == "header":
		o.header--
		if name == "title" && o.title != nil {
			o.doc.Name = squeeze(o.title.String())
			o.title = nil
		}
	case action == "skip":
		o.skip--
	case action == "verse", action == "div":
		o.b.endVerse()
	case strings.HasPrefix(action, "span:"):
		o.b.closeSpan(action[5:])
	}
}

func (o *osisReader) chars(s string) {
	switch {
	case o.header > 0:
		if o.title != nil {
			o.title.WriteString(s)
		}
	case o.skip == 0:
		o.b.addText(s)
	}
}
