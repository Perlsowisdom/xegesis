import { describe, expect, it } from "vitest";
import { NodeLike, Painted, cover, nodeAt, paintFor, partsOf, pointOf, tidyRange, unitOffset } from "./verserange";

// A tiny stand-in for the DOM: elements with children, and text nodes.
type N = NodeLike & { childNodes: N[] };
const text = (data: string): N => ({ nodeType: 3, data, childNodes: [], parentNode: null });
function el(kids: N[], dataset: Record<string, string> = {}): N {
  const e: N = { nodeType: 1, childNodes: kids, parentNode: null, dataset };
  for (const k of kids) k.parentNode = e;
  return e;
}

// Romans 8:28 drawn as the page draws it: a verse number, then the text with a tagged word and
// words of Christ inside it.
function verse28() {
  const and = text("And we ");
  const know = text("know");
  const rest = text(" that all things");
  const vt = el([and, el([know], { strongs: "G1492" }), el([rest])], { tb: "45008028" });
  const num = el([text("28")]);
  const v = el([num, text(" "), vt, text(" ")]);
  return { v, vt, and, know, rest, num };
}

describe("a selection's place in a verse", () => {
  it("counts characters of the verse's text, through nested words", () => {
    const { and, know, rest } = verse28();
    expect(pointOf(and, 4)).toEqual({ verse: 45008028, offset: 4 });
    expect(pointOf(know, 0)).toEqual({ verse: 45008028, offset: 7 });
    expect(pointOf(know, 4)).toEqual({ verse: 45008028, offset: 11 });
    expect(pointOf(rest, 5)).toEqual({ verse: 45008028, offset: 16 });
  });
  it("reads an element and child index as a selection gives them", () => {
    const { vt } = verse28();
    expect(pointOf(vt, 0)).toEqual({ verse: 45008028, offset: 0 });
    expect(pointOf(vt, 2)).toEqual({ verse: 45008028, offset: 11 });
    expect(pointOf(vt, 3)).toEqual({ verse: 45008028, offset: 27 });
  });
  it("is nothing outside the verse's text, such as on the verse number", () => {
    const { num, v } = verse28();
    expect(pointOf(num.childNodes[0], 1)).toBeNull();
    expect(pointOf(v.childNodes[1], 0)).toBeNull();
  });
  it("counts letters outside the basic plane as one character", () => {
    const t = text("a\u{1D400}b c");
    el([t], { tb: "1001001" });
    expect(pointOf(t, 3)).toEqual({ verse: 1001001, offset: 2 });
    expect(unitOffset("a\u{1D400}b", 2)).toBe(3);
  });
});

describe("back from a place to the page", () => {
  it("finds the text node and index for an offset", () => {
    const { vt, and, know, rest } = verse28();
    expect(nodeAt(vt, 0)).toEqual({ node: and, offset: 0 });
    expect(nodeAt(vt, 9)).toEqual({ node: know, offset: 2 });
    expect(nodeAt(vt, 16)).toEqual({ node: rest, offset: 5 });
    expect(nodeAt(vt, 999)).toEqual({ node: rest, offset: rest.data!.length });
  });
  it("round-trips every offset", () => {
    const { vt } = verse28();
    for (let i = 0; i <= 27; i++) {
      const at = nodeAt(vt, i)!;
      expect(pointOf(at.node, at.offset)!.offset).toBe(i);
    }
  });
});

describe("tidying a selection's range", () => {
  const verses = [{ verse: 45008028, length: 27 }, { verse: 45008029, length: 24 }];
  const p = (verse: number, offset: number) => ({ verse, offset });
  it("drops a verse the selection only touches at its edge", () => {
    expect(tidyRange({ start: p(45008028, 4), end: p(45008029, 0) }, verses)).toEqual({ start: p(45008028, 4), end: p(45008028, 27) });
    expect(tidyRange({ start: p(45008028, 27), end: p(45008029, 3) }, verses)).toEqual({ start: p(45008029, 0), end: p(45008029, 3) });
  });
  it("is nothing when no character is selected", () => {
    expect(tidyRange({ start: p(45008028, 27), end: p(45008029, 0) }, verses)).toBeNull();
  });
});

describe("the words of a range", () => {
  const texts = [{ verse: 45008028, text: "And we know that all things" }, { verse: 45008029, text: "For whom he did foreknow" }];
  it("cuts one verse's words", () => {
    expect(partsOf({ start: { verse: 45008028, offset: 7 }, end: { verse: 45008028, offset: 16 } }, texts))
      .toEqual([{ verse: 45008028, text: "know that" }]);
  });
  it("cuts words across two verses, starting and ending mid-verse", () => {
    expect(partsOf({ start: { verse: 45008028, offset: 21 }, end: { verse: 45008029, offset: 8 } }, texts))
      .toEqual([{ verse: 45008028, text: "things" }, { verse: 45008029, text: "For whom" }]);
  });
});

describe("painting a verse", () => {
  const r = (a: number, b: number, v = 45008028): Painted["range"] => ({ start: { verse: v, offset: a }, end: { verse: v, offset: b } });
  const hl = (id: string, created_at: number, extra: Partial<Painted>): Painted => ({ id, style: "yellow", created_at, verse_start: 45008028, verse_end: 45008028, ...extra });
  it("draws the highlighted words only", () => {
    const p = paintFor([hl("a", 1, { range: r(7, 16), module: "KJV" })], 45008028, 27, "KJV");
    expect(p.whole).toBeNull();
    expect(p.segments.map((s) => [s.start, s.end, s.hl.id])).toEqual([[7, 16, "a"]]);
  });
  it("lets the newer highlight win where two overlap", () => {
    const segs = cover([{ start: 0, end: 20, hl: hl("old", 1, {}) }], { start: 5, end: 10, hl: hl("new", 2, {}) });
    expect(segs.map((s) => [s.start, s.end, s.hl.id])).toEqual([[0, 5, "old"], [5, 10, "new"], [10, 20, "old"]]);
  });
  it("keeps an older whole-verse highlight on the verse, until words in it are highlighted", () => {
    const whole = hl("w", 1, {});
    expect(paintFor([whole], 45008028, 27, "KJV")).toEqual({ whole, segments: [], faint: null });
    const p = paintFor([whole, hl("a", 2, { range: r(7, 16), module: "KJV" })], 45008028, 27, "KJV");
    expect(p.whole).toBeNull();
    expect(p.segments.map((s) => [s.start, s.end, s.hl.id])).toEqual([[0, 7, "w"], [7, 16, "a"], [16, 27, "w"]]);
    // A whole-verse highlight made later covers older words.
    expect(paintFor([hl("a", 1, { range: r(7, 16), module: "KJV" }), hl("w", 2, {})], 45008028, 27, "KJV").whole?.id).toBe("w");
  });
  it("shows words highlighted in another Bible on the whole verse, faintly", () => {
    const other = hl("a", 1, { range: r(7, 16), module: "WEB" });
    expect(paintFor([other], 45008028, 27, "KJV")).toEqual({ whole: null, segments: [], faint: other });
  });
  it("runs a range spanning verses to the end of the first and from the start of the next", () => {
    const span = hl("s", 1, { range: { start: { verse: 45008028, offset: 21 }, end: { verse: 45008029, offset: 8 } }, module: "KJV", verse_end: 45008029 });
    expect(paintFor([span], 45008028, 27, "KJV").segments.map((s) => [s.start, s.end])).toEqual([[21, 27]]);
    expect(paintFor([span], 45008029, 24, "KJV").segments.map((s) => [s.start, s.end])).toEqual([[0, 8]]);
  });
});
