// Package licence holds each library item's licence and decides what printing, exporting and
// sharing it allows. Limits come from the licence the user confirmed at import, not from a single
// rule for every book.
package licence

import (
	"fmt"
	"math"
	"strings"
)

type Type string

const (
	PublicDomain     Type = "public_domain"
	CreativeCommons  Type = "creative_commons"
	OpenOther        Type = "open_other"
	PersonalPurchase Type = "personal_purchase"
	OwnWork          Type = "own_work"
	Unknown          Type = "unknown"
)

// Types lists every licence type. The interface offers four of them in plain words (Plain).
var Types = []Type{PublicDomain, CreativeCommons, OpenOther, PersonalPurchase, OwnWork, Unknown}

// Limit caps how much of an item one print or export may include. A zero field is not a limit; a
// nil *Limit means no limit at all.
type Limit struct {
	Pages   int     `json:"pages,omitempty"`
	Percent float64 `json:"percent,omitempty"`
}

// DefaultPersonalLimit applies to purchased and unknown items until the user sets their own.
var DefaultPersonalLimit = Limit{Percent: 10}

type Licence struct {
	Type         Type   `json:"type"`
	SPDX         string `json:"spdx,omitempty"` // SPDX or Creative Commons ID, e.g. CC-BY-SA-4.0
	RightsHolder string `json:"rights_holder,omitempty"`
	URL          string `json:"url,omitempty"`
	Attribution  string `json:"attribution,omitempty"`
	Print        *Limit `json:"print_limit,omitempty"`
	Export       *Limit `json:"export_limit,omitempty"`
	Commercial   bool   `json:"commercial_allowed"`
	Share        bool   `json:"share_allowed"` // may a copy of the file be given to others
	Store        bool   `json:"store_allowed"` // may the file be kept in the library
	Confirmed    bool   `json:"confirmed_by_user"`
	Origin       string `json:"origin,omitempty"` // FromFile, ByDefault or ByUser
}

// Defaults returns the starting licence the import prompt shows for a type. The user confirms or
// changes it before it is saved.
func Defaults(t Type, spdx string) (Licence, error) {
	l := Licence{Type: t, SPDX: spdx, Store: true}
	switch t {
	case PublicDomain, OwnWork:
		l.Commercial, l.Share = true, true
	case CreativeCommons:
		id := strings.ToUpper(spdx)
		if id != "" && !strings.HasPrefix(id, "CC") {
			return Licence{}, fmt.Errorf("licence: %q is not a Creative Commons licence", spdx)
		}
		l.Share = true
		l.Commercial = !strings.Contains(id, "-NC")
	case OpenOther:
		l.Share = true
	case PersonalPurchase, Unknown:
		p, e := DefaultPersonalLimit, DefaultPersonalLimit
		l.Print, l.Export = &p, &e
	default:
		return Licence{}, fmt.Errorf("licence: unknown type %q", t)
	}
	return l, nil
}

// NeedsAttribution reports whether copies must credit the rights holder.
func (l Licence) NeedsAttribution() bool {
	switch l.Type {
	case PublicDomain, OwnWork:
		return false
	case CreativeCommons:
		return !strings.HasPrefix(strings.ToUpper(l.SPDX), "CC0")
	}
	return l.Attribution != "" || l.RightsHolder != ""
}

// AttributionLine is the credit printed with any copy, or "" when none is needed.
func (l Licence) AttributionLine() string {
	if !l.NeedsAttribution() {
		return ""
	}
	if l.Attribution != "" {
		return l.Attribution
	}
	parts := []string{}
	if l.RightsHolder != "" {
		parts = append(parts, "© "+l.RightsHolder)
	}
	if l.SPDX != "" {
		parts = append(parts, "licensed under "+l.SPDX)
	}
	if l.URL != "" {
		parts = append(parts, l.URL)
	}
	return strings.Join(parts, ", ")
}

type Action string

const (
	Print  Action = "print"
	Export Action = "export"
	Share  Action = "share"
)

// Request describes something the user wants to do with an item.
type Request struct {
	Action     Action `json:"action"`
	Pages      int    `json:"pages,omitempty"`       // pages the print or export would include
	TotalPages int    `json:"total_pages,omitempty"` // pages in the whole item, for percentage limits
	Commercial bool   `json:"commercial,omitempty"`
}

type Decision struct {
	Allowed     bool   `json:"allowed"`
	MaxPages    int    `json:"max_pages,omitempty"` // the most this licence allows, when limited
	Attribution string `json:"attribution,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Confirm     bool   `json:"confirm,omitempty"` // the licence was never confirmed; ask the user
}

// Check decides whether the licence allows a request.
func (l Licence) Check(r Request) Decision {
	d := Decision{Attribution: l.AttributionLine(), Confirm: !l.Confirmed}
	deny := func(reason string) Decision { d.Reason = reason; return d }
	if r.Commercial && !l.Commercial {
		return deny("the licence does not allow commercial use")
	}
	var limit *Limit
	switch r.Action {
	case Share:
		if !l.Share {
			return deny("the licence does not allow giving copies to others")
		}
		d.Allowed = true
		return d
	case Print:
		limit = l.Print
	case Export:
		limit = l.Export
	default:
		return deny(fmt.Sprintf("unknown action %q", r.Action))
	}
	if limit == nil {
		d.Allowed = true
		return d
	}
	max, ok := limit.maxPages(r.TotalPages)
	if !ok {
		return deny("the limit is a percentage and the item's length is unknown")
	}
	d.MaxPages = max
	if r.Pages > max {
		return deny(fmt.Sprintf("the licence allows %s of at most %d pages at a time", r.Action, max))
	}
	d.Allowed = true
	return d
}

func (lim Limit) maxPages(total int) (int, bool) {
	max := math.MaxInt
	if lim.Pages > 0 {
		max = lim.Pages
	}
	if lim.Percent > 0 {
		if total <= 0 {
			return 0, false
		}
		if p := int(math.Floor(float64(total) * lim.Percent / 100)); p < max {
			max = p
		}
	}
	if max < 1 && (lim.Pages > 0 || lim.Percent > 0) {
		max = 1
	}
	return max, true
}
