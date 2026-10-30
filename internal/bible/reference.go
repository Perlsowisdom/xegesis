package bible

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// registerReference adds the commands for commentaries, cross-references, lexicons and word studies.
func registerReference(r *command.Registry, c cmds) {
	r.Add(
		command.Define("reference.for_passage", "List every installed commentary entry and cross-reference for a passage. "+
			"Cross-references come from cross-reference sets and from the notes of installed Bibles.", false, c.forPassage),
		command.Define("lexicon.lookup", "Look up a Strong's number or an original-language word in the installed lexicons and dictionaries.", false, c.lookup),
		command.Define("word.study", "Find where a Strong's number occurs in the tagged Bibles, grouped by how each translation renders it.", false, c.wordStudy),
	)
}

// modulesOf lists installed modules of the given kinds, or the named ones.
func (c cmds) modulesOf(ids []string, kinds ...string) ([]*Module, error) {
	if len(ids) == 0 {
		all, err := c.l.List()
		if err != nil {
			return nil, err
		}
		for _, m := range all {
			for _, k := range kinds {
				if m.Kind == k {
					ids = append(ids, m.ID)
				}
			}
		}
	}
	var out []*Module
	for _, id := range ids {
		m, err := c.module(id)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

type ForPassageParams struct {
	Ref           string   `json:"ref" required:"true" desc:"A passage, e.g. \"John 3:16\" or \"Rom 8\""`
	Versification string   `json:"versification,omitempty" desc:"Numbering of ref: english (default) or hebrew"`
	Modules       []string `json:"modules,omitempty" desc:"Commentaries and cross-reference sets to use; all installed ones when omitted"`
	Notes         *bool    `json:"bible_notes,omitempty" desc:"Include cross-references from the notes of installed Bibles (default true)"`
}

// RefEntry is one commentary entry.
type RefEntry struct {
	ID     verse.ID `json:"id"`
	End    verse.ID `json:"end,omitempty"`
	Ref    string   `json:"ref"`
	Text   string   `json:"text"`
	Markup string   `json:"markup,omitempty"`
}

// Commentary is one installed work's entries for the passage.
type Commentary struct {
	Module      string     `json:"module"`
	Name        string     `json:"name"`
	Attribution string     `json:"attribution,omitempty"`
	Entries     []RefEntry `json:"entries"`
}

// CrossRef is one cross-reference: from a verse of the passage to another passage.
type CrossRef struct {
	From    verse.ID `json:"from" desc:"The verse of the passage, in the requested numbering"`
	FromRef string   `json:"from_ref"`
	To      string   `json:"to" desc:"OSIS reference of the target"`
	ToRef   string   `json:"to_ref"`
	Sources []string `json:"sources" desc:"Modules that give it"`
}

type ForPassageResult struct {
	Ref          string       `json:"ref"`
	Commentaries []Commentary `json:"commentaries"`
	CrossRefs    []CrossRef   `json:"cross_references"`
}

func (c cmds) forPassage(ctx context.Context, _ *command.Env, p ForPassageParams) (ForPassageResult, error) {
	r, err := parseRef(p.Ref)
	if err != nil {
		return ForPassageResult{}, err
	}
	base := system(p.Versification)
	out := ForPassageResult{Ref: r.String(), Commentaries: []Commentary{}, CrossRefs: []CrossRef{}}
	works, err := c.modulesOf(p.Modules, KindCommentary, KindCrossRef)
	if err != nil {
		return out, err
	}
	refs := crossRefs{byKey: map[string]int{}}
	for _, m := range works {
		vs, err := m.Read(ctx, m.ToModuleRange(ctx, r, base))
		if err != nil {
			return out, err
		}
		if m.Kind == KindCrossRef {
			for _, v := range vs {
				refs.add(m.FromModule(ctx, v.ID, base), v.Markup, m.ID, false)
			}
			continue
		}
		com := Commentary{Module: m.ID, Name: m.Name, Attribution: m.Licence.AttributionLine(), Entries: []RefEntry{}}
		for _, v := range vs {
			e := RefEntry{ID: v.ID, Ref: refString(v.ID, v.End), Text: v.Text, Markup: v.Markup}
			if v.End != v.ID {
				e.End = v.End
			}
			com.Entries = append(com.Entries, e)
		}
		if len(com.Entries) > 0 {
			out.Commentaries = append(out.Commentaries, com)
		}
	}
	if p.Notes == nil || *p.Notes {
		bibles, err := c.modulesOf(nil, KindBible)
		if err != nil {
			return out, err
		}
		for _, m := range bibles {
			vs, err := m.Read(ctx, m.ToModuleRange(ctx, r, base))
			if err != nil {
				return out, err
			}
			for _, v := range vs {
				refs.add(m.FromModule(ctx, v.ID, base), v.Markup, m.ID, true)
			}
		}
	}
	out.CrossRefs = refs.list()
	return out, nil
}

var (
	noteRe = regexp.MustCompile(`(?s)<x>(.*?)</x>`)
	refRe  = regexp.MustCompile(`<r o="([^"]+)">([^<]*)</r>`)
)

type crossRefs struct {
	byKey map[string]int
	items []CrossRef
}

// add collects the <r> targets of markup: all of them for a cross-reference set, those inside
// cross-reference notes for a Bible.
func (c *crossRefs) add(from verse.ID, markup, module string, notesOnly bool) {
	parts := []string{markup}
	if notesOnly {
		parts = nil
		for _, m := range noteRe.FindAllStringSubmatch(markup, -1) {
			parts = append(parts, m[1])
		}
	}
	for _, part := range parts {
		for _, m := range refRe.FindAllStringSubmatch(part, -1) {
			to := m[1]
			key := from.OSIS() + ">" + to
			if i, ok := c.byKey[key]; ok {
				if !contains(c.items[i].Sources, module) {
					c.items[i].Sources = append(c.items[i].Sources, module)
				}
				continue
			}
			toRef := strings.TrimSpace(unescape(m[2]))
			if id, err := osisVerse(to); err == nil && (toRef == "" || strings.ContainsAny(toRef, "<>")) {
				toRef = id.String()
			}
			c.byKey[key] = len(c.items)
			c.items = append(c.items, CrossRef{From: from, FromRef: from.String(), To: to, ToRef: toRef, Sources: []string{module}})
		}
	}
}

func (c *crossRefs) list() []CrossRef {
	out := append([]CrossRef{}, c.items...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return len(out[i].Sources) > len(out[j].Sources)
	})
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

type LookupParams struct {
	Strong  string   `json:"strong,omitempty" desc:"Strong's number, e.g. G26 or H7225"`
	Word    string   `json:"word,omitempty" desc:"An original-language word or a headword; accents and vowel points are ignored"`
	Modules []string `json:"modules,omitempty" desc:"Lexicons and dictionaries to use; all installed ones when omitted"`
	Limit   int      `json:"limit,omitempty" desc:"Most entries per module (default 20)"`
	Prefix  bool     `json:"prefix,omitempty" desc:"With word, also find headwords that begin with it, e.g. GRACE OF GOD for grace"`
}

type LookupEntry struct {
	Module      string `json:"module"`
	Kind        string `json:"kind" desc:"lexicon or dictionary"`
	Name        string `json:"name"`
	Attribution string `json:"attribution,omitempty"`
	Headword
}

func (c cmds) lookup(ctx context.Context, _ *command.Env, p LookupParams) ([]LookupEntry, error) {
	strong := ""
	if p.Strong != "" {
		if strong = normStrong(p.Strong); strong == "" {
			return nil, command.Errorf("bad_params", "strong must look like H7225 or G26")
		}
	} else if strings.TrimSpace(p.Word) == "" {
		return nil, command.Errorf("bad_params", "lexicon.lookup: strong or word is required")
	}
	if p.Limit <= 0 {
		p.Limit = 20
	}
	mods, err := c.modulesOf(p.Modules, KindLexicon, KindDictionary)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(mods, func(i, j int) bool { return mods[i].Kind == KindLexicon && mods[j].Kind != KindLexicon })
	out := []LookupEntry{}
	for _, m := range mods {
		if strong != "" && m.Kind != KindLexicon {
			continue
		}
		var hs []Headword
		if p.Prefix && strong == "" {
			hs, err = m.LookupPrefix(ctx, p.Word, p.Limit)
		} else {
			hs, err = m.Lookup(ctx, strong, p.Word, p.Limit)
		}
		if err != nil {
			return nil, command.Errorf("failed", "%s: %v", m.ID, err)
		}
		for _, h := range hs {
			out = append(out, LookupEntry{Module: m.ID, Kind: m.Kind, Name: m.Name, Attribution: m.Licence.AttributionLine(), Headword: h})
		}
	}
	for i := range out {
		if err := c.followLink(ctx, mods, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// followLink resolves an entry that only points to a headword ("@LINK ἀγαπάω"). Some SWORD lexicons
// are just such an index from Strong's numbers into a companion lexicon, so the headword is looked
// up in every installed lexicon and dictionary. Without a match the entry reads "See ἀγαπάω".
func (c cmds) followLink(ctx context.Context, mods []*Module, e *LookupEntry) error {
	target, ok := strings.CutPrefix(strings.TrimSpace(e.Text), "@LINK")
	if !ok {
		return nil
	}
	target = strings.TrimSpace(target)
	for _, m := range mods {
		hs, err := m.Lookup(ctx, "", target, 1)
		if err != nil {
			return command.Errorf("failed", "%s: %v", m.ID, err)
		}
		if len(hs) == 1 && !strings.HasPrefix(strings.TrimSpace(hs[0].Text), "@LINK") {
			strong := e.Strong
			*e = LookupEntry{Module: m.ID, Kind: m.Kind, Name: m.Name, Attribution: m.Licence.AttributionLine(), Headword: hs[0]}
			e.Strong = strong
			return nil
		}
	}
	e.Lemma, e.Text, e.Markup = target, "See "+target+".", "See "+target+"."
	return nil
}

type WordStudyParams struct {
	Strong        string   `json:"strong" required:"true" desc:"Strong's number, e.g. G26"`
	Modules       []string `json:"modules,omitempty" desc:"Bibles to search; all installed Bibles with Strong's numbers when omitted"`
	Ref           string   `json:"ref,omitempty" desc:"Only verses in this passage, e.g. \"John\""`
	Versification string   `json:"versification,omitempty" desc:"Numbering of ref: english (default) or hebrew"`
	Limit         int      `json:"limit,omitempty" desc:"Most verses listed per rendering (default 50)"`
}

// Rendering is one way a translation renders the word, with where it does.
type Rendering struct {
	Text  string   `json:"text" desc:"The words tagged with the number, lower-cased"`
	Count int      `json:"count"`
	Refs  []string `json:"refs" desc:"References of the first verses, in the requested numbering"`
	IDs   []int    `json:"ids"`
}

type WordStudyModule struct {
	Module      string      `json:"module"`
	Name        string      `json:"name"`
	Occurrences int         `json:"occurrences" desc:"Tagged words with the number"`
	Verses      int         `json:"verses"`
	Renderings  []Rendering `json:"renderings" desc:"Most frequent first"`
}

type WordStudyResult struct {
	Strong      string            `json:"strong"`
	Occurrences int               `json:"occurrences"`
	Bibles      []WordStudyModule `json:"bibles"`
}

var wordRe = regexp.MustCompile(`(?s)<w s="([^"]*)"[^>]*>(.*?)</w>`)

func (c cmds) wordStudy(ctx context.Context, _ *command.Env, p WordStudyParams) (WordStudyResult, error) {
	n := normStrong(p.Strong)
	if n == "" {
		return WordStudyResult{}, command.Errorf("bad_params", "strong must look like H7225 or G26")
	}
	if p.Limit <= 0 {
		p.Limit = 50
	}
	r := verse.Range{Start: 0, End: 999_999_999}
	if p.Ref != "" {
		var err error
		if r, err = parseRef(p.Ref); err != nil {
			return WordStudyResult{}, err
		}
	}
	base := system(p.Versification)
	mods, err := c.modulesOf(p.Modules, KindBible)
	if err != nil {
		return WordStudyResult{}, err
	}
	out := WordStudyResult{Strong: n, Bibles: []WordStudyModule{}}
	for _, m := range mods {
		scope := r
		if p.Ref != "" {
			scope = m.ToModuleRange(ctx, r, base)
		}
		ws, err := studyModule(ctx, m, n, scope, base, p.Limit)
		if err != nil {
			return out, command.Errorf("failed", "%s: %v", m.ID, err)
		}
		if ws.Verses > 0 {
			out.Occurrences += ws.Occurrences
			out.Bibles = append(out.Bibles, ws)
		}
	}
	return out, nil
}

func studyModule(ctx context.Context, m *Module, number string, r verse.Range, base verse.System, limit int) (WordStudyModule, error) {
	ws := WordStudyModule{Module: m.ID, Name: m.Name, Renderings: []Rendering{}}
	rows, err := m.db.QueryContext(ctx, `SELECT v.id, v.markup FROM strongs s JOIN verses v ON v.id = s.verse_id
		WHERE (s.number = ? OR s.number >= ? AND s.number < ?) AND v.id <= ? AND v.end_id >= ? ORDER BY v.id`,
		number, number+"A", number+"[", int64(r.End), int64(r.Start))
	if err != nil {
		return ws, err
	}
	defer rows.Close()
	byText := map[string]*Rendering{}
	last := verse.ID(-1)
	for rows.Next() {
		var id verse.ID
		var markup string
		if err := rows.Scan(&id, &markup); err != nil {
			return ws, err
		}
		if id == last {
			continue
		}
		last = id
		ws.Verses++
		for _, w := range wordRe.FindAllStringSubmatch(markup, -1) {
			if !tagged(w[1], number) {
				continue
			}
			text := strings.ToLower(strings.TrimSpace(unescape(stripTags(w[2]))))
			if text == "" {
				text = "(untranslated)"
			}
			ws.Occurrences++
			rd := byText[text]
			if rd == nil {
				rd = &Rendering{Text: text, Refs: []string{}, IDs: []int{}}
				byText[text] = rd
			}
			rd.Count++
			if len(rd.Refs) < limit && (len(rd.IDs) == 0 || rd.IDs[len(rd.IDs)-1] != int(id)) {
				e := m.FromModule(ctx, id, base)
				rd.Refs = append(rd.Refs, e.String())
				rd.IDs = append(rd.IDs, int(e))
			}
		}
	}
	if err := rows.Err(); err != nil {
		return ws, err
	}
	for _, rd := range byText {
		ws.Renderings = append(ws.Renderings, *rd)
	}
	sort.Slice(ws.Renderings, func(i, j int) bool {
		if ws.Renderings[i].Count != ws.Renderings[j].Count {
			return ws.Renderings[i].Count > ws.Renderings[j].Count
		}
		return ws.Renderings[i].Text < ws.Renderings[j].Text
	})
	return ws, nil
}

// tagged reports whether a word's s attribute holds the number (or the number with a letter suffix).
func tagged(attr, number string) bool {
	for _, s := range strings.Fields(attr) {
		if s == number || baseStrong(s) == number {
			return true
		}
	}
	return false
}

var unescape = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`).Replace
