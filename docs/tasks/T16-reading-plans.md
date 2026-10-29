# T16: Reading plans

Brief for one agent (Sonnet). Read `AGENTS.md` first, especially the design philosophy and plain words.

## What to build

The `reading_plans`, `plan_days` and `plan_progress` tables exist; nothing uses them yet.

1. **Ready-made plans, generated rather than copied.** Plan lists published by others may be copyrighted,
   so Xegesis builds its own from simple, factual rules:
   - Whole Bible in a year;
   - New Testament in 90 days;
   - Psalms and Proverbs in a month;
   - Gospels in 30 days;
   - Chronological (if a public-domain or openly licensed order can be found; otherwise leave it out
     and say so).

   Readings split by chapter, balanced by verse count using the numbering data in `internal/verse` and
   the installed Bibles.
2. **Your own plans:**
   - choose books or ranges, a number of days or an end date, and which weekdays to read;
   - or list the days by hand;
   - a plan can also include library books by section (T26 locators).
3. **Following a plan:**
   - The Reader shows "Today's reading" with one click to open it and one to mark it done.
   - Catching up: a day behind shows "Catch up" with the missed readings. "Reschedule" spreads the
     remaining days from today.
   - Progress shows as days done, days left and the expected finish date.
4. **Commands:**
   - `plan.templates`;
   - `plan.create`, from a template or custom;
   - `plan.list`, `plan.today`, `plan.mark` and `plan.unmark`;
   - `plan.reschedule`, `plan.delete`, `plan.days`.

   Every write goes through `store.Write`.
5. **Interface:**
   - a Plans section, in a new file `views/plans.ts` with minimal wiring in `main.ts`;
   - a small "Today's reading" card in the Reader;
   - a plan's start screen asks only for the plan and the start date; everything else is under
     More options.

## Rules

- Work only in your worktree.
- Another agent (T14, notes and lists) works at the same time:
  - use migration number **0007** if you need one; T14 uses 0006;
  - do not touch notes or lists code.
- Do not commit `frontend/dist`.
- Use a scratch library (`XEGESIS_LIBRARY`).

## Done when

- A Whole Bible in a Year plan created today has 365 balanced days covering every chapter once.
- Today's reading opens and can be marked done.
- Missing two days shows Catch up, and Reschedule moves the rest.
- A custom plan with weekdays only and an end date works.
- `gofmt`, `go vet`, `go test`, the frontend tests and build, and the desktop build pass.
