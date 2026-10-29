package sword

import (
	"bytes"
	"compress/bzip2"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Dictionary modules (lexicons, Bible dictionaries) are keyed by headword. Layouts, all
// little-endian, as seen in public-domain CrossWire modules:
//
//	RawLD   <name>.idx  start u32, size u16   into <name>.dat, where an entry is "KEY\r\n" + text
//	RawLD4              start u32, size u32
//	zLD     <name>.idx  start u32, size u32   into <name>.dat, where an entry is "KEY\r\n" + block u32 + entry u32
//	        <name>.zdx  offset u32, size u32  compressed blocks in <name>.zdt; a block holds a count u32,
//	                                         then start u32 and size u32 per entry, then the entry texts
var ldDrivers = map[string]struct{ compressed, size4 bool }{
	"rawld": {false, false}, "rawld4": {false, true}, "zld": {true, false},
}

// ldPrefix is the file name prefix of a dictionary's files: the last part of its DataPath.
func ldPrefix(c Conf) string {
	return path.Base(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(c.Get("DataPath")), "./"), "/"))
}

// Dict is a dictionary module's files in a folder.
type Dict struct {
	dir, prefix string
	conf        Conf
	compressed  bool
	size4       bool
	latin       bool
}

// OpenDict checks a dictionary module folder.
func OpenDict(dir string, c Conf) (*Dict, error) {
	if err := Supported(c); err != nil {
		return nil, err
	}
	d, ok := ldDrivers[strings.ToLower(c.Driver())]
	if !ok {
		return nil, fmt.Errorf("sword: %s is not a dictionary driver", c.Driver())
	}
	out := &Dict{dir: dir, prefix: ldPrefix(c), conf: c, compressed: d.compressed, size4: d.size4,
		latin: strings.EqualFold(c.Get("Encoding"), "Latin-1")}
	idx, err := os.Stat(out.file(".idx"))
	if err != nil {
		return nil, err
	}
	rs := int64(6)
	if d.size4 || d.compressed {
		rs = 8
	}
	if idx.Size()%rs != 0 {
		return nil, fmt.Errorf("sword: %s.idx is not a whole number of %d-byte entries", out.prefix, rs)
	}
	return out, nil
}

func (d *Dict) file(ext string) string { return filepath.Join(d.dir, d.prefix+ext) }

// Walk calls emit for every entry with its key and text, in key order.
func (d *Dict) Walk(emit func(key, text string) error) error {
	idx, err := os.ReadFile(d.file(".idx"))
	if err != nil {
		return err
	}
	dat, err := os.ReadFile(d.file(".dat"))
	if err != nil {
		return err
	}
	var blocks *ldBlocks
	if d.compressed {
		if blocks, err = d.openBlocks(); err != nil {
			return err
		}
		defer blocks.close()
	}
	rs := 6
	if d.size4 || d.compressed {
		rs = 8
	}
	for n := 0; (n+1)*rs <= len(idx); n++ {
		rec := idx[n*rs:]
		start, size := binary.LittleEndian.Uint32(rec), uint32(binary.LittleEndian.Uint16(rec[4:]))
		if rs == 8 {
			size = binary.LittleEndian.Uint32(rec[4:])
		}
		end := uint64(start) + uint64(size)
		if end > uint64(len(dat)) {
			return fmt.Errorf("sword: %s entry %d runs past the end of the data file", d.prefix, n)
		}
		key, body, ok := bytes.Cut(dat[start:end], []byte("\n"))
		if !ok {
			continue
		}
		k := strings.TrimSpace(d.decode(key))
		if d.compressed {
			if len(body) < 8 {
				continue
			}
			body, err = blocks.entry(binary.LittleEndian.Uint32(body), binary.LittleEndian.Uint32(body[4:]))
			if err != nil {
				return fmt.Errorf("sword: %s %q: %w", d.prefix, k, err)
			}
		}
		text := strings.TrimSpace(d.decode(bytes.TrimRight(body, "\x00")))
		if k == "" || text == "" {
			continue
		}
		if err := emit(k, text); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dict) decode(b []byte) string {
	t := Text{latin: d.latin}
	return t.decode(b)
}

type ldBlocks struct {
	f      *os.File
	index  []byte
	bzip2  bool
	cached int
	block  []byte
}

func (d *Dict) openBlocks() (*ldBlocks, error) {
	index, err := os.ReadFile(d.file(".zdx"))
	if err != nil {
		return nil, err
	}
	f, err := os.Open(d.file(".zdt"))
	if err != nil {
		return nil, err
	}
	return &ldBlocks{f: f, index: index, cached: -1, bzip2: strings.EqualFold(d.conf.Get("CompressType"), "BZIP2")}, nil
}

func (b *ldBlocks) close() { b.f.Close() }

func (b *ldBlocks) entry(block, n uint32) ([]byte, error) {
	if err := b.load(int(block)); err != nil {
		return nil, err
	}
	if len(b.block) < 4 {
		return nil, errors.New("empty block")
	}
	count := binary.LittleEndian.Uint32(b.block)
	if n >= count || uint64(4+8*count) > uint64(len(b.block)) {
		return nil, fmt.Errorf("entry %d is not in block %d", n, block)
	}
	rec := b.block[4+8*n:]
	start, size := binary.LittleEndian.Uint32(rec), binary.LittleEndian.Uint32(rec[4:])
	end := uint64(start) + uint64(size)
	if end > uint64(len(b.block)) {
		return nil, fmt.Errorf("entry %d runs past the end of block %d", n, block)
	}
	return b.block[start:end], nil
}

func (b *ldBlocks) load(n int) error {
	if n == b.cached {
		return nil
	}
	if n < 0 || (n+1)*8 > len(b.index) {
		return fmt.Errorf("block %d is not in the block index", n)
	}
	off, size := binary.LittleEndian.Uint32(b.index[n*8:]), binary.LittleEndian.Uint32(b.index[n*8+4:])
	if size > maxBlock {
		return fmt.Errorf("block %d is too large", n)
	}
	comp := make([]byte, size)
	if _, err := b.f.ReadAt(comp, int64(off)); err != nil {
		return err
	}
	var r io.Reader
	if b.bzip2 {
		r = bzip2.NewReader(bytes.NewReader(comp))
	} else {
		zr, err := zlib.NewReader(bytes.NewReader(comp))
		if err != nil {
			return err
		}
		defer zr.Close()
		r = zr
	}
	data, err := io.ReadAll(io.LimitReader(r, maxBlock+1))
	if err != nil {
		return err
	}
	if len(data) > maxBlock {
		return fmt.Errorf("block %d expands too far", n)
	}
	b.cached, b.block = n, data
	return nil
}
