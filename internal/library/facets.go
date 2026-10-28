package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path"
	"sort"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Types and traditions are item facets named "type" and "tradition", one value each. A value set
// automatically has origin metadata (or bisac) and carries its reason; a value the user chose has
// origin user, always wins and is never changed by a scan or by facet.suggest.

const (
	facetType      = "type"
	facetTradition = "tradition"
	traditionsKey  = "facets.traditions"
)

func isAuto(origin string) bool { return origin != "user" }

// current is a facet value an item has now.
type current struct{ value, origin, reason string }

// currentFacets returns, for each item, its rows of one facet.
func currentFacets(ctx context.Context, s *store.Store, facet string, ids []string) (map[string][]current, error) {
	out := map[string][]current{}
	if len(ids) == 0 {
		return out, nil
	}
	marks, args := in(ids)
	rows, err := queryRows(ctx, s, `SELECT item_id, value, origin, reason FROM item_facets WHERE facet = ? AND item_id IN (`+marks+`)`,
		append([]any{facet}, args...)...)
	for _, r := range rows {
		id := str(r["item_id"])
		out[id] = append(out[id], current{str(r["value"]), str(r["origin"]), str(r["reason"])})
	}
	return out, err
}

func userChose(cur []current) bool {
	for _, c := range cur {
		if c.origin == "user" {
			return true
		}
	}
	return false
}

// putFacet makes value the item's only value of a facet. Automatic values go in with a reason and
// confidence; the user's have none. It reports whether anything changed.
func putFacet(tx *store.Tx, itemID, facet string, cur []current, value, origin, reason string, conf float64) (bool, error) {
	changed := false
	have := false
	for _, c := range cur {
		if c.value != value {
			if err := tx.Delete("item_facets", store.Row{"item_id": itemID, "facet": facet, "value": c.value}); err != nil {
				return changed, err
			}
			changed = true
		} else {
			have = true
		}
	}
	row := store.Row{"origin": origin, "reason": nullable(reason), "confidence": nil}
	if origin != "user" {
		row["confidence"] = conf
	}
	key := store.Row{"item_id": itemID, "facet": facet, "value": value}
	if have {
		return changed, tx.Update("item_facets", key, row)
	}
	for k, v := range key {
		row[k] = v
	}
	return true, tx.Create("item_facets", row)
}

func dropAuto(tx *store.Tx, itemID, facet string, cur []current) (bool, error) {
	changed := false
	for _, c := range cur {
		if isAuto(c.origin) {
			if err := tx.Delete("item_facets", store.Row{"item_id": itemID, "facet": facet, "value": c.value}); err != nil {
				return changed, err
			}
			changed = true
		}
	}
	return changed, nil
}

// What the classifier knows about items -------------------------------------------------------------

type itemInfo struct {
	ClassifyInput
	id, publisher string
	authors       []string
	folderLabels  []string
	paths         []string // folder paths of the item's files, lowercase
}

func loadInfo(ctx context.Context, s *store.Store, ids []string) (map[string]*itemInfo, error) {
	out := map[string]*itemInfo{}
	if len(ids) == 0 {
		return out, nil
	}
	marks, args := in(ids)
	rows, err := queryRows(ctx, s, `SELECT id, kind, title, subtitle, series, publisher FROM items WHERE id IN (`+marks+`)`, args...)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		id := str(r["id"])
		out[id] = &itemInfo{id: id, publisher: str(r["publisher"]), ClassifyInput: ClassifyInput{Kind: str(r["kind"]),
			Title: str(r["title"]), Subtitle: str(r["subtitle"]), Series: str(r["series"])}}
	}
	if rows, err = queryRows(ctx, s, `SELECT item_id, value FROM item_facets WHERE facet = 'subject' AND item_id IN (`+marks+`)`, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if it := out[str(r["item_id"])]; it != nil {
			it.Subjects = append(it.Subjects, str(r["value"]))
		}
	}
	if rows, err = queryRows(ctx, s, `SELECT f.item_id, f.rel_path, lf.label FROM files f
		JOIN library_folders lf ON lf.id = f.folder_id WHERE f.deleted_at IS NULL AND f.item_id IN (`+marks+`)`, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if it := out[str(r["item_id"])]; it != nil {
			dir := path.Dir(str(r["rel_path"]))
			if dir != "." {
				it.Folders = append(it.Folders, strings.Split(dir, "/")...)
				it.paths = append(it.paths, strings.ToLower(dir))
			}
			it.folderLabels = append(it.folderLabels, strings.ToLower(str(r["label"])))
		}
	}
	authors, err := authorsOf(ctx, s, ids)
	for id, a := range authors {
		if it := out[id]; it != nil {
			it.authors = a
		}
	}
	return out, err
}

