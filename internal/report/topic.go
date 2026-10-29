package report

import (
	"context"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
)

type TopicParams struct {
	Topic string `json:"topic" required:"true" desc:"A word or phrase, e.g. \"grace\" or \"covenant\""`
	Paging
}

// Section groups of a topic report, in order.
const (
	gTopical    = "topical"
	gDictionary = "dictionary"
	gVerses     = "verses"
	gBooks      = "books"
	gTopicNotes = "notes"
)

func (s *service) topic(ctx context.Context, env *command.Env, p TopicParams) (Report, error) {
	q := strings.Join(strings.Fields(p.Topic), " ")
	if q == "" {
		return Report{}, command.Errorf("bad_params", "topic is required")
	}
	pg, err := newPager(p.Paging)
	if err != nil {
		return Report{}, err
	}
	mods, err := s.modules(ctx, env)
	if err != nil {
		return Report{}, err
	}
	t := topicReport{s: s, env: env, pg: pg, mods: mods, bibles: mods.bibles(p.Bibles), q: q}
	rep := Report{Kind: "topic", Query: q, Subject: q, Title: "Topic report: " + q}
	rep.Sections, err = gather(ctx, pg, []part{
		{[]string{gTopical, gDictionary}, "Topical index and dictionaries", t.articles},
		{[]string{gVerses}, "Bible verses", t.verses},
		{[]string{gBooks}, "In your library", t.books},
		{[]string{gTopicNotes}, "Your notes", t.notes},
	})
	return rep, err
}

type topicReport struct {
	s      *service
	env    *command.Env
	pg     *pager
	mods   modules
	bibles []string
	q      string
}

// articles finds the topic's headwords in the dictionaries: topical indexes, whose entries are
// mostly references, come first, then dictionary articles.
func (t topicReport) articles(ctx context.Context) ([]Section, error) {
	found, err := call[[]bible.LookupEntry](ctx, t.s, t.env, "lexicon.lookup", map[string]any{"word": t.q, "prefix": true, "limit": 5})
	if err != nil {
		return nil, err
	}
	byModule := map[string][]bible.LookupEntry{}
	var order []string
	for _, e := range found {
		if e.Kind != bible.KindDictionary {
			continue
		}
		if _, ok := byModule[e.Module]; !ok {
			order = append(order, e.Module)
		}
		byModule[e.Module] = append(byModule[e.Module], e)
	}
	var topical, dict []Section
	for _, m := range order {
		es := byModule[m]
		group := gDictionary
		if topicalIndex(t.mods.byID[m], es) {
			group = gTopical
		}
		secs := t.s.headwordSections(group, es, t.mods, t.pg)
		if group == gTopical {
			topical = append(topical, secs...)
		} else {
			dict = append(dict, secs...)
		}
	}
	return append(topical, dict...), nil
}

// topicalIndex reports whether a dictionary is a topical index: its name says so, or its entries
// are mostly Bible references.
func topicalIndex(m bible.Meta, es []bible.LookupEntry) bool {
	if strings.Contains(strings.ToLower(m.Name), "topic") {
		return true
	}
	refs, chars := 0, 0
	for _, e := range es {
		_, links := linkRefs(e.Markup)
		refs += len(links)
		chars += len([]rune(e.Text))
	}
	return refs >= 5 && chars < refs*40
}

func (t topicReport) verses(ctx context.Context) ([]Section, error) {
	if len(t.bibles) == 0 {
		return nil, nil
	}
	m := t.bibles[0]
	sec := Section{ID: gVerses, Group: gVerses, Title: "Bible verses", Sources: []Source{t.mods.source(m)}}
	from, n := t.pg.start(gVerses), t.pg.size(20)
	hits, err := call[[]bible.Hit](ctx, t.s, t.env, "bible.search", map[string]any{"query": t.q, "modules": []string{m}, "limit": from + n + 1})
	if err != nil {
		return nil, err
	}
	for i, h := range hits {
		if i < from || i >= from+n {
			continue
		}
		sec.Entries = append(sec.Entries, Entry{Ref: h.ID.OSIS(), RefLabel: h.Ref, Text: h.Text, Snippet: h.Snippet})
	}
	sec.Total = min(len(hits), from+n)
	if len(hits) > from+n {
		sec.More = cursor(gVerses, from+n)
	}
	return []Section{sec}, nil
}

func (t topicReport) books(ctx context.Context) ([]Section, error) {
	return librarySection(ctx, t.s, t.env, t.pg, gBooks, "In your library", "search.query", map[string]any{"query": t.q})
}

func (t topicReport) notes(ctx context.Context) ([]Section, error) {
	from, n := t.pg.start(gTopicNotes), t.pg.size(20)
	ns, err := call[[]core.Note](ctx, t.s, t.env, "note.search", map[string]any{"query": t.q, "limit": n + 1, "offset": from})
	if err != nil {
		return nil, err
	}
	sec := Section{ID: gTopicNotes, Group: gTopicNotes, Title: "Your notes", Sources: yours("Your notes")}
	if len(ns) > n {
		ns = ns[:n]
		sec.More = cursor(gTopicNotes, from+n)
	}
	sec.Entries = noteEntries(ns)
	sec.Total = from + len(ns)
	return []Section{sec}, nil
}
