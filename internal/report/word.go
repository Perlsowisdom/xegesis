package report

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/lists"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

type WordParams struct {
	Word string `json:"word" required:"true" desc:"A Strong's number (G25), an original-language word (ἀγαπάω) or an English word (love)"`
	Paging
}

// Section groups of a word report, in order.
const (
	gLexicon     = "lexicon"
	gOccurrences = "occurrences"
	gRenderings  = "renderings"
	gRelated     = "related"
	gUsed        = "books"
	gWordLists   = "lists"
)

func (s *service) word(ctx context.Context, env *command.Env, p WordParams) (Report, error) {
	q := strings.TrimSpace(p.Word)
	if q == "" {
		return Report{}, command.Errorf("bad_params", "word is required")
	}
	pg, err := newPager(p.Paging)
	if err != nil {
		return Report{}, err
	}
	mods, err := s.modules(ctx, env)
	if err != nil {
		return Report{}, err
	}
	w := wordReport{s: s, env: env, pg: pg, mods: mods, bibles: mods.bibles(p.Bibles), query: q}
	w.asked = len(w.bibles) > 0 && len(p.Bibles) > 0 && w.bibles[0] == p.Bibles[0]
	if err := w.resolve(ctx); err != nil {
		return Report{}, err
	}
	rep := Report{Kind: "word", Query: firstNonEmpty(w.strong, q), Strong: w.strong, Subject: w.subject(), Candidates: w.candidates}
	rep.Title = "Word report: " + rep.Subject
	rep.Sections, err = gather(ctx, pg, []part{
		{[]string{gLexicon}, "Lexicon entries", w.lexicon},
		{[]string{gOccurrences, gRenderings}, "Where it occurs", w.occurrences},
		{[]string{gRelated}, "Related words", w.related},
		{[]string{gUsed}, "Books that use it", w.books},
		{[]string{gWordLists}, "In your word lists", w.lists},
	})
	return rep, err
}

type wordReport struct {
	s      *service
	env    *command.Env
	pg     *pager
	mods   modules
	bibles []string
	query  string
	asked  bool // the Bibles to read were given

	strong     string
	head       bible.LookupEntry // the first lexicon entry, for the lemma
	candidates []Entry
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

func (w *wordReport) subject() string {
	return strings.TrimSpace(firstNonEmpty(w.strong+" "+w.head.Lemma, w.query))
}

// resolve finds the Strong's number: the word itself, a lexicon headword, or the number the
// tagged Bibles most often give an English word.
func (w *wordReport) resolve(ctx context.Context) error {
	if n := bible.NormStrong(w.query); n != "" {
		w.strong = n
	} else {
		found, err := call[[]bible.LookupEntry](ctx, w.s, w.env, "lexicon.lookup", map[string]any{"word": w.query, "limit": 3})
		if err != nil {
			return err
		}
		for _, e := range found {
			if e.Strong != "" {
				w.strong = e.Strong
				break
			}
		}
		if w.strong == "" && isLatin(w.query) {
			if err := w.english(ctx); err != nil {
				return err
			}
		}
	}
	if w.strong == "" {
		return nil
	}
	found, err := call[[]bible.LookupEntry](ctx, w.s, w.env, "lexicon.lookup", map[string]any{"strong": w.strong, "limit": 1})
	if err != nil {
		return err
	}
	if len(found) > 0 {
		w.head = found[0]
	}
	return nil
}

func isLatin(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) {
			return false
		}
	}
	return true
}

