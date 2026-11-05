package assistant

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Perlsowisdom/xegesis/internal/licence"
)

func lic(t *testing.T, ty licence.Type) licence.Licence {
	t.Helper()
	l, err := licence.Defaults(ty, "")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestGateOpenTextGoesFreely(t *testing.T) {
	long := strings.Repeat("word ", 5000)
	for _, g := range []Gate{{}, {Local: true}} {
		if r := g.Work("KJV", lic(t, licence.PublicDomain), long); r.Text != long || r.Shortened || r.Refused {
			t.Fatalf("public domain changed: %+v", r.Notice)
		}
		if r := g.Book("Open book", lic(t, licence.PublicDomain), long); r.Text != long {
			t.Fatal("open book changed")
		}
	}
}

func TestGateShortensToQuoteLimit(t *testing.T) {
	l := lic(t, licence.PersonalPurchase) // a percentage limit: one page of quotation
	long := strings.Repeat("grace ", 1000)
	r := Gate{}.Work("NETfree", l, long)
	if !r.Shortened || r.Refused || utf8.RuneCountInString(r.Text) > QuoteLimit(l)+2 || !strings.Contains(r.Notice, "NETfree") {
		t.Fatalf("got %d chars, %+v", utf8.RuneCountInString(r.Text), r.Notice)
	}
	l.Export = &licence.Limit{Pages: 2}
	if QuoteLimit(l) != 4000 {
		t.Fatal(QuoteLimit(l))
	}
	short := "For God so loved the world"
	if r := (Gate{}).Work("X", l, short); r.Text != short || r.Shortened {
		t.Fatal("short text was changed")
	}
}

func TestGateRefusesWhatMayNotBeKept(t *testing.T) {
	l := lic(t, licence.PublicDomain)
	l.Store = false
	if r := (Gate{Local: true}).Work("Live", l, "text"); !r.Refused || r.Text != "" {
		t.Fatalf("%+v", r)
	}
}

func TestGatePersonalCopiesOnlyToLocal(t *testing.T) {
	for _, ty := range []licence.Type{licence.PersonalPurchase, licence.Unknown} {
		l := lic(t, ty)
		if !Personal(l) {
			t.Fatalf("%s is not personal", ty)
		}
		r := Gate{Local: false}.Book("My Commentary", l, "some text")
		if !r.Refused || r.Text != "" || !strings.Contains(r.Notice, "personal copy") {
			t.Fatalf("online: %+v", r)
		}
		r = Gate{Local: true}.Book("My Commentary", l, "some text")
		if r.Refused || r.Text != "some text" {
			t.Fatalf("local: %+v", r)
		}
	}
	if Personal(lic(t, licence.CreativeCommons)) {
		t.Fatal("CC is not personal")
	}
}

func TestLocalMeansThisComputer(t *testing.T) {
	loc, _ := preset("local")
	cus, _ := preset("custom")
	oai, _ := preset("openai")
	cases := []struct {
		p    Preset
		addr string
		want bool
	}{
		{loc, "http://localhost:11434/v1", true},
		{loc, "http://127.0.0.1:1234/v1", true},
		{loc, "http://192.168.1.5:11434/v1", false},
		{cus, "http://[::1]:8080/v1", true},
		{cus, "https://example.com/v1", false},
		{oai, "http://127.0.0.1:9999/v1", false}, // an online provider stays online wherever it points
	}
	for _, c := range cases {
		if got := local(c.p, c.addr); got != c.want {
			t.Errorf("%s %s: %v", c.p.ID, c.addr, got)
		}
	}
}
