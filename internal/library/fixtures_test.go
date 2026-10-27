package library

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const containerXML = `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`

const epubOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:identifier id="uid">urn:uuid:0b7e6d3c</dc:identifier>
    <dc:identifier>urn:isbn:978-0-00-000000-2</dc:identifier>
    <dc:title id="t1">Institutes of the Christian Religion</dc:title>
    <dc:title id="t2">Volume One</dc:title>
    <meta refines="#t2" property="title-type">subtitle</meta>
    <dc:creator id="c1">John Calvin</dc:creator>
    <meta refines="#c1" property="role">aut</meta>
    <meta refines="#c1" property="file-as">Calvin, Jean</meta>
    <dc:contributor opf:role="trl">Henry Beveridge</dc:contributor>
    <dc:contributor opf:role="bkp">Some Tool</dc:contributor>
    <dc:language>en</dc:language>
    <dc:date>1845-01-01</dc:date>
    <dc:publisher>Calvin Translation Society</dc:publisher>
    <dc:subject>Theology</dc:subject>
    <dc:subject>Reformation</dc:subject>
    <dc:rights>Public domain in the USA.</dc:rights>
    <meta name="calibre:series" content="Institutes"/>
    <meta name="calibre:series_index" content="1.0"/>
  </metadata>
</package>`

func writeEPUB(t *testing.T, path, opf string) {
	writeZip(t, path, map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": containerXML,
		"OEBPS/content.opf":      opf,
	})
}

const coreXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
  xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:title>Sermon on Romans 8</dc:title>
  <dc:subject>Romans</dc:subject>
  <dc:creator>Jane Doe; Joe Bloggs</dc:creator>
  <cp:keywords>hope, suffering</cp:keywords>
  <dc:language>en-GB</dc:language>
  <dcterms:created xsi:type="dcterms:W3CDTF">2024-03-01T10:00:00Z</dcterms:created>
</cp:coreProperties>`

func writeDOCX(t *testing.T, path string) {
	writeZip(t, path, map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
		"docProps/core.xml":   coreXML,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"/>`,
	})
}

// pdfDoc is a minimal PDF whose Info dictionary has a literal title with escapes and a UTF-16 author.
const pdfDoc = "%PDF-1.4\n" +
	"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n" +
	"2 0 obj\n<< /Type /Pages /Kids [] /Count 0 >>\nendobj\n" +
	"3 0 obj\n<< /Title (Commentary on \\(Romans\\)) /Author <FEFF004A006F0068006E002000430061006C00760069006E>" +
	" /CreationDate (D:15400101000000) >>\nendobj\n" +
	"trailer\n<< /Size 4 /Root 1 0 R /Info 3 0 R >>\nstartxref\n0\n%%EOF\n"
