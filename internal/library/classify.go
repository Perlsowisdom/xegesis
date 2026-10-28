package library

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// The book types, in plain words. They are stored as written.
const (
	TypeBible       = "Bible"
	TypeCommentaryO = "Commentary (Old Testament)"
	TypeCommentaryN = "Commentary (New Testament)"
	TypeCommentaryW = "Commentary (whole Bible)"
	TypeDictionary  = "Dictionary or encyclopedia"
	TypeLexicon     = "Lexicon or grammar"
	TypeTheology    = "Theology"
	TypeHistory     = "Church history"
	TypeDevotional  = "Devotional"
	TypeSermons     = "Sermons"
	TypeBiography   = "Biography"
	TypeMonograph   = "Monograph (other)"
	TypeFiction     = "Fiction"
	TypeOther       = "Other"
)

// BookTypes lists the types a book can have. Monograph and Other are never guessed; they are for
// the user to choose.
var BookTypes = []string{TypeBible, TypeCommentaryO, TypeCommentaryN, TypeCommentaryW, TypeDictionary,
	TypeLexicon, TypeTheology, TypeHistory, TypeDevotional, TypeSermons, TypeBiography, TypeMonograph,
	TypeFiction, TypeOther}

// DefaultTraditions is the starting list of traditions. The user can add and remove names.
var DefaultTraditions = []string{"Anglican", "Baptist", "Catholic", "Eastern Orthodox", "Evangelical",
	"Jewish", "Lutheran", "Methodist", "Pentecostal", "Presbyterian", "Reformed"}

// ClassifyInput is what the classifier knows about a book.
type ClassifyInput struct {
	Kind     string
	Title    string
	Subtitle string
	Series   string
	Subjects []string
	Folders  []string // names of the folders the book's file is in
}

// Suggestion is a guessed type with how sure the guess is and why, in plain words.
type Suggestion struct {
	Value      string
	Confidence float64
	Reason     string
	Origin     string // metadata, or bisac when a subject code decided it
}

var kindTypes = map[string]string{"bible": TypeBible, "commentary": TypeCommentaryW, "dictionary": TypeDictionary,
	"lexicon": TypeLexicon}

type keyword struct {
	re    *regexp.Regexp
	value string
	conf  float64
	say   string // how the reason names what was found
}

func kw(re, value string, conf float64, say string) keyword {
	return keyword{regexp.MustCompile(`(?i)\b(?:` + re + `)`), value, conf, say}
}

var keywords = []keyword{
	kw(`commentar(?:y|ies)\b`, TypeCommentaryW, 0.8, "a commentary"),
	kw(`exposition\b|expositor`, TypeCommentaryW, 0.55, "an exposition"),
	kw(`dictionar(?:y|ies)\b|encyclop(?:a)?edia`, TypeDictionary, 0.85, "a dictionary or encyclopedia"),
	kw(`concordance`, TypeDictionary, 0.7, "a concordance"),
	kw(`lexicon|grammar\b|syntax\b`, TypeLexicon, 0.85, "a lexicon or grammar"),
	kw(`sermons?\b`, TypeSermons, 0.75, "sermons"),
	kw(`devotional|daily readings|meditations\b`, TypeDevotional, 0.7, "devotions"),
	kw(`church history|history of the church|history of christianity|ecclesiastical history`, TypeHistory, 0.75, "church history"),
	kw(`biograph|memoirs?\b`, TypeBiography, 0.7, "a biography"),
	kw(`novel\b|novels\b|fiction\b`, TypeFiction, 0.75, "fiction"),
	kw(`theolog|doctrine|dogmatics|systematic`, TypeTheology, 0.65, "theology"),
}

// source says where a clue was found and how much to trust it.
type source int

const (
	fromSubject source = iota
	fromSeries
	fromTitle
	fromSubtitle
	fromFolder
)

var trust = map[source]float64{fromSubject: 1.15, fromSeries: 1.0, fromTitle: 1.0, fromSubtitle: 0.9, fromFolder: 0.7}

