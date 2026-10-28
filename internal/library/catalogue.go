package library

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

type Facet struct {
	Facet  string `json:"facet" required:"true" desc:"e.g. subject, type, tradition"`
	Value  string `json:"value" required:"true"`
	Origin string `json:"origin,omitempty" enum:"metadata,bisac,user"`
}

type ItemsParams struct {
	Kind         string `json:"kind,omitempty" desc:"Only items of this kind, e.g. book, bible, commentary"`
	FolderID     string `json:"folder_id,omitempty" desc:"Only items with a file in this library folder"`
	Author       string `json:"author,omitempty" desc:"Text in a credited person's name"`
	Text         string `json:"text,omitempty" desc:"Text in the title or subtitle"`
	Facet        string `json:"facet,omitempty" desc:"Only items with this facet, e.g. subject"`
	Value        string `json:"value,omitempty" desc:"With facet (or alone), only items with this facet value"`
	NeedsLicence bool   `json:"needs_licence,omitempty" desc:"Only items with no licence recorded yet (older libraries)"`
	AddedSince   int64  `json:"added_since,omitempty" desc:"Only items added at or after this time (Unix milliseconds), e.g. the since of library.add"`
	LicenceType  string `json:"licence_type,omitempty" desc:"Only items with this licence type"`
	Limit        int    `json:"limit,omitempty" desc:"Most items to return (default 100, at most 1000)"`
	Offset       int    `json:"offset,omitempty"`
}

type ItemSummary struct {
	ID            string           `json:"id"`
	Kind          string           `json:"kind"`
	Title         string           `json:"title"`
	Subtitle      string           `json:"subtitle,omitempty"`
	Year          int              `json:"year,omitempty"`
	Language      string           `json:"language,omitempty"`
	Authors       []string         `json:"authors"`
	Status        string           `json:"status" enum:"online,offline,missing" desc:"online: a file can be opened now; offline: its folder's drive is not mounted; missing: its files are gone"`
	Licence       string           `json:"licence" desc:"Licence type"`
	LicenceName   string           `json:"licence_name" desc:"The licence in plain words: Public domain, Free to share, Personal copy or My own work"`
	LicenceOrigin string           `json:"licence_origin" enum:"file,default,user" desc:"file: from the book's own details; default: none stated; user: set by the user"`
	Suggested     *licence.Licence `json:"suggested_licence,omitempty" desc:"When no licence is recorded yet, the one the item will get"`
}

type ItemsResult struct {
	Items []ItemSummary `json:"items"`
	Total int           `json:"total" desc:"Matching items before paging"`
}

func listItems(ctx context.Context, env *command.Env, p ItemsParams) (ItemsResult, error) {
	if err := needStore(env); err != nil {
		return ItemsResult{}, err
	}
	if p.Limit <= 0 {
		p.Limit = 100
	}
	p.Limit = min(p.Limit, 1000)
	where, args := itemsWhere(p)
	res := ItemsResult{Items: []ItemSummary{}}
	if err := env.Store.QueryRow(ctx, `SELECT COUNT(*) FROM items i LEFT JOIN licences l ON l.item_id = i.id `+where, args...).Scan(&res.Total); err != nil {
		return res, err
	}
	rows, err := queryRows(ctx, env.Store, `SELECT i.id, i.kind, i.title, i.subtitle, i.year, i.language,
		l.type AS licence, l.origin AS origin, l.share_allowed AS share FROM items i
		LEFT JOIN licences l ON l.item_id = i.id `+where+`
		ORDER BY i.title COLLATE NOCASE, i.id LIMIT ? OFFSET ?`, append(args, p.Limit, p.Offset)...)
	if err != nil {
		return res, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = str(r["id"])
	}
	authors, err := authorsOf(ctx, env.Store, ids)
	if err != nil {
		return res, err
	}
	status, err := newStatus(ctx, env.Store)
	if err != nil {
		return res, err
	}
	states, err := status.items(ctx, env.Store, ids)
	if err != nil {
		return res, err
	}
	rights, err := rightsOf(ctx, env.Store, ids)
	if err != nil {
		return res, err
	}
	for _, r := range rows {
		id := str(r["id"])
		it := ItemSummary{ID: id, Kind: str(r["kind"]), Title: str(r["title"]), Subtitle: str(r["subtitle"]),
			Year: int(num(r["year"])), Language: str(r["language"]), Authors: authors[id], Status: states[id],
			Licence: str(r["licence"]), LicenceOrigin: str(r["origin"])}
		if it.Authors == nil {
			it.Authors = []string{}
		}
		l := licence.Licence{Type: licence.Type(it.Licence), Share: num(r["share"]) == 1}
		if it.Licence == "" {
			l = suggest(rights[id])
			it.Suggested = &l
			it.Licence, it.LicenceOrigin = string(l.Type), l.Origin
		}
		it.LicenceName = l.Plain()
		res.Items = append(res.Items, it)
	}
	return res, nil
}

