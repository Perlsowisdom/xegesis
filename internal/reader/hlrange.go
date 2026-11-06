package reader

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// A Bible highlight may cover exact words: a range from a character in one verse to a character in
// the same or a later verse. Offsets count characters (Unicode code points) of the verse's plain
// text as the app draws it, without verse numbers or notes, so they depend on the Bible the range
// was made in; it is kept with the range. The range is stored in the anchor's locator_json, and
// the anchor's verse_start and verse_end still name the verses, so lookups by verse keep working.
// Highlights without a range cover whole verses, as before.

// TextPoint is a place in a verse's text.
type TextPoint struct {
	Verse  int64 `json:"verse" desc:"Verse ID (BBCCCVVV), in English numbering"`
	Offset int   `json:"offset" desc:"Characters into the verse's text"`
}

// VerseRange is the exact words of a Bible highlight: from Start up to, not including, End.
type VerseRange struct {
	Start TextPoint `json:"start"`
	End   TextPoint `json:"end"`
}

// verseLocator is what a ranged Bible highlight keeps in locator_json.
type verseLocator struct {
	Range  VerseRange  `json:"range"`
	Module string      `json:"module"`
	Quote  string      `json:"quote,omitempty"`
	Parts  []QuotePart `json:"parts,omitempty"`
}

func before(a, b TextPoint) bool {
	return a.Verse < b.Verse || (a.Verse == b.Verse && a.Offset < b.Offset)
}

// empty reports whether the range covers no characters.
func (r VerseRange) empty() bool { return !before(r.Start, r.End) }

// overlaps reports whether two ranges share a character.
func (r VerseRange) overlaps(o VerseRange) bool {
	return before(r.Start, o.End) && before(o.Start, r.End)
}

// validRange checks a range given to highlight.create or highlight.update.
func validRange(id string, r VerseRange, module, quote string) error {
	switch {
	case !verse.ID(r.Start.Verse).Valid() || !verse.ID(r.End.Verse).Valid():
		return command.Errorf("bad_params", "%s: the range needs verse IDs (BBCCCVVV)", id)
	case r.Start.Offset < 0 || r.End.Offset < 0:
		return command.Errorf("bad_params", "%s: a range's offsets cannot be negative", id)
	case r.empty():
		return command.Errorf("bad_params", "%s: the range must end after it starts", id)
	case verse.ID(r.Start.Verse).Book() != verse.ID(r.End.Verse).Book():
		return command.Errorf("bad_params", "%s: a range stays within one book of the Bible", id)
	case module == "":
		return command.Errorf("bad_params", "%s: give the Bible (module) the range was made in", id)
	case len(quote) > 20000:
		return command.Errorf("bad_params", "%s: the quoted words are too long", id)
	}
	return nil
}

// cut gives what is left of r after taking away the part cut covers: nothing, one piece or two.
func (r VerseRange) cut(cut VerseRange) []VerseRange {
	if !r.overlaps(cut) {
		return []VerseRange{r}
	}
	out := []VerseRange{}
	if left := (VerseRange{Start: r.Start, End: cut.Start}); !left.empty() {
		out = append(out, left)
	}
	if right := (VerseRange{Start: cut.End, End: r.End}); !right.empty() {
		out = append(out, right)
	}
	return out
}

// ranged is a ranged highlight in the library, as read for splitting.
type ranged struct {
	id, anchor, style string
	created           int64
	loc               verseLocator
}

// overlapping lists the ranged highlights in module that share characters with r; skip is left out.
func overlapping(ctx context.Context, st *store.Store, r VerseRange, module, skip string) ([]ranged, error) {
	rows, err := st.Query(ctx, `SELECT h.id, h.anchor_id, h.style, h.created_at, a.locator_json FROM highlights h JOIN anchors a ON a.id = h.anchor_id
		WHERE h.deleted_at IS NULL AND a.item_id IS NULL AND a.verse_start <= ? AND a.verse_end >= ? AND a.locator_json IS NOT NULL AND h.id <> ?`,
		r.End.Verse, r.Start.Verse, skip)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ranged{}
	for rows.Next() {
		var x ranged
		var loc sql.NullString
		if err := rows.Scan(&x.id, &x.anchor, &x.style, &x.created, &loc); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(loc.String), &x.loc) != nil || x.loc.Module != module || !x.loc.Range.overlaps(r) {
			continue
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// split takes the part r covers out of each overlapping highlight, so two colours never cover the
// same words: a highlight wholly inside r is deleted, one that r cuts is shortened, and one that r
// falls inside becomes two. The pieces keep their colour, age and their share of the words.
func split(tx *store.Tx, r VerseRange, list []ranged) error {
	for _, x := range list {
		pieces := x.loc.Range.cut(r)
		if len(pieces) == 0 {
			if err := tx.Delete("highlights", store.ByID(x.id)); err != nil {
				return err
			}
			continue
		}
		for i, p := range pieces {
			parts := cutParts(x.loc, p)
			loc := verseLocator{Range: p, Module: x.loc.Module, Quote: joinParts(parts), Parts: parts}
			raw, _ := json.Marshal(loc)
			fields := store.Row{"verse_start": p.Start.Verse, "verse_end": p.End.Verse, "locator_json": string(raw)}
			if i == 0 {
				if err := tx.Update("anchors", store.ByID(x.anchor), fields); err != nil {
					return err
				}
				continue
			}
			fields["id"], fields["versification"] = store.NewID(), string(verse.English)
			if err := tx.Create("anchors", fields); err != nil {
				return err
			}
			if err := tx.Create("highlights", store.Row{"id": store.NewID(), "anchor_id": fields["id"], "style": x.style, "created_at": x.created}); err != nil {
				return err
			}
		}
	}
	return nil
}

// cutParts cuts a piece's words out of the old highlight's words, verse by verse: each part starts
// at the range's start offset in the first verse and at 0 in the others.
func cutParts(old verseLocator, p VerseRange) []QuotePart {
	out := []QuotePart{}
	for _, w := range old.Parts {
		if w.Verse < p.Start.Verse || w.Verse > p.End.Verse {
			continue
		}
		from := 0
		if w.Verse == old.Range.Start.Verse {
			from = old.Range.Start.Offset
		}
		q := []rune(w.Text)
		a, b := 0, len(q)
		if w.Verse == p.Start.Verse {
			a = p.Start.Offset - from
		}
		if w.Verse == p.End.Verse {
			b = p.End.Offset - from
		}
		a, b = max(0, min(a, len(q))), max(0, min(b, len(q)))
		if a < b {
			out = append(out, QuotePart{Verse: w.Verse, Text: string(q[a:b])})
		}
	}
	return out
}

// QuotePart is the highlighted words of one verse.
type QuotePart struct {
	Verse int64  `json:"verse" desc:"Verse ID"`
	Text  string `json:"text" desc:"The highlighted words of that verse, exactly as shown"`
}

// joinParts is the quote of a range's words: the verses' words with their spacing tidied.
func joinParts(parts []QuotePart) string {
	words := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.Join(strings.Fields(p.Text), " "); t != "" {
			words = append(words, t)
		}
	}
	return strings.Join(words, " ")
}
