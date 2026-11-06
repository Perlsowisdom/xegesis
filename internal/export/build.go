package export

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/document"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/lists"
	"github.com/Perlsowisdom/xegesis/internal/plan"
	"github.com/Perlsowisdom/xegesis/internal/reader"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Limits are in pages of reader.CharsPerPage characters, as for book.copy.
const perPage = reader.CharsPerPage

// owner is a library book or Bible whose licence governs some of the content.
type owner struct {
	key    string
	module bool
	name   string // its title or abbreviation
	lic    licence.Licence
	total  int    // its length in pages, for percentage limits
	max    int    // pages per day the licence allows; 0 when unlimited
	used   int    // characters already taken today
	left   int    // characters still allowed in this document
	block  string // why nothing at all may be taken, when that is so
	credit string
}

func (o *owner) limited() bool { return o.max > 0 || o.block != "" }

type charge struct {
	o *owner
	n int
}

// unit is a block, or one row of a side-by-side grid, that a limit keeps or cuts whole.
type unit struct {
	block   Block
	head    []string // for a grid row: the column heads
	row     *GridRow
	charges []charge
}

// draft is a piece before limits are applied.
type draft struct {
	piece  Piece
	units  []unit
	owners []*owner
	rest   func(kept int) *Source // the source for the units after the first kept, or nil
}

type built struct {
	doc      Doc
	action   string
	owners   []*owner
	taken    map[*owner]int
	infos    []PieceInfo
	cuts     []string
	warnings []string
}

type builder struct {
	s      *service
	ctx    context.Context
	env    *command.Env
	opt    Options
	action string
	owners map[string]*owner
	order  []*owner
	warned map[string]bool
	warn   []string
}

func (s *service) build(ctx context.Context, env *command.Env, sources []Source, title string, opt Options, act string) (*built, error) {
	if len(sources) == 0 {
		return nil, command.Errorf("bad_params", "nothing to print or export: give at least one source")
	}
	bd := &builder{s: s, ctx: ctx, env: env, opt: opt, action: act, owners: map[string]*owner{}, warned: map[string]bool{}}
	var drafts []draft
	for i, src := range sources {
		ds, err := bd.source(src)
		if err != nil {
			if ce, ok := err.(*command.Error); ok {
				return nil, command.Errorf(ce.Code, "item %d (%s): %s", i+1, src.Kind, ce.Message)
			}
			return nil, err
		}
		drafts = append(drafts, ds...)
	}
	out := &built{action: act, owners: bd.order, taken: map[*owner]int{}, warnings: bd.warn, cuts: []string{}}
	for _, d := range drafts {
		p, info := out.apply(d)
		out.doc.Pieces = append(out.doc.Pieces, p)
		out.infos = append(out.infos, info)
	}
	if err := out.check(); err != nil {
		return nil, err
	}
	out.doc.Opt = opt
	out.doc.Title = strings.TrimSpace(title)
	if out.doc.Title == "" && len(out.doc.Pieces) > 0 {
		out.doc.Title = out.doc.Pieces[0].Title
	}
	if out.warnings == nil {
		out.warnings = []string{}
	}
	return out, nil
}

// apply keeps units in order while every owner they draw on has room, and says what was cut.
func (b *built) apply(d draft) (Piece, PieceInfo) {
	p := d.piece
	info := PieceInfo{Title: p.Title, Credits: p.Credits, Licence: licenceWords(d.owners)}
	whole := map[*owner]int{}
	all := 0
	for _, u := range d.units {
		for _, c := range u.charges {
			whole[c.o] += c.n
			all += c.n
		}
	}
	info.Pages = roundPages(all)
	kept := 0
	var stop *owner
	for _, u := range d.units {
		if o := b.blocker(u); o != nil {
			stop = o
			break
		}
		for _, c := range u.charges {
			if c.o.limited() {
				c.o.left -= c.n
				b.taken[c.o] += c.n
			}
		}
		addUnit(&p, u)
		kept++
	}
	for _, o := range d.owners {
		if !o.lic.Confirmed && o.lic.Type == licence.Unknown {
			info.Confirm = true
		}
	}
	if stop != nil {
		keptChars := 0
		for _, u := range d.units[:kept] {
			for _, c := range u.charges {
				if c.o == stop {
					keptChars += c.n
				}
			}
		}
		cut := &Cut{Reason: b.reason(stop, keptChars, whole[stop]), Kept: roundPages(keptChars), Total: roundPages(whole[stop])}
		if d.rest != nil {
			cut.Rest = d.rest(kept)
		}
		info.Cut = cut
		b.cuts = append(b.cuts, p.Title+": "+cut.Reason)
		p.Blocks = append(p.Blocks, Block{Kind: Notice, Spans: plain("Shortened. " + cut.Reason)})
	}
	return p, info
}

