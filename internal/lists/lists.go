// Package lists holds the user's saved lists: verse lists, word lists (Strong's numbers and lemmas
// from the word panel) and excerpt lists (selected book text). Excerpts are copies, so they pass
// through book.copy and keep its licence limits and attribution.
package lists

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/passage"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

const maxName = 200

// Register adds the list commands to r. Adding an excerpt calls book.copy through r, so the
// reader's commands must be registered too.
func Register(r *command.Registry) { register(r, &service{reg: r}) }

func register(r *command.Registry, s *service) {
	r.Add(
		command.Define("list.create", "Make a new empty list of verses, words or excerpts.", true, s.create),
		command.Define("list.rename", "Rename a list.", true, s.rename),
		command.Define("list.delete", "Delete a list. It waits in the bin (bin.list) until restored or deleted for good.", true, s.delete),
		command.Define("list.list", "List the saved lists with how many entries each has, most recently changed first.", false, s.list),
		command.Define("list.add", "Add to a list: verses to a verse list, a word (with its Strong's number and lemma) to a word list, or selected book text to an excerpt list.", true, s.add),
		command.Define("list.remove", "Take one entry out of its list.", true, s.remove),
		command.Define("list.reorder", "Put a list's entries in a new order.", true, s.reorder),
		command.Define("list.items", "Show a list's entries in order.", false, s.items),
	)
}

type service struct {
	reg *command.Registry
}

func now() int64 { return time.Now().UnixMilli() }

func needStore(env *command.Env) error {
	if env.Store == nil {
		return command.Errorf("unavailable", "no library is open")
	}
	return nil
}

