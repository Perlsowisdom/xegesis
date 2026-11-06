package reader

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/command/core"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/search"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Fixtures ---------------------------------------------------------------------------------------

func zipBytes(t *testing.T, entries [][2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e[0])
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e[1]))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A 1x1 PNG.
var png, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")

const chapter1 = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>One</title>
<link rel="stylesheet" href="https://example.com/x.css"/><style>p{color:red}</style><script>alert(1)</script></head>
<body onload="evil()"><h1 id="start">Chapter One</h1>
<p style="background:url(https://example.com/t.png)" onclick="evil()">The righteous shall live by faith, as Paul writes in Rom 1:17.</p>
<p>Second <em>paragraph</em> with a picture <img src="../images/dot.png" alt="dot"/> and a remote one
<img src="https://example.com/track.png"/> and <a href="https://example.com/">a web link</a>.</p>
<iframe src="https://example.com/frame"></iframe><svg><text>svg text</text></svg>
<div>See <a href="ch2.xhtml#grace">the next chapter</a>; <a href="javascript:evil()">bad</a>.</div>
<p>&#160;</p><p>Last <b>words</b> of chapter one.</p></body></html>`

const chapter2 = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><h2>Chapter Two</h2>
<p id="grace">All things work together for good (Romans 8:28&#8211;30).</p>
<p>Grace upon grace, a long paragraph about sanctification and mercy.</p></body></html>`

func epubBytes(t *testing.T) []byte {
	return zipBytes(t, [][2]string{
		{"mimetype", "application/epub+zip"},
		{"META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`},
		{"OEBPS/content.opf", `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:1</dc:identifier><dc:title>Lectures on Romans</dc:title>
    <dc:creator>Martin Luther</dc:creator>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="c1" href="text/ch1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="text/ch2.xhtml" media-type="application/xhtml+xml"/>
    <item id="dot" href="images/dot.png" media-type="image/png"/>
  </manifest>
  <spine><itemref idref="c1"/><itemref idref="c2"/></spine>
</package>`},
		{"OEBPS/nav.xhtml", `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body>
<nav epub:type="toc"><ol><li><a href="text/ch1.xhtml">Chapter One</a>
<ol><li><a href="text/ch2.xhtml#grace">On grace</a></li></ol></li></ol></nav>
<nav epub:type="landmarks"><ol><li><a href="text/ch1.xhtml">Start</a></li></ol></nav></body></html>`},
		{"OEBPS/text/ch1.xhtml", chapter1},
		{"OEBPS/text/ch2.xhtml", chapter2},
		{"OEBPS/images/dot.png", string(png)},
	})
}

