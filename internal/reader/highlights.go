package reader

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/passage"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Anchor is a place in a book, stored as an anchor's locator_json. Reflowable books are anchored
// by chapter and by paragraph markers (book.section's data-p) with character offsets into the
// paragraph's text; PDF pages by rectangles on the page.
type Anchor struct {
	Section   int          `json:"section" desc:"Section: EPUB chapter in reading order, PDF page, or 1"`
	CFI       string       `json:"cfi,omitempty" desc:"EPUB: the chapter's canonical fragment identifier"`
	Para      int          `json:"para,omitempty" desc:"Paragraph where the passage starts"`
	Offset    int          `json:"offset,omitempty" desc:"Characters into that paragraph's text"`
	EndPara   int          `json:"end_para,omitempty" desc:"Paragraph where the passage ends"`
	EndOffset int          `json:"end_offset,omitempty" desc:"Characters into that paragraph's text"`
	Page      int          `json:"page,omitempty" desc:"PDF: 1-based page"`
	Rects     [][4]float64 `json:"rects,omitempty" desc:"PDF: [x, y, width, height] boxes as fractions of the page size"`
	Text      string       `json:"text,omitempty" desc:"The passage's text, to find it again if the book changes"`
}

func (a Anchor) valid() bool {
	if a.Section < 1 || a.Para < 0 || a.Offset < 0 || a.EndPara < 0 || a.EndOffset < 0 || len(a.Text) > 20000 {
		return false
	}
	for _, r := range a.Rects {
		for _, v := range r {
			if v < -0.01 || v > 1.01 {
				return false
			}
		}
	}
	return a.Page > 0 || a.Para > 0
}

type HighlightCreateParams struct {
	ItemID        string  `json:"item_id,omitempty" desc:"The book; give it with locator"`
	Locator       *Anchor `json:"locator,omitempty" desc:"Where in the book"`
	Ref           string  `json:"ref,omitempty" desc:"Or Bible verses to highlight, e.g. \"John 3:16\""`
	Start         int     `json:"verse_start,omitempty" desc:"Or the first verse ID (BBCCCVVV), with verse_end"`
	End           int     `json:"verse_end,omitempty" desc:"The last verse ID"`
	Versification string  `json:"versification,omitempty" enum:"english,hebrew" desc:"Numbering of the verses given; default english. Highlights are kept in English numbering and show in every Bible"`
	Style         string  `json:"style,omitempty" enum:"yellow,green,blue,pink,purple,underline,yellow-underline,green-underline,blue-underline,pink-underline,purple-underline" desc:"Default yellow. A colour with -underline underlines in that colour; underline alone is a yellow underline"`
	RangeParams
}

// RangeParams give a Bible highlight exact words instead of whole verses.
type RangeParams struct {
	Range  *VerseRange `json:"range,omitempty" desc:"Bible: only these words, from a character in one verse to one in the same or a later verse (English numbering). The verses are taken from it. Overlapped parts of other highlights in the same Bible are taken out of them"`
	Module string      `json:"module,omitempty" desc:"With range: the Bible the offsets count in"`
	Parts  []QuotePart `json:"parts,omitempty" desc:"With range: each verse's highlighted words, as shown"`
	Quote  string      `json:"quote,omitempty" desc:"With range and no parts: the highlighted words"`
}

// locator checks the range and gives what is stored for it.
func (p RangeParams) locator(id string) (verseLocator, error) {
	r := *p.Range
	if err := validRange(id, r, p.Module, p.Quote); err != nil {
		return verseLocator{}, err
	}
	loc := verseLocator{Range: r, Module: p.Module, Quote: p.Quote}
	for _, w := range p.Parts {
		if w.Verse < r.Start.Verse || w.Verse > r.End.Verse {
			return loc, command.Errorf("bad_params", "%s: the words of verse %d are outside the range", id, w.Verse)
		}
	}
	if len(p.Parts) > 0 {
		loc.Parts, loc.Quote = p.Parts, joinParts(p.Parts)
	}
	if len(loc.Quote) > 20000 {
		return loc, command.Errorf("bad_params", "%s: the quoted words are too long", id)
	}
	return loc, nil
}

