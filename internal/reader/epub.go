package reader

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html"
)

// EPUB 3.3: META-INF/container.xml names the package document. Its manifest lists the resources,
// its spine gives the reading order, and the navigation document (EPUB 3, the manifest item with
// the "nav" property) or the NCX (EPUB 2, named by the spine's toc attribute) holds the table of
// contents.

type opfPackage struct {
	Manifest []struct {
		ID         string `xml:"id,attr"`
		Href       string `xml:"href,attr"`
		MediaType  string `xml:"media-type,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Spine struct {
		Toc   string `xml:"toc,attr"`
		Items []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

// epub is an open EPUB file.
type epub struct {
	zr    *zip.ReadCloser
	files map[string]*zip.File // by lower-case path
	spine []spineItem
	nav   string // path of the EPUB 3 navigation document
	ncx   string // path of the EPUB 2 NCX
}

type spineItem struct {
	idref, href, path string
	html              bool
}

const maxEntry = 64 << 20 // the largest zip entry read, in bytes

func openEPUB(file string) (*epub, error) {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return nil, err
	}
	e := &epub{zr: zr, files: map[string]*zip.File{}}
	for _, f := range zr.File {
		e.files[strings.ToLower(f.Name)] = f
	}
	if err := e.load(); err != nil {
		zr.Close()
		return nil, err
	}
	return e, nil
}

func (e *epub) Close() error { return e.zr.Close() }

func (e *epub) load() error {
	var c struct {
		Rootfiles []struct {
			Path string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := e.readXML("META-INF/container.xml", &c); err != nil {
		return err
	}
	if len(c.Rootfiles) == 0 {
		return fmt.Errorf("epub: container.xml names no package document")
	}
	opf := c.Rootfiles[0].Path
	var pkg opfPackage
	if err := e.readXML(opf, &pkg); err != nil {
		return err
	}
	type entry struct{ href, media string }
	byID := map[string]entry{}
	for _, it := range pkg.Manifest {
		byID[it.ID] = entry{it.Href, it.MediaType}
		if strings.Contains(" "+it.Properties+" ", " nav ") {
			e.nav = resolve(opf, it.Href)
		}
	}
	if t, ok := byID[pkg.Spine.Toc]; ok {
		e.ncx = resolve(opf, t.href)
	}
	for _, ref := range pkg.Spine.Items {
		m := byID[ref.IDRef]
		e.spine = append(e.spine, spineItem{idref: ref.IDRef, href: m.href, path: resolve(opf, m.href),
			html: m.href != "" && strings.Contains(m.media, "html")})
	}
	return nil
}

// resolve turns an href relative to the document at base into a path inside the zip, without its
// fragment.
func resolve(base, href string) string {
	href, _, _ = strings.Cut(href, "#")
	if u, err := url.PathUnescape(href); err == nil {
		href = u
	}
	if strings.HasPrefix(href, "/") {
		return strings.TrimPrefix(path.Clean(href), "/")
	}
	return strings.TrimPrefix(path.Join(path.Dir(base), href), "/")
}

func (e *epub) read(name string) ([]byte, error) {
	f := e.files[strings.ToLower(name)]
	if f == nil {
		return nil, fmt.Errorf("epub: missing %s", name)
	}
	if f.UncompressedSize64 > maxEntry {
		return nil, fmt.Errorf("epub: %s is too large", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxEntry))
}

func (e *epub) readXML(name string, v any) error {
	b, err := e.read(name)
	if err != nil {
		return err
	}
	if err := xml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("epub: %s: %w", name, err)
	}
	return nil
}

// section returns the 1-based spine position of a zip path, or 0.
func (e *epub) section(p string) int {
	for i, it := range e.spine {
		if strings.EqualFold(it.path, p) {
			return i + 1
		}
	}
	return 0
}

// cfi is the canonical fragment identifier of a spine item, as search locators give it.
func (e *epub) cfi(section int) string {
	return fmt.Sprintf("epubcfi(/6/%d[%s]!)", 2*section, e.spine[section-1].idref)
}

// TOCEntry is one line of a table of contents.
type TOCEntry struct {
	Title   string `json:"title"`
	Section int    `json:"section" desc:"Section (EPUB chapter) the entry opens"`
	Anchor  string `json:"anchor,omitempty" desc:"Element ID within the section's HTML to scroll to"`
	Level   int    `json:"level" desc:"1 for top-level entries"`
}

// toc reads the navigation document or NCX. It returns nil when the book has neither.
func (e *epub) toc() []TOCEntry {
	if e.nav != "" {
		if b, err := e.read(e.nav); err == nil {
			if out := e.navTOC(b); len(out) > 0 {
				return out
			}
		}
	}
	if e.ncx != "" {
		if b, err := e.read(e.ncx); err == nil {
			return e.ncxTOC(b)
		}
	}
	return nil
}

func (e *epub) entry(base, href, title string, level int) (TOCEntry, bool) {
	title = strings.Join(strings.Fields(title), " ")
	if title == "" || href == "" || hasScheme(href) {
		return TOCEntry{}, false
	}
	sec := e.section(resolve(base, href))
	if sec == 0 {
		return TOCEntry{}, false
	}
	t := TOCEntry{Title: title, Section: sec, Level: max(level, 1)}
	if _, frag, ok := strings.Cut(href, "#"); ok && frag != "" {
		t.Anchor = bookID(frag)
	}
	return t, true
}

// navTOC reads the links in the navigation document's toc nav; the depth of nested lists is the
// entry's level.
func (e *epub) navTOC(b []byte) []TOCEntry {
	z := html.NewTokenizer(bytes.NewReader(b))
	var out []TOCEntry
	inTOC, navDepth, depth := false, 0, 0
	href, inLink := "", false
	var text strings.Builder
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return out
		}
		t := z.Token()
		switch tt {
		case html.StartTagToken:
			switch t.Data {
			case "nav":
				if inTOC {
					navDepth++
				} else if strings.Contains(" "+attrOf(t, "epub:type")+" "+attrOf(t, "type")+" ", " toc ") {
					inTOC, navDepth = true, 1
				}
			case "ol", "ul":
				if inTOC {
					depth++
				}
			case "a":
				if inTOC {
					href, inLink = attrOf(t, "href"), true
					text.Reset()
				}
			}
		case html.EndTagToken:
			switch t.Data {
			case "nav":
				if inTOC {
					if navDepth--; navDepth == 0 {
						return out
					}
				}
			case "ol", "ul":
				if inTOC {
					depth--
				}
			case "a":
				if inLink {
					if en, ok := e.entry(e.nav, href, text.String(), depth); ok {
						out = append(out, en)
					}
					inLink = false
				}
			}
		case html.TextToken:
			if inLink {
				text.WriteString(t.Data)
			}
		}
	}
}

type navPoint struct {
	Label   string `xml:"navLabel>text"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Children []navPoint `xml:"navPoint"`
}

func (e *epub) ncxTOC(b []byte) []TOCEntry {
	var doc struct {
		Points []navPoint `xml:"navMap>navPoint"`
	}
	if xml.Unmarshal(b, &doc) != nil {
		return nil
	}
	var out []TOCEntry
	var walk func(ps []navPoint, level int)
	walk = func(ps []navPoint, level int) {
		for _, p := range ps {
			if en, ok := e.entry(e.ncx, p.Content.Src, p.Label, level); ok {
				out = append(out, en)
			}
			walk(p.Children, level+1)
		}
	}
	walk(doc.Points, 1)
	return out
}

func attrOf(t html.Token, key string) string {
	for _, a := range t.Attr {
		k := a.Key
		if a.Namespace != "" {
			k = a.Namespace + ":" + k
		}
		if k == key {
			return a.Val
		}
	}
	return ""
}

func hasScheme(href string) bool {
	if strings.HasPrefix(href, "//") {
		return true
	}
	i := strings.IndexAny(href, ":/?#")
	return i > 0 && href[i] == ':'
}
