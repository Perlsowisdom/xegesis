// Package plan makes and follows reading plans: ready-made ones built from simple rules, plans of
// the user's own, and the day-by-day record of what has been read.
package plan

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Perlsowisdom/xegesis/internal/command"
	"github.com/Perlsowisdom/xegesis/internal/store"
	"github.com/Perlsowisdom/xegesis/internal/verse"
)

// Register adds the reading plan commands to r.
func Register(r *command.Registry) { register(r, &service{reg: r, today: localToday}) }

func register(r *command.Registry, s *service) {
	r.Add(
		command.Define("plan.templates", "List the ready-made reading plans and the rule each is built from.", false, templatesCmd),
		command.Define("plan.create", "Start a reading plan from a ready-made one or from books, ranges and days of your own.", true, s.create),
		command.Define("plan.list", "List reading plans with days done, days left and the expected finish.", false, s.list),
		command.Define("plan.today", "Show today's reading for each plan, and the days missed if a plan has fallen behind.", false, s.todayCmd),
		command.Define("plan.days", "List every day of a plan with its readings and whether it is done.", false, s.daysCmd),
		command.Define("plan.mark", "Mark days of a plan as read. With no day given, today's reading, or else the first one not yet read.", true, s.mark),
		command.Define("plan.unmark", "Mark a day of a plan as not read.", true, s.unmark),
		command.Define("plan.reschedule", "Spread the days not yet read over the coming days, starting today.", true, s.reschedule),
		command.Define("plan.delete", "Delete a reading plan. It can be restored from its history.", true, s.delete),
	)
}

type service struct {
	reg   *command.Registry
	today func() string // the local date, YYYY-MM-DD; tests replace it
}

func localToday() string { return time.Now().Format(dateLayout) }

func now() int64 { return time.Now().UnixMilli() }

func needStore(env *command.Env) error {
	if env.Store == nil {
		return command.Errorf("unavailable", "no library is open")
	}
	return nil
}

// Day is one day of a plan.
type Day struct {
	Day         int       `json:"day" desc:"The day's number in the plan, from 1"`
	Date        string    `json:"date" desc:"The day it is scheduled for, YYYY-MM-DD"`
	Readings    []Reading `json:"readings"`
	Done        bool      `json:"done"`
	CompletedAt int64     `json:"completed_at,omitempty"`
}

// Plan is a reading plan with its progress.
type Plan struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Template  string   `json:"template,omitempty"`
	StartDate string   `json:"start_date"`
	Weekdays  []string `json:"weekdays" desc:"The days of the week it is read on"`
	Days      int      `json:"days" desc:"Days in the plan"`
	DaysDone  int      `json:"days_done"`
	DaysLeft  int      `json:"days_left"`
	Behind    int      `json:"behind" desc:"Days whose date has passed without being read"`
	Finish    string   `json:"finish,omitempty" desc:"Expected finish date; when behind, the date if reading carries on from today"`
	Finished  bool     `json:"finished"`
}

func (s *service) loadPlan(ctx context.Context, env *command.Env, id, today string) (Plan, error) {
	ps, err := s.plans(ctx, env, id, today)
	if err != nil {
		return Plan{}, err
	}
	if len(ps) == 0 {
		return Plan{}, command.Errorf("not_found", "no reading plan %s", id)
	}
	return ps[0], nil
}

