package repos

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/bible"
	"github.com/Perlsowisdom/xegesis/internal/licence"
)

// KindData is a source of openly licensed datasets that Xegesis knows by name: STEPBible's
// tagged texts and lexicons and OpenBible.info's cross-references. Their items are listed
// without downloading a catalogue.
const KindData = "data"

const (
	DefaultSTEPBible = "https://raw.githubusercontent.com/STEPBible/STEPBible-Data/master"
	DefaultOpenBible = "https://a.openbible.info/data"
)

// dataset is one built-in item: its files (relative to the source URL) and how to install them.
type dataset struct {
	source, id, name, abbr, lang, kind string
	about, rights, attribution         string
	files                              []string
	size                               int64
	install                            func(ctx context.Context, s *Service, o bible.InstallOptions, paths []string) (bible.Meta, []string, error)
}

const stepRights = "STEPBible Data (github.com/STEPBible/STEPBible-Data): \"Data created by www.STEPBible.org based on work " +
	"at Tyndale House Cambridge (CC BY 4.0)\". \"This licence allows you to: Include any part of this data in software or " +
	"publications without requesting permission\"; \"Refer others to github.com/STEPBible as the source of the data. " +
	"Please do not redistribute it yourself.\""

var datasets = []dataset{
	{source: "stepbible", id: "TAHOT", name: "Hebrew Old Testament with Strong's numbers (STEPBible TAHOT)", abbr: "TAHOT",
		lang: "hbo", kind: "bible",
		about: "The Leningrad Codex text with every word tagged with its extended Strong's number, morphology and an " +
			"English gloss, in English verse numbering. Translators Amalgamated Hebrew OT by STEPBible.org.",
		rights: stepRights, attribution: "Translators Amalgamated Hebrew OT (TAHOT), STEPBible.org, based on work at Tyndale House Cambridge, CC BY 4.0",
		files: []string{
			"Translators Amalgamated OT+NT/TAHOT Gen-Deu - Translators Amalgamated Hebrew OT - STEPBible.org CC BY.txt",
			"Translators Amalgamated OT+NT/TAHOT Jos-Est - Translators Amalgamated Hebrew OT - STEPBible.org CC BY.txt",
			"Translators Amalgamated OT+NT/TAHOT Job-Sng - Translators Amalgamated Hebrew OT - STEPBible.org CC BY.txt",
			"Translators Amalgamated OT+NT/TAHOT Isa-Mal - Translators Amalgamated Hebrew OT - STEPBible.org CC BY.txt",
		}, size: 70208913, install: installSTEPText},
	{source: "stepbible", id: "TAGNT", name: "Greek New Testament with Strong's numbers (STEPBible TAGNT)", abbr: "TAGNT",
		lang: "grc", kind: "bible",
		about: "The Greek New Testament (the words of the Nestle-Aland based editions) with every word tagged with its " +
			"Strong's number, morphology and an English gloss. Translators Amalgamated Greek NT by STEPBible.org.",
		rights: stepRights, attribution: "Translators Amalgamated Greek NT (TAGNT), STEPBible.org, based on work at Tyndale House Cambridge, CC BY 4.0",
		files: []string{
			"Translators Amalgamated OT+NT/TAGNT Mat-Jhn - Translators Amalgamated Greek NT - STEPBible.org CC-BY.txt",
			"Translators Amalgamated OT+NT/TAGNT Act-Rev - Translators Amalgamated Greek NT - STEPBible.org CC-BY.txt",
		}, size: 30128964, install: installSTEPText},
	{source: "stepbible", id: "TBESG", name: "Greek lexicon (STEPBible brief lexicon, Abbott-Smith)", abbr: "TBESG",
		lang: "en", kind: "dictionary",
		about: "Brief definitions for every Greek Strong's number, from Abbott-Smith's Manual Greek Lexicon of the New " +
			"Testament, edited by Tyndale House to match the extended Strong's numbers.",
		rights: stepRights, attribution: "Translators Brief lexicon of Extended Strongs for Greek (TBESG), STEPBible.org, based on work at Tyndale House Cambridge, CC BY 4.0",
		files: []string{"Lexicons/TBESG - Translators Brief lexicon of Extended Strongs for Greek - STEPBible.org CC BY.txt"},
		size:  4736912, install: installSTEPLexicon(true)},
	{source: "stepbible", id: "TBESH", name: "Hebrew word list (STEPBible brief lexicon, glosses)", abbr: "TBESH",
		lang: "en", kind: "dictionary",
		about: "The Hebrew word, transliteration, part of speech and a short English gloss for every Hebrew Strong's " +
			"number. The longer definitions in this file come from the Abridged BDB of Online Bible, which asks for " +
			"permission before use, so Xegesis keeps only the glosses written by Tyndale House.",
		rights: stepRights + "\nTBESH: \"Meaning: These are based on the Abridged BDB by Online Bible, © Larry Pierce of " +
			"OnlineBible.net. They are for guidance only. Permission should be gained from Online Bible before these are " +
			"applied in any project.\" (Not installed.)",
		attribution: "Translators Brief lexicon of Extended Strongs for Hebrew (TBESH), STEPBible.org, based on work at Tyndale House Cambridge, CC BY 4.0",
		files:       []string{"Lexicons/TBESH - Translators Brief lexicon of Extended Strongs for Hebrew - STEPBible.org CC BY.txt"},
		size:        3288045, install: installSTEPLexicon(false)},
	{source: "openbible", id: "cross-references", name: "Cross-references (OpenBible.info)", abbr: "OBXR",
		lang: "en", kind: "commentary",
		about: "About 340,000 cross-references between Bible verses, ranked by readers' votes. Xegesis keeps the 25 " +
			"best for each verse.",
		rights:      "OpenBible.info cross-references: \"licensed under a Creative Commons Attribution License\" (CC BY 4.0).",
		attribution: "Cross-references from OpenBible.info, CC BY 4.0",
		files:       []string{"cross-references.zip"}, size: 1983632, install: installOpenBible},
}

