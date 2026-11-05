// Package document holds the user's sermons, lessons and Bible studies: a title, a main passage, an
// outline of points (each optionally tied to verses) and the full text in Markdown. Bible text quoted
// into a document goes through document.quote, which keeps the Bible's licence: text the licence
// does not let the app store is not copied, and every quotation carries its credit line.
//
// Source: Phase 1 survey, row "Sermon or lesson builder with verses and outlines".
package document

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

const (
	maxTitle  = 300
	maxPoints = 200
	maxBody   = 4 << 20
)

// Kinds are the kinds of document, in the order the interface offers them.
var Kinds = []string{"sermon", "lesson", "study"}

// Register adds the document commands to r. Quoting calls bible.modules and bible.read through r.
func Register(r *command.Registry) {
	s := &service{reg: r}
	r.Add(
		command.Define("document.create", "Start a new sermon, lesson or Bible study.", true, s.create),
		command.Define("document.get", "Show one sermon, lesson or study with its outline and text.", false, s.get),
		command.Define("document.list", "List sermons, lessons and studies, most recently changed first.", false, s.list),
		command.Define("document.update", "Change a document's title, kind, main passage, outline, text, date preached or theme.", true, s.update),
		command.Define("document.delete", "Delete a document. It stays in document.deleted until restored.", true, s.delete),
		command.Define("document.restore", "Bring back a deleted document.", true, s.restore),
		command.Define("document.deleted", "List deleted documents, most recently deleted first.", false, s.deleted),
		command.Define("document.move", "File a sermon, lesson or study in a folder, or take it out of its folder.", true, s.move),
		command.Define("document.quote", "Quote a Bible passage into a document's text, with its reference and credit line, "+
			"as far as the Bible's licence allows.", true, s.quote),
	)
}

type service struct {
	reg *command.Registry
}

func now() int64 { return time.Now().UnixMilli() }

// Point is one point of an outline.
type Point struct {
	Heading string `json:"h" desc:"The point, in a few words"`
	Ref     string `json:"ref,omitempty" desc:"Verses the point rests on, e.g. \"Rom 8:31\""`
	Level   int    `json:"level,omitempty" desc:"1 for a main point (default), 2 for a sub-point"`
}

// outline is how outline_json is stored. Older rows may hold just the points as an array.
type outline struct {
	Passage  string   `json:"passage,omitempty"`  // the first passage, kept for older readers
	Passages []string `json:"passages,omitempty"` // every passage, in order
	Points   []Point  `json:"points"`
}

// all lists the outline's passages; a row from before several passages holds just Passage.
func (o outline) all() []string {
	if len(o.Passages) > 0 {
		return o.Passages
	}
	if o.Passage != "" {
		return []string{o.Passage}
	}
	return nil
}

func parseOutline(raw sql.NullString) outline {
	o := outline{Points: []Point{}}
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return o
	}
	if strings.HasPrefix(strings.TrimSpace(raw.String), "[") {
		json.Unmarshal([]byte(raw.String), &o.Points)
	} else {
		json.Unmarshal([]byte(raw.String), &o)
	}
	if o.Points == nil {
		o.Points = []Point{}
	}
	return o
}

// Document is a sermon, lesson or study as callers see it.
type Document struct {
	ID         string           `json:"id"`
	Kind       string           `json:"kind" enum:"sermon,lesson,study"`
	Title      string           `json:"title"`
	Passage    *core.RefResult  `json:"passage,omitempty" desc:"The main passage (the first of passages)"`
	Passages   []core.RefResult `json:"passages,omitempty" desc:"All the sermon's passages, in order"`
	Points     []Point          `json:"points" desc:"The outline, in order"`
	Body       string           `json:"body" desc:"The full text, in Markdown"`
	Words      int              `json:"words" desc:"Words in the text"`
	PreachedOn string           `json:"preached_on,omitempty" desc:"When it was preached or taught, as YYYY-MM-DD"`
	Theme      string           `json:"theme,omitempty" desc:"Its theme or topic, in a few words"`
	FolderID   string           `json:"folder_id,omitempty" desc:"The folder it is filed in (folder.list)"`
	CreatedAt  int64            `json:"created_at"`
	UpdatedAt  int64            `json:"updated_at"`
	DeletedAt  int64            `json:"deleted_at,omitempty"`
}

