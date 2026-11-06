package reader

import (
	"os"
	"regexp"
	"strconv"

	xhtml "golang.org/x/net/html"
)

// Verse numbers inside a Bible-type book. The sanitiser notes, while it numbers paragraphs, where
// text looks like the start of a verse: a superscript number, a number in an element whose class or
// id says "verse", or a number leading a paragraph or span. Positions are a paragraph number and a
// count of UTF-16 units of text after that paragraph's marker, as highlights count them.

type verseMark struct {
	N, Para, Offset int
	Strong          bool // class, id or a number alone in a span; superscripts and leading digits are weaker
}

var (
	verseClass = regexp.MustCompile(`(?i)(^|[\s_-])(verse|verses|versenum|versenumber|vnum|vn|vs|v-num|versenumbers?)($|[\s_-])`)
	verseID    = regexp.MustCompile(`(?i)^(?:v|vs|ver|verse)[-_.]?(\d{1,3})$`)
	numberOnly = regexp.MustCompile(`^[\s\x{a0}]*(\d{1,3})[a-c]?(?:[-\x{2013}]\d{1,3})?[\s\x{a0}]*$`)
	leadNumber = regexp.MustCompile(`^\s*(\d{1,3})(?:[-\x{2013}]\d{1,3})?[\s\x{a0}]+\S`)
)

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// noteTag looks at a kept start tag for a verse marker or an anchor. pushed is true when the
// element is now open.
func (s *sanitizer) noteTag(tag string, t xhtml.Token, pushed bool) {
	if id := attrOf(t, "id"); id != "" {
		if s.ids == nil {
			s.ids = map[string]int{}
		}
		p := s.para
		if !s.started {
			p++
		}
		if _, dup := s.ids[bookID(id)]; !dup {
			s.ids[bookID(id)] = p
		}
		if m := verseID.FindStringSubmatch(id); m != nil {
			s.pending, _ = strconv.Atoi(m[1])
		}
	}
	if !pushed {
		return
	}
	cls := attrOf(t, "class")
	switch {
	case verseClass.MatchString(cls) || verseClass.MatchString(attrOf(t, "epub:type")):
		s.setMark(true)
	case tag == "sup":
		s.setMark(false)
	}
}

func (s *sanitizer) setMark(strong bool) {
	if !s.mark {
		s.mark, s.markStrong, s.markDepth = true, strong, len(s.open)
	} else if strong {
		s.markStrong = true
	}
}

// markClosed ends a marker element when the open elements fall below it.
func (s *sanitizer) markClosed() {
	if s.mark && len(s.open) < s.markDepth {
		s.mark, s.markStrong = false, false
	}
}

// noteText records a verse start in a run of text written inside a numbered paragraph.
func (s *sanitizer) noteText(t string) {
	if !s.started {
		return
	}
	var n int
	strong := false
	atoi := func(m []string) int { v, _ := strconv.Atoi(m[1]); return v }
	switch m := numberOnly.FindStringSubmatch(t); {
	case s.pending > 0:
		n, strong, s.pending = s.pending, true, 0
	case s.mark && m != nil && (s.links == 0 || s.markStrong):
		n, strong = atoi(m), s.markStrong
	case m != nil && len(s.open) > 0 && inlineStart[s.open[len(s.open)-1]]:
		n = atoi(m)
	case s.off == 0:
		if l := leadNumber.FindStringSubmatch(t); l != nil {
			n = atoi(l)
		}
	}
	if n > 0 {
		s.marks = append(s.marks, verseMark{N: n, Para: s.para, Offset: s.off, Strong: strong})
	}
}

var inlineStart = map[string]bool{"span": true, "b": true, "strong": true, "small": true}

// pickVerse finds verse v among the marks that lie in paragraphs [from, to). Marks are accepted
// in order as a run of rising numbers starting at 1 or 2, so footnote numbers and stray digits are
// passed over. to == 0 means to the end of the section.
func pickVerse(marks []verseMark, from, to, v int) (verseMark, bool) {
	last := 0
	for _, m := range marks {
		if m.Para < from || to > 0 && m.Para >= to {
			continue
		}
		ok := m.N > last && (m.Strong || (last == 0 && m.N <= 2) || (last > 0 && m.N <= last+3))
		if !ok {
			continue
		}
		last = m.N
		if m.N == v {
			return m, true
		}
	}
	return verseMark{}, false
}

// renderedSection sanitises one section of a reflowable book again, for its verse marks.
func renderedSection(f book, n int) (rendered, bool) {
	switch f.file.Format {
	case "epub":
		e, err := openEPUB(f.file.Path)
		if err != nil || n < 1 || n > len(e.spine) || !e.spine[n-1].html {
			return rendered{}, false
		}
		defer e.Close()
		doc, err := e.read(e.spine[n-1].path)
		if err != nil {
			return rendered{}, false
		}
		return sanitize(doc, epubRes{e: e, base: e.spine[n-1].path}), true
	case "html":
		doc, err := os.ReadFile(f.file.Path)
		if err != nil || n != 1 {
			return rendered{}, false
		}
		return sanitize(doc, docRes{}), true
	}
	return rendered{}, false
}

// verseAt finds where verse v starts in the chapter that entries[i] places. It gives up, so the
// chapter opens instead, when the chapter's bounds in its section cannot be told.
func verseAt(r rendered, entries []MapEntry, i, v int) (verseMark, bool) {
	e := entries[i]
	from := 0
	if e.Anchor != "" {
		if from = r.ids[e.Anchor]; from == 0 {
			return verseMark{}, false
		}
	}
	to := 0
	for j, o := range entries {
		if j == i || o.Section != e.Section || o.Chapter == 0 && o.Book == e.Book && o.Anchor == "" {
			continue
		}
		p := r.ids[o.Anchor]
		switch {
		case o.Anchor == "" || p == 0:
			return verseMark{}, false
		case p > from && (to == 0 || p < to):
			to = p
		}
	}
	return pickVerse(r.marks, from, to, v)
}
