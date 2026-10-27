package search

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

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
    <dc:identifier id="uid">urn:uuid:1</dc:identifier>
    <dc:title>Lectures on Romans</dc:title>
    <dc:creator>Martin Luther</dc:creator>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="c1" href="text/ch%201.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="text/ch2.xhtml" media-type="application/xhtml+xml"/>
    <item id="css" href="style.css" media-type="text/css"/>
  </manifest>
  <spine><itemref idref="c1"/><itemref idref="c2"/></spine>
</package>`},
		{"OEBPS/text/ch 1.xhtml", `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>One</title><style>p{color:red}</style></head>
<body><h1>Chapter One</h1><p>The righteous shall live by faith, as Paul writes in Rom 1:17.</p>
<p>Second <em>paragraph</em> of the first chapter.</p></body></html>`},
		{"OEBPS/text/ch2.xhtml", `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body><h2>Chapter Two</h2>
<p>All things work together for good (Romans 8:28&#8211;30).</p></body></html>`},
	})
}

func docxBytes(t *testing.T) []byte {
	return zipBytes(t, [][2]string{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`},
		{"docProps/core.xml", `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
  xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Sermon on Hope</dc:title><dc:creator>Jane Doe</dc:creator></cp:coreProperties>`},
		{"word/document.xml", `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Hope in Suffering</w:t></w:r></w:p>
<w:p><w:r><w:t xml:space="preserve">We know that </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>suffering produces</w:t></w:r><w:r><w:t xml:space="preserve"> endurance.</w:t></w:r></w:p>
<w:p><w:r><w:t>Compare John 3:16; 4:2 and Ps 23:1, 4.</w:t></w:r></w:p>
</w:body></w:document>`},
	})
}

// pdfBytes builds a PDF with one page per content stream, the odd pages Flate-compressed.
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
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, len(pages)))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	for i, content := range pages {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", 5+2*i))
		if i%2 == 1 {
			var z bytes.Buffer
			w := zlib.NewWriter(&z)
			w.Write([]byte(content))
			w.Close()
			obj(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", z.Len(), z.String()))
		} else {
			obj(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
		}
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
	"BT /F1 12 Tf 72 720 Td (Justification by faith alone) Tj 0 -14 Td (continues on this line.) Tj "+
		"0 -40 Td (A new paragraph about grace.) Tj ET",
	"BT /F1 12 Tf 72 720 Td (Page two speaks of 1 Cor. 13:4 and love.) Tj ET",
)

// books writes one book of each format into dir.
func books(t *testing.T, dir string) {
	writeFile(t, filepath.Join(dir, "luther.epub"), epubBytes(t))
	writeFile(t, filepath.Join(dir, "sermon.docx"), docxBytes(t))
	writeFile(t, filepath.Join(dir, "commentary.pdf"), testPDF)
	writeFile(t, filepath.Join(dir, "page.html"), []byte(`<html><head><title>Grace</title><script>var x = "hidden";</script></head>
<body><h1>Amazing Grace</h1><p>Grace that is greater than all our sin.</p><div>Saved by grace through faith.</div></body></html>`))
	writeFile(t, filepath.Join(dir, "notes.md"), []byte("---\ntitle: Study Notes\nauthor: John Bunyan\n---\n\n# Pilgrim\n\n"+
		"The **pilgrim** walks the narrow way, see [Matthew 7:14](http://x).\n\n## Burden\n\nHis burden falls at the cross.\n"))
	writeFile(t, filepath.Join(dir, "plain.txt"), []byte("Title: Plain Text\nAuthor: Anon\n\nFirst paragraph\nwraps here.\n\nSecond paragraph about mercy.\n"))
}