func (s source) reason(text, say string) string {
	switch s {
	case fromSubject:
		return fmt.Sprintf("the subject %q points to %s", text, say)
	case fromFolder:
		return fmt.Sprintf("it is in a folder named %q, which suggests %s", text, say)
	case fromSeries:
		return "series name mentions " + say
	case fromSubtitle:
		return "subtitle mentions " + say
	}
	return "title mentions " + say
}

// ClassifyType suggests a type for a book from its details. It returns false when nothing in them
// points to a type.
func ClassifyType(in ClassifyInput) (Suggestion, bool) {
	if t := kindTypes[in.Kind]; t != "" {
		return Suggestion{t, 1, "it was added as a " + in.Kind, "metadata"}, true
	}
	var best Suggestion
	have := false
	consider := func(s Suggestion) {
		if s.Value != "" && (!have || s.Confidence > best.Confidence) {
			best, have = s, true
		}
	}
	for _, s := range in.Subjects {
		consider(bisacType(s))
		consider(lcshType(s))
		consider(keywordType(s, fromSubject, in))
	}
	consider(bibleTitle(in))
	consider(keywordType(in.Series, fromSeries, in))
	consider(keywordType(in.Title, fromTitle, in))
	consider(keywordType(in.Subtitle, fromSubtitle, in))
	for _, f := range in.Folders {
		consider(keywordType(f, fromFolder, in))
	}
	return best, have
}

func keywordType(text string, src source, in ClassifyInput) Suggestion {
	text = strings.TrimSpace(text)
	if text == "" {
		return Suggestion{}
	}
	for _, k := range keywords {
		if !k.re.MatchString(text) {
			continue
		}
		s := Suggestion{Value: k.value, Confidence: k.conf * trust[src], Origin: "metadata"}
		say := k.say
		if k.value == TypeCommentaryW {
			var scoped bool
			s.Value, say, scoped = commentaryScope(text, in)
			if scoped {
				s.Confidence += 0.1
			}
		}
		s.Confidence = math.Round(min(s.Confidence, 0.95)*100) / 100
		s.Reason = src.reason(text, say)
		return s
	}
	return Suggestion{}
}

var (
	oldTestament = regexp.MustCompile(`(?i)\bold testament\b|\bbible\.?\s*o\.?t\b|\bhebrew bible\b`)
	newTestament = regexp.MustCompile(`(?i)\bnew testament\b|\bbible\.?\s*n\.?t\b`)
	stopWords    = map[string]bool{"the": true, "a": true, "an": true, "is": true, "am": true, "on": true, "of": true,
		"to": true, "in": true, "and": true, "or": true, "for": true, "by": true, "as": true, "so": true}
)

// commentaryScope narrows "a commentary" to the Old or New Testament, or to one book, from the
// title, series, subtitle and subjects. It returns the type, the words for the reason and whether
// the scope was found.
func commentaryScope(text string, in ClassifyInput) (string, string, bool) {
	for _, t := range []string{in.Title, in.Subtitle, in.Series, text} {
		if v, name := bookIn(t); name != "" {
			return v, "a commentary on " + name, true
		}
	}
	for _, t := range append([]string{in.Series, in.Title, in.Subtitle, text}, in.Subjects...) {
		switch {
		case oldTestament.MatchString(t):
			return TypeCommentaryO, "a commentary on the Old Testament", true
		case newTestament.MatchString(t):
			return TypeCommentaryN, "a commentary on the New Testament", true
		}
	}
	return TypeCommentaryW, "a commentary", false
}

func testamentType(b verse.Book) string {
	switch {
	case b.Number <= 39:
		return TypeCommentaryO
	case b.Number <= 66:
		return TypeCommentaryN
	}
	return TypeCommentaryW // the extra books between the Testaments
}

