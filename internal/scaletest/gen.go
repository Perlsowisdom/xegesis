// Package scaletest generates synthetic libraries for the 10,000-book test (T12) and holds the
// test itself. The heavy test needs the scale build tag; see docs/performance.md.
package scaletest

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"fmt"
	"html"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// Planted text the measurements search for.
const (
	RareWord = "zarephathite"   // in about one book in 2,000
	Phrase   = "living hope of" // in about one book in 50, as "the living hope of glory"
	PageText = 1800             // characters of text on a printed page, roughly
)

// Config says what to generate.
type Config struct {
	Dir      string  // the two library folders are made inside it
	Books    int     // number of books
	Seed     uint64  // the same seed gives the same library
	External float64 // share of books on the "external drive" folder (default 0.3)
	MaxPages int     // largest book (default 1000)
}

// Stats describes what was generated.
type Stats struct {
	Internal, External string // the two library folders
	Books              int
	Formats            map[string]int
	Pages              int64
	TextBytes          int64 // characters of headings and paragraphs, the text an index holds
	FileBytes          int64
	RareBooks          int
	PhraseBooks        int
	LargestBook        string // path of the book with the most pages
	LargestPages       int
}

// book is one book's plan.
type book struct {
	i                  int
	format, rel        string
	title, author, pub string
	year, pages        int
	rights             string
	refRate            float64
	rare, phrase       bool
}

var vocabulary = newVocab(60000, 1)

// Generate writes a library of synthetic books: mostly EPUB, some PDF and DOCX, from a few pages to
// MaxPages, with chapters, headings and Bible references, some public domain.
func Generate(c Config) (Stats, error) {
	if c.External == 0 {
		c.External = 0.3
	}
	if c.MaxPages == 0 {
		c.MaxPages = 1000
	}
	st := Stats{Internal: filepath.Join(c.Dir, "Library"), External: filepath.Join(c.Dir, "External drive", "Books"),
		Books: c.Books, Formats: map[string]int{}}
	plans := make([]book, c.Books)
	for i := range plans {
		plans[i] = plan(c, i)
		b := &plans[i]
		st.Formats[b.format]++
		st.Pages += int64(b.pages)
		if b.rare {
			st.RareBooks++
		}
		if b.phrase {
			st.PhraseBooks++
		}
	}
	var text, size atomic.Int64
	var firstErr error
	var mu sync.Mutex
	work := make(chan int)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				b := plans[i]
				root := st.Internal
				if rand.New(rand.NewPCG(c.Seed, uint64(i)+1e9)).Float64() < c.External {
					root = st.External
				}
				path := filepath.Join(root, filepath.FromSlash(b.rel))
				n, err := write(path, b, c.Seed)
				if err == nil {
					fi, serr := os.Stat(path)
					if err = serr; err == nil {
						size.Add(fi.Size())
					}
				}
				text.Add(n)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
				}
				if b.pages > st.LargestPages && b.format == "epub" {
					st.LargestPages, st.LargestBook = b.pages, path
				}
				mu.Unlock()
			}
		}()
	}
	for i := range plans {
		work <- i
	}
	close(work)
	wg.Wait()
	st.TextBytes, st.FileBytes = text.Load(), size.Load()
	return st, firstErr
}

var subjects = []string{"Commentaries", "Theology", "Church History", "Sermons", "Devotional", "Biblical Studies",
	"Languages", "Biography", "Missions", "Ethics"}

var publishers = []string{"Example House", "Lampstand Press", "Open Books Trust", "Riverside Publishing",
	"Cornerstone Media", "Old Paths Library"}

var rightsStatements = []string{"Public domain", "This work is in the public domain.",
	"Public Domain Mark 1.0", "CC0 1.0 Universal (public domain dedication)"}

func plan(c Config, i int) book {
	r := rand.New(rand.NewPCG(c.Seed, uint64(i)))
	b := book{i: i, year: 1600 + r.IntN(425), pub: publishers[r.IntN(len(publishers))]}
	switch x := r.Float64(); {
	case x < 0.85:
		b.format = "epub"
	case x < 0.95:
		b.format = "pdf"
	default:
		b.format = "docx"
	}
	// Log-normal sizes: median about 60 pages, a long tail up to MaxPages.
	b.pages = int(math.Round(math.Exp(math.Log(60) + 1.0*r.NormFloat64())))
	b.pages = max(3, min(b.pages, c.MaxPages))
	if b.format != "epub" {
		b.pages = min(b.pages, 400)
	}
	b.title = titleCase(words(r, 2+r.IntN(4)))
	b.author = titleCase(words(r, 1)) + " " + titleCase(words(r, 1))
	subject := subjects[r.IntN(len(subjects))]
	b.refRate = 0.15
	if subject == "Commentaries" || subject == "Sermons" || subject == "Biblical Studies" {
		b.refRate = 0.6
	}
	if r.Float64() < 0.2 {
		b.rights = rightsStatements[r.IntN(len(rightsStatements))]
	}
	b.rare = i%2000 == 17
	b.phrase = i%50 == 3
	b.rel = fmt.Sprintf("%s/%s/%05d %s.%s", subject, b.author, i, b.title, b.format)
	return b
}