func docxBytes(t *testing.T) []byte {
	return zipBytes(t, [][2]string{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`},
		{"docProps/core.xml", `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
  xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Sermon on Hope</dc:title><dc:creator>Jane Doe</dc:creator></cp:coreProperties>`},
		{"word/document.xml", `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Hope in Suffering</w:t></w:r></w:p>
<w:p><w:r><w:t xml:space="preserve">We know that suffering produces endurance.</w:t></w:r></w:p>
<w:p><w:r><w:t>Compare John 3:16 and &lt;script&gt; text.</w:t></w:r></w:p>
</w:body></w:document>`},
	})
}

func pdfBytes(pages ...string) []byte {
	var b bytes.Buffer
	offsets := []int{}
	obj := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	b.WriteString("%PDF-1.4\n")
	kids := ""
	for i := range pages {
		kids += fmt.Sprintf("%d 0 R ", 4+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>", kids, len(pages)))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	for i, content := range pages {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", 5+2*i))
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return b.Bytes()
}

var testPDF = pdfBytes(
	"BT /F1 12 Tf 72 720 Td (Justification by faith alone.) Tj ET",
	"BT /F1 12 Tf 72 720 Td (Page two speaks of propitiation and love.) Tj ET",
)

func writeBooks(t *testing.T, dir string) {
	for name, body := range map[string][]byte{
		"luther.epub":    epubBytes(t),
		"sermon.docx":    docxBytes(t),
		"commentary.pdf": testPDF,
		"page.html": []byte(`<html><head><title>Grace</title><script>var x = "hidden";</script></head>
<body><h1>Amazing Grace</h1><p onclick="x()">Grace that is greater than all our sin, John 1:16.</p>
<img src="file:///C:/Windows/win.ini"/><img src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(png) + `"/>
<a href="#top">Top</a></body></html>`),
		"notes.md": []byte("# Pilgrim\n\nThe **pilgrim** walks the narrow way.\n"),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Environment ------------------------------------------------------------------------------------

type env struct {
	t      *testing.T
	data   string
	books  string
	st     *store.Store
	reg    *command.Registry
	svc    *service
	env    *command.Env
	items  map[string]string // item ID by format
	titles map[string]string // item ID by title
}

// setup opens a library with one folder of books, scanned and indexed. Index files stay open
// until the process ends, so the data folder is removed on a best-effort basis.
func setup(t *testing.T) *env { return setupWith(t, nil) }

// setupWith adds extra files (name to bytes) to the library; e.titles finds their items by title.
func setupWith(t *testing.T, extra map[string][]byte) *env {
	t.Helper()
	data, err := os.MkdirTemp("", "xegesis-reader-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(data) })
	e := &env{t: t, data: data, books: filepath.Join(data, "books"), items: map[string]string{}}
	if err := os.MkdirAll(e.books, 0o755); err != nil {
		t.Fatal(err)
	}
	writeBooks(t, e.books)
	for name, body := range extra {
		if err := os.WriteFile(filepath.Join(e.books, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e.titles = map[string]string{}
	e.openStore()
	var f library.Folder
	e.must("library.add_folder", map[string]any{"path": e.books}, &f)
	e.must("library.scan", map[string]any{"folder_id": f.ID}, nil)
	e.must("search.index", map[string]any{"folder_id": f.ID, "wait": true}, nil)
	var items struct {
		Items []library.ItemSummary `json:"items"`
	}
	e.must("library.items", map[string]any{}, &items)
	for _, it := range items.Items {
		var full library.Item
		e.must("library.item", map[string]any{"id": it.ID}, &full)
		e.items[full.Files[0].Format] = it.ID
		e.titles[full.Title] = it.ID
	}
	return e
}

// openStore opens library.db and a fresh registry, as when the app starts.
func (e *env) openStore() {
	st, err := store.Open(filepath.Join(e.data, "library.db"))
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { st.Close() })
	e.st = st
	e.env = &command.Env{Store: st}
	e.reg = command.NewRegistry()
	core.Register(e.reg)
	library.Register(e.reg)
	search.Register(e.reg, e.data)
	e.svc = newService(e.reg)
	register(e.reg, e.svc)
}

func (e *env) run(id string, params, out any) error {
	e.t.Helper()
	raw, _ := json.Marshal(params)
	res, err := e.reg.Run(context.Background(), e.env, id, raw)
	if err != nil {
		return err
	}
	if out != nil {
		reflect.ValueOf(out).Elem().SetZero()
		b, _ := json.Marshal(res)
		if err := json.Unmarshal(b, out); err != nil {
			e.t.Fatal(err)
		}
	}
	return nil
}

func (e *env) must(id string, params, out any) {
	e.t.Helper()
	if err := e.run(id, params, out); err != nil {
		e.t.Fatalf("%s: %v", id, err)
	}
}

func (e *env) fails(id string, params any, code string) {
	e.t.Helper()
	err := e.run(id, params, nil)
	ce, ok := err.(*command.Error)
	if !ok || ce.Code != code {
		e.t.Fatalf("%s(%v): got %v, want %s", id, params, err, code)
	}
}

// paraTexts reads sanitised HTML back as the text of each numbered paragraph.
func paraTexts(t *testing.T, doc string) []string {
	t.Helper()
	z := xhtml.NewTokenizer(strings.NewReader(doc))
	var out []string
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			break
		}
		tok := z.Token()
		switch tt {
		case xhtml.StartTagToken:
			if attrOf(tok, "class") == "bp" {
				if want := fmt.Sprint(len(out) + 1); attrOf(tok, "data-p") != want {
					t.Fatalf("marker %s out of order, want %s", attrOf(tok, "data-p"), want)
				}
				out = append(out, "")
			}
		case xhtml.TextToken:
			if len(out) > 0 {
				out[len(out)-1] += tok.Data
			}
		}
	}
	for i := range out {
		out[i] = strings.Join(strings.Fields(out[i]), " ")
	}
	return out
}
