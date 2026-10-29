package bible

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Module kinds. Bibles, commentaries and cross-reference sets are keyed by verse and use the
// verses table; dictionaries and lexicons are keyed by headword and use the entries table.
const (
	KindBible      = "bible"
	KindCommentary = "commentary"
	KindCrossRef   = "crossref"
	KindDictionary = "dictionary"
	KindLexicon    = "lexicon" // a dictionary keyed by Strong's numbers
)

// VerseKeyed reports whether a kind stores verses.
func VerseKeyed(kind string) bool {
	return kind == "" || kind == KindBible || kind == KindCommentary || kind == KindCrossRef
}

// Headword is one dictionary or lexicon entry.
type Headword struct {
	Key      string `json:"key" desc:"The headword as the source gives it"`
	Strong   string `json:"strong,omitempty" desc:"Strong's number, e.g. G26, when the entry is keyed by one"`
	Lemma    string `json:"lemma,omitempty" desc:"The word in its original language"`
	Translit string `json:"transliteration,omitempty"`
	Morph    string `json:"morphology,omitempty"`
	Gloss    string `json:"gloss,omitempty" desc:"A short English meaning"`
	Text     string `json:"text"`
	Markup   string `json:"markup,omitempty"`
}

// fold makes a lookup key: lower case, without accents or vowel points.
func fold(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, strings.ToLower(strings.TrimSpace(s)))
	if err != nil {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return out
}

