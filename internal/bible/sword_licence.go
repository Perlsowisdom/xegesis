package bible

import (
	"regexp"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/licence"
)

// MappedLicence is a licence read from a catalogue or a module's own .conf. Clear is false when
// the words do not say plainly what is allowed; the user is then asked.
type MappedLicence struct {
	Licence licence.Licence
	Clear   bool
	Rights  string // what the source said, word for word
}

func MustLicence(t licence.Type, spdx string) licence.Licence {
	l, err := licence.Defaults(t, spdx)
	if err != nil {
		l, _ = licence.Defaults(licence.Unknown, "")
	}
	return l
}

// SwordLicence maps a module's DistributionLicense, using the values the CrossWire .conf
// documentation lists.
func SwordLicence(c sword.Conf) MappedLicence {
	dl := strings.TrimSpace(c.Get("DistributionLicense"))
	copyright := sword.PlainRTF(firstNonEmpty(c.Get("Copyright"), c.Get("ShortCopyright")))
	var rights []string
	for _, kv := range [][2]string{{"DistributionLicense", dl}, {"Copyright", copyright},
		{"CopyrightHolder", c.Get("CopyrightHolder")}, {"DistributionNotes", sword.PlainRTF(c.Get("DistributionNotes"))}} {
		if kv[1] != "" {
			rights = append(rights, kv[0]+": "+kv[1])
		}
	}
	m := MappedLicence{Rights: strings.Join(rights, "\n"), Clear: true}
	lower := strings.ToLower(dl)
	switch {
	case lower == "public domain":
		m.Licence = MustLicence(licence.PublicDomain, "")
	case strings.HasPrefix(lower, "creative commons:"):
		spdx := ccSPDX(strings.TrimSpace(dl[len("creative commons:"):]))
		m.Licence, m.Clear = MustLicence(licence.CreativeCommons, spdx), spdx != ""
	case lower == "gpl" || lower == "gfdl":
		// Free licences for the module as a whole; the version is not stated.
		m.Licence = MustLicence(licence.OpenOther, "")
		m.Licence.Commercial = true
	case lower == "copyrighted; freely distributable", lower == "copyrighted; free non-commercial distribution":
		m.Licence = MustLicence(licence.OpenOther, "")
	case strings.HasPrefix(lower, "copyrighted; permission to distribute granted to"),
		lower == "copyrighted; permission granted to distribute non-commercially in sword format":
		// The repository may distribute it; the user may read it but not pass it on.
		m.Licence = RestrictedLicence()
	default:
		m.Licence, m.Clear = RestrictedLicence(), false
	}
	if m.Licence.Type != licence.PublicDomain {
		m.Licence.RightsHolder = strings.TrimSpace(c.Get("CopyrightHolder"))
		m.Licence.Attribution = copyright
	}
	return m
}

// RestrictedLicence is a copyrighted text the user may read and quote within the usual limits.
func RestrictedLicence() licence.Licence {
	l := MustLicence(licence.OpenOther, "")
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
