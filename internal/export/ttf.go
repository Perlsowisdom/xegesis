package export

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// The PDF writer embeds TrueType fonts found on the system, subset to the glyphs used. Nothing is
// bundled: Windows, macOS and Linux all ship fonts with Latin, Greek and Hebrew. Fonts whose
// licence forbids embedding (OS/2 fsType "restricted") are not used.

type ttfFont struct {
	name       string
	data       []byte
	tables     map[string][]byte
	unitsPerEm int
	numGlyphs  int
	advances   []uint16
	cmap       map[rune]uint16
	bbox       [4]int16
	ascent     int16
	descent    int16
	capHeight  int16
	italic     float64
	longLoca   bool
}

var errNotTTF = errors.New("not a TrueType font")

func be16(b []byte, off int) uint16 {
	if off+2 > len(b) {
		return 0
	}
	return binary.BigEndian.Uint16(b[off:])
}

func be32(b []byte, off int) uint32 {
	if off+4 > len(b) {
		return 0
	}
	return binary.BigEndian.Uint32(b[off:])
}

func parseTTF(name string, data []byte) (*ttfFont, error) {
	off := 0
	if string(data[:min(4, len(data))]) == "ttcf" {
		off = int(be32(data, 12)) // the first font of a collection
	}
	if v := be32(data, off); v != 0x00010000 && v != 0x74727565 {
		return nil, errNotTTF
	}
	n := int(be16(data, off+4))
	f := &ttfFont{name: name, data: data, tables: map[string][]byte{}}
	for i := 0; i < n; i++ {
		rec := off + 12 + 16*i
		if rec+16 > len(data) {
			return nil, errNotTTF
		}
		tag := string(data[rec : rec+4])
		o, l := int(be32(data, rec+8)), int(be32(data, rec+12))
		if o < 0 || l < 0 || o+l > len(data) {
			return nil, errNotTTF
		}
		f.tables[tag] = data[o : o+l]
	}
	for _, t := range []string{"head", "hhea", "maxp", "hmtx", "loca", "glyf", "cmap"} {
		if f.tables[t] == nil {
			return nil, errNotTTF
		}
	}
	if os2 := f.tables["OS/2"]; os2 != nil && be16(os2, 8)&0x000f == 0x0002 {
		return nil, errors.New("the font's licence does not allow embedding")
	}
	head, hhea := f.tables["head"], f.tables["hhea"]
	f.unitsPerEm = int(be16(head, 18))
	for i := range f.bbox {
		f.bbox[i] = int16(be16(head, 36+2*i))
	}
	f.longLoca = be16(head, 50) == 1
	f.ascent, f.descent = int16(be16(hhea, 4)), int16(be16(hhea, 6))
	f.capHeight = f.ascent * 7 / 10
	if os2 := f.tables["OS/2"]; len(os2) >= 90 && be16(os2, 0) >= 2 {
		f.capHeight = int16(be16(os2, 88))
	}
	if post := f.tables["post"]; len(post) >= 8 {
		f.italic = float64(int32(be32(post, 4))) / 65536
	}
	f.numGlyphs = int(be16(f.tables["maxp"], 4))
	nh := int(be16(hhea, 34))
	hmtx := f.tables["hmtx"]
	f.advances = make([]uint16, f.numGlyphs)
	var last uint16
	for g := 0; g < f.numGlyphs; g++ {
		if g < nh {
			last = be16(hmtx, 4*g)
		}
		f.advances[g] = last
	}
	if f.unitsPerEm == 0 || f.numGlyphs == 0 {
		return nil, errNotTTF
	}
	f.cmap = parseCmap(f.tables["cmap"])
	if len(f.cmap) == 0 {
		return nil, errNotTTF
	}
	return f, nil
}

// parseCmap reads the Unicode subtable: format 12 (full range) or format 4 (BMP).
func parseCmap(c []byte) map[rune]uint16 {
	n := int(be16(c, 2))
	best, bestFmt := -1, 0
	for i := 0; i < n; i++ {
		rec := 4 + 8*i
		pid, eid, off := be16(c, rec), be16(c, rec+2), int(be32(c, rec+4))
		if !(pid == 3 && (eid == 1 || eid == 10) || pid == 0) || off >= len(c) {
			continue
		}
		f := int(be16(c, off))
		if f == 12 && bestFmt != 12 || f == 4 && bestFmt == 0 {
			best, bestFmt = off, f
		}
	}
	m := map[rune]uint16{}
	switch bestFmt {
	case 4:
		seg := int(be16(c, best+6)) / 2
		ends, starts := best+14, best+16+2*seg
		deltas, ranges := starts+2*seg, starts+4*seg
		for s := 0; s < seg; s++ {
			end, start := int(be16(c, ends+2*s)), int(be16(c, starts+2*s))
			delta, ro := be16(c, deltas+2*s), int(be16(c, ranges+2*s))
			for r := start; r <= end && r != 0xFFFF; r++ {
				var g uint16
				if ro == 0 {
					g = uint16(r) + delta
				} else {
					at := ranges + 2*s + ro + 2*(r-start)
					if g = be16(c, at); g != 0 {
						g += delta
					}
				}
				if g != 0 {
					m[rune(r)] = g
				}
			}
		}
	case 12:
		groups := int(be32(c, best+12))
		for i := 0; i < groups && i < 1<<20; i++ {
			rec := best + 16 + 12*i
			start, end, g := be32(c, rec), be32(c, rec+4), be32(c, rec+8)
			for r := start; r <= end && r-start < 1<<16; r++ {
				m[rune(r)] = uint16(g + r - start)
			}
		}
	}
	return m
}