// dataSources are the built-in dataset sources.
func (s *Service) dataSources() []Source {
	return []Source{
		{ID: "stepbible", Name: "STEPBible", Kind: KindData, URL: s.stepbible},
		{ID: "openbible", Name: "OpenBible.info", Kind: KindData, URL: s.openbible},
	}
}

func (s *Service) dataItems(src Source) []Item {
	src.URL = s.stepbible
	if src.ID == "openbible" {
		src.URL = s.openbible
	}
	var out []Item
	for _, d := range datasets {
		if d.source != src.ID {
			continue
		}
		lic := must(licence.CreativeCommons, "CC-BY-4.0")
		lic.Attribution = d.attribution
		lic.URL = "https://creativecommons.org/licenses/by/4.0/"
		it := Item{Source: src.ID, SourceName: src.Name, ID: d.id, Name: d.name, Abbreviation: d.abbr, Kind: d.kind,
			Size: d.size, Licence: lic, LicenceClear: true, Rights: d.rights, About: d.about, Supported: true,
			Origin: strings.TrimRight(src.URL, "/") + "#" + d.id, base: src.URL}
		it.Language, it.LanguageName = lang(d.lang, "")
		if d.lang == "hbo" {
			it.LanguageName = "Hebrew"
		}
		if d.lang == "grc" {
			it.LanguageName = "Greek"
		}
		out = append(out, it)
	}
	return out
}

func findDataset(source, id string) (dataset, bool) {
	for _, d := range datasets {
		if d.source == source && d.id == id {
			return d, true
		}
	}
	return dataset{}, false
}

// installData downloads a dataset's files into a temporary folder and installs them.
func (s *Service) installData(ctx context.Context, key string, it Item, o bible.InstallOptions) (bible.Meta, []string, error) {
	d, ok := findDataset(it.Source, it.ID)
	if !ok {
		return bible.Meta{}, nil, fmt.Errorf("no dataset %s", it.ID)
	}
	dir, err := os.MkdirTemp("", "xegesis-data-*")
	if err != nil {
		return bible.Meta{}, nil, err
	}
	defer os.RemoveAll(dir)
	var paths []string
	var total int64
	for i, f := range d.files {
		p := filepath.Join(dir, fmt.Sprintf("%d%s", i, filepath.Ext(f)))
		n, err := s.download(ctx, key, it.base+"/"+escapePath(f), p, total)
		if err != nil {
			return bible.Meta{}, nil, err
		}
		total += n
		paths = append(paths, p)
	}
	s.setProgress(key, func(p *Progress) { p.Phase = "converting" })
	return d.install(ctx, s, o, paths)
}

// escapePath escapes each part of a slash-separated path for a URL.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

func openAll(paths []string) ([]io.Reader, func(), error) {
	var rs []io.Reader
	var fs []*os.File
	done := func() {
		for _, f := range fs {
			f.Close()
		}
	}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			done()
			return nil, nil, err
		}
		fs = append(fs, f)
		rs = append(rs, f)
	}
	return rs, done, nil
}

func installSTEPText(ctx context.Context, s *Service, o bible.InstallOptions, paths []string) (bible.Meta, []string, error) {
	rs, done, err := openAll(paths)
	if err != nil {
		return bible.Meta{}, nil, err
	}
	defer done()
	o.Versification = "english"
	return s.lib.InstallSTEP(ctx, o, rs)
}

func installSTEPLexicon(meanings bool) func(context.Context, *Service, bible.InstallOptions, []string) (bible.Meta, []string, error) {
	return func(ctx context.Context, s *Service, o bible.InstallOptions, paths []string) (bible.Meta, []string, error) {
		f, err := os.Open(paths[0])
		if err != nil {
			return bible.Meta{}, nil, err
		}
		defer f.Close()
		heads, err := bible.ReadSTEPLexicon(f, meanings)
		if err != nil {
			return bible.Meta{}, nil, err
		}
		o.Kind = bible.KindLexicon
		m, err := s.lib.InstallHeadwords(ctx, o, heads)
		return m, nil, err
	}
}

func installOpenBible(ctx context.Context, s *Service, o bible.InstallOptions, paths []string) (bible.Meta, []string, error) {
	dir := filepath.Join(filepath.Dir(paths[0]), "unzipped")
	if err := unzipTo(paths[0], dir, ".txt"); err != nil {
		return bible.Meta{}, nil, err
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.txt"))
	if len(matches) == 0 {
		return bible.Meta{}, nil, fmt.Errorf("the download holds no cross-reference list")
	}
	f, err := os.Open(matches[0])
	if err != nil {
		return bible.Meta{}, nil, err
	}
	defer f.Close()
	return s.lib.InstallCrossRefs(ctx, o, f, 25)
}

// unzipTo extracts the files of a zip archive with the given extension into dir, flat, refusing
// archives that expand beyond maxDownload.
func unzipTo(path, dir, ext string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("the download is not a zip archive: %w", err)
	}
	defer zr.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var total int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.EqualFold(filepath.Ext(f.Name), ext) {
			continue
		}
		if err := extract(f, filepath.Join(dir, filepath.Base(f.Name)), &total); err != nil {
			return err
		}
	}
	return nil
}

func extract(f *zip.File, dest string, total *int64) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	n, err := io.Copy(out, io.LimitReader(r, maxDownload-*total+1))
	*total += n
	if err == nil && *total > maxDownload {
		err = fmt.Errorf("the archive expands beyond %d bytes", int64(maxDownload))
	}
	return err
}
