package reader

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"net/url"
	"path"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"

	"github.com/Perlsowisdom/xegesis/internal/search"
)

// Sanitised HTML keeps a fixed set of text elements and attributes; everything else is dropped,
// keeping its text. Nothing that can run code or fetch a resource survives: no scripts, styles,
// forms, frames, objects or remote URLs. Each paragraph starts with an empty marker,
// <span class="bp" id="bp-N" data-p="N">, numbered as search numbers paragraphs, so a search
// locator's Para and a highlight's position name the same place. Bible references in the text
// become links (<a class="vref" data-ref="OSIS">).

var keepTags = map[string]bool{
	"p": true, "div": true, "span": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"em": true, "i": true, "strong": true, "b": true, "u": true, "s": true, "sub": true, "sup": true,
	"small": true, "br": true, "hr": true, "blockquote": true, "pre": true, "code": true, "ul": true,
	"ol": true, "li": true, "dl": true, "dt": true, "dd": true, "table": true, "thead": true, "tbody": true,
	"tfoot": true, "tr": true, "td": true, "th": true, "caption": true, "figure": true, "figcaption": true,
	"img": true, "a": true, "section": true, "article": true, "aside": true, "header": true, "footer": true,
	"nav": true, "abbr": true, "cite": true, "q": true, "mark": true, "ruby": true, "rt": true, "rp": true,
	"bdi": true, "bdo": true, "del": true, "ins": true, "var": true, "kbd": true, "samp": true, "dfn": true,
	"main": true, "address": true, "wbr": true,
}

var voidTags = map[string]bool{"br": true, "hr": true, "img": true, "wbr": true}

// Attributes kept as they are; id, href and src are rewritten.
var keepAttrs = map[string]bool{"lang": true, "dir": true, "title": true, "alt": true, "colspan": true,
	"rowspan": true, "start": true, "reversed": true, "abbr": true, "scope": true}

var imageTypes = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml", ".bmp": "image/bmp", ".avif": "image/avif"}

const maxImage = 16 << 20

var idChars = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// bookID turns an id or fragment from a book into one that cannot clash with the interface's own.
func bookID(id string) string {
	if u, err := url.PathUnescape(id); err == nil {
		id = u
	}
	return "bk-" + idChars.ReplaceAllString(id, "_")
}

// resources resolves what a document refers to: images inside the book and links to its sections.
type resources interface {
	image(src string) (mediaType string, data []byte, ok bool)
	link(href string) (section int, anchor string, ok bool)
}

// rendered is a sanitised section.
type rendered struct {
	html    string
	paras   int    // paragraphs numbered
	heading string // the first heading's text
}

type sanitizer struct {
	res     resources
	out     strings.Builder
	open    []string // elements written and not yet closed
	skip    int
	para    int
	started bool // the current paragraph has text, so it has its marker
	heading bool
	first   strings.Builder
	links   int // open <a> elements
}

// sanitize renders an HTML or XHTML document's body.
func sanitize(doc []byte, res resources) rendered {
	s := &sanitizer{res: res}
	z := xhtml.NewTokenizer(bytes.NewReader(doc))
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			break
		}
		t := z.Token()
		switch tt {
		case xhtml.TextToken:
			if s.skip == 0 {
				s.text(t.Data)
			}
		case xhtml.StartTagToken, xhtml.EndTagToken, xhtml.SelfClosingTagToken:
			s.tag(tt, t)
		}
	}
	for i := len(s.open) - 1; i >= 0; i-- {
		s.out.WriteString("</" + s.open[i] + ">")
	}
	return rendered{html: s.out.String(), paras: s.para, heading: strings.Join(strings.Fields(s.first.String()), " ")}
}

// tag mirrors search's paragraph rules (search.htmlBlocks) and writes the element if it is kept.
func (s *sanitizer) tag(tt xhtml.TokenType, t xhtml.Token) {
	tag := strings.ToLower(t.Data)
	switch {
	case search.IsSkipTag(tag) && tt == xhtml.StartTagToken:
		s.skip++
		return
	case search.IsSkipTag(tag) && tt == xhtml.EndTagToken:
		s.skip = max(s.skip-1, 0)
		return
	case search.IsHeadingTag(tag):
		s.flush()
		s.heading = tt == xhtml.StartTagToken
	case search.IsBlockTag(tag):
		s.flush()
	}
	if s.skip > 0 || !keepTags[tag] {
		return
	}
	if tt == xhtml.EndTagToken {
		s.close(tag)
		return
	}
	attrs, ok := s.attrs(tag, t)
	if !ok {
		return
	}
	s.out.WriteString("<" + tag + attrs + ">")
	if !voidTags[tag] {
		if tt == xhtml.SelfClosingTagToken {
			s.out.WriteString("</" + tag + ">")
		} else {
			s.open = append(s.open, tag)
			if tag == "a" {
				s.links++
			}
		}
	}
}

// close ends the innermost open element named tag, and any opened inside it.
func (s *sanitizer) close(tag string) {
	for i := len(s.open) - 1; i >= 0; i-- {
		if s.open[i] != tag {
			continue
		}
		for j := len(s.open) - 1; j >= i; j-- {
			s.out.WriteString("</" + s.open[j] + ">")
			if s.open[j] == "a" {
				s.links--
			}
		}
		s.open = s.open[:i]
		return
	}
}

func (s *sanitizer) flush() { s.started = false }

