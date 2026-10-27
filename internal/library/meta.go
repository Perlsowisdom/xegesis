package library

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Meta is what a book file says about itself.
type Meta struct {
	Title     string
	Subtitle  string
	Language  string
	Year      int
	Publisher string
	ISBN      string
	Series    string
	SeriesNo  string
	Rights    string
	People    []Person
	Subjects  []string
}

// Person is a name credited in a file's metadata.
type Person struct {
	Name     string
	SortName string
	Role     string // author, editor, translator
}

// formats maps file extensions to the format names stored in files.format.
var formats = map[string]string{
	".epub": "epub", ".pdf": "pdf", ".docx": "docx", ".html": "html", ".htm": "html",
	".xhtml": "html", ".md": "md", ".markdown": "md", ".txt": "txt",
}

func formatOf(name string) string { return formats[strings.ToLower(filepath.Ext(name))] }

// extract reads metadata from a file. It always returns a title, falling back to the file name.
func extract(file, format string) (Meta, error) {
	var m Meta
	var err error
	switch format {
	case "epub":
		m, err = epubMeta(file)
	case "docx":
		m, err = docxMeta(file)
	case "pdf":
		m, err = pdfMeta(file)
	case "html":
		m, err = withHead(file, htmlMeta)
	case "md":
		m, err = withHead(file, mdMeta)
	case "txt":
		m, err = withHead(file, txtMeta)
	}
	m.tidy()
	if m.Title == "" {
		m.Title = titleFromName(file)
	}
	return m, err
}

func titleFromName(file string) string {
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	return strings.TrimSpace(strings.NewReplacer("_", " ").Replace(base))
}

var spaces = regexp.MustCompile(`\s+`)

func clean(s string) string { return strings.TrimSpace(spaces.ReplaceAllString(s, " ")) }

func (m *Meta) tidy() {
	m.Title, m.Subtitle, m.Language = clean(m.Title), clean(m.Subtitle), clean(m.Language)
	m.Publisher, m.ISBN, m.Rights = clean(m.Publisher), clean(m.ISBN), clean(m.Rights)
	m.Series, m.SeriesNo = clean(m.Series), clean(m.SeriesNo)
	seen := map[string]bool{}
	var people []Person
	for _, p := range m.People {
		p.Name = clean(p.Name)
		if p.Role == "" {
			p.Role = "author"
		}
		if p.Name == "" || seen[p.Role+"\x00"+p.Name] {
			continue
		}
		seen[p.Role+"\x00"+p.Name] = true
		if p.SortName = clean(p.SortName); p.SortName == "" {
			p.SortName = sortName(p.Name)
		}
		people = append(people, p)
	}
	m.People = people
	seen = map[string]bool{}
	var subjects []string
	for _, s := range m.Subjects {
		if s = clean(s); s != "" && !seen[strings.ToLower(s)] {
			seen[strings.ToLower(s)] = true
			subjects = append(subjects, s)
		}
	}
	m.Subjects = subjects
}

// sortName turns "John Calvin" into "Calvin, John". Names that already have a comma are kept.
func sortName(name string) string {
	if strings.Contains(name, ",") {
		return name
	}
	parts := strings.Fields(name)
	if len(parts) < 2 {
		return name
	}
	return parts[len(parts)-1] + ", " + strings.Join(parts[:len(parts)-1], " ")
}

var yearRe = regexp.MustCompile(`(?:^|\D)(\d{4})`)

func yearOf(date string) int {
	var y int
	if m := yearRe.FindStringSubmatch(date); m != nil {
		fmt.Sscan(m[1], &y)
	}
	return y
}

var isbnRe = regexp.MustCompile(`^(?:97[89])?\d{9}[\dX]$`)

// isbnOf returns the ISBN in an identifier, or "".
func isbnOf(id, scheme string) string {
	s := strings.ToUpper(strings.TrimSpace(id))
	s = strings.TrimPrefix(strings.TrimPrefix(s, "URN:ISBN:"), "ISBN:")
	s = strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s))
	if isbnRe.MatchString(s) || (strings.EqualFold(scheme, "isbn") && s != "") {
		return s
	}
	return ""
}

func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' })
}

// EPUB (OCF container, OPF package metadata) -----------------------------------------------

type xmlElem struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Value   string     `xml:",chardata"`
}