// Tradition rules ----------------------------------------------------------------------------------

type Rule struct {
	ID        string `json:"id"`
	Facet     string `json:"facet" enum:"tradition"`
	Field     string `json:"field" enum:"author,publisher,series,folder" desc:"What the rule looks at"`
	Text      string `json:"text" desc:"Words to look for, ignoring capitals"`
	Value     string `json:"value" desc:"The tradition books that match belong to"`
	CreatedAt int64  `json:"created_at"`
}

// matches reports whether the rule applies to the item.
func (r Rule) matches(it *itemInfo) bool {
	t := strings.ToLower(r.Text)
	has := func(vals ...string) bool {
		for _, v := range vals {
			if strings.Contains(strings.ToLower(v), t) {
				return true
			}
		}
		return false
	}
	switch r.Field {
	case "author":
		return has(it.authors...)
	case "publisher":
		return has(it.publisher)
	case "series":
		return has(it.Series)
	case "folder":
		return has(it.paths...) || has(it.folderLabels...)
	}
	return false
}

func (r Rule) reason() string {
	if r.Field == "folder" {
		return "it is in a folder matching your rule \"" + r.Text + "\""
	}
	return "its " + r.Field + " matches your rule \"" + r.Text + "\""
}

func loadRules(ctx context.Context, s *store.Store) ([]Rule, error) {
	rows, err := queryRows(ctx, s, `SELECT id, facet, match_field, match_text, value, created_at FROM facet_rules ORDER BY created_at, id`)
	rules := []Rule{}
	for _, r := range rows {
		rules = append(rules, Rule{ID: str(r["id"]), Facet: str(r["facet"]), Field: str(r["match_field"]),
			Text: str(r["match_text"]), Value: str(r["value"]), CreatedAt: num(r["created_at"])})
	}
	return rules, err
}

// Applying ---------------------------------------------------------------------------------------

// ApplyResult counts what an automatic pass did.
type ApplyResult struct {
	Typed      int       `json:"typed" desc:"Books given a type"`
	Traditions int       `json:"traditions" desc:"Books given a tradition by a rule"`
	Unsure     int       `json:"unsure" desc:"Books nothing pointed to a type for; they are left without one"`
	KeptYours  int       `json:"kept_yours" desc:"Books left alone because you chose for them"`
	Examples   []Applied `json:"examples" desc:"Some of the types set, with the reason"`
}

// Applied is one automatic choice, with its reason.
type Applied struct {
	ItemID     string  `json:"item_id"`
	Title      string  `json:"title"`
	Facet      string  `json:"facet"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence,omitempty"`
	Reason     string  `json:"reason"`
}

const maxExamples = 20