// plans loads one plan (id given) or all of them, newest first.
func (s *service) plans(ctx context.Context, env *command.Env, id, today string) ([]Plan, error) {
	q := `SELECT p.id, p.name, COALESCE(p.template, ''), COALESCE(p.start_date, ''), p.weekdays,
		(SELECT COUNT(*) FROM plan_schedule WHERE plan_id = p.id),
		(SELECT COUNT(*) FROM plan_progress WHERE plan_id = p.id),
		(SELECT COUNT(*) FROM plan_schedule s WHERE s.plan_id = p.id AND s.day_date < ?
			AND NOT EXISTS (SELECT 1 FROM plan_progress g WHERE g.plan_id = s.plan_id AND g.day_no = s.day_no)),
		COALESCE((SELECT MAX(s.day_date) FROM plan_schedule s WHERE s.plan_id = p.id
			AND NOT EXISTS (SELECT 1 FROM plan_progress g WHERE g.plan_id = s.plan_id AND g.day_no = s.day_no)), '')
		FROM reading_plans p WHERE p.deleted_at IS NULL`
	args := []any{today}
	if id != "" {
		q += ` AND p.id = ?`
		args = append(args, id)
	}
	rows, err := env.Store.Query(ctx, q+` ORDER BY p.created_at DESC, p.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		var p Plan
		var mask int
		var last string
		if err := rows.Scan(&p.ID, &p.Name, &p.Template, &p.StartDate, &mask, &p.Days, &p.DaysDone, &p.Behind, &last); err != nil {
			return nil, err
		}
		p.Weekdays = maskNames(mask)
		p.DaysLeft = p.Days - p.DaysDone
		p.Finished = p.DaysLeft == 0
		p.Finish = finish(last, p.Behind, p.DaysLeft, mask, today)
		out = append(out, p)
	}
	return out, rows.Err()
}

// finish is the scheduled date of the last unread day, or, for a plan that has fallen behind, the
// date reading would end if the days left were read on the plan's weekdays starting today.
func finish(last string, behind, left, mask int, today string) string {
	if left == 0 {
		return ""
	}
	if behind == 0 {
		return last
	}
	t, err := parseDate(today)
	if err != nil {
		return last
	}
	d := nextDates(t, mask, left)
	return d[len(d)-1]
}

type ListParams struct {
	Date string `json:"date,omitempty" desc:"Count as of this day instead of today, YYYY-MM-DD"`
}

func (s *service) list(ctx context.Context, env *command.Env, p ListParams) ([]Plan, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	today, err := s.asOf(p.Date)
	if err != nil {
		return nil, err
	}
	return s.plans(ctx, env, "", today)
}

func (s *service) asOf(date string) (string, error) {
	if date == "" {
		return s.today(), nil
	}
	if _, err := parseDate(date); err != nil {
		return "", err
	}
	return date, nil
}

// Days ------------------------------------------------------------------------------------------

func (s *service) loadDays(ctx context.Context, env *command.Env, planID string) ([]Day, error) {
	rows, err := env.Store.Query(ctx, `SELECT s.day_no, s.day_date, g.completed_at, d.verse_start, d.verse_end,
		d.item_id, i.title, d.locator_json
		FROM plan_schedule s
		LEFT JOIN plan_progress g ON g.plan_id = s.plan_id AND g.day_no = s.day_no
		LEFT JOIN plan_days d ON d.plan_id = s.plan_id AND d.day_no = s.day_no
		LEFT JOIN items i ON i.id = d.item_id
		WHERE s.plan_id = ? ORDER BY s.day_no, (d.item_id IS NOT NULL), d.verse_start`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	days := []Day{}
	for rows.Next() {
		var no int
		var date string
		var done, vs, ve sql.NullInt64
		var item, title, loc sql.NullString
		if err := rows.Scan(&no, &date, &done, &vs, &ve, &item, &title, &loc); err != nil {
			return nil, err
		}
		if len(days) == 0 || days[len(days)-1].Day != no {
			days = append(days, Day{Day: no, Date: date, Done: done.Valid, CompletedAt: done.Int64, Readings: []Reading{}})
		}
		d := &days[len(days)-1]
		switch {
		case item.Valid:
			var l struct {
				Section int `json:"section"`
			}
			if json.Unmarshal([]byte(loc.String), &l) != nil || l.Section < 1 {
				l.Section = 1
			}
			t := title.String
			if t == "" {
				t = "a book"
			}
			d.Readings = append(d.Readings, Reading{Ref: fmt.Sprintf("%s, section %d", t, l.Section),
				ItemID: item.String, Section: l.Section, Title: title.String})
		case vs.Valid && ve.Valid:
			d.Readings = append(d.Readings, bibleReading(verse.ID(vs.Int64), verse.ID(ve.Int64)))
		}
	}
	return days, rows.Err()
}

type PlanParams struct {
	PlanID string `json:"plan_id" required:"true" desc:"The plan, from plan.list"`
}

func (s *service) daysCmd(ctx context.Context, env *command.Env, p PlanParams) ([]Day, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	if _, err := s.loadPlan(ctx, env, p.PlanID, s.today()); err != nil {
		return nil, err
	}
	return s.loadDays(ctx, env, p.PlanID)
}

// Today -----------------------------------------------------------------------------------------

// Today is where one plan stands on a day.
type Today struct {
	Plan   Plan   `json:"plan"`
	Date   string `json:"date"`
	Status string `json:"status" enum:"today,done_for_today,rest,upcoming,behind,finished" desc:"today: a reading is due; done_for_today: today's reading is read; rest: nothing is scheduled today; upcoming: the plan has not started; behind: days were missed; finished"`
	Today  *Day   `json:"today,omitempty" desc:"The reading scheduled for today"`
	Next   *Day   `json:"next,omitempty" desc:"The next day not yet read, when today has no reading to do"`
	Missed []Day  `json:"missed" desc:"Days before today that were not read, oldest first (the ones to catch up)"`
}

type TodayParams struct {
	PlanID string `json:"plan_id,omitempty" desc:"Only this plan; otherwise every plan not yet finished"`
	Date   string `json:"date,omitempty" desc:"Show as of this day instead of today, YYYY-MM-DD"`
}

func (s *service) todayCmd(ctx context.Context, env *command.Env, p TodayParams) ([]Today, error) {
	if err := needStore(env); err != nil {
		return nil, err
	}
	date, err := s.asOf(p.Date)
	if err != nil {
		return nil, err
	}
	plans, err := s.plans(ctx, env, p.PlanID, date)
	if err != nil {
		return nil, err
	}
	if p.PlanID != "" && len(plans) == 0 {
		return nil, command.Errorf("not_found", "no reading plan %s", p.PlanID)
	}
	out := []Today{}
	for _, pl := range plans {
		if p.PlanID == "" && pl.Finished {
			continue
		}
		days, err := s.loadDays(ctx, env, pl.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, whereItStands(pl, days, date))
	}
	return out, nil
}

func whereItStands(pl Plan, days []Day, date string) Today {
	t := Today{Plan: pl, Date: date, Missed: []Day{}}
	var next *Day
	for i := range days {
		d := &days[i]
		switch {
		case d.Date == date:
			t.Today = d
		case !d.Done && d.Date < date:
			t.Missed = append(t.Missed, *d)
		case !d.Done && d.Date > date && next == nil:
			next = d
		}
	}
	switch {
	case pl.Finished:
		t.Status = "finished"
	case len(t.Missed) > 0:
		t.Status = "behind"
	case t.Today != nil && !t.Today.Done:
		t.Status = "today"
	case t.Today != nil:
		t.Status = "done_for_today"
	case pl.StartDate > date:
		t.Status = "upcoming"
	default:
		t.Status = "rest"
	}
	if t.Today == nil || t.Today.Done {
		t.Next = next
	}
	return t
}

// Marking ---------------------------------------------------------------------------------------

type MarkParams struct {
	PlanID string `json:"plan_id" required:"true" desc:"The plan, from plan.list"`
	Days   []int  `json:"days,omitempty" desc:"Day numbers to mark; omit for today's reading, or the first day not yet read"`
	Date   string `json:"date,omitempty" desc:"Treat this as today, YYYY-MM-DD"`
}

func (s *service) mark(ctx context.Context, env *command.Env, p MarkParams) (Plan, error) {
	if err := needStore(env); err != nil {
		return Plan{}, err
	}
	date, err := s.asOf(p.Date)
	if err != nil {
		return Plan{}, err
	}
	days, err := s.loadDays(ctx, env, p.PlanID)
	if err != nil {
		return Plan{}, err
	}
	if len(days) == 0 {
		return Plan{}, command.Errorf("not_found", "no reading plan %s", p.PlanID)
	}
	nums := p.Days
	if len(nums) == 0 {
		n := defaultDay(days, date)
		if n == 0 {
			return Plan{}, command.Errorf("bad_params", "every day of this plan is already read")
		}
		nums = []int{n}
	}
	for _, n := range nums {
		if n < 1 || n > len(days) {
			return Plan{}, command.Errorf("bad_params", "this plan has days 1 to %d, not %d", len(days), n)
		}
	}
	err = env.Store.Write(ctx, "plan.mark", func(tx *store.Tx) error {
		for _, n := range nums {
			key := store.Row{"plan_id": p.PlanID, "day_no": int64(n)}
			if _, err := tx.Get("plan_progress", key); err == nil {
				continue
			}
			if err := tx.Create("plan_progress", store.Row{"plan_id": p.PlanID, "day_no": int64(n), "completed_at": now()}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Plan{}, err
	}
	return s.loadPlan(ctx, env, p.PlanID, date)
}

// defaultDay is today's day if it is unread, otherwise the first unread day.
func defaultDay(days []Day, date string) int {
	first := 0
	for _, d := range days {
		if d.Done {
			continue
		}
		if d.Date == date {
			return d.Day
		}
		if first == 0 {
			first = d.Day
		}
	}
	return first
}

type UnmarkParams struct {
	PlanID string `json:"plan_id" required:"true" desc:"The plan, from plan.list"`
	Day    int    `json:"day" required:"true" desc:"The day number to mark as not read"`
}

func (s *service) unmark(ctx context.Context, env *command.Env, p UnmarkParams) (Plan, error) {
	if err := needStore(env); err != nil {
		return Plan{}, err
	}
	err := env.Store.Write(ctx, "plan.unmark", func(tx *store.Tx) error {
		key := store.Row{"plan_id": p.PlanID, "day_no": int64(p.Day)}
		if _, err := tx.Get("plan_progress", key); err != nil {
			return nil // already not read
		}
		return tx.Delete("plan_progress", key)
	})
	if err != nil {
		return Plan{}, err
	}
	return s.loadPlan(ctx, env, p.PlanID, s.today())
}

// Rescheduling ----------------------------------------------------------------------------------

type RescheduleParams struct {
	PlanID string `json:"plan_id" required:"true" desc:"The plan, from plan.list"`
	From   string `json:"from,omitempty" desc:"First day to read again, YYYY-MM-DD; default today"`
}

func (s *service) reschedule(ctx context.Context, env *command.Env, p RescheduleParams) (Plan, error) {
	if err := needStore(env); err != nil {
		return Plan{}, err
	}
	from, err := s.asOf(p.From)
	if err != nil {
		return Plan{}, err
	}
	start, _ := parseDate(from)
	var mask int
	if err := env.Store.QueryRow(ctx, `SELECT weekdays FROM reading_plans WHERE id = ? AND deleted_at IS NULL`, p.PlanID).Scan(&mask); err != nil {
		return Plan{}, command.Errorf("not_found", "no reading plan %s", p.PlanID)
	}
	days, err := s.loadDays(ctx, env, p.PlanID)
	if err != nil {
		return Plan{}, err
	}
	var left []int
	for _, d := range days {
		if !d.Done {
			left = append(left, d.Day)
		}
	}
	sort.Ints(left)
	dates := nextDates(start, mask, len(left))
	err = env.Store.Write(ctx, "plan.reschedule", func(tx *store.Tx) error {
		for i, n := range left {
			key := store.Row{"plan_id": p.PlanID, "day_no": int64(n)}
			if err := tx.Update("plan_schedule", key, store.Row{"day_date": dates[i]}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Plan{}, err
	}
	return s.loadPlan(ctx, env, p.PlanID, from)
}

type IDParams struct {
	ID string `json:"id" required:"true" desc:"The plan, from plan.list"`
}

func (s *service) delete(ctx context.Context, env *command.Env, p IDParams) (struct{}, error) {
	if err := needStore(env); err != nil {
		return struct{}{}, err
	}
	return struct{}{}, env.Store.Write(ctx, "plan.delete", func(tx *store.Tx) error {
		return tx.Delete("reading_plans", store.ByID(p.ID))
	})
}