func insertHeadwords(ctx context.Context, tx *sql.Tx, heads []Headword) error {
	if len(heads) == 0 {
		return nil
	}
	st, err := tx.PrepareContext(ctx, `INSERT INTO entries VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, h := range heads {
		if _, err := st.ExecContext(ctx, h.Key, fold(h.Key), h.Strong, h.Lemma, fold(h.Lemma), h.Translit,
			h.Morph, h.Gloss, h.Text, h.Markup); err != nil {
			return err
		}
	}
	return nil
}

// InstallHeadwords makes a dictionary or lexicon module from its entries.
func (l *Library) InstallHeadwords(ctx context.Context, o InstallOptions, heads []Headword) (Meta, error) {
	if len(heads) == 0 {
		return Meta{}, errors.New("bible: the dictionary has no entries")
	}
	if o.Kind == "" {
		o.Kind = KindDictionary
	}
	m, err := o.meta(Doc{}, &Doc{})
	if err != nil {
		return Meta{}, err
	}
	m.VerseCount = len(heads)
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return Meta{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := os.Stat(l.path(m.ID)); err == nil && !o.Replace {
		return Meta{}, fmt.Errorf("bible: module %q is already installed", m.ID)
	}
	if old, ok := l.open[m.ID]; ok {
		old.Close()
		delete(l.open, m.ID)
	}
	return m, write(ctx, l.path(m.ID), m, nil, nil, heads...)
}

// Lookup finds entries by Strong's number, by original-language word or by headword. A Strong's
// number with a letter suffix (H1254A) also finds the plain number's entries, and the other way round.
func (m *Module) Lookup(ctx context.Context, strong, word string, limit int) ([]Headword, error) {
	if VerseKeyed(m.Kind) {
		return nil, nil
	}
	var rows *sql.Rows
	var err error
	cols := `SELECT key, strong, lemma, translit, morph, gloss, text, markup FROM entries`
	if strong != "" {
		base := baseStrong(strong)
		rows, err = m.db.QueryContext(ctx, cols+` WHERE strong = ? OR strong = ? OR (strong >= ? AND strong < ?)
			ORDER BY strong = ? DESC, rowid LIMIT ?`, strong, base, base+"A", base+"[", strong, limit)
	} else {
		f := fold(word)
		rows, err = m.db.QueryContext(ctx, cols+` WHERE lemma_fold = ? OR fold = ? ORDER BY rowid LIMIT ?`, f, f, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Headword{}
	for rows.Next() {
		var h Headword
		if err := rows.Scan(&h.Key, &h.Strong, &h.Lemma, &h.Translit, &h.Morph, &h.Gloss, &h.Text, &h.Markup); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

var strongSuffix = regexp.MustCompile(`^([HG][0-9]+)[A-Za-z]*$`)

// baseStrong drops a letter suffix: H1254A becomes H1254.
func baseStrong(s string) string {
	if m := strongSuffix.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

// NormStrong normalises a Strong's number such as "strong:H07225", "h7225" or "G0026" to H7225 or
// G26, keeping a letter suffix (H1254A); it returns "" for other values.
func NormStrong(s string) string { return normStrong(s) }

// DictMarkup converts a dictionary entry in TEI, ThML, OSIS or plain text into Xegesis markup,
// and picks out the headword fields TEI gives (orth, pron, def).
func DictMarkup(src, sourceType string) Headword {
	var h Headword
	b := newBuilder()
	b.startVerse(1_001_001, 1_001_001)
	var err error
	if strings.Contains(src, "<entryFree") || strings.Contains(src, "<orth") {
		sourceType = "tei"
	}
	switch strings.ToLower(sourceType) {
	case "tei":
		err = readTEIFragment(src, b, &h)
	case "thml":
		err = readThMLFragment(src, b)
	case "osis":
		err = readOSISFragment(src, b)
	default:
		for i, para := range strings.Split(strings.ReplaceAll(src, "\r", ""), "\n\n") {
			if i > 0 {
				b.paragraph()
			}
			b.addText(para)
		}
	}
	if err != nil {
		b.addText(stripTags(src))
	}
	b.closeAll()
	d := b.doc()
	if len(d.Verses) > 0 {
		h.Text, h.Markup = d.Verses[0].Text, d.Verses[0].Markup
	}
	return h
}

// readTEIFragment reads the TEI used in dictionary entries: orth (the word; type="trans" is a
// transliteration), pron, def, ref and lb.
func readTEIFragment(s string, b *builder, h *Headword) error {
	d := xml.NewDecoder(strings.NewReader("<entry>" + s + "</entry>"))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	var stack []string
	field := ""
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			tag := ""
			switch strings.ToLower(t.Name.Local) {
			case "orth":
				field = "orth"
				if get(t, "type") == "trans" {
					field = "trans"
				} else if get(t, "type") != "" {
					field = "skip"
				}
			case "pron":
				field = "pron"
			case "ref":
				ref := ""
				if id, err := osisVerse(get(t, "osisRef")); err == nil {
					ref = id.OSIS()
				}
				b.openSpan("r", attr("o", ref), false, false)
				tag = "r"
			case "lb", "p":
				b.paragraph()
			}
			stack = append(stack, tag)
		case xml.EndElement:
			switch strings.ToLower(t.Name.Local) {
			case "orth", "pron":
				field = ""
			}
			if n := len(stack); n > 0 {
				if stack[n-1] != "" {
					b.closeSpan(stack[n-1])
				}
				stack = stack[:n-1]
			}
		case xml.CharData:
			text := string(t)
			switch field {
			case "orth":
				if h.Lemma == "" {
					h.Lemma = strings.TrimSpace(text)
				}
				continue
			case "trans":
				if h.Translit == "" {
					h.Translit = strings.TrimSpace(text)
				}
				continue
			case "pron", "skip":
				continue
			}
			b.addText(text)
		}
	}
}

// STEPRef turns a STEPBible reference such as "Jhn.13.35" into a verse ID.
func STEPRef(s string) (verse.ID, error) {
	p := strings.Split(strings.TrimSpace(s), ".")
	if len(p) < 3 {
		return 0, fmt.Errorf("not a verse reference: %q", s)
	}
	if b, ok := stepBooks[p[0]]; ok {
		p[0] = b
	}
	return osisVerse(strings.Join(p[:3], "."))
}

// stepBooks maps STEPBible book abbreviations that differ from OSIS IDs and aliases.
var stepBooks = map[string]string{
	"Exo": "Exod", "Deu": "Deut", "Jdg": "Judg", "Rut": "Ruth", "1Ki": "1Kgs", "2Ki": "2Kgs",
	"Sng": "Song", "Ezk": "Ezek", "Jol": "Joel", "Amo": "Amos", "Oba": "Obad", "Jon": "Jonah",
	"Nam": "Nah", "Mrk": "Mark", "Jhn": "John", "Php": "Phil", "1Jn": "1John", "2Jn": "2John",
	"3Jn": "3John", "Jud": "Jude", "Psa": "Ps", "Pro": "Prov", "Ecc": "Eccl", "Est": "Esth",
	"Lam": "Lam", "Mat": "Matt", "Luk": "Luke", "Act": "Acts", "Rom": "Rom", "1Co": "1Cor",
	"2Co": "2Cor", "1Th": "1Thess", "2Th": "2Thess", "1Ti": "1Tim", "2Ti": "2Tim", "Tit": "Titus",
	"Phm": "Phlm", "Jas": "Jas", "1Pe": "1Pet", "2Pe": "2Pet", "Rev": "Rev", "Zep": "Zeph",
	"Hag": "Hag", "Zec": "Zech", "Mal": "Mal", "Hab": "Hab", "Mic": "Mic", "Hos": "Hos",
	"Dan": "Dan", "Isa": "Isa", "Jer": "Jer", "Job": "Job", "Neh": "Neh", "Ezr": "Ezra",
	"1Ch": "1Chr", "2Ch": "2Chr", "1Sa": "1Sam", "2Sa": "2Sam", "Jos": "Josh", "Num": "Num",
	"Lev": "Lev", "Gen": "Gen", "Col": "Col", "Eph": "Eph", "Gal": "Gal", "Heb": "Heb",
}

// joinRepeats makes one entry of consecutive verses with the same text, as commentaries store a
// note on several verses once per verse.
func joinRepeats(vs []Verse) []Verse {
	var out []Verse
	for _, v := range vs {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if last.Markup == v.Markup && v.ID.Book() == last.ID.Book() && v.ID.Chapter() == last.End.Chapter() &&
				v.ID.Verse() == last.End.Verse()+1 {
				last.End = v.End
				continue
			}
		}
		out = append(out, v)
	}
	return out
}