type Highlight struct {
	ID        string      `json:"id"`
	ItemID    string      `json:"item_id,omitempty"`
	Locator   *Anchor     `json:"locator,omitempty"`
	Ref       string      `json:"ref,omitempty" desc:"The highlighted verses, for a Bible highlight"`
	Start     int64       `json:"verse_start,omitempty" desc:"First highlighted verse ID, in English numbering"`
	End       int64       `json:"verse_end,omitempty" desc:"Last highlighted verse ID"`
	Style     string      `json:"style"`
	CreatedAt int64       `json:"created_at"`
	ItemTitle string      `json:"item_title,omitempty" desc:"The book's title, for a book highlight"`
	Text      string      `json:"text,omitempty" desc:"The highlighted words: a book highlight's own, a ranged Bible highlight's exact words, or a whole-verse highlight's verses in the module asked for"`
	Range     *VerseRange `json:"range,omitempty" desc:"A Bible highlight on exact words: where they start and end"`
	Module    string      `json:"module,omitempty" desc:"The Bible the range was made in; other Bibles show the whole verses"`
	Parts     []QuotePart `json:"parts,omitempty" desc:"Each verse's highlighted words"`
}

func (s *service) highlightCreate(ctx context.Context, env *command.Env, p HighlightCreateParams) (Highlight, error) {
	anchor := store.Row{"id": store.NewID()}
	r, haveVerses, err := passage.Resolve("highlight.create", p.Ref, p.Start, p.End, p.Versification)
	if err != nil {
		return Highlight{}, err
	}
	var cuts []ranged
	var cut VerseRange
	if p.Range != nil {
		if p.ItemID != "" || p.Locator != nil {
			return Highlight{}, command.Errorf("bad_params", "highlight.create: a range is for Bible verses; a book passage uses locator")
		}
		if p.Versification != "" && verse.System(p.Versification) != verse.English {
			return Highlight{}, command.Errorf("bad_params", "highlight.create: a range is given in English numbering")
		}
		loc, err := p.RangeParams.locator("highlight.create")
		if err != nil {
			return Highlight{}, err
		}
		if haveVerses && (int64(r.Start) != loc.Range.Start.Verse || int64(r.End) != loc.Range.End.Verse) {
			return Highlight{}, command.Errorf("bad_params", "highlight.create: the verses do not match the range")
		}
		r, haveVerses = verse.Range{Start: verse.ID(loc.Range.Start.Verse), End: verse.ID(loc.Range.End.Verse)}, true
		raw, _ := json.Marshal(loc)
		anchor["locator_json"] = string(raw)
		cut = loc.Range
		if cuts, err = overlapping(ctx, env.Store, cut, loc.Module, ""); err != nil {
			return Highlight{}, err
		}
	} else if p.Module != "" || len(p.Parts) > 0 || p.Quote != "" {
		return Highlight{}, command.Errorf("bad_params", "highlight.create: module, parts and quote go with range")
	}
	switch {
	case p.ItemID != "" && p.Locator != nil && !haveVerses:
		if !p.Locator.valid() {
			return Highlight{}, command.Errorf("bad_params", "highlight.create: the locator needs a section and a paragraph or page")
		}
		if err := itemExists(ctx, env.Store, p.ItemID); err != nil {
			return Highlight{}, err
		}
		raw, _ := json.Marshal(p.Locator)
		anchor["item_id"], anchor["locator_json"] = p.ItemID, string(raw)
	case haveVerses && p.ItemID == "" && p.Locator == nil:
		anchor["verse_start"], anchor["verse_end"], anchor["versification"] = int64(r.Start), int64(r.End), string(verse.English)
	default:
		return Highlight{}, command.Errorf("bad_params", "highlight.create: give item_id with locator, or ref")
	}
	if p.Style == "" {
		p.Style = "yellow"
	}
	id := store.NewID()
	err = env.Store.Write(ctx, "highlight.create", func(tx *store.Tx) error {
		if err := split(tx, cut, cuts); err != nil {
			return err
		}
		if err := tx.Create("anchors", anchor); err != nil {
			return err
		}
		return tx.Create("highlights", store.Row{"id": id, "anchor_id": anchor["id"], "style": p.Style, "created_at": now()})
	})
	if err != nil {
		return Highlight{}, err
	}
	list, err := queryHighlights(ctx, env.Store, ` AND h.id = ?`, id)
	if err != nil || len(list) == 0 {
		return Highlight{}, err
	}
	return list[0], nil
}

type HighlightListParams struct {
	ItemID  string `json:"item_id,omitempty" desc:"Only highlights in this book"`
	Section int    `json:"section,omitempty" desc:"With item_id, only this section or PDF page"`
	Ref     string `json:"ref,omitempty" desc:"Only highlights on verses overlapping this reference"`
	Book    string `json:"book,omitempty" desc:"Only highlights in this book of the Bible, e.g. \"John\""`
	On      string `json:"on,omitempty" enum:"verses,books" desc:"Only highlights on Bible verses, or only in library books"`
	Colour  string `json:"colour,omitempty" enum:"yellow,green,blue,pink,purple" desc:"Only highlights of this colour, underlined or not"`
	Module  string `json:"module,omitempty" desc:"A Bible (bible.modules) to give Bible highlights' text from"`
	Query   string `json:"query,omitempty" desc:"Only highlights whose text, place or book title has all these words"`
	Newest  bool   `json:"newest,omitempty" desc:"Newest first; otherwise in reading order (verse order, then by book and place)"`
	Limit   int    `json:"limit,omitempty" desc:"Most highlights to return (default 500)"`
}

