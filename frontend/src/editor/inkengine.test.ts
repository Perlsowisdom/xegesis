import { describe, expect, it } from "vitest";
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