func (e xmlElem) attr(local string) string {
	for _, a := range e.Attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func zipEntry(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			r, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer r.Close()
			return io.ReadAll(io.LimitReader(r, 4<<20))
		}
	}
	return nil, fmt.Errorf("%s not found", name)
}

func epubMeta(file string) (Meta, error) {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return Meta{}, err
	}
	defer zr.Close()
	b, err := zipEntry(&zr.Reader, "META-INF/container.xml")
	if err != nil {
		return Meta{}, err
	}
	var c struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := xml.Unmarshal(b, &c); err != nil || len(c.Rootfiles) == 0 {
		return Meta{}, fmt.Errorf("epub container: no package document")
	}
	if b, err = zipEntry(&zr.Reader, path.Clean(c.Rootfiles[0].FullPath)); err != nil {
		return Meta{}, err
	}
	return opfMeta(b)
}

var epubRoles = map[string]string{"aut": "author", "edt": "editor", "trl": "translator"}

func opfMeta(b []byte) (Meta, error) {
	var pkg struct {
		Metadata struct {
			Elems []xmlElem `xml:",any"`
		} `xml:"metadata"`
	}
	if err := xml.Unmarshal(b, &pkg); err != nil {
		return Meta{}, fmt.Errorf("epub package: %w", err)
	}
	elems := pkg.Metadata.Elems
	refines := map[string]map[string]string{} // EPUB 3: <meta refines="#id" property="role">
	for _, e := range elems {
		if e.XMLName.Local == "meta" && e.attr("refines") != "" {
			id := strings.TrimPrefix(e.attr("refines"), "#")
			if refines[id] == nil {
				refines[id] = map[string]string{}
			}
			refines[id][e.attr("property")] = strings.TrimSpace(e.Value)
		}
	}
	var m Meta
	for _, e := range elems {
		ref := refines[e.attr("id")]
		v := strings.TrimSpace(e.Value)
		switch e.XMLName.Local {
		case "title":
			if ref["title-type"] == "subtitle" {
				m.Subtitle = v
			} else if m.Title == "" {
				m.Title = v
			}
		case "creator", "contributor":
			role := e.attr("role")
			if role == "" {
				role = ref["role"]
			}
			r, ok := epubRoles[role]
			if !ok && e.XMLName.Local == "contributor" {
				continue
			}
			sort := e.attr("file-as")
			if sort == "" {
				sort = ref["file-as"]
			}
			m.People = append(m.People, Person{Name: v, SortName: sort, Role: r})
		case "language":
			if m.Language == "" {
				m.Language = v
			}
		case "date":
			if m.Year == 0 {
				m.Year = yearOf(v)
			}
		case "publisher":
			m.Publisher = v
		case "identifier":
			if m.ISBN == "" {
				m.ISBN = isbnOf(v, e.attr("scheme")+ref["identifier-type"])
			}
		case "subject":
			m.Subjects = append(m.Subjects, v)
		case "rights":
			m.Rights = v
		case "meta":
			opfExtra(&m, e, refines)
		}
	}
	return m, nil
}

// opfExtra reads series information: EPUB 3 collections and the widely used calibre:series meta.
func opfExtra(m *Meta, e xmlElem, refines map[string]map[string]string) {
	switch {
	case e.attr("property") == "belongs-to-collection" && e.attr("refines") == "":
		m.Series = strings.TrimSpace(e.Value)
		m.SeriesNo = refines[e.attr("id")]["group-position"]
	case e.attr("name") == "calibre:series":
		m.Series = e.attr("content")
	case e.attr("name") == "calibre:series_index":
		m.SeriesNo = strings.TrimSuffix(e.attr("content"), ".0")
	}
}

// DOCX (OOXML core properties) -------------------------------------------------------------

func docxMeta(file string) (Meta, error) {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return Meta{}, err
	}
	defer zr.Close()
	b, err := zipEntry(&zr.Reader, "docProps/core.xml")
	if err != nil {
		return Meta{}, nil // core properties are optional
	}
	var c struct {
		Title    string `xml:"title"`
		Subject  string `xml:"subject"`
		Creator  string `xml:"creator"`
		Keywords string `xml:"keywords"`
		Language string `xml:"language"`
		Created  string `xml:"created"`
	}
	if err := xml.Unmarshal(b, &c); err != nil {
		return Meta{}, fmt.Errorf("docx core properties: %w", err)
	}
	m := Meta{Title: c.Title, Language: c.Language, Year: yearOf(c.Created)}
	for _, name := range strings.Split(c.Creator, ";") {
		m.People = append(m.People, Person{Name: name})
	}
	m.Subjects = append(splitList(c.Subject), splitList(c.Keywords)...)
	return m, nil
}