func (s *sanitizer) attrs(tag string, t xhtml.Token) (string, bool) {
	var b strings.Builder
	put := func(k, v string) { fmt.Fprintf(&b, ` %s="%s"`, k, html.EscapeString(v)) }
	for _, a := range t.Attr {
		k := strings.ToLower(a.Key)
		switch {
		case k == "id":
			put("id", bookID(a.Val))
		case k == "lang" || k == "xml:lang" || a.Namespace == "xml" && k == "lang":
			put("lang", a.Val)
		case keepAttrs[k]:
			put(k, a.Val)
		}
	}
	switch tag {
	case "img":
		src := attrOf(t, "src")
		media, data, ok := s.res.image(src)
		if !ok {
			return "", false
		}
		put("src", "data:"+media+";base64,"+base64.StdEncoding.EncodeToString(data))
	case "a":
		href := attrOf(t, "href")
		switch {
		case href == "":
		case hasScheme(href):
			put("class", "external")
			if l := strings.ToLower(href); strings.HasPrefix(l, "http:") || strings.HasPrefix(l, "https:") || strings.HasPrefix(l, "mailto:") {
				put("data-url", href)
			}
		default:
			if sec, anchor, ok := s.res.link(href); ok {
				put("href", "#")
				put("data-section", fmt.Sprint(sec))
				if anchor != "" {
					put("data-anchor", anchor)
				}
			}
		}
	}
	return b.String(), true
}

// text writes a run of text: the paragraph's marker first, then the text with Bible references
// linked.
func (s *sanitizer) text(t string) {
	if !s.started && strings.TrimSpace(t) != "" {
		s.para++
		s.started = true
		fmt.Fprintf(&s.out, `<span class="bp" id="bp-%d" data-p="%d"></span>`, s.para, s.para)
	}
	if s.heading && s.first.Len() < 200 && s.para > 0 {
		s.first.WriteString(t)
	}
	if s.links > 0 {
		s.out.WriteString(html.EscapeString(t))
		return
	}
	s.out.WriteString(linkRefs(t))
}

// linkRefs escapes text and wraps each Bible reference in a link to it.
func linkRefs(t string) string {
	var b strings.Builder
	pos := 0
	for _, c := range search.FindRefs(t) {
		if c.Start < pos {
			continue // a further reference in a list already linked
		}
		b.WriteString(html.EscapeString(t[pos:c.Start]))
		fmt.Fprintf(&b, `<a href="#" class="vref" data-ref="%s">%s</a>`, osis(c), html.EscapeString(t[c.Start:c.End]))
		pos = c.End
	}
	b.WriteString(html.EscapeString(t[pos:]))
	return b.String()
}

func osis(c search.Citation) string {
	o := c.Range.Start.OSIS()
	if c.Range.End != c.Range.Start {
		o += "-" + c.Range.End.OSIS()
	}
	return o
}

// blocksHTML renders extracted paragraphs and headings (DOCX, Markdown and plain text).
func blocksHTML(blocks []search.Block) rendered {
	var b strings.Builder
	r := rendered{paras: len(blocks)}
	for i, bl := range blocks {
		tag := "p"
		if bl.Heading {
			tag = "h2"
			if r.heading == "" {
				r.heading = bl.Text
			}
		}
		fmt.Fprintf(&b, `<%s><span class="bp" id="bp-%d" data-p="%d"></span>%s</%s>`, tag, i+1, i+1, linkRefs(bl.Text), tag)
	}
	r.html = b.String()
	return r
}

// epubRes resolves the images and links of one EPUB chapter.
type epubRes struct {
	e    *epub
	base string // the chapter's path in the zip
}

func (r epubRes) image(src string) (string, []byte, bool) {
	if src == "" || hasScheme(src) {
		return dataImage(src)
	}
	p := resolve(r.base, src)
	media, ok := imageTypes[strings.ToLower(path.Ext(p))]
	if !ok {
		return "", nil, false
	}
	f := r.e.files[strings.ToLower(p)]
	if f == nil || f.UncompressedSize64 > maxImage {
		return "", nil, false
	}
	b, err := r.e.read(p)
	return media, b, err == nil
}

func (r epubRes) link(href string) (int, string, bool) {
	file, frag, _ := strings.Cut(href, "#")
	sec := 0
	if file == "" {
		sec = r.e.section(r.base)
	} else {
		sec = r.e.section(resolve(r.base, file))
	}
	if sec == 0 {
		return 0, "", false
	}
	if frag != "" {
		frag = bookID(frag)
	}
	return sec, frag, true
}

// docRes serves a single-file document: only images already inside it as data URIs, and links
// within it.
type docRes struct{}

func (docRes) image(src string) (string, []byte, bool) { return dataImage(src) }

func (docRes) link(href string) (int, string, bool) {
	if !strings.HasPrefix(href, "#") || len(href) < 2 {
		return 0, "", false
	}
	return 1, bookID(href[1:]), true
}

// dataImage accepts an image embedded in the document as a base64 data URI.
func dataImage(src string) (string, []byte, bool) {
	rest, ok := strings.CutPrefix(src, "data:")
	if !ok {
		return "", nil, false
	}
	meta, payload, ok := strings.Cut(rest, ",")
	media, enc, _ := strings.Cut(meta, ";")
	known := false
	for _, m := range imageTypes {
		known = known || strings.EqualFold(m, media)
	}
	if !ok || !known || enc != "base64" || len(payload) > maxImage*4/3 {
		return "", nil, false
	}
	b, err := base64.StdEncoding.DecodeString(payload)
	return strings.ToLower(media), b, err == nil
}
