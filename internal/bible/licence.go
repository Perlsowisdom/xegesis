package bible

import (
	"bytes"
	"html"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/licence"
)

// headBytes is how much of each file is read for licence statements; they sit in the header.
const headBytes = 64 << 10

var (
	osisRights    = regexp.MustCompile(`(?is)<rights[^>]*>(.*?)</rights>`)
	usxRem        = regexp.MustCompile(`(?is)<para[^>]*style="rem"[^>]*>(.*?)</para>`)
	usfmMarker    = regexp.MustCompile(`^\\[a-z]+[0-9]*\*?\s*`)
	copyrightWord = regexp.MustCompile(`(?i)copyright|©`)
)

// DetectLicence returns the licence a Bible's own files state, read from an eBible.org copr.htm, a
// CrossWire .conf, an OSIS <rights> element, or USFM \rem and copyright lines. A file that states
// none, or states one in words Xegesis cannot read, is a personal copy; any copyright line it has
// becomes the credit shown with copies.
func DetectLicence(path string) licence.Licence {
	statements := rightsStatements(path)
	for _, s := range statements {
		if l, ok := licence.FromRights(s); ok {
			if l.Type != licence.PublicDomain {
				l.Attribution = copyrightLine(statements)
			}
			return l
		}
	}
	l := licence.Default()
	l.Attribution = copyrightLine(statements)
	return l
}

// rightsStatements lists what the files say about their rights, most authoritative first.
func rightsStatements(path string) []string {
	var pages, confs, texts []string
	visit := func(p string) {
		name := strings.ToLower(filepath.Base(p))
		switch {
		case strings.HasPrefix(name, "copr") && (strings.HasSuffix(name, ".htm") || strings.HasSuffix(name, ".html")):
			if b := head(p); b != nil {
				pages = append(pages, string(b))
			}
		case strings.HasSuffix(name, ".conf"):
			confs = append(confs, confRights(p)...)
		case detect(name, nil) != "":
			texts = append(texts, textRights(p)...)
		}
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				visit(p)
			}
			return nil
		})
	} else {
		visit(path)
	}
	return append(append(pages, confs...), texts...)
}

func head(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, headBytes))
	return bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
}

func confRights(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	c, err := sword.ParseConf(f)
	if err != nil {
		return nil
	}
	var out []string
	for _, k := range []string{"DistributionLicense", "Copyright", "ShortCopyright"} {
		if v := strings.TrimSpace(sword.PlainRTF(c.Get(k))); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// textRights reads OSIS <rights>, USX rem paragraphs, and USFM \rem lines and lines that speak of
// copyright before the first chapter.
func textRights(path string) []string {
	b := head(path)
	var out []string
	add := func(s string) {
		if s = strings.Join(strings.Fields(html.UnescapeString(stripTags(s))), " "); s != "" {
			out = append(out, s)
		}
	}
	switch detect(path, b) {
	case "osis":
		for _, m := range osisRights.FindAllSubmatch(b, -1) {
			add(string(m[1]))
		}
	case "usx":
		for _, m := range usxRem.FindAllSubmatch(b, -1) {
			add(string(m[1]))
		}
	case "usfm":
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, `\c `) {
				break
			}
			if strings.HasPrefix(line, `\rem `) || speaksOfRights(line) {
				add(usfmMarker.ReplaceAllString(line, ""))
			}
		}
	}
	return out
}

func speaksOfRights(s string) bool {
	l := strings.ToLower(s)
	for _, w := range []string{"copyright", "©", "public domain", "creative commons", "licen"} {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

// copyrightLine is the first statement naming a copyright, short enough to print with a quotation.
func copyrightLine(statements []string) string {
	for _, s := range statements {
		s = strings.Join(strings.Fields(html.UnescapeString(stripTags(s))), " ")
		loc := copyrightWord.FindStringIndex(s)
		if loc == nil {
			continue
		}
		s = s[loc[0]:]
		if end := strings.IndexAny(s, ".\n"); end > 0 {
			s = s[:end]
		}
		if r := []rune(s); len(r) > 200 {
			s = string(r[:200])
		}
		return strings.TrimSpace(s)
	}
	return ""
}
