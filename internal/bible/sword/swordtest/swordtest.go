// Package swordtest writes tiny SWORD Bible modules for tests, in the KJV numbering.
package swordtest

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"os"
	"path/filepath"

	"github.com/Perlsowisdom/xegesis/internal/bible/sword"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Conf is a .conf file for a module written by ZText or RawText.
func Conf(name, driver, extra string) string {
	return "[" + name + "]\nDescription=" + name + " test module\nDataPath=./modules/texts/" + name + "/\n" +
		"ModDrv=" + driver + "\nSourceType=OSIS\nEncoding=UTF-8\nCompressType=ZIP\nBlockType=BOOK\n" +
		"Versification=KJV\nLang=en\nVersion=1.0\nDistributionLicense=Public Domain\n" + extra
}

// Key is an index position: a verse, or a book (chapter 0) or chapter (verse 0) introduction.
type Key struct{ Book, Chapter, Verse int }

// positions maps keys to index entries for one testament of the KJV numbering.
func positions(testament int) (map[Key]int, int) {
	v, _ := sword.LookupVersification("KJV")
	books := v.OT
	if testament == 1 {
		books = v.NT
	}
	pos := map[Key]int{}
	n := 2
	for _, b := range books {
		pos[Key{b.Number, 0, 0}] = n
		n++
		for c, count := range b.Verses {
			pos[Key{b.Number, c + 1, 0}] = n
			n++
			for vs := 1; vs <= count; vs++ {
				pos[Key{b.Number, c + 1, vs}] = n
				n++
			}
		}
	}
	return pos, n
}

func split(entries map[Key]string) [2]map[Key]string {
	out := [2]map[Key]string{{}, {}}
	for k, s := range entries {
		t := 0
		if k.Book >= 40 {
			t = 1
		}
		out[t][k] = s
	}
	return out
}

// ZText writes a zText module (one zlib block per testament) into dir. Testaments without
// entries are left out.
func ZText(dir string, entries map[Key]string) error {
	for t, es := range split(entries) {
		if len(es) == 0 {
			continue
		}
		pos, n := positions(t)
		var block bytes.Buffer
		idx := make([]byte, 10*n)
		for k, s := range es {
			p := pos[k]
			binary.LittleEndian.PutUint32(idx[p*10:], 0)
			binary.LittleEndian.PutUint32(idx[p*10+4:], uint32(block.Len()))
			binary.LittleEndian.PutUint16(idx[p*10+8:], uint16(len(s)))
			block.WriteString(s)
		}
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		zw.Write(block.Bytes())
		zw.Close()
		bzs := make([]byte, 12)
		binary.LittleEndian.PutUint32(bzs[4:], uint32(z.Len()))
		binary.LittleEndian.PutUint32(bzs[8:], uint32(block.Len()))
		name := filepath.Join(dir, [2]string{"ot", "nt"}[t])
		for ext, data := range map[string][]byte{".bzv": idx, ".bzs": bzs, ".bzz": z.Bytes()} {
			if err := os.WriteFile(name+ext, data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// RawText writes a RawText module into dir.
func RawText(dir string, entries map[Key]string) error {
	for t, es := range split(entries) {
		if len(es) == 0 {
			continue
		}
		pos, n := positions(t)
		var text bytes.Buffer
		idx := make([]byte, 6*n)
		for k, s := range es {
			p := pos[k]
			binary.LittleEndian.PutUint32(idx[p*6:], uint32(text.Len()))
			binary.LittleEndian.PutUint16(idx[p*6+4:], uint16(len(s)))
			text.WriteString(s)
		}
		name := filepath.Join(dir, [2]string{"ot", "nt"}[t])
		if err := os.WriteFile(name, text.Bytes(), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(name+".vss", idx, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ID is a verse ID for a key, for checking results.
func ID(k Key) verse.ID {
	id, _ := verse.Make(k.Book, k.Chapter, k.Verse)
	return id
}

// K makes a Key.
func K(book, chapter, verse int) Key { return Key{Book: book, Chapter: chapter, Verse: verse} }
