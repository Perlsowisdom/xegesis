// What every ink surface shares: the pen (tool, colour, width), palm rejection, the low-latency
// capabilities the end-to-end test reads, the delegated ink trail, and the SVG shape of a stroke.
// Used by the ink block in notes (inknode.ts) and the ink layer on PDF pages (document/pdfink.ts).

import { InkColour, InkStroke, InkTool, inkOpacity, outlinePath } from "./ink";

// The pen, shared by every ink surface on the screen.
export type Pen = { tool: InkTool | "erase"; colour: InkColour; width: 1 | 2 | 3 };
export const pen: Pen = { tool: "pen", colour: "ink", width: 2 };
// Tool strips redraw their pressed state from here after any of them changes the pen.
export const bars = new Set<() => void>();
export const refreshBars = () => bars.forEach((f) => f());

// Once a pen has touched the screen, touches count as a resting palm for a while.
const PEN_PRESENCE_MS = 120_000;
let penAt = -Infinity;
export const notePen = () => (penAt = performance.now());
export const palm = (e: PointerEvent) => e.pointerType === "touch" && performance.now() - penAt < PEN_PRESENCE_MS;

export const NS = "http://www.w3.org/2000/svg";
export const MIN_STEP = 0.15; // page units between points kept: almost every point the pen reports
export const ERASE_REACH = 7;

// What this webview can do for low-latency ink; read by the end-to-end test and nothing else.
export const caps = { desynchronized: false, delegated: false, predicted: false };
if (typeof window !== "undefined") (window as unknown as { __xgInk: typeof caps }).__xgInk = caps;

// The delegated ink trail (Chromium, so WebView2 on Windows): the system draws the pen tip itself.
export interface InkPresenter {
  updateInkTrailStartPoint(e: PointerEvent, style: { color: string; diameter: number }): void;
}
export const inkApi = () => (navigator as unknown as { ink?: { requestPresenter(o: { presentationArea: Element }): Promise<InkPresenter> } }).ink;

export function strokeEl(k: InkStroke): SVGPathElement {
  const p = document.createElementNS(NS, "path");
  p.setAttribute("d", pathOf(k));
  p.style.fill = k.c === "ink" ? "var(--text)" : `var(--hl-${k.c})`;
  if (k.t === "hl") p.style.fillOpacity = String(inkOpacity("hl"));
  return p;
}

const pathCache = new WeakMap<InkStroke, string>();
const pathOf = (k: InkStroke) => {
  let d = pathCache.get(k);
  if (d === undefined) pathCache.set(k, (d = outlinePath(k)));
  return d;
};
