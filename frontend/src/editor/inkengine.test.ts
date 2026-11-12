import { describe, expect, it } from "vitest";
import golden from "../../../internal/export/testdata/ink-golden.json";
import { strokeSize } from "./ink";
import { centreline, outline, pressureWidth, radii, strokeOutline } from "./inkengine";

const distToPoly = (x: number, y: number, s: { x: number; y: number }[]) => {
  let best = Infinity;
  for (let i = 1; i < s.length; i++) {
    const dx = s[i].x - s[i - 1].x, dy = s[i].y - s[i - 1].y;
    const l2 = dx * dx + dy * dy;
    const t = l2 ? Math.max(0, Math.min(1, ((x - s[i - 1].x) * dx + (y - s[i - 1].y) * dy) / l2)) : 0;
    best = Math.min(best, Math.hypot(x - s[i - 1].x - t * dx, y - s[i - 1].y - t * dy));
  }
  return best;
};

describe("stroke engine", () => {
  it("passes through every pen point", () => {
    const pts = [[0, 0, 0.5], [7, 3, 0.6], [15, 2, 0.7], [20, 12, 0.4], [31, 14, 0.5]];
    const c = centreline(pts, radii(pts, 5, false));
    for (const p of pts) expect(distToPoly(p[0], p[1], c)).toBeLessThan(0.05);
  });
  it("keeps a tight loop round", () => {
    const pts = Array.from({ length: 13 }, (_, i) => [50 + 10 * Math.cos((i / 12) * 2 * Math.PI), 50 + 10 * Math.sin((i / 12) * 2 * Math.PI), 0.5]);
    const c = centreline(pts, radii(pts, 3, false));
    for (const q of c) expect(Math.abs(Math.hypot(q.x - 50, q.y - 50) - 10)).toBeLessThan(0.3);
  });
  it("does not round a sharp corner past a small tolerance", () => {
    // a pen samples every couple of units: a right angle at (40, 0)
    const pts = [...Array.from({ length: 21 }, (_, i) => [2 * i, 0, 0.5]), ...Array.from({ length: 20 }, (_, i) => [40, 2 * (i + 1), 0.5])];
    const c = centreline(pts, radii(pts, 3, false));
    expect(distToPoly(40, 0, c)).toBeLessThan(0.01);
    // nothing swings past the corner by more than 0.3 units
    for (const q of c) {
      expect(q.x).toBeLessThanOrEqual(40.3);
      expect(q.y).toBeGreaterThanOrEqual(-0.3);
    }
    expect(Math.min(...c.filter((q) => q.x > 30 && q.y < 10).map((q) => Math.hypot(40 - q.x, q.y)))).toBeLessThan(0.3);
  });
  it("draws heavier with more pressure, and by speed without it", () => {
    expect(pressureWidth(1)).toBeGreaterThan(pressureWidth(0.2));
    const slow = radii([[0, 0, 0], [1, 0, 0], [2, 0, 0]], 5, false)[1];
    const fast = radii([[0, 0, 0], [20, 0, 0], [40, 0, 0]], 5, false)[1];
    expect(slow).toBeGreaterThan(fast);
  });
  it("makes a closed outline, also for one point", () => {
    expect(strokeOutline([[5, 5, 0]], 4, false).length).toBeGreaterThan(5);
    const o = strokeOutline([[0, 0, 0.5], [30, 0, 0.5]], 4, false);
    const ys = o.map((q) => q[1]);
    expect(Math.max(...ys)).toBeGreaterThan(1);
    expect(Math.min(...ys)).toBeLessThan(-1);
    expect(outline([{ x: 1, y: 1, r: 2 }]).length).toBeGreaterThan(5);
  });
});

// The same strokes are drawn by internal/export/inkoutline.go for HTML and PDF. Both test against
// internal/export/testdata/ink-golden.json, so a change to either engine fails the other's test.
// After changing the engine on purpose, replace the file with JSON.stringify(made) from this test,
// then make the Go port match.
describe("the golden strokes shared with the Go export", () => {
  const strokes: { name: string; tool: "pen" | "hl"; w: 1 | 2 | 3; points: number[][] }[] = [
    { name: "pen with pressure", tool: "pen", w: 2, points: [[10, 20, 0.2], [40, 25, 0.5], [70, 10, 0.9], [100, 40, 0.6]] },
    { name: "mouse, speed sets the width", tool: "pen", w: 1, points: [[5, 5, 0], [9, 8, 0], [20, 12, 0], [26, 30, 0], [31, 52, 0], [50, 60, 0], [52, 61, 0], [53, 61.5, 0]] },
    { name: "highlighter, one width", tool: "hl", w: 2, points: [[10, 10, 0], [60, 12, 0], [120, 9, 0]] },
    { name: "a single point", tool: "pen", w: 3, points: [[40, 40, 0.7]] },
    { name: "a sharp corner", tool: "pen", w: 2, points: [[0, 0, 0.5], [50, 0, 0.5], [50, 1, 0.5], [0, 2, 0.5]] },
    { name: "repeated points", tool: "pen", w: 1, points: [[10, 10, 0.4], [10, 10, 0.4], [30, 30, 0.6], [30, 30, 0.6]] },
  ];
  const round = (o: number[][]) => o.map((p) => [Math.round(p[0] * 1000) / 1000, Math.round(p[1] * 1000) / 1000]);
  const made = strokes.map((s) => ({ ...s, size: strokeSize(s.tool, s.w), outline: round(strokeOutline(s.points, strokeSize(s.tool, s.w), s.tool === "hl")) }));

  it("match the file", () => {
    expect(golden).toEqual(made);
  });
});
