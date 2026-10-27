package reader

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Perlsowisdom/xegesis/internal/command"
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
	ItemID  string  `json:"item_id,omitempty" desc:"The book; give it with locator"`
	Locator *Anchor `json:"locator,omitempty" desc:"Where in the book"`
	Ref     string  `json:"ref,omitempty" desc:"Or Bible verses to highlight, e.g. \"John 3:16\""`
	Style   string  `json:"style,omitempty" enum:"yellow,green,blue,pink,purple,underline" desc:"Default yellow"`
}

type Highlight struct {
	ID        string  `json:"id"`
	ItemID    string  `json:"item_id,omitempty"`
	Locator   *Anchor `json:"locator,omitempty"`
	Ref       string  `json:"ref,omitempty" desc:"The highlighted verses, for a Bible highlight"`
	Style     string  `json:"style"`
	CreatedAt int64   `json:"created_at"`
}

func (s *service) highlightCreate(ctx context.Context, env *command.Env, p HighlightCreateParams) (Highlight, error) {
	anchor := store.Row{"id": store.NewID()}
	switch {
	case p.ItemID != "" && p.Locator != nil && p.Ref == "":
		if !p.Locator.valid() {
			return Highlight{}, command.Errorf("bad_params", "highlight.create: the locator needs a section and a paragraph or page")
		}
		if err := itemExists(ctx, env.Store, p.ItemID); err != nil {
			return Highlight{}, err
		}
		raw, _ := json.Marshal(p.Locator)
		anchor["item_id"], anchor["locator_json"] = p.ItemID, string(raw)
	case p.Ref != "" && p.ItemID == "" && p.Locator == nil:
		r, err := verse.Parse(p.Ref)
		if err != nil {
			return Highlight{}, command.Errorf("bad_params", "%v", err)
		}
		anchor["verse_start"], anchor["verse_end"], anchor["versification"] = int64(r.Start), int64(r.End), string(verse.English)
	default:
		return Highlight{}, command.Errorf("bad_params", "highlight.create: give item_id with locator, or ref")
	}
	if p.Style == "" {
		p.Style = "yellow"
	}
	id := store.NewID()
	err := env.Store.Write(ctx, "highlight.create", func(tx *store.Tx) error {
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
	return queryHighlights(ctx, env.Store, where, args...)
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
	rows, err := st.Query(ctx, `SELECT h.id, h.style, h.created_at, a.item_id, a.locator_json, a.verse_start, a.verse_end
		FROM highlights h JOIN anchors a ON a.id = h.anchor_id WHERE h.deleted_at IS NULL`+where+
		` ORDER BY h.created_at, h.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Highlight{}
	for rows.Next() {
		var h Highlight
		var item, loc sql.NullString
		var start, end sql.NullInt64
		if err := rows.Scan(&h.ID, &h.Style, &h.CreatedAt, &item, &loc, &start, &end); err != nil {
			return nil, err
		}
		h.ItemID = item.String
		if loc.Valid {
			var a Anchor
			if json.Unmarshal([]byte(loc.String), &a) == nil {
				h.Locator = &a
			}
		}
		if start.Valid && end.Valid {
			h.Ref = verse.Range{Start: verse.ID(start.Int64), End: verse.ID(end.Int64)}.String()
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