func (b *built) blocker(u unit) *owner {
	need := map[*owner]int{}
	for _, c := range u.charges {
		need[c.o] += c.n
	}
	for _, c := range u.charges {
		if c.o.limited() && (c.o.block != "" || need[c.o] > c.o.left) {
			return c.o
		}
	}
	return nil
}

func addUnit(p *Piece, u unit) {
	if u.row == nil {
		p.Blocks = append(p.Blocks, u.block)
		return
	}
	if n := len(p.Blocks); n > 0 && p.Blocks[n-1].Kind == Table {
		p.Blocks[n-1].Table.Rows = append(p.Blocks[n-1].Table.Rows, *u.row)
		return
	}
	p.Blocks = append(p.Blocks, Block{Kind: Table, Table: &Grid{Head: u.head, Rows: []GridRow{*u.row}}})
}

// check confirms with licence.Check that what is taken from each owner today stays within its limit.
func (b *built) check() error {
	for o, n := range b.taken {
		if n == 0 || o.block != "" {
			continue
		}
		d := o.lic.Check(licence.Request{Action: licence.Action(b.action), Pages: pagesOf(o.used + n), TotalPages: o.total})
		if !d.Allowed {
			return fmt.Errorf("export: %s would pass its limit: %s", o.name, d.Reason)
		}
	}
	return nil
}

func (b *built) reason(o *owner, kept, whole int) string {
	if o.block != "" {
		return o.block
	}
	verb := "printed"
	if b.action == "export" {
		verb = "exported or copied"
	}
	s := fmt.Sprintf("%s: %s per day", o.lic.Plain(), limitWords(limitOf(o.lic, b.action), o.module))
	if kept == 0 {
		s += fmt.Sprintf(", and today's share is used up; none of its %s fit", pageWords(whole))
	} else if roundPages(kept) < 0.1 {
		s += fmt.Sprintf("; less than 0.1 of its %s fit", pageWords(whole))
	} else {
		s += fmt.Sprintf("; %s of %s fit", numWords(kept), pageWords(whole))
	}
	if o.used > 0 {
		s += fmt.Sprintf(" (%s already %s today)", pageWords(o.used), verb)
	}
	return s + ". The rest can be " + strings.SplitN(verb, " ", 2)[0] + " tomorrow."
}

func limitOf(l licence.Licence, act string) *licence.Limit {
	if act == "print" {
		return l.Print
	}
	return l.Export
}

func limitWords(l *licence.Limit, module bool) string {
	what := "this book"
	if module {
		what = "this Bible"
	}
	switch {
	case l == nil:
		return "no limit"
	case l.Pages > 0 && l.Percent > 0:
		return fmt.Sprintf("up to %d pages or %s%% of %s, whichever is less,", l.Pages, trimFloat(l.Percent), what)
	case l.Pages > 0:
		return fmt.Sprintf("up to %d pages of %s", l.Pages, what)
	}
	return fmt.Sprintf("up to %s%% of %s", trimFloat(l.Percent), what)
}

func pagesOf(chars int) int { return (chars + perPage - 1) / perPage }

func roundPages(chars int) float64 {
	return float64(int(float64(chars)/perPage*10+0.5)) / 10
}

func numWords(chars int) string { return trimFloat(max(roundPages(chars), 0.1)) }

