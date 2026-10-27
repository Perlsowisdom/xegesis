package library

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

var (
	ccURLRe  = regexp.MustCompile(`(?i)creativecommons\.org/(?:licenses|publicdomain)/([a-z-]+)/(\d\.\d)`)
	ccNameRe = regexp.MustCompile(`(?i)\bCC[- ]?(0|BY(?:[- ](?:SA|NC|ND))*)(?:[- ](\d\.\d))?\b`)
)

// suggest proposes a licence from a file's rights statement. It is only a starting answer: the
// user confirms or changes it with library.set_licence.
func suggest(rights string) licence.Licence {
	r := strings.ToLower(rights)
	t, spdx := licence.Unknown, ""
	switch {
	case strings.Contains(r, "public domain") && !strings.Contains(r, "creativecommons"):
		t = licence.PublicDomain
	case ccURLRe.MatchString(rights):
		m := ccURLRe.FindStringSubmatch(rights)
		t, spdx = licence.CreativeCommons, ccID(m[1], m[2])
	case ccNameRe.MatchString(rights):
		m := ccNameRe.FindStringSubmatch(rights)
		t, spdx = licence.CreativeCommons, ccID(m[1], m[2])
	case strings.Contains(r, "creative commons"):
		t = licence.CreativeCommons
	}
	l, err := licence.Defaults(t, spdx)
	if err != nil {
		l, _ = licence.Defaults(licence.Unknown, "")
	}
	return l
}

// ccID builds an SPDX ID such as CC-BY-SA-4.0 or CC0-1.0.
func ccID(kind, version string) string {
	kind = strings.ToUpper(strings.ReplaceAll(kind, " ", "-"))
	if kind == "0" || kind == "ZERO" {
		return "CC0-1.0"
	}
	id := "CC-" + kind
	if version != "" {
		id += "-" + version
	}
	return id
}

type SetLicenceParams struct {
	ItemIDs      []string       `json:"item_ids,omitempty" desc:"Items to set; or use pending"`
	Pending      bool           `json:"pending,omitempty" desc:"Apply to every item with no licence decided yet"`
	FolderID     string         `json:"folder_id,omitempty" desc:"With pending, only items in this library folder"`
	UseSuggested bool           `json:"use_suggested,omitempty" desc:"Give each item the licence suggested by its own metadata instead of type"`
	Type         string         `json:"type,omitempty" enum:"public_domain,creative_commons,open_other,personal_purchase,own_work,unknown"`
	SPDX         string         `json:"spdx,omitempty" desc:"SPDX or Creative Commons ID, e.g. CC-BY-SA-4.0"`
	RightsHolder string         `json:"rights_holder,omitempty"`
	URL          string         `json:"url,omitempty"`
	Attribution  string         `json:"attribution,omitempty"`
	PrintLimit   *licence.Limit `json:"print_limit,omitempty" desc:"Most of the item one print may include; omit for the type's default"`
	ExportLimit  *licence.Limit `json:"export_limit,omitempty"`
	NoLimits     bool           `json:"no_limits,omitempty" desc:"Remove print and export limits"`
	Commercial   *bool          `json:"commercial_allowed,omitempty"`
	Share        *bool          `json:"share_allowed,omitempty"`
	Store        *bool          `json:"store_allowed,omitempty"`
}

type SetLicenceResult struct {
	Updated int `json:"updated" desc:"Items whose licence was recorded"`
}