func refOf(s string) (*core.RefResult, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	r, err := verse.Parse(s)
	if err != nil {
		return nil, command.Errorf("bad_params", "%v", err)
	}
	osis := r.Start.OSIS()
	if r.End != r.Start {
		osis += "-" + r.End.OSIS()
	}
	return &core.RefResult{Start: r.Start, End: r.End, Display: r.String(), OSIS: osis}, nil
}

func cleanTitle(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxTitle {
		return "", command.Errorf("bad_params", "the title can be up to %d characters", maxTitle)
	}
	return s, nil
}

func cleanKind(k string) (string, error) {
	if k == "" {
		return "sermon", nil
	}
	for _, x := range Kinds {
		if k == x {
			return k, nil
		}
	}
	return "", command.Errorf("bad_params", "kind must be sermon, lesson or study")
}

// cleanOutline checks the main passage and points and returns outline_json.
func cleanOutline(passages []string, points []Point) (string, error) {
	o := outline{Points: []Point{}}
	if len(passages) > maxPassages {
		return "", command.Errorf("bad_params", "a document can have up to %d passages", maxPassages)
	}
	for _, x := range passages {
		r, err := refOf(x)
		if err != nil {
			return "", err
		}
		if r != nil {
			o.Passages = append(o.Passages, r.Display)
		}
	}
	if len(o.Passages) > 0 {
		o.Passage = o.Passages[0]
	}
	if len(points) > maxPoints {
		return "", command.Errorf("bad_params", "an outline can have up to %d points", maxPoints)
	}
	for _, p := range points {
		p.Heading = strings.Join(strings.Fields(p.Heading), " ")
		if p.Heading == "" && strings.TrimSpace(p.Ref) == "" {
			continue
		}
		if len(p.Heading) > maxTitle {
			return "", command.Errorf("bad_params", "an outline point can be up to %d characters", maxTitle)
		}
		if r, err := refOf(p.Ref); err != nil {
			return "", err
		} else if r != nil {
			p.Ref = r.Display
		} else {
			p.Ref = ""
		}
		if p.Level != 2 {
			p.Level = 1
		}
		o.Points = append(o.Points, p)
	}
	raw, err := json.Marshal(o)
	return string(raw), err
}

func words(s string) int { return len(strings.Fields(s)) }

const docSelect = `SELECT id, kind, title, body_md, outline_json, created_at, updated_at, COALESCE(deleted_at, 0),
	COALESCE(preached_on, ''), COALESCE(theme, ''), COALESCE(folder_id, '') FROM documents`

func scan(rows *sql.Rows) (Document, error) {
	var d Document
	var raw sql.NullString
	if err := rows.Scan(&d.ID, &d.Kind, &d.Title, &d.Body, &raw, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt,
		&d.PreachedOn, &d.Theme, &d.FolderID); err != nil {
		return d, err
	}
	o := parseOutline(raw)
	d.Points = o.Points
	for _, x := range o.all() {
		if r, _ := refOf(x); r != nil {
			d.Passages = append(d.Passages, *r)
		}
	}
	if len(d.Passages) > 0 {
		d.Passage = &d.Passages[0]
	}
	d.Words = words(d.Body)
	return d, nil
}

