package plan

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

type env struct {
	reg   *command.Registry
	env   *command.Env
	today string
}

func setup(t *testing.T) *env {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	e := &env{reg: command.NewRegistry(), env: &command.Env{Store: s}, today: "2026-03-02"} // a Monday
	register(e.reg, &service{reg: e.reg, today: func() string { return e.today }})
	return e
}

func (e *env) run(t *testing.T, id, params string, out any) error {
	t.Helper()
	res, err := e.reg.Run(context.Background(), e.env, id, json.RawMessage(params))
	if err != nil {
		return err
	}
	if out != nil {
		b, _ := json.Marshal(res)
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatal(err)
		}
	}
	return nil
}

func (e *env) must(t *testing.T, id, params string, out any) {
	t.Helper()
	if err := e.run(t, id, params, out); err != nil {
		t.Fatalf("%s: %v", id, err)
	}
}

func code(err error) string {
	var ce *command.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func TestTemplatesDescribeThemselves(t *testing.T) {
	e := setup(t)
	var ts []Template
	e.must(t, "plan.templates", "", &ts)
	if len(ts) != 4 {
		t.Fatalf("%d templates", len(ts))
	}
	for _, tp := range ts {
		if tp.ID == "" || tp.Name == "" || tp.Rule == "" || tp.Days == 0 {
			t.Errorf("incomplete template %+v", tp)
		}
	}
	for _, d := range e.reg.List() {
		if len(d.ID) > 5 && d.ID[:5] == "plan." && d.Summary == "" {
			t.Errorf("%s has no summary", d.ID)
		}
	}
}

// Whole Bible in a year: 365 days, every chapter once, in order, with balanced days.
func TestBibleInAYear(t *testing.T) {
	e := setup(t)
	var p Plan
	e.must(t, "plan.create", `{"template":"bible-year"}`, &p)
	if p.Days != 365 || p.Name != "Whole Bible in a year" || p.StartDate != e.today || p.DaysLeft != 365 {
		t.Fatalf("plan = %+v", p)
	}
	if p.Finish != "2027-03-01" {
		t.Errorf("finish = %s, want 2027-03-01", p.Finish)
	}
	var days []Day
	e.must(t, "plan.days", `{"plan_id":"`+p.ID+`"}`, &days)
	if len(days) != 365 {
		t.Fatalf("%d days", len(days))
	}
	seen := map[int]int{}
	next := verseID(1, 1, 1)
	most := 0
	for _, d := range days {
		if len(d.Readings) == 0 {
			t.Fatalf("day %d is empty", d.Day)
		}
		total := 0
		for _, r := range d.Readings {
			if r.Start != next {
				t.Fatalf("day %d starts at %v, want %v", d.Day, r.Start, next)
			}
			total += r.Verses
			for ch := r.Start.Chapter(); ch <= r.End.Chapter(); ch++ {
				seen[r.Start.Book()*1000+ch]++
			}
			next = nextVerse(r.End)
		}
		most = max(most, total)
	}
	chapters := 0
	for b := 1; b <= 66; b++ {
		chapters += len(chapterVerses[b])
	}
	if len(seen) != chapters || chapters != 1189 {
		t.Errorf("%d chapters read, Bible has %d", len(seen), chapters)
	}
	for k, n := range seen {
		if n != 1 {
			t.Errorf("chapter %d read %d times", k, n)
		}
	}
	if most > 200 { // average 85; Psalm 119 alone has 176
		t.Errorf("heaviest day has %d verses", most)
	}
	if days[0].Date != "2026-03-02" || days[364].Date != "2027-03-01" {
		t.Errorf("dates %s to %s", days[0].Date, days[364].Date)
	}
	if days[0].Readings[0].Ref != "Genesis 1-3" {
		t.Errorf("first reading = %q", days[0].Readings[0].Ref)
	}
}

func nextVerse(end verse.ID) verse.ID {
	counts := chapterVerses[end.Book()]
	if end.Verse() < counts[end.Chapter()-1] {
		return verseID(end.Book(), end.Chapter(), end.Verse()+1)
	}
	if end.Chapter() < len(counts) {
		return verseID(end.Book(), end.Chapter()+1, 1)
	}
	return verseID(end.Book()+1, 1, 1)
}

func TestOtherTemplates(t *testing.T) {
	e := setup(t)
	for id, want := range map[string]int{"nt-90": 90, "psalms-proverbs-30": 30, "gospels-30": 30} {
		var p Plan
		e.must(t, "plan.create", `{"template":"`+id+`"}`, &p)
		if p.Days != want {
			t.Errorf("%s has %d days, want %d", id, p.Days, want)
		}
	}
	var days []Day
	var ps []Plan
	e.must(t, "plan.list", "", &ps)
	for _, p := range ps {
		if p.Template == "gospels-30" {
			e.must(t, "plan.days", `{"plan_id":"`+p.ID+`"}`, &days)
		}
	}
	if days[0].Readings[0].Start != verseID(40, 1, 1) || days[29].Readings[len(days[29].Readings)-1].End != verseID(43, 21, 25) {
		t.Errorf("gospels run from %v to %v", days[0].Readings[0].Start, days[29].Readings[0].End)
	}
}

func TestTodayMarkAndProgress(t *testing.T) {
	e := setup(t)
	var p Plan
	e.must(t, "plan.create", `{"template":"gospels-30"}`, &p)
	var td []Today
	e.must(t, "plan.today", "", &td)
	if len(td) != 1 || td[0].Status != "today" || td[0].Today == nil || td[0].Today.Day != 1 || len(td[0].Missed) != 0 {
		t.Fatalf("today = %+v", td)
	}
	e.must(t, "plan.mark", `{"plan_id":"`+p.ID+`"}`, &p)
	if p.DaysDone != 1 || p.DaysLeft != 29 || p.Behind != 0 {
		t.Errorf("after mark: %+v", p)
	}
	e.must(t, "plan.today", "", &td)
	if td[0].Status != "done_for_today" || td[0].Next == nil || td[0].Next.Day != 2 {
		t.Errorf("after mark: %+v", td[0])
	}
	e.must(t, "plan.mark", `{"plan_id":"`+p.ID+`"}`, &p) // marking again today takes the next unread day
	if p.DaysDone != 2 {
		t.Errorf("second mark: %+v", p)
	}
	e.must(t, "plan.unmark", `{"plan_id":"`+p.ID+`","day":2}`, &p)
	e.must(t, "plan.unmark", `{"plan_id":"`+p.ID+`","day":2}`, &p) // harmless twice
	if p.DaysDone != 1 {
		t.Errorf("after unmark: %+v", p)
	}
	if err := e.run(t, "plan.mark", `{"plan_id":"`+p.ID+`","days":[31]}`, nil); code(err) != "bad_params" {
		t.Errorf("day 31: %v", err)
	}
	if err := e.run(t, "plan.today", `{"plan_id":"nope"}`, nil); code(err) != "not_found" {
		t.Errorf("unknown plan: %v", err)
	}
}

// Two days missed shows the days to catch up; reschedule moves what is left.
func TestCatchUpAndReschedule(t *testing.T) {
	e := setup(t)
	var p Plan
	e.must(t, "plan.create", `{"template":"gospels-30"}`, &p) // days 1..30 on 2026-03-02..2026-03-31
	e.must(t, "plan.mark", `{"plan_id":"`+p.ID+`"}`, nil)
	e.today = "2026-03-05" // days 2 and 3 were missed; day 4 is today
	var td []Today
	e.must(t, "plan.today", "", &td)
	got := td[0]
	if got.Status != "behind" || len(got.Missed) != 2 || got.Missed[0].Day != 2 || got.Missed[1].Day != 3 || got.Today.Day != 4 {
		t.Fatalf("today = %+v", got)
	}
	if got.Plan.Behind != 2 || got.Plan.Finish != "2026-04-02" { // 29 days left from the 5th
		t.Errorf("plan = %+v", got.Plan)
	}
	e.must(t, "plan.reschedule", `{"plan_id":"`+p.ID+`"}`, &p)
	if p.Behind != 0 || p.Finish != "2026-04-02" {
		t.Errorf("after reschedule: %+v", p)
	}
	e.must(t, "plan.today", "", &td)
	if td[0].Status != "today" || td[0].Today.Day != 2 || len(td[0].Missed) != 0 {
		t.Errorf("after reschedule: %+v", td[0])
	}
	var days []Day
	e.must(t, "plan.days", `{"plan_id":"`+p.ID+`"}`, &days)
	if days[0].Date != "2026-03-02" || !days[0].Done || days[1].Date != "2026-03-05" || days[29].Date != "2026-04-02" {
		t.Errorf("dates: %v %v %v", days[0], days[1].Date, days[29].Date)
	}
}

func TestCustomWeekdaysWithEndDate(t *testing.T) {
	e := setup(t)
	var p Plan
	// Mark 2026-03-02 (Monday) to 2026-03-13 (Friday): ten weekdays.
	e.must(t, "plan.create", `{"name":"Letters","ranges":["Rom","1Cor-2Cor","Gal 1-6"],"weekdays":["mon","tue","wed","thu","fri"],"end":"2026-03-13"}`, &p)
	if p.Days != 10 || p.Name != "Letters" || len(p.Weekdays) != 5 || p.Finish != "2026-03-13" {
		t.Fatalf("plan = %+v", p)
	}
	var days []Day
	e.must(t, "plan.days", `{"plan_id":"`+p.ID+`"}`, &days)
	for _, d := range days {
		if wd := mustDate(t, d.Date).Weekday(); wd == time.Sunday || wd == time.Saturday {
			t.Errorf("day %d is on %s", d.Day, d.Date)
		}
	}
	if days[4].Date != "2026-03-06" || days[5].Date != "2026-03-09" {
		t.Errorf("week break: %s %s", days[4].Date, days[5].Date)
	}
	// Saturday: nothing to read.
	e.today = "2026-03-07"
	var td []Today
	e.must(t, "plan.today", "", &td)
	if td[0].Status != "behind" { // Friday's reading is unread
		t.Errorf("status = %s", td[0].Status)
	}
	e.must(t, "plan.mark", `{"plan_id":"`+p.ID+`","days":[1,2,3,4,5]}`, nil)
	e.must(t, "plan.today", "", &td)
	if td[0].Status != "rest" || td[0].Today != nil || td[0].Next == nil || td[0].Next.Date != "2026-03-09" {
		t.Errorf("on the weekend: %+v", td[0])
	}
	// Both days and end is a mistake; an end before the start is too.
	for _, bad := range []string{`{"ranges":["Rom"],"days":3,"end":"2026-03-13"}`, `{"ranges":["Rom"],"end":"2026-01-01"}`,
		`{"ranges":["Rom"],"days":17}`, `{"ranges":["Rom"],"weekdays":["funday"]}`, `{}`, `{"template":"x"}`,
		`{"template":"nt-90","ranges":["Rom"]}`, `{"ranges":["Nowhere"]}`} {
		if err := e.run(t, "plan.create", bad, nil); code(err) != "bad_params" {
			t.Errorf("%s: %v", bad, err)
		}
	}
}

func TestDaysByHandAndBooks(t *testing.T) {
	e := setup(t)
	err := e.env.Store.Write(context.Background(), "test.seed", func(tx *store.Tx) error {
		return tx.Create("items", store.Row{"id": "BOOK1", "kind": "monograph", "title": "On Grace", "source": "own",
			"created_at": int64(1), "updated_at": int64(1)})
	})
	if err != nil {
		t.Fatal(err)
	}
	var p Plan
	e.must(t, "plan.create", `{"name":"Mixed","manual":[
		{"refs":["John 3:16-21","Ps 23"]},
		{"refs":["Rom 8"],"books":[{"item_id":"BOOK1","from":2,"to":3}]},
		{"refs":["John 3:16-21","John 3:16-21"]}]}`, &p)
	if p.Days != 3 {
		t.Fatalf("plan = %+v", p)
	}
	var days []Day
	e.must(t, "plan.days", `{"plan_id":"`+p.ID+`"}`, &days)
	if r := days[0].Readings; len(r) != 2 || r[0].Ref != "Psalms 23" && r[0].Ref != "John 3:16-21" {
		t.Errorf("day 1 = %+v", r)
	}
	r := days[1].Readings
	if len(r) != 3 || r[0].Ref != "Romans 8" || r[1].Ref != "On Grace, section 2" || r[2].Section != 3 || r[2].ItemID != "BOOK1" {
		t.Errorf("day 2 = %+v", r)
	}
	if len(days[2].Readings) != 1 {
		t.Errorf("day 3 = %+v", days[2].Readings)
	}
	// Book sections can also be spread by the rules, with the passages.
	e.must(t, "plan.create", `{"ranges":["Ruth"],"books":[{"item_id":"BOOK1","to":4}],"days":4}`, &p)
	e.must(t, "plan.days", `{"plan_id":"`+p.ID+`"}`, &days)
	n := 0
	for _, d := range days {
		n += len(d.Readings)
	}
	if len(days) != 4 || n < 4 {
		t.Errorf("%d days, %d readings", len(days), n)
	}
	if err := e.run(t, "plan.create", `{"books":[{"item_id":"NOPE","to":2}]}`, nil); code(err) != "not_found" {
		t.Errorf("missing book: %v", err)
	}
}

func TestDeleteAndHistory(t *testing.T) {
	e := setup(t)
	var p Plan
	e.must(t, "plan.create", `{"template":"gospels-30"}`, &p)
	e.must(t, "plan.delete", `{"id":"`+p.ID+`"}`, nil)
	var ps []Plan
	e.must(t, "plan.list", "", &ps)
	if len(ps) != 0 {
		t.Errorf("deleted plan listed: %+v", ps)
	}
	if err := e.run(t, "plan.days", `{"plan_id":"`+p.ID+`"}`, nil); code(err) != "not_found" {
		t.Errorf("days of deleted plan: %v", err)
	}
	h, err := e.env.Store.History(context.Background(), "reading_plans", p.ID)
	if err != nil || len(h) < 2 {
		t.Errorf("history = %v, %v", h, err)
	}
}

func TestSplitIsBalancedAndComplete(t *testing.T) {
	units, _ := bookUnits([]int{19})
	for _, n := range []int{1, 7, 30, 150} {
		runs, err := split(units, n)
		if err != nil || len(runs) != n {
			t.Fatalf("n=%d: %d runs, %v", n, len(runs), err)
		}
		count := 0
		for _, r := range runs {
			if len(r) == 0 {
				t.Errorf("n=%d: empty run", n)
			}
			count += len(r)
		}
		if count != 150 {
			t.Errorf("n=%d: %d chapters", n, count)
		}
	}
	if _, err := split(units, 151); code(err) != "bad_params" {
		t.Errorf("too many days: %v", err)
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := parseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
