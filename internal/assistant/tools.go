package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
)

// Runner runs a registered command with params and decodes its result into out.
type Runner func(ctx context.Context, id string, params, out any) error

// RegistryRunner runs commands through a registry.
func RegistryRunner(reg *command.Registry, env *command.Env) Runner {
	return func(ctx context.Context, id string, params, out any) error {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		res, err := reg.Run(ctx, env, id, raw)
		if err != nil {
			return err
		}
		if out == nil {
			return nil
		}
		b, err := json.Marshal(res)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, out)
	}
}

// Deps is what a tool works with.
type Deps struct {
	Run  Runner
	Gate Gate
	// Bible is the Bible open in the Reader, used when the model names none.
	Bible string
	mu    sync.Mutex
	items map[string]licence.Licence
}

// Result is a tool's answer: what the model reads, and a line for the person.
type Result struct {
	Content string `json:"content" desc:"Exactly what is sent to the model"`
	Label   string `json:"label" desc:"The tool call in words, e.g. \"Read Romans 8 (NET)\""`
	Notice  string `json:"notice,omitempty" desc:"What the licence gate did"`
}

// Tool is one read-only tool. The list (Tools) is the only place tools are defined, so the MCP
// assistant mode (T36b) offers the same ones.
type Tool struct {
	Def ToolDef
	Run func(ctx context.Context, d *Deps, args json.RawMessage) (Result, error)
}

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func num(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }

// maxResult bounds any one result's characters, so one call cannot fill the model's context.
const maxResult = 12000

// Tools lists the assistant's tools. None of them writes.
func Tools() []Tool {
	return []Tool{
		{ToolDef{"search_library", "Search the text of the books in the user's library. Returns matching places with a short snippet.",
			obj(map[string]any{"query": str("Words to find"), "limit": num("Most results, up to 10 (default 8)")}, "query")}, searchLibrary},
		{ToolDef{"read_passage", "Read a Bible passage in one of the user's Bibles. Long passages may be shortened by the Bible's licence.",
			obj(map[string]any{"ref": str("The passage, e.g. \"Romans 8:28-30\""), "bible": str("A Bible's abbreviation or ID; the open Bible when omitted")}, "ref")}, readPassage},
		{ToolDef{"list_bibles", "List the user's Bibles with their abbreviations and licences.", obj(map[string]any{})}, listBibles},
		{ToolDef{"read_book", "Read one section (an EPUB chapter or a PDF page) of a library book, as found by search_library.",
			obj(map[string]any{"item_id": str("The book's item_id"), "section": num("The section number (default 1)")}, "item_id")}, readBook},
		{ToolDef{"list_notes", "List the user's notes, optionally only those on a passage or with certain words.",
			obj(map[string]any{"ref": str("Only notes on this passage"), "query": str("Only notes with these words")})}, listNotes},
		{ToolDef{"read_note", "Read one of the user's notes.", obj(map[string]any{"id": str("The note's id")}, "id")}, readNote},
		{ToolDef{"list_sermons", "List the user's sermons, lessons and studies, optionally only those with certain words.",
			obj(map[string]any{"query": str("Only those with these words")})}, listSermons},
		{ToolDef{"read_sermon", "Read one sermon, lesson or study.", obj(map[string]any{"id": str("Its id")}, "id")}, readSermon},
		{ToolDef{"list_highlights", "List the user's highlights, optionally only those on a passage.",
			obj(map[string]any{"ref": str("Only highlights on this passage"), "query": str("Only highlights with these words")})}, listHighlights},
		{ToolDef{"passage_report", "Gather what the library holds about a passage: cross-references, commentaries, notes, original-language words.",
			obj(map[string]any{"ref": str("The passage")}, "ref")}, passageReport},
		{ToolDef{"word_report", "Gather what the library holds about a word: a Strong's number, an original-language word or an English word.",
			obj(map[string]any{"word": str("e.g. G26, ἀγάπη or love")}, "word")}, wordReport},
		{ToolDef{"look_up_word", "Look up a Strong's number or an original-language word in the user's lexicons and dictionaries.",
			obj(map[string]any{"word": str("e.g. G26 or ἀγάπη")}, "word")}, lookUpWord},
	}
}

