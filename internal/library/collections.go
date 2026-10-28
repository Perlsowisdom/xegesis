package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Collections group books. The built-in groupings (type, tradition, year, author) are computed from
// book details and have no rows; the user's own collections are rows in collections, either
// hand-picked (their books are in collection_items) or by rule (rule_json, see docs/collections.md).

const (
	ruleVersion = 1
	builtinAll  = "b:all"
)

// CollectionRule is the rule format, version 1. A book matches when it meets every condition given;
// a condition left out matches any book. At least one condition is needed.
type CollectionRule struct {
	Version   int    `json:"version,omitempty" desc:"The rule format; 1 (the default)"`
	Type      string `json:"type,omitempty" desc:"Book type, such as Commentary (New Testament), or none for books with no type yet"`
	Tradition string `json:"tradition,omitempty" desc:"Tradition, or none for books with no tradition yet"`
	Author    string `json:"author,omitempty" desc:"Text in a credited person's name"`
	Text      string `json:"text,omitempty" desc:"Text in the title or subtitle"`
	Language  string `json:"language,omitempty" desc:"Language code, such as en"`
	YearFrom  int    `json:"year_from,omitempty" desc:"Published in or after this year"`
	YearTo    int    `json:"year_to,omitempty" desc:"Published in or before this year"`
	FolderID  string `json:"folder_id,omitempty" desc:"Has a file in this library folder"`
	Licence   string `json:"licence,omitempty" desc:"Licence type, such as public_domain"`
}

func (r CollectionRule) empty() bool {
	return r.Type == "" && r.Tradition == "" && r.Author == "" && r.Text == "" && r.Language == "" &&
		r.YearFrom == 0 && r.YearTo == 0 && r.FolderID == "" && r.Licence == ""
}

func (r CollectionRule) params() ItemsParams {
	return ItemsParams{Type: r.Type, Tradition: r.Tradition, Author: r.Author, Text: r.Text, Language: r.Language,
		YearFrom: r.YearFrom, YearTo: r.YearTo, FolderID: r.FolderID, LicenceType: r.Licence}
}

func (r CollectionRule) check() error {
	if r.Version != 0 && r.Version != ruleVersion {
		return command.Errorf("bad_params", "rule version %d is not known; this app reads version %d", r.Version, ruleVersion)
	}
	switch {
	case r.empty():
		return command.Errorf("bad_params", "a rule needs at least one condition")
	case r.Type != "" && r.Type != "none" && !contains(BookTypes, r.Type):
		return command.Errorf("bad_params", "%q is not a type; the types are %s", r.Type, strings.Join(BookTypes, ", "))
	case r.Licence != "" && !contains(licenceTypeNames(), r.Licence):
		return command.Errorf("bad_params", "%q is not a licence type", r.Licence)
	case r.YearFrom < 0 || r.YearTo < 0 || (r.YearTo > 0 && r.YearFrom > r.YearTo):
		return command.Errorf("bad_params", "the first year must not be after the last")
	}
	return nil
}

func licenceTypeNames() []string {
	var out []string
	for _, t := range licence.Types {
		out = append(out, string(t))
	}
	return out
}

func parseRule(raw string) (CollectionRule, error) {
	var r CollectionRule
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return r, fmt.Errorf("collection rule: %w", err)
	}
	if r.Version > ruleVersion {
		return r, fmt.Errorf("collection rule version %d is newer than this app understands", r.Version)
	}
	return r, nil
}

// Node is one entry in the collections tree.
type Node struct {
	ID          string          `json:"id" desc:"A built-in grouping (b:...) or one of your collections"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind" enum:"builtin,manual,rule" desc:"builtin: computed from book details; manual: hand-picked; rule: books that match a rule"`
	Count       int             `json:"count" desc:"Books in it"`
	ParentID    string          `json:"parent_id,omitempty"`
	Position    int             `json:"position,omitempty"`
	Rule        *CollectionRule `json:"rule,omitempty"`
	Children    []*Node         `json:"children,omitempty"`
	HasChildren bool            `json:"has_children,omitempty" desc:"Children exist but were not listed (authors, unless asked for)"`
}

