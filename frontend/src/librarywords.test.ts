import { describe, expect, it } from "vitest";
import { NOT_SET } from "./facetwords";
import {
  Book, Filters, NO_YEAR, activeCount, applyFilters, chips, countLine, coverHue, eraOf, initials, matches, options, toggle, valuesOf,
} from "./librarywords";

const books: Book[] = [
  { id: "1", title: "Institutes", authors: ["John Calvin"], year: 1559, type: "Theology", tradition: "Reformed", licence_name: "Public domain" },
  { id: "2", title: "Romans", authors: ["John Calvin"], year: 1540, type: "Commentary (New Testament)", tradition: "Reformed", licence_name: "Public domain" },
  { id: "3", title: "KJV", authors: [], year: 1611, type: "Bible", licence_name: "Public domain" },
  { id: "4", title: "Small Catechism", authors: ["Martin Luther"], year: 1529, type: "Theology", tradition: "Lutheran", licence_name: "Free to share" },
  { id: "5", title: "Notes on Mark", authors: ["A. Writer", "B. Helper"], type: "Commentary (New Testament)", tradition: "Reformed", licence_name: "Personal copy" },
];

const ids = (bs: Book[]) => bs.map((b) => b.id);

describe("filter combination", () => {
  it("passes everything with no filters", () => {
    expect(ids(applyFilters(books, {}))).toEqual(["1", "2", "3", "4", "5"]);
  });
  it("takes any chosen value within a facet (OR)", () => {
    expect(ids(applyFilters(books, { type: ["Bible", "Commentary (New Testament)"] }))).toEqual(["2", "3", "5"]);
  });
  it("needs every facet across them (AND)", () => {
    const f: Filters = { type: ["Bible", "Commentary (New Testament)"], tradition: ["Reformed"] };
    expect(ids(applyFilters(books, f))).toEqual(["2", "5"]);
    expect(ids(applyFilters(books, { ...f, era: ["1500 to 1699"] }))).toEqual(["2"]);
  });
  it("matches any of several authors and the missing values", () => {
    expect(ids(applyFilters(books, { author: ["B. Helper"] }))).toEqual(["5"]);
    expect(ids(applyFilters(books, { tradition: [NOT_SET] }))).toEqual(["3"]);
    expect(ids(applyFilters(books, { era: [NO_YEAR] }))).toEqual(["5"]);
  });
  it("combines with the words typed", () => {
    expect(ids(applyFilters(books, { tradition: ["Reformed"] }, "calvin rom"))).toEqual(["2"]);
  });
  it("can leave one facet out", () => {
    expect(matches(books[2], { type: ["Theology"], licence: ["Public domain"] }, "type")).toBe(true);
    expect(matches(books[2], { type: ["Theology"], licence: ["Public domain"] })).toBe(false);
  });
});

describe("options", () => {
  it("counts each value given the other facets", () => {
    const got = options(books, "type", { tradition: ["Reformed"] }, ["Bible", "Commentary (New Testament)", "Theology"]);
    expect(got).toEqual([{ value: "Commentary (New Testament)", count: 2 }, { value: "Theology", count: 1 }]);
  });
  it("keeps a chosen value at zero and puts unknowns last", () => {
    const got = options(books, "tradition", { type: ["Bible"], tradition: ["Lutheran"] });
    expect(got).toEqual([{ value: "Lutheran", count: 0 }, { value: NOT_SET, count: 1 }]);
  });
  it("orders eras oldest first", () => {
    expect(options(books, "era", {}).map((o) => o.value)).toEqual(["1500 to 1699", NO_YEAR]);
  });
  it("orders authors by name", () => {
    expect(options(books, "author", {}).map((o) => o.value)).toEqual(["A. Writer", "B. Helper", "John Calvin", "Martin Luther", "No author given"]);
  });
});

describe("toggle, chips and counts", () => {
  it("adds and removes a value without changing the original", () => {
    const a: Filters = {};
    const b = toggle(a, "type", "Bible", true);
    const c = toggle(b, "type", "Theology", true);
    expect(a).toEqual({});
    expect(c).toEqual({ type: ["Bible", "Theology"] });
    expect(toggle(toggle(c, "type", "Bible", false), "type", "Theology", false)).toEqual({});
    expect(activeCount(c)).toBe(2);
  });
  it("labels chips in facet order", () => {
    expect(chips({ era: ["Before 1500"], type: ["Bible"], author: ["John Calvin"] }).map((c) => c.label))
      .toEqual(["Bible", "Author: John Calvin", "Era: Before 1500"]);
  });
  it("reads the count line", () => {
    expect(countLine(12, 140)).toBe("12 of 140 books");
    expect(countLine(140, 140)).toBe("140 books");
    expect(countLine(1, 1)).toBe("1 book");
    expect(countLine(0, 1200)).toBe("0 of 1,200 books");
  });
});

describe("eras, values and covers", () => {
  it("puts years in eras", () => {
    expect([1200, 1500, 1699, 1700, 1950, 2024, 0].map((y) => eraOf(y))).toEqual(
      ["Before 1500", "1500 to 1699", "1500 to 1699", "1700 to 1899", "1900 to 1999", "2000 and later", NO_YEAR]);
    expect(valuesOf(books[2], "author")).toEqual(["No author given"]);
  });
  it("draws steady cover colours and marks", () => {
    expect(coverHue(books[0])).toBe(coverHue({ title: "Institutes", type: "Theology" }));
    expect(coverHue(books[0])).toBeGreaterThanOrEqual(0);
    expect(initials("The Institutes of the Christian Religion")).toBe("IC");
    expect(initials("KJV")).toBe("K");
  });
});
