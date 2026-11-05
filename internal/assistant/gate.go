package assistant

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/reader"
)

// Gate decides how much of a work's text may reach the model. Every tool that returns text from
// a Bible, a reference work or a library book passes it through here. The rules are those for
// quoting into a note (document.quote): open text goes freely, limited text only up to its quoting
// limit, and a work whose licence forbids keeping a copy not at all. On top of that, the text of a
// personal copy of a library book goes only to a model on this computer.
type Gate struct {
	Local bool // the provider runs on this computer
}

// Gated is text after the gate.
type Gated struct {
	Text      string `json:"text"`
	Shortened bool   `json:"shortened,omitempty"`
	Refused   bool   `json:"refused,omitempty"`
	Notice    string `json:"notice,omitempty" desc:"What the gate did, in words, for the model and the person"`
}

// QuoteLimit is the most characters of a work one result may hold, or 0 for no limit; the same
// measure document.quote uses.
func QuoteLimit(l licence.Licence) int {
	if l.Export == nil {
		return 0
	}
	if l.Export.Pages > 0 {
		return l.Export.Pages * reader.CharsPerPage
	}
	return reader.CharsPerPage
}

// Work gates the text of a Bible or reference work.
func (g Gate) Work(name string, l licence.Licence, text string) Gated {
	if !l.Store {
		return Gated{Refused: true, Notice: fmt.Sprintf("Not sent: the licence of %s does not allow keeping a copy of its text.", name)}
	}
	return limit(name, l, text)
}

// Book gates the text of a library book.
func (g Gate) Book(title string, l licence.Licence, text string) Gated {
	if Personal(l) && !g.Local {
		return Gated{Refused: true, Notice: fmt.Sprintf("Not sent: %q is a personal copy, and its text goes only to a model on this computer.", title)}
	}
	if !l.Store {
		return Gated{Refused: true, Notice: fmt.Sprintf("Not sent: the licence of %q does not allow keeping a copy of its text.", title)}
	}
	return limit(title, l, text)
}

// Personal reports whether a licence makes a work a personal copy (unknown licences count).
func Personal(l licence.Licence) bool { return l.Plain() == "Personal copy" }

func limit(name string, l licence.Licence, text string) Gated {
	max := QuoteLimit(l)
	if max == 0 || utf8.RuneCountInString(text) <= max {
		return Gated{Text: text}
	}
	return Gated{Text: cut(text, max), Shortened: true,
		Notice: fmt.Sprintf("Shortened to about %d characters, the quoting limit of %s's licence. Ask for fewer verses or a smaller part to read more.", max, name)}
}

// cut keeps at most max characters, ending at a word break when one is near.
func cut(s string, max int) string {
	n := 0
	for i := range s {
		if n == max {
			out := s[:i]
			if j := strings.LastIndexAny(out, " \n"); j > len(out)*3/4 {
				out = out[:j]
			}
			return strings.TrimRight(out, " \n") + " …"
		}
		n++
	}
	return s
}

// Budget shortens text the gate has already passed, such as a long note, so one result cannot
// fill the model's context.
func Budget(text string, max int) (string, bool) {
	if utf8.RuneCountInString(text) <= max {
		return text, false
	}
	return cut(text, max), true
}