func pageWords(chars int) string {
	n := max(roundPages(chars), 0.1)
	if n == 1 {
		return "1 page"
	}
	return trimFloat(n) + " pages"
}

func trimFloat(f float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
}

func licenceWords(os []*owner) string {
	if len(os) == 0 {
		return "My own work"
	}
	var names []string
	seen := map[string]bool{}
	for _, o := range os {
		if n := o.lic.Plain(); !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	return strings.Join(names, " · ")
}

func (b *built) preview(format string) Preview {
	infos := b.infos
	if infos == nil {
		infos = []PieceInfo{}
	}
	return Preview{Title: b.doc.Title, Format: format, Pieces: infos, Cuts: b.cuts, Warnings: append([]string{}, b.warnings...),
		Body: Body(b.doc), CSS: CSS(b.doc.Opt, b.doc.Title)}
}

// Owners -----------------------------------------------------------------------------------------

func (bd *builder) newOwner(key string, module bool, name string, lic licence.Licence, total int) *owner {
	o := &owner{key: key, module: module, name: name, lic: lic, total: max(total, 1)}
	o.credit = creditLine(name, lic)
	d := lic.Check(licence.Request{Action: licence.Action(bd.action), TotalPages: o.total})
	switch {
	case d.Allowed && d.MaxPages == 0:
	case d.MaxPages > 0:
		o.max = d.MaxPages
	default:
		o.block = fmt.Sprintf("%s: %s, so none of it can be included", lic.Plain(), d.Reason)
	}
	if o.limited() && bd.env.Store != nil {
		col := "item_id"
		if module {
			col = "module"
		}
		bd.env.Store.QueryRow(bd.ctx, `SELECT COALESCE(SUM(chars), 0) FROM copy_log WHERE `+col+` = ? AND day = ? AND action = ?`,
			key, bd.s.today(), bd.action).Scan(&o.used)
		o.left = max(o.max*perPage-o.used, 0)
	}
	if !lic.Confirmed && lic.Type == licence.Unknown && !bd.warned[key] {
		bd.warned[key] = true
		bd.warn = append(bd.warn, fmt.Sprintf("The licence of %s was never confirmed, so it is treated as a personal copy.", name))
	}
	bd.owners[ownerKey(key, module)] = o
	bd.order = append(bd.order, o)
	return o
}

func ownerKey(key string, module bool) string {
	if module {
		return "bible:" + key
	}
	return "item:" + key
}

// creditLine is the credit to print with an item's content, named so it reads on its own.
func creditLine(name string, l licence.Licence) string {
	line := l.AttributionLine()
	if line == "" {
		return ""
	}
	if name != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(name)) {
		return name + ": " + line
	}
	return line
}

func (bd *builder) moduleOwner(id string) (*owner, *bible.Module, error) {
	if bd.s.bibles == nil {
		return nil, nil, command.Errorf("unavailable", "no Bibles are installed")
	}
	m, err := bd.s.bibles.Get(id)
	if err != nil {
		return nil, nil, command.Errorf("not_found", "no Bible module %q", id)
	}
	if o, ok := bd.owners[ownerKey(id, true)]; ok {
		return o, m, nil
	}
	chars, err := m.TextChars(bd.ctx)
	if err != nil {
		return nil, nil, err
	}
	name := m.Abbreviation
	if name == "" {
		name = m.Name
	}
	return bd.newOwner(id, true, name, m.Licence, pagesOf(chars)), m, nil
}

func (bd *builder) itemOwner(e reader.Excerpt) *owner {
	if o, ok := bd.owners[ownerKey(e.ItemID, false)]; ok {
		return o
	}
	return bd.newOwner(e.ItemID, false, e.Title, e.Licence, e.TotalPages)
}

