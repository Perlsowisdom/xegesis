package report

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
)

type SaveParams struct {
	Kind   string   `json:"kind" required:"true" enum:"passage,word,topic"`
	Query  string   `json:"query" required:"true" desc:"The passage, word or topic, as the report's query"`
	Bibles []string `json:"bibles,omitempty" desc:"Bibles the report reads, as for the report command"`
	Title  string   `json:"title,omitempty" desc:"The note's title; the report's title when omitted"`
}

// save builds the report and keeps its headings and links as a note. No text of the works it draws
// on is copied, so the note may be kept and shared whatever their licences; a passage report's note
// is attached to the passage.
func (s *service) save(ctx context.Context, env *command.Env, p SaveParams) (core.Note, error) {
	pg := Paging{Bibles: p.Bibles}
	var rep Report
	var err error
	switch p.Kind {
	case "passage":
		rep, err = s.passage(ctx, env, PassageParams{Ref: p.Query, Paging: pg})
	case "word":
		rep, err = s.word(ctx, env, WordParams{Word: p.Query, Paging: pg})
	default:
		rep, err = s.topic(ctx, env, TopicParams{Topic: p.Query, Paging: pg})
	}
	if err != nil {
		return core.Note{}, err
	}
	params := map[string]any{"title": firstNonEmpty(strings.TrimSpace(p.Title), rep.Title), "body": Markdown(rep), "tags": []string{"report"}}
	if rep.Ref != "" {
		params["ref"] = rep.Ref
	}
	return call[core.Note](ctx, s, env, "note.create", params)
}

// Markdown is a report's outline: each section's title and sources, and its entries as links.
// Links use the app's own addresses: #passage/<OSIS>, #word/<Strong's number>, #topic/<words>,
// #note/<id>, #list/<id>, #plan/<id> and #book/<item>?file=…&spine=…&para=… (also page, offset, href).
// They have no scheme, so the notes editor keeps them.
func Markdown(rep Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", mdLink(rep.Subject, reportLink(rep)))
	for _, sec := range rep.Sections {
		if sec.Error != "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", mdText(sec.Title))
		var names []string
		for _, src := range sec.Sources {
			n := mdText(src.Name)
			if src.Attribution != "" {
				n += " (" + mdText(src.Attribution) + ")"
			}
			names = append(names, n)
		}
		if len(names) > 0 {
			fmt.Fprintf(&b, "*From %s*\n\n", strings.Join(names, "; "))
		}
		for _, e := range sec.Entries {
			fmt.Fprintf(&b, "- %s\n", entryLine(e))
		}
		if sec.Total > len(sec.Entries) {
			fmt.Fprintf(&b, "- … and %d more\n", sec.Total-len(sec.Entries))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func reportLink(rep Report) string {
	return "#report/" + rep.Kind + "/" + url.PathEscape(rep.Query)
}

// entryLine is an entry as links and short facts, never its text.
func entryLine(e Entry) string {
	var parts []string
	switch {
	case e.Book != nil:
		parts = append(parts, mdLink(strings.TrimSpace(e.Book.Title+bookPlace(e.Book)), bookLink(e.Book)))
		if e.RefLabel != "" && e.Ref != "" {
			parts = append(parts, mdLink(e.RefLabel, "#passage/"+e.Ref))
		}
	case e.NoteID != "":
		parts = append(parts, mdLink(e.Heading, "#note/"+e.NoteID))
	case e.ListID != "":
		parts = append(parts, mdLink(e.Heading, "#list/"+e.ListID))
	case e.PlanID != "":
		parts = append(parts, mdLink(e.Heading, "#plan/"+e.PlanID))
	case e.Strong != "" && e.Ref == "":
		parts = append(parts, mdLink(firstNonEmpty(e.Heading, e.Strong), "#word/"+e.Strong))
	case e.Strong != "":
		parts = append(parts, mdLink(firstNonEmpty(e.Heading, e.RefLabel), "#word/"+e.Strong))
	case e.Topic != "":
		parts = append(parts, mdLink(e.Heading, "#topic/"+url.PathEscape(e.Topic)))
	case e.Heading != "" && e.Ref == "":
		parts = append(parts, mdText(e.Heading))
	}
	if e.Ref != "" && e.Book == nil {
		parts = append(parts, mdLink(firstNonEmpty(e.RefLabel, e.Ref), "#passage/"+e.Ref))
	}
	if e.Strong != "" && e.Ref != "" {
		parts = append(parts, e.Strong)
	}
	if e.Gloss != "" {
		parts = append(parts, mdText(e.Gloss))
	}
	if e.Count > 0 && e.Book == nil && len(e.Links) == 0 {
		parts = append(parts, fmt.Sprintf("%d×", e.Count))
	}
	for _, x := range e.Extra {
		if x != "" {
			parts = append(parts, mdText(x))
		}
	}
	if len(e.Links) > 0 {
		var ls []string
		for _, l := range e.Links[:min(len(e.Links), 30)] {
			ls = append(ls, mdLink(firstNonEmpty(l.Label, l.Ref), "#passage/"+l.Ref))
		}
		if len(e.Links) > 30 {
			ls = append(ls, fmt.Sprintf("and %d more", len(e.Links)-30))
		}
		parts = append(parts, strings.Join(ls, ", "))
	}
	return strings.Join(parts, " · ")
}

func bookPlace(b *BookLink) string {
	switch {
	case b.Heading != "":
		return ", " + b.Heading
	case b.Locator.Page > 0:
		return fmt.Sprintf(", page %d", b.Locator.Page)
	}
	return ""
}

func bookLink(b *BookLink) string {
	v := url.Values{}
	v.Set("file", b.FileID)
	l := b.Locator
	for k, n := range map[string]int{"spine": l.Spine, "page": l.Page, "para": l.Para, "offset": l.Offset} {
		if n > 0 {
			v.Set(k, fmt.Sprint(n))
		}
	}
	if l.Href != "" {
		v.Set("href", l.Href)
	}
	return "#book/" + url.PathEscape(b.ItemID) + "?" + v.Encode()
}

// mdEscape keeps text from reading as Markdown marks; the notes' Markdown has no escapes.
var mdEscape = strings.NewReplacer("[", "(", "]", ")", "*", "'", "`", "'", "\n", " ")

func mdText(s string) string { return mdEscape.Replace(strings.TrimSpace(s)) }

func mdLink(text, target string) string {
	text = mdText(text)
	if text == "" {
		text = target
	}
	return "[" + text + "](" + strings.NewReplacer("(", "%28", ")", "%29", " ", "%20").Replace(target) + ")"
}