type List struct {
	ID        string `json:"id"`
	Kind      string `json:"kind" enum:"verses,words,excerpts"`
	Name      string `json:"name"`
	Count     int    `json:"count" desc:"Entries in the list"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type CreateParams struct {
	Kind string `json:"kind" required:"true" enum:"verses,words,excerpts"`
	Name string `json:"name" required:"true"`
}

func cleanName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || len(s) > maxName {
		return "", command.Errorf("bad_params", "give the list a name of up to %d characters", maxName)
	}
	return s, nil
}

func (s *service) create(ctx context.Context, env *command.Env, p CreateParams) (List, error) {
	name, err := cleanName(p.Name)
	if err != nil {
		return List{}, err
	}
	switch p.Kind {
	case "verses", "words", "excerpts":
	default:
		return List{}, command.Errorf("bad_params", "list.create: kind must be verses, words or excerpts")
	}
	id, t := store.NewID(), now()
	err = env.Store.Write(ctx, "list.create", func(tx *store.Tx) error {
		return tx.Create("lists", store.Row{"id": id, "kind": p.Kind, "name": name, "created_at": t, "updated_at": t})
	})
	if err != nil {
		return List{}, err
	}
	return getList(ctx, env, id)
}

const listSelect = `SELECT l.id, l.kind, l.name, l.created_at, l.updated_at,
	(SELECT COUNT(*) FROM list_entries e WHERE e.list_id = l.id) FROM lists l WHERE l.deleted_at IS NULL`

func scanList(rows *sql.Rows) (List, error) {
	var l List
	err := rows.Scan(&l.ID, &l.Kind, &l.Name, &l.CreatedAt, &l.UpdatedAt, &l.Count)
	return l, err
}

func getList(ctx context.Context, env *command.Env, id string) (List, error) {
	rows, err := env.Store.Query(ctx, listSelect+` AND l.id = ?`, id)
	if err != nil {
		return List{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return List{}, command.Errorf("not_found", "no list %s", id)
	}
	return scanList(rows)
}

type RenameParams struct {
	ID   string `json:"id" required:"true"`
	Name string `json:"name" required:"true"`
}

// liveList reads a list that has not been deleted.
func liveList(tx *store.Tx, id string) (store.Row, error) {
	cur, err := tx.Get("lists", store.ByID(id))
	if err != nil {
		return nil, err
	}
	if cur["deleted_at"] != nil {
		return nil, command.Errorf("not_found", "list %s is deleted", id)
	}
	return cur, nil
}

// touch moves a list's updated_at forward.
func touch(tx *store.Tx, cur store.Row) error {
	t := now()
	if prev, ok := cur["updated_at"].(int64); ok && t <= prev {
		t = prev + 1
	}
	return tx.Update("lists", store.ByID(cur["id"].(string)), store.Row{"updated_at": t})
}

func (s *service) rename(ctx context.Context, env *command.Env, p RenameParams) (List, error) {
	name, err := cleanName(p.Name)
	if err != nil {
		return List{}, err
	}
	err = env.Store.Write(ctx, "list.rename", func(tx *store.Tx) error {
		cur, err := liveList(tx, p.ID)
		if err != nil {
			return err
		}
		if err := tx.Update("lists", store.ByID(p.ID), store.Row{"name": name}); err != nil {
			return err
		}
		return touch(tx, cur)
	})
	if err != nil {
		return List{}, err
	}
	return getList(ctx, env, p.ID)
}

type IDParams struct {
	ID string `json:"id" required:"true"`
}

func (s *service) delete(ctx context.Context, env *command.Env, p IDParams) (struct{}, error) {
	return struct{}{}, env.Store.Write(ctx, "list.delete", func(tx *store.Tx) error {
		if _, err := liveList(tx, p.ID); err != nil {
			return err
		}
		return tx.Delete("lists", store.ByID(p.ID))
	})
}

type ListParams struct {
	Kind string `json:"kind,omitempty" enum:"verses,words,excerpts" desc:"Only lists of this kind"`
}

func (s *service) list(ctx context.Context, env *command.Env, p ListParams) ([]List, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	q, args := listSelect, []any{}
	if p.Kind != "" {
		q += ` AND l.kind = ?`
		args = append(args, p.Kind)
	}
	rows, err := env.Store.Query(ctx, q+` ORDER BY l.updated_at DESC, l.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []List{}
	for rows.Next() {
		l, err := scanList(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Entries -----------------------------------------------------------------------------------------

type Entry struct {
	ID          string          `json:"id"`
	Position    int             `json:"position"`
	Ref         *core.RefResult `json:"ref,omitempty" desc:"Verse lists: the verses, in English numbering"`
	Word        string          `json:"word,omitempty" desc:"Word lists: the word as shown"`
	Strong      string          `json:"strong,omitempty" desc:"Word lists: Strong's number, e.g. G26"`
	Lemma       string          `json:"lemma,omitempty" desc:"Word lists: the dictionary form"`
	Language    string          `json:"language,omitempty"`
	ItemID      string          `json:"item_id,omitempty" desc:"Excerpt lists: the book the text is from"`
	Title       string          `json:"title,omitempty" desc:"Excerpt lists: that book's title"`
	Locator     map[string]any  `json:"locator,omitempty" desc:"Excerpt lists: where in the book, as highlight.create's locator"`
	Text        string          `json:"text,omitempty" desc:"Excerpt lists: the copied text"`
	Attribution string          `json:"attribution,omitempty" desc:"Excerpt lists: the credit that goes with the text"`
	Note        string          `json:"note,omitempty"`
	Already     bool            `json:"already,omitempty" desc:"list.add: the list already had this, so nothing was added"`
}

type Items struct {
	List    List    `json:"list"`
	Entries []Entry `json:"entries"`
}

const entrySelect = `SELECT e.id, e.position, e.word, e.strong, e.lemma, e.language, e.note, e.attribution,
	a.verse_start, a.verse_end, a.item_id, a.locator_json, i.title
	FROM list_entries e LEFT JOIN anchors a ON a.id = e.anchor_id LEFT JOIN items i ON i.id = a.item_id`

func queryEntries(ctx context.Context, env *command.Env, where string, args ...any) ([]Entry, error) {
	rows, err := env.Store.Query(ctx, entrySelect+where+` ORDER BY e.position, e.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		var word, strong, lemma, lang, note, attr, item, loc, title sql.NullString
		var start, end sql.NullInt64
		if err := rows.Scan(&e.ID, &e.Position, &word, &strong, &lemma, &lang, &note, &attr, &start, &end, &item, &loc, &title); err != nil {
			return nil, err
		}
		e.Word, e.Strong, e.Lemma, e.Language, e.Note = word.String, strong.String, lemma.String, lang.String, note.String
		e.Attribution, e.ItemID, e.Title = attr.String, item.String, title.String
		if start.Valid && end.Valid {
			r := verse.Range{Start: verse.ID(start.Int64), End: verse.ID(end.Int64)}
			e.Ref = &core.RefResult{Start: r.Start, End: r.End, Display: r.String(), OSIS: osis(r)}
		}
		if loc.Valid {
			json.Unmarshal([]byte(loc.String), &e.Locator)
			e.Text, _ = e.Locator["text"].(string)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func osis(r verse.Range) string {
	o := r.Start.OSIS()
	if r.End != r.Start {
		o += "-" + r.End.OSIS()
	}
	return o
}

func (s *service) items(ctx context.Context, env *command.Env, p IDParams) (Items, error) {
	if err := needStore(env); err != nil {
		return Items{}, err
	}
	l, err := getList(ctx, env, p.ID)
	if err != nil {
		return Items{}, err
	}
	es, err := queryEntries(ctx, env, ` WHERE e.list_id = ?`, p.ID)
	return Items{List: l, Entries: es}, err
}

type AddParams struct {
	List string `json:"list" required:"true" desc:"The list's ID"`
	// Verse lists.
	Ref           string `json:"ref,omitempty" desc:"Verse lists: the passage, e.g. \"John 3:16-18\""`
	VerseStart    int    `json:"verse_start,omitempty" desc:"Verse lists: or the first verse ID (BBCCCVVV), with verse_end"`
	VerseEnd      int    `json:"verse_end,omitempty"`
	Versification string `json:"versification,omitempty" enum:"english,hebrew" desc:"Numbering of the verses given; default english"`
	// Word lists.
	Word     string `json:"word,omitempty" desc:"Word lists: the word as shown in the text"`
	Strong   string `json:"strong,omitempty" desc:"Word lists: Strong's number, e.g. G26"`
	Lemma    string `json:"lemma,omitempty" desc:"Word lists: the dictionary form"`
	Language string `json:"language,omitempty" desc:"Word lists: language code, e.g. grc or hbo"`
	// Excerpt lists.
	ItemID  string         `json:"item_id,omitempty" desc:"Excerpt lists: the book"`
	FileID  string         `json:"file_id,omitempty" desc:"Excerpt lists: which of the book's files"`
	Locator map[string]any `json:"locator,omitempty" desc:"Excerpt lists: where in the book, as highlight.create's locator"`
	Text    string         `json:"text,omitempty" desc:"Excerpt lists: the selected text; it counts as a copy under the book's licence"`

	Note string `json:"note,omitempty" desc:"Your own comment on this entry"`
}

type copyResult struct {
	Allowed     bool   `json:"allowed"`
	Attribution string `json:"attribution"`
	Reason      string `json:"reason"`
}

func (s *service) add(ctx context.Context, env *command.Env, p AddParams) (Entry, error) {
	var kind string
	err := func() error {
		rows, err := env.Store.Query(ctx, `SELECT kind FROM lists WHERE id = ? AND deleted_at IS NULL`, p.List)
		if err != nil {
			return err
		}
		defer rows.Close()
		if !rows.Next() {
			return command.Errorf("not_found", "no list %s", p.List)
		}
		return rows.Scan(&kind)
	}()
	if err != nil {
		return Entry{}, err
	}
	var pos int
	if err := env.Store.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM list_entries WHERE list_id = ?`, p.List).Scan(&pos); err != nil {
		return Entry{}, err
	}
	id := store.NewID()
	row := store.Row{"id": id, "list_id": p.List, "position": int64(pos), "note": nullable(strings.TrimSpace(p.Note))}
	anchor := store.Row{"id": store.NewID()}
	var dup string
	var dupArgs []any
	switch kind {
	case "verses":
		r, ok, err := passage.Resolve("list.add", p.Ref, p.VerseStart, p.VerseEnd, p.Versification)
		if err != nil {
			return Entry{}, err
		}
		if !ok {
			return Entry{}, command.Errorf("bad_params", "list.add: this is a list of verses; give ref")
		}
		anchor["verse_start"], anchor["verse_end"], anchor["versification"] = int64(r.Start), int64(r.End), string(verse.English)
		dup, dupArgs = ` AND a.verse_start = ? AND a.verse_end = ?`, []any{int64(r.Start), int64(r.End)}
	case "words":
		anchor = nil
		strong := normStrong(p.Strong)
		word := strings.TrimSpace(p.Word)
		if word == "" {
			word = strings.TrimSpace(p.Lemma)
		}
		if word == "" {
			word = strong
		}
		if word == "" {
			return Entry{}, command.Errorf("bad_params", "list.add: this is a list of words; give word or strong")
		}
		row["word"], row["strong"], row["lemma"] = word, nullable(strong), nullable(strings.TrimSpace(p.Lemma))
		row["language"] = nullable(strings.TrimSpace(p.Language))
		if strong != "" {
			dup, dupArgs = ` AND e.strong = ?`, []any{strong}
		} else {
			dup, dupArgs = ` AND e.strong IS NULL AND LOWER(e.word) = ?`, []any{strings.ToLower(word)}
		}
	case "excerpts":
		loc, err := s.excerpt(ctx, env, p)
		if err != nil {
			return Entry{}, err
		}
		anchor["item_id"], anchor["locator_json"] = p.ItemID, loc.locator
		row["attribution"] = nullable(loc.attribution)
	}
	if dup != "" {
		have, err := queryEntries(ctx, env, ` WHERE e.list_id = ?`+dup, append([]any{p.List}, dupArgs...)...)
		if err != nil {
			return Entry{}, err
		}
		if len(have) > 0 {
			have[0].Already = true
			return have[0], nil
		}
	}
	err = env.Store.Write(ctx, "list.add", func(tx *store.Tx) error {
		cur, err := liveList(tx, p.List)
		if err != nil {
			return err
		}
		if anchor != nil {
			if err := tx.Create("anchors", anchor); err != nil {
				return err
			}
			row["anchor_id"] = anchor["id"]
		}
		if err := tx.Create("list_entries", row); err != nil {
			return err
		}
		return touch(tx, cur)
	})
	if err != nil {
		return Entry{}, err
	}
	es, err := queryEntries(ctx, env, ` WHERE e.id = ?`, id)
	if err != nil || len(es) == 0 {
		return Entry{}, err
	}
	return es[0], nil
}

type excerpt struct {
	locator     string
	attribution string
}

// excerpt checks an excerpt's place in its book and copies its text under the book's licence.
func (s *service) excerpt(ctx context.Context, env *command.Env, p AddParams) (excerpt, error) {
	text := strings.TrimSpace(p.Text)
	if p.ItemID == "" || p.Locator == nil || text == "" {
		return excerpt{}, command.Errorf("bad_params", "list.add: this is a list of excerpts; give item_id, locator and text")
	}
	if n, ok := p.Locator["section"].(float64); !ok || n < 1 {
		return excerpt{}, command.Errorf("bad_params", "list.add: the locator needs a section of 1 or more")
	}
	loc := map[string]any{}
	for k, v := range p.Locator {
		loc[k] = v
	}
	loc["text"] = text
	raw, err := json.Marshal(loc)
	if err != nil || len(raw) > 64<<10 {
		return excerpt{}, command.Errorf("bad_params", "list.add: the locator is not valid")
	}
	params, _ := json.Marshal(map[string]any{"item_id": p.ItemID, "file_id": p.FileID, "text": text})
	res, err := s.reg.Run(ctx, env, "book.copy", params)
	if err != nil {
		return excerpt{}, err
	}
	var cr copyResult
	b, _ := json.Marshal(res)
	if err := json.Unmarshal(b, &cr); err != nil {
		return excerpt{}, err
	}
	if !cr.Allowed {
		return excerpt{}, command.Errorf("not_allowed", "this text cannot be added to a list: %s", cr.Reason)
	}
	return excerpt{locator: string(raw), attribution: cr.Attribution}, nil
}

// normStrong upper-cases a Strong's number such as "g26" and drops spaces.
func normStrong(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *service) remove(ctx context.Context, env *command.Env, p IDParams) (struct{}, error) {
	return struct{}{}, env.Store.Write(ctx, "list.remove", func(tx *store.Tx) error {
		e, err := tx.Get("list_entries", store.ByID(p.ID))
		if err != nil {
			return err
		}
		l, err := liveList(tx, e["list_id"].(string))
		if err != nil {
			return err
		}
		if err := tx.Delete("list_entries", store.ByID(p.ID)); err != nil {
			return err
		}
		return touch(tx, l)
	})
}

type ReorderParams struct {
	List string   `json:"list" required:"true"`
	IDs  []string `json:"ids" required:"true" desc:"Entry IDs in the new order; entries left out follow in their old order"`
}

func (s *service) reorder(ctx context.Context, env *command.Env, p ReorderParams) (Items, error) {
	es, err := queryEntries(ctx, env, ` WHERE e.list_id = ?`, p.List)
	if err != nil {
		return Items{}, err
	}
	have := map[string]bool{}
	for _, e := range es {
		have[e.ID] = true
	}
	order := make([]string, 0, len(es))
	seen := map[string]bool{}
	for _, id := range p.IDs {
		if !have[id] || seen[id] {
			return Items{}, command.Errorf("bad_params", "list.reorder: %s is not an entry of this list, or is repeated", id)
		}
		seen[id] = true
		order = append(order, id)
	}
	for _, e := range es {
		if !seen[e.ID] {
			order = append(order, e.ID)
		}
	}
	was := map[string]int{}
	for _, e := range es {
		was[e.ID] = e.Position
	}
	err = env.Store.Write(ctx, "list.reorder", func(tx *store.Tx) error {
		cur, err := liveList(tx, p.List)
		if err != nil {
			return err
		}
		moved := false
		for i, id := range order {
			if was[id] == i+1 {
				continue
			}
			moved = true
			if err := tx.Update("list_entries", store.ByID(id), store.Row{"position": int64(i + 1)}); err != nil {
				return err
			}
		}
		if moved {
			return touch(tx, cur)
		}
		return nil
	})
	if err != nil {
		return Items{}, err
	}
	return s.items(ctx, env, IDParams{ID: p.List})
}