func setLicence(ctx context.Context, env *command.Env, p SetLicenceParams) (SetLicenceResult, error) {
	if (len(p.ItemIDs) > 0) == p.Pending {
		return SetLicenceResult{}, command.Errorf("bad_params", "give either item_ids or pending")
	}
	if (p.Type != "") == p.UseSuggested {
		return SetLicenceResult{}, command.Errorf("bad_params", "give either type or use_suggested")
	}
	var base licence.Licence
	if p.Type != "" {
		var err error
		if base, err = licence.Defaults(licence.Type(p.Type), p.SPDX); err != nil {
			return SetLicenceResult{}, command.Errorf("bad_params", "%v", err)
		}
	}
	ids, err := licenceTargets(ctx, env.Store, p)
	if err != nil {
		return SetLicenceResult{}, err
	}
	var res SetLicenceResult
	for _, batch := range chunks(ids, batchSize) {
		existing, err := licenceIDs(ctx, env.Store, batch)
		if err != nil {
			return res, err
		}
		rights := map[string]string{}
		if p.UseSuggested {
			if rights, err = rightsOf(ctx, env.Store, batch); err != nil {
				return res, err
			}
		}
		err = env.Store.Write(ctx, "library.set_licence", func(tx *store.Tx) error {
			for _, id := range batch {
				l := base
				if p.UseSuggested {
					l = suggest(rights[id])
				}
				if err := putLicence(tx, id, existing[id], p.apply(l)); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return res, err
		}
		res.Updated += len(batch)
	}
	return res, nil
}

func (p SetLicenceParams) apply(l licence.Licence) licence.Licence {
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&l.RightsHolder, p.RightsHolder)
	set(&l.URL, p.URL)
	set(&l.Attribution, p.Attribution)
	if p.UseSuggested {
		set(&l.SPDX, p.SPDX)
	}
	if p.NoLimits {
		l.Print, l.Export = nil, nil
	}
	if p.PrintLimit != nil {
		l.Print = p.PrintLimit
	}
	if p.ExportLimit != nil {
		l.Export = p.ExportLimit
	}
	for dst, v := range map[*bool]*bool{&l.Commercial: p.Commercial, &l.Share: p.Share, &l.Store: p.Store} {
		if v != nil {
			*dst = *v
		}
	}
	l.Confirmed = true
	return l
}

func licenceTargets(ctx context.Context, s *store.Store, p SetLicenceParams) ([]string, error) {
	if p.Pending {
		q := `SELECT i.id FROM items i WHERE i.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM licences l WHERE l.item_id = i.id)`
		args := []any{}
		if p.FolderID != "" {
			q += ` AND EXISTS (SELECT 1 FROM files f WHERE f.item_id = i.id AND f.folder_id = ? AND f.deleted_at IS NULL)`
			args = append(args, p.FolderID)
		}
		return queryStrings(ctx, s, q+` ORDER BY i.id`, args...)
	}
	want := map[string]bool{}
	for _, id := range p.ItemIDs {
		want[id] = true
	}
	var ids []string
	for _, batch := range chunks(p.ItemIDs, batchSize) {
		marks, args := in(batch)
		found, err := queryStrings(ctx, s, `SELECT id FROM items WHERE deleted_at IS NULL AND id IN (`+marks+`)`, args...)
		if err != nil {
			return nil, err
		}
		for _, id := range found {
			if want[id] {
				ids = append(ids, id)
				delete(want, id)
			}
		}
	}
	for id := range want {
		return nil, command.Errorf("not_found", "no item %s", id)
	}
	return ids, nil
}

// licenceIDs maps item IDs to the IDs of their existing licence rows.
func licenceIDs(ctx context.Context, s *store.Store, items []string) (map[string]string, error) {
	return pairs(ctx, s, `SELECT item_id, id FROM licences WHERE item_id IN (%s)`, items)
}

func rightsOf(ctx context.Context, s *store.Store, items []string) (map[string]string, error) {
	return pairs(ctx, s, `SELECT item_id, value FROM item_facets WHERE facet = 'rights' AND item_id IN (%s)`, items)
}

// pairs runs a two-column query whose %s is replaced by placeholders for ids.
func pairs(ctx context.Context, s *store.Store, q string, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	marks, args := in(ids)
	rows, err := s.Query(ctx, strings.Replace(q, "%s", marks, 1), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func limitJSON(l *licence.Limit) any {
	if l == nil {
		return nil
	}
	b, _ := json.Marshal(l)
	return string(b)
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func putLicence(tx *store.Tx, itemID, licenceID string, l licence.Licence) error {
	row := store.Row{"type": string(l.Type), "spdx_or_cc": nullable(l.SPDX), "rights_holder": nullable(l.RightsHolder),
		"licence_url": nullable(l.URL), "attribution_text": nullable(l.Attribution),
		"print_limit": limitJSON(l.Print), "export_limit": limitJSON(l.Export),
		"commercial_allowed": boolInt(l.Commercial), "share_allowed": boolInt(l.Share),
		"store_allowed": boolInt(l.Store), "confirmed_by_user": boolInt(l.Confirmed), "updated_at": now()}
	if licenceID != "" {
		return tx.Update("licences", store.ByID(licenceID), row)
	}
	row["id"], row["item_id"] = store.NewID(), itemID
	return tx.Create("licences", row)
}

func licenceFromRow(r store.Row) licence.Licence {
	l := licence.Licence{Type: licence.Type(str(r["type"])), SPDX: str(r["spdx_or_cc"]),
		RightsHolder: str(r["rights_holder"]), URL: str(r["licence_url"]), Attribution: str(r["attribution_text"]),
		Commercial: num(r["commercial_allowed"]) == 1, Share: num(r["share_allowed"]) == 1,
		Store: num(r["store_allowed"]) == 1, Confirmed: num(r["confirmed_by_user"]) == 1}
	for dst, col := range map[**licence.Limit]string{&l.Print: "print_limit", &l.Export: "export_limit"} {
		if s := str(r[col]); s != "" {
			var lim licence.Limit
			if json.Unmarshal([]byte(s), &lim) == nil {
				*dst = &lim
			}
		}
	}
	return l
}
