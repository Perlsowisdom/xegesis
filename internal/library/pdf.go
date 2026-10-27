package library

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"regexp"
	"strconv"
	"unicode/utf16"
)

// PDF document information dictionary (ISO 32000-1, 14.3.3). The trailer names the Info object;
// it is found by scanning for "N G obj". Info objects inside compressed object streams, and
// encrypted files, are not read, and the caller falls back to the file name.

var infoRefRe = regexp.MustCompile(`/Info\s+(\d+)\s+(\d+)\s+R`)

func pdfMeta(file string) (Meta, error) {
	f, err := os.Open(file)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Meta{}, err
	}
	tail := make([]byte, min(st.Size(), 64<<10))
	if _, err := f.ReadAt(tail, st.Size()-int64(len(tail))); err != nil && err != io.EOF {
		return Meta{}, err
	}
	refs := infoRefRe.FindAllSubmatch(tail, -1)
	if len(refs) == 0 || bytes.Contains(tail, []byte("/Encrypt")) {
		return Meta{}, nil
	}
	ref := refs[len(refs)-1]
	dict := findObject(f, string(ref[1]), string(ref[2]))
	if dict == nil {
		return Meta{}, nil
	}
	m := Meta{Title: pdfString(dict, "Title")}
	if a := pdfString(dict, "Author"); a != "" {
		for _, name := range splitList(a) {
			m.People = append(m.People, Person{Name: name})
		}
	}
	m.Subjects = splitList(pdfString(dict, "Keywords"))
	m.Year = yearOf(pdfString(dict, "CreationDate"))
	return m, nil
}

// findObject returns the body of the last definition of object num gen, up to "endobj".
func findObject(r io.ReaderAt, num, gen string) []byte {
	re := regexp.MustCompile(`(?:^|[\s>])` + num + `\s+` + gen + `\s+obj\b`)
	const chunk, overlap = 1 << 20, 64
	var found []byte
	buf := make([]byte, chunk+overlap)
	for off := int64(0); ; off += chunk {
		n, err := r.ReadAt(buf, off)
		if locs := re.FindAllIndex(buf[:n], -1); len(locs) > 0 {
			body := make([]byte, 16<<10)
			k, _ := r.ReadAt(body, off+int64(locs[len(locs)-1][1]))
			body = body[:k]
			if end := bytes.Index(body, []byte("endobj")); end >= 0 {
				body = body[:end]
			}
			found = body
		}
		if err != nil || n < len(buf) {
			return found
		}
	}
}

// pdfString returns the text string value of /key in a dictionary, or "".
func pdfString(dict []byte, key string) string {
	k := []byte("/" + key)
	for i := bytes.Index(dict, k); i >= 0; {
		j := i + len(k)
		if j < len(dict) && !isDelim(dict[j]) && !isSpace(dict[j]) {
			next := bytes.Index(dict[j:], k)
			if next < 0 {
				return ""
			}
			i = j + next
			continue
		}
		for j < len(dict) && isSpace(dict[j]) {
			j++
		}
		if j >= len(dict) {
			return ""
		}
		switch dict[j] {
		case '(':
			return decodeText(literal(dict[j+1:]))
		case '<':
			return decodeText(hexString(dict[j+1:]))
		}
		return ""
	}
	return ""
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == 0
}

func isDelim(c byte) bool { return bytes.IndexByte([]byte("()<>[]{}/%"), c) >= 0 }

func literal(b []byte) []byte {
	var out []byte
	depth := 0
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c == '\\' && i+1 < len(b):
			i++
			switch e := b[i]; e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r', '\n':
				if e == '\r' && i+1 < len(b) && b[i+1] == '\n' {
					i++
				}
			default:
				if e >= '0' && e <= '7' {
					j := i
					for j < len(b) && j < i+3 && b[j] >= '0' && b[j] <= '7' {
						j++
					}
					v, _ := strconv.ParseUint(string(b[i:j]), 8, 8)
					out = append(out, byte(v))
					i = j - 1
				} else {
					out = append(out, e)
				}
			}
		case c == '(':
			depth++
			out = append(out, c)
		case c == ')':
			if depth == 0 {
				return out
			}
			depth--
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

func hexString(b []byte) []byte {
	var digits []byte
	for _, c := range b {
		if c == '>' {
			break
		}
		if _, err := strconv.ParseUint(string(c), 16, 8); err == nil {
			digits = append(digits, c)
		}
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	out := make([]byte, len(digits)/2)
	for i := range out {
		v, _ := strconv.ParseUint(string(digits[2*i:2*i+2]), 16, 8)
		out[i] = byte(v)
	}
	return out
}

// decodeText decodes a PDF text string: UTF-16BE or UTF-8 with a byte order mark, otherwise
// PDFDocEncoding, read here as Latin-1.
func decodeText(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		b = b[2:]
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(b[2*i:])
		}
		return string(utf16.Decode(u))
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:])
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}
