import { describe, expect, it } from "vitest";
import { DayInfo, PlanInfo, TodayInfo, behindText, finishText, friendlyDate, handDays, isoDate, localDate, progressText, readingText, splitList, statusText } from "./planwords";

const plan = (over: Partial<PlanInfo> = {}): PlanInfo => ({ id: "P", name: "Gospels", start_date: "2026-03-02", weekdays: [],
  days: 30, days_done: 12, days_left: 18, behind: 0, finish: "2026-03-31", finished: false, ...over });
const now = new Date(2026, 2, 5);

describe("dates", () => {
  it("round trips local dates", () => {
    expect(isoDate(localDate("2026-03-02"))).toBe("2026-03-02");
    expect(isoDate(new Date(2027, 0, 9))).toBe("2027-01-09");
  });
  it("shows the year only when it is not this year", () => {
    expect(friendlyDate("2026-03-02", now)).toBe("Mon 2 Mar");
    expect(friendlyDate("2027-03-01", now)).toContain("2027");
    expect(friendlyDate("")).toBe("");
  });
});

describe("words", () => {
  it("counts days", () => {
    expect(progressText(plan())).toBe("12 days done, 18 days left");
    expect(progressText(plan({ days_done: 29, days_left: 1 }))).toBe("29 days done, 1 day left");
    expect(progressText(plan({ finished: true, days_left: 0 }))).toBe("All 30 days done");
  });
  it("says when it will finish", () => {
    expect(finishText(plan(), now)).toBe("Finish on Tue 31 Mar");
    expect(finishText(plan({ behind: 2 }), now)).toContain("if you read each day from today");
    expect(finishText(plan({ finished: true }), now)).toBe("Finished");
  });
  it("says what was missed", () => {
    expect(behindText(1)).toBe("You missed 1 day.");
    expect(behindText(2)).toBe("You missed 2 days.");
  });
  it("describes where a plan stands", () => {
    const day: DayInfo = { day: 3, date: "2026-03-06", readings: [{ ref: "Mark 1" }, { ref: "Mark 2" }], done: false };
    const t = (status: TodayInfo["status"], over: Partial<TodayInfo> = {}): TodayInfo => ({ plan: plan(), date: "2026-03-05", status, missed: [], ...over });
    expect(statusText(t("today"))).toBe("Today");
    expect(statusText(t("behind", { missed: [day, day] }))).toBe("You missed 2 days.");
    expect(statusText(t("rest", { next: day }), now)).toBe("No reading today. Next: Fri 6 Mar");
    expect(readingText(day)).toBe("Mark 1; Mark 2");
  });
});

describe("typed lists", () => {
  it("splits books and passages", () => {
    expect(splitList("Mark, Rom 1-8;\nPs 23")).toEqual(["Mark", "Rom 1-8", "Ps 23"]);
    expect(splitList("  ")).toEqual([]);
  });
  it("reads days listed by hand", () => {
    expect(handDays("John 1; Ps 23\n\nRom 8")).toEqual([{ refs: ["John 1", "Ps 23"] }, { refs: ["Rom 8"] }]);
  });
});
