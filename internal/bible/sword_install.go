package bible

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// InstallSWORD installs one SWORD module whose data files are in dir, as described by c. It is the
// one installer behind both bible.get (files downloaded from a repository) and bible.install (a
// folder or .zip the user chose). phase, when not nil, is told "converting" before the slow step.
// A locked module (CipherKey) is refused; there is no way to unlock one.
func (l *Library) InstallSWORD(ctx context.Context, dir string, c sword.Conf, o InstallOptions, phase func(string)) (Meta, []string, error) {
	if c.Locked() {
		return Meta{}, nil, sword.ErrLocked
	}
	say := func(s string) {
		if phase != nil {
			phase(s)
		}
	}
	if c.Kind() == "dictionary" {
		return l.installSWORDDict(ctx, dir, c, o, say)
	}
	if c.Kind() == "commentary" {
		o.Kind = KindCommentary
	}
	text, err := sword.Open(dir, c)
	if err != nil {
		return Meta{}, nil, err
	}
	say("converting")
	o.Versification = string(text.System())
	o.Mappings = englishRows(text.Versification())
	return l.InstallEntries(ctx, o, EntryMarkup(c.Get("SourceType")), func(emit func(Entry) error) error {
		return text.Walk(func(id verse.ID, intro bool, markup string) error {
			return emit(Entry{ID: id, Intro: intro, Markup: markup})
		})
	})
}

// englishRows turns a SWORD numbering's english table into module mapping rows.
func englishRows(v *sword.Versification) []Mapping {
	pairs := v.EnglishMap()
	out := make([]Mapping, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, Mapping{System: verse.English, Module: p.Module, SystemID: p.English})
	}
	return out
}

// installSWORDDict installs a SWORD dictionary. Modules marked as Strong's dictionaries
// (Feature=GreekDef or HebrewDef), whose keys are numbers, become lexicons keyed by Strong's number.
func (l *Library) installSWORDDict(ctx context.Context, dir string, c sword.Conf, o InstallOptions, say func(string)) (Meta, []string, error) {
	d, err := sword.OpenDict(dir, c)
	if err != nil {
		return Meta{}, nil, err
	}
	prefix := ""
	for _, f := range c.All("Feature") {
		switch strings.TrimSpace(f) {
		case "GreekDef":
			prefix = "G"
		case "HebrewDef":
			prefix = "H"
		}
	}
	say("converting")
	var heads []Headword
	err = d.Walk(func(k, text string) error {
		h := DictMarkup(text, c.Get("SourceType"))
		h.Key = k
		if prefix != "" {
			num := strings.TrimLeft(strings.TrimRight(k, `\ `), "0")
			switch {
			case num != "" && strings.Trim(num, "0123456789") == "":
				h.Strong = prefix + num
			case strings.Trim(k, "0123456789 ") == "" || strings.EqualFold(k, "intro"):
				return nil // the introduction and other non-word keys
			default:
				// A lexicon keyed by original-language headwords (Abbott-Smith, for one); its
				// Strong's-keyed companion points here with "@LINK <headword>".
				if h.Lemma == "" {
					h.Lemma = strings.ToLower(k) // keys are often capitals: ἈΓΑΠΆΩ becomes ἀγαπάω
				}
			}
		}
		heads = append(heads, h)
		return nil
	})
	if err != nil {
		return Meta{}, nil, err
	}
	o.Kind = KindDictionary
	if prefix != "" {
		o.Kind = KindLexicon
	}
	m, err := l.InstallHeadwords(ctx, o, heads)
	return m, nil, err
}

// SWORDRoot returns the folder holding mods.d when dir is a SWORD module folder (or has one
// wrapper folder around it), else "".
func SWORDRoot(dir string) string {
	root := contentDir(dir)
	for _, d := range []string{dir, root} {
		if m, _ := filepath.Glob(filepath.Join(d, "mods.d", "*.conf")); len(m) > 0 {
			return d
		}
	}
	return ""
}

// installSWORDFolder installs every module described in root/mods.d. The user's licence, when
// given, applies to all of them; otherwise each module's own .conf decides, and a copyrighted
// text is a personal copy. Modules that cannot be installed are reported in the error, after the
// others have been installed.
func (c cmds) installSWORDFolder(ctx context.Context, root string, p InstallParams, user *licence.Licence) (InstallResult, error) {
	files, _ := filepath.Glob(filepath.Join(root, "mods.d", "*.conf"))
	sort.Strings(files)
	var res InstallResult
	res.Warnings = []string{}
	var problems []string
	for _, f := range files {
		conf, err := readConf(f)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", filepath.Base(f), err))
			continue
		}
		if conf.Locked() {
			problems = append(problems, fmt.Sprintf("%s is locked: it needs the publisher's unlock key, which Xegesis does not support", firstNonEmpty(conf.Get("Description"), conf.Name)))
			continue
		}
		o := swordOptions(conf, p, len(files) == 1, user)
		o.Replace = p.Replace
		m, warns, err := c.l.InstallSWORD(ctx, filepath.Join(root, filepath.FromSlash(conf.DataPath())), conf, o, nil)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", firstNonEmpty(conf.Get("Description"), conf.Name), err))
			continue
		}
		res.Modules = append(res.Modules, m)
		res.Warnings = append(res.Warnings, warns...)
	}
	if len(res.Modules) == 0 {
		if len(problems) == 0 {
			problems = []string{"no SWORD modules found"}
		}
		return res, errors.New(strings.Join(problems, "; "))
	}
	res.Module = res.Modules[0]
	for _, pr := range problems {
		res.Warnings = append(res.Warnings, "Not installed: "+pr)
	}
	return res, nil
}

func readConf(path string) (sword.Conf, error) {
	f, err := os.Open(path)
	if err != nil {
		return sword.Conf{}, err
	}
	defer f.Close()
	return sword.ParseConf(f)
}

// swordOptions fills the install options for one module from its .conf and the caller's overrides.
// Name, ID and the other descriptive fields are only taken from the caller for a single module.
func swordOptions(c sword.Conf, p InstallParams, single bool, user *licence.Licence) InstallOptions {
	o := InstallOptions{Name: firstNonEmpty(c.Get("Description"), c.Name),
		Abbreviation: firstNonEmpty(c.Get("Abbreviation"), c.Name), Language: swordLang(c.Get("Lang")),
		Version: c.Get("Version"), Source: p.Source}
	if single {
		o.ID = p.ID
		o.Name = firstNonEmpty(p.Name, o.Name)
		o.Abbreviation = firstNonEmpty(p.Abbreviation, o.Abbreviation)
		o.Language = firstNonEmpty(p.Language, o.Language)
		o.Version = firstNonEmpty(p.Version, o.Version)
	}
	if o.ID == "" {
		o.ID = slug(c.Name)
	}
	if user != nil {
		o.Licence = *user
	} else {
		o.Licence = SwordInstalledLicence(c)
	}
	return o
}

func swordLang(s string) string {
	if validLang.MatchString(s) {
		return s
	}
	return ""
}

// SwordInstalledLicence is the licence for a module the user installs themselves: the one its
// .conf states when that is clear and open, else a personal copy that keeps the credit line.
func SwordInstalledLicence(c sword.Conf) licence.Licence {
	m := SwordLicence(c)
	if m.Clear && (m.Licence.Type != licence.OpenOther || m.Licence.Share) {
		return m.Licence
	}
	l := licence.Default()
	l.RightsHolder = m.Licence.RightsHolder
	l.Attribution = m.Licence.Attribution
	return l
}
