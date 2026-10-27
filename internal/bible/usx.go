package bible

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

// readUSX parses one USX 3 file. Its elements carry USFM styles, so it shares the USFM handler.
func readUSX(r io.Reader, b *builder) error {
	h := &usfmHandler{b: b}
	d := xml.NewDecoder(r)
	d.Strict = false
	d.Entity = xml.HTMLEntity
	var stack []string // the style each open element must close
	skip := 0
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			line, _ := d.InputPos()
			return fmt.Errorf("usx: line %d: %w", line, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			action := usxStart(h, t)
			if action == "skip" {
				skip++
			}
			stack = append(stack, action)
		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			action := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if action == "skip" {
				skip--
			}
			usxEnd(h, action)
		case xml.CharData:
			if skip == 0 {
				h.text(string(t))
			}
		}
	}
	h.endPara()
	b.endVerse()
	b.closeAll()
	return nil
}

func attrs(t xml.StartElement) map[string]string {
	m := map[string]string{}
	for _, a := range t.Attr {
		m[a.Name.Local] = a.Value
	}
	return m
}

// usxStart handles an opening element and returns what its end must do.
func usxStart(h *usfmHandler, t xml.StartElement) string {
	a := attrs(t)
	style := a["style"]
	switch t.Name.Local {
	case "usx", "table", "row", "cell", "ref", "optbreak", "ms", "sidebar":
	case "book":
		h.setBook(a["code"])
		return "skip"
	case "chapter":
		if a["eid"] == "" {
			h.setChapter(a["number"])
		}
	case "verse":
		if a["eid"] != "" {
			h.b.endVerse()
		} else {
			h.startVerse(a["number"])
		}
	case "para":
		if !h.para(style) {
			h.b.warn("unknown USX paragraph style %q kept as plain text", style)
		}
		return "para"
	case "note":
		h.noteOpen(style)
		return "note"
	case "char":
		if !h.charOpen(style, a) {
			h.b.warn("unknown USX character style %q kept as plain text", style)
			return ""
		}
		return "char:" + style
	case "figure", "periph":
		return "skip"
	default:
		h.b.warn("unknown USX element <%s> kept as plain text", t.Name.Local)
	}
	return ""
}

func usxEnd(h *usfmHandler, action string) {
	switch {
	case action == "para":
		h.endPara()
	case action == "note":
		h.noteClose()
	case len(action) > 5 && action[:5] == "char:":
		h.charClose(action[5:])
	}
}
