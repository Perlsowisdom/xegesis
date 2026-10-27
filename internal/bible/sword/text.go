package sword

import (
	"bytes"
	"compress/bzip2"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Index layouts, all little-endian:
//
//	RawText  ot.vss / nt.vss  start u32, size u16   text in ot / nt
//	RawText4                  start u32, size u32
//	zText    ot.bzv / nt.bzv  block u32, start u32, size u16   (start and size within the block)
//	zText4                    block u32, start u32, size u32
//	zText    ot.bzs / nt.bzs  offset u32, size u32, uncompressed size u32   blocks in ot.bzz / nt.bzz
type driver struct {
	compressed bool
	sizeBytes  int // 2 or 4
}

var drivers = map[string]driver{
	"rawtext": {false, 2}, "rawtext4": {false, 4},
	"ztext": {true, 2}, "ztext4": {true, 4},
}

func (d driver) recordSize() int {
	if d.compressed {
		return 8 + d.sizeBytes
	}
	return 4 + d.sizeBytes
}

var testaments = [2]string{"ot", "nt"}

// ErrLocked is returned for enciphered modules.
var ErrLocked = errors.New("sword: the module is locked; it needs the publisher's unlock key")

// Supported reports whether Xegesis can read a module, and why not.
func Supported(c Conf) error {
	if c.Locked() {
		return ErrLocked
	}
	d, ok := drivers[strings.ToLower(c.Driver())]
	if !ok {
		return fmt.Errorf("sword: %s modules (%s) cannot be read yet", c.Kind(), c.Driver())
	}
	if d.compressed {
		switch strings.ToUpper(c.Get("CompressType")) {
		case "ZIP", "BZIP2":
		case "", "LZSS", "XZ":
			return fmt.Errorf("sword: %s compression cannot be read yet", firstNonEmpty(c.Get("CompressType"), "LZSS"))
		default:
			return fmt.Errorf("sword: unknown compression %q", c.Get("CompressType"))
		}
	}
	switch strings.ToUpper(c.Get("Encoding")) {
	case "", "UTF-8", "LATIN-1":
	default:
		return fmt.Errorf("sword: %s text encoding cannot be read", c.Get("Encoding"))
	}
	switch strings.ToLower(c.Get("SourceType")) {
	case "", "plaintext", "osis", "thml", "gbf":
	default:
		return fmt.Errorf("sword: %s markup cannot be read", c.Get("SourceType"))
	}
	if _, ok := LookupVersification(c.Get("Versification")); !ok {
		return fmt.Errorf("sword: the %s verse numbering is not supported yet", c.Get("Versification"))
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// DataFiles lists the files a module's folder may hold. A testament's files are absent when the
// module has no books in it.
func DataFiles(c Conf) []string {
	d := drivers[strings.ToLower(c.Driver())]
	var out []string
	for _, t := range testaments {
		if d.compressed {
			out = append(out, t+".bzs", t+".bzv", t+".bzz")
		} else {
			out = append(out, t, t+".vss")
		}
	}
	return out
}

// Text is a Bible module's files in a folder.
type Text struct {
	dir   string
	conf  Conf
	drv   driver
	v11n  *Versification
	latin bool
}

// Open checks a module folder: the module must be supported, and each testament's index must
// have exactly the entries its numbering needs, which also confirms the numbering is right.
func Open(dir string, c Conf) (*Text, error) {
	if err := Supported(c); err != nil {
		return nil, err
	}
	v, _ := LookupVersification(c.Get("Versification"))
	t := &Text{dir: dir, conf: c, drv: drivers[strings.ToLower(c.Driver())], v11n: v,
		latin: strings.EqualFold(c.Get("Encoding"), "Latin-1")}
	found := false
	for i := range testaments {
		n, ok, err := t.entries(i)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		found = true
		if want := v.Entries(i); n != want {
			return nil, fmt.Errorf("sword: %s index has %d entries; the %s numbering needs %d",
				testaments[i], n, v.Name, want)
		}
	}
	if !found {
		return nil, errors.New("sword: the module folder has no text files")
	}
	return t, nil
}

// System is the Xegesis numbering the module's verse IDs are in.
func (t *Text) System() verse.System { return t.v11n.System }

func (t *Text) indexName(i int) string {
	if t.drv.compressed {
		return testaments[i] + ".bzv"
	}
	return testaments[i] + ".vss"
}

// entries counts a testament's index entries; ok is false when the testament is absent.
func (t *Text) entries(i int) (int, bool, error) {
	info, err := os.Stat(filepath.Join(t.dir, t.indexName(i)))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	rs := int64(t.drv.recordSize())
	if info.Size()%rs != 0 {
		return 0, false, fmt.Errorf("sword: %s is %d bytes, not a whole number of %d-byte entries",
			t.indexName(i), info.Size(), rs)
	}
	return int(info.Size() / rs), true, nil
}

// Walk calls emit for every non-empty entry in canonical order.
func (t *Text) Walk(emit func(id verse.ID, intro bool, markup string) error) error {
	for i := range testaments {
		if _, ok, err := t.entries(i); err != nil || !ok {
			if err != nil {
				return err
			}
			continue
		}
		if err := t.walkTestament(i, emit); err != nil {
			return fmt.Errorf("sword: %s: %w", testaments[i], err)
		}
	}
	return nil
}

func (t *Text) walkTestament(i int, emit func(verse.ID, bool, string) error) error {
	index, err := os.ReadFile(filepath.Join(t.dir, t.indexName(i)))
	if err != nil {
		return err
	}
	src, err := t.source(i)
	if err != nil {
		return err
	}
	defer src.close()
	rs := t.drv.recordSize()
	for n, s := range t.v11n.slots(i) {
		raw, err := src.entry(index[n*rs : (n+1)*rs])
		if err != nil {
			return fmt.Errorf("entry %d (%s): %w", n, s.id, err)
		}
		text := t.decode(raw)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if err := emit(s.id, s.intro, text); err != nil {
			return err
		}
	}
	return nil
}

func (t *Text) decode(raw []byte) string {
	if t.latin {
		out, err := charmap.Windows1252.NewDecoder().Bytes(raw)
		if err == nil {
			return string(out)
		}
	}
	if !utf8.Valid(raw) {
		return strings.ToValidUTF8(string(raw), "�")
	}
	return string(raw)
}

// source reads entry text for one testament.
type source interface {
	entry(rec []byte) ([]byte, error)
	close()
}

func (t *Text) source(i int) (source, error) {
	name := filepath.Join(t.dir, testaments[i])
	if !t.drv.compressed {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		return &rawSource{f: f, sizeBytes: t.drv.sizeBytes}, nil
	}
	blocks, err := os.ReadFile(name + ".bzs")
	if err != nil {
		return nil, err
	}
	if len(blocks)%12 != 0 {
		return nil, fmt.Errorf("%s.bzs is not a whole number of 12-byte entries", testaments[i])
	}
	f, err := os.Open(name + ".bzz")
	if err != nil {
		return nil, err
	}
	return &zSource{f: f, blocks: blocks, sizeBytes: t.drv.sizeBytes, bzip2: strings.EqualFold(t.conf.Get("CompressType"), "BZIP2"), cached: -1}, nil
}

func size(rec []byte, n int) uint32 {
	if n == 2 {
		return uint32(binary.LittleEndian.Uint16(rec))
	}
	return binary.LittleEndian.Uint32(rec)
}

// maxEntry caps one entry, so a damaged index cannot ask for gigabytes.
const maxEntry = 16 << 20

type rawSource struct {
	f         *os.File
	sizeBytes int
}

func (s *rawSource) entry(rec []byte) ([]byte, error) {
	start, n := binary.LittleEndian.Uint32(rec), size(rec[4:], s.sizeBytes)
	if n == 0 {
		return nil, nil
	}
	if n > maxEntry {
		return nil, fmt.Errorf("entry of %d bytes is too large", n)
	}
	buf := make([]byte, n)
	if _, err := s.f.ReadAt(buf, int64(start)); err != nil {
		return nil, err
	}
	return buf, nil
}

func (s *rawSource) close() { s.f.Close() }

type zSource struct {
	f         *os.File
	blocks    []byte
	sizeBytes int
	bzip2     bool
	cached    int
	block     []byte
}

func (s *zSource) entry(rec []byte) ([]byte, error) {
	b := int(binary.LittleEndian.Uint32(rec))
	start, n := binary.LittleEndian.Uint32(rec[4:]), size(rec[8:], s.sizeBytes)
	if n == 0 {
		return nil, nil
	}
	if err := s.load(b); err != nil {
		return nil, err
	}
	end := uint64(start) + uint64(n)
	if end > uint64(len(s.block)) {
		return nil, fmt.Errorf("entry runs past the end of block %d", b)
	}
	return s.block[start:end], nil
}

// maxBlock caps a decompressed block.
const maxBlock = 256 << 20

func (s *zSource) load(b int) error {
	if b == s.cached {
		return nil
	}
	if b < 0 || (b+1)*12 > len(s.blocks) {
		return fmt.Errorf("block %d is not in the block index", b)
	}
	rec := s.blocks[b*12:]
	off, n := binary.LittleEndian.Uint32(rec), binary.LittleEndian.Uint32(rec[4:])
	if n > maxBlock {
		return fmt.Errorf("block %d is too large", b)
	}
	comp := make([]byte, n)
	if _, err := s.f.ReadAt(comp, int64(off)); err != nil {
		return fmt.Errorf("block %d: %w", b, err)
	}
	var r io.Reader
	if s.bzip2 {
		r = bzip2.NewReader(bytes.NewReader(comp))
	} else {
		zr, err := zlib.NewReader(bytes.NewReader(comp))
		if err != nil {
			return fmt.Errorf("block %d: %w", b, err)
		}
		defer zr.Close()
		r = zr
	}
	data, err := io.ReadAll(io.LimitReader(r, maxBlock+1))
	if err != nil {
		return fmt.Errorf("block %d: %w", b, err)
	}
	if len(data) > maxBlock {
		return fmt.Errorf("block %d expands too far", b)
	}
	s.cached, s.block = b, data
	return nil
}

func (s *zSource) close() { s.f.Close() }
