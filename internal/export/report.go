package export

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

// Reports (T13) come from report.passage, report.word and report.topic. Their result is read by
// shape rather than by type, so printing works with whatever those commands return: a title, and
// sections that each have a title, an attribution and lists of entries. An entry that carries a
// library book's item_id and text counts against that book's licence like any excerpt.

var reportTitleKeys = []string{"title", "ref", "display", "query", "word", "topic"}
var entryTitleKeys = []string{"ref", "display", "title", "word", "lemma", "headword", "name"}
var entryTextKeys = []string{"text", "snippet", "gloss", "definition", "summary", "body"}

func (bd *builder) report(src Source) ([]draft, error) {
	if src.Report == "" {
		return nil, command.Errorf("bad_params", "a report needs report: passage, word or topic")
	}
	id := "report." + src.Report
	if _, ok := bd.s.reg.Describe(id); !ok {
		return nil, command.Errorf("unavailable", "reports cannot be printed yet: this version has no %s", id)
	}
	params := src.Params
	if len(params) == 0 {
		params = json.RawMessage("{}")
	}
	var res map[string]any
	if err := bd.run(id, params, &res); err != nil {
		return nil, err
	}
	title := firstString(res, reportTitleKeys)
	if title == "" {
		title = strings.ToUpper(src.Report[:1]) + src.Report[1:] + " report"
	}
	d := draft{piece: Piece{Title: title}}
	sections, _ := res["sections"].([]any)
	for _, s := range sections {
		sec, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if t := firstString(sec, []string{"title", "heading", "name"}); t != "" {
			d.units = append(d.units, unit{block: Block{Kind: Heading, Level: 2, Spans: plain(t)}})
		}
		if t, ok := sec["text"].(string); ok && t != "" {
			d.units = append(d.units, unit{block: Block{Kind: Para, Spans: plain(t)}})
		}
		for _, e := range entriesOf(sec) {
			u, err := bd.reportEntry(e, &d)
			if err != nil {
				return nil, err
			}
			d.units = append(d.units, u)
		}
		if a, ok := sec["attribution"].(string); ok && a != "" && !contains(d.piece.Credits, a) {
			d.piece.Credits = append(d.piece.Credits, a)
		}
	}
	return []draft{d}, nil
}

func (bd *builder) reportEntry(e map[string]any, d *draft) (unit, error) {
	head := firstString(e, entryTitleKeys)
	text := firstString(e, entryTextKeys)
	var spans []Span
	if head != "" {
		spans = append(spans, Span{Text: head, Bold: true})
	}
	if text != "" {
		if head != "" {
			text = " " + text
		}
		spans = append(spans, Span{Text: text})
	}
	u := unit{block: Block{Kind: Bullet, Spans: spans}}
	if item, ok := e["item_id"].(string); ok && item != "" && text != "" {
		o, err := bd.itemOwnerByID(item)
		if err != nil {
			return unit{}, err
		}
		d.owners = appendOwner(d.owners, o)
		u.charges = []charge{{o, chars(text)}}
		if o.credit != "" && !contains(d.piece.Credits, o.credit) {
			d.piece.Credits = append(d.piece.Credits, o.credit)
		}
	}
	return u, nil
}

// entriesOf finds a section's entries: every list of objects in it, in key order.
func entriesOf(sec map[string]any) []map[string]any {
	keys := make([]string, 0, len(sec))
	for k := range sec {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []map[string]any
	for _, k := range keys {
		list, ok := sec[k].([]any)
		if !ok {
			continue
		}
		for _, x := range list {
			if m, ok := x.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

func firstString(m map[string]any, keys []string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case map[string]any:
			if s := firstString(v, []string{"display", "title", "text"}); s != "" {
				return s
			}
		}
	}
	return ""
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// dateWords writes YYYY-MM-DD as "Mon 3 Nov 2026".
func dateWords(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return fmt.Sprintf("%s %d %s %d", t.Format("Mon"), t.Day(), t.Format("Jan"), t.Year())
}