// autoFacets sets types and traditions for the items automatically. Values the user chose are never
// touched. When refresh is false an item that already has an automatic value keeps it.
func autoFacets(ctx context.Context, s *store.Store, cmd string, ids []string, refresh, dry bool) (ApplyResult, error) {
	res := ApplyResult{Examples: []Applied{}}
	rules, err := loadRules(ctx, s)
	if err != nil {
		return res, err
	}
	for _, batch := range chunks(ids, batchSize) {
		if err := autoBatch(ctx, s, cmd, batch, rules, refresh, dry, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

func autoBatch(ctx context.Context, s *store.Store, cmd string, ids []string, rules []Rule, refresh, dry bool, res *ApplyResult) error {
	info, err := loadInfo(ctx, s, ids)
	if err != nil {
		return err
	}
	types, err := currentFacets(ctx, s, facetType, ids)
	if err != nil {
		return err
	}
	trads, err := currentFacets(ctx, s, facetTradition, ids)
	if err != nil {
		return err
	}
	type change struct {
		id, facet string
		cur       []current
		drop      bool
		sug       Suggestion
	}
	var changes []change
	for _, id := range ids {
		it := info[id]
		if it == nil {
			continue
		}
		if userChose(types[id]) || userChose(trads[id]) {
			res.KeptYours++
		}
		if !userChose(types[id]) && (refresh || len(types[id]) == 0) {
			if sug, ok := ClassifyType(it.ClassifyInput); ok {
				changes = append(changes, change{id: id, facet: facetType, cur: types[id], sug: sug})
				res.Typed++
				if len(res.Examples) < maxExamples {
					res.Examples = append(res.Examples, Applied{id, it.Title, facetType, sug.Value, sug.Confidence, sug.Reason})
				}
			} else {
				res.Unsure++
			}
		}
		if !userChose(trads[id]) {
			if rule, ok := firstRule(rules, it); ok {
				sug := Suggestion{rule.Value, 1, rule.reason(), "metadata"}
				if len(trads[id]) == 0 || refresh || trads[id][0].value != rule.Value {
					changes = append(changes, change{id: id, facet: facetTradition, cur: trads[id], sug: sug})
					res.Traditions++
				}
			} else if refresh && len(trads[id]) > 0 {
				changes = append(changes, change{id: id, facet: facetTradition, cur: trads[id], drop: true})
			}
		}
	}
	if dry || len(changes) == 0 {
		return nil
	}
	return s.Write(ctx, cmd, func(tx *store.Tx) error {
		for _, c := range changes {
			var err error
			if c.drop {
				_, err = dropAuto(tx, c.id, c.facet, c.cur)
			} else {
				_, err = putFacet(tx, c.id, c.facet, c.cur, c.sug.Value, c.sug.Origin, c.sug.Reason, c.sug.Confidence)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func firstRule(rules []Rule, it *itemInfo) (Rule, bool) {
	for _, r := range rules {
		if r.matches(it) {
			return r, true
		}
	}
	return Rule{}, false
}

// autoNew is called after a scan with the items it added.
func autoNew(ctx context.Context, s *store.Store, cmd string, ids []string) (ApplyResult, error) {
	return autoFacets(ctx, s, cmd, ids, false, false)
}

// Vocabulary ---------------------------------------------------------------------------------------

type VocabularyResult struct {
	Types      []string `json:"types" desc:"The types a book can have, in plain words"`
	Traditions []string `json:"traditions" desc:"The traditions on your list"`
}

func traditions(ctx context.Context, s *store.Store) ([]string, error) {
	var raw string
	err := s.QueryRow(ctx, `SELECT value_json FROM settings WHERE key = ?`, traditionsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return append([]string{}, DefaultTraditions...), nil
	} else if err != nil {
		return nil, err
	}
	var list []string
	if json.Unmarshal([]byte(raw), &list) != nil {
		return append([]string{}, DefaultTraditions...), nil
	}
	return list, nil
}

func saveTraditions(tx *store.Tx, list []string) error {
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i]) < strings.ToLower(list[j]) })
	return putSetting(tx, traditionsKey, list)
}

func addTradition(list []string, name string) ([]string, bool) {
	for _, t := range list {
		if strings.EqualFold(t, name) {
			return list, false
		}
	}
	return append(list, name), true
}

func vocabulary(ctx context.Context, env *command.Env, _ struct{}) (VocabularyResult, error) {
	res := VocabularyResult{Types: BookTypes, Traditions: DefaultTraditions}
	if env.Store != nil {
		var err error
		if res.Traditions, err = traditions(ctx, env.Store); err != nil {
			return res, err
		}
	}
	return res, nil
}

type TraditionsParams struct {
	Action string `json:"action,omitempty" enum:"list,add,remove" desc:"list (the default), add a tradition to the list, or remove one from it"`
	Name   string `json:"name,omitempty" desc:"The tradition, for add and remove"`
}

type TraditionsResult struct {
	Traditions []string `json:"traditions"`
}

func editTraditions(ctx context.Context, env *command.Env, p TraditionsParams) (TraditionsResult, error) {
	if err := needStore(env); err != nil {
		return TraditionsResult{}, err
	}
	list, err := traditions(ctx, env.Store)
	if err != nil {
		return TraditionsResult{}, err
	}
	name := strings.TrimSpace(p.Name)
	switch p.Action {
	case "", "list":
		return TraditionsResult{list}, nil
	case "add", "remove":
		if name == "" {
			return TraditionsResult{list}, command.Errorf("bad_params", "give the name of the tradition")
		}
	default:
		return TraditionsResult{list}, command.Errorf("bad_params", "action must be list, add or remove")
	}
	if p.Action == "add" {
		list, _ = addTradition(list, name)
	} else {
		var kept []string
		for _, t := range list {
			if !strings.EqualFold(t, name) {
				kept = append(kept, t)
			}
		}
		if len(kept) == len(list) {
			return TraditionsResult{list}, command.Errorf("not_found", "%q is not on your list of traditions", name)
		}
		list = kept
	}
	err = env.Store.Write(ctx, "facet.traditions", func(tx *store.Tx) error { return saveTraditions(tx, list) })
	return TraditionsResult{list}, err
}

// facet.suggest ------------------------------------------------------------------------------------

type SuggestParams struct {
	ItemIDs  []string `json:"item_ids,omitempty" desc:"Books to look at; omit for every book (or every book in folder_id)"`
	FolderID string   `json:"folder_id,omitempty" desc:"Only books with a file in this library folder"`
	Redo     bool     `json:"redo,omitempty" desc:"Also redo types and traditions that were set automatically before; ones you chose are never changed"`
	Preview  bool     `json:"preview,omitempty" desc:"Show what would be set without setting it"`
}

func suggestFacets(ctx context.Context, env *command.Env, p SuggestParams) (ApplyResult, error) {
	if err := needStore(env); err != nil {
		return ApplyResult{}, err
	}
	ids := p.ItemIDs
	if len(ids) == 0 {
		q, args := `SELECT i.id FROM items i WHERE i.deleted_at IS NULL`, []any{}
		if p.FolderID != "" {
			q += ` AND EXISTS (SELECT 1 FROM files f WHERE f.item_id = i.id AND f.folder_id = ? AND f.deleted_at IS NULL)`
			args = append(args, p.FolderID)
		}
		var err error
		if ids, err = queryStrings(ctx, env.Store, q+` ORDER BY i.id`, args...); err != nil {
			return ApplyResult{}, err
		}
	}
	return autoFacets(ctx, env.Store, "facet.suggest", ids, p.Redo, p.Preview)
}

// facet.set ----------------------------------------------------------------------------------------

type SetParams struct {
	ItemIDs   []string `json:"item_ids" required:"true" desc:"The books to change"`
	Type      *string  `json:"type,omitempty" desc:"The type to give them; an empty text goes back to automatic. Leave out to keep the type as it is"`
	Tradition *string  `json:"tradition,omitempty" desc:"The tradition to give them (added to your list if new); an empty text goes back to automatic"`
}

type SetResult struct {
	Updated int `json:"updated" desc:"Books changed"`
}

func setFacets(ctx context.Context, env *command.Env, p SetParams) (SetResult, error) {
	if err := needStore(env); err != nil {
		return SetResult{}, err
	}
	if p.Type == nil && p.Tradition == nil {
		return SetResult{}, command.Errorf("bad_params", "give a type, a tradition or both")
	}
	choice := map[string]*string{facetType: p.Type, facetTradition: p.Tradition}
	if p.Type != nil && *p.Type != "" && !contains(BookTypes, strings.TrimSpace(*p.Type)) {
		return SetResult{}, command.Errorf("bad_params", "%q is not a type; the types are %s", *p.Type, strings.Join(BookTypes, ", "))
	}
	ids, err := licenceTargets(ctx, env.Store, SetLicenceParams{ItemIDs: p.ItemIDs})
	if err != nil {
		return SetResult{}, err
	}
	var res SetResult
	for _, batch := range chunks(ids, batchSize) {
		if err := setBatch(ctx, env.Store, batch, choice); err != nil {
			return res, err
		}
		res.Updated += len(batch)
	}
	return res, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func setBatch(ctx context.Context, s *store.Store, ids []string, choice map[string]*string) error {
	curs := map[string]map[string][]current{}
	for facet, v := range choice {
		if v == nil {
			continue
		}
		c, err := currentFacets(ctx, s, facet, ids)
		if err != nil {
			return err
		}
		curs[facet] = c
	}
	var back []string // items sent back to automatic
	list, err := traditions(ctx, s)
	if err != nil {
		return err
	}
	grew := false
	if v := choice[facetTradition]; v != nil && strings.TrimSpace(*v) != "" {
		list, grew = addTradition(list, strings.TrimSpace(*v))
	}
	err = s.Write(ctx, "facet.set", func(tx *store.Tx) error {
		for facet, v := range choice {
			if v == nil {
				continue
			}
			value := strings.TrimSpace(*v)
			for _, id := range ids {
				var err error
				if value == "" {
					_, err = dropAll(tx, id, facet, curs[facet][id])
					back = append(back, id)
				} else {
					_, err = putFacet(tx, id, facet, curs[facet][id], value, "user", "", 0)
				}
				if err != nil {
					return err
				}
			}
		}
		if grew {
			return saveTraditions(tx, list)
		}
		return nil
	})
	if err != nil || len(back) == 0 {
		return err
	}
	_, err = autoFacets(ctx, s, "facet.set", dedupe(back), true, false)
	return err
}

func dropAll(tx *store.Tx, itemID, facet string, cur []current) (bool, error) {
	for _, c := range cur {
		if err := tx.Delete("item_facets", store.Row{"item_id": itemID, "facet": facet, "value": c.value}); err != nil {
			return false, err
		}
	}
	return len(cur) > 0, nil
}

func dedupe(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// facet.rules --------------------------------------------------------------------------------------

type RulesParams struct {
	Action string `json:"action,omitempty" enum:"list,add,remove,apply" desc:"list (the default); add a rule; remove one; or apply all rules again"`
	ID     string `json:"id,omitempty" desc:"The rule, for remove"`
	Field  string `json:"field,omitempty" enum:"author,publisher,series,folder" desc:"For add: what the rule looks at"`
	Text   string `json:"text,omitempty" desc:"For add: words to look for, ignoring capitals"`
	Value  string `json:"value,omitempty" desc:"For add: the tradition matching books belong to (added to your list if new)"`
}

type RulesResult struct {
	Rules   []Rule `json:"rules"`
	Applied int    `json:"applied" desc:"Books whose tradition was set by the rules just now"`
	Summary string `json:"summary" desc:"One line saying what happened"`
}

func editRules(ctx context.Context, env *command.Env, p RulesParams) (RulesResult, error) {
	if err := needStore(env); err != nil {
		return RulesResult{}, err
	}
	var res RulesResult
	var err error
	switch p.Action {
	case "", "list":
		res.Rules, err = loadRules(ctx, env.Store)
		res.Summary = "These are your rules."
		return res, err
	case "add":
		err = addRule(ctx, env.Store, p)
	case "remove":
		err = removeRule(ctx, env.Store, p.ID)
	case "apply":
	default:
		return res, command.Errorf("bad_params", "action must be list, add, remove or apply")
	}
	if err != nil {
		return res, err
	}
	ids, err := queryStrings(ctx, env.Store, `SELECT id FROM items WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return res, err
	}
	// Redoing is what lets a removed or edited rule take its tags back; your own choices stay.
	r, err := autoFacets(ctx, env.Store, "facet.rules", ids, true, false)
	if err != nil {
		return res, err
	}
	res.Applied = r.Traditions
	if res.Rules, err = loadRules(ctx, env.Store); err != nil {
		return res, err
	}
	res.Summary = "Rules applied: " + plural(res.Applied, "book", "books") + " now follow them."
	return res, nil
}

func addRule(ctx context.Context, s *store.Store, p RulesParams) error {
	text, value := strings.TrimSpace(p.Text), strings.TrimSpace(p.Value)
	if text == "" || value == "" || !contains([]string{"author", "publisher", "series", "folder"}, p.Field) {
		return command.Errorf("bad_params", "a rule needs a field (author, publisher, series or folder), words to look for and a tradition")
	}
	list, err := traditions(ctx, s)
	if err != nil {
		return err
	}
	list, grew := addTradition(list, value)
	return s.Write(ctx, "facet.rules", func(tx *store.Tx) error {
		if grew {
			if err := saveTraditions(tx, list); err != nil {
				return err
			}
		}
		return tx.Create("facet_rules", store.Row{"id": store.NewID(), "facet": facetTradition, "match_field": p.Field,
			"match_text": text, "value": value, "created_at": now()})
	})
}

func removeRule(ctx context.Context, s *store.Store, id string) error {
	if id == "" {
		return command.Errorf("bad_params", "give the id of the rule to remove")
	}
	return s.Write(ctx, "facet.rules", func(tx *store.Tx) error {
		err := tx.Delete("facet_rules", store.ByID(id))
		if errors.Is(err, store.ErrNotFound) {
			return command.Errorf("not_found", "no rule %s", id)
		}
		return err
	})
}

func flt(v any) float64 {
	f, _ := v.(float64)
	return f
}

// chosen picks the value to show for a facet: the user's if there is one, else the automatic one.
func chosen(cur []current) (value, by, reason string) {
	for _, c := range cur {
		if c.origin == "user" {
			return c.value, "user", ""
		}
	}
	if len(cur) > 0 {
		return cur[0].value, "auto", cur[0].reason
	}
	return "", "", ""
}

// facetFilter adds the type, tradition and set-by conditions of library.items.
func facetFilter(p ItemsParams, add func(string, ...any)) {
	for facet, v := range map[string]string{facetType: p.Type, facetTradition: p.Tradition} {
		switch v {
		case "":
		case "none":
			add("NOT EXISTS (SELECT 1 FROM item_facets x WHERE x.item_id = i.id AND x.facet = '" + facet + "')")
		default:
			add("EXISTS (SELECT 1 FROM item_facets x WHERE x.item_id = i.id AND x.facet = '"+facet+"' AND x.value = ?)", v)
		}
	}
	switch p.SetBy {
	case "auto":
		add("EXISTS (SELECT 1 FROM item_facets x WHERE x.item_id = i.id AND x.facet IN ('type', 'tradition') AND x.origin <> 'user')")
	case "user":
		add("EXISTS (SELECT 1 FROM item_facets x WHERE x.item_id = i.id AND x.facet IN ('type', 'tradition') AND x.origin = 'user')")
	}
}
