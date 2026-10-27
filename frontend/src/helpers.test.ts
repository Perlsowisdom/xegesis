import { describe, expect, it } from "vitest";
import {
  alignColumns, changedFields, errorText, exampleParams, markSegments, neighbourChapters, pageLabel, parseError,
  splitList, verseParts,
} from "./helpers";

describe("parseError", () => {
  it("reads code: message strings", () => {
    expect(parseError("not_found: no note 7")).toEqual({ code: "not_found", message: "no note 7" });
  });
  it("keeps unknown text whole", () => {
    expect(parseError(new Error("boom: x"))).toEqual({ code: "failed", message: "boom: x" });
  });
  it("reads objects", () => {
    expect(parseError({ code: "bad_params", message: "m" })).toEqual({ code: "bad_params", message: "m" });
  });
});

it("errorText drops the command prefix", () => {
  expect(errorText({ code: "bad_params", message: "note.create: ref is required" }))
    .toBe("Please check the details: ref is required");
});

it("exampleParams fills required fields", () => {
  const s = { type: "object", required: ["a", "b"], properties: { a: { type: "string" }, b: { type: "integer" }, c: { type: "string" } } };
  expect(JSON.parse(exampleParams(s))).toEqual({ a: "", b: 0 });
});

it("verseParts splits BBCCCVVV", () => {
  expect(verseParts(45008028)).toEqual({ book: 45, chapter: 8, verse: 28 });
});

describe("neighbourChapters", () => {
  const books = [
    { number: 1, osis: "Gen", name: "Genesis", chapters: 50 },
    { number: 2, osis: "Exod", name: "Exodus", chapters: 40 },
  ];
  it("moves within a book", () => {
    expect(neighbourChapters(1002001, books)).toEqual({ prev: "Gen 1", next: "Gen 3" });
  });
  it("crosses books and stops at the ends", () => {
    expect(neighbourChapters(1050001, books)).toEqual({ prev: "Gen 49", next: "Exod 1" });
    expect(neighbourChapters(1001001, books).prev).toBeNull();
    expect(neighbourChapters(2040001, books).next).toBeNull();
  });
});

it("alignColumns matches verses by base ID", () => {
  const v = (id: number) => ({ id, base_id: id, end: 0, ref: "", text: String(id) });
  const rows = alignColumns([
    { module: "a", ref: "", attribution: "", verses: [v(1), v(2)] },
    { module: "b", ref: "", attribution: "", verses: [v(2), v(3)] },
  ]);
  expect(rows.map((r) => r.base)).toEqual([1, 2, 3]);
  expect(rows[0].cells[1]).toBeNull();
  expect(rows[1].cells.map((c) => c?.text)).toEqual(["2", "2"]);
});

it("splitList and changedFields", () => {
  expect(splitList("A, B\n\nC")).toEqual(["A", "B", "C"]);
  expect(changedFields({ a: "x", b: 1 }, { a: "x", b: 2 })).toEqual({ b: 2 });
});

it("pageLabel", () => {
  expect(pageLabel(0, 0, 0)).toBe("No items");
  expect(pageLabel(50, 50, 120)).toBe("51–100 of 120");
});

it("markSegments", () => {
  expect(markSegments("a <mark>b</mark> c")).toEqual([
    { text: "a ", mark: false }, { text: "b", mark: true }, { text: " c", mark: false },
  ]);
});
