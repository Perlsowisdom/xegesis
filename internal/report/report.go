// Package report gathers in one place what the library holds about a passage, a word or a topic.
// A report is built only from installed Bibles, reference works and books, the user's own notes,
// lists and plans, by running other commands, so it shows exactly what those commands show. Its
// sections are gathered at the same time and each one names its sources with their attribution and
// licence. The result's shape is described by the commands' schemas (xegesis describe report.passage).
//
// Sources of the idea: the Phase 1 functional survey (passage, word and topic study), STEPBible's word
// pages (lexicon entry, counts by book, renderings) and the SWORD Project's topical indexes.
package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/search"
)

// Register adds the report commands to r. They run other commands through r.
func Register(r *command.Registry) {
	s := &service{reg: r}
	r.Add(
		command.Define("report.passage", "Gather everything about a passage: the text in your Bibles side by side, commentaries, "+
			"cross-references, books that cite it, your notes, highlights, lists and reading-plan days, and its original-language words.", false, s.passage),
		command.Define("report.word", "Gather everything about a word (a Strong's number, an original-language word or an English word): "+
			"lexicon entries, where it occurs with a count by book, how each Bible translates it, related words and books that use it.", false, s.word),
		command.Define("report.topic", "Gather everything about a topic: topical index entries, dictionary articles, Bible verses, "+
			"library passages and your notes that match.", false, s.topic),
		command.Define("report.save", "Save a report as a note: its headings and links (references, words, books), "+
			"without the text of the works it draws on.", true, s.save),
	)
}

type service struct{ reg *command.Registry }

// Report is what report.passage, report.word and report.topic return.
type Report struct {
	Kind       string    `json:"kind" desc:"passage, word or topic"`
	Query      string    `json:"query" desc:"What to pass to run the report again: an OSIS reference, a Strong's number or the topic"`
	Subject    string    `json:"subject" desc:"What the report is about, in words, e.g. \"Romans 8:28\" or \"G25 ἀγαπάω\""`
	Title      string    `json:"title" desc:"e.g. \"Passage report: Romans 8:28\""`
	Ref        string    `json:"ref,omitempty" desc:"Passage reports: the passage as an OSIS reference"`
	Strong     string    `json:"strong,omitempty" desc:"Word reports: the Strong's number the report is about"`
	Candidates []Entry   `json:"candidates,omitempty" desc:"Word reports for an English word: the Strong's numbers it translates, most frequent first"`
	Sections   []Section `json:"sections" desc:"In a fixed order; sections with nothing to show are left out"`
}

// Source is a work a section is drawn from, with what its licence asks of copies.
type Source struct {
	Module      string `json:"module,omitempty" desc:"Installed module ID, for Bibles and reference works"`
	Name        string `json:"name"`
	Attribution string `json:"attribution,omitempty" desc:"Credit line to show with the section"`
	Licence     string `json:"licence,omitempty" desc:"Licence type: public_domain, creative_commons, open_other, personal_purchase, own_work or unknown"`
	SPDX        string `json:"spdx,omitempty"`
}

// Section is one part of a report.
type Section struct {
	ID      string   `json:"id" desc:"Unique in the report and stable: the group, or group:module"`
	Group   string   `json:"group" desc:"Sections of one group are shown together, e.g. every Bible's text side by side. Passage reports: text, commentary, crossrefs, words, citing, notes, highlights, lists, plans; word reports: lexicon, occurrences, renderings, related, books, lists; topic reports: topical, dictionary, verses, books, notes"`
	Title   string   `json:"title"`
	Sources []Source `json:"sources" desc:"Where the entries come from"`
	Entries []Entry  `json:"entries"`
	Chart   []Bar    `json:"chart,omitempty" desc:"Word reports: verses by book of the Bible"`
	Total   int      `json:"total" desc:"Entries in the whole section, when known"`
	More    string   `json:"more,omitempty" desc:"Cursor for the next page: run the same command again with cursor set to this"`
	Hint    string   `json:"hint,omitempty" desc:"A note on the section, e.g. that some library folders are still being indexed"`
	Error   string   `json:"error,omitempty" desc:"Why the section could not be gathered"`
}