func words(r *rand.Rand, n int) string {
	w := make([]string, n)
	for i := range w {
		w[i] = vocabulary.word(r)
	}
	return strings.Join(w, " ")
}

func titleCase(s string) string {
	f := strings.Fields(s)
	for i, w := range f {
		f[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(f, " ")
}

// para is one block of a book.
type para struct {
	text    string
	heading int // 0 for a paragraph, 1 or 2 for a heading
}

// chapters makes the book's text: chapters of about 20 pages, a section heading every few pages,
// paragraphs of 40 to 160 words.
func chapters(b book, seed uint64) [][]para {
	r := rand.New(rand.NewPCG(seed^0x5eed, uint64(b.i)))
	nch := max(1, (b.pages+19)/20)
	per := b.pages * PageText / nch
	var out [][]para
	rareAt, phraseAt := r.IntN(nch), r.IntN(nch)
	for c := range nch {
		ch := []para{{text: fmt.Sprintf("Chapter %d: %s", c+1, titleCase(words(r, 2+r.IntN(3)))), heading: 1}}
		n, sinceHeading := 0, 0
		for n < per {
			if sinceHeading > 3*PageText {
				ch = append(ch, para{text: titleCase(words(r, 2+r.IntN(4))), heading: 2})
				sinceHeading = 0
			}
			p := paragraph(r, b.refRate)
			if b.rare && c == rareAt && len(ch) == 2 {
				p += " The " + RareWord + " widow shared her bread."
			}
			if b.phrase && c == phraseAt && len(ch) == 3 {
				p += " We wait for the " + Phrase + " glory."
			}
			ch = append(ch, para{text: p})
			n += len(p)
			sinceHeading += len(p)
		}
		out = append(out, ch)
	}
	return out
}

func paragraph(r *rand.Rand, refRate float64) string {
	var b strings.Builder
	n := 40 + r.IntN(120)
	sentence := 0
	for i := range n {
		w := vocabulary.word(r)
		if sentence == 0 {
			w = strings.ToUpper(w[:1]) + w[1:]
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(w)
		sentence++
		if sentence > 8 && r.IntN(8) == 0 || i == n-1 {
			if r.Float64() < refRate/3 {
				b.WriteString(" (" + reference(r) + ")")
			}
			b.WriteByte('.')
			sentence = 0
		} else if r.IntN(14) == 0 {
			b.WriteByte(',')
		}
	}
	return b.String()
}

// refBooks are book names as authors write them, with chapter counts.
var refBooks = []struct {
	name     string
	chapters int
}{{"Gen", 50}, {"Genesis", 50}, {"Exod.", 40}, {"Deut", 34}, {"Ps", 150}, {"Psalm", 150}, {"Prov", 31},
	{"Isa", 66}, {"Isaiah", 66}, {"Jer", 52}, {"Matt", 28}, {"Matthew", 28}, {"Mark", 16}, {"Luke", 24},
	{"John", 21}, {"Acts", 28}, {"Rom", 16}, {"Romans", 16}, {"1 Cor", 16}, {"2 Cor.", 13}, {"Gal", 6},
	{"Eph", 6}, {"Phil", 4}, {"Col", 4}, {"Heb", 13}, {"Hebrews", 13}, {"Jas", 5}, {"1 Pet", 5}, {"Rev", 22}}

func reference(r *rand.Rand) string {
	if r.IntN(40) == 0 {
		return "Rom 8:28"
	}
	b := refBooks[r.IntN(len(refBooks))]
	ref := fmt.Sprintf("%s %d:%d", b.name, 1+r.IntN(b.chapters), 1+r.IntN(12))
	if r.IntN(4) == 0 {
		ref += fmt.Sprintf("-%d", 13+r.IntN(5))
	}
	return ref
}

// write writes one book and returns the characters of text in it.
func write(path string, b book, seed uint64) (int64, error) {
	chs := chapters(b, seed)
	var n int64
	for _, ch := range chs {
		for _, p := range ch {
			n += int64(len(p.text))
		}
	}
	var body []byte
	var err error
	switch b.format {
	case "epub":
		body, err = epub(b, chs)
	case "pdf":
		body = pdf(b, chs)
	default:
		body, err = docx(b, chs)
	}
	if err != nil {
		return n, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return n, err
	}
	return n, os.WriteFile(path, body, 0o644)
}

func zipFiles(files [][2]string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i, f := range files {
		h := &zip.FileHeader{Name: f[0], Method: zip.Deflate}
		if i == 0 && f[0] == "mimetype" {
			h.Method = zip.Store
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f[1])); err != nil {
			return nil, err
		}
	}
	err := zw.Close()
	return buf.Bytes(), err
}

func esc(s string) string { return html.EscapeString(s) }

func epub(b book, chs [][]para) ([]byte, error) {
	var manifest, spine, nav strings.Builder
	files := [][2]string{{"mimetype", "application/epub+zip"},
		{"META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
<rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`}}
	for c, ch := range chs {
		name := fmt.Sprintf("ch%03d.xhtml", c+1)
		fmt.Fprintf(&manifest, `<item id="c%d" href="text/%s" media-type="application/xhtml+xml"/>`+"\n", c+1, name)
		fmt.Fprintf(&spine, `<itemref idref="c%d"/>`, c+1)
		fmt.Fprintf(&nav, `<li><a href="text/%s">%s</a></li>`, name, esc(ch[0].text))
		var x strings.Builder
		x.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
			`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>` + esc(ch[0].text) + "</title></head><body>\n")
		for _, p := range ch {
			tag := "p"
			if p.heading > 0 {
				tag = fmt.Sprintf("h%d", p.heading)
			}
			fmt.Fprintf(&x, "<%s>%s</%s>\n", tag, esc(p.text), tag)
		}
		x.WriteString("</body></html>")
		files = append(files, [2]string{"OEBPS/text/" + name, x.String()})
	}
	rights := ""
	if b.rights != "" {
		rights = "<dc:rights>" + esc(b.rights) + "</dc:rights>"
	}
	files = append(files,
		[2]string{"OEBPS/content.opf", fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:identifier id="uid">urn:xegesis:scaletest:%d</dc:identifier>
<dc:title>%s</dc:title><dc:creator>%s</dc:creator><dc:publisher>%s</dc:publisher>
<dc:date>%d</dc:date><dc:language>en</dc:language>%s
</metadata>
<manifest>
<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
%s</manifest>
<spine>%s</spine>
</package>`, b.i, esc(b.title), esc(b.author), esc(b.pub), b.year, rights, manifest.String(), spine.String())},
		[2]string{"OEBPS/nav.xhtml", `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body>
<nav epub:type="toc"><ol>` + nav.String() + `</ol></nav></body></html>`})
	return zipFiles(files)
}

func docx(b book, chs [][]para) ([]byte, error) {
	var d strings.Builder
	d.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, ch := range chs {
		for _, p := range ch {
			d.WriteString("<w:p>")
			if p.heading > 0 {
				fmt.Fprintf(&d, `<w:pPr><w:pStyle w:val="Heading%d"/></w:pPr>`, p.heading)
			}
			d.WriteString(`<w:r><w:t xml:space="preserve">` + esc(p.text) + "</w:t></w:r></w:p>\n")
		}
	}
	d.WriteString("</w:body></w:document>")
	rights := ""
	if b.rights != "" {
		rights = "<dc:rights>" + esc(b.rights) + "</dc:rights>"
	}
	return zipFiles([][2]string{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`},
		{"docProps/core.xml", `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` + esc(b.title) + `</dc:title><dc:creator>` +
			esc(b.author) + `</dc:creator>` + rights + `</cp:coreProperties>`},
		{"word/document.xml", d.String()},
	})
}

// pdf lays the text out in lines of about 90 characters, 40 lines to a page, with compressed
// content streams.
func pdf(bk book, chs [][]para) []byte {
	var pages []string
	var page strings.Builder
	lines := 0
	flush := func() {
		if lines > 0 {
			pages = append(pages, "BT /F1 11 Tf 72 740 Td 14 TL "+page.String()+"ET")
			page.Reset()
			lines = 0
		}
	}
	line := func(s string) {
		page.WriteString("(" + pdfEsc(s) + ") Tj T* ")
		if lines++; lines >= 40 {
			flush()
		}
	}
	for _, ch := range chs {
		for _, p := range ch {
			words := strings.Fields(p.text)
			var cur strings.Builder
			for _, w := range words {
				if cur.Len()+len(w) > 90 {
					line(cur.String())
					cur.Reset()
				}
				if cur.Len() > 0 {
					cur.WriteByte(' ')
				}
				cur.WriteString(w)
			}
			line(cur.String())
			page.WriteString("T* ")
		}
	}
	flush()
	var b bytes.Buffer
	var offsets []int
	obj := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	b.WriteString("%PDF-1.4\n")
	var kids strings.Builder
	for i := range pages {
		fmt.Fprintf(&kids, "%d 0 R ", 4+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), len(pages)))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	for i, content := range pages {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", 5+2*i))
		var z bytes.Buffer
		w := zlib.NewWriter(&z)
		w.Write([]byte(content))
		w.Close()
		obj(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", z.Len(), z.String()))
	}
	obj(fmt.Sprintf("<< /Title (%s) /Author (%s) >>", pdfEsc(bk.title), pdfEsc(bk.author)))
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1,
		len(offsets), xref)
	return b.Bytes()
}

func pdfEsc(s string) string {
	return strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(s)
}
