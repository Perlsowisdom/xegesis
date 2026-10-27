package bible

import (
	"strconv"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// USFM 3 and USX share one set of styles ("markers"), so both importers feed this handler.

// usfmBooks maps USFM book codes to verse book numbers.
var usfmBooks = map[string]int{}

func init() {
	codes := "GEN EXO LEV NUM DEU JOS JDG RUT 1SA 2SA 1KI 2KI 1CH 2CH EZR NEH EST JOB PSA PRO ECC SNG " +
		"ISA JER LAM EZK DAN HOS JOL AMO OBA JON MIC NAM HAB ZEP HAG ZEC MAL MAT MRK LUK JHN ACT ROM " +
		"1CO 2CO GAL EPH PHP COL 1TH 2TH 1TI 2TI TIT PHM HEB JAS 1PE 2PE 1JN 2JN 3JN JUD REV " +
		"TOB JDT ESG WIS SIR BAR LJE S3Y SUS BEL 1MA 2MA 3MA 4MA MAN 1ES 2ES PS2"
	for i, c := range strings.Fields(codes) {
		usfmBooks[c] = i + 1
	}
}

func styleSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, f := range strings.Fields(s) {
		m[f] = true
	}
	return m
}

var (
	usfmHeader = styleSet("ide h h1 h2 h3 toc1 toc2 toc3 toca1 toca2 toca3 mt mt1 mt2 mt3 mt4 mte mte1 mte2 " +
		"imt imt1 imt2 is is1 is2 ip ipi im imi ipq imq ipr iq iq1 iq2 ib ili ili1 ili2 iot io io1 io2 io3 " +
		"iex imte ie rem usfm sts cl cp restore periph")
	usfmHeading = styleSet("s s1 s2 s3 s4 ms ms1 ms2 ms3 mr sr r d sp qa cd sd sd1 sd2")
	usfmPara    = styleSet("p m po pr cls pmo pm pmc pmr pi pi1 pi2 pi3 mi nb pc ph ph1 ph2 b q q1 q2 q3 q4 " +
		"qr qc qm qm1 qm2 qd lh li li1 li2 li3 lf lim lim1 lim2 tr th1 th2 th3 thr1 thr2 tc1 tc2 tc3 tc4 " +
		"tcr1 tcr2 tcr3 pb")
	usfmNoteInner = styleSet("fr ft fq fqa fk fl fw fp fv fdc fm xo xk xq xt xta xop xot xnt xdc")
	usfmSkipChar  = styleSet("ca va vp fig jmp")
	usfmPassChar  = styleSet("nd bk pn png addpn qs qac sls tl dc k ord sig lit it bd bdit em no sc sup rb " +
		"pro wg wh wa ndx rq litl lik liv liv1 liv2")
	usfmCharTag = map[string]string{"wj": "j", "add": "add"}
)

type usfmHandler struct {
	b        *builder
	book     int
	chapter  int
	mode     string // "", "header" or "heading": the kind of the open paragraph
	note     string // "f" or "x" inside a note
	noteSkip bool   // inside a note's origin reference (\fr, \xo)
	skipChar int
	chars    []string // open character styles
}

func (h *usfmHandler) setBook(code string) {
	h.endPara()
	h.b.endVerse()
	h.b.closeAll()
	h.chars = nil
	h.book, h.chapter = usfmBooks[strings.ToUpper(code)], 0
	if h.book == 0 {
		h.b.warn("unknown USFM book %q skipped", code)
	}
	h.mode = "header"
}

func (h *usfmHandler) setChapter(num string) {
	h.endPara()
	h.b.endVerse()
	n, err := strconv.Atoi(strings.TrimSpace(num))
	if err != nil {
		h.b.warn("bad chapter number %q", num)
		return
	}
	h.chapter = n
}

func (h *usfmHandler) startVerse(num string) {
	h.endPara()
	if h.book == 0 || h.chapter == 0 {
		h.b.endVerse()
		if h.book != 0 {
			h.b.warn("verse %s before any chapter skipped", num)
		}
		return
	}
	first, last := verseNumbers(num)
	id, err := verse.Make(h.book, h.chapter, first)
	end, err2 := verse.Make(h.book, h.chapter, last)
	if err != nil || err2 != nil || first < 0 {
		h.b.endVerse()
		h.b.warn("verse %q in chapter %d skipped", num, h.chapter)
		return
	}
	h.b.startVerse(id, end)
}

