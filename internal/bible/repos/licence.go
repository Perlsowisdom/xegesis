package repos

import (
	"regexp"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/licence"
)

// mapped is a licence read from a catalogue. Clear is false when the source's words do not say
// plainly what is allowed; the user is then asked.
type mapped struct {
	Licence licence.Licence
	Clear   bool
	Rights  string // what the source said, word for word
}

func must(t licence.Type, spdx string) licence.Licence {
	l, err := licence.Defaults(t, spdx)
	if err != nil {
		l, _ = licence.Defaults(licence.Unknown, "")
	}
	return l
}

// swordLicence maps a module's DistributionLicense, using the values the CrossWire .conf
// documentation lists.
func swordLicence(c sword.Conf) mapped {
	dl := strings.TrimSpace(c.Get("DistributionLicense"))
	copyright := sword.PlainRTF(firstNonEmpty(c.Get("Copyright"), c.Get("ShortCopyright")))
	var rights []string
	for _, kv := range [][2]string{{"DistributionLicense", dl}, {"Copyright", copyright},
		{"CopyrightHolder", c.Get("CopyrightHolder")}, {"DistributionNotes", sword.PlainRTF(c.Get("DistributionNotes"))}} {
		if kv[1] != "" {
			rights = append(rights, kv[0]+": "+kv[1])
		}
	}
	m := mapped{Rights: strings.Join(rights, "\n"), Clear: true}
	lower := strings.ToLower(dl)
	switch {
	case lower == "public domain":
		m.Licence = must(licence.PublicDomain, "")
	case strings.HasPrefix(lower, "creative commons:"):
		spdx := ccSPDX(strings.TrimSpace(dl[len("creative commons:"):]))
		m.Licence, m.Clear = must(licence.CreativeCommons, spdx), spdx != ""
	case lower == "gpl" || lower == "gfdl":
		// Free licences for the module as a whole; the version is not stated.
		m.Licence = must(licence.OpenOther, "")
		m.Licence.Commercial = true
	case lower == "copyrighted; freely distributable", lower == "copyrighted; free non-commercial distribution":
		m.Licence = must(licence.OpenOther, "")
	case strings.HasPrefix(lower, "copyrighted; permission to distribute granted to"),
		lower == "copyrighted; permission granted to distribute non-commercially in sword format":
		// The repository may distribute it; the user may read it but not pass it on.
		m.Licence = restricted()
	default:
		m.Licence, m.Clear = restricted(), false
	}
	if m.Licence.Type != licence.PublicDomain {
		m.Licence.RightsHolder = strings.TrimSpace(c.Get("CopyrightHolder"))
		m.Licence.Attribution = copyright
	}
	return m
}

// restricted is a copyrighted text the user may read and quote within the usual limits.
func restricted() licence.Licence {
	l := must(licence.OpenOther, "")
	l.Share = false
	p, e := licence.DefaultPersonalLimit, licence.DefaultPersonalLimit
	l.Print, l.Export = &p, &e
	return l
}

var ccCode = regexp.MustCompile(`(?i)^(?:cc[- ])?(by(?:-nc)?(?:-sa|-nd)?)\s*([0-9]\.[0-9])$`)

// ccSPDX turns "BY-SA 4.0" or "CC0" into an SPDX ID, or "" when no version is given.
func ccSPDX(s string) string {
	if strings.EqualFold(s, "cc0") || strings.EqualFold(s, "cc0 1.0") {
		return "CC0-1.0"
	}
	m := ccCode.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return ""
	}
	return "CC-" + strings.ToUpper(m[1]) + "-" + m[2]
}

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
