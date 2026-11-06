package reader

import (
	"context"
	"database/sql"
	"encoding/json"
	"unicode"

	"github.com/Perlsowisdom/xegesis/internal/store"
)

// A book (EPUB) highlight is exact words too: from an offset in one paragraph of a section to an
// offset in the same or a later paragraph, the end not included. A new one over older ones takes
// its part out of them, as a Bible range does (hlrange.go), so two colours never cover the same
// words. PDF highlights are boxes on a page and are left alone.

type bookPoint struct{ para, off int }

func (a bookPoint) before(b bookPoint) bool {
	return a.para < b.para || (a.para == b.para && a.off < b.off)
}

type bookSpan struct{ start, end bookPoint }

func (s bookSpan) empty() bool { return !s.start.before(s.end) }

func (s bookSpan) overlaps(o bookSpan) bool { return s.start.before(o.end) && o.start.before(s.end) }

// span gives the words an anchor covers; ok is false for PDF boxes and for an anchor without an end.
func (a Anchor) span() (bookSpan, bool) {
	if a.Page > 0 || a.Para < 1 {
		return bookSpan{}, false
	}
	endPara, endOff := a.EndPara, a.EndOffset
	if endPara == 0 {
		endPara, endOff = a.Para, a.EndOffset
	}
	s := bookSpan{start: bookPoint{a.Para, a.Offset}, end: bookPoint{endPara, endOff}}
	return s, !s.empty()
}

// bookHL is a book highlight read for splitting.
type bookHL struct {
	id, anchor, style string
	created           int64
	loc               Anchor
	span              bookSpan
}

// bookOverlapping lists the highlights in the same section of a book that share words with s.
func bookOverlapping(ctx context.Context, st *store.Store, itemID string, section int, s bookSpan) ([]bookHL, error) {
	rows, err := st.Query(ctx, `SELECT h.id, h.anchor_id, h.style, h.created_at, a.locator_json FROM highlights h JOIN anchors a ON a.id = h.anchor_id
		WHERE h.deleted_at IS NULL AND a.item_id = ? AND a.locator_json IS NOT NULL`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []bookHL{}
	for rows.Next() {
		var x bookHL
		var loc sql.NullString
		if err := rows.Scan(&x.id, &x.anchor, &x.style, &x.created, &loc); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(loc.String), &x.loc) != nil || x.loc.Section != section {
			continue
		}
		sp, ok := x.loc.span()
		if !ok || !sp.overlaps(s) {
			continue
		}
		x.span = sp
		out = append(out, x)
	}
	return out, rows.Err()
}

// cut gives what is left of x after taking away s: nothing, one piece or two.
func (x bookHL) cut(s bookSpan) []bookSpan {
	out := []bookSpan{}
	if left := (bookSpan{x.span.start, s.start}); x.span.start.before(s.start) && !left.empty() {
		out = append(out, left)
	}
	if right := (bookSpan{s.end, x.span.end}); s.end.before(x.span.end) && !right.empty() {
		out = append(out, right)
	}
	return out
}

// tighten cuts a piece's words out of the old highlight's words and trims the piece to them. That is
// only exact for words in one paragraph whose text was kept as it is; otherwise the piece keeps its
// bounds and no text. ok is false when the cut words are blank, so the piece shows nothing.
func (x bookHL) tighten(p bookSpan) (out bookSpan, text string, ok bool) {
	q := []rune(x.loc.Text)
	if x.span.start.para != x.span.end.para || len(q) != x.span.end.off-x.span.start.off {
		return p, "", true
	}
	a, b := p.start.off-x.span.start.off, p.end.off-x.span.start.off
	a, b = max(0, min(a, len(q))), max(0, min(b, len(q)))
	for a < b && unicode.IsSpace(q[a]) {
		a++
	}
	for b > a && unicode.IsSpace(q[b-1]) {
		b--
	}
	if a >= b {
		return p, "", false
	}
	base := x.span.start.off
	return bookSpan{bookPoint{p.start.para, base + a}, bookPoint{p.end.para, base + b}}, string(q[a:b]), true
}

// splitBook takes the words s covers out of each overlapping book highlight: one wholly inside s is
// deleted, one s cuts is shortened, and one s falls inside becomes two. The pieces keep their colour
// and age.
func splitBook(tx *store.Tx, itemID string, s bookSpan, list []bookHL) error {
	for _, x := range list {
		kept := 0
		for _, p := range x.cut(s) {
			p, text, ok := x.tighten(p)
			if !ok {
				continue
			}
			loc := x.loc
			loc.Para, loc.Offset, loc.EndPara, loc.EndOffset, loc.Text = p.start.para, p.start.off, p.end.para, p.end.off, text
			raw, _ := json.Marshal(loc)
			if kept == 0 {
				if err := tx.Update("anchors", store.ByID(x.anchor), store.Row{"locator_json": string(raw)}); err != nil {
					return err
				}
			} else {
				id := store.NewID()
				if err := tx.Create("anchors", store.Row{"id": id, "item_id": itemID, "locator_json": string(raw)}); err != nil {
					return err
				}
				if err := tx.Create("highlights", store.Row{"id": store.NewID(), "anchor_id": id, "style": x.style, "created_at": x.created}); err != nil {
					return err
				}
			}
			kept++
		}
		if kept == 0 {
			if err := tx.Delete("highlights", store.ByID(x.id)); err != nil {
				return err
			}
		}
	}
	return nil
}