func (f *ttfFont) glyph(r rune) (uint16, bool) {
	g, ok := f.cmap[r]
	return g, ok
}

// advance is a glyph's width in thousandths of the font size.
func (f *ttfFont) advance(g uint16) float64 {
	if int(g) >= len(f.advances) {
		return 0
	}
	return float64(f.advances[g]) * 1000 / float64(f.unitsPerEm)
}

func (f *ttfFont) glyphData(g int) []byte {
	loca, glyf := f.tables["loca"], f.tables["glyf"]
	var a, b int
	if f.longLoca {
		a, b = int(be32(loca, 4*g)), int(be32(loca, 4*g+4))
	} else {
		a, b = 2*int(be16(loca, 2*g)), 2*int(be16(loca, 2*g+2))
	}
	if a < 0 || b > len(glyf) || a >= b {
		return nil
	}
	return glyf[a:b]
}

// components lists the glyphs a composite glyph is built from.
func components(g []byte) []int {
	if len(g) < 10 || int16(be16(g, 0)) >= 0 {
		return nil
	}
	var out []int
	for p := 10; p+4 <= len(g); {
		flags := be16(g, p)
		out = append(out, int(be16(g, p+2)))
		p += 4
		if flags&0x0001 != 0 {
			p += 4
		} else {
			p += 2
		}
		switch {
		case flags&0x0008 != 0:
			p += 2
		case flags&0x0040 != 0:
			p += 4
		case flags&0x0080 != 0:
			p += 8
		}
		if flags&0x0020 == 0 {
			break
		}
	}
	return out
}

// subset returns a font file with only the used glyphs' outlines; glyph IDs are unchanged, so the
// PDF can address glyphs by their IDs (Identity CIDToGIDMap).
func (f *ttfFont) subset(used map[uint16]bool) []byte {
	keep := map[int]bool{0: true}
	var add func(int)
	add = func(g int) {
		if g >= f.numGlyphs || keep[g] && g != 0 {
			return
		}
		keep[g] = true
		for _, c := range components(f.glyphData(g)) {
			add(c)
		}
	}
	for g := range used {
		add(int(g))
	}
	add(0)
	var glyf []byte
	loca := make([]byte, 4*(f.numGlyphs+1))
	for g := 0; g < f.numGlyphs; g++ {
		binary.BigEndian.PutUint32(loca[4*g:], uint32(len(glyf)))
		if keep[g] {
			glyf = append(glyf, f.glyphData(g)...)
			for len(glyf)%4 != 0 {
				glyf = append(glyf, 0)
			}
		}
	}
	binary.BigEndian.PutUint32(loca[4*f.numGlyphs:], uint32(len(glyf)))
	head := append([]byte(nil), f.tables["head"]...)
	binary.BigEndian.PutUint32(head[8:], 0) // checkSumAdjustment
	binary.BigEndian.PutUint16(head[50:], 1)
	tables := map[string][]byte{"head": head, "loca": loca, "glyf": glyf}
	for _, t := range []string{"cmap", "hhea", "hmtx", "maxp", "cvt ", "fpgm", "prep", "OS/2", "post"} {
		if b := f.tables[t]; b != nil {
			tables[t] = b
		}
	}
	if post := tables["post"]; len(post) >= 32 { // keep the header only: version 3, no glyph names
		p := append([]byte(nil), post[:32]...)
		binary.BigEndian.PutUint32(p, 0x00030000)
		tables["post"] = p
	}
	return writeSFNT(tables)
}

func writeSFNT(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	n := len(tags)
	es := 0
	for 1<<(es+1) <= n {
		es++
	}
	out := make([]byte, 12+16*n)
	binary.BigEndian.PutUint32(out, 0x00010000)
	binary.BigEndian.PutUint16(out[4:], uint16(n))
	binary.BigEndian.PutUint16(out[6:], uint16(16<<es))
	binary.BigEndian.PutUint16(out[8:], uint16(es))
	binary.BigEndian.PutUint16(out[10:], uint16(16*n-16<<es))
	for i, t := range tags {
		data := tables[t]
		rec := 12 + 16*i
		copy(out[rec:], t)
		binary.BigEndian.PutUint32(out[rec+4:], checksum(data))
		binary.BigEndian.PutUint32(out[rec+8:], uint32(len(out)))
		binary.BigEndian.PutUint32(out[rec+12:], uint32(len(data)))
		out = append(out, data...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}
	return out
}

func checksum(b []byte) uint32 {
	var s uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:min(i+4, len(b))])
		s += binary.BigEndian.Uint32(w[:])
	}
	return s
}

