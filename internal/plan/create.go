package plan

import (
	"context"
	"encoding/json"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
)

// BookPart is some sections of a library book.
type BookPart struct {
	ItemID string `json:"item_id" required:"true" desc:"The library book"`
	From   int    `json:"from,omitempty" desc:"First section (default 1)"`
	To     int    `json:"to,omitempty" desc:"Last section (default: the last one)"`
}

// ManualDay is one day of a plan written by hand.
type ManualDay struct {
	Refs  []string   `json:"refs,omitempty" desc:"Bible passages to read that day, e.g. [\"Rom 1-3\", \"Ps 23\"]"`
	Books []BookPart `json:"books,omitempty" desc:"Sections of library books to read that day"`
}

type CreateParams struct {
	Template string      `json:"template,omitempty" desc:"A ready-made plan from plan.templates"`
	Name     string      `json:"name,omitempty" desc:"The plan's name; defaults to the template's name"`
	Start    string      `json:"start,omitempty" desc:"First day to read, YYYY-MM-DD; default today"`
	Ranges   []string    `json:"ranges,omitempty" desc:"For your own plan: books or passages to read in order, e.g. [\"Mark\", \"Rom 1-8\", \"Gen-Deut\"]"`
	Books    []BookPart  `json:"books,omitempty" desc:"For your own plan: sections of library books, read after the passages"`
	Manual   []ManualDay `json:"manual,omitempty" desc:"Instead of ranges: list the days by hand, one entry per day"`
	Days     int         `json:"days,omitempty" desc:"How many reading days to spread the readings over"`
	End      string      `json:"end,omitempty" desc:"Or the last day, YYYY-MM-DD; the readings are spread over the reading days up to it"`
	Weekdays []string    `json:"weekdays,omitempty" desc:"Days of the week to read on, e.g. [\"mon\",\"tue\",\"wed\",\"thu\",\"fri\"]; default every day"`
}

func (s *service) create(ctx context.Context, env *command.Env, p CreateParams) (Plan, error) {
	if err := needStore(env); err != nil {
		return Plan{}, err
	}
	start := p.Start
	if start == "" {
		start = s.today()
	}
	from, err := parseDate(start)
	if err != nil {
		return Plan{}, err
	}
	mask, err := weekdayMask(p.Weekdays)
	if err != nil {
		return Plan{}, err
	}
	days, name, err := s.build(ctx, env, p, from, mask)
	if err != nil {
		return Plan{}, err
	}
	if p.Name != "" {
		name = p.Name
	}
	dates := nextDates(from, mask, len(days))
	id := store.NewID()
	err = env.Store.Write(ctx, "plan.create", func(tx *store.Tx) error {
		err := tx.Create("reading_plans", store.Row{"id": id, "name": name, "start_date": start,
			"weekdays": int64(mask), "template": nullable(p.Template), "created_at": now()})
		if err != nil {
			return err
		}
		for i, d := range days {
			no := int64(i + 1)
			if err := tx.Create("plan_schedule", store.Row{"plan_id": id, "day_no": no, "day_date": dates[i]}); err != nil {
				return err
			}
			for j, r := range d {
				row := store.Row{"plan_id": id, "day_no": no}
				if r.ItemID != "" {
					loc, _ := json.Marshal(map[string]int{"section": r.Section})
					// A book reading has no verses; its position in the day keeps the key unique.
					row["verse_start"], row["item_id"], row["locator_json"] = int64(j+1), r.ItemID, string(loc)
				} else {
					row["verse_start"], row["verse_end"] = int64(r.Start), int64(r.End)
				}
				if err := tx.Create("plan_days", row); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return Plan{}, err
	}
	return s.loadPlan(ctx, env, id, s.today())
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
