package document

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/reader"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// A quotation from a Bible whose licence limits exports may be at most this many characters per
// page the licence allows (the same page equivalent book.copy uses); a percentage-only limit
// allows one page, since a Bible's share of a quotation is tiny but its length is not measured here.
const charsPerPage = reader.CharsPerPage

type QuoteParams struct {
	ID     string `json:"id" required:"true" desc:"The document to quote into"`
	Ref    string `json:"ref" required:"true" desc:"The passage, e.g. \"Rom 8:28-30\""`
	Module string `json:"module" required:"true" desc:"The Bible to quote, from bible.modules"`
}

type QuoteResult struct {
	Allowed     bool      `json:"allowed"`
	Reason      string    `json:"reason,omitempty" desc:"Why nothing was quoted"`
	Quote       string    `json:"quote,omitempty" desc:"The Markdown added to the end of the text"`
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
	if _, err := get(ctx, env, p.ID); err != nil {
		return QuoteResult{}, err
	}
	r, err := refOf(p.Ref)
	if err != nil {
		return QuoteResult{}, err
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
	if max := quoteLimit(lic); max > 0 && utf8.RuneCountInString(text) > max {
		res.Reason = fmt.Sprintf("the licence of %s allows quoting about %d characters at a time; choose fewer verses", name(*meta), max)
		return res, nil
	}
	err = env.Store.Write(ctx, "document.quote", func(tx *store.Tx) error {
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
// link to the passage, the Bible's short name and its credit.
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
	fmt.Fprintf(&b, "\n>\n> — [%s](#passage/%s) (%s)", display, osis, bibleName)
	if credit != "" {
		b.WriteString(". " + credit)
	}
	b.WriteByte('\n')
	return b.String()
}