// verseNumbers reads "5", "1-2" or "3a".
func verseNumbers(s string) (int, int) {
	a, b, ok := strings.Cut(strings.TrimSpace(s), "-")
	first := leadingInt(a)
	if !ok {
		return first, first
	}
	return first, leadingInt(b)
}

func leadingInt(s string) int {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return -1
	}
	return n
}

// para starts a paragraph-level style. It reports false for styles it does not know.
func (h *usfmHandler) para(style string) bool {
	if !usfmHeader[style] && !usfmHeading[style] && !usfmPara[style] {
		return false
	}
	h.endPara()
	switch {
	case usfmHeader[style]:
		h.mode = "header"
	case usfmHeading[style]:
		kind := ""
		if style == "d" {
			kind = "psalm"
		}
		h.b.openSpan("h", attr("k", kind), true, true)
		h.mode = "heading"
	case usfmPara[style]:
		if style != "nb" {
			h.b.paragraph()
		}
	}
	return true
}

func (h *usfmHandler) endPara() {
	if h.mode == "heading" {
		h.b.closeSpan("h")
	}
	h.mode = ""
}

func (h *usfmHandler) noteOpen(style string) {
	h.note = "f"
	if strings.HasPrefix(style, "x") || style == "ex" {
		h.note = "x"
	}
	h.b.openSpan(h.note, "", true, false)
}

func (h *usfmHandler) noteClose() {
	if h.note != "" {
		h.b.closeSpan(h.note)
	}
	h.note, h.noteSkip = "", false
}

// charOpen starts a character style; attrs holds USFM 3 word attributes. It reports false for
// styles it does not know.
func (h *usfmHandler) charOpen(style string, attrs map[string]string) bool {
	switch {
	case h.note != "" && usfmNoteInner[style]:
		h.noteSkip = style == "fr" || style == "xo"
		return true
	case usfmSkipChar[style]:
		h.skipChar++
	case style == "w":
		h.word(attrs)
	case usfmCharTag[style] != "":
		h.b.openSpan(usfmCharTag[style], "", false, false)
	case usfmPassChar[style]:
	default:
		return false
	}
	h.chars = append(h.chars, style)
	return true
}

func (h *usfmHandler) word(attrs map[string]string) {
	var nums []string
	for _, s := range strings.FieldsFunc(attrs["strong"], func(r rune) bool { return r == ',' || r == ' ' }) {
		if n := normStrong(s); n != "" {
			nums = append(nums, n)
		}
	}
	h.b.strong(nums)
	h.b.openSpan("w", attr("s", strings.Join(nums, " "))+attr("m", attrs["x-morph"]), false, false)
}

func (h *usfmHandler) charClose(style string) {
	if h.note != "" && usfmNoteInner[style] {
		h.noteSkip = false
		return
	}
	for i := len(h.chars) - 1; i >= 0; i-- {
		if h.chars[i] != style {
			continue
		}
		h.chars = append(h.chars[:i], h.chars[i+1:]...)
		switch {
		case usfmSkipChar[style]:
			h.skipChar--
		case style == "w":
			h.b.closeSpan("w")
		case usfmCharTag[style] != "":
			h.b.closeSpan(usfmCharTag[style])
		}
		return
	}
}

func (h *usfmHandler) text(s string) {
	if h.mode == "header" || h.skipChar > 0 || h.noteSkip || h.book == 0 {
		return
	}
	h.b.addText(s)
}

// readUSFM parses one USFM file.
func readUSFM(src string, b *builder) {
	h := &usfmHandler{b: b}
	t := &usfmTokens{s: src}
	arg := ""   // marker waiting for its argument: c, v, id or a note caller
	var w *word // a \w being collected, whose attributes follow its text
	for {
		marker, closing, text, ok := t.next()
		if !ok {
			break
		}
		if marker == "" {
			text = takeArg(h, &arg, text)
			if w != nil {
				w.text.WriteString(text)
				continue
			}
			h.text(stripAttrs(text, h.chars))
			continue
		}
		if w != nil {
			w.flush(h, marker == "w" && closing)
			w = nil
			if marker == "w" && closing {
				continue
			}
		}
		if arg != "" {
			takeArg(h, &arg, "")
		}
		switch {
		case closing && (marker == "f" || marker == "fe" || marker == "x" || marker == "ef" || marker == "ex"):
			h.noteClose()
		case closing:
			h.charClose(marker)
		case marker == "id" || marker == "c" || marker == "v":
			arg = marker
		case marker == "f" || marker == "fe" || marker == "x" || marker == "ef" || marker == "ex":
			h.noteOpen(marker)
			arg = "caller"
		case marker == "w":
			w = &word{}
		case strings.HasSuffix(marker, "-s") || strings.HasSuffix(marker, "-e") || marker == "ts":
		case h.para(marker), h.charOpen(marker, nil):
		default:
			b.warn("unknown USFM marker \\%s kept as plain text", marker)
		}
	}
	if w != nil {
		w.flush(h, false)
	}
	h.endPara()
	b.endVerse()
	b.closeAll()
}