// ToolDefs is the tool list as the model sees it.
func ToolDefs() []ToolDef {
	ts := Tools()
	out := make([]ToolDef, len(ts))
	for i, t := range ts {
		out[i] = t.Def
	}
	return out
}

// RunTool runs a tool by name. A failure becomes a result the model can read.
func RunTool(ctx context.Context, d *Deps, name, args string) Result {
	for _, t := range Tools() {
		if t.Def.Name != name {
			continue
		}
		raw := json.RawMessage(args)
		if strings.TrimSpace(args) == "" {
			raw = json.RawMessage("{}")
		}
		r, err := t.Run(ctx, d, raw)
		if err != nil {
			return Result{Content: "Error: " + err.Error(), Label: fmt.Sprintf("%s failed: %v", plainName(name), err)}
		}
		if r.Notice != "" && !strings.Contains(r.Content, r.Notice) {
			r.Content += "\n\n[" + r.Notice + "]"
		}
		return r
	}
	return Result{Content: "Error: no tool " + name, Label: "Asked for an unknown tool " + name}
}

func plainName(n string) string {
	return strings.ToUpper(n[:1]) + strings.ReplaceAll(n[1:], "_", " ")
}

func args[T any](raw json.RawMessage) (T, error) {
	var a T
	if err := json.Unmarshal(raw, &a); err != nil {
		return a, fmt.Errorf("the arguments are not valid JSON: %v", err)
	}
	return a, nil
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	s, short := Budget(string(b), maxResult)
	if short {
		s += "\n[The list was cut short; ask for less.]"
	}
	return s
}

var tagRE = regexp.MustCompile(`<[^>]*>`)

// plain strips markup from HTML or a snippet.
func plain(s string) string {
	s = strings.NewReplacer("</p>", "\n", "<br>", "\n", "<br/>", "\n", "</h1>", "\n", "</h2>", "\n", "</h3>", "\n", "</li>", "\n").Replace(s)
	s = html.UnescapeString(tagRE.ReplaceAllString(s, ""))
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// itemLicence is a library book's licence: the one decided, or the cautious default.
func (d *Deps) itemLicence(ctx context.Context, id string) (licence.Licence, string, error) {
	var it struct {
		Title     string           `json:"title"`
		Licence   *licence.Licence `json:"licence"`
		Suggested *licence.Licence `json:"suggested_licence"`
	}
	d.mu.Lock()
	l, ok := d.items[id]
	d.mu.Unlock()
	if ok {
		return l, "", nil
	}
	if err := d.Run(ctx, "library.item", map[string]any{"id": id}, &it); err != nil {
		return licence.Licence{}, "", err
	}
	if it.Licence != nil {
		l = *it.Licence
	} else {
		l, _ = licence.Defaults(licence.Unknown, "")
	}
	d.mu.Lock()
	if d.items == nil {
		d.items = map[string]licence.Licence{}
	}
	d.items[id] = l
	d.mu.Unlock()
	return l, it.Title, nil
}

type moduleMeta struct {
	ID           string          `json:"id"`
	Kind         string          `json:"kind"`
	Name         string          `json:"name"`
	Abbreviation string          `json:"abbreviation"`
	Licence      licence.Licence `json:"licence"`
}

func (m moduleMeta) short() string {
	if m.Abbreviation != "" {
		return m.Abbreviation
	}
	if m.Name != "" {
		return m.Name
	}
	return m.ID
}

func (d *Deps) modules(ctx context.Context, kind string) ([]moduleMeta, error) {
	var ms []moduleMeta
	err := d.Run(ctx, "bible.modules", map[string]any{"kind": kind}, &ms)
	return ms, err
}

func searchLibrary(ctx context.Context, d *Deps, raw json.RawMessage) (Result, error) {
	a, err := args[struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}](raw)
	if err != nil {
		return Result{}, err
	}
	if a.Limit <= 0 || a.Limit > 10 {
		a.Limit = 8
	}
	var res struct {
		Hits []struct {
			ItemID  string         `json:"item_id"`
			FileID  string         `json:"file_id"`
			Title   string         `json:"title"`
			Authors []string       `json:"authors"`
			Heading string         `json:"heading"`
			Locator map[string]any `json:"locator"`
			Snippet string         `json:"snippet"`
		} `json:"hits"`
	}
	if err := d.Run(ctx, "search.query", map[string]any{"query": a.Query, "limit": a.Limit}, &res); err != nil {
		return Result{}, err
	}
	type hit struct {
		ItemID  string         `json:"item_id"`
		Title   string         `json:"title"`
		Authors []string       `json:"authors,omitempty"`
		Heading string         `json:"heading,omitempty"`
		Where   map[string]any `json:"where,omitempty"`
		Snippet string         `json:"snippet,omitempty"`
		Notice  string         `json:"notice,omitempty"`
	}
	out := []hit{}
	withheld := 0
	for _, h := range res.Hits {
		l, _, err := d.itemLicence(ctx, h.ItemID)
		if err != nil {
			return Result{}, err
		}
		g := d.Gate.Book(h.Title, l, plain(h.Snippet))
		if g.Refused {
			withheld++
		}
		out = append(out, hit{ItemID: h.ItemID, Title: h.Title, Authors: h.Authors, Heading: h.Heading, Where: h.Locator, Snippet: g.Text, Notice: g.Notice})
	}
	label := fmt.Sprintf("Searched the library for %q: %d found", a.Query, len(out))
	if withheld > 0 {
		label += fmt.Sprintf(" (text of %d personal copies not sent)", withheld)
	}
	return Result{Content: toJSON(out), Label: label}, nil
}