func query(ctx context.Context, env *command.Env, where string, args ...any) ([]Document, error) {
	rows, err := env.Store.Query(ctx, docSelect+" "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func get(ctx context.Context, env *command.Env, id string) (Document, error) {
	list, err := query(ctx, env, `WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return Document{}, err
	}
	if len(list) == 0 {
		return Document{}, command.Errorf("not_found", "no document %s", id)
	}
	return list[0], nil
}

func needStore(env *command.Env) error {
	if env.Store == nil {
		return command.Errorf("unavailable", "no library is open")
	}
	return nil
}

// Commands -----------------------------------------------------------------------------------------

type CreateParams struct {
	Kind     string   `json:"kind,omitempty" enum:"sermon,lesson,study" desc:"Default sermon"`
	Title    string   `json:"title,omitempty" desc:"Default \"Untitled\" with the kind"`
	Passage  string   `json:"passage,omitempty" desc:"The main passage, e.g. \"Rom 8:28-39\""`
	Passages []string `json:"passages,omitempty" desc:"Several passages, in order (instead of passage)"`
	Points   []Point  `json:"points,omitempty" desc:"The outline"`
	Body     string   `json:"body,omitempty" desc:"The full text, in Markdown"`
	// Details a sermon list shows.
	PreachedOn string `json:"preached_on,omitempty" desc:"When it was preached or taught, as YYYY-MM-DD"`
	Theme      string `json:"theme,omitempty" desc:"Its theme or topic, in a few words"`
	FolderID   string `json:"folder_id,omitempty" desc:"File it in this folder (folder.list)"`
}

const maxTheme = 200

const maxPassages = 20

// cleanDate checks a YYYY-MM-DD date; "" means none.
func cleanDate(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return nil, command.Errorf("bad_params", "the date preached must be YYYY-MM-DD, e.g. 2026-11-01")
	}
	return s, nil
}

func cleanTheme(s string) (any, error) {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxTheme {
		return nil, command.Errorf("bad_params", "the theme can be up to %d characters", maxTheme)
	}
	if s == "" {
		return nil, nil
	}
	return s, nil
}

// untitled is the title a document gets when none is given.
func untitled(kind string) string {
	return map[string]string{"sermon": "Untitled sermon", "lesson": "Untitled lesson", "study": "Untitled study"}[kind]
}

func (s *service) create(ctx context.Context, env *command.Env, p CreateParams) (Document, error) {
	kind, err := cleanKind(p.Kind)
	if err != nil {
		return Document{}, err
	}
	title, err := cleanTitle(p.Title)
	if err != nil {
		return Document{}, err
	}
	if title == "" {
		title = untitled(kind)
	}
	passages := p.Passages
	if passages == nil {
		passages = []string{p.Passage}
	}
	ol, err := cleanOutline(passages, p.Points)
	if err != nil {
		return Document{}, err
	}
	if len(p.Body) > maxBody {
		return Document{}, command.Errorf("bad_params", "the text is too long")
	}
	preached, err := cleanDate(p.PreachedOn)
	if err != nil {
		return Document{}, err
	}
	theme, err := cleanTheme(p.Theme)
	if err != nil {
		return Document{}, err
	}
	id, t := store.NewID(), now()
	err = env.Store.Write(ctx, "document.create", func(tx *store.Tx) error {
		var folder any
		if p.FolderID != "" {
			if err := core.LiveFolder(tx, p.FolderID); err != nil {
				return err
			}
			folder = p.FolderID
		}
		return tx.Create("documents", store.Row{"id": id, "kind": kind, "title": title, "body_md": p.Body,
			"outline_json": ol, "created_at": t, "updated_at": t, "preached_on": preached, "theme": theme, "folder_id": folder})
	})
	if err != nil {
		return Document{}, err
	}
	return get(ctx, env, id)
}

type IDParams struct {
	ID string `json:"id" required:"true"`
}

func (s *service) get(ctx context.Context, env *command.Env, p IDParams) (Document, error) {
	if err := needStore(env); err != nil {
		return Document{}, err
	}
	return get(ctx, env, p.ID)
}

type ListParams struct {
	Kind   string `json:"kind,omitempty" enum:"sermon,lesson,study" desc:"Only this kind"`
	Query  string `json:"query,omitempty" desc:"Only documents whose title, passage, outline or text contains these words"`
	Ref    string `json:"ref,omitempty" desc:"Only documents whose main passage or outline points overlap this reference"`
	Folder string `json:"folder_id,omitempty" desc:"Only documents filed in this folder, or none for those in no folder"`
	Limit  int    `json:"limit,omitempty" desc:"Most documents to return (default 200)"`
}

func (s *service) list(ctx context.Context, env *command.Env, p ListParams) ([]Document, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	if p.Limit <= 0 {
		p.Limit = 200
	}
	where, args := `WHERE deleted_at IS NULL`, []any{}
	if p.Kind != "" {
		if _, err := cleanKind(p.Kind); err != nil {
			return nil, err
		}
		where += ` AND kind = ?`
		args = append(args, p.Kind)
	}
	for _, w := range strings.Fields(strings.ToLower(p.Query)) {
		where += ` AND instr(lower(title || ' ' || body_md || ' ' || COALESCE(outline_json, '') || ' ' || COALESCE(theme, '')), ?) > 0`
		args = append(args, w)
	}
	switch p.Folder {
	case "":
	case "none":
		where += ` AND folder_id IS NULL`
	default:
		where += ` AND folder_id = ?`
		args = append(args, p.Folder)
	}
	all, err := query(ctx, env, where+` ORDER BY updated_at DESC, rowid DESC`, args...)
	if err != nil {
		return nil, err
	}
	var want *verse.Range
	if p.Ref != "" {
		r, err := verse.Parse(p.Ref)
		if err != nil {
			return nil, command.Errorf("bad_params", "%v", err)
		}
		want = &r
	}
	out := []Document{}
	for _, d := range all {
		if want != nil && !touches(d, *want) {
			continue
		}
		out = append(out, d)
		if len(out) == p.Limit {
			break
		}
	}
	return out, nil
}

// touches reports whether a document's main passage or any point's verses overlap r.
func touches(d Document, r verse.Range) bool {
	over := func(x *core.RefResult) bool { return x != nil && x.Start <= r.End && x.End >= r.Start }
	for i := range d.Passages {
		if over(&d.Passages[i]) {
			return true
		}
	}
	for _, p := range d.Points {
		if x, err := refOf(p.Ref); err == nil && over(x) {
			return true
		}
	}
	return false
}

type UpdateParams struct {
	ID       string    `json:"id" required:"true"`
	Kind     *string   `json:"kind,omitempty" enum:"sermon,lesson,study" desc:"Omit to keep"`
	Title    *string   `json:"title,omitempty" desc:"Omit to keep"`
	Passage  *string   `json:"passage,omitempty" desc:"The main passage; \"\" to clear; omit to keep"`
	Passages *[]string `json:"passages,omitempty" desc:"All the passages, in order (instead of passage); [] to clear; omit to keep"`
	Points   *[]Point  `json:"points,omitempty" desc:"The whole outline; omit to keep"`
	Body     *string   `json:"body,omitempty" desc:"The full text in Markdown; omit to keep"`
	// A sermon's details.
	PreachedOn *string `json:"preached_on,omitempty" desc:"When it was preached, as YYYY-MM-DD; \"\" to clear; omit to keep"`
	Theme      *string `json:"theme,omitempty" desc:"Its theme or topic; \"\" to clear; omit to keep"`
}

func live(tx *store.Tx, id string) (store.Row, error) {
	cur, err := tx.Get("documents", store.ByID(id))
	if err != nil {
		return nil, err
	}
	if cur["deleted_at"] != nil {
		return nil, command.Errorf("not_found", "document %s is deleted", id)
	}
	return cur, nil
}

// bumped is an updated_at that moves forward even for two edits within one millisecond.
func bumped(cur store.Row) int64 {
	t := now()
	if prev, ok := cur["updated_at"].(int64); ok && t <= prev {
		t = prev + 1
	}
	return t
}

func (s *service) update(ctx context.Context, env *command.Env, p UpdateParams) (Document, error) {
	fields := store.Row{}
	if p.Kind != nil {
		k, err := cleanKind(*p.Kind)
		if err != nil {
			return Document{}, err
		}
		fields["kind"] = k
	}
	if p.Body != nil {
		if len(*p.Body) > maxBody {
			return Document{}, command.Errorf("bad_params", "the text is too long")
		}
		fields["body_md"] = *p.Body
	}
	if p.PreachedOn != nil {
		v, err := cleanDate(*p.PreachedOn)
		if err != nil {
			return Document{}, err
		}
		fields["preached_on"] = v
	}
	if p.Theme != nil {
		v, err := cleanTheme(*p.Theme)
		if err != nil {
			return Document{}, err
		}
		fields["theme"] = v
	}
	var title string
	if p.Title != nil {
		var err error
		if title, err = cleanTitle(*p.Title); err != nil {
			return Document{}, err
		}
	}
	err := env.Store.Write(ctx, "document.update", func(tx *store.Tx) error {
		cur, err := live(tx, p.ID)
		if err != nil {
			return err
		}
		if p.Title != nil {
			if title == "" {
				k, _ := cur["kind"].(string)
				if x, ok := fields["kind"].(string); ok {
					k = x
				}
				title = untitled(k)
			}
			fields["title"] = title
		}
		if p.Passage != nil || p.Passages != nil || p.Points != nil {
			raw, _ := cur["outline_json"].(string)
			old := parseOutline(sql.NullString{String: raw, Valid: raw != ""})
			passages, points := old.all(), old.Points
			if p.Passage != nil {
				passages = []string{*p.Passage}
			}
			if p.Passages != nil {
				passages = *p.Passages
			}
			if p.Points != nil {
				points = *p.Points
			}
			ol, err := cleanOutline(passages, points)
			if err != nil {
				return err
			}
			fields["outline_json"] = ol
		}
		if len(fields) == 0 {
			return nil
		}
		fields["updated_at"] = bumped(cur)
		return tx.Update("documents", store.ByID(p.ID), fields)
	})
	if err != nil {
		return Document{}, err
	}
	return get(ctx, env, p.ID)
}

func (s *service) delete(ctx context.Context, env *command.Env, p IDParams) (struct{}, error) {
	return struct{}{}, env.Store.Write(ctx, "document.delete", func(tx *store.Tx) error {
		if _, err := live(tx, p.ID); err != nil {
			return err
		}
		return tx.Delete("documents", store.ByID(p.ID))
	})
}

func (s *service) restore(ctx context.Context, env *command.Env, p IDParams) (Document, error) {
	err := env.Store.Write(ctx, "document.restore", func(tx *store.Tx) error {
		cur, err := tx.Get("documents", store.ByID(p.ID))
		if err != nil {
			return err
		}
		if cur["deleted_at"] == nil {
			return command.Errorf("bad_params", "document %s is not deleted", p.ID)
		}
		fields := store.Row{"deleted_at": nil}
		core.UnfileIfGone(tx, cur, fields)
		return tx.Update("documents", store.ByID(p.ID), fields)
	})
	if err != nil {
		return Document{}, err
	}
	return get(ctx, env, p.ID)
}

type DeletedParams struct {
	Limit int `json:"limit,omitempty" desc:"Most documents to return (default 100)"`
}

func (s *service) deleted(ctx context.Context, env *command.Env, p DeletedParams) ([]Document, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	if p.Limit <= 0 {
		p.Limit = 100
	}
	return query(ctx, env, `WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC, id LIMIT ?`, p.Limit)
}

type MoveParams struct {
	ID       string `json:"id" required:"true"`
	FolderID string `json:"folder_id,omitempty" desc:"The folder to file it in (folder.list); empty takes it out of its folder"`
}

// move files a document in a folder. It is not an edit, so updated_at stays.
func (s *service) move(ctx context.Context, env *command.Env, p MoveParams) (Document, error) {
	err := env.Store.Write(ctx, "document.move", func(tx *store.Tx) error {
		if _, err := live(tx, p.ID); err != nil {
			return err
		}
		var folder any
		if p.FolderID != "" {
			if err := core.LiveFolder(tx, p.FolderID); err != nil {
				return err
			}
			folder = p.FolderID
		}
		return tx.Update("documents", store.ByID(p.ID), store.Row{"folder_id": folder})
	})
	if err != nil {
		return Document{}, err
	}
	return get(ctx, env, p.ID)
}
