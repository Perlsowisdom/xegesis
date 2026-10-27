package repos

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/licence"
)

// Source kinds.
const (
	KindSWORD  = "sword"  // a SWORD repository: mods.d.tar.gz and module folders
	KindEBible = "ebible" // eBible.org's translations.csv and USFM zips
)

// Source is one catalogue.
type Source struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind" desc:"sword or ebible"`
	URL       string `json:"url" desc:"Repository folder (sword) or site (ebible)"`
	Refreshed int64  `json:"refreshed_at,omitempty" desc:"Unix time in milliseconds of the last successful refresh"`
	Items     int    `json:"items" desc:"Entries in the cached catalogue"`
	Error     string `json:"error,omitempty" desc:"Why the last refresh failed"`
}

// Item is one text a catalogue offers.
type Item struct {
	Source       string          `json:"source"`
	SourceName   string          `json:"source_name"`
	ID           string          `json:"id" desc:"The item's ID in its source (SWORD module name or eBible.org translation ID)"`
	Name         string          `json:"name"`
	Abbreviation string          `json:"abbreviation,omitempty"`
	Language     string          `json:"language" desc:"BCP 47 language tag"`
	LanguageName string          `json:"language_name"`
	Kind         string          `json:"kind" desc:"bible, commentary, dictionary or book"`
	Version      string          `json:"version,omitempty"`
	Size         int64           `json:"size,omitempty" desc:"Bytes to download, when the catalogue says"`
	Licence      licence.Licence `json:"licence" desc:"The licence mapped from the catalogue's own fields"`
	LicenceClear bool            `json:"licence_clear" desc:"False when the catalogue's words are unclear; bible.get then needs licence_type"`
	Rights       string          `json:"rights" desc:"What the source says about copyright and distribution, word for word"`
	About        string          `json:"about,omitempty"`
	Locked       bool            `json:"locked" desc:"Enciphered; needs the publisher's unlock key and is never installed"`
	Supported    bool            `json:"supported" desc:"Xegesis can install it"`
	Reason       string          `json:"reason,omitempty" desc:"Why it cannot be installed"`
	Origin       string          `json:"origin" desc:"Recorded as the installed module's source"`
	Installed    bool            `json:"installed"`
	Module       string          `json:"module,omitempty" desc:"Installed module ID"`
	Update       bool            `json:"update_available,omitempty"`

	conf *sword.Conf
	base string // SWORD repository URL or eBible.org site
}

// parseMasterList reads CrossWire's masterRepoList.conf: lines "id=Type=Name|host|path" in a
// [Repos] section. Only HTTPS repositories are used.
func parseMasterList(r io.Reader) []Source {
	var out []Source
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == '[' {
			continue
		}
		parts := strings.SplitN(line, "=", 3)
		if len(parts) != 3 || parts[1] != "HTTPSPackagePreference" {
			continue
		}
		f := strings.Split(parts[2], "|")
		if len(f) < 3 || f[0] == "" || f[1] == "" || strings.ContainsAny(f[1], "/ ") {
			continue
		}
		id := slug(f[0])
		if seen[id] || id == "" {
			continue
		}
		seen[id] = true
		p := "/" + strings.Trim(f[2], "/")
		out = append(out, Source{ID: id, Name: f[0], Kind: KindSWORD, URL: "https://" + f[1] + p})
	}
	return out
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// maxConf caps one .conf file in a catalogue archive.
const maxConf = 1 << 20