func (s *service) highlightList(ctx context.Context, env *command.Env, p HighlightListParams) ([]Highlight, error) {
	if env.Store == nil {
		return nil, command.Errorf("unavailable", "no library is open")
	}
	where, args := "", []any{}
	if p.ItemID != "" {
		where += ` AND a.item_id = ?`
		args = append(args, p.ItemID)
		if p.Section > 0 {
			where += ` AND json_extract(a.locator_json, '$.section') = ?`
			args = append(args, p.Section)
		}
	}
	if p.Ref != "" {
		r, err := verse.Parse(p.Ref)
		if err != nil {
			return nil, command.Errorf("bad_params", "%v", err)
		}
		where += ` AND a.verse_start <= ? AND a.verse_end >= ?`
		args = append(args, int64(r.End), int64(r.Start))
	}
	if p.Book != "" {
		b, ok := verse.LookupBook(p.Book)
		if !ok {
			return nil, command.Errorf("bad_params", "highlight.list: unknown book of the Bible %q", p.Book)
		}
		where += ` AND a.verse_start BETWEEN ? AND ?`
		args = append(args, int64(b.Number)*1_000_000, int64(b.Number+1)*1_000_000-1)
	}
	switch p.On {
	case "verses":
		where += ` AND a.verse_start IS NOT NULL`
	case "books":
		where += ` AND a.item_id IS NOT NULL`
	}
	if p.Colour != "" {
		styles := []any{p.Colour, p.Colour + "-underline"}
		if p.Colour == "yellow" {
			styles = append(styles, "underline")
		}
		where += ` AND h.style IN (?` + strings.Repeat(", ?", len(styles)-1) + `)`
		args = append(args, styles...)
	}
	order := ` ORDER BY a.verse_start IS NULL, a.verse_start, json_extract(a.locator_json, '$.range.start.offset'), a.verse_end, i.title, json_extract(a.locator_json, '$.section'),
		json_extract(a.locator_json, '$.para'), json_extract(a.locator_json, '$.offset'), h.created_at, h.rowid`
	if p.Newest {
		order = ` ORDER BY h.created_at DESC, h.rowid DESC`
	}
	list, err := queryHighlights(ctx, env.Store, where+order, args...)
	if err != nil {
		return nil, err
	}
	if p.Module != "" {
		s.verseText(ctx, env, p.Module, list)
	}
	if q := strings.Fields(strings.ToLower(p.Query)); len(q) > 0 {
		kept := list[:0]
		for _, h := range list {
			hay := strings.ToLower(h.Text + " " + h.Ref + " " + h.ItemTitle)
			all := true
			for _, w := range q {
				all = all && strings.Contains(hay, w)
			}
			if all {
				kept = append(kept, h)
			}
		}
		list = kept
	}
	if p.Limit <= 0 {
		p.Limit = 500
	}
	if len(list) > p.Limit {
		list = list[:p.Limit]
	}
	return list, nil
}

// verseText gives Bible highlights their words from a Bible, when it is installed and has them.
func (s *service) verseText(ctx context.Context, env *command.Env, module string, list []Highlight) {
	for i := range list {
		h := &list[i]
		if h.Ref == "" || h.Text != "" {
			continue
		}
		raw, _ := json.Marshal(map[string]any{"module": module, "ref": h.Ref})
		res, err := s.reg.Run(ctx, env, "bible.read", raw)
		if err != nil {
			continue
		}
		b, _ := json.Marshal(res)
		var pas struct {
			Verses []struct {
				Text string `json:"text"`
			} `json:"verses"`
		}
		if json.Unmarshal(b, &pas) != nil {
			continue
		}
		words := make([]string, 0, len(pas.Verses))
		for _, v := range pas.Verses {
			words = append(words, strings.Join(strings.Fields(v.Text), " "))
		}
		h.Text = strings.Join(words, " ")
	}
}

type HighlightUpdateParams struct {
	ID    string `json:"id" required:"true"`
	Style string `json:"style,omitempty" enum:"yellow,green,blue,pink,purple,underline,yellow-underline,green-underline,blue-underline,pink-underline,purple-underline" desc:"The new colour, with -underline to underline in it"`
	RangeParams
}

