package export

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// DOCX writes a Word document: a zip of WordprocessingML parts (ECMA-376), written directly. It
// has a title, headings, verses with superscript numbers, side-by-side tables, credits, and an
// optional header (the title) and footer (page numbers).
func DOCX(d Doc) ([]byte, error) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	links := &relList{}
	body := docxBody(d, links)
	parts := []struct{ name, data string }{
		{"[Content_Types].xml", contentTypes},
		{"_rels/.rels", rootRels},
		{"docProps/core.xml", coreXML(d.Title)},
		{"docProps/app.xml", appXML},
		{"word/styles.xml", stylesXML(d.Opt)},
		{"word/settings.xml", settingsXML},
		{"word/header1.xml", headerXML(d)},
		{"word/footer1.xml", footerXML(d.Opt)},
		{"word/_rels/document.xml.rels", links.xml()},
		{"word/document.xml", body},
	}
	for _, p := range parts {
		w, err := z.CreateHeader(&zip.FileHeader{Name: p.name, Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(p.data)); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const xmlHead = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
const wNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

const contentTypes = xmlHead + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
<Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>
<Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/>
<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>
<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`

const rootRels = xmlHead + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const appXML = xmlHead + `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>Xegesis</Application></Properties>`

const settingsXML = xmlHead + `<w:settings ` + wNS + `><w:defaultTabStop w:val="720"/><w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat></w:settings>`

func coreXML(title string) string {
	return xmlHead + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + xesc(title) + `</dc:title><dc:creator>Xegesis</dc:creator>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + time.Now().UTC().Format(time.RFC3339) + `</dcterms:created></cp:coreProperties>`
}

func stylesXML(o Options) string {
	sz := int(o.fontSize()*2 + 0.5)
	style := func(id, name, ppr, rpr string) string {
		return fmt.Sprintf(`<w:style w:type="paragraph" w:styleId="%s"><w:name w:val="%s"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr>%s</w:pPr><w:rPr>%s</w:rPr></w:style>`, id, name, ppr, rpr)
	}
	return xmlHead + `<w:styles ` + wNS + `>` +
		`<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Georgia" w:hAnsi="Georgia" w:eastAsia="Georgia" w:cs="Times New Roman"/>` +
		fmt.Sprintf(`<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, sz, sz) + `<w:lang w:val="en-GB"/></w:rPr></w:rPrDefault>` +
		`<w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="300" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/></w:style>` +
		style("Title", "Title", `<w:spacing w:after="240"/>`, fmt.Sprintf(`<w:b/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, sz*16/10, sz*16/10)) +
		style("Heading1", "heading 1", `<w:keepNext/><w:spacing w:before="320" w:after="60"/><w:outlineLvl w:val="0"/>`, fmt.Sprintf(`<w:b/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, sz*13/10, sz*13/10)) +
		style("Heading2", "heading 2", `<w:keepNext/><w:spacing w:before="240" w:after="60"/><w:outlineLvl w:val="1"/>`, fmt.Sprintf(`<w:b/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, sz*11/10, sz*11/10)) +
		style("Heading3", "heading 3", `<w:keepNext/><w:spacing w:before="200" w:after="40"/><w:outlineLvl w:val="2"/>`, `<w:b/>`) +
		style("Subtitle", "Subtitle", `<w:spacing w:after="120"/>`, `<w:i/><w:color w:val="444444"/>`) +
		style("Quote", "Quote", `<w:ind w:left="567"/>`, `<w:i/>`) +
		style("Credit", "Credit", `<w:spacing w:before="60"/>`, fmt.Sprintf(`<w:color w:val="444444"/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, sz*8/10, sz*8/10)) +
		style("Notice", "Notice", `<w:pBdr><w:top w:val="single" w:sz="4" w:space="2" w:color="CC9900"/><w:left w:val="single" w:sz="4" w:space="4" w:color="CC9900"/><w:bottom w:val="single" w:sz="4" w:space="2" w:color="CC9900"/><w:right w:val="single" w:sz="4" w:space="4" w:color="CC9900"/></w:pBdr><w:shd w:val="clear" w:color="auto" w:fill="FFF8E6"/>`, ``) +
		style("ListItem", "List Paragraph", `<w:ind w:left="567" w:hanging="283"/><w:spacing w:after="40"/>`, ``) +
		style("HeaderFooter", "Header and Footer", `<w:jc w:val="center"/><w:spacing w:after="0"/>`, fmt.Sprintf(`<w:color w:val="555555"/><w:sz w:val="%d"/><w:szCs w:val="%d"/>`, 18, 18)) +
		`</w:styles>`
}

func headerXML(d Doc) string {
	p := ""
	if d.Opt.header() && d.Title != "" {
		p = para("HeaderFooter", false, run(d.Title, runStyle{italic: true}))
	} else {
		p = `<w:p/>`
	}
	return xmlHead + `<w:hdr ` + wNS + `>` + p + `</w:hdr>`
}

func footerXML(o Options) string {
	p := `<w:p/>`
	if o.footer() {
		p = `<w:p><w:pPr><w:pStyle w:val="HeaderFooter"/></w:pPr><w:fldSimple w:instr=" PAGE "><w:r><w:t>1</w:t></w:r></w:fldSimple></w:p>`
	}
	return xmlHead + `<w:ftr ` + wNS + `>` + p + `</w:ftr>`
}

// relList holds document.xml's relationships: styles and the like, then one per web link.
type relList struct{ links []string }

func (l *relList) add(url string) string {
	l.links = append(l.links, url)
	return fmt.Sprintf("rIdL%d", len(l.links))
}

func (l *relList) xml() string {
	var b strings.Builder
	b.WriteString(xmlHead + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	b.WriteString(`<Relationship Id="rIdS" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`)
	b.WriteString(`<Relationship Id="rIdT" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>`)
	b.WriteString(`<Relationship Id="rIdH" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/>`)
	b.WriteString(`<Relationship Id="rIdF" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>`)
	for i, u := range l.links {
		fmt.Fprintf(&b, `<Relationship Id="rIdL%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="%s" TargetMode="External"/>`, i+1, xesc(u))
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

type runStyle struct {
	bold, italic, code, sup, under bool
}

func run(text string, st runStyle) string {
	var pr strings.Builder
	if st.bold {
		pr.WriteString(`<w:b/><w:bCs/>`)
	}
	if st.italic {
		pr.WriteString(`<w:i/><w:iCs/>`)
	}
	if st.under {
		pr.WriteString(`<w:u w:val="single"/>`)
	}
	if st.code {
		pr.WriteString(`<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas" w:cs="Consolas"/>`)
	}
	if st.sup {
		pr.WriteString(`<w:color w:val="555555"/><w:vertAlign w:val="superscript"/>`)
	}
	if isRTL(text) {
		pr.WriteString(`<w:rtl/>`)
	}
	rpr := ""
	if pr.Len() > 0 {
		rpr = "<w:rPr>" + pr.String() + "</w:rPr>"
	}
	return `<w:r>` + rpr + `<w:t xml:space="preserve">` + xesc(text) + `</w:t></w:r>`
}

func runs(spans []Span, links *relList) string {
	var b strings.Builder
	for _, s := range spans {
		if s.Break {
			b.WriteString(`<w:r><w:br/></w:r>`)
			continue
		}
		r := run(s.Text, runStyle{bold: s.Bold, italic: s.Italic, code: s.Code, under: s.Under})
		if u := safeURL(s.Link); u != "" && links != nil {
			r = `<w:hyperlink r:id="` + links.add(u) + `">` + strings.Replace(r, `<w:r>`, `<w:r><w:rPr><w:u w:val="single"/></w:rPr>`, 1) + `</w:hyperlink>`
			r = strings.Replace(r, `</w:rPr><w:rPr>`, ``, 1)
		}
		b.WriteString(r)
	}
	return b.String()
}

func para(style string, rtl bool, content string) string {
	pr := ""
	if style != "" {
		pr = `<w:pStyle w:val="` + style + `"/>`
	}
	if rtl {
		pr += `<w:bidi/>`
	}
	if pr != "" {
		pr = "<w:pPr>" + pr + "</w:pPr>"
	}
	return "<w:p>" + pr + content + "</w:p>"
}

func docxBody(d Doc, links *relList) string {
	var b strings.Builder
	b.WriteString(xmlHead + `<w:document ` + wNS + `><w:body>`)
	if d.Title != "" {
		b.WriteString(para("Title", isRTL(d.Title), run(d.Title, runStyle{})))
	}
	for _, p := range d.Pieces {
		if d.showTitle(p) {
			b.WriteString(para("Heading1", isRTL(p.Title), run(p.Title, runStyle{})))
		}
		if p.Subtitle != "" {
			b.WriteString(para("Subtitle", isRTL(p.Subtitle), run(p.Subtitle, runStyle{})))
		}
		docxBlocks(&b, p.Blocks, d.Opt, links)
		if !d.creditsAtEnd() {
			for _, c := range p.Credits {
				b.WriteString(para("Credit", isRTL(c), run(c, runStyle{})))
			}
		}
	}
	if cs := d.credits(); d.creditsAtEnd() && len(cs) > 0 {
		b.WriteString(para("Heading1", false, run("Credits", runStyle{})))
		for _, c := range cs {
			b.WriteString(para("Credit", isRTL(c), run(c, runStyle{})))
		}
	}
	w, h, _ := pageSize(d.Opt)
	fmt.Fprintf(&b, `<w:sectPr><w:headerReference w:type="default" r:id="rIdH"/><w:footerReference w:type="default" r:id="rIdF"/>`+
		`<w:pgSz w:w="%d" w:h="%d"/><w:pgMar w:top="1020" w:right="907" w:bottom="1134" w:left="907" w:header="510" w:footer="567" w:gutter="0"/></w:sectPr>`,
		twips(w), twips(h))
	b.WriteString(`</w:body></w:document>`)
	return b.String()
}

func twips(mm float64) int { return int(mm/25.4*1440 + 0.5) }

func docxBlocks(b *strings.Builder, bs []Block, opt Options, links *relList) {
	n := 0
	for i := 0; i < len(bs); i++ {
		bl := bs[i]
		rtl := isRTL(spansText(bl.Spans))
		if bl.Kind != Numbered && bl.Kind != Bullet { // numbering goes on over sub-point bullets
			n = 0
		}
		switch bl.Kind {
		case Heading:
			b.WriteString(para(fmt.Sprintf("Heading%d", min(max(bl.Level, 2), 3)), rtl, runs(bl.Spans, links)))
		case Quote:
			b.WriteString(para("Quote", rtl, runs(bl.Spans, links)))
		case Bullet:
			mark := "• "
			if bl.Check != "" {
				mark = checkMark(bl.Check, "☐ ", "☑ ")
			}
			b.WriteString(para("ListItem", rtl, run(mark, runStyle{})+runs(bl.Spans, links)))
		case Numbered:
			n++
			b.WriteString(para("ListItem", rtl, run(fmt.Sprintf("%d. ", n), runStyle{})+runs(bl.Spans, links)))
		case Notice:
			b.WriteString(para("Notice", false, runs(bl.Spans, links)))
		case Verse:
			var content strings.Builder
			start := i
			for ; i < len(bs) && bs[i].Kind == Verse; i++ {
				if i > start {
					content.WriteString(run(" ", runStyle{}))
				}
				if opt.verseNumbers() && bs[i].Num != "" {
					content.WriteString(run(bs[i].Num+" ", runStyle{sup: true}))
				}
				content.WriteString(runs(bs[i].Spans, links))
			}
			i--
			b.WriteString(para("", rtl, content.String()))
		case Table:
			docxTable(b, bl.Table, opt, links)
		case Handwriting:
			// Word has no place for strokes yet; they stay in the Markdown and HTML copies.
		case Rule:
			b.WriteString(`<w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="999999"/></w:pBdr></w:pPr></w:p>`)
		default:
			b.WriteString(para("", rtl, runs(bl.Spans, links)))
		}
	}
}

func docxTable(b *strings.Builder, g *Grid, opt Options, links *relList) {
	w, _, _ := pageSize(opt)
	width := twips(w) - 2*907
	numW := 0
	if g.numbered(opt) {
		numW = 520
	}
	cols := g.Cols()
	colWs := make([]int, cols)
	for i, f := range g.fractions() {
		colWs[i] = int(float64(width-numW) * f)
	}
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="` + fmt.Sprint(width) + `" w:type="dxa"/><w:tblLayout w:type="fixed"/>` +
		`<w:tblCellMar><w:left w:w="60" w:type="dxa"/><w:right w:w="100" w:type="dxa"/></w:tblCellMar></w:tblPr><w:tblGrid>`)
	if numW > 0 {
		fmt.Fprintf(b, `<w:gridCol w:w="%d"/>`, numW)
	}
	for _, w := range colWs {
		fmt.Fprintf(b, `<w:gridCol w:w="%d"/>`, w)
	}
	b.WriteString(`</w:tblGrid>`)
	cell := func(w int, content string, head bool) string {
		pr := fmt.Sprintf(`<w:tcW w:w="%d" w:type="dxa"/>`, w)
		if head {
			pr += `<w:tcBorders><w:bottom w:val="single" w:sz="4" w:space="0" w:color="999999"/></w:tcBorders>`
		}
		return `<w:tc><w:tcPr>` + pr + `</w:tcPr>` + content + `</w:tc>`
	}
	if g.HasHead() {
		b.WriteString(`<w:tr><w:trPr><w:tblHeader/><w:cantSplit/></w:trPr>`)
		if numW > 0 {
			b.WriteString(cell(numW, `<w:p/>`, true))
		}
		for i, h := range g.headSpans() {
			bold := make([]Span, len(h))
			for j, s := range h {
				s.Bold = true
				bold[j] = s
			}
			b.WriteString(cell(colWs[min(i, cols-1)], para("", isRTL(spansText(h)), runs(bold, links)), true))
		}
		b.WriteString(`</w:tr>`)
	}
	for _, r := range g.Rows {
		b.WriteString(`<w:tr><w:trPr><w:cantSplit/></w:trPr>`)
		if numW > 0 {
			b.WriteString(cell(numW, para("", false, run(r.Num, runStyle{sup: true})), false))
		}
		for i, c := range r.Cells {
			b.WriteString(cell(colWs[min(i, cols-1)], para("", isRTL(spansText(c)), runs(c, links)), false))
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl>`)
	b.WriteString(`<w:p/>`)
}

// xesc escapes text for XML and drops characters XML 1.0 cannot hold (control characters).
func xesc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '"':
			b.WriteString("&quot;")
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(' ')
		case r < 0x20 || r == 0xFFFE || r == 0xFFFF || r >= 0xD800 && r <= 0xDFFF:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isRTL reports whether text starts, in its first letter, in a right-to-left script.
func isRTL(s string) bool {
	for _, r := range s {
		if rtlRune(r) {
			return true
		}
		if unicode.IsLetter(r) {
			return false
		}
	}
	return false
}

func rtlRune(r rune) bool {
	return r >= 0x0590 && r <= 0x08FF || r >= 0xFB1D && r <= 0xFDFF || r >= 0xFE70 && r <= 0xFEFF
}
