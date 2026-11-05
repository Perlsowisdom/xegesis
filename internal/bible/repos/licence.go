package repos

import (
	"regexp"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/licence"
)

type mapped = bible.MappedLicence

func must(t licence.Type, spdx string) licence.Licence { return bible.MustLicence(t, spdx) }

func swordLicence(c sword.Conf) mapped { return bible.SwordLicence(c) }

func restricted() licence.Licence { return bible.RestrictedLicence() }

// ebibleLicence maps eBible.org's catalogue fields. Only "public domain" is clear from the
// catalogue alone; other licences are read from the translation's copyright page (resolveEBible).
func ebibleLicence(copyright string, redistributable bool) mapped {
	m := mapped{Rights: "Copyright: " + copyright}
	if !redistributable {
		m.Rights += "\nRedistributable: False"
	}
	if strings.Contains(strings.ToLower(copyright), "public domain") {
		m.Licence, m.Clear = must(licence.PublicDomain, ""), true
		return m
	}
	m.Licence = restricted()
	m.Licence.Attribution = copyright
	return m
}

var ccLink = regexp.MustCompile(`creativecommons\.org/(licenses/([a-z-]+)/([0-9.]+)|publicdomain/(zero)/1\.0|publicdomain/(mark))`)

// ccFromPage finds a Creative Commons licence link on a copyright page.
func ccFromPage(page string) (licence.Licence, string, bool) {
	m := ccLink.FindStringSubmatch(page)
	switch {
	case m == nil:
		return licence.Licence{}, "", false
	case m[5] != "":
		return must(licence.PublicDomain, ""), "https://" + m[0], true
	case m[4] != "":
		return must(licence.CreativeCommons, "CC0-1.0"), "https://" + m[0], true
	}
	spdx := "CC-" + strings.ToUpper(m[2]) + "-" + m[3]
	l, err := licence.Defaults(licence.CreativeCommons, spdx)
	if err != nil {
		return licence.Licence{}, "", false
	}
	l.URL = "https://" + m[0]
	return l, l.URL, true
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