func listBibles(ctx context.Context, d *Deps, _ json.RawMessage) (Result, error) {
	ms, err := d.modules(ctx, "bible")
	if err != nil {
		return Result{}, err
	}
	type bible struct {
		ID, Abbreviation, Name, Licence string
	}
	out := []bible{}
	for _, m := range ms {
		out = append(out, bible{m.ID, m.Abbreviation, m.Name, m.Licence.Plain()})
	}
	return Result{Content: toJSON(out), Label: fmt.Sprintf("Listed your Bibles: %d", len(out))}, nil
}

func (d *Deps) pickBible(ms []moduleMeta, want string) (moduleMeta, bool) {
	for _, w := range []string{want, d.Bible} {
		if w == "" {
			continue
		}
		for _, m := range ms {
			if strings.EqualFold(m.ID, w) || strings.EqualFold(m.Abbreviation, w) {
				return m, true
			}
		}
		if w == want {
			return moduleMeta{}, false
		}
	}
	for _, m := range ms {
		if m.Licence.Store {
			return m, true
		}
	}
	return moduleMeta{}, len(ms) > 0
}

func readPassage(ctx context.Context, d *Deps, raw json.RawMessage) (Result, error) {
	a, err := args[struct {
		Ref   string `json:"ref"`
		Bible string `json:"bible"`
	}](raw)
	if err != nil {
		return Result{}, err
	}
	ms, err := d.modules(ctx, "bible")
	if err != nil {
		return Result{}, err
	}
	m, ok := d.pickBible(ms, a.Bible)
	if !ok {
		if a.Bible != "" {
			return Result{}, fmt.Errorf("no Bible %q; list_bibles lists them", a.Bible)
		}
		return Result{}, fmt.Errorf("no Bibles are installed")
	}
	var pas struct {
		Ref    string `json:"ref"`
		Verses []struct {
			ID   int64  `json:"id"`
			End  int64  `json:"end"`
			Text string `json:"text"`
		} `json:"verses"`
	}
	if err := d.Run(ctx, "bible.read", map[string]any{"module": m.ID, "ref": a.Ref}, &pas); err != nil {
		return Result{}, err
	}
	where := pas.Ref
	if where == "" {
		where = a.Ref
	}
	if len(pas.Verses) == 0 {
		return Result{Content: fmt.Sprintf("%s has no verses for %s.", m.short(), where), Label: fmt.Sprintf("Read %s (%s): no verses", where, m.short())}, nil
	}
	var b strings.Builder
	for _, v := range pas.Verses {
		num := fmt.Sprint(v.ID % 1000)
		if v.End != 0 && v.End != v.ID {
			num += fmt.Sprintf("-%d", v.End%1000)
		}
		fmt.Fprintf(&b, "%s %s\n", num, strings.Join(strings.Fields(v.Text), " "))
	}
	g := d.Gate.Work(m.short(), m.Licence, b.String())
	head := fmt.Sprintf("%s (%s)", where, m.short())
	if a := m.Licence.AttributionLine(); a != "" && !g.Refused {
		head += ". " + a
	}
	label := fmt.Sprintf("Read %s (%s)", where, m.short())
	switch {
	case g.Refused:
		label = fmt.Sprintf("Read %s (%s, not sent: its licence does not allow it)", where, m.short())
	case g.Shortened:
		label = fmt.Sprintf("Read %s (%s, shortened to the licence limit)", where, m.short())
	}
	return Result{Content: head + "\n" + g.Text, Label: label, Notice: g.Notice}, nil
}

