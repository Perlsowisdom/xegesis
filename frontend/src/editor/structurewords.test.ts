import { describe, expect, it } from "vitest";
import { carryFolds, foldId, foldedRange, outlineMarks } from "./structurewords";

describe("folding", () => {
  it("hides a section up to the next heading as high", () => {
    // # A, p, ## B, p, # C, p
    const levels = [1, 0, 2, 0, 1, 0];
    expect(foldedRange(levels, [true, false, false, false, false, false])).toEqual([false, true, true, true, false, false]);
    expect(foldedRange(levels, [false, false, true, false, false, false])).toEqual([false, false, false, true, false, false]);
  });
  it("hides to the end when nothing as high follows", () => {
    expect(foldedRange([2, 0, 3, 0], [true, false, false, false])).toEqual([false, true, true, true]);
  });
  it("names alike headings apart", () => {
    expect(foldId(2, "Point", 0)).not.toBe(foldId(2, "Point", 1));
  });
});

describe("outline numbering", () => {
  it("numbers by depth", () => {
    expect(outlineMarks([1, 2, 2, 3, 1, 2])).toEqual([
      { depth: 0, label: "1" }, { depth: 1, label: "a" }, { depth: 1, label: "b" }, { depth: 2, label: "i" },
      { depth: 0, label: "2" }, { depth: 1, label: "a" },
    ]);
  });
  it("starts at the shallowest level used", () => {
    expect(outlineMarks([2, 3, 2]).map((m) => m.depth)).toEqual([0, 1, 0]);
  });
});

describe("folds follow an edited heading", () => {
  it("re-saves the key when the words change", () => {
    const folded = new Set(["1|Alpha|0"]);
    const changed = carryFolds(new Map([["a", "1|Alpha|0"]]), new Map([["a", "1|Alpha two|0"]]), folded);
    expect(changed).toBe(true);
    expect([...folded]).toEqual(["1|Alpha two|0"]);
  });
  it("leaves other folds and unchanged headings alone", () => {
    const folded = new Set(["1|B|0"]);
    expect(carryFolds(new Map([["a", "1|A|0"]]), new Map([["a", "1|A2|0"]]), folded)).toBe(false);
    expect([...folded]).toEqual(["1|B|0"]);
  });
});