// Text formats ----------------------------------------------------------------------------

const headSize = 64 << 10

// withHead passes the start of a text file, without a byte order mark, to fn.
func withHead(file string, fn func([]byte) Meta) (Meta, error) {
	f, err := os.Open(file)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, headSize))
	if err != nil {
		return Meta{}, err
	}
	return fn(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))), nil
}

var (
	titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	metaRe  = regexp.MustCompile(`(?is)<meta\s[^>]*>`)
	attrRe  = regexp.MustCompile(`(?is)([a-z][\w.:-]*)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	langRe  = regexp.MustCompile(`(?is)<html[^>]*?\blang\s*=\s*["']?([\w-]+)`)
	tagRe   = regexp.MustCompile(`<[^>]*>`)
)

func htmlMeta(b []byte) Meta {
	var m Meta
	if t := titleRe.FindSubmatch(b); t != nil {
		m.Title = html.UnescapeString(tagRe.ReplaceAllString(string(t[1]), ""))
	}
	if l := langRe.FindSubmatch(b); l != nil {
		m.Language = string(l[1])
	}
	for _, tag := range metaRe.FindAll(b, -1) {
		attrs := map[string]string{}
		for _, a := range attrRe.FindAllSubmatch(tag, -1) {
			attrs[strings.ToLower(string(a[1]))] = html.UnescapeString(strings.Trim(string(a[2]), `"'`))
		}
		content := attrs["content"]
		switch strings.ToLower(attrs["name"]) {
		case "author", "dc.creator", "dcterms.creator":
			m.People = append(m.People, Person{Name: content})
		case "keywords", "dc.subject", "dcterms.subject":
			m.Subjects = append(m.Subjects, splitList(content)...)
		case "dc.rights", "dcterms.rights":
			m.Rights = content
		case "dc.date", "dcterms.date", "dcterms.issued":
			m.Year = yearOf(content)
		case "dc.publisher", "dcterms.publisher":
			m.Publisher = content
		}
	}
	return m
}

// mdMeta reads YAML front matter (title, subtitle, author, date, lang) or else the first heading.
func mdMeta(b []byte) Meta {
	var m Meta
	sc := bufio.NewScanner(bytes.NewReader(b))
	first, front := true, false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case first && line == "---":
			front = true
		case front && (line == "---" || line == "..."):
			front = false
		case front:
			k, v, ok := strings.Cut(line, ":")
			if ok {
				frontField(&m, strings.ToLower(strings.TrimSpace(k)), strings.Trim(strings.TrimSpace(v), `"'`))
			}
		case m.Title == "" && strings.HasPrefix(line, "# "):
			m.Title = strings.TrimSpace(strings.Trim(line[2:], "#"))
		}
		first = false
		if !front && m.Title != "" {
			break
		}
	}
	return m
}

func frontField(m *Meta, k, v string) {
	switch k {
	case "title":
		m.Title = v
	case "subtitle":
		m.Subtitle = v
	case "author", "authors":
		for _, n := range splitList(strings.Trim(v, "[]")) {
			m.People = append(m.People, Person{Name: strings.Trim(strings.TrimSpace(n), `"'`)})
		}
	case "date", "year":
		m.Year = yearOf(v)
	case "lang", "language":
		m.Language = v
	case "rights", "license", "licence":
		m.Rights = v
	}
}

// txtMeta reads "Title:", "Author:" and "Language:" header lines, as plain-text ebooks often have.
func txtMeta(b []byte) Meta {
	var m Meta
	sc := bufio.NewScanner(bytes.NewReader(b))
	for n := 0; sc.Scan() && n < 200; n++ {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(strings.ToLower(k)) {
		case "title":
			if m.Title == "" {
				m.Title = v
			}
		case "author":
			m.People = append(m.People, Person{Name: v})
		case "language":
			m.Language = v
		}
	}
	return m
}