func readBook(ctx context.Context, d *Deps, raw json.RawMessage) (Result, error) {
	a, err := args[struct {
		ItemID  string `json:"item_id"`
		Section int    `json:"section"`
	}](raw)
	if err != nil {
		return Result{}, err
	}
	if a.Section < 1 {
		a.Section = 1
	}
	l, title, err := d.itemLicence(ctx, a.ItemID)
	if err != nil {
		return Result{}, err
	}
	if title == "" {
		title = a.ItemID
	}
	// A refusal is decided before the text is read.
	if g := d.Gate.Book(title, l, "x"); g.Refused {
		return Result{Content: g.Notice, Label: fmt.Sprintf("Read %q (not sent: a personal copy goes only to a model on this computer)", title), Notice: g.Notice}, nil
	}
	var sec struct {
		Sections int    `json:"sections"`
		Title    string `json:"title"`
		HTML     string `json:"html"`
	}
	if err := d.Run(ctx, "book.section", map[string]any{"item_id": a.ItemID, "section": a.Section}, &sec); err != nil {
		return Result{}, err
	}
	g := d.Gate.Book(title, l, plain(sec.HTML))
	text, short := Budget(g.Text, maxResult)
	head := fmt.Sprintf("%s, section %d of %d", title, a.Section, sec.Sections)
	if sec.Title != "" {
		head += ": " + sec.Title
	}
	label := fmt.Sprintf("Read %q, section %d", title, a.Section)
	if g.Shortened {
		label += " (shortened to the licence limit)"
	} else if short {
		label += " (shortened)"
	}
	return Result{Content: head + "\n" + text, Label: label, Notice: g.Notice}, nil
}

type noteRow struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	Ref   string `json:"ref,omitempty"`
	Start string `json:"start,omitempty"`
}

type noteJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Ref   *struct {
		Display string `json:"display"`
	} `json:"ref"`
	ItemTitle string `json:"item_title"`
}

func (n noteJSON) row() noteRow {
	r := noteRow{ID: n.ID, Title: n.Title}
	if n.Ref != nil {
		r.Ref = n.Ref.Display
	}
	r.Start, _ = Budget(strings.Join(strings.Fields(n.Body), " "), 160)
	return r
}

func listNotes(ctx context.Context, d *Deps, raw json.RawMessage) (Result, error) {
	a, err := args[struct {
		Ref   string `json:"ref"`
		Query string `json:"query"`
	}](raw)
	if err != nil {
		return Result{}, err
	}
	var notes []noteJSON
	label := "Listed your notes"
	if a.Ref != "" {
		err = d.Run(ctx, "note.list", map[string]any{"ref": a.Ref, "limit": 30}, &notes)
		label = "Listed your notes on " + a.Ref
	} else {
		p := map[string]any{"limit": 30}
		if a.Query != "" {
			p["query"] = a.Query
			label = fmt.Sprintf("Searched your notes for %q", a.Query)
		}
		err = d.Run(ctx, "note.search", p, &notes)
	}
	if err != nil {
		return Result{}, err
	}
	rows := make([]noteRow, len(notes))
	for i, n := range notes {
		rows[i] = n.row()
	}
	return Result{Content: toJSON(rows), Label: fmt.Sprintf("%s: %d", label, len(rows))}, nil
}

func readNote(ctx context.Context, d *Deps, raw json.RawMessage) (Result, error) {
	a, err := args[struct {
		ID string `json:"id"`
	}](raw)
	if err != nil {
		return Result{}, err
	}
	var n noteJSON
	if err := d.Run(ctx, "note.get", map[string]any{"id": a.ID}, &n); err != nil {
		return Result{}, err
	}
	body, short := Budget(n.Body, maxResult)
	title := n.Title
	if title == "" {
		title = "Untitled note"
	}
	head := "Note: " + title
	if n.Ref != nil {
		head += " (on " + n.Ref.Display + ")"
	}
	label := fmt.Sprintf("Read your note %q", title)
	if short {
		label += " (shortened)"
	}
	return Result{Content: head + "\n" + body, Label: label}, nil
}