type ListParams struct {
	Authors bool `json:"authors,omitempty" desc:"Also list every author under Author (can be long)"`
}

type ListResult struct {
	Tree []*Node `json:"tree" desc:"All books, then Type, Tradition, Year and Author, then your collections, each with its book count"`
}

func collectionList(ctx context.Context, env *command.Env, p ListParams) (ListResult, error) {
	if err := needStore(env); err != nil {
		return ListResult{}, err
	}
	tree, err := builtinTree(ctx, env.Store, p.Authors)
	if err != nil {
		return ListResult{}, err
	}
	mine, err := userTree(ctx, env.Store)
	return ListResult{Tree: append(tree, mine...)}, err
}

// Built-in groupings -------------------------------------------------------------------------------

func builtinTree(ctx context.Context, s *store.Store, authors bool) ([]*Node, error) {
	var total int
	if err := s.QueryRow(ctx, `SELECT COUNT(*) FROM items WHERE deleted_at IS NULL`).Scan(&total); err != nil {
		return nil, err
	}
	out := []*Node{{ID: builtinAll, Name: "All books", Kind: "builtin", Count: total}}
	for _, f := range []struct{ facet, name string }{{facetType, "Type"}, {facetTradition, "Tradition"}} {
		n, err := facetGroup(ctx, s, f.facet, f.name)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	year, err := yearGroup(ctx, s)
	if err != nil {
		return nil, err
	}
	author, err := authorGroup(ctx, s, authors)
	if err != nil {
		return nil, err
	}
	return append(out, year, author), nil
}

// facetGroup lists the values of a facet with the number of books having each.
func facetGroup(ctx context.Context, s *store.Store, facet, name string) (*Node, error) {
	g := &Node{ID: "b:" + facet, Name: name, Kind: "builtin"}
	rows, err := s.Query(ctx, `SELECT x.value, COUNT(DISTINCT x.item_id) FROM item_facets x
		JOIN items i ON i.id = x.item_id AND i.deleted_at IS NULL WHERE x.facet = ? GROUP BY x.value`, facet)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for rows.Next() {
		var v string
		var n int
		if err := rows.Scan(&v, &n); err != nil {
			rows.Close()
			return nil, err
		}
		counts[v] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var order []string
	if facet == facetType {
		order = append(order, BookTypes...)
	}
	var extra []string
	for v := range counts {
		if !contains(order, v) {
			extra = append(extra, v)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return strings.ToLower(extra[i]) < strings.ToLower(extra[j]) })
	for _, v := range append(order, extra...) {
		if counts[v] > 0 {
			g.Children = append(g.Children, &Node{ID: "b:" + facet + ":" + v, Name: v, Kind: "builtin", Count: counts[v], ParentID: g.ID})
		}
	}
	err = s.QueryRow(ctx, `SELECT COUNT(DISTINCT x.item_id) FROM item_facets x
		JOIN items i ON i.id = x.item_id AND i.deleted_at IS NULL WHERE x.facet = ?`, facet).Scan(&g.Count)
	if err != nil {
		return nil, err
	}
	var total int
	if err := s.QueryRow(ctx, `SELECT COUNT(*) FROM items WHERE deleted_at IS NULL`).Scan(&total); err != nil {
		return nil, err
	}
	if none := total - g.Count; none > 0 {
		g.Children = append(g.Children, &Node{ID: "b:" + facet + ":none", Name: "Not set yet", Kind: "builtin", Count: none, ParentID: g.ID})
	}
	return g, nil
}

func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(n) + suffix
}

// yearGroup groups books by century, then by decade. Books with no year are left out.
func yearGroup(ctx context.Context, s *store.Store) (*Node, error) {
	g := &Node{ID: "b:year", Name: "Year", Kind: "builtin"}
	rows, err := s.Query(ctx, `SELECT year, COUNT(*) FROM items WHERE deleted_at IS NULL AND year > 0 GROUP BY year`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	centuries, decades := map[int]int{}, map[int]int{}
	for rows.Next() {
		var y, n int
		if err := rows.Scan(&y, &n); err != nil {
			return nil, err
		}
		g.Count += n
		centuries[y/100+1] += n
		decades[y/10*10] += n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var cs []int
	for c := range centuries {
		cs = append(cs, c)
	}
	sort.Ints(cs)
	at := map[int]*Node{}
	for _, c := range cs {
		cn := &Node{ID: "b:year:c" + strconv.Itoa(c), Name: ordinal(c) + " century", Kind: "builtin", Count: centuries[c], ParentID: g.ID}
		at[c] = cn
		g.Children = append(g.Children, cn)
	}
	for d, n := range decades {
		cn := at[d/100+1]
		cn.Children = append(cn.Children, &Node{ID: "b:year:d" + strconv.Itoa(d), Name: strconv.Itoa(d) + "s", Kind: "builtin", Count: n, ParentID: cn.ID})
	}
	for _, cn := range g.Children {
		sort.Slice(cn.Children, func(i, j int) bool { return cn.Children[i].ID < cn.Children[j].ID })
	}
	return g, nil
}

// authorGroup lists authors by name. Its children are only listed on request, as there can be
// thousands.
func authorGroup(ctx context.Context, s *store.Store, list bool) (*Node, error) {
	g := &Node{ID: "b:author", Name: "Author", Kind: "builtin"}
	err := s.QueryRow(ctx, `SELECT COUNT(DISTINCT ip.item_id) FROM item_people ip
		JOIN items i ON i.id = ip.item_id AND i.deleted_at IS NULL WHERE ip.role = 'author'`).Scan(&g.Count)
	if err != nil {
		return nil, err
	}
	g.HasChildren = g.Count > 0
	if !list || g.Count == 0 {
		return g, nil
	}
	rows, err := s.Query(ctx, `SELECT pe.name, COUNT(DISTINCT ip.item_id) FROM item_people ip
		JOIN people pe ON pe.id = ip.person_id JOIN items i ON i.id = ip.item_id AND i.deleted_at IS NULL
		WHERE ip.role = 'author' GROUP BY pe.name ORDER BY pe.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		g.Children = append(g.Children, &Node{ID: "b:author:" + name, Name: name, Kind: "builtin", Count: n, ParentID: g.ID})
	}
	g.HasChildren = false
	return g, rows.Err()
}

// builtinParams turns a built-in grouping's ID into the book filter it stands for.
func builtinParams(id string) (ItemsParams, bool) {
	parts := strings.SplitN(id, ":", 3)
	if parts[0] != "b" || len(parts) < 2 {
		return ItemsParams{}, false
	}
	value := ""
	if len(parts) == 3 {
		value = parts[2]
	}
	switch parts[1] {
	case "all":
		return ItemsParams{}, len(parts) == 2
	case facetType, facetTradition:
		p := ItemsParams{}
		switch {
		case len(parts) == 2:
			p.Facet = parts[1]
		case parts[1] == facetType:
			p.Type = value
		default:
			p.Tradition = value
		}
		return p, true
	case "author":
		if len(parts) == 2 {
			return ItemsParams{authorAny: true}, true
		}
		return ItemsParams{authorExact: value}, true
	case "year":
		if len(parts) == 2 {
			return ItemsParams{YearFrom: 1}, true
		}
		n, err := strconv.Atoi(value[min(1, len(value)):])
		if err != nil || n <= 0 || (value[0] != 'c' && value[0] != 'd') {
			return ItemsParams{}, false
		}
		if value[0] == 'c' {
			return ItemsParams{YearFrom: max((n-1)*100, 1), YearTo: n*100 - 1}, true
		}
		return ItemsParams{YearFrom: max(n, 1), YearTo: n + 9}, true
	}
	return ItemsParams{}, false
}

// User collections ---------------------------------------------------------------------------------

type collectionRow struct {
	id, parent, name, kind, rule string
	position                     int
}

func loadCollections(ctx context.Context, s *store.Store) ([]collectionRow, error) {
	rows, err := queryRows(ctx, s, `SELECT id, parent_id, name, kind, rule_json, position FROM collections
		WHERE deleted_at IS NULL ORDER BY position, name COLLATE NOCASE, id`)
	var out []collectionRow
	for _, r := range rows {
		out = append(out, collectionRow{str(r["id"]), str(r["parent_id"]), str(r["name"]), str(r["kind"]), str(r["rule_json"]), int(num(r["position"]))})
	}
	return out, err
}

func userTree(ctx context.Context, s *store.Store) ([]*Node, error) {
	rows, err := loadCollections(ctx, s)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	manual, err := manualCounts(ctx, s)
	if err != nil {
		return nil, err
	}
	byID := map[string]*Node{}
	for _, r := range rows {
		n := &Node{ID: r.id, Name: r.name, Kind: r.kind, ParentID: r.parent, Position: r.position}
		if r.kind == "manual" {
			n.Count = manual[r.id]
		} else {
			rule, err := parseRule(r.rule)
			if err != nil {
				return nil, err
			}
			n.Rule = &rule
			if n.Count, err = countMatching(ctx, s, rule.params()); err != nil {
				return nil, err
			}
		}
		byID[r.id] = n
	}
	var top []*Node
	for _, r := range rows { // rows are in order, so children stay in order
		n := byID[r.id]
		if p := byID[r.parent]; p != nil {
			p.Children = append(p.Children, n)
		} else {
			n.ParentID = ""
			top = append(top, n)
		}
	}
	return top, nil
}

func manualCounts(ctx context.Context, s *store.Store) (map[string]int, error) {
	rows, err := s.Query(ctx, `SELECT ci.collection_id, COUNT(*) FROM collection_items ci
		JOIN items i ON i.id = ci.item_id AND i.deleted_at IS NULL GROUP BY ci.collection_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// countMatching counts the books a filter matches.
func countMatching(ctx context.Context, s *store.Store, p ItemsParams) (int, error) {
	conds, args := itemsConds(p)
	var n int
	err := s.QueryRow(ctx, `SELECT COUNT(*) FROM items i LEFT JOIN licences l ON l.item_id = i.id WHERE `+
		strings.Join(conds, " AND "), args...).Scan(&n)
	return n, err
}

// collectionConds returns the conditions (on items i and licences l) that select a collection's
// books: a built-in grouping, a hand-picked collection or a rule.
func collectionConds(ctx context.Context, s *store.Store, id string) ([]string, []any, error) {
	if strings.HasPrefix(id, "b:") {
		p, ok := builtinParams(id)
		if !ok {
			return nil, nil, command.Errorf("not_found", "no grouping %s", id)
		}
		conds, args := itemsConds(p)
		return conds, args, nil
	}
	c, err := getCollection(ctx, s, id)
	if err != nil {
		return nil, nil, err
	}
	if c.kind == "manual" {
		return []string{"EXISTS (SELECT 1 FROM collection_items ci WHERE ci.collection_id = ? AND ci.item_id = i.id)"}, []any{id}, nil
	}
	rule, err := parseRule(c.rule)
	if err != nil {
		return nil, nil, err
	}
	conds, args := itemsConds(rule.params())
	return conds, args, nil
}

// CollectionItemIDs lists the IDs of the books in a collection or built-in grouping, for the search
// filter.
func CollectionItemIDs(ctx context.Context, s *store.Store, id string) ([]string, error) {
	conds, args, err := collectionConds(ctx, s, id)
	if err != nil {
		return nil, err
	}
	return queryStrings(ctx, s, `SELECT i.id FROM items i LEFT JOIN licences l ON l.item_id = i.id WHERE `+
		strings.Join(conds, " AND "), args...)
}

func getCollection(ctx context.Context, s *store.Store, id string) (collectionRow, error) {
	rows, err := queryRows(ctx, s, `SELECT id, parent_id, name, kind, rule_json, position FROM collections
		WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return collectionRow{}, err
	}
	if len(rows) == 0 {
		return collectionRow{}, command.Errorf("not_found", "no collection %s", id)
	}
	r := rows[0]
	return collectionRow{str(r["id"]), str(r["parent_id"]), str(r["name"]), str(r["kind"]), str(r["rule_json"]), int(num(r["position"]))}, nil
}

// collection.items --------------------------------------------------------------------------------

type CollectionItemsParams struct {
	CollectionID string `json:"collection_id" required:"true" desc:"One of your collections, or a built-in grouping from collection.list"`
	Limit        int    `json:"limit,omitempty" desc:"Most items to return (default 100, at most 1000)"`
	Offset       int    `json:"offset,omitempty"`
}

func collectionItems(ctx context.Context, env *command.Env, p CollectionItemsParams) (ItemsResult, error) {
	return listItems(ctx, env, ItemsParams{CollectionID: p.CollectionID, Limit: p.Limit, Offset: p.Offset})
}

// collection.create -------------------------------------------------------------------------------

type CreateParams struct {
	Name     string          `json:"name" required:"true"`
	ParentID string          `json:"parent_id,omitempty" desc:"Put it inside this collection of yours; omit for the top level"`
	Rule     *CollectionRule `json:"rule,omitempty" desc:"Make a collection of the books matching this rule; omit for a hand-picked one"`
	ItemIDs  []string        `json:"item_ids,omitempty" desc:"For a hand-picked collection: books to start with"`
}

type CollectionResult struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Kind     string          `json:"kind" enum:"manual,rule"`
	ParentID string          `json:"parent_id,omitempty"`
	Rule     *CollectionRule `json:"rule,omitempty"`
	Count    int             `json:"count" desc:"Books in it"`
}

func collectionCreate(ctx context.Context, env *command.Env, p CreateParams) (CollectionResult, error) {
	if err := needStore(env); err != nil {
		return CollectionResult{}, err
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return CollectionResult{}, command.Errorf("bad_params", "give the collection a name")
	}
	if p.Rule != nil && len(p.ItemIDs) > 0 {
		return CollectionResult{}, command.Errorf("bad_params", "a collection is either hand-picked or by rule, not both")
	}
	if p.ParentID != "" {
		if _, err := getCollection(ctx, env.Store, p.ParentID); err != nil {
			return CollectionResult{}, err
		}
	}
	row := store.Row{"id": store.NewID(), "name": name, "kind": "manual", "parent_id": nullable(p.ParentID), "created_at": now(), "updated_at": now()}
	res := CollectionResult{ID: row["id"].(string), Name: name, Kind: "manual", ParentID: p.ParentID}
	if p.Rule != nil {
		rule := *p.Rule
		if err := rule.check(); err != nil {
			return res, err
		}
		rule.Version = ruleVersion
		raw, _ := json.Marshal(rule)
		row["kind"], row["rule_json"] = "rule", string(raw)
		res.Kind, res.Rule = "rule", &rule
	}
	ids, err := licenceTargets(ctx, env.Store, SetLicenceParams{ItemIDs: p.ItemIDs})
	if err != nil {
		return res, err
	}
	if row["position"], err = nextPosition(ctx, env.Store, p.ParentID); err != nil {
		return res, err
	}
	err = env.Store.Write(ctx, "collection.create", func(tx *store.Tx) error {
		if err := tx.Create("collections", row); err != nil {
			return err
		}
		return addToCollection(ctx, env.Store, tx, res.ID, ids)
	})
	if err != nil {
		return res, err
	}
	res.Count, err = countOf(ctx, env.Store, res.ID)
	return res, err
}

func nextPosition(ctx context.Context, s *store.Store, parent string) (int, error) {
	var n int
	err := s.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM collections
		WHERE deleted_at IS NULL AND COALESCE(parent_id, '') = ?`, parent).Scan(&n)
	return n, err
}

func countOf(ctx context.Context, s *store.Store, id string) (int, error) {
	conds, args, err := collectionConds(ctx, s, id)
	if err != nil {
		return 0, err
	}
	var n int
	err = s.QueryRow(ctx, `SELECT COUNT(*) FROM items i LEFT JOIN licences l ON l.item_id = i.id WHERE `+
		strings.Join(conds, " AND "), args...).Scan(&n)
	return n, err
}

// addToCollection puts books in a hand-picked collection, skipping ones already there.
func addToCollection(ctx context.Context, s *store.Store, tx *store.Tx, id string, items []string) error {
	if len(items) == 0 {
		return nil
	}
	have := map[string]bool{}
	var pos int
	for _, batch := range chunks(items, batchSize) {
		marks, args := in(batch)
		found, err := queryStrings(ctx, s, `SELECT item_id FROM collection_items WHERE collection_id = ? AND item_id IN (`+marks+`)`,
			append([]any{id}, args...)...)
		if err != nil {
			return err
		}
		for _, f := range found {
			have[f] = true
		}
	}
	if err := s.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0) FROM collection_items WHERE collection_id = ?`, id).Scan(&pos); err != nil {
		return err
	}
	for _, it := range items {
		if have[it] {
			continue
		}
		have[it] = true
		pos++
		if err := tx.Create("collection_items", store.Row{"collection_id": id, "item_id": it, "position": pos}); err != nil {
			return err
		}
	}
	return nil
}

// collection.update -------------------------------------------------------------------------------

type UpdateParams struct {
	ID       string          `json:"id" required:"true"`
	Name     *string         `json:"name,omitempty" desc:"A new name"`
	ParentID *string         `json:"parent_id,omitempty" desc:"Move it inside this collection; an empty text moves it to the top level"`
	Rule     *CollectionRule `json:"rule,omitempty" desc:"A new rule, for a collection made by rule"`
	Move     string          `json:"move,omitempty" enum:"up,down" desc:"Move it up or down among the collections beside it"`
}

func collectionUpdate(ctx context.Context, env *command.Env, p UpdateParams) (CollectionResult, error) {
	if err := needStore(env); err != nil {
		return CollectionResult{}, err
	}
	c, err := getCollection(ctx, env.Store, p.ID)
	if err != nil {
		return CollectionResult{}, err
	}
	fields := store.Row{}
	if p.Name != nil {
		if strings.TrimSpace(*p.Name) == "" {
			return CollectionResult{}, command.Errorf("bad_params", "give the collection a name")
		}
		fields["name"] = strings.TrimSpace(*p.Name)
	}
	if p.Rule != nil {
		if c.kind != "rule" {
			return CollectionResult{}, command.Errorf("bad_params", "only a collection made by rule has a rule")
		}
		rule := *p.Rule
		if err := rule.check(); err != nil {
			return CollectionResult{}, err
		}
		rule.Version = ruleVersion
		raw, _ := json.Marshal(rule)
		fields["rule_json"] = string(raw)
	}
	parent := c.parent
	if p.ParentID != nil {
		if err := checkMove(ctx, env.Store, c.id, *p.ParentID); err != nil {
			return CollectionResult{}, err
		}
		if *p.ParentID != c.parent {
			parent = *p.ParentID
			fields["parent_id"] = nullable(parent)
			if fields["position"], err = nextPosition(ctx, env.Store, parent); err != nil {
				return CollectionResult{}, err
			}
		}
	}
	err = env.Store.Write(ctx, "collection.update", func(tx *store.Tx) error {
		if len(fields) > 0 {
			fields["updated_at"] = now()
			if err := tx.Update("collections", store.ByID(c.id), fields); err != nil {
				return err
			}
		}
		if p.Move != "" {
			return moveCollection(ctx, env.Store, tx, c, p.Move)
		}
		return nil
	})
	if err != nil {
		return CollectionResult{}, err
	}
	return describe(ctx, env.Store, p.ID)
}

func describe(ctx context.Context, s *store.Store, id string) (CollectionResult, error) {
	c, err := getCollection(ctx, s, id)
	if err != nil {
		return CollectionResult{}, err
	}
	res := CollectionResult{ID: c.id, Name: c.name, Kind: c.kind, ParentID: c.parent}
	if c.kind == "rule" {
		r, err := parseRule(c.rule)
		if err != nil {
			return res, err
		}
		res.Rule = &r
	}
	res.Count, err = countOf(ctx, s, id)
	return res, err
}

// checkMove refuses a move that would put a collection inside itself or one of its own.
func checkMove(ctx context.Context, s *store.Store, id, parent string) error {
	if parent == "" {
		return nil
	}
	all, err := loadCollections(ctx, s)
	if err != nil {
		return err
	}
	up := map[string]string{}
	for _, c := range all {
		up[c.id] = c.parent
	}
	if _, ok := up[parent]; !ok {
		return command.Errorf("not_found", "no collection %s", parent)
	}
	for at, n := parent, 0; at != "" && n <= len(all); at, n = up[at], n+1 {
		if at == id {
			return command.Errorf("bad_params", "a collection cannot go inside itself")
		}
	}
	return nil
}

// moveCollection swaps the collection with its neighbour, renumbering its siblings 1, 2, 3...
func moveCollection(ctx context.Context, s *store.Store, tx *store.Tx, c collectionRow, dir string) error {
	all, err := loadCollections(ctx, s)
	if err != nil {
		return err
	}
	var sib []collectionRow
	for _, o := range all {
		if o.parent == c.parent {
			sib = append(sib, o)
		}
	}
	at := -1
	for i, o := range sib {
		if o.id == c.id {
			at = i
		}
	}
	to := at - 1
	if dir == "down" {
		to = at + 1
	}
	if at < 0 || to < 0 || to >= len(sib) {
		return nil
	}
	sib[at], sib[to] = sib[to], sib[at]
	for i, o := range sib {
		if o.position != i+1 {
			if err := tx.Update("collections", store.ByID(o.id), store.Row{"position": i + 1}); err != nil {
				return err
			}
		}
	}
	return nil
}

// collection.delete -------------------------------------------------------------------------------

type DeleteParams struct {
	ID string `json:"id" required:"true"`
}

type DeleteResult struct {
	Deleted int `json:"deleted" desc:"Collections deleted: this one and any inside it"`
}

func collectionDelete(ctx context.Context, env *command.Env, p DeleteParams) (DeleteResult, error) {
	if err := needStore(env); err != nil {
		return DeleteResult{}, err
	}
	if _, err := getCollection(ctx, env.Store, p.ID); err != nil {
		return DeleteResult{}, err
	}
	all, err := loadCollections(ctx, env.Store)
	if err != nil {
		return DeleteResult{}, err
	}
	doomed := []string{p.ID}
	for i := 0; i < len(doomed); i++ {
		for _, c := range all {
			if c.parent == doomed[i] {
				doomed = append(doomed, c.id)
			}
		}
	}
	err = env.Store.Write(ctx, "collection.delete", func(tx *store.Tx) error {
		for _, id := range doomed {
			if err := tx.Delete("collections", store.ByID(id)); err != nil {
				return err
			}
		}
		return nil
	})
	return DeleteResult{len(doomed)}, err
}

// collection.add and collection.remove ------------------------------------------------------------

type MembersParams struct {
	CollectionID string   `json:"collection_id" required:"true" desc:"A hand-picked collection of yours"`
	ItemIDs      []string `json:"item_ids" required:"true"`
}

type MembersResult struct {
	Changed int `json:"changed" desc:"Books added or removed"`
	Count   int `json:"count" desc:"Books now in the collection"`
}

func handPicked(ctx context.Context, env *command.Env, id string) error {
	if err := needStore(env); err != nil {
		return err
	}
	c, err := getCollection(ctx, env.Store, id)
	if err != nil {
		return err
	}
	if c.kind != "manual" {
		return command.Errorf("bad_params", "%q is made by a rule, so its books cannot be added or removed; change its rule instead", c.name)
	}
	return nil
}

func collectionAdd(ctx context.Context, env *command.Env, p MembersParams) (MembersResult, error) {
	if err := handPicked(ctx, env, p.CollectionID); err != nil {
		return MembersResult{}, err
	}
	ids, err := licenceTargets(ctx, env.Store, SetLicenceParams{ItemIDs: p.ItemIDs})
	if err != nil {
		return MembersResult{}, err
	}
	before, err := countOf(ctx, env.Store, p.CollectionID)
	if err != nil {
		return MembersResult{}, err
	}
	err = env.Store.Write(ctx, "collection.add", func(tx *store.Tx) error {
		return addToCollection(ctx, env.Store, tx, p.CollectionID, ids)
	})
	if err != nil {
		return MembersResult{}, err
	}
	n, err := countOf(ctx, env.Store, p.CollectionID)
	return MembersResult{Changed: n - before, Count: n}, err
}

func collectionRemove(ctx context.Context, env *command.Env, p MembersParams) (MembersResult, error) {
	if err := handPicked(ctx, env, p.CollectionID); err != nil {
		return MembersResult{}, err
	}
	var res MembersResult
	err := env.Store.Write(ctx, "collection.remove", func(tx *store.Tx) error {
		for _, it := range dedupe(p.ItemIDs) {
			key := store.Row{"collection_id": p.CollectionID, "item_id": it}
			if err := tx.Delete("collection_items", key); errors.Is(err, store.ErrNotFound) {
				continue
			} else if err != nil {
				return err
			}
			res.Changed++
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	res.Count, err = countOf(ctx, env.Store, p.CollectionID)
	return res, err
}

// collection.preview ------------------------------------------------------------------------------

type PreviewParams struct {
	Rule  CollectionRule `json:"rule" required:"true"`
	Limit int            `json:"limit,omitempty" desc:"Most examples to return (default 5, at most 50)"`
}

type Example struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Authors []string `json:"authors"`
	Year    int      `json:"year,omitempty"`
}

type PreviewResult struct {
	Count    int       `json:"count" desc:"Books the rule matches"`
	Examples []Example `json:"examples" desc:"Some of them"`
}

func collectionPreview(ctx context.Context, env *command.Env, p PreviewParams) (PreviewResult, error) {
	res := PreviewResult{Examples: []Example{}}
	if err := needStore(env); err != nil {
		return res, err
	}
	if err := p.Rule.check(); err != nil {
		return res, err
	}
	conds, args := itemsConds(p.Rule.params())
	where := ` FROM items i LEFT JOIN licences l ON l.item_id = i.id WHERE ` + strings.Join(conds, " AND ")
	if err := env.Store.QueryRow(ctx, `SELECT COUNT(*)`+where, args...).Scan(&res.Count); err != nil {
		return res, err
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 5
	}
	rows, err := queryRows(ctx, env.Store, `SELECT i.id, i.title, i.year`+where+` ORDER BY i.title COLLATE NOCASE, i.id LIMIT ?`,
		append(args, min(limit, 50))...)
	if err != nil {
		return res, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = str(r["id"])
	}
	authors, err := authorsOf(ctx, env.Store, ids)
	for _, r := range rows {
		id := str(r["id"])
		a := authors[id]
		if a == nil {
			a = []string{}
		}
		res.Examples = append(res.Examples, Example{ID: id, Title: str(r["title"]), Authors: a, Year: int(num(r["year"]))})
	}
	return res, err
}
