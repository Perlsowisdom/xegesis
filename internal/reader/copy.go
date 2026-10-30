package reader

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/library"
	"github.com/Perlsowisdom/xegesis/internal/licence"
	"github.com/Perlsowisdom/xegesis/internal/search"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// Licence limits are in pages. Copied text is measured in page equivalents of charsPerPage
// characters; a book's length is its PDF page count, or its text length in page equivalents.
const charsPerPage = 2000

// effectiveLicence is the licence the user decided, or, while undecided, the cautious default for
// an unknown licence with any credit the metadata suggests.
func effectiveLicence(it library.Item) licence.Licence {
	if it.Licence != nil {
		return *it.Licence
	}
	l, _ := licence.Defaults(licence.Unknown, "")
	if it.Suggested != nil {
		l.RightsHolder, l.Attribution = it.Suggested.RightsHolder, it.Suggested.Attribution
	}
	return l
}

type CopyParams struct {
	BookParams
	Text string `json:"text" required:"true" desc:"The selected text to copy"`
}

type CopyResult struct {
	Allowed     bool   `json:"allowed"`
	Text        string `json:"text,omitempty" desc:"What to put on the clipboard: the text and the attribution line"`
	Attribution string `json:"attribution,omitempty"`
	Reason      string `json:"reason,omitempty" desc:"Why the copy was refused"`
	Chars       int    `json:"chars" desc:"Characters in this copy"`
	CopiedToday int    `json:"copied_today" desc:"Characters copied from this book today, including this copy if allowed"`
	DailyLimit  int    `json:"daily_limit,omitempty" desc:"Characters the licence allows per day; absent when unlimited"`
	Confirm     bool   `json:"confirm,omitempty" desc:"The licence was never confirmed by the user"`
}

func (s *service) copyCmd(ctx context.Context, env *command.Env, p CopyParams) (CopyResult, error) {
	b, err := s.open(ctx, env, p.ItemID, p.FileID)
	if err != nil {
		return CopyResult{}, err
	}
	lic := effectiveLicence(b.item)
	day := s.today()
	used, err := copiedOn(ctx, env.Store, b.item.ID, day)
	if err != nil {
		return CopyResult{}, err
	}
	n := utf8.RuneCountInString(p.Text)
	res := CopyResult{Chars: n, CopiedToday: used, Attribution: lic.AttributionLine(), Confirm: !lic.Confirmed}
	total := 0
	if lic.Export != nil && lic.Export.Percent > 0 {
		if total, err = s.bookPages(b); err != nil {
			return CopyResult{}, err
		}
	}
	once := lic.Check(licence.Request{Action: licence.Export, Pages: pages(n), TotalPages: total})
	daily := lic.Check(licence.Request{Action: licence.Export, Pages: pages(used + n), TotalPages: total})
	if daily.MaxPages > 0 {
		res.DailyLimit = daily.MaxPages * charsPerPage
	}
	switch {
	case !once.Allowed:
		res.Reason = fmt.Sprintf("this selection is too long to copy: %s (about %d characters)", once.Reason, once.MaxPages*charsPerPage)
		return res, nil
	case !daily.Allowed:
		res.Reason = fmt.Sprintf("the licence allows copying about %d characters of this book a day, and %d have been copied today",
			res.DailyLimit, used)
		return res, nil
	}
	err = env.Store.Write(ctx, "book.copy", func(tx *store.Tx) error {
		return tx.Create("copy_log", store.Row{"id": store.NewID(), "item_id": b.item.ID, "day": day,
			"chars": int64(n), "created_at": now()})
	})
	if err != nil {
		return CopyResult{}, err
	}
	res.Allowed, res.CopiedToday, res.Text = true, used+n, p.Text
	if res.Attribution != "" {
		res.Text = strings.TrimRight(p.Text, "\r\n ") + "\n\n" + res.Attribution
	}
	return res, nil
}

// pages is the number of page equivalents n characters take.
func pages(n int) int { return (n + charsPerPage - 1) / charsPerPage }

func copiedOn(ctx context.Context, st *store.Store, itemID, day string) (int, error) {
	var n int
	err := st.QueryRow(ctx, `SELECT COALESCE(SUM(chars), 0) FROM copy_log WHERE item_id = ? AND day = ? AND action = 'export'`, itemID, day).Scan(&n)
	return n, err
}

// bookPages is the book's length for percentage limits, cached per file version.
func (s *service) bookPages(b book) (int, error) {
	if b.file.Format == "pdf" {
		return pdfPages(b.file.Path)
	}
	key := b.file.ID + "@" + b.file.SHA256
	s.mu.Lock()
	n, ok := s.length[key]
	s.mu.Unlock()
	if !ok {
		secs, err := search.ReadSections(b.file.Path, b.file.Format)
		if err != nil {
			return 0, unreadable(err)
		}
		for _, sec := range secs {
			for _, bl := range sec.Blocks {
				n += utf8.RuneCountInString(bl.Text)
			}
		}
		s.mu.Lock()
		if len(s.length) > 256 {
			clear(s.length)
		}
		s.length[key] = n
		s.mu.Unlock()
	}
	return max(pages(n), 1), nil
}