func itemsWhere(p ItemsParams) (string, []any) {
	conds := []string{"i.deleted_at IS NULL"}
	var args []any
	add := func(cond string, a ...any) {
		conds = append(conds, cond)
		args = append(args, a...)
	}
	if p.Kind != "" {
		add("i.kind = ?", p.Kind)
	}
	if p.FolderID != "" {
		add("EXISTS (SELECT 1 FROM files f WHERE f.item_id = i.id AND f.folder_id = ? AND f.deleted_at IS NULL)", p.FolderID)
	}
	if p.Author != "" {
		add(`EXISTS (SELECT 1 FROM item_people ip JOIN people pe ON pe.id = ip.person_id
			WHERE ip.item_id = i.id AND pe.name LIKE ? ESCAPE '\')`, likeArg(p.Author))
	}
	if p.Text != "" {
		add(`(i.title LIKE ? ESCAPE '\' OR i.subtitle LIKE ? ESCAPE '\')`, likeArg(p.Text), likeArg(p.Text))
	}
	switch {
	case p.Facet != "" && p.Value != "":
		add("EXISTS (SELECT 1 FROM item_facets x WHERE x.item_id = i.id AND x.facet = ? AND x.value = ?)", p.Facet, p.Value)
	case p.Facet != "":
		add("EXISTS (SELECT 1 FROM item_facets x WHERE x.item_id = i.id AND x.facet = ?)", p.Facet)
	case p.Value != "":
		add("EXISTS (SELECT 1 FROM item_facets x WHERE x.item_id = i.id AND x.value = ?)", p.Value)
	}
	if p.NeedsLicence {
		add("l.id IS NULL")
	}
	if p.AddedSince > 0 {
		add("i.created_at >= ?", p.AddedSince)
	}
	if p.LicenceType != "" {
		add("l.type = ?", p.LicenceType)
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

func authorsOf(ctx context.Context, s *store.Store, ids []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(ids) == 0 {
		return out, nil
	}
	marks, args := in(ids)
	rows, err := s.Query(ctx, `SELECT ip.item_id, pe.name FROM item_people ip JOIN people pe ON pe.id = ip.person_id
		WHERE ip.role = 'author' AND ip.item_id IN (`+marks+`) ORDER BY ip.item_id, ip.position`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = append(out[id], name)
	}
	return out, rows.Err()
}

// status knows which library folders are online, found by a quick check of each marker.
type status struct {
	paths map[string]string // folder ID -> current path, "" if offline
}

func newStatus(ctx context.Context, s *store.Store) (*status, error) {
	rows, err := queryFolders(ctx, s, "WHERE deleted_at IS NULL")
	if err != nil {
		return nil, err
	}
	l, err := newLocator(ctx, s, false)
	if err != nil {
		return nil, err
	}
	st := &status{paths: map[string]string{}}
	for _, f := range rows {
		st.paths[f.id] = l.find(f)
	}
	return st, nil
}

func (st *status) items(ctx context.Context, s *store.Store, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	marks, args := in(ids)
	rows, err := queryRows(ctx, s, `SELECT item_id, folder_id, deleted_at IS NULL AS live FROM files
		WHERE item_id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		id, live := str(r["item_id"]), num(r["live"]) == 1
		switch {
		case live && st.paths[str(r["folder_id"])] != "":
			out[id] = "online"
		case live && out[id] != "online":
			out[id] = "offline"
		case out[id] == "":
			out[id] = "missing"
		}
	}
	for _, id := range ids {
		if out[id] == "" {
			out[id] = "online" // not file-backed, e.g. a Bible module
		}
	}
	return out, nil
}

// Item details ---------------------------------------------------------------------------------

type File struct {
	ID       string `json:"id"`
	FolderID string `json:"folder_id"`
	RelPath  string `json:"rel_path" desc:"Path within the library folder, with / separators"`
	Format   string `json:"format"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	Online   bool   `json:"online"`
	Path     string `json:"path,omitempty" desc:"Full path, when the folder is online"`
}

type ItemPerson struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SortName string `json:"sort_name"`
	Role     string `json:"role"`
}

type Item struct {
	ID          string           `json:"id"`
	Kind        string           `json:"kind"`
	Title       string           `json:"title"`
	Subtitle    string           `json:"subtitle,omitempty"`
	Language    string           `json:"language,omitempty"`
	Year        int              `json:"year,omitempty"`
	Publisher   string           `json:"publisher,omitempty"`
	Series      string           `json:"series,omitempty"`
	SeriesNo    string           `json:"series_no,omitempty"`
	ISBN        string           `json:"isbn,omitempty"`
	Source      string           `json:"source"`
	SourceRef   string           `json:"source_ref,omitempty"`
	ModuleID    string           `json:"module_id,omitempty"`
	CreatedAt   int64            `json:"created_at"`
	UpdatedAt   int64            `json:"updated_at"`
	Status      string           `json:"status" enum:"online,offline,missing"`
	Files       []File           `json:"files"`
	People      []ItemPerson     `json:"people"`
	Facets      []Facet          `json:"facets"`
	Licence     *licence.Licence `json:"licence" desc:"The item's licence; its origin says whether it came from the book's own details, the default or the user"`
	LicenceName string           `json:"licence_name" desc:"The licence in plain words"`
	Suggested   *licence.Licence `json:"suggested_licence,omitempty" desc:"When no licence is recorded yet, the one the item will get"`
}

func getItem(ctx context.Context, env *command.Env, p IDParams) (Item, error) {
	if err := needStore(env); err != nil {
		return Item{}, err
	}
	return loadItem(ctx, env.Store, p.ID)
}

func loadItem(ctx context.Context, s *store.Store, id string) (Item, error) {
	rows, err := queryRows(ctx, s, `SELECT * FROM items WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return Item{}, err
	}
	if len(rows) == 0 {
		return Item{}, command.Errorf("not_found", "no item %s", id)
	}
	r := rows[0]
	it := Item{ID: id, Kind: str(r["kind"]), Title: str(r["title"]), Subtitle: str(r["subtitle"]),
		Language: str(r["language"]), Year: int(num(r["year"])), Publisher: str(r["publisher"]),
		Series: str(r["series"]), SeriesNo: str(r["series_no"]), ISBN: str(r["isbn"]), Source: str(r["source"]),
		SourceRef: str(r["source_ref"]), ModuleID: str(r["module_id"]), CreatedAt: num(r["created_at"]),
		UpdatedAt: num(r["updated_at"]), Files: []File{}, People: []ItemPerson{}, Facets: []Facet{}}
	st, err := newStatus(ctx, s)
	if err != nil {
		return it, err
	}
	states, err := st.items(ctx, s, []string{id})
	if err != nil {
		return it, err
	}
	it.Status = states[id]
	if err := it.loadFiles(ctx, s, st); err != nil {
		return it, err
	}
	if err := it.loadPeople(ctx, s); err != nil {
		return it, err
	}
	if err := it.loadFacets(ctx, s); err != nil {
		return it, err
	}
	return it, it.loadLicence(ctx, s)
}

func (it *Item) loadFiles(ctx context.Context, s *store.Store, st *status) error {
	rows, err := queryRows(ctx, s, `SELECT id, folder_id, rel_path, format, size_bytes, sha256 FROM files
		WHERE item_id = ? AND deleted_at IS NULL ORDER BY rel_path`, it.ID)
	for _, r := range rows {
		f := File{ID: str(r["id"]), FolderID: str(r["folder_id"]), RelPath: str(r["rel_path"]),
			Format: str(r["format"]), Size: num(r["size_bytes"]), SHA256: str(r["sha256"])}
		if dir := st.paths[f.FolderID]; dir != "" {
			f.Online, f.Path = true, filepath.Join(dir, filepath.FromSlash(f.RelPath))
		}
		it.Files = append(it.Files, f)
	}
	return err
}

func (it *Item) loadPeople(ctx context.Context, s *store.Store) error {
	rows, err := queryRows(ctx, s, `SELECT pe.id, pe.name, pe.sort_name, ip.role FROM item_people ip
		JOIN people pe ON pe.id = ip.person_id WHERE ip.item_id = ? ORDER BY ip.role, ip.position`, it.ID)
	for _, r := range rows {
		it.People = append(it.People, ItemPerson{ID: str(r["id"]), Name: str(r["name"]),
			SortName: str(r["sort_name"]), Role: str(r["role"])})
	}
	return err
}

func (it *Item) loadFacets(ctx context.Context, s *store.Store) error {
	rows, err := queryRows(ctx, s, `SELECT facet, value, origin FROM item_facets WHERE item_id = ?
		ORDER BY facet, value`, it.ID)
	for _, r := range rows {
		it.Facets = append(it.Facets, Facet{Facet: str(r["facet"]), Value: str(r["value"]), Origin: str(r["origin"])})
	}
	return err
}

func (it *Item) loadLicence(ctx context.Context, s *store.Store) error {
	rows, err := queryRows(ctx, s, `SELECT * FROM licences WHERE item_id = ?`, it.ID)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		l := licenceFromRow(rows[0])
		it.Licence, it.LicenceName = &l, l.Plain()
		return nil
	}
	rights := ""
	for _, f := range it.Facets {
		if f.Facet == "rights" {
			rights = f.Value
		}
	}
	sug := suggest(rights)
	it.Suggested, it.Licence, it.LicenceName = &sug, &sug, sug.Plain()
	return nil
}

// queryRows runs a query and returns every row as column values.
func queryRows(ctx context.Context, s *store.Store, q string, args ...any) ([]store.Row, error) {
	rows, err := s.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []store.Row
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := store.Row{}
		for i, c := range cols {
			r[c] = vals[i]
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
