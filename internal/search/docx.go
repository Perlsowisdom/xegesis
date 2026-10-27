package search

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

// DOCX (ECMA-376 Part 1, 17.3): word/document.xml holds paragraphs (w:p) of runs whose text is in
// w:t elements. A paragraph styled Title or HeadingN is a heading.

func docxText(file string) ([]section, error) {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	b, err := zipFile(&zr.Reader, "word/document.xml")
	if err != nil {
		return nil, err
	}
	blocks, err := docxBlocks(b)
	if err != nil {
		return nil, err
	}
	return []section{{blocks: blocks}}, nil
}

func docxBlocks(b []byte) ([]block, error) {
	d := xml.NewDecoder(bytes.NewReader(b))
	var out []block
	var buf strings.Builder
	inText, heading, depth := false, false, 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				if depth == 0 {
					buf.Reset()
					heading = false
				}
				depth++
			case "pStyle":
				v := strings.ToLower(attr(t, "val"))
				heading = heading || v == "title" || strings.HasPrefix(v, "heading")
			case "t":
				inText = true
			case "tab", "br", "cr":
				buf.WriteByte(' ')
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				if depth--; depth == 0 {
					if s := clean(buf.String()); s != "" {
						out = append(out, block{text: s, heading: heading})
					}
				}
			}
		case xml.CharData:
			if inText {
				buf.Write(t)
			}
		}
	}
}

func attr(e xml.StartElement, local string) string {
	for _, a := range e.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}
