package report

import (
	"context"
	"fmt"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/lists"
	"github.com/Perlsowisdom/xegesis/internal/plan"
	"github.com/Perlsowisdom/xegesis/internal/reader"
	"github.com/Perlsowisdom/xegesis/internal/search"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

type PassageParams struct {
	Ref string `json:"ref" required:"true" desc:"A passage, e.g. \"Romans 8:28-30\""`
	Paging
}

// Section groups of a passage report, in order.
const (
	gText       = "text"
	gCommentary = "commentary"
	gCrossRefs  = "crossrefs"
	gWords      = "words"
	gCiting     = "citing"
	gNotes      = "notes"
	gHighlights = "highlights"
	gLists      = "lists"
	gPlans      = "plans"
)

func (s *service) passage(ctx context.Context, env *command.Env, p PassageParams) (Report, error) {
	r, err := verse.Parse(p.Ref)
	if err != nil {
		return Report{}, command.Errorf("bad_params", "%v", err)
	}
	pg, err := newPager(p.Paging)
	if err != nil {
		return Report{}, err
	}
	osis := osisOf(r)
	rep := Report{Kind: "passage", Query: osis, Ref: osis, Subject: r.String(), Title: "Passage report: " + r.String()}
	mods, err := s.modules(ctx, env)
	if err != nil {
		return rep, err
	}
	pr := passageReport{s: s, env: env, pg: pg, r: r, osis: osis, mods: mods, bibles: mods.bibles(p.Bibles)}
	rep.Sections, err = gather(ctx, pg, []part{
		{[]string{gText}, "Bible text", pr.text},
		{[]string{gCommentary, gCrossRefs}, "Commentaries and cross-references", pr.reference},
		{[]string{gWords}, "Original-language words", pr.words},
		{[]string{gCiting}, "Books that cite it", pr.citing},
		{[]string{gNotes}, "Your notes", pr.notes},
		{[]string{gHighlights}, "Your highlights", pr.highlights},
		{[]string{gLists}, "In your lists", pr.lists},
		{[]string{gPlans}, "In your reading plans", pr.plans},
	})
	return rep, err
}

type passageReport struct {
	s      *service
	env    *command.Env
	pg     *pager
	r      verse.Range
	osis   string
	mods   modules
	bibles []string
}

// text is the passage in each Bible, one section per Bible, lined up by verse.
func (p passageReport) text(ctx context.Context) ([]Section, error) {
	if len(p.bibles) == 0 {
		return nil, nil
	}
	res, err := call[bible.ParallelResult](ctx, p.s, p.env, "bible.parallel", map[string]any{"modules": p.bibles, "ref": p.osis, "markup": true})
	if err != nil {
		return nil, err
	}
	var out []Section
	for _, col := range res.Columns {
		src := p.mods.source(col.Module)
		sec := Section{ID: gText + ":" + col.Module, Group: gText, Title: src.Name, Sources: []Source{src}}
		var all []Entry
		for _, v := range col.Verses {
			id := v.BaseID
			if id == 0 {
				id = v.ID
			}
			all = append(all, Entry{Ref: id.OSIS(), RefLabel: v.Ref, Text: v.Text, Markup: v.Markup})
		}
		p.pg.page(&sec, all, 100)
		out = append(out, sec)
	}
	return out, nil
}

// reference is the commentary entries, one section per commentary, and the cross-references.
func (p passageReport) reference(ctx context.Context) ([]Section, error) {
	res, err := call[bible.ForPassageResult](ctx, p.s, p.env, "reference.for_passage", map[string]any{"ref": p.osis})
	if err != nil {
		return nil, err
	}
	var out []Section
	for _, c := range res.Commentaries {
		sec := Section{ID: gCommentary + ":" + c.Module, Group: gCommentary, Title: c.Name, Sources: []Source{p.mods.source(c.Module)}}
		var all []Entry
		for _, e := range c.Entries {
			end := e.End
			if end == 0 {
				end = e.ID
			}
			markup, _ := linkRefs(e.Markup)
			all = append(all, Entry{Heading: e.Ref, Ref: osisOf(verse.Range{Start: e.ID, End: end}), RefLabel: e.Ref, Text: e.Text, Markup: markup})
		}
		p.pg.page(&sec, all, 20)
		out = append(out, sec)
	}
	xs := Section{ID: gCrossRefs, Group: gCrossRefs, Title: "Cross-references"}
	seen := map[string]bool{}
	var all []Entry
	byFrom := map[verse.ID]int{}
	for _, x := range res.CrossRefs {
		for _, m := range x.Sources {
			if !seen[m] {
				seen[m] = true
				xs.Sources = append(xs.Sources, p.mods.source(m))
			}
		}
		i, ok := byFrom[x.From]
		if !ok {
			i = len(all)
			byFrom[x.From] = i
			all = append(all, Entry{Heading: x.FromRef, Ref: x.From.OSIS(), RefLabel: x.FromRef})
		}
		all[i].Links = append(all[i].Links, Link{Ref: x.To, Label: x.ToRef})
		all[i].Count++
	}
	p.pg.page(&xs, all, 50)
	return append(out, xs), nil
}

// words lists the original-language words of the passage from a tagged Bible, with glosses.
func (p passageReport) words(ctx context.Context) ([]Section, error) {
	var orig, other []string
	for _, x := range p.mods.ofKind(bible.KindBible) {
		if original(x) {
			orig = append(orig, x.ID)
		} else {
			other = append(other, x.ID)
		}
	}
	for _, ids := range [][]string{orig, other} {
		if len(ids) == 0 {
			continue
		}
		res, err := call[bible.ParallelResult](ctx, p.s, p.env, "bible.parallel", map[string]any{"modules": ids, "ref": p.osis, "markup": true})
		if err != nil {
			return nil, err
		}
		for _, col := range res.Columns {
			var all []Entry
			for _, v := range col.Verses {
				id := v.BaseID
				if id == 0 {
					id = v.ID
				}
				for _, w := range tagged(v.Markup) {
					all = append(all, Entry{Heading: w.text, Strong: w.main(), Gloss: w.gloss, Morph: w.morph, Ref: id.OSIS(), RefLabel: v.Ref})
				}
			}
			if len(all) == 0 {
				continue
			}
			src := p.mods.source(col.Module)
			title := "Original-language words"
			if !original(p.mods.byID[col.Module]) {
				title = "Words with Strong's numbers"
			}
			sec := Section{ID: gWords, Group: gWords, Title: title, Sources: []Source{src}}
			p.pg.page(&sec, all, 100)
			p.s.addLemmas(ctx, p.env, sec.Entries)
			return []Section{sec}, nil
		}
	}
	return nil, nil
}

// addLemmas fills in each word's dictionary form, and its gloss where the Bible gives none, from
// the first lexicon that has the number.
func (s *service) addLemmas(ctx context.Context, env *command.Env, es []Entry) {
	seen := map[string]bible.LookupEntry{}
	for i := range es {
		n := es[i].Strong
		e, ok := seen[n]
		if !ok {
			found, err := call[[]bible.LookupEntry](ctx, s, env, "lexicon.lookup", map[string]any{"strong": n, "limit": 1})
			if err == nil && len(found) > 0 {
				e = found[0]
			}
			seen[n] = e
		}
		es[i].Lemma, es[i].Translit = e.Lemma, e.Translit
		if es[i].Gloss == "" {
			es[i].Gloss = e.Gloss
		}
	}
}

// bookHits turns library search hits into entries.
func bookHits(hits []search.Hit) []Entry {
	out := make([]Entry, 0, len(hits))
	for _, h := range hits {
		e := Entry{Heading: h.Title, Snippet: h.Snippet, RefLabel: h.Cites, Book: &BookLink{ItemID: h.ItemID, FileID: h.FileID,
			Title: h.Title, Authors: h.Authors, Heading: h.Heading, Locator: h.Locator, Online: h.Online}}
		if r, err := verse.Parse(h.Cites); err == nil {
			e.Ref = osisOf(r)
		}
		out = append(out, e)
	}
	return out
}

// librarySection pages through a library search.
func librarySection(ctx context.Context, s *service, env *command.Env, pg *pager, id, title, cmd string, params map[string]any) ([]Section, error) {
	sec := Section{ID: id, Group: groupOf(id), Title: title, Sources: []Source{{Name: "Books in your library"}}}
	from, n := pg.start(id), pg.size(20)
	params["offset"], params["limit"] = from, min(n, 200)
	res, err := call[search.QueryResult](ctx, s, env, cmd, params)
	if err != nil {
		return nil, err
	}
	sec.Entries = bookHits(res.Hits)
	sec.Total = from + len(res.Hits)
	if res.More && from+len(res.Hits)+n <= 1000 {
		sec.More = cursor(id, from+len(res.Hits))
	}
	if len(res.Incomplete) > 0 {
		sec.Hint = "Some library folders are not fully indexed yet, so books may be missing."
	}
	return []Section{sec}, nil
}

func (p passageReport) citing(ctx context.Context) ([]Section, error) {
	return librarySection(ctx, p.s, p.env, p.pg, gCiting, "Books that cite it", "search.refs", map[string]any{"ref": p.osis})
}

func (p passageReport) notes(ctx context.Context) ([]Section, error) {
	ns, err := call[[]core.Note](ctx, p.s, p.env, "note.list", map[string]any{"ref": p.osis, "limit": maxLimit})
	if err != nil {
		return nil, err
	}
	sec := Section{ID: gNotes, Group: gNotes, Title: "Your notes", Sources: yours("Your notes")}
	p.pg.page(&sec, noteEntries(ns), 20)
	return []Section{sec}, nil
}

func noteEntries(ns []core.Note) []Entry {
	out := make([]Entry, 0, len(ns))
	for _, n := range ns {
		e := Entry{Heading: n.Title, NoteID: n.ID, Text: summary(n.Body, 240)}
		if e.Heading == "" {
			e.Heading = "Untitled"
		}
		if n.Ref != nil {
			e.Ref, e.RefLabel = n.Ref.OSIS, n.Ref.Display
		} else if n.ItemTitle != "" {
			e.RefLabel = "In " + n.ItemTitle
		}
		out = append(out, e)
	}
	return out
}

// summary is the start of Markdown text as plain text.
func summary(md string, n int) string {
	t := strings.Join(strings.Fields(strings.NewReplacer("#", "", "*", "", "`", "", ">", "").Replace(md)), " ")
	if r := []rune(t); len(r) > n {
		return strings.TrimSpace(string(r[:n])) + "…"
	}
	return t
}

func (p passageReport) highlights(ctx context.Context) ([]Section, error) {
	hs, err := call[[]reader.Highlight](ctx, p.s, p.env, "highlight.list", map[string]any{"ref": p.osis})
	if err != nil {
		return nil, err
	}
	sec := Section{ID: gHighlights, Group: gHighlights, Title: "Your highlights", Sources: yours("Your highlights")}
	var all []Entry
	for _, h := range hs {
		if h.Start == 0 {
			continue
		}
		r := verse.Range{Start: verse.ID(h.Start), End: verse.ID(h.End)}
		all = append(all, Entry{Heading: r.String(), Ref: osisOf(r), RefLabel: r.String(), Extra: []string{h.Style}})
	}
	p.pg.page(&sec, all, 20)
	return []Section{sec}, nil
}

func (p passageReport) lists(ctx context.Context) ([]Section, error) {
	ls, err := call[[]lists.List](ctx, p.s, p.env, "list.list", map[string]any{"kind": "verses"})
	if err != nil {
		return nil, err
	}
	sec := Section{ID: gLists, Group: gLists, Title: "In your lists", Sources: yours("Your lists")}
	var all []Entry
	for _, l := range ls {
		items, err := call[lists.Items](ctx, p.s, p.env, "list.items", map[string]any{"id": l.ID})
		if err != nil {
			return nil, err
		}
		for _, e := range items.Entries {
			if e.Ref != nil && p.r.Overlaps(verse.Range{Start: e.Ref.Start, End: e.Ref.End}) {
				all = append(all, Entry{Heading: l.Name, ListID: l.ID, Ref: e.Ref.OSIS, RefLabel: e.Ref.Display})
			}
		}
	}
	p.pg.page(&sec, all, 20)
	return []Section{sec}, nil
}

func (p passageReport) plans(ctx context.Context) ([]Section, error) {
	ps, err := call[[]plan.Plan](ctx, p.s, p.env, "plan.list", map[string]any{})
	if err != nil {
		return nil, err
	}
	sec := Section{ID: gPlans, Group: gPlans, Title: "In your reading plans", Sources: yours("Your reading plans")}
	var all []Entry
	for _, pl := range ps {
		days, err := call[[]plan.Day](ctx, p.s, p.env, "plan.days", map[string]any{"plan_id": pl.ID})
		if err != nil {
			return nil, err
		}
		for _, d := range days {
			for _, rd := range d.Readings {
				if rd.Start == 0 || !p.r.Overlaps(verse.Range{Start: rd.Start, End: max(rd.End, rd.Start)}) {
					continue
				}
				all = append(all, Entry{Heading: pl.Name, PlanID: pl.ID, Day: d.Day, Done: d.Done, Ref: rd.OSIS, RefLabel: rd.Ref,
					Extra: []string{fmt.Sprintf("Day %d", d.Day), d.Date}})
			}
		}
	}
	p.pg.page(&sec, all, 20)
	return []Section{sec}, nil
}
