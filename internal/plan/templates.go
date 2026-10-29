package plan

import (
	"context"

	"github.com/Perlsowisdom/xegesis/internal/command"
)

// Template is a ready-made plan. Each is built from a simple rule over the books of the Bible,
// not copied from any published plan (docs/plans.md).
type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Days        int    `json:"days" desc:"How many reading days the plan has"`
	Rule        string `json:"rule" desc:"How the readings are made"`
	books       []int
}

func seq(from, to int) []int {
	var out []int
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

var templates = []Template{
	{ID: "bible-year", Name: "Whole Bible in a year", Days: 365,
		Description: "Every chapter of the Bible, in the order of the books, one portion a day for a year.",
		Rule:        "Genesis to Revelation, chapters in order, cut into 365 days of about the same number of verses.",
		books:       seq(1, 66)},
	{ID: "nt-90", Name: "New Testament in 90 days", Days: 90,
		Description: "Matthew to Revelation in three months.",
		Rule:        "The 27 New Testament books, chapters in order, cut into 90 days of about the same number of verses.",
		books:       seq(40, 66)},
	{ID: "psalms-proverbs-30", Name: "Psalms and Proverbs in a month", Days: 30,
		Description: "All of Psalms, then all of Proverbs, in thirty days.",
		Rule:        "Psalms 1-150 then Proverbs 1-31, cut into 30 days of about the same number of verses.",
		books:       []int{19, 20}},
	{ID: "gospels-30", Name: "Gospels in 30 days", Days: 30,
		Description: "Matthew, Mark, Luke and John in a month.",
		Rule:        "The four Gospels, chapters in order, cut into 30 days of about the same number of verses.",
		books:       seq(40, 43)},
}

func findTemplate(id string) (Template, bool) {
	for _, t := range templates {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

func templatesCmd(context.Context, *command.Env, struct{}) ([]Template, error) {
	return append([]Template(nil), templates...), nil
}
