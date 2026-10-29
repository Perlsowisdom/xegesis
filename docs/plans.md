# Reading plans

Plans are made by the commands `plan.templates`, `plan.create`, `plan.list`, `plan.today`, `plan.days`,
`plan.mark`, `plan.unmark`, `plan.reschedule` and `plan.delete`. Every write goes through `store.Write` and
is logged. Plans are included in `export.data` and `import.data` (the `reading_plan` group, including each
day's date).

## Storage

- `reading_plans`: name, start date, `weekdays` (bit 0 Monday to bit 6 Sunday) and the `template` it came from.
- `plan_days`: what to read on each day. A Bible passage has `verse_start` and `verse_end`; a library book
  section has `item_id`, `locator_json` (`{"section": n}`) and a small ordinal in `verse_start` that only
  keeps the key unique.
- `plan_schedule` (migration 0007): the calendar date of each day. Skipped weekdays and rescheduling
  change only these dates.
- `plan_progress`: the days read.

Dates are in the user's local calendar. "Today" is the local date (`date` parameters let scripts ask
about another day).

## Ready-made plans

No published plan is copied. Each is made by one rule: take the chapters of some books in order, then cut
them into runs of consecutive chapters with about the same number of verses. Verse counts come from the
public-domain KJV numbering in `internal/bible/sword/kjv.txt`. A day never splits a chapter, and
neighbouring chapters of one book are shown as one reading ("Genesis 1-3").

| Template | Books | Days |
| --- | --- | --- |
| `bible-year` Whole Bible in a year | Genesis to Revelation (1,189 chapters) | 365 |
| `nt-90` New Testament in 90 days | Matthew to Revelation | 90 |
| `psalms-proverbs-30` Psalms and Proverbs in a month | Psalms 1-150, then Proverbs 1-31 | 30 |
| `gospels-30` Gospels in 30 days | Matthew, Mark, Luke, John | 30 |

The cutting rule: day *i* of *n* ends after the chapter that brings the running total of verses closest
to *i*/*n* of the whole. Every day has at least one chapter.

A chronological plan is left out. No openly licensed, agreed order of events was found that could be
used and checked, and a plan built from a published one risks copying it.

## Your own plans

`plan.create` takes `ranges` (books such as `Mark`, book ranges such as `Gen-Deut`, or passages such as
`Rom 1-8`), optionally `books` (library book sections, each counting as 30 verses), and either `days` or
`end` (the last day; the readings are spread over the reading days up to it). The same rule cuts them up.
`weekdays` limits which days of the week are read. With no length, a plan reads one chapter or section a
day. `manual` lists the days by hand instead.

## Following a plan

- `plan.today` gives each plan's reading for today and, when days before today were not read, the
  `missed` days (the interface shows these as "Catch up").
- `plan.reschedule` gives the days not yet read new dates, one per reading day, starting today (or `from`).
- A plan's expected finish is the date of its last unread day; when it has fallen behind, it is the date
  reading would end if the days left were read from today.