func (s *service) highlightUpdate(ctx context.Context, env *command.Env, p HighlightUpdateParams) (Highlight, error) {
	if p.Style == "" && p.Range == nil {
		return Highlight{}, command.Errorf("bad_params", "highlight.update: give style, or range")
	}
	var loc verseLocator
	var cuts []ranged
	if p.Range != nil {
		var err error
		if loc, err = p.RangeParams.locator("highlight.update"); err != nil {
			return Highlight{}, err
		}
		if cuts, err = overlapping(ctx, env.Store, loc.Range, loc.Module, p.ID); err != nil {
			return Highlight{}, err
		}
	} else if p.Module != "" || len(p.Parts) > 0 || p.Quote != "" {
		return Highlight{}, command.Errorf("bad_params", "highlight.update: module, parts and quote go with range")
	}
	err := env.Store.Write(ctx, "highlight.update", func(tx *store.Tx) error {
		cur, err := tx.Get("highlights", store.ByID(p.ID))
		if err != nil {
			return err
		}
		if cur["deleted_at"] != nil {
			return command.Errorf("not_found", "highlight %s is deleted", p.ID)
		}
		if p.Range != nil {
			anchorID, _ := cur["anchor_id"].(string)
			a, err := tx.Get("anchors", store.ByID(anchorID))
			if err != nil {
				return err
			}
			if a["verse_start"] == nil {
				return command.Errorf("bad_params", "highlight.update: a range is for Bible highlights")
			}
			if err := split(tx, loc.Range, cuts); err != nil {
				return err
			}
			raw, _ := json.Marshal(loc)
			fields := store.Row{"verse_start": loc.Range.Start.Verse, "verse_end": loc.Range.End.Verse, "locator_json": string(raw)}
			if err := tx.Update("anchors", store.ByID(anchorID), fields); err != nil {
				return err
			}
		}
		if p.Style == "" || cur["style"] == p.Style {
			return nil
		}
		return tx.Update("highlights", store.ByID(p.ID), store.Row{"style": p.Style})
	})
	if err != nil {
		return Highlight{}, err
	}
	list, err := queryHighlights(ctx, env.Store, ` AND h.id = ?`, p.ID)
	if err != nil || len(list) == 0 {
		return Highlight{}, err
	}
	return list[0], nil
}

type IDParams struct {
	ID string `json:"id" required:"true"`
}

func (s *service) highlightDelete(ctx context.Context, env *command.Env, p IDParams) (struct{}, error) {
	return struct{}{}, env.Store.Write(ctx, "highlight.delete", func(tx *store.Tx) error {
		cur, err := tx.Get("highlights", store.ByID(p.ID))
		if err != nil {
			return err
		}
		if cur["deleted_at"] != nil {
			return command.Errorf("not_found", "highlight %s is deleted", p.ID)
		}
		return tx.Delete("highlights", store.ByID(p.ID))
	})
}

func queryHighlights(ctx context.Context, st *store.Store, where string, args ...any) ([]Highlight, error) {
	if !strings.Contains(where, "ORDER BY") {
		where += ` ORDER BY h.created_at, h.id`
	}
	rows, err := st.Query(ctx, `SELECT h.id, h.style, h.created_at, a.item_id, a.locator_json, a.verse_start, a.verse_end, COALESCE(i.title, '')
		FROM highlights h JOIN anchors a ON a.id = h.anchor_id LEFT JOIN items i ON i.id = a.item_id WHERE h.deleted_at IS NULL`+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Highlight{}
	for rows.Next() {
		var h Highlight
		var item, loc sql.NullString
		var start, end sql.NullInt64
		if err := rows.Scan(&h.ID, &h.Style, &h.CreatedAt, &item, &loc, &start, &end, &h.ItemTitle); err != nil {
			return nil, err
		}
		h.ItemID = item.String
		if loc.Valid && !item.Valid {
			var v verseLocator
			if json.Unmarshal([]byte(loc.String), &v) == nil && !v.Range.empty() {
				h.Range, h.Module, h.Parts, h.Text = &v.Range, v.Module, v.Parts, v.Quote
			}
		} else if loc.Valid {
			var a Anchor
			if json.Unmarshal([]byte(loc.String), &a) == nil {
				h.Locator = &a
				h.Text = a.Text
			}
		}
		if start.Valid && end.Valid {
			h.Ref = verse.Range{Start: verse.ID(start.Int64), End: verse.ID(end.Int64)}.String()
			h.Start, h.End = start.Int64, end.Int64
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