// Entry is one item in a section. Which fields are set depends on the section; every reference,
// word and book in it can be opened.
type Entry struct {
	Heading  string     `json:"heading,omitempty"`
	Text     string     `json:"text,omitempty" desc:"Plain text"`
	Markup   string     `json:"markup,omitempty" desc:"The same in Xegesis markup, with <r o=\"OSIS\"> references and <w s=\"G25\"> words"`
	Snippet  string     `json:"snippet,omitempty" desc:"Text around a match, with matches wrapped in <mark></mark>"`
	Ref      string     `json:"ref,omitempty" desc:"The passage the entry is about, as an OSIS reference"`
	RefLabel string     `json:"ref_label,omitempty" desc:"That passage in words"`
	Strong   string     `json:"strong,omitempty"`
	Lemma    string     `json:"lemma,omitempty"`
	Translit string     `json:"transliteration,omitempty"`
	Gloss    string     `json:"gloss,omitempty"`
	Morph    string     `json:"morphology,omitempty"`
	Count    int        `json:"count,omitempty"`
	Links    []Link     `json:"links,omitempty" desc:"Further passages, e.g. cross-reference targets"`
	Book     *BookLink  `json:"book,omitempty" desc:"A place in a library book"`
	NoteID   string     `json:"note_id,omitempty"`
	ListID   string     `json:"list_id,omitempty"`
	PlanID   string     `json:"plan_id,omitempty"`
	Day      int        `json:"day,omitempty" desc:"Reading plans: the day's number"`
	Done     bool       `json:"done,omitempty" desc:"Reading plans: the day has been read"`
	Topic    string     `json:"topic,omitempty" desc:"A headword to open as a topic report"`
	Module   string     `json:"module,omitempty" desc:"The module an entry of a several-source section comes from"`
	Extra    []string   `json:"extra,omitempty" desc:"Short facts shown after the heading"`
	Words    []WordLink `json:"words,omitempty" desc:"Related words"`
}

// Link is a passage to open.
type Link struct {
	Ref   string `json:"ref" desc:"OSIS reference"`
	Label string `json:"label"`
}

// WordLink is a word to open as a word report.
type WordLink struct {
	Strong string `json:"strong"`
	Lemma  string `json:"lemma,omitempty"`
}

// BookLink is a place in a library book, as search.query returns it.
type BookLink struct {
	ItemID  string         `json:"item_id"`
	FileID  string         `json:"file_id"`
	Title   string         `json:"title"`
	Authors []string       `json:"authors"`
	Heading string         `json:"heading,omitempty"`
	Locator search.Locator `json:"locator"`
	Online  bool           `json:"online"`
}

// Bar is one bar of a chart.
type Bar struct {
	Label string `json:"label" desc:"Book name"`
	Book  string `json:"book" desc:"OSIS book ID"`
	Count int    `json:"count"`
}

// Paging is shared by the report commands.
type Paging struct {
	Sections []string `json:"sections,omitempty" desc:"Only these sections, by ID or group, e.g. [\"citing\", \"commentary\"]; all when omitted"`
	Cursor   string   `json:"cursor,omitempty" desc:"A section's more value: only that section's next page is returned"`
	Limit    int      `json:"limit,omitempty" desc:"Most entries per section (default 20, Bible text 100; at most 500)"`
	Bibles   []string `json:"bibles,omitempty" desc:"Bibles to show text from, in order; the installed ones (at most 6) when omitted"`
}

const maxLimit = 500

// pager decides which sections to gather and which page of each.
type pager struct {
	want   map[string]bool // section IDs or groups; nil means all
	cursor string          // the section being paged
	offset int
	limit  int
}

func newPager(p Paging) (*pager, error) {
	pg := &pager{limit: min(max(p.Limit, 0), maxLimit)}
	if p.Cursor != "" {
		id, off, ok := strings.Cut(p.Cursor, "@")
		n, err := strconv.Atoi(off)
		if !ok || err != nil || n < 0 || id == "" {
			return nil, command.Errorf("bad_params", "cursor %q is not a section's more value", p.Cursor)
		}
		pg.cursor, pg.offset, pg.want = id, n, map[string]bool{id: true}
		return pg, nil
	}
	if len(p.Sections) > 0 {
		pg.want = map[string]bool{}
		for _, s := range p.Sections {
			pg.want[s] = true
		}
	}
	return pg, nil
}

func groupOf(id string) string {
	g, _, _ := strings.Cut(id, ":")
	return g
}

// wants reports whether a section is asked for.
func (p *pager) wants(id string) bool {
	return p.want == nil || p.want[id] || p.want[groupOf(id)] && p.cursor == ""
}

// wantsGroup reports whether any section of the group may be asked for.
func (p *pager) wantsGroup(group string) bool {
	if p.want == nil || p.want[group] {
		return true
	}
	for id := range p.want {
		if groupOf(id) == group {
			return true
		}
	}
	return false
}

