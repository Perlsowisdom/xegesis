package plan

import (
	"strings"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

const dateLayout = "2006-01-02"

// maxDays keeps a mistyped end date from making a plan of thousands of days.
const maxDays = 3660

var weekdayNames = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// weekdayMask turns names such as "mon" or "Monday" into a mask with bit 0 for Monday. No names
// means every day.
func weekdayMask(names []string) (int, error) {
	if len(names) == 0 {
		return 127, nil
	}
	mask := 0
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		found := false
		for i, w := range weekdayNames {
			if len(n) >= 3 && strings.HasPrefix(n, w) {
				mask |= 1 << i
				found = true
			}
		}
		if !found {
			return 0, command.Errorf("bad_params", "%q is not a day of the week; use mon, tue, wed, thu, fri, sat or sun", n)
		}
	}
	return mask, nil
}

func maskNames(mask int) []string {
	out := []string{}
	for i, w := range weekdayNames {
		if mask&(1<<i) != 0 {
			out = append(out, w)
		}
	}
	return out
}

func parseDate(s string) (time.Time, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return t, command.Errorf("bad_params", "%q is not a date; use YYYY-MM-DD", s)
	}
	return t, nil
}

// readsOn says whether the mask includes the weekday of t.
func readsOn(mask int, t time.Time) bool { return mask&(1<<((int(t.Weekday())+6)%7)) != 0 }

// nextDates lists the first n reading days on or after from.
func nextDates(from time.Time, mask, n int) []string {
	out := make([]string, 0, n)
	for t := from; len(out) < n; t = t.AddDate(0, 0, 1) {
		if readsOn(mask, t) {
			out = append(out, t.Format(dateLayout))
		}
	}
	return out
}

// countDates counts the reading days from from to end, both included.
func countDates(from, end time.Time, mask int) (int, error) {
	if end.Before(from) {
		return 0, command.Errorf("bad_params", "the end date is before the start date")
	}
	if end.Sub(from) > maxDays*24*time.Hour {
		return 0, command.Errorf("bad_params", "a plan can run for at most %d days", maxDays)
	}
	n := 0
	for t := from; !t.After(end); t = t.AddDate(0, 0, 1) {
		if readsOn(mask, t) {
			n++
		}
	}
	if n == 0 {
		return 0, command.Errorf("bad_params", "none of the chosen weekdays falls between the start and end dates")
	}
	return n, nil
}
