// Ink: handwriting and drawing kept as vector strokes, in the note's Markdown as a fenced block:
//
//   ```ink
//   {"v":2,"h":260,"s":[{"t":"pen","c":"ink","w":2,"p":[[12.53,30,0],[14.01,33.5,0.612]]}]}
//   ```
//
// The page is always INK_W units wide whatever its width on screen, so strokes line up at every
// width; h is its height in the same units. A stroke has a tool (t: "pen" or "hl" for the
// highlighter), a colour (c: "ink" or a highlight colour name), a width step (w: 1 to 3) and points
// [x, y, pressure]. x and y keep two decimals; pressure is 0 to 1 with three decimals, or 0 when the
// device gave none (mouse, finger), in which case the pen's speed stands in for it. Version 1 (the
// first release) wrote one decimal and pressure 0 to 100; it is still read. One line of compact JSON,
// so it is one Markdown block.
// This file has no dependency on the editor, so it is tested on its own.

import { polygonPath, strokeOutline } from "./inkengine";

export const INK_W = 1000;
export const INK_MIN_H = 200;
export const INK_MAX_H = 8000;

export type InkTool = "pen" | "hl";
export type InkColour = "ink" | "yellow" | "green" | "blue" | "pink" | "purple";
export const INK_COLOURS: InkColour[] = ["ink", "yellow", "green", "blue", "pink", "purple"];
export const INK_COLOUR_LABEL: Record<InkColour, string> = {
  ink: "Black", yellow: "Yellow", green: "Green", blue: "Blue", pink: "Pink", purple: "Purple",
};
// The light-theme values of the highlight colours (src/style.css), for files that have no theme.
export const INK_HEX: Record<InkColour, string> = {
  ink: "#2a2622", yellow: "#d9b43c", green: "#5f9e55", blue: "#4f7fbf", pink: "#c4607f", purple: "#8b6bbd",
};

export interface InkStroke {
  t: InkTool;
  c: InkColour;
  w: 1 | 2 | 3;
  p: number[][];
}

export interface InkDoc {
  h: number;
  s: InkStroke[];
}

export const emptyInk = (): InkDoc => ({ h: 320, s: [] });

// strokeSize is a stroke's nominal width in page units.
export function strokeSize(t: InkTool, w: 1 | 2 | 3): number {
  return (t === "hl" ? [14, 24, 40] : [2.5, 5, 10])[w - 1];
}

export function inkOpacity(t: InkTool): number {
  return t === "hl" ? 0.38 : 1;
}

const round2 = (n: number) => Math.round(n * 100) / 100;
const round3 = (n: number) => Math.round(n * 1000) / 1000;

export function serializeInk(d: InkDoc): string {
  const s = d.s.map((k) => ({ t: k.t, c: k.c, w: k.w, p: k.p.map(([x, y, p]) => [round2(x), round2(y), round3(p ?? 0)]) }));
  return "```ink\n" + JSON.stringify({ v: 2, h: Math.round(d.h), s }) + "\n```";
}

// parseInk reads a fenced ink block (the lines of one Markdown block); null when it is not one.
export function parseInk(lines: string[]): InkDoc | null {
  if (lines.length !== 3 || lines[0] !== "```ink" || lines[2] !== "```") return null;
  let j: unknown;
  try {
    j = JSON.parse(lines[1]);
  } catch {
    return null;
  }
  if (!j || typeof j !== "object") return null;
  const o = j as { v?: unknown; h?: unknown; s?: unknown };
  if ((o.v !== 1 && o.v !== 2) || typeof o.h !== "number" || !Array.isArray(o.s)) return null;
  const s: InkStroke[] = [];
  for (const k of o.s as unknown[]) {
    const st = stroke(k, o.v === 1 ? 0.01 : 1);
    if (!st) return null;
    s.push(st);
  }
  return { h: clampH(o.h), s };
}

// stroke reads one stroke; pscale turns its stored pressure into 0 to 1.
function stroke(k: unknown, pscale: number): InkStroke | null {
  if (!k || typeof k !== "object") return null;
  const o = k as Record<string, unknown>;
  if ((o.t !== "pen" && o.t !== "hl") || !INK_COLOURS.includes(o.c as InkColour) || ![1, 2, 3].includes(o.w as number) || !Array.isArray(o.p)) return null;
  const p: number[][] = [];
  for (const pt of o.p as unknown[]) {
    if (!Array.isArray(pt) || pt.length !== 3 || pt.some((n) => typeof n !== "number" || !isFinite(n))) return null;
    p.push([pt[0], pt[1], Math.min(1, Math.max(0, pt[2] * pscale))]);
  }
  return { t: o.t, c: o.c as InkColour, w: o.w as 1 | 2 | 3, p };
}

export const clampH = (h: number) => Math.min(INK_MAX_H, Math.max(INK_MIN_H, Math.round(h)));

// growTo is the height the page needs once something is drawn down to y: always some blank room below.
export function growTo(h: number, y: number): number {
  return y > h - 80 ? clampH(y + 240) : h;
}

// outlinePath is the SVG path of a stroke's shape (inkengine.ts).
export function outlinePath(k: InkStroke): string {
  return polygonPath(strokeOutline(k.p, strokeSize(k.t, k.w), k.t === "hl"));
}

// hits is whether the point (x, y) touches the stroke (for the eraser).
export function hits(k: InkStroke, x: number, y: number, reach: number): boolean {
  const r = reach + strokeSize(k.t, k.w) / 2;
  const p = k.p;
  if (p.length === 1) return Math.hypot(p[0][0] - x, p[0][1] - y) <= r;
  for (let i = 1; i < p.length; i++) {
    if (segDist(x, y, p[i - 1][0], p[i - 1][1], p[i][0], p[i][1]) <= r) return true;
  }
  return false;
}

function segDist(px: number, py: number, ax: number, ay: number, bx: number, by: number): number {
  const dx = bx - ax, dy = by - ay;
  const len2 = dx * dx + dy * dy;
  const t = len2 === 0 ? 0 : Math.max(0, Math.min(1, ((px - ax) * dx + (py - ay) * dy) / len2));
  return Math.hypot(px - (ax + t * dx), py - (ay + t * dy));
}

// inkSVG draws the page as a standalone SVG string, in the light-theme colours (for HTML and PDF).
export function inkSVG(d: InkDoc): string {
  const paths = d.s.map((k) => {
    const op = inkOpacity(k.t);
    return `<path d="${outlinePath(k)}" fill="${INK_HEX[k.c]}"${op < 1 ? ` fill-opacity="${op}"` : ""}/>`;
  });
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${INK_W} ${d.h}" width="100%">${paths.join("")}</svg>`;
}