// takeArg gives a waiting marker the first word of text and returns the rest.
func takeArg(h *usfmHandler, arg *string, text string) string {
	if *arg == "" {
		return text
	}
	text = strings.TrimLeft(text, " \t\r\n")
	i := strings.IndexAny(text, " \t\r\n")
	if i < 0 {
		i = len(text)
	}
	val, rest := text[:i], text[i:]
	switch *arg {
	case "id":
		h.setBook(val)
	case "c":
		h.setChapter(val)
	case "v":
		h.startVerse(val)
	}
	*arg = ""
	return strings.TrimPrefix(rest, " ")
}

// stripAttrs drops "|attributes" before the end of an open character style.
func stripAttrs(text string, open []string) string {
	if len(open) == 0 {
		return text
	}
	if i := strings.IndexByte(text, '|'); i >= 0 {
		return text[:i]
	}
	return text
}

type word struct{ text strings.Builder }

// flush writes a collected \w; complete says its closing marker was reached.
func (w *word) flush(h *usfmHandler, complete bool) {
	text, attrs := w.text.String(), map[string]string{}
	if i := strings.IndexByte(text, '|'); i >= 0 && complete {
		attrs = parseAttrs(text[i+1:])
		text = text[:i]
	}
	h.charOpen("w", attrs)
	h.text(text)
	if complete {
		h.charClose("w")
	}
}

// parseAttrs reads USFM 3 attributes: key="value" pairs, or a bare default lemma.
func parseAttrs(s string) map[string]string {
	m := map[string]string{}
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "=") {
		m["lemma"] = s
		return m
	}
	for s != "" {
		k, rest, ok := strings.Cut(s, `="`)
		if !ok {
			break
		}
		v, after, _ := strings.Cut(rest, `"`)
		m[strings.TrimSpace(k)] = v
		s = strings.TrimSpace(after)
	}
	return m
}

// usfmTokens splits USFM into markers and text.
type usfmTokens struct {
	s string
	i int
}

// next returns a marker (without "\" or "+"), whether it is a closing marker, or a run of text.
func (t *usfmTokens) next() (marker string, closing bool, text string, ok bool) {
	if t.i >= len(t.s) {
		return "", false, "", false
	}
	if t.s[t.i] != '\\' {
		j := strings.IndexByte(t.s[t.i:], '\\')
		if j < 0 {
			j = len(t.s) - t.i
		}
		text = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "~", " ", "//", "").Replace(t.s[t.i : t.i+j])
		t.i += j
		return "", false, text, true
	}
	j := t.i + 1
	if j < len(t.s) && t.s[j] == '+' {
		j++
	}
	start := j
	for j < len(t.s) && isMarkerChar(t.s[j]) {
		j++
	}
	marker = t.s[start:j]
	if j < len(t.s) && t.s[j] == '*' {
		closing = true
		j++
	} else if j < len(t.s) && (t.s[j] == ' ' || t.s[j] == '\n' || t.s[j] == '\t') {
		j++
	} else if j+1 < len(t.s) && t.s[j] == '\r' && t.s[j+1] == '\n' {
		j += 2
	}
	t.i = j
	if strings.HasSuffix(marker, "-s") || strings.HasSuffix(marker, "-e") {
		t.skipMilestone()
	}
	if marker == "" {
		return "", false, "\\", true
	}
	return marker, closing, "", true
}

// skipMilestone passes over a milestone's attributes up to its closing "\*".
func (t *usfmTokens) skipMilestone() {
	rest := t.s[t.i:]
	if end := strings.Index(rest, `\*`); end >= 0 && end == strings.IndexByte(rest, '\\') {
		t.i += end + 2
	}
}

func isMarkerChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-'
}