// Finding fonts ----------------------------------------------------------------------------------

// family is a serif face in four styles (regular, bold, italic, bold italic) and fallbacks, by
// file name, for characters it lacks.
type family struct {
	styles    [4][]string
	fallbacks []string
}

var families = []family{
	{styles: [4][]string{{"times.ttf", "Times New Roman.ttf", "DejaVuSerif.ttf", "LiberationSerif-Regular.ttf", "NotoSerif-Regular.ttf"},
		{"timesbd.ttf", "Times New Roman Bold.ttf", "DejaVuSerif-Bold.ttf", "LiberationSerif-Bold.ttf", "NotoSerif-Bold.ttf"},
		{"timesi.ttf", "Times New Roman Italic.ttf", "DejaVuSerif-Italic.ttf", "LiberationSerif-Italic.ttf", "NotoSerif-Italic.ttf"},
		{"timesbi.ttf", "Times New Roman Bold Italic.ttf", "DejaVuSerif-BoldItalic.ttf", "LiberationSerif-BoldItalic.ttf", "NotoSerif-BoldItalic.ttf"}},
		fallbacks: []string{"arial.ttf", "Arial Unicode.ttf", "Arial.ttf", "seguisym.ttf", "DejaVuSans.ttf", "NotoSansHebrew-Regular.ttf",
			"NotoSans-Regular.ttf", "FreeSerif.ttf", "SBL_Hbrw.ttf", "Cardo-Regular.ttf"}},
}

func fontDirs() []string {
	var dirs []string
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		win := os.Getenv("WINDIR")
		if win == "" {
			win = `C:\Windows`
		}
		dirs = append(dirs, filepath.Join(win, "Fonts"))
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			dirs = append(dirs, filepath.Join(la, "Microsoft", "Windows", "Fonts"))
		}
	case "darwin":
		dirs = append(dirs, "/System/Library/Fonts/Supplemental", "/System/Library/Fonts", "/Library/Fonts", filepath.Join(home, "Library", "Fonts"))
	default:
		dirs = append(dirs, "/usr/share/fonts", "/usr/local/share/fonts", filepath.Join(home, ".local", "share", "fonts"), filepath.Join(home, ".fonts"))
	}
	if d := os.Getenv("XEGESIS_FONTS"); d != "" {
		dirs = append([]string{d}, dirs...)
	}
	return dirs
}

var (
	fontIndexOnce sync.Once
	fontIndex     map[string]string // lower-case file name to path
	fontCache     sync.Map          // path to *ttfFont
)

func findFont(name string) string {
	fontIndexOnce.Do(func() {
		fontIndex = map[string]string{}
		for _, d := range fontDirs() {
			filepath.WalkDir(d, func(p string, e os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if !e.IsDir() {
					k := strings.ToLower(e.Name())
					if _, ok := fontIndex[k]; !ok {
						fontIndex[k] = p
					}
				}
				return nil
			})
		}
	})
	return fontIndex[strings.ToLower(name)]
}

func loadFont(path string) *ttfFont {
	if f, ok := fontCache.Load(path); ok {
		return f.(*ttfFont)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	name := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	f, err := parseTTF(name, data)
	if err != nil {
		return nil
	}
	fontCache.Store(path, f)
	return f
}

// fontSet is the faces a PDF uses: four styles (missing ones fall back to regular) and fallbacks.
type fontSet struct {
	styles    [4]*ttfFont
	fallbacks []*ttfFont
}

func systemFonts() (*fontSet, error) {
	for _, fam := range families {
		fs := &fontSet{}
		for st, names := range fam.styles {
			for _, n := range names {
				if p := findFont(n); p != "" {
					if f := loadFont(p); f != nil {
						fs.styles[st] = f
						break
					}
				}
			}
		}
		if fs.styles[0] == nil {
			continue
		}
		for st := range fs.styles {
			if fs.styles[st] == nil {
				fs.styles[st] = fs.styles[0]
			}
		}
		for _, n := range fam.fallbacks {
			if p := findFont(n); p != "" {
				if f := loadFont(p); f != nil {
					fs.fallbacks = append(fs.fallbacks, f)
				}
			}
		}
		return fs, nil
	}
	return nil, errors.New("no font for PDF was found on this computer (looked for Times New Roman, DejaVu Serif, Liberation Serif and Noto Serif); print and choose Save as PDF instead, or set XEGESIS_FONTS to a folder of TrueType fonts")
}
