package bible

import (
	"bufio"
	"context"
	"io"
	"regexp"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// STEPBible data (github.com/STEPBible/STEPBible-Data, CC BY 4.0, Tyndale House Cambridge) is
// tab-separated text with explanatory headers. Word lines of the tagged texts start with a
// reference such as "Gen.1.1#01=L" (TAHOT, the Hebrew Old Testament, in English verse numbering,
// with the Hebrew numbering in brackets: "Psa.3.1(3.2)#01=L") or "Mat.1.1#01=NKO" (TAGNT, the Greek
// New Testament; the letters say which editions have the word). Lexicon lines start with an
// extended Strong's number such as "G0026" or "H0001G".

var stepWord = regexp.MustCompile(`^([1-3]?[A-Z][a-z]{1,2})\.([0-9]+)\.([0-9]+)[^#\t]*#[0-9]+=([^\t]*)\t`)

// StepText says which tagged text a file holds.
type StepText string

const (
	StepHebrew StepText = "tahot"
	StepGreek  StepText = "tagnt"
)

// InstallSTEP makes one Bible module from STEPBible tagged-text files (TAHOT and/or TAGNT), read
// in order. Each word becomes a <w> element with its Strong's number, morphology and gloss.
func (l *Library) InstallSTEP(ctx context.Context, o InstallOptions, files []io.Reader) (Meta, []string, error) {
	b := newBuilder()
	for _, f := range files {
		if err := readSTEPText(ctx, f, b); err != nil {
			return Meta{}, nil, err
		}
	}
	b.endVerse()
	return l.finish(ctx, o, Doc{}, b)
}

func readSTEPText(ctx context.Context, r io.Reader, b *builder) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	cur := verse.ID(-1)
	glue := false // the last word ended with a maqqef, so no space follows
	for n := 0; sc.Scan(); n++ {
		if n%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		line := sc.Text()
		m := stepWord.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		f := strings.Split(line, "\t")
		id, err := STEPRef(m[1] + "." + m[2] + "." + m[3])
		if err != nil {
			b.warn("%s: %v", m[0], err)
			continue
		}
		greek := id.Book() >= 40
		if greek && !strings.Contains(m[4], "N") {
			continue // not in the Nestle-Aland based text
		}
		if id != cur {
			b.startVerse(id, id)
			cur, glue = id, true
		}
		word, strong, morph, gloss := stepFields(f, greek)
		if word == "" {
			continue
		}
		if !glue {
			b.addText(" ")
		}
		b.openSpan("w", attr("s", strong)+attr("m", morph)+attr("g", gloss), false, false)
		b.addText(word)
		b.closeSpan("w")
		if strong != "" {
			b.strong([]string{strong})
		}
		glue = strings.HasSuffix(word, "־")
	}
	return sc.Err()
}

var braced = regexp.MustCompile(`\{([HG][0-9]+[A-Za-z]?)\}`)

// stepFields picks the word, its main Strong's number (without the extended letter), its
// morphology code and its English gloss from a word line.
func stepFields(f []string, greek bool) (word, strong, morph, gloss string) {
	get := func(i int) string {
		if i < len(f) {
			return strings.TrimSpace(f[i])
		}
		return ""
	}
	if greek {
		word, _, _ = strings.Cut(get(1), " (")
		gloss = get(2)
		code := get(3)
		num, mo, _ := strings.Cut(code, "=")
		if i := strings.LastIndex(strings.ReplaceAll(num, "«", "|"), "|"); i >= 0 {
			num = strings.ReplaceAll(num, "«", "|")[i+1:]
		}
		return word, baseStrong(normStrong(num)), mo, gloss
	}
	word = strings.NewReplacer("/", "", `\`, "").Replace(get(1))
	gloss = get(3)
	if m := braced.FindStringSubmatch(get(4)); m != nil {
		strong = baseStrong(normStrong(m[1]))
	}
	return word, strong, get(5), gloss
}

var stepLexLine = regexp.MustCompile(`^([HG][0-9]{4}[A-Za-z]?)\t`)

// ReadSTEPLexicon reads a STEPBible brief lexicon (TBESH or TBESG). Each line becomes an entry
// keyed by its Strong's number. With meanings false only the gloss is kept, not the definition.
func ReadSTEPLexicon(r io.Reader, meanings bool) ([]Headword, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var out []Headword
	for sc.Scan() {
		line := sc.Text()
		if !stepLexLine.MatchString(line) {
			continue
		}
		f := strings.Split(line, "\t")
		for len(f) < 8 {
			f = append(f, "")
		}
		// eStrong, dStrong ("H0001G = …"), uStrong, word, transliteration, morphology, gloss, meaning
		d, _, _ := strings.Cut(f[1], " ")
		h := Headword{Key: strings.TrimSpace(d), Strong: normStrong(f[0]), Lemma: strings.TrimSpace(f[3]),
			Translit: strings.TrimSpace(f[4]), Morph: strings.TrimSpace(f[5]), Gloss: strings.TrimSpace(f[6])}
		if h.Key == "" {
			h.Key = f[0]
		}
		if meanings {
			h.Text, h.Markup = stepMeaning(f[7])
		}
		if h.Text == "" {
			h.Text, h.Markup = h.Gloss, esc(h.Gloss)
		}
		out = append(out, h)
	}
	return out, sc.Err()
}

var (
	stepRef  = regexp.MustCompile(`<ref='([^']*)'>(.*?)</ref>`)
	stepTags = regexp.MustCompile(`<[^>]*>`)
	stepBR   = regexp.MustCompile(`(?i)<br\s*/?>`)
)

// stepMeaning turns a lexicon definition's light HTML into Xegesis markup: <br> becomes a
// paragraph, <ref='Jhn.13.35'> a reference, and other tags are dropped.
func stepMeaning(s string) (string, string) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "__", "")
	var text, markup strings.Builder
	parts := stepBR.Split(s, -1)
	for i, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if i > 0 && markup.Len() > 0 {
			markup.WriteString("<p/>")
			text.WriteString(" ")
		}
		last := 0
		for _, m := range stepRef.FindAllStringSubmatchIndex(p, -1) {
			plain := clean(p[last:m[0]])
			text.WriteString(plain)
			markup.WriteString(esc(plain))
			label := clean(p[m[4]:m[5]])
			target := ""
			first, _, _ := strings.Cut(p[m[2]:m[3]], ";")
			if id, err := STEPRef(strings.TrimRight(strings.TrimSpace(first), ".")); err == nil {
				target = id.OSIS()
			}
			text.WriteString(label)
			markup.WriteString("<r" + attr("o", target) + ">" + esc(label) + "</r>")
			last = m[1]
		}
		plain := clean(p[last:])
		text.WriteString(plain)
		markup.WriteString(esc(plain))
	}
	return squeeze(text.String()), squeeze(markup.String())
}

func clean(s string) string { return unescape(stepTags.ReplaceAllString(s, "")) }