// english finds the Strong's numbers tagged on an English word in verses that contain it.
func (w *wordReport) english(ctx context.Context) error {
	var ids []string
	for _, x := range w.mods.ofKind(bible.KindBible) {
		if !original(x) {
			ids = append(ids, x.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	hits, err := call[[]bible.Hit](ctx, w.s, w.env, "bible.search", map[string]any{"query": w.query, "modules": ids, "limit": 30})
	if err != nil {
		return err
	}
	counts := map[string]int{}
	want := strings.ToLower(w.query)
	for _, h := range hits {
		p, err := call[bible.Passage](ctx, w.s, w.env, "bible.read", map[string]any{"module": h.Module, "ref": h.Ref, "markup": true})
		if err != nil {
			return err
		}
		for _, v := range p.Verses {
			for _, t := range tagged(v.Markup) {
				if containsWord(strings.ToLower(t.text), want) {
					counts[t.main()]++
				}
			}
		}
	}
	for n, c := range counts {
		w.candidates = append(w.candidates, Entry{Strong: n, Count: c})
	}
	sort.Slice(w.candidates, func(i, j int) bool {
		a, b := w.candidates[i], w.candidates[j]
		return a.Count > b.Count || a.Count == b.Count && a.Strong < b.Strong
	})
	if len(w.candidates) > 8 {
		w.candidates = w.candidates[:8]
	}
	w.s.addLemmas(ctx, w.env, w.candidates)
	for i := range w.candidates {
		w.candidates[i].Heading = strings.TrimSpace(w.candidates[i].Strong + " " + w.candidates[i].Lemma)
	}
	if len(w.candidates) > 0 {
		w.strong = w.candidates[0].Strong
	}
	return nil
}

func containsWord(text, word string) bool {
	for _, f := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }) {
		if f == word {
			return true
		}
	}
	return false
}

func (w *wordReport) lexicon(ctx context.Context) ([]Section, error) {
	params := map[string]any{"strong": w.strong, "limit": 5}
	if w.strong == "" {
		params = map[string]any{"word": w.query, "limit": 5}
	}
	found, err := call[[]bible.LookupEntry](ctx, w.s, w.env, "lexicon.lookup", params)
	if err != nil {
		return nil, err
	}
	return w.s.headwordSections(gLexicon, found, w.mods, w.pg), nil
}

// headwordSections makes one section per module of lexicon or dictionary entries.
func (s *service) headwordSections(group string, found []bible.LookupEntry, mods modules, pg *pager) []Section {
	var out []Section
	idx := map[string]int{}
	var all [][]Entry
	for _, e := range found {
		i, ok := idx[e.Module]
		if !ok {
			i = len(out)
			idx[e.Module] = i
			out = append(out, Section{ID: group + ":" + e.Module, Group: group, Title: e.Name, Sources: []Source{mods.source(e.Module)}})
			all = append(all, nil)
		}
		markup, links := linkRefs(e.Markup)
		heading := firstNonEmpty(e.Lemma, e.Key)
		if e.Lemma != "" && e.Key != "" && !strings.EqualFold(e.Key, e.Lemma) && bible.NormStrong(e.Key) == "" {
			heading += " (" + e.Key + ")"
		}
		all[i] = append(all[i], Entry{Heading: heading, Strong: e.Strong, Lemma: e.Lemma, Translit: e.Translit, Gloss: e.Gloss,
			Morph: e.Morph, Text: e.Text, Markup: markup, Links: links, Count: len(links), Topic: topicOf(e)})
	}
	for i := range out {
		pg.page(&out[i], all[i], 20)
	}
	return out
}

// topicOf is the headword of a dictionary article, to open as a topic report.
func topicOf(e bible.LookupEntry) string {
	if e.Kind != bible.KindDictionary {
		return ""
	}
	return strings.ToLower(e.Key)
}

// occurrences lists where the word occurs, with verses by book, and how each Bible renders it.
func (w *wordReport) occurrences(ctx context.Context) ([]Section, error) {
	if w.strong == "" {
		return nil, nil
	}
	res, err := call[bible.WordStudyResult](ctx, w.s, w.env, "word.study", map[string]any{"strong": w.strong, "limit": 100_000})
	if err != nil || len(res.Bibles) == 0 {
		return nil, err
	}
	// The verses come from an original-language Bible if one has the word, else the one with most.
	main := res.Bibles[0]
	for _, b := range res.Bibles {
		if o, mo := original(w.mods.byID[b.Module]), original(w.mods.byID[main.Module]); o && !mo || o == mo && b.Occurrences > main.Occurrences {
			main = b
		}
	}
	var out []Section
	if w.pg.wantsGroup(gOccurrences) {
		sec, err := w.verses(ctx, main)
		if err != nil {
			return nil, err
		}
		out = append(out, sec)
	}
	for _, b := range res.Bibles {
		title := "How " + b.Name + " translates it"
		if original(w.mods.byID[b.Module]) {
			title = "Forms in " + b.Name
		}
		sec := Section{ID: gRenderings + ":" + b.Module, Group: gRenderings, Title: title, Sources: []Source{w.mods.source(b.Module)}}
		var all []Entry
		for _, rd := range b.Renderings {
			e := Entry{Heading: rd.Text, Count: rd.Count}
			for _, ref := range rd.Refs[:min(len(rd.Refs), 10)] {
				if r, err := verse.Parse(ref); err == nil {
					e.Links = append(e.Links, Link{Ref: osisOf(r), Label: ref})
				}
			}
			all = append(all, e)
		}
		w.pg.page(&sec, all, 20)
		out = append(out, sec)
	}
	return out, nil
}

// verses is the occurrences section: a chart of verses by book and a page of the verses, read from
// the first Bible asked for or else the Bible the numbers came from.
func (w *wordReport) verses(ctx context.Context, b bible.WordStudyModule) (Section, error) {
	seen := map[int]bool{}
	var ids []int
	for _, rd := range b.Renderings {
		for _, id := range rd.IDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	sort.Ints(ids)
	sec := Section{ID: gOccurrences, Group: gOccurrences, Title: "Where it occurs", Sources: []Source{w.mods.source(b.Module)}}
	for _, id := range ids {
		book := verse.ID(id).Book()
		if n := len(sec.Chart); n > 0 && sec.Chart[n-1].Book == bookOSIS(book) {
			sec.Chart[n-1].Count++
			continue
		}
		bk, _ := verse.BookByNumber(book)
		sec.Chart = append(sec.Chart, Bar{Label: bk.Name, Book: bk.OSIS, Count: 1})
	}
	all := make([]Entry, len(ids))
	for i, id := range ids {
		all[i] = Entry{Ref: verse.ID(id).OSIS(), RefLabel: verse.ID(id).String(), Strong: w.strong}
	}
	w.pg.page(&sec, all, 20)
	display := b.Module
	if w.asked {
		display = w.bibles[0]
	}
	if display != b.Module {
		sec.Sources = append(sec.Sources, w.mods.source(display))
	}
	for i := range sec.Entries {
		res, err := call[bible.ParallelResult](ctx, w.s, w.env, "bible.parallel", map[string]any{"modules": []string{display}, "ref": sec.Entries[i].Ref, "markup": true})
		if err != nil {
			return sec, err
		}
		if len(res.Columns) > 0 && len(res.Columns[0].Verses) > 0 {
			v := res.Columns[0].Verses[0]
			sec.Entries[i].Text, sec.Entries[i].Markup, sec.Entries[i].Module = v.Text, v.Markup, display
		}
	}
	return sec, nil
}

func bookOSIS(n int) string {
	b, _ := verse.BookByNumber(n)
	return b.OSIS
}

// related lists the words the lexicon entries point to.
func (w *wordReport) related(ctx context.Context) ([]Section, error) {
	if w.strong == "" {
		return nil, nil
	}
	found, err := call[[]bible.LookupEntry](ctx, w.s, w.env, "lexicon.lookup", map[string]any{"strong": w.strong, "limit": 5})
	if err != nil {
		return nil, err
	}
	sec := Section{ID: gRelated, Group: gRelated, Title: "Related words"}
	seen := map[string]bool{}
	var all []Entry
	for _, e := range found {
		nums := relatedNumbers(e.Text, w.strong)
		if len(nums) > 0 && !seen[e.Module] {
			seen[e.Module] = true
			sec.Sources = append(sec.Sources, w.mods.source(e.Module))
		}
		for _, n := range nums {
			if !seen[n] {
				seen[n] = true
				all = append(all, Entry{Strong: n, Module: e.Module})
			}
		}
	}
	w.pg.page(&sec, all, 20)
	w.s.addLemmas(ctx, w.env, sec.Entries)
	for i := range sec.Entries {
		sec.Entries[i].Heading = strings.TrimSpace(sec.Entries[i].Strong + " " + sec.Entries[i].Lemma)
	}
	return []Section{sec}, nil
}

// books finds library passages that use the word: its lemma or transliteration, or the English word.
func (w *wordReport) books(ctx context.Context) ([]Section, error) {
	terms := []string{}
	for _, t := range []string{w.head.Lemma, plainTranslit(w.head.Translit)} {
		if t != "" && !strings.ContainsAny(t, " ") {
			terms = append(terms, t)
		}
	}
	if bible.NormStrong(w.query) == "" && !strings.EqualFold(w.query, w.head.Lemma) {
		terms = append(terms, w.query) // the English or original word that was asked for
	}
	if len(terms) == 0 {
		return nil, nil
	}
	return librarySection(ctx, w.s, w.env, w.pg, gUsed, "Books that use it", "search.query",
		map[string]any{"query": strings.Join(terms, " "), "mode": "any"})
}

// plainTranslit drops accents from a transliteration: agapaō becomes agapao.
func plainTranslit(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'ā', 'á', 'à':
			return 'a'
		case 'ē', 'é', 'è':
			return 'e'
		case 'ī', 'í':
			return 'i'
		case 'ō', 'ó':
			return 'o'
		case 'ū', 'ú':
			return 'u'
		}
		return r
	}, s)
}

func (w *wordReport) lists(ctx context.Context) ([]Section, error) {
	if w.strong == "" {
		return nil, nil
	}
	ls, err := call[[]lists.List](ctx, w.s, w.env, "list.list", map[string]any{"kind": "words"})
	if err != nil {
		return nil, err
	}
	sec := Section{ID: gWordLists, Group: gWordLists, Title: "In your word lists", Sources: yours("Your lists")}
	var all []Entry
	for _, l := range ls {
		items, err := call[lists.Items](ctx, w.s, w.env, "list.items", map[string]any{"id": l.ID})
		if err != nil {
			return nil, err
		}
		for _, e := range items.Entries {
			if bible.NormStrong(e.Strong) == w.strong {
				all = append(all, Entry{Heading: l.Name, ListID: l.ID, Strong: w.strong, Lemma: e.Lemma, Extra: []string{e.Word}})
			}
		}
	}
	w.pg.page(&sec, all, 20)
	return []Section{sec}, nil
}
