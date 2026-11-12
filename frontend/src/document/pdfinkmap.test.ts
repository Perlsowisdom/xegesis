import { describe, expect, it } from "vitest";
import { INK_W } from "../editor/ink";
import { extent, onScreen, pagePoint, unitPoint, unitStroke, viewBox } from "./pdfinkmap";

describe("page coordinates for ink on a PDF page", () => {
  it("turns a pointer into fractions of the page box, whatever its size or place", () => {
    expect(pagePoint(150, 300, { x: 100, y: 200, w: 500, h: 700 }, 0.5)).toEqual([0.1, 0.1429, 0.5]);
    // the same spot on a page drawn twice as large and moved is the same fraction
    expect(pagePoint(200, 400, { x: 100, y: 200, w: 1000, h: 1400 }, 0.5)).toEqual([0.1, 0.1429, 0.5]);
  });

  it("keeps a point just off the page and clamps the rest, and the pressure to 0 to 1", () => {
    expect(pagePoint(0, 0, { x: 100, y: 100, w: 100, h: 100 }, 3)).toEqual([-0.04, -0.04, 1]);
    expect(pagePoint(5000, 5000, { x: 0, y: 0, w: 100, h: 100 }, -1)).toEqual([1.04, 1.04, 0]);
  });

  it("scales y by the page's height over its width for the engine, which is 1000 units wide", () => {
    expect(unitPoint([0.5, 0.5, 0.3], 1.4)).toEqual([500, 700, 0.3]);
    const k = unitStroke({ t: "pen", c: "blue", w: 2, p: [[0, 0, 0], [1, 1, 1]] }, 1.5);
    expect(k.p).toEqual([[0, 0, 0], [INK_W, INK_W * 1.5, 1]]);
    expect(viewBox(1.5)).toBe("0 0 1000 1500");
  });

  it("keeps a stroke's box at the same fraction of the page at every zoom", () => {
    const strokes = [{ t: "pen" as const, c: "ink" as const, w: 1 as const, p: [[0.2, 0.3, 0], [0.6, 0.5, 0]] }];
    const box = extent(strokes)!;
    for (const w of [400, 600, 1200]) {
      const page = { x: 12, y: 30, w, h: w * 1.4 };
      const s = onScreen(box, page);
      expect(s.x / page.w - 12 / page.w).toBeCloseTo(0.2, 6);
      expect((s.y - page.y) / page.h).toBeCloseTo(0.3, 6);
      expect(s.w / page.w).toBeCloseTo(0.4, 6);
      expect(s.h / page.h).toBeCloseTo(0.2, 6);
    }
  });

  it("has no extent without strokes", () => {
    expect(extent([])).toBeNull();
  });
});
