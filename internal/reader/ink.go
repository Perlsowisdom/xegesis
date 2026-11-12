package reader

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Ink on a PDF page is one annotation per (book, page): a highlights row with the style "ink" whose
// anchor's locator_json holds the page and its strokes, so it is listed, backed up, exported and
// restored from history like any highlight. One row per page, not per stroke, keeps a page of
// handwriting to one row to list and one change to undo; the whole page of strokes is a few kilobytes.
// Points are fractions of the page box: x of its width, y of its height, so strokes line up at every
// zoom. The book file never changes.

const inkStyle = "ink"

// InkStroke is one pen or highlighter line. T is "pen" or "hl", C a colour name, W a width step.
type InkStroke struct {
	T string       `json:"t" desc:"pen or hl (highlighter)"`
	C string       `json:"c" desc:"ink, yellow, green, blue, pink or purple"`
	W int          `json:"w" desc:"Width step, 1 to 3"`
	P [][3]float64 `json:"p" desc:"Points [x, y, pressure]: x and y as fractions of the page's width and height; pressure 0 to 1, or 0 when the device gave none"`
}

// PageInk is the ink on one PDF page. In listings Strokes is left out and Count says how many.
type PageInk struct {
	Aspect  float64     `json:"aspect" desc:"The page's height divided by its width"`
	Strokes []InkStroke `json:"strokes,omitempty"`
	Count   int         `json:"count,omitempty" desc:"Number of strokes (listings leave the strokes out)"`
}

const (
	maxInkStrokes = 5000
	maxInkPoints  = 200000
	maxInkLine    = 20000
)

var inkColours = map[string]bool{"ink": true, "yellow": true, "green": true, "blue": true, "pink": true, "purple": true}

func (p PageInk) valid() error {
	if !(p.Aspect >= 0.2 && p.Aspect <= 5) {
		return command.Errorf("bad_params", "ink.page.set: aspect (page height over width) must be between 0.2 and 5")
	}
	if len(p.Strokes) > maxInkStrokes {
		return command.Errorf("bad_params", "ink.page.set: at most %d strokes on a page", maxInkStrokes)
	}
	total := 0
	for i, k := range p.Strokes {
		if (k.T != "pen" && k.T != "hl") || !inkColours[k.C] || k.W < 1 || k.W > 3 || len(k.P) == 0 || len(k.P) > maxInkLine {
			return command.Errorf("bad_params", "ink.page.set: stroke %d is not a pen or highlighter line with a colour, a width of 1 to 3 and points", i+1)
		}
		total += len(k.P)
		for _, q := range k.P {
			if math.IsNaN(q[0]+q[1]+q[2]) || math.IsInf(q[0]+q[1]+q[2], 0) || q[0] < -0.05 || q[0] > 1.05 || q[1] < -0.05 || q[1] > 1.05 || q[2] < 0 || q[2] > 1 {
				return command.Errorf("bad_params", "ink.page.set: stroke %d has a point off the page", i+1)
			}
		}
	}
	if total > maxInkPoints {
		return command.Errorf("bad_params", "ink.page.set: too many points on one page")
	}
	return nil
}

type InkGetParams struct {
	ItemID string `json:"item_id" required:"true" desc:"The PDF book"`
	Page   int    `json:"page" required:"true" desc:"1-based page"`
}

type InkSetParams struct {
	ItemID  string      `json:"item_id" required:"true" desc:"The PDF book"`
	Page    int         `json:"page" required:"true" desc:"1-based page"`
	Aspect  float64     `json:"aspect" required:"true" desc:"The page's height divided by its width"`
	Strokes []InkStroke `json:"strokes" desc:"All the strokes now on the page, replacing what was there; none clears the page"`
}

type InkPage struct {
	ID   string  `json:"id,omitempty" desc:"The ink annotation, for highlight.delete; empty when the page has no ink"`
	Page int     `json:"page"`
	Ink  PageInk `json:"ink"`
}

// inkRow finds a page's live ink annotation.
func inkRow(ctx context.Context, st *store.Store, itemID string, page int) (id, anchorID string, ink PageInk, err error) {
	var loc sql.NullString
	err = st.QueryRow(ctx, `SELECT h.id, h.anchor_id, a.locator_json FROM highlights h JOIN anchors a ON a.id = h.anchor_id
		WHERE h.deleted_at IS NULL AND h.style = ? AND a.item_id = ? AND json_extract(a.locator_json, '$.page') = ?`, inkStyle, itemID, page).Scan(&id, &anchorID, &loc)
	if err == sql.ErrNoRows {
		return "", "", PageInk{}, nil
	}
	if err != nil {
		return "", "", PageInk{}, err
	}
	var a Anchor
	if json.Unmarshal([]byte(loc.String), &a) == nil && a.Ink != nil {
		ink = *a.Ink
	}
	return id, anchorID, ink, nil
}

func (s *service) inkGet(ctx context.Context, env *command.Env, p InkGetParams) (InkPage, error) {
	if env.Store == nil {
		return InkPage{}, command.Errorf("unavailable", "no library is open")
	}
	id, _, ink, err := inkRow(ctx, env.Store, p.ItemID, p.Page)
	if err != nil {
		return InkPage{}, err
	}
	ink.Count = len(ink.Strokes)
	if ink.Strokes == nil {
		ink.Strokes = []InkStroke{}
	}
	return InkPage{ID: id, Page: p.Page, Ink: ink}, nil
}

func (s *service) inkSet(ctx context.Context, env *command.Env, p InkSetParams) (InkPage, error) {
	if env.Store == nil {
		return InkPage{}, command.Errorf("unavailable", "no library is open")
	}
	if p.Page < 1 {
		return InkPage{}, command.Errorf("bad_params", "ink.page.set: page is 1 or more")
	}
	ink := PageInk{Aspect: p.Aspect, Strokes: p.Strokes}
	if err := ink.valid(); err != nil {
		return InkPage{}, err
	}
	if err := itemExists(ctx, env.Store, p.ItemID); err != nil {
		return InkPage{}, err
	}
	id, anchorID, _, err := inkRow(ctx, env.Store, p.ItemID, p.Page)
	if err != nil {
		return InkPage{}, err
	}
	raw, _ := json.Marshal(Anchor{Section: p.Page, Page: p.Page, Text: fmt.Sprintf("Ink · page %d", p.Page), Ink: &ink})
	err = env.Store.Write(ctx, "ink.page.set", func(tx *store.Tx) error {
		if id != "" {
			return tx.Update("anchors", store.ByID(anchorID), store.Row{"locator_json": string(raw)})
		}
		if len(p.Strokes) == 0 {
			return nil
		}
		id, anchorID = store.NewID(), store.NewID()
		if err := tx.Create("anchors", store.Row{"id": anchorID, "item_id": p.ItemID, "locator_json": string(raw)}); err != nil {
			return err
		}
		return tx.Create("highlights", store.Row{"id": id, "anchor_id": anchorID, "style": inkStyle, "created_at": now()})
	})
	if err != nil {
		return InkPage{}, err
	}
	ink.Count = len(ink.Strokes)
	if ink.Strokes == nil {
		ink.Strokes = []InkStroke{}
	}
	return InkPage{ID: id, Page: p.Page, Ink: ink}, nil
}