func listSermons(ctx context.Context, d *Deps, raw json.RawMessage) (Result, error) {
	a, err := args[struct {
		Query string `json:"query"`
	}](raw)
	if err != nil {
		return Result{}, err
	}
	var docs []struct {
		ID      string `json:"id"`
		Kind    string `json:"kind"`
		Title   string `json:"title"`
		Passage *struct {
			Display string `json:"display"`
		} `json:"passage"`
		Passages []struct {
			Display string `json:"display"`
		} `json:"passages"`
	}
	p := map[string]any{"limit": 30}
	if a.Query != "" {
		p["query"] = a.Query
	}
	if err := d.Run(ctx, "document.list", p, &docs); err != nil {
		return Result{}, err
	}
	type row struct {
		ID, Kind, Title string
		Passage         string `json:",omitempty"`
	}
	rows := make([]row, len(docs))
	for i, x := range docs {
		rows[i] = row{ID: x.ID, Kind: x.Kind, Title: x.Title}
		rows[i].Passage = passagesText(x.Passages, x.Passage)
	}
	return Result{Content: toJSON(rows), Label: fmt.Sprintf("Listed your sermons and studies: %d", len(rows))}, nil
}

func readSermon(ctx context.Context, d *Deps, raw json.RawMessage) (Result, error) {
	a, err := args[struct {
		ID string `json:"id"`
	}](raw)
	if err != nil {
		return Result{}, err
	}
	var doc struct {
		Kind    string `json:"kind"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		Passage *struct {
			Display string `json:"display"`
		} `json:"passage"`
		Passages []struct {
			Display string `json:"display"`
		} `json:"passages"`
	}
	if err := d.Run(ctx, "document.get", map[string]any{"id": a.ID}, &doc); err != nil {
		return Result{}, err
	}
	body, short := Budget(doc.Body, maxResult)
	head := fmt.Sprintf("%s: %s", doc.Kind, doc.Title)
	if p := passagesText(doc.Passages, doc.Passage); p != "" {
		head += " (" + p + ")"
	}
	label := fmt.Sprintf("Read your %s %q", doc.Kind, doc.Title)
	if short {
		label += " (shortened)"
	}
	return Result{Content: head + "\n" + body, Label: label}, nil
}

func listHighlights(ctx context.Context, d *Deps, raw json.RawMessage) (Result, error) {
	a, err := args[struct {
		Ref   string `json:"ref"`
		Query string `json:"query"`
	}](raw)
	if err != nil {
		return Result{}, err
	}
	p := map[string]any{"limit": 50}
	if a.Ref != "" {
		p["ref"] = a.Ref
	}
	if a.Query != "" {
		p["query"] = a.Query
	}
	var hls []struct {
		ItemID    string `json:"item_id"`
		ItemTitle string `json:"item_title"`
		Ref       string `json:"ref"`
		Style     string `json:"style"`
		Text      string `json:"text"`
		Module    string `json:"module"`
	}
	if err := d.Run(ctx, "highlight.list", p, &hls); err != nil {
		return Result{}, err
	}
	bibles, err := d.modules(ctx, "bible")
	if err != nil {
		return Result{}, err
	}
	type row struct {
		Where  string `json:"where"`
		Style  string `json:"style"`
		Text   string `json:"text,omitempty"`
		Notice string `json:"notice,omitempty"`
	}
	rows := make([]row, 0, len(hls))
	withheld := 0
	for _, h := range hls {
		r := row{Style: h.Style}
		var g Gated
		if h.ItemID != "" {
			l, _, err := d.itemLicence(ctx, h.ItemID)
			if err != nil {
				return Result{}, err
			}
			r.Where = h.ItemTitle
			g = d.Gate.Book(h.ItemTitle, l, h.Text)
		} else {
			r.Where = h.Ref
			m, _ := d.pickBible(bibles, h.Module)
			g = d.Gate.Work(m.short(), m.Licence, h.Text)
		}
		if g.Refused {
			withheld++
		}
		r.Text, r.Notice = g.Text, g.Notice
		rows = append(rows, r)
	}
	label := fmt.Sprintf("Listed your highlights: %d", len(rows))
	if withheld > 0 {
		label += fmt.Sprintf(" (words of %d not sent)", withheld)
	}
	return Result{Content: toJSON(rows), Label: label}, nil
}

// report runs a report command and gates each section's text by its sources' licences.
func report(ctx context.Context, d *Deps, id string, params map[string]any, what string) (Result, error) {
	var rep struct {
		Title    string `json:"title"`
		Sections []struct {
			Title   string `json:"title"`
			Group   string `json:"group"`
			Sources []struct {
				Module  string `json:"module"`
				Name    string `json:"name"`
				Licence string `json:"licence"`
			} `json:"sources"`
			Entries []struct {
				Heading  string   `json:"heading,omitempty"`
				Text     string   `json:"text,omitempty"`
				Snippet  string   `json:"snippet,omitempty"`
				RefLabel string   `json:"ref_label,omitempty"`
				Strong   string   `json:"strong,omitempty"`
				Lemma    string   `json:"lemma,omitempty"`
				Gloss    string   `json:"gloss,omitempty"`
				Count    int      `json:"count,omitempty"`
				Module   string   `json:"module,omitempty"`
				Extra    []string `json:"extra,omitempty"`
				Book     *struct {
					ItemID string `json:"item_id"`
					Title  string `json:"title"`
				} `json:"book,omitempty"`
			} `json:"entries"`
		} `json:"sections"`
	}
	if err := d.Run(ctx, id, params, &rep); err != nil {
		return Result{}, err
	}
	mods, err := d.modules(ctx, "all")
	if err != nil {
		return Result{}, err
	}
	byID := map[string]moduleMeta{}
	for _, m := range mods {
		byID[m.ID] = m
	}
	var b strings.Builder
	b.WriteString(rep.Title + "\n")
	limited := 0
	for _, s := range rep.Sections {
		fmt.Fprintf(&b, "\n## %s\n", s.Title)
		used := map[string]int{} // characters sent per work in this section
		for _, e := range s.Entries {
			line := strings.TrimSpace(strings.Join(nonEmpty(e.Heading, e.RefLabel, e.Strong, e.Lemma, e.Gloss), " · "))
			if e.Count > 0 {
				line += fmt.Sprintf(" (%d)", e.Count)
			}
			if len(e.Extra) > 0 {
				line += " — " + strings.Join(e.Extra, ", ")
			}
			text := plain(firstNonEmpty(e.Text, e.Snippet))
			if text != "" {
				var g Gated
				key := e.Module
				switch {
				case e.Book != nil:
					key = "book:" + e.Book.ItemID
					l, _, err := d.itemLicence(ctx, e.Book.ItemID)
					if err != nil {
						return Result{}, err
					}
					g = d.Gate.Book(e.Book.Title, l, text)
				case s.Group == "notes":
					g = Gated{Text: text}
				default:
					m := byID[firstNonEmpty(e.Module, sourceModule(s.Sources))]
					if m.ID == "" { // not from a module: the user's own lists and plans
						g = Gated{Text: text}
						break
					}
					key = m.ID
					// The quoting limit holds for all of one work's entries in a section together.
					if max := QuoteLimit(m.Licence); max > 0 && m.Licence.Store {
						left := max - used[key]
						if left <= 0 {
							g = Gated{Shortened: true, Notice: fmt.Sprintf("Left out: the quoting limit of %s's licence is used up.", m.short())}
							break
						}
						g = limitTo(m.short(), text, left)
						used[key] += len([]rune(g.Text))
					} else {
						g = d.Gate.Work(m.short(), m.Licence, text)
					}
				}
				if g.Refused || g.Shortened {
					limited++
				}
				if g.Text != "" {
					line += "\n" + g.Text
				}
				if g.Notice != "" {
					line += "\n[" + g.Notice + "]"
				}
			}
			if line != "" {
				b.WriteString("- " + line + "\n")
			}
		}
	}
	content, short := Budget(b.String(), maxResult)
	label := fmt.Sprintf("Ran a %s report on %s", what, firstNonEmpty(fmt.Sprint(params["ref"]), fmt.Sprint(params["word"])))
	if limited > 0 {
		label += " (some text shortened or left out by licences)"
	} else if short {
		label += " (shortened)"
	}
	return Result{Content: content, Label: label}, nil
}

func limitTo(name, text string, max int) Gated {
	if len([]rune(text)) <= max {
		return Gated{Text: text}
	}
	return Gated{Text: cut(text, max), Shortened: true, Notice: fmt.Sprintf("Shortened to the quoting limit of %s's licence.", name)}
}

func sourceModule(srcs []struct {
	Module  string `json:"module"`
	Name    string `json:"name"`
	Licence string `json:"licence"`
}) string {
	if len(srcs) == 1 {
		return srcs[0].Module
	}
	return ""
}

func nonEmpty(xs ...string) []string {
	out := []string{}
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			out = append(out, x)
		}
	}
	return out
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" && x != "<nil>" {
			return x
		}
	}
	return ""
}

func passageReport(ctx context.Context, d *Deps, raw json.RawMessage) (Result, error) {
	a, err := args[struct {
		Ref string `json:"ref"`
	}](raw)
	if err != nil {
		return Result{}, err
	}
	return report(ctx, d, "report.passage", map[string]any{"ref": a.Ref}, "passage")
}

func wordReport(ctx context.Context, d *Deps, raw json.RawMessage) (Result, error) {
	a, err := args[struct {
		Word string `json:"word"`
	}](raw)
	if err != nil {
		return Result{}, err
	}
	return report(ctx, d, "report.word", map[string]any{"word": a.Word}, "word")
}

var strongRE = regexp.MustCompile(`^[GHgh]\d+[a-zA-Z]?$`)

func lookUpWord(ctx context.Context, d *Deps, raw json.RawMessage) (Result, error) {
	a, err := args[struct {
		Word string `json:"word"`
	}](raw)
	if err != nil {
		return Result{}, err
	}
	p := map[string]any{"limit": 3}
	if w := strings.TrimSpace(a.Word); strongRE.MatchString(w) {
		p["strong"] = w
	} else {
		p["word"] = w
	}
	var entries []struct {
		Module   string `json:"module"`
		Name     string `json:"name"`
		Key      string `json:"key"`
		Strong   string `json:"strong"`
		Lemma    string `json:"lemma"`
		Translit string `json:"transliteration"`
		Gloss    string `json:"gloss"`
		Text     string `json:"text"`
	}
	if err := d.Run(ctx, "lexicon.lookup", p, &entries); err != nil {
		return Result{}, err
	}
	mods, err := d.modules(ctx, "all")
	if err != nil {
		return Result{}, err
	}
	byID := map[string]moduleMeta{}
	for _, m := range mods {
		byID[m.ID] = m
	}
	var b strings.Builder
	limited := 0
	for _, e := range entries {
		m := byID[e.Module]
		if m.ID == "" {
			m = moduleMeta{ID: e.Module, Name: e.Name}
			m.Licence, _ = licence.Defaults(licence.Unknown, "")
		}
		g := d.Gate.Work(m.short(), m.Licence, plain(e.Text))
		if g.Refused || g.Shortened {
			limited++
		}
		fmt.Fprintf(&b, "%s: %s\n", m.short(), strings.Join(nonEmpty(e.Key, e.Strong, e.Lemma, e.Translit, e.Gloss), " · "))
		if g.Text != "" {
			b.WriteString(g.Text + "\n")
		}
		if g.Notice != "" {
			b.WriteString("[" + g.Notice + "]\n")
		}
		b.WriteByte('\n')
	}
	if len(entries) == 0 {
		b.WriteString("No entries found.")
	}
	label := fmt.Sprintf("Looked up %s: %d entries", a.Word, len(entries))
	if limited > 0 {
		label += " (some shortened by licences)"
	}
	content, _ := Budget(b.String(), maxResult)
	return Result{Content: content, Label: label}, nil
}

// passagesText lists a document's passages, such as "Matthew 14:22-33; Psalms 84".
func passagesText(list []struct {
	Display string `json:"display"`
}, first *struct {
	Display string `json:"display"`
}) string {
	var out []string
	for _, r := range list {
		out = append(out, r.Display)
	}
	if len(out) == 0 && first != nil {
		out = append(out, first.Display)
	}
	return strings.Join(out, "; ")
}
