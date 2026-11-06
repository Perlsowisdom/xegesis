// The stroke engine: turns pen points into the smooth, variable-width shape of a line of ink.
// Written for this project from standard published maths: a centripetal Catmull-Rom spline (alpha 0.5)
// through the real pen points, converted to cubic Béziers, then offset on both sides by the width at
// each point. The spline passes through every point, so there is no lag and sharp turns stay sharp.
// Points are [x, y, pressure] with pressure 0 to 1, or 0 when the device gave none. Pure functions.
// internal/export/inkoutline.go is the same algorithm for HTML and PDF; keep the two in step.

export type Pt = number[];

export const WIDTH_STEP = 2.5; // longest chord, in page units, before a spline segment is split further
export const CAP_STEPS = 8;

// pressureWidth is the share of a stroke's nominal size that a pressure from 0 to 1 draws.
export function pressureWidth(p: number): number {
  return 0.4 + 0.85 * Math.pow(Math.min(1, Math.max(0, p)), 0.9);
}

// speedWidth stands in for pressure on a mouse or finger: slow is wide, fast is thin.
export function speedWidth(step: number): number {
  return 1.2 - 0.7 * Math.min(1, Math.max(0, step / 24));
}

// radii is the half-width at every point (page units): from pressure when the device gave it, else
// from how far the pen travelled between points, smoothed a little along the stroke.
export function radii(pts: Pt[], size: number, flat: boolean): number[] {
  const n = pts.length;
  const real = pts.some((q) => q[2] > 0);
  let f = new Array<number>(n);
  for (let i = 0; i < n; i++) {
    if (flat) f[i] = 1;
    else if (real) f[i] = pressureWidth(pts[i][2] > 0 ? pts[i][2] : 0.5);
    else {
      const a = pts[Math.max(0, i - 1)], b = pts[Math.min(n - 1, i + 1)];
      f[i] = speedWidth(Math.hypot(b[0] - a[0], b[1] - a[1]) / (i === 0 || i === n - 1 ? 1 : 2));
    }
  }
  if (!flat) for (let pass = 0; pass < (real ? 1 : 4); pass++) {
    const g = f.slice();
    for (let i = 1; i < n - 1; i++) g[i] = 0.25 * f[i - 1] + 0.5 * f[i] + 0.25 * f[i + 1];
    f = g;
  }
  return f.map((v) => (v * size) / 2);
}

// bezier returns the cubic Bézier control points of the centripetal Catmull-Rom segment p1 to p2,
// given its neighbours p0 and p3.
export function bezier(p0: Pt, p1: Pt, p2: Pt, p3: Pt): [number, number, number, number] {
  const d1 = Math.sqrt(Math.hypot(p1[0] - p0[0], p1[1] - p0[1]));
  const d2 = Math.sqrt(Math.hypot(p2[0] - p1[0], p2[1] - p1[1]));
  const d3 = Math.sqrt(Math.hypot(p3[0] - p2[0], p3[1] - p2[1]));
  // tangent end next to b: (da² c − db² a + (2da² + 3da·db + db²) b) / (3 da (da + db))
  const ctl = (a: number, b: number, c: number, da: number, db: number): number =>
    (da * da * c - db * db * a + (2 * da * da + 3 * da * db + db * db) * b) / (3 * da * (da + db));
  const out: [number, number, number, number] = [0, 0, 0, 0];
  for (let k = 0; k < 2; k++) {
    out[k] = d1 > 1e-9 && d2 > 1e-9 ? ctl(p0[k], p1[k], p2[k], d1, d2) : p1[k] + (p2[k] - p1[k]) / 3;
    out[2 + k] = d3 > 1e-9 && d2 > 1e-9 ? ctl(p3[k], p2[k], p1[k], d3, d2) : p2[k] - (p2[k] - p1[k]) / 3;
  }
  return out;
}

export interface Sample {
  x: number;
  y: number;
  r: number;
}