// itemOwnerByID measures a book for an excerpt taken earlier. When the book cannot be opened, its
// licence still decides: without limits nothing is lost, with limits its length is unknown.
func (bd *builder) itemOwnerByID(itemID string) (*owner, error) {
	if o, ok := bd.owners[ownerKey(itemID, false)]; ok {
		return o, nil
	}
	var e reader.Excerpt
	if err := bd.run("book.excerpt", map[string]any{"item_id": itemID}, &e); err == nil {
		return bd.itemOwner(e), nil
	}
	var it library.Item
	if err := bd.run("library.item", map[string]any{"id": itemID}, &it); err != nil {
		return nil, err
	}
	lic, _ := licence.Defaults(licence.Unknown, "")
	if it.Licence != nil {
		lic = *it.Licence
	}
	o := bd.newOwner(itemID, false, it.Title, lic, 0)
	if o.limited() && o.block == "" {
		o.block = fmt.Sprintf("%s: the book cannot be opened (is its library folder connected?), so how much of it may be included cannot be measured", lic.Plain())
	}
	return o, nil
}

// run calls another command and decodes its result into out.
func (bd *builder) run(id string, params any, out any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	res, err := bd.s.reg.Run(bd.ctx, bd.env, id, raw)
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

// Sources ----------------------------------------------------------------------------------------

func (bd *builder) source(src Source) ([]draft, error) {
	switch src.Kind {
	case "passage":
		return bd.passage(src)
	case "book":
		return bd.book(src)
	case "note":
		return bd.note(src)
	case "list":
		return bd.list(src)
	case "plan":
		return bd.plan(src)
	case "report":
		return bd.report(src)
	case "document":
		return bd.document(src)
	}
	return nil, command.Errorf("bad_params", "unknown kind %q (use passage, book, note, list, plan, report or document)", src.Kind)
}

func chars(s string) int { return utf8.RuneCountInString(s) }

func verseNum(id verse.ID, multiChapter bool) string {
	if multiChapter {
		return fmt.Sprintf("%d:%d", id.Chapter(), id.Verse())
	}
	return fmt.Sprint(id.Verse())
}

func (bd *builder) passage(src Source) ([]draft, error) {
	if src.Ref == "" || len(src.Modules) == 0 {
		return nil, command.Errorf("bad_params", "a passage needs ref and modules")
	}
	var res bible.ParallelResult
	if err := bd.run("bible.parallel", map[string]any{"modules": src.Modules, "ref": src.Ref, "versification": src.Versification}, &res); err != nil {
		return nil, err
	}
	owners := make([]*owner, len(res.Columns))
	for i, col := range res.Columns {
		o, _, err := bd.moduleOwner(col.Module)
		if err != nil {
			return nil, err
		}
		owners[i] = o
	}
	base := func(v bible.ReadVerse) verse.ID {
		if v.BaseID != 0 {
			return v.BaseID
		}
		return v.ID
	}
	// The passage spans chapters when its verses do.
	chapters := map[int]bool{}
	for _, col := range res.Columns {
		for _, v := range col.Verses {
			chapters[base(v).Book()*1000+base(v).Chapter()] = true
		}
	}
	multi := len(chapters) > 1
	restFrom := func(modules []string, first verse.ID, last verse.ID) *Source {
		r := verse.Range{Start: first, End: last}
		return &Source{Kind: "passage", Ref: r.String(), Modules: modules, Versification: src.Versification}
	}
	var out []draft
	if len(res.Columns) > 1 && bd.opt.sideBySide() {
		ids := []verse.ID{}
		cells := map[verse.ID][][]string{}
		for c, col := range res.Columns {
			for _, v := range col.Verses {
				id := base(v)
				if _, ok := cells[id]; !ok {
					ids = append(ids, id)
					cells[id] = make([][]string, len(res.Columns))
				}
				cells[id][c] = append(cells[id][c], v.Text)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		d := draft{piece: Piece{Title: res.Ref}, owners: owners}
		var head, names []string
		for i, o := range owners {
			head = append(head, o.name)
			names = append(names, bd.moduleName(res.Columns[i].Module))
			if o.credit != "" {
				d.piece.Credits = append(d.piece.Credits, o.credit)
			}
		}
		d.piece.Subtitle = strings.Join(names, " · ")
		for _, id := range ids {
			row := GridRow{Num: verseNum(id, multi)}
			var ch []charge
			for c := range res.Columns {
				t := strings.Join(cells[id][c], " ")
				row.Cells = append(row.Cells, plain(t))
				ch = append(ch, charge{owners[c], chars(t)})
			}
			d.units = append(d.units, unit{head: head, row: &row, charges: ch})
		}
		last := ids
		d.rest = func(kept int) *Source {
			if kept >= len(last) {
				return nil
			}
			return restFrom(src.Modules, last[kept], last[len(last)-1])
		}
		out = append(out, d)
	} else {
		for i, col := range res.Columns {
			o := owners[i]
			d := draft{piece: Piece{Title: res.Ref + " · " + o.name, Subtitle: bd.moduleName(col.Module)}, owners: []*owner{o}}
			if o.credit != "" {
				d.piece.Credits = []string{o.credit}
			}
			var ids []verse.ID
			for _, v := range col.Verses {
				ids = append(ids, base(v))
				d.units = append(d.units, unit{block: Block{Kind: Verse, Num: verseNum(base(v), multi), Spans: plain(v.Text)},
					charges: []charge{{o, chars(v.Text)}}})
			}
			module := col.Module
			d.rest = func(kept int) *Source {
				if kept >= len(ids) {
					return nil
				}
				return restFrom([]string{module}, ids[kept], ids[len(ids)-1])
			}
			out = append(out, d)
		}
	}
	if bd.opt.Notes {
		n, err := bd.notesOn(res.Ref)
		if err != nil {
			return nil, err
		}
		if n != nil {
			out = append(out, *n)
		}
	}
	return out, nil
}

func (bd *builder) moduleName(id string) string {
	if m, err := bd.s.bibles.Get(id); err == nil && m.Name != "" {
		return m.Name
	}
	return id
}

func (bd *builder) notesOn(ref string) (*draft, error) {
	var notes []core.Note
	if err := bd.run("note.list", map[string]any{"ref": ref}, &notes); err != nil {
		return nil, err
	}
	if len(notes) == 0 {
		return nil, nil
	}
	d := draft{piece: Piece{Title: "Your notes on " + ref}}
	for _, n := range notes {
		title := n.Title
		if title == "" && n.Ref != nil {
			title = n.Ref.Display
		}
		if title != "" {
			d.units = append(d.units, unit{block: Block{Kind: Heading, Level: 2, Spans: plain(title)}})
		}
		for _, b := range parseNote(n.Body) {
			d.units = append(d.units, unit{block: b})
		}
	}
	return &d, nil
}

func (bd *builder) book(src Source) ([]draft, error) {
	if src.ItemID == "" || strings.TrimSpace(src.Text) == "" && src.From < 1 {
		return nil, command.Errorf("bad_params", "a book excerpt needs item_id and either text or from")
	}
	var e reader.Excerpt
	if err := bd.run("book.excerpt", map[string]any{"item_id": src.ItemID, "file_id": src.FileID, "text": src.Text,
		"from": src.From, "to": src.To}, &e); err != nil {
		return nil, err
	}
	o := bd.itemOwner(e)
	d := draft{piece: Piece{Title: e.Title}, owners: []*owner{o}}
	sub := []string{}
	if len(e.Authors) > 0 {
		sub = append(sub, strings.Join(e.Authors, ", "))
	}
	if e.Where != "" {
		sub = append(sub, e.Where)
	}
	d.piece.Subtitle = strings.Join(sub, " · ")
	if o.credit != "" {
		d.piece.Credits = []string{o.credit}
	}
	blocks := e.Blocks
	skip := min(max(src.Skip, 0), len(blocks))
	blocks = blocks[skip:]
	for _, b := range blocks {
		k := Block{Kind: Para, Spans: plain(b.Text)}
		if b.Heading {
			k = Block{Kind: Heading, Level: 2, Spans: plain(b.Text)}
		}
		d.units = append(d.units, unit{block: k, charges: []charge{{o, chars(b.Text)}}})
	}
	d.rest = func(kept int) *Source {
		if kept >= len(blocks) {
			return nil
		}
		r := src
		if strings.TrimSpace(src.Text) != "" {
			var rest []string
			for _, b := range blocks[kept:] {
				rest = append(rest, b.Text)
			}
			r.Text, r.Skip = strings.Join(rest, "\n"), 0
		} else {
			r.Skip = skip + kept
		}
		return &r
	}
	return []draft{d}, nil
}

func (bd *builder) note(src Source) ([]draft, error) {
	if src.ID == "" {
		return nil, command.Errorf("bad_params", "a note needs id")
	}
	var n core.Note
	if err := bd.run("note.get", map[string]any{"id": src.ID}, &n); err != nil {
		return nil, err
	}
	d := draft{piece: Piece{Title: n.Title}}
	if d.piece.Title == "" {
		d.piece.Title = "Note"
	}
	switch {
	case n.Ref != nil:
		d.piece.Subtitle = n.Ref.Display
	case n.ItemTitle != "":
		d.piece.Subtitle = n.ItemTitle
	}
	for _, b := range parseNote(n.Body) {
		d.units = append(d.units, unit{block: b})
	}
	if len(n.Tags) > 0 {
		d.units = append(d.units, unit{block: Block{Kind: Para, Spans: []Span{{Text: "Tags: " + strings.Join(n.Tags, ", "), Italic: true}}}})
	}
	return []draft{d}, nil
}

// document is a sermon, lesson or study: its outline as a numbered list, then its text. Bible text
// quoted into it already passed its licence check (document.quote) and carries its credit.
func (bd *builder) document(src Source) ([]draft, error) {
	if src.ID == "" {
		return nil, command.Errorf("bad_params", "a document needs id")
	}
	var doc document.Document
	if err := bd.run("document.get", map[string]any{"id": src.ID}, &doc); err != nil {
		return nil, err
	}
	sub := []string{map[string]string{"sermon": "Sermon", "lesson": "Lesson", "study": "Bible study"}[doc.Kind]}
	var shown []string
	for _, r := range doc.Passages {
		shown = append(shown, r.Display)
	}
	if len(shown) == 0 && doc.Passage != nil {
		shown = append(shown, doc.Passage.Display)
	}
	if len(shown) > 0 {
		sub = append(sub, strings.Join(shown, "; "))
	}
	d := draft{piece: Piece{Title: doc.Title, Subtitle: strings.Join(sub, " · ")}}
	if len(doc.Points) > 0 {
		d.units = append(d.units, unit{block: Block{Kind: Heading, Level: 2, Spans: plain("Outline")}})
		main := 0
		for _, p := range doc.Points {
			spans := plain(p.Heading)
			if p.Ref != "" {
				if len(spans) > 0 {
					spans = append(spans, Span{Text: " "})
				}
				spans = append(spans, Span{Text: "(" + p.Ref + ")", Italic: true})
			}
			if p.Level == 2 {
				d.units = append(d.units, unit{block: Block{Kind: Bullet, Spans: spans}})
				continue
			}
			main++
			d.units = append(d.units, unit{block: Block{Kind: Numbered, Num: fmt.Sprint(main), Spans: spans}})
		}
	}
	blocks := parseNote(doc.Body)
	if len(blocks) > 0 && len(doc.Points) > 0 {
		d.units = append(d.units, unit{block: Block{Kind: Heading, Level: 2, Spans: plain("Text")}})
	}
	for _, b := range blocks {
		d.units = append(d.units, unit{block: b})
	}
	return []draft{d}, nil
}

func (bd *builder) list(src Source) ([]draft, error) {
	if src.ID == "" {
		return nil, command.Errorf("bad_params", "a list needs id")
	}
	var it lists.Items
	if err := bd.run("list.items", map[string]any{"id": src.ID}, &it); err != nil {
		return nil, err
	}
	d := draft{piece: Piece{Title: it.List.Name, Subtitle: map[string]string{"verses": "Verse list", "words": "Word list",
		"excerpts": "Excerpts"}[it.List.Kind]}}
	var bib *owner
	if it.List.Kind == "verses" && src.Module != "" {
		o, _, err := bd.moduleOwner(src.Module)
		if err != nil {
			return nil, err
		}
		bib = o
		d.owners = append(d.owners, o)
		if o.credit != "" {
			d.piece.Credits = append(d.piece.Credits, o.credit)
		}
	}
	credits := map[string]bool{}
	for _, e := range it.Entries {
		note := Block{Kind: Para, Spans: []Span{{Text: e.Note, Italic: true}}}
		switch it.List.Kind {
		case "verses":
			if e.Ref == nil {
				continue
			}
			spans := []Span{{Text: e.Ref.Display, Bold: true}}
			var ch []charge
			if bib != nil {
				var p bible.Passage
				if err := bd.run("bible.read", map[string]any{"module": src.Module, "ref": e.Ref.Display}, &p); err == nil {
					var ts []string
					for _, v := range p.Verses {
						ts = append(ts, v.Text)
					}
					t := strings.Join(ts, " ")
					spans = append(spans, Span{Text: " " + t})
					ch = append(ch, charge{bib, chars(t)})
				}
			}
			d.units = append(d.units, unit{block: Block{Kind: Para, Spans: spans}, charges: ch})
		case "words":
			parts := []string{}
			for _, s := range []string{e.Strong, e.Lemma} {
				if s != "" {
					parts = append(parts, s)
				}
			}
			spans := []Span{{Text: e.Word, Bold: true}}
			if len(parts) > 0 {
				spans = append(spans, Span{Text: " (" + strings.Join(parts, ", ") + ")"})
			}
			d.units = append(d.units, unit{block: Block{Kind: Bullet, Spans: spans}})
		case "excerpts":
			o, err := bd.itemOwnerByID(e.ItemID)
			if err != nil {
				return nil, err
			}
			d.owners = appendOwner(d.owners, o)
			d.units = append(d.units, unit{block: Block{Kind: Quote, Spans: plain(e.Text)}, charges: []charge{{o, chars(e.Text)}}})
			src := e.Title
			if e.Attribution != "" {
				src += ". " + e.Attribution
			}
			d.units = append(d.units, unit{block: Block{Kind: Para, Spans: []Span{{Text: "— " + src, Italic: true}}}})
			if c := o.credit; c != "" && !credits[c] {
				credits[c] = true
				d.piece.Credits = append(d.piece.Credits, c)
			}
		}
		if e.Note != "" {
			d.units = append(d.units, unit{block: note})
		}
	}
	return []draft{d}, nil
}

func appendOwner(os []*owner, o *owner) []*owner {
	for _, x := range os {
		if x == o {
			return os
		}
	}
	return append(os, o)
}

func (bd *builder) plan(src Source) ([]draft, error) {
	if src.ID == "" {
		return nil, command.Errorf("bad_params", "a reading plan needs id")
	}
	var plans []plan.Plan
	if err := bd.run("plan.list", map[string]any{}, &plans); err != nil {
		return nil, err
	}
	var pl *plan.Plan
	for i := range plans {
		if plans[i].ID == src.ID {
			pl = &plans[i]
		}
	}
	if pl == nil {
		return nil, command.Errorf("not_found", "no reading plan %s", src.ID)
	}
	var days []plan.Day
	if err := bd.run("plan.days", map[string]any{"plan_id": src.ID}, &days); err != nil {
		return nil, err
	}
	d := draft{piece: Piece{Title: pl.Name, Subtitle: fmt.Sprintf("%d days, from %s · %d read", pl.Days, dateWords(pl.StartDate), pl.DaysDone)}}
	for _, day := range days {
		var rs []string
		for _, r := range day.Readings {
			rs = append(rs, r.Ref)
		}
		spans := []Span{{Text: fmt.Sprintf("Day %d · %s", day.Day, dateWords(day.Date)), Bold: true}, {Text: "  " + strings.Join(rs, "; ")}}
		if day.Done {
			spans = append(spans, Span{Text: "  (read)", Italic: true})
		}
		d.units = append(d.units, unit{block: Block{Kind: Para, Spans: spans}})
	}
	return []draft{d}, nil
}
