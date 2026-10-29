// Package sword reads Bible modules of the CrossWire SWORD Project. It is written from CrossWire's
// published documentation (the wiki's module .conf reference and repository pages) and from
// inspecting public-domain modules; no SWORD, JSword or front-end code was used (ADR 0007).
//
// Supported: the RawText, RawText4, zText and zText4 drivers with ZIP (zlib) or BZIP2 compression,
// OSIS, ThML, GBF or plain-text entries in UTF-8 or Latin-1, in the verse numberings listed in
// Versifications. Locked (enciphered) modules are refused.
package sword

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Conf is a module's .conf file: its name and every key with its values in file order.
type Conf struct {
	Name   string
	fields map[string][]string
}

var validName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// keys whose values may continue on the next line after a trailing backslash.
var continued = map[string]bool{
	"About": true, "Copyright": true, "CopyrightNotes": true, "CopyrightContactName": true,
	"CopyrightContactNotes": true, "CopyrightContactAddress": true, "DistributionNotes": true,
	"TextSource": true, "UnlockInfo": true,
}

// ParseConf reads a .conf file. The first line that is not blank or a comment must be the
// [ModName] section header. Repeated keys keep every value.
func ParseConf(r io.Reader) (Conf, error) {
	c := Conf{fields: map[string][]string{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	var key string // key whose value continues on the next line
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if key != "" {
			vals := c.fields[key]
			more, cont := cutContinuation(line)
			vals[len(vals)-1] += "\n" + more
			if !cont {
				key = ""
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if c.Name == "" {
			name, ok := strings.CutPrefix(trimmed, "[")
			name, ok2 := strings.CutSuffix(name, "]")
			if !ok || !ok2 || !validName.MatchString(name) {
				return c, fmt.Errorf("sword: conf must start with [ModName], found %q", trimmed)
			}
			c.Name = name
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v, cont := cutContinuation(strings.TrimSpace(v))
		c.fields[k] = append(c.fields[k], v)
		if cont && continued[baseKey(k)] {
			key = k
		}
	}
	if err := sc.Err(); err != nil {
		return c, err
	}
	if c.Name == "" {
		return c, errors.New("sword: empty conf file")
	}
	return c, nil
}

func cutContinuation(s string) (string, bool) {
	if strings.HasSuffix(s, `\`) && !strings.HasSuffix(s, `\\`) {
		return strings.TrimSuffix(s, `\`), true
	}
	return s, false
}

// baseKey drops a locale suffix such as "_fr" from a key.
func baseKey(k string) string {
	if i := strings.IndexByte(k, '_'); i > 0 && !strings.HasPrefix(k, "History") {
		return k[:i]
	}
	return k
}

// Get returns a key's first value, or "".
func (c Conf) Get(key string) string {
	if v := c.fields[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// All returns every value of a repeatable key.
func (c Conf) All(key string) []string { return c.fields[key] }

// Has reports whether the key is present, even with an empty value.
func (c Conf) Has(key string) bool { _, ok := c.fields[key]; return ok }

// Locked reports a module whose text is enciphered (a CipherKey entry). Xegesis does not read
// these: they need the publisher's unlock key.
func (c Conf) Locked() bool { return c.Has("CipherKey") }

// Size is the InstallSize in bytes, or 0 when not given.
func (c Conf) Size() int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(c.Get("InstallSize")), 10, 64)
	return n
}

// Driver is the ModDrv value.
func (c Conf) Driver() string { return c.Get("ModDrv") }

// Kind sorts a module into bible, commentary, dictionary or book by its driver.
func (c Conf) Kind() string {
	switch strings.ToLower(c.Driver()) {
	case "rawtext", "rawtext4", "ztext", "ztext4":
		return "bible"
	case "rawcom", "rawcom4", "zcom", "zcom4", "hrefcom", "rawfiles":
		return "commentary"
	case "rawld", "rawld4", "zld":
		return "dictionary"
	}
	return "book"
}

// DataPath is the module's folder relative to the repository root, without "./" and with a
// trailing slash.
// For a dictionary the DataPath ends with its files' name prefix, which is left out here.
func (c Conf) DataPath() string {
	p := strings.TrimPrefix(strings.TrimSpace(c.Get("DataPath")), "./")
	if c.Kind() == "dictionary" {
		p = strings.TrimSuffix(p, "/")
		if i := strings.LastIndexByte(p, '/'); i >= 0 {
			p = p[:i]
		} else {
			p = ""
		}
	}
	if p != "" && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

// About is the About text with its RTF codes (\par, \qc, \pard, \u{n}?) turned into plain text.
func (c Conf) About() string { return PlainRTF(c.Get("About")) }

var rtfUnicode = regexp.MustCompile(`\\u(-?[0-9]+)\??`)

// PlainRTF removes the small RTF subset the .conf documentation allows.
func PlainRTF(s string) string {
	s = rtfUnicode.ReplaceAllStringFunc(s, func(m string) string {
		n, err := strconv.Atoi(strings.TrimSuffix(m[2:], "?"))
		if err != nil {
			return ""
		}
		if n < 0 {
			n += 65536
		}
		return string(rune(n))
	})
	s = strings.NewReplacer(`\pard`, "", `\par`, "\n", `\qc`, "").Replace(s)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