// start is the first entry to return for a section.
func (p *pager) start(id string) int {
	if id == p.cursor {
		return p.offset
	}
	return 0
}

// size is the page size for a section whose default is def.
func (p *pager) size(def int) int {
	if p.limit > 0 {
		return p.limit
	}
	return def
}

// page puts one page of all into s, with a cursor when more follow.
func (p *pager) page(s *Section, all []Entry, def int) {
	s.Total = len(all)
	from, n := p.start(s.ID), p.size(def)
	if from > len(all) {
		from = len(all)
	}
	to := min(from+n, len(all))
	s.Entries = all[from:to]
	if to < len(all) {
		s.More = cursor(s.ID, to)
	}
}

func cursor(id string, offset int) string { return fmt.Sprintf("%s@%d", id, offset) }

// part gathers the sections of some groups.
type part struct {
	groups []string
	title  string // shown if the part fails
	run    func(ctx context.Context) ([]Section, error)
}

// gather runs the parts that are asked for at the same time and returns their sections in order.
// A part that fails becomes a section with an error; parts that need an open library are left out
// when none is open. If ctx ends, every part stops and gather returns its error.
func gather(ctx context.Context, pg *pager, parts []part) ([]Section, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([][]Section, len(parts))
	var wg sync.WaitGroup
	for i, p := range parts {
		asked := false
		for _, g := range p.groups {
			asked = asked || pg.wantsGroup(g)
		}
		if !asked {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			secs, err := p.run(ctx)
			if err != nil {
				secs = failed(p, err)
			}
			results[i] = secs
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := []Section{}
	for _, secs := range results {
		for _, s := range secs {
			if !pg.wants(s.ID) || (len(s.Entries) == 0 && s.Error == "") {
				continue
			}
			if s.Sources == nil {
				s.Sources = []Source{}
			}
			out = append(out, s)
		}
	}
	return out, nil
}

func failed(p part, err error) []Section {
	var ce *command.Error
	if errors.As(err, &ce) && (ce.Code == "unavailable" || ce.Code == "not_found") {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return []Section{{ID: p.groups[0], Group: p.groups[0], Title: p.title, Error: err.Error()}}
}

// call runs another command with params and returns its result as T.
func call[T any](ctx context.Context, s *service, env *command.Env, id string, params any) (T, error) {
	var out T
	raw, err := json.Marshal(params)
	if err != nil {
		return out, err
	}
	res, err := s.reg.Run(ctx, env, id, raw)
	if err != nil {
		return out, err
	}
	if v, ok := res.(T); ok {
		return v, nil
	}
	b, err := json.Marshal(res)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(b, &out)
}

// modules lists the installed modules by ID.
type modules struct {
	list []bible.Meta
	byID map[string]bible.Meta
}

func (s *service) modules(ctx context.Context, env *command.Env) (modules, error) {
	list, err := call[[]bible.Meta](ctx, s, env, "bible.modules", map[string]string{"kind": "all"})
	m := modules{list: list, byID: map[string]bible.Meta{}}
	for _, x := range list {
		m.byID[x.ID] = x
	}
	return m, err
}

func (m modules) source(id string) Source {
	x, ok := m.byID[id]
	if !ok {
		return Source{Module: id, Name: id}
	}
	return Source{Module: id, Name: x.Name, Attribution: x.Licence.AttributionLine(), Licence: string(x.Licence.Type), SPDX: x.Licence.SPDX}
}

func (m modules) ofKind(kind string) []bible.Meta {
	var out []bible.Meta
	for _, x := range m.list {
		if x.Kind == kind {
			out = append(out, x)
		}
	}
	return out
}

// bibles returns the Bibles to show text from: those asked for that are installed, or the
// installed ones up to six.
func (m modules) bibles(asked []string) []string {
	var out []string
	for _, id := range asked {
		if x, ok := m.byID[id]; ok && x.Kind == bible.KindBible {
			out = append(out, id)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, x := range m.ofKind(bible.KindBible) {
		if len(out) < 6 {
			out = append(out, x.ID)
		}
	}
	return out
}

// original reports whether a module is in Greek, Hebrew or Aramaic.
func original(x bible.Meta) bool {
	lang, _, _ := strings.Cut(strings.ToLower(x.Language), "-")
	switch lang {
	case "grc", "el", "gr", "hbo", "he", "heb", "arc", "x-hbo":
		return true
	}
	return false
}

// yours is the source of the user's own data.
func yours(name string) []Source { return []Source{{Name: name, Licence: "own_work"}} }
