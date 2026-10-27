package search

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// Elements whose start or end begins a new paragraph.
var blockTags = map[string]bool{
	"p": true, "div": true, "li": true, "blockquote": true, "pre": true, "tr": true, "td": true,
	"th": true, "dd": true, "dt": true, "section": true, "article": true, "aside": true,
	"header": true, "footer": true, "figcaption": true, "table": true, "ul": true, "ol": true,
	"dl": true, "br": true, "hr": true, "body": true,
}

var headingTags = map[string]bool{"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true}

// Elements whose content is not text to read.
var skipTags = map[string]bool{"head": true, "script": true, "style": true, "noscript": true, "template": true, "svg": true}

// htmlBlocks reads the visible text of an HTML or XHTML document as paragraphs and headings.
func htmlBlocks(b []byte) []block {
	z := html.NewTokenizer(bytes.NewReader(b))
	var out []block
	var buf strings.Builder
	skip, inHeading := 0, false
	flush := func() {
		if t := clean(buf.String()); t != "" {
			out = append(out, block{text: t, heading: inHeading})
		}
		buf.Reset()
	}
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			flush()
			return out
		case html.TextToken:
			if skip == 0 {
				buf.Write(z.Text())
			}
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := strings.ToLower(string(name))
			switch {
			case skipTags[tag] && tt == html.StartTagToken:
				skip++
			case skipTags[tag] && tt == html.EndTagToken:
				skip = max(skip-1, 0)
			case headingTags[tag]:
				flush()
				inHeading = tt == html.StartTagToken
			case blockTags[tag]:
				flush()
			default:
				if tt != html.EndTagToken && (tag == "img" || tag == "wbr") {
					buf.WriteByte(' ')
				}
			}
		}
	}
}
