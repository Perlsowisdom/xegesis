package export_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/library"
)

// A PDF with two text pages, as the test PDF of the reader package.
func twoPagePDF() []byte {
	var b bytes.Buffer
	var offsets []int
	obj := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	b.WriteString("%PDF-1.4\n")
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj("<< /Type /Pages /Kids [4 0 R 6 0 R] /Count 2 /MediaBox [0 0 612 792] >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	for i, text := range []string{"Justification by faith alone.", "Page two speaks of love."} {
		content := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)
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

func TestPdfPageInkIsExported(t *testing.T) {
	e := setup(t)
	dir := filepath.Join(e.data, "pdfs")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "faith.pdf"), twoPagePDF(), 0o644)
	var f library.Folder
	e.must("library.add_folder", map[string]any{"path": dir}, &f)
	e.must("library.scan", map[string]any{"folder_id": f.ID}, nil)
	var items struct {
		Items []library.ItemSummary `json:"items"`
	}
	e.must("library.items", map[string]any{"text": "faith"}, &items)
	if len(items.Items) != 1 {
		t.Fatalf("items %+v", items.Items)
	}
	pdfID := items.Items[0].ID
	e.must("library.set_licence", map[string]any{"item_ids": []string{pdfID}, "type": "public_domain"}, nil)

	// Ink on page 2 only; its points are fractions of the page, 1.5 times as tall as it is wide.
	e.must("ink.page.set", map[string]any{"item_id": pdfID, "page": 2, "aspect": 1.5, "strokes": []map[string]any{
		{"t": "pen", "c": "blue", "w": 2, "p": [][3]float64{{0.1, 0.1, 0}, {0.5, 0.2, 0.6}, {0.9, 0.4, 0}}},
		{"t": "hl", "c": "yellow", "w": 1, "p": [][3]float64{{0.1, 0.5, 0}, {0.9, 0.5, 0}}},
	}}, nil)

	src := []map[string]any{{"kind": "book", "item_id": pdfID, "from": 1, "to": 2}}
	_, page := e.make("html", src, nil)
	h := string(page)
	for _, want := range []string{"Ink · page 2", `viewBox="0 0 1000 1500"`, "#4f7fbf", "#d9b43c", "Page two speaks of love"} {
		if !strings.Contains(h, want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
	if strings.Contains(h, "Ink · page 1") || strings.Count(h, "<svg") != 1 {
		t.Errorf("ink only on page 2, one drawing; got %d drawings", strings.Count(h, "<svg"))
	}

	_, md := e.make("md", src, nil)
	if !strings.Contains(string(md), "```ink\n{\"v\":2,\"h\":1500,") || !strings.Contains(string(md), "[500,300,0.6]") {
		t.Errorf("Markdown has no ink block in page units:\n%s", md)
	}

	_, withInk := e.make("pdf", src, nil)
	src[0]["no_ink"] = true
	_, without := e.make("pdf", src, nil)
	if !bytes.HasPrefix(withInk, []byte("%PDF")) || len(withInk) <= len(without) {
		t.Errorf("the PDF with ink (%d bytes) should hold more than the one without (%d)", len(withInk), len(without))
	}
	_, h2 := e.make("html", src, nil)
	if strings.Contains(string(h2), "<svg") {
		t.Errorf("no_ink still drew the ink")
	}

	// A text selection is not a page, so it carries no ink.
	_, sel := e.make("html", []map[string]any{{"kind": "book", "item_id": pdfID, "text": "Page two speaks of love."}}, nil)
	if strings.Contains(string(sel), "<svg") {
		t.Errorf("selected text carries ink")
	}
}
