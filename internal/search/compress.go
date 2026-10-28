package search

import (
	"bytes"
	"compress/flate"
	"database/sql/driver"
	"fmt"
	"io"
	"sync"

	"modernc.org/sqlite"
)

// Passage text is stored compressed (index format 2). Passages are short, a few hundred
// characters, so each is deflated with a preset dictionary of common English and Bible-study
// words, which roughly halves what deflate alone leaves. The first byte says how a blob is stored:
// rawText as is, flateV1 deflated with dictV1. A new dictionary needs a new tag.
const (
	rawText byte = 0
	flateV1 byte = 1
)

// dictV1 holds frequent words and word pairs, the most frequent last, as deflate prefers.
const dictV1 = `according to the words of the prophet the apostle Paul writes in his letter to the church ` +
	`righteousness justification sanctification redemption atonement resurrection covenant commandment ` +
	`kingdom of heaven kingdom of God Son of Man the Holy Spirit the Lord Jesus Christ the Father ` +
	`salvation faith hope love grace mercy glory sin death life truth wisdom judgment prayer worship ` +
	`Israel Jerusalem Moses Abraham David Peter Isaiah Jeremiah gospel scripture chapter verse ` +
	`however therefore because through between against without himself themselves something nothing ` +
	`should would could might must shall will there their they them these those where which while ` +
	`about after again also always another any been before being both came come does done each even ` +
	`every first from good great have here into know like little long made make many more most much ` +
	`never only other over people said same see such than then thing think time under upon very ` +
	`well were what when who whom whose why with world year your. It is not that we have but that he ` +
	`has. This is the way in which it was. He said to them, and they said to him, for it is written, ` +
	`as it is in the one of the all the by the at the on the with the from the for the is the ` +
	`that the to the in the and the of the `

var (
	writers = sync.Pool{New: func() any {
		w, _ := flate.NewWriterDict(nil, flate.BestCompression, []byte(dictV1))
		return w
	}}
	bufs = sync.Pool{New: func() any { return new(bytes.Buffer) }}
)

// packText compresses a passage's text for the passages table.
func packText(s string) []byte {
	buf := bufs.Get().(*bytes.Buffer)
	defer bufs.Put(buf)
	buf.Reset()
	buf.WriteByte(flateV1)
	w := writers.Get().(*flate.Writer)
	defer writers.Put(w)
	w.Reset(buf)
	io.WriteString(w, s)
	w.Close()
	if buf.Len() >= len(s)+1 {
		return append([]byte{rawText}, s...)
	}
	return bytes.Clone(buf.Bytes())
}

// unpackText reverses packText.
func unpackText(b []byte) (string, error) {
	if len(b) == 0 {
		return "", fmt.Errorf("empty passage blob")
	}
	switch b[0] {
	case rawText:
		return string(b[1:]), nil
	case flateV1:
		r := flate.NewReaderDict(bytes.NewReader(b[1:]), []byte(dictV1))
		defer r.Close()
		out, err := io.ReadAll(r)
		return string(out), err
	}
	return "", fmt.Errorf("passage blob kind %d is from a newer Xegesis", b[0])
}

// The index files' passages_text view calls xg_unpack, so FTS5 reads plain text for snippets and
// highlighting while the table stores it compressed. Functions registered here are available on
// every connection the driver opens.
func init() {
	err := sqlite.RegisterDeterministicScalarFunction("xg_unpack", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			switch v := args[0].(type) {
			case []byte:
				return unpackText(v)
			case nil:
				return nil, nil
			case string:
				return unpackText([]byte(v))
			}
			return nil, fmt.Errorf("xg_unpack: unexpected %T", args[0])
		})
	if err != nil {
		panic(err)
	}
}
