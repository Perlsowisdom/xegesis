package licence

import (
	"regexp"
	"strings"
)

// Where a licence came from.
const (
	FromFile  = "file"    // the book or Bible's own details name it
	ByDefault = "default" // nothing was stated, so the item is a personal copy
	ByUser    = "user"    // the user chose it
)

var (
	ccURLRe  = regexp.MustCompile(`(?i)creativecommons\.org/(?:licenses|publicdomain)/([a-z-]+)(?:/(\d\.\d))?`)
	ccNameRe = regexp.MustCompile(`(?i)\bCC[- ]?(0|BY(?:[- ](?:SA|NC|ND))*)(?:[- ]v?(\d\.\d))?\b`)
	ccWordRe = regexp.MustCompile(`(?i)creative commons(?: licen[cs]e)?[:\s]+(0|zero|BY(?:[- ](?:SA|NC|ND))*)(?:[- ]v?(\d\.\d))?\b`)
)

// FromRights reads a rights statement, such as an EPUB's dc:rights or an OSIS <rights>, and returns
// the licence it names. ok is false when it names none.
func FromRights(rights string) (l Licence, ok bool) {
	r := strings.ToLower(rights)
	t, spdx := Type(""), ""
	switch {
	case r == "":
	case ccURLRe.MatchString(rights):
		m := ccURLRe.FindStringSubmatch(rights)
		if strings.EqualFold(m[1], "mark") {
			t = PublicDomain
		} else {
			t, spdx = CreativeCommons, ccID(m[1], m[2])
		}
	case ccNameRe.MatchString(rights):
		m := ccNameRe.FindStringSubmatch(rights)
		t, spdx = CreativeCommons, ccID(m[1], m[2])
	case ccWordRe.MatchString(rights):
		m := ccWordRe.FindStringSubmatch(rights)
		t, spdx = CreativeCommons, ccID(m[1], m[2])
	case strings.Contains(r, "public domain") && !strings.Contains(r, "not in the public domain"):
		t = PublicDomain
	case strings.Contains(r, "creative commons"):
		t = CreativeCommons
	}
	if t == "" {
		return Licence{}, false
	}
	l, err := Defaults(t, spdx)
	if err != nil {
		return Licence{}, false
	}
	l.Origin = FromFile
	return l, true
}

// Default is the licence of an item that states none: a personal copy with the usual limits.
func Default() Licence {
	l, _ := Defaults(PersonalPurchase, "")
	l.Origin = ByDefault
	return l
}

// Detect returns the licence a rights statement names, or the default.
func Detect(rights string) Licence {
	if l, ok := FromRights(rights); ok {
		return l
	}
	return Default()
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

// Plain names a licence in the interface's four plain words. Unknown licences behave as personal
// copies and are named so.
func (l Licence) Plain() string {
	switch l.Type {
	case PublicDomain:
		return "Public domain"
	case CreativeCommons:
		return "Free to share"
	case OpenOther:
		if l.Share {
			return "Free to share"
		}
	case OwnWork:
		return "My own work"
	}
	return "Personal copy"
}
