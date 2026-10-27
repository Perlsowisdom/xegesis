package search

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
)

// EPUB (EPUB 3.3, sections 3 and 5): META-INF/container.xml names the package document, whose
// manifest lists the content documents and whose spine gives their reading order.

type epubContainer struct {
	Rootfiles []struct {
		Path string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

type epubPackage struct {
	Manifest []struct {
		ID        string `xml:"id,attr"`
		Href      string `xml:"href,attr"`
		MediaType string `xml:"media-type,attr"`
	} `xml:"manifest>item"`
	Spine []struct {
		IDRef  string `xml:"idref,attr"`
		Linear string `xml:"linear,attr"`
	} `xml:"spine>itemref"`
}

// epubText returns one section per content document in spine order.
func epubText(file string) ([]section, error) {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var c epubContainer
	if err := readXML(&zr.Reader, "META-INF/container.xml", &c); err != nil {
		return nil, err
	}
	if len(c.Rootfiles) == 0 {
		return nil, fmt.Errorf("epub: container.xml names no package document")
	}
	opf := c.Rootfiles[0].Path
	var pkg epubPackage
	if err := readXML(&zr.Reader, opf, &pkg); err != nil {
		return nil, err
	}
	hrefs := map[string]string{}
	for _, it := range pkg.Manifest {
		if strings.Contains(it.MediaType, "html") {
			hrefs[it.ID] = it.Href
		}
	}
	var out []section
	for i, ref := range pkg.Spine {
		href, ok := hrefs[ref.IDRef]
		if !ok {
			continue
		}
		b, err := zipFile(&zr.Reader, resolve(opf, href))
		if err != nil {
			return nil, err
		}
		loc := Locator{Spine: i + 1, Href: href, CFI: fmt.Sprintf("epubcfi(/6/%d[%s]!)", 2*(i+1), ref.IDRef)}
		out = append(out, section{loc: loc, blocks: htmlBlocks(b)})
	}
	return out, nil
}

// resolve turns an href in the package document into a path inside the zip.
func resolve(opf, href string) string {
	if u, err := url.PathUnescape(href); err == nil {
		href = u
	}
	href, _, _ = strings.Cut(href, "#")
	return path.Join(path.Dir(opf), href)
}

func zipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name || strings.EqualFold(f.Name, name) {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, 256<<20))
		}
	}
	return nil, fmt.Errorf("missing %s", name)
}

func readXML(zr *zip.Reader, name string, v any) error {
	b, err := zipFile(zr, name)
	if err != nil {
		return err
	}
	if err := xml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
