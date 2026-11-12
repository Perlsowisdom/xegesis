// Ink on a PDF page is pinned to the page box: a point is [x, y, pressure] with x and y as fractions
// (0 to 1) of the page's width and height, so strokes line up at every zoom and in every pane. The
// stroke engine works in a page 1000 units wide (editor/ink.ts), so a stroke is turned into those
// units, with y scaled by the page's height over its width, to be shaped and drawn. Pure functions.

import { INK_W, InkStroke } from "../editor/ink";

export type PageStroke = InkStroke; // t, c, w and points in page fractions

export interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

const r4 = (n: number) => Math.round(n * 10000) / 10000;
const r3 = (n: number) => Math.round(n * 1000) / 1000;

// pagePoint is where a pointer is on the page, in page fractions (clamped to just off the page).
export function pagePoint(clientX: number, clientY: number, box: Box, pressure: number): number[] {
  const c = (v: number) => Math.min(1.04, Math.max(-0.04, v));
  return [r4(c((clientX - box.x) / box.w)), r4(c((clientY - box.y) / box.h)), r3(Math.min(1, Math.max(0, pressure)))];
}

// unitPoint turns a page-fraction point into the engine's units (INK_W wide) for a page of this aspect (height / width).
export function unitPoint(p: number[], aspect: number): number[] {
  return [p[0] * INK_W, p[1] * INK_W * aspect, p[2]];
}

export function unitStroke(k: PageStroke, aspect: number): InkStroke {
  return { t: k.t, c: k.c, w: k.w, p: k.p.map((p) => unitPoint(p, aspect)) };
}

// viewBox is the SVG view of a page's ink: the page in engine units.
export const viewBox = (aspect: number) => `0 0 ${INK_W} ${r4(INK_W * aspect)}`;

// extent is the box that holds every point of the strokes, in page fractions; null with no strokes.
export function extent(list: PageStroke[]): Box | null {
  let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
  for (const k of list) for (const [x, y] of k.p) {
    x0 = Math.min(x0, x); y0 = Math.min(y0, y); x1 = Math.max(x1, x); y1 = Math.max(y1, y);
  }
  return x0 > x1 ? null : { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
}

// onScreen is a box in page fractions as pixels, on a page drawn at this rectangle.
export function onScreen(b: Box, page: Box): Box {
  return { x: page.x + b.x * page.w, y: page.y + b.y * page.h, w: b.w * page.w, h: b.h * page.h };
}
