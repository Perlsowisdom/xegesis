import { describe, expect, it } from "vitest";
import { chapterOsis, chapterToggle } from "./chapterwords";

const books = [{ number: 45, osis: "Rom", name: "Romans", chapters: 16 }];
const chap = { start: 45008001, end: 45008039 };

describe("chapterToggle", () => {
  it("offers the whole chapter for a verse or range", () => {
    expect(chapterToggle({ start: 45008028, end: 45008028 }, chap, false, books)).toEqual({ label: "Show all of Romans 8", expand: true });
    expect(chapterToggle({ start: 45008028, end: 45008030 }, chap, false, books)?.expand).toBe(true);
  });
  it("offers the way back when the chapter is shown", () => {
    expect(chapterToggle({ start: 45008028, end: 45008028 }, chap, true, books)).toEqual({ label: "Show only Romans 8:28", expand: false });
    expect(chapterToggle({ start: 45008028, end: 45008030 }, chap, true, books)?.label).toBe("Show only Romans 8:28-30");
  });
  it("has no button for a whole chapter or several chapters", () => {
    expect(chapterToggle(chap, chap, false, books)).toBeNull();
    expect(chapterToggle({ start: 45008028, end: 45009002 }, null, false, books)).toBeNull();
  });
});

describe("chapterOsis", () => {
  it("names the chapter or none across chapters", () => {
    expect(chapterOsis({ start: 45008028, end: 45008030 }, books)).toBe("Rom 8");
    expect(chapterOsis({ start: 45008028, end: 45009002 }, books)).toBeNull();
  });
});