// bookIn finds the first Bible book named in text, looking after the word "commentary" first so
// that an author's name ("John Calvin's Commentary on Romans") is not taken for a book.
func bookIn(text string) (string, string) {
	order := []string{text}
	if i := strings.Index(strings.ToLower(text), "commentar"); i >= 0 {
		order = []string{text[i:], text[:i]}
	}
	for _, part := range order {
		words := strings.FieldsFunc(part, func(r rune) bool {
			return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127)
		})
		for i := range words {
			for n := min(3, len(words)-i); n >= 1; n-- {
				cand := strings.Join(words[i:i+n], " ")
				if len(cand) < 3 || n == 1 && stopWords[strings.ToLower(cand)] {
					continue
				}
				if b, ok := verse.LookupBook(cand); ok {
					return testamentType(b), b.Name
				}
			}
		}
	}
	return "", ""
}

var (
	lcshBible  = regexp.MustCompile(`(?i)^bible\b`)
	lcshTexts  = regexp.MustCompile(`(?i)^bible\.\s*(?:english|greek|hebrew|latin|german|spanish|french)\b|--\s*(?:versions?|translations?)\b`)
	justBible  = regexp.MustCompile(`(?i)^(?:the )?bible$|^(?:the )?(?:new|old) testament$`)
	notBible   = regexp.MustCompile(`(?i)\b(?:study|commentary|dictionary|handbook|introduction|story|stories|guide|concordance|history|companion|reading)\b`)
	versionRe  = regexp.MustCompile(`(?i)\b(?:holy bible|english standard version|king james|new international version|new american standard|new revised standard|revised standard version|american standard version|new living translation|christian standard bible|world english bible|douay-rheims)\b`)
	bisacRe    = regexp.MustCompile(`\b([A-Z]{3})\d{6}\b`)
	bisacTypes = map[string]string{"FIC": TypeFiction, "BIO": TypeBiography, "BIB": TypeBible}
)

// lcshType reads Library of Congress style subjects, such as "Bible. Romans -- Commentaries".
func lcshType(subject string) Suggestion {
	if !lcshBible.MatchString(subject) {
		return Suggestion{}
	}
	why := func(what string) string { return fmt.Sprintf("the subject %q says it is %s", subject, what) }
	if strings.Contains(strings.ToLower(subject), "commentar") {
		if v, name := bookIn(strings.SplitN(subject, "--", 2)[0]); name != "" {
			return Suggestion{v, 0.92, why("a commentary on " + name), "metadata"}
		}
		switch {
		case oldTestament.MatchString(subject):
			return Suggestion{TypeCommentaryO, 0.9, why("a commentary on the Old Testament"), "metadata"}
		case newTestament.MatchString(subject):
			return Suggestion{TypeCommentaryN, 0.9, why("a commentary on the New Testament"), "metadata"}
		}
		return Suggestion{TypeCommentaryW, 0.8, why("a commentary on the Bible"), "metadata"}
	}
	if lcshTexts.MatchString(subject) {
		return Suggestion{TypeBible, 0.8, why("a Bible text"), "metadata"}
	}
	return Suggestion{}
}

func bibleTitle(in ClassifyInput) Suggestion {
	t := strings.TrimSpace(in.Title)
	if notBible.MatchString(t) || notBible.MatchString(in.Subtitle) {
		return Suggestion{}
	}
	if m := versionRe.FindString(t); m != "" {
		return Suggestion{TypeBible, 0.85, fmt.Sprintf("title names a Bible (%q)", m), "metadata"}
	}
	if justBible.MatchString(t) {
		return Suggestion{TypeBible, 0.8, "title is the Bible or a Testament", "metadata"}
	}
	return Suggestion{}
}

// bisacType reads subject codes. Only the major headings of the publishing trade's subject codes
// are mapped, and only those whose meaning is certain: FIC is fiction, BIO is biography and BIB is
// Bibles. The full list needs a licence, so finer codes are not guessed at.
func bisacType(subject string) Suggestion {
	m := bisacRe.FindStringSubmatch(subject)
	if m == nil || bisacTypes[m[1]] == "" {
		return Suggestion{}
	}
	t := bisacTypes[m[1]]
	return Suggestion{t, 0.9, fmt.Sprintf("its subject code %s is in the %s section", m[0], strings.ToLower(t)), "bisac"}
}
