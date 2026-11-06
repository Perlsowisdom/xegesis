package document

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/reader"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// A quotation from a Bible whose licence limits exports may be at most this many characters per
// page the licence allows (the same page equivalent book.copy uses); a percentage-only limit
// allows one page, since a Bible's share of a quotation is tiny but its length is not measured here.
const charsPerPage = reader.CharsPerPage

type QuoteParams struct {
	ID      string `json:"id,omitempty" desc:"The document to quote into; not needed with preview"`
	Ref     string `json:"ref" required:"true" desc:"The passage, e.g. \"Rom 8:28-30\""`
	Module  string `json:"module,omitempty" desc:"The Bible to quote, from bible.modules; or give book"`
	Book    string `json:"book,omitempty" desc:"Or a library book typed Bible (its item id) to quote from; its own licence decides, as for book.copy"`
	Keep    bool   `json:"keep,omitempty" desc:"With book and preview: count the quotation against the book's daily copy limit, as when it is put in the text; typing a preview does not"`
	Preview bool   `json:"preview,omitempty" desc:"Only return the quotation, for an editor to place where the cursor is; nothing is saved"`
	Text    string `json:"text,omitempty" desc:"The document's text as the editor holds it, when it is not the saved one (a note, or text not saved yet); quotations in it from a limited Bible count towards that Bible's limit for one document"`
	Words   string `json:"words,omitempty" desc:"Quote only these words of the passage, such as a highlight's exact words; they must be in its text"`
}

type QuoteResult struct {
	Allowed     bool      `json:"allowed"`
	Reason      string    `json:"reason,omitempty" desc:"Why nothing was quoted"`
	Quote       string    `json:"quote,omitempty" desc:"The Markdown added to the end of the text, or to place with preview"`
	Attribution string    `json:"attribution,omitempty"`
	Document    *Document `json:"document,omitempty" desc:"The document after the quotation was added"`
}

func (s *service) call(ctx context.Context, env *command.Env, id string, params, out any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	res, err := s.reg.Run(ctx, env, id, raw)
	if err != nil {
		return err
	}
	b, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func (s *service) quote(ctx context.Context, env *command.Env, p QuoteParams) (QuoteResult, error) {
	switch {
	case p.ID == "" && !p.Preview:
		return QuoteResult{}, command.Errorf("bad_params", "document.quote: give id, or preview")
	case p.ID != "":
		if _, err := get(ctx, env, p.ID); err != nil {
			return QuoteResult{}, err
		}
	}
	r, err := refOf(p.Ref)
	if err != nil {
		return QuoteResult{}, err
	}
	if p.Book != "" {
		return s.quoteBook(ctx, env, p, r)
	}
	if p.Module == "" {
		return QuoteResult{}, command.Errorf("bad_params", "document.quote: give module or book")
	}
	var mods []bible.Meta
	if err := s.call(ctx, env, "bible.modules", map[string]any{}, &mods); err != nil {
		return QuoteResult{}, err
	}
	var meta *bible.Meta
	for i := range mods {
		if mods[i].ID == p.Module {
			meta = &mods[i]
		}
	}
	if meta == nil {
		return QuoteResult{}, command.Errorf("not_found", "no Bible module %q", p.Module)
	}
	lic := meta.Licence
	res := QuoteResult{Attribution: lic.AttributionLine()}
	if !lic.Store {
		res.Reason = fmt.Sprintf("the licence of %s does not allow keeping a copy of its text, so it is shown live in the Reader instead", name(*meta))
		return res, nil
	}
	var pas bible.Passage
	if err := s.call(ctx, env, "bible.read", map[string]any{"module": p.Module, "ref": r.Display}, &pas); err != nil {
		return QuoteResult{}, err
	}
	if len(pas.Verses) == 0 {
		res.Reason = fmt.Sprintf("%s has no verses for %s", name(*meta), r.Display)
		return res, nil
	}
	text := quoteMarkdown(pas, r.Display, r.OSIS, name(*meta), res.Attribution)
	if p.Words != "" {
		if !wordsIn(p.Words, pas) {
			res.Reason = fmt.Sprintf("those words are not in %s's text of %s", name(*meta), r.Display)
			return res, nil
		}
		text = wordsMarkdown(p.Words, r.Display, r.OSIS, name(*meta), res.Attribution)
	}
	if max := quoteLimit(lic); max > 0 && utf8.RuneCountInString(text) > max {
		res.Reason = fmt.Sprintf("the licence of %s allows quoting about %d characters at a time; choose fewer verses", name(*meta), max)
		return res, nil
	}
	if max := quoteLimit(lic); max > 0 {
		held, err := s.heldChars(ctx, env, p, name(*meta))
		if err != nil {
			return QuoteResult{}, err
		}
		if held+utf8.RuneCountInString(text) > max {
			res.Reason = fmt.Sprintf("the licence of %s allows quoting about %d characters in one document, and this one already has %d", name(*meta), max, held)
			return res, nil
		}
	}
	return s.deliver(ctx, env, p, res, text)
}

// deliver returns an allowed quotation, or adds it to the end of the document.
func (s *service) deliver(ctx context.Context, env *command.Env, p QuoteParams, res QuoteResult, text string) (QuoteResult, error) {
	if p.Preview {
		res.Allowed, res.Quote = true, text
		return res, nil
	}
	err := env.Store.Write(ctx, "document.quote", func(tx *store.Tx) error {
		cur, err := live(tx, p.ID)
		if err != nil {
			return err
		}
		body, _ := cur["body_md"].(string)
		body = strings.TrimRight(body, "\n ")
		if body != "" {
			body += "\n\n"
		}
		return tx.Update("documents", store.ByID(p.ID), store.Row{"body_md": body + text, "updated_at": bumped(cur)})
	})
	if err != nil {
		return QuoteResult{}, err
	}
	d, err := get(ctx, env, p.ID)
	if err != nil {
		return QuoteResult{}, err
	}
	res.Allowed, res.Quote, res.Document = true, text, &d
	return res, nil
}

// heldChars counts the characters the document already quotes from a Bible: those of the text the
// editor sent, or else of the saved document.
func (s *service) heldChars(ctx context.Context, env *command.Env, p QuoteParams, bibleName string) (int, error) {
	body := p.Text
	if body == "" && p.ID != "" {
		d, err := get(ctx, env, p.ID)
		if err != nil {
			return 0, err
		}
		body = d.Body
	}
	return QuotedChars(body, bibleName), nil
}

var creditRe = regexp.MustCompile(`^> — \[[^\]]+\]\(#passage/[^)\s]*\) \(([^)]+)\)`)

// QuotedChars is the length of the verse quotations from a Bible (named as in the credit line) in
// some Markdown: each is a block of "> " lines whose last line is its credit.
func QuotedChars(md, bibleName string) int {
	n := 0
	for _, block := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n\n") {
		block = strings.Trim(block, "\n")
		lines := strings.Split(block, "\n")
		if len(lines) < 2 || !strings.HasPrefix(lines[0], "> ") {
			continue
		}
		if m := creditRe.FindStringSubmatch(lines[len(lines)-1]); m != nil && m[1] == bibleName {
			n += utf8.RuneCountInString(block)
		}
	}
	return n
}

