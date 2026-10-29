package bible

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// InstallCrossRefs makes a cross-reference set from tab-separated lines "from<TAB>to<TAB>votes"
// with OSIS references, as OpenBible.info publishes them ("Gen.1.1<TAB>John.1.1-John.1.3<TAB>384").
// Lines with no votes or fewer than one are left out; each verse keeps its best perVerse targets.
func (l *Library) InstallCrossRefs(ctx context.Context, o InstallOptions, r io.Reader, perVerse int) (Meta, []string, error) {
	type target struct {
		start verse.ID
		label string
		votes int
	}
	byVerse := map[verse.ID][]target{}
	var warnings []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 2 || strings.HasPrefix(f[0], "From") || strings.HasPrefix(f[0], "#") {
			continue
		}
		votes := 1
		if len(f) > 2 {
			v, err := strconv.Atoi(strings.TrimSpace(f[2]))
			if err != nil || v < 1 {
				continue
			}
			votes = v
		}
		from, err := osisVerse(strings.TrimSpace(f[0]))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("line %d: %v", n, err))
			continue
		}
		startRef, endRef, _ := strings.Cut(strings.TrimSpace(f[1]), "-")
		start, err := osisVerse(startRef)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("line %d: %v", n, err))
			continue
		}
		label := start.String()
		if end, err := osisVerse(endRef); err == nil && end != start {
			label = verse.Range{Start: start, End: end}.String()
		}
		byVerse[from] = append(byVerse[from], target{start, label, votes})
	}
	if err := sc.Err(); err != nil {
		return Meta{}, warnings, err
	}
	b := newBuilder()
	ids := make([]verse.ID, 0, len(byVerse))
	for id := range byVerse {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		ts := byVerse[id]
		sort.SliceStable(ts, func(i, j int) bool { return ts[i].votes > ts[j].votes })
		if perVerse > 0 && len(ts) > perVerse {
			ts = ts[:perVerse]
		}
		b.startVerse(id, id)
		for i, t := range ts {
			if i > 0 {
				b.addText("; ")
			}
			b.openSpan("r", attr("o", t.start.OSIS()), false, false)
			b.addText(t.label)
			b.closeSpan("r")
		}
	}
	b.endVerse()
	o.Kind = KindCrossRef
	m, w, err := l.finish(ctx, o, Doc{}, b)
	if len(warnings) > 20 {
		warnings = append(warnings[:20], fmt.Sprintf("and %d more", len(warnings)-20))
	}
	return m, append(warnings, w...), err
}