// centreline samples the spline through the points densely, with the half-width at each sample.
export function centreline(pts: Pt[], r: number[]): Sample[] {
  const n = pts.length;
  const out: Sample[] = [{ x: pts[0][0], y: pts[0][1], r: r[0] }];
  for (let i = 0; i < n - 1; i++) {
    const p1 = pts[i], p2 = pts[i + 1];
    const p0 = i > 0 ? pts[i - 1] : [2 * p1[0] - p2[0], 2 * p1[1] - p2[1]];
    const p3 = i + 2 < n ? pts[i + 2] : [2 * p2[0] - p1[0], 2 * p2[1] - p1[1]];
    const [c1x, c1y, c2x, c2y] = bezier(p0, p1, p2, p3);
    const steps = Math.min(32, Math.max(1, Math.ceil(Math.hypot(p2[0] - p1[0], p2[1] - p1[1]) / WIDTH_STEP)));
    for (let s = 1; s <= steps; s++) {
      const t = s / steps, u = 1 - t;
      const a = u * u * u, b = 3 * u * u * t, c = 3 * u * t * t, d = t * t * t;
      out.push({
        x: a * p1[0] + b * c1x + c * c2x + d * p2[0],
        y: a * p1[1] + b * c1y + c * c2y + d * p2[1],
        r: r[i] + (r[i + 1] - r[i]) * t,
      });
    }
  }
  return out;
}

// outline is the closed polygon around the samples, with round ends.
export function outline(s: Sample[]): number[][] {
  // drop samples on top of each other so every direction is defined
  const q: Sample[] = [];
  for (const p of s) {
    const l = q[q.length - 1];
    if (!l || Math.hypot(p.x - l.x, p.y - l.y) > 1e-6) q.push(p);
  }
  const arc = (x: number, y: number, r: number, from: number, sweep: number, out: number[][]) => {
    for (let k = 1; k < CAP_STEPS; k++) {
      const a = from + (sweep * k) / CAP_STEPS;
      out.push([x + r * Math.cos(a), y + r * Math.sin(a)]);
    }
  };
  if (q.length < 2) {
    const p = q[0] ?? s[0];
    const o: number[][] = [[p.x + p.r, p.y]];
    arc(p.x, p.y, p.r, 0, 2 * Math.PI, o);
    return o;
  }
  const n = q.length;
  const left: number[][] = [], right: number[][] = [];
  const ang = new Array<number>(n);
  for (let i = 0; i < n; i++) {
    const a = q[Math.max(0, i - 1)], b = q[Math.min(n - 1, i + 1)];
    const th = Math.atan2(b.y - a.y, b.x - a.x);
    ang[i] = th;
    const nx = -Math.sin(th), ny = Math.cos(th);
    left.push([q[i].x + nx * q[i].r, q[i].y + ny * q[i].r]);
    right.push([q[i].x - nx * q[i].r, q[i].y - ny * q[i].r]);
  }
  const out = left.slice();
  const e = q[n - 1];
  arc(e.x, e.y, e.r, ang[n - 1] + Math.PI / 2, -Math.PI, out); // left, round the tip, to right
  for (let i = n - 1; i >= 0; i--) out.push(right[i]);
  arc(q[0].x, q[0].y, q[0].r, ang[0] - Math.PI / 2, -Math.PI, out);
  return out;
}

// strokeOutline is the polygon of a whole stroke. size is its nominal width; flat draws one width.
export function strokeOutline(pts: Pt[], size: number, flat: boolean): number[][] {
  if (pts.length === 0) return [];
  const p = pts.length === 1 ? [pts[0], pts[0]] : pts;
  return outline(centreline(p, radii(p, size, flat)));
}

export function polygonPath(o: number[][]): string {
  if (o.length < 3) return "";
  const f = (n: number) => String(Math.round(n * 100) / 100);
  let d = `M${f(o[0][0])},${f(o[0][1])}`;
  for (let i = 1; i < o.length; i++) d += `L${f(o[i][0])},${f(o[i][1])}`;
  return d + "Z";
}
