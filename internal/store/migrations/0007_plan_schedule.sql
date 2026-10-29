-- library.db, schema version 7.
-- Reading plans: each day has a calendar date (so plans can skip weekdays and be rescheduled),
-- and a plan remembers which weekdays it reads on and the template it came from.

CREATE TABLE plan_schedule (
    plan_id   TEXT NOT NULL REFERENCES reading_plans(id),
    day_no    INTEGER NOT NULL,
    day_date  TEXT NOT NULL,                    -- ISO date, in the user's local calendar
    PRIMARY KEY (plan_id, day_no)
);

CREATE INDEX plan_schedule_by_date ON plan_schedule(plan_id, day_date);

ALTER TABLE reading_plans ADD COLUMN weekdays INTEGER NOT NULL DEFAULT 127; -- bit 0 Monday ... bit 6 Sunday
ALTER TABLE reading_plans ADD COLUMN template TEXT;

PRAGMA user_version = 7;