// parseSWORD reads a repository's mods.d.tar.gz.
func parseSWORD(src Source, data []byte) ([]Item, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var out []Item
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg || !strings.HasSuffix(strings.ToLower(h.Name), ".conf") {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(tr, maxConf))
		if err != nil {
			return nil, err
		}
		c, err := sword.ParseConf(bytes.NewReader(body))
		if err != nil {
			continue // one bad file does not spoil the catalogue
		}
		out = append(out, swordItem(src, c))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func swordItem(src Source, c sword.Conf) Item {
	lic := swordLicence(c)
	it := Item{Source: src.ID, SourceName: src.Name, ID: c.Name, Name: firstNonEmpty(c.Get("Description"), c.Name),
		Abbreviation: firstNonEmpty(c.Get("Abbreviation"), c.Name), Kind: c.Kind(), Version: c.Get("Version"),
		Size: c.Size(), Licence: lic.Licence, LicenceClear: lic.Clear, Rights: lic.Rights, About: c.About(),
		Locked: c.Locked(), Origin: src.URL + "#" + c.Name, conf: &c, base: src.URL}
	it.Language, it.LanguageName = lang(c.Get("Lang"), "")
	if err := sword.Supported(c); err != nil {
		it.Reason = strings.TrimPrefix(err.Error(), "sword: ")
	} else if strings.Contains(c.Get("DataPath"), "..") {
		it.Reason = "its data path leaves the repository"
	} else {
		it.Supported = true
	}
	return it
}

// lang normalises a language code ("eng" becomes "en") and names it in English.
func lang(code, name string) (string, string) {
	t, err := language.Parse(strings.TrimSpace(code))
	if err != nil || code == "" {
		return "und", firstNonEmpty(name, "Unknown language")
	}
	if name == "" {
		b, _ := t.Base()
		name = display.English.Languages().Name(b)
	}
	return t.String(), firstNonEmpty(name, t.String())
}

// eBible.org translations.csv columns used here.
var ebibleColumns = []string{"languageCode", "translationId", "languageNameInEnglish", "title",
	"Redistributable", "Copyright", "UpdateDate", "OTverses", "NTverses", "downloadable", "shortTitle", "description"}

// parseEBible reads eBible.org's translations.csv.
func parseEBible(src Source, data []byte) ([]Item, error) {
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	for _, c := range ebibleColumns {
		if _, ok := col[c]; !ok {
			return nil, fmt.Errorf("translations.csv has no %q column", c)
		}
	}
	var out []Item
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		get := func(name string) string {
			if i := col[name]; i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		if it, ok := ebibleItem(src, get); ok {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func ebibleItem(src Source, get func(string) string) (Item, bool) {
	id := get("translationId")
	if !validItemID(id) {
		return Item{}, false
	}
	verses := atoi(get("OTverses")) + atoi(get("NTverses"))
	lic := ebibleLicence(get("Copyright"), strings.EqualFold(get("Redistributable"), "true"))
	it := Item{Source: src.ID, SourceName: src.Name, ID: id, Name: firstNonEmpty(get("title"), id),
		Abbreviation: abbreviation(get("shortTitle"), id), Kind: "bible", Version: get("UpdateDate"),
		Licence: lic.Licence, LicenceClear: lic.Clear, Rights: lic.Rights, About: get("description"),
		Origin: src.URL + "/" + id + "/", base: src.URL}
	it.Language, it.LanguageName = lang(get("languageCode"), get("languageNameInEnglish"))
	switch {
	case !strings.EqualFold(get("downloadable"), "true"):
		it.Reason = "eBible.org does not offer it for download"
	case verses == 0:
		it.Reason = "eBible.org lists no verses for it"
	default:
		it.Supported = true
	}
	return it, true
}

func abbreviation(short, id string) string {
	if short != "" && len(short) <= 12 && !strings.Contains(short, " ") {
		return short
	}
	return strings.ToUpper(id)
}

func validItemID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// newer reports whether version a is later than b: dotted numbers compare numerically, other
// strings (eBible.org dates) as text.
func newer(a, b string) bool {
	if a == "" || a == b {
		return false
	}
	if b == "" {
		return true
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	numeric := true
	for _, p := range append(append([]string{}, pa...), pb...) {
		if _, err := strconv.Atoi(p); err != nil {
			numeric = false
		}
	}
	if !numeric {
		return a > b
	}
	for i := 0; i < max(len(pa), len(pb)); i++ {
		x, y := 0, 0
		if i < len(pa) {
			x = atoi(pa[i])
		}
		if i < len(pb) {
			y = atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// joinURL appends slash-separated parts to a base URL.
func joinURL(base string, parts ...string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path.Join(parts...), "/")
}