func name(m bible.Meta) string {
	if m.Abbreviation != "" {
		return m.Abbreviation
	}
	if m.Name != "" {
		return m.Name
	}
	return m.ID
}

// quoteLimit is the most characters one quotation may have, or 0 for no limit.
func quoteLimit(l licence.Licence) int {
	if l.Export == nil {
		return 0
	}
	if l.Export.Pages > 0 {
		return l.Export.Pages * charsPerPage
	}
	return charsPerPage
}

// quoteMarkdown is the passage as a Markdown block quote: bold verse numbers, then a line with a
// link to the passage, the Bible's short name and its credit. The editor shows a quote whose last
// line starts with "— " as a verse quotation, the credit in small type.
func quoteMarkdown(p bible.Passage, display, osis, bibleName, credit string) string {
	var b strings.Builder
	b.WriteString("> ")
	for i, v := range p.Verses {
		if i > 0 {
			b.WriteByte(' ')
		}
		num := fmt.Sprint(v.ID.Verse())
		if v.End != 0 && v.End != v.ID {
			num += fmt.Sprintf("-%d", v.End.Verse())
		}
		fmt.Fprintf(&b, "**%s** %s", num, strings.Join(strings.Fields(v.Text), " "))
	}
	fmt.Fprintf(&b, "\n> — [%s](#passage/%s) (%s)", display, osis, bibleName)
	if credit != "" {
		b.WriteString(". " + credit)
	}
	b.WriteByte('\n')
	return b.String()
}

// fold keeps only a text's letters and digits, in lower case, so words can be found in a passage
// whatever its punctuation and spacing.
func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func wordsIn(words string, p bible.Passage) bool {
	var all strings.Builder
	for _, v := range p.Verses {
		all.WriteString(fold(v.Text))
	}
	w := fold(words)
	return w != "" && strings.Contains(all.String(), w)
}

// wordsMarkdown quotes some words of a passage the way quoteMarkdown quotes it all, without verse numbers.
func wordsMarkdown(words, display, osis, bibleName, credit string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "> %s\n> — [%s](#passage/%s) (%s)", strings.Join(strings.Fields(words), " "), display, osis, bibleName)
	if credit != "" {
		b.WriteString(". " + credit)
	}
	b.WriteByte('\n')
	return b.String()
}

// quoteBook quotes verses from a library book typed Bible. Its text comes from book.verse_text and
// the book's licence decides, through book.copy: a preview only checks, and the quotation counts
// towards the daily limit when it is kept.
func (s *service) quoteBook(ctx context.Context, env *command.Env, p QuoteParams, r *core.RefResult) (QuoteResult, error) {
	var vt reader.VerseText
	if err := s.call(ctx, env, "book.verse_text", map[string]any{"item_id": p.Book, "ref": r.Display}, &vt); err != nil {
		return QuoteResult{}, err
	}
	res := QuoteResult{}
	if !vt.Found {
		res.Reason = vt.Reason
		return res, nil
	}
	var cp reader.CopyResult
	keep := !p.Preview || p.Keep
	if err := s.call(ctx, env, "book.copy", map[string]any{"item_id": p.Book, "text": vt.Text, "preview": !keep}, &cp); err != nil {
		return QuoteResult{}, err
	}
	res.Attribution = cp.Attribution
	if !cp.Allowed {
		res.Reason = cp.Reason
		return res, nil
	}
	return s.deliver(ctx, env, p, res, wordsMarkdown(vt.Text, r.Display, r.OSIS, vt.Title, cp.Attribution))
}
