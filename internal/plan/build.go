package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

// build works out the readings of every day, and a name for the plan.
func (s *service) build(ctx context.Context, env *command.Env, p CreateParams, from time.Time, mask int) ([][]Reading, string, error) {
	own := len(p.Ranges) > 0 || len(p.Books) > 0
	switch {
	case p.Template != "":
		if own || len(p.Manual) > 0 {
			return nil, "", command.Errorf("bad_params", "a ready-made plan cannot be mixed with readings of your own")
		}
		t, ok := findTemplate(p.Template)
		if !ok {
			return nil, "", command.Errorf("bad_params", "no ready-made plan %q; plan.templates lists them", p.Template)
		}
		units, err := bookUnits(t.books)
		if err != nil {
			return nil, "", err
		}
		days, err := spread(units, t.Days, p, from, mask)
		return days, t.Name, err
	case len(p.Manual) > 0:
		if own || p.Days > 0 || p.End != "" {
			return nil, "", command.Errorf("bad_params", "a plan listed by hand has the days you list; leave out ranges, books, days and end")
		}
		days, err := s.manualDays(ctx, env, p.Manual)
		return days, "My reading plan", err
	case own:
		units, err := rangesUnits(p.Ranges)
		if err != nil {
			return nil, "", err
		}
		more, err := s.bookUnits(ctx, env, p.Books)
		if err != nil {
			return nil, "", err
		}
		units = append(units, more...)
		days, err := spread(units, len(units), p, from, mask)
		name := "My reading plan"
		if len(p.Ranges) == 1 && len(p.Books) == 0 {
			name = p.Ranges[0]
		}
		return days, name, err
	}
	return nil, "", command.Errorf("bad_params", "choose a ready-made plan, give ranges or books to read, or list the days by hand")
}

// spread cuts units over the number of days that days or end ask for, or def.
func spread(units []unit, def int, p CreateParams, from time.Time, mask int) ([][]Reading, error) {
	n := def
	switch {
	case p.Days > 0 && p.End != "":
		return nil, command.Errorf("bad_params", "give a number of days or an end date, not both")
	case p.Days > 0:
		n = p.Days
	case p.End != "":
		end, err := parseDate(p.End)
		if err != nil {
			return nil, err
		}
		if n, err = countDates(from, end, mask); err != nil {
			return nil, err
		}
	}
	if n > maxDays {
		return nil, command.Errorf("bad_params", "a plan can run for at most %d days", maxDays)
	}
	runs, err := split(units, n)
	if err != nil {
		return nil, err
	}
	days := make([][]Reading, len(runs))
	for i, r := range runs {
		days[i] = readings(r)
	}
	return days, nil
}

func rangesUnits(ranges []string) ([]unit, error) {
	var out []unit
	for _, r := range ranges {
		a, z, err := parseRange(r)
		if err != nil {
			return nil, err
		}
		u, err := rangeUnits(a, z)
		if err != nil {
			return nil, err
		}
		out = append(out, u...)
	}
	return out, nil
}

// bookUnits makes one unit per section of the given library books.
func (s *service) bookUnits(ctx context.Context, env *command.Env, parts []BookPart) ([]unit, error) {
	var out []unit
	for _, bp := range parts {
		var title string
		if err := env.Store.QueryRow(ctx, `SELECT title FROM items WHERE id = ? AND deleted_at IS NULL`, bp.ItemID).Scan(&title); err != nil {
			return nil, command.Errorf("not_found", "no library book %s", bp.ItemID)
		}
		from, to := max(bp.From, 1), bp.To
		if to == 0 {
			res, err := s.reg.Run(ctx, env, "book.open", json.RawMessage(fmt.Sprintf(`{"item_id":%q}`, bp.ItemID)))
			if err != nil {
				return nil, err
			}
			b, _ := json.Marshal(res)
			var o struct {
				Sections int `json:"sections"`
			}
			json.Unmarshal(b, &o)
			to = o.Sections
		}
		if to < from {
			return nil, command.Errorf("bad_params", "%q has no sections %d to %d", title, from, to)
		}
		for n := from; n <= to; n++ {
			out = append(out, unit{weight: sectionWeight, book: &bookPart{item: bp.ItemID, title: title, section: n}})
		}
	}
	return out, nil
}

func (s *service) manualDays(ctx context.Context, env *command.Env, list []ManualDay) ([][]Reading, error) {
	if len(list) > maxDays {
		return nil, command.Errorf("bad_params", "a plan can run for at most %d days", maxDays)
	}
	days := make([][]Reading, 0, len(list))
	for i, md := range list {
		units, err := rangesUnits(md.Refs)
		if err != nil {
			return nil, err
		}
		more, err := s.bookUnits(ctx, env, md.Books)
		if err != nil {
			return nil, err
		}
		units = append(units, more...)
		if len(units) == 0 {
			return nil, command.Errorf("bad_params", "day %d has nothing to read", i+1)
		}
		days = append(days, dedupe(readings(units)))
	}
	return days, nil
}

// dedupe drops a passage listed twice in one day, which would otherwise collide in storage.
func dedupe(rs []Reading) []Reading {
	seen := map[string]bool{}
	out := rs[:0]
	for _, r := range rs {
		k := fmt.Sprint(r.Start, r.ItemID, r.Section)
		if !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	return out
}
