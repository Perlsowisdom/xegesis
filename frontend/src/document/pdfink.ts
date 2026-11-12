// The ink layer over one PDF page: strokes drawn on top of the page, pinned to its box (pdfinkmap.ts),
// with the same pen, highlighter, eraser, live drawing and palm rejection as ink in notes. Finished
// strokes are an SVG; the stroke being drawn is on a desynchronized canvas, with predicted points and
// the system's ink trail where the webview has them. The layer only draws and reports each change as a
// whole new set of strokes; saving and history are bookink.ts's.

import { INK_W, hits, strokeSize } from "../editor/ink";
import { strokeOutline } from "../editor/inkengine";
import { ERASE_REACH, InkPresenter, MIN_STEP, NS, caps, inkApi, notePen, palm, pen, strokeEl } from "../editor/inkpen";
import { PageStroke, pagePoint, unitPoint, unitStroke, viewBox } from "./pdfinkmap";

export class PageInkLayer {
  readonly el = document.createElement("div");
  private svg = document.createElementNS(NS, "svg");
  private canvas: HTMLCanvasElement | null = null; // only while a stroke is being drawn
  private ctx: CanvasRenderingContext2D | null = null;
  private strokes: PageStroke[] = [];
  private units = new WeakMap<PageStroke, ReturnType<typeof unitStroke>>();
  private cur: { stroke: PageStroke; id: number; tail: number[][]; frame: number; colour: string } | null = null;
  private erasing: { id: number; gone: Set<PageStroke>; last?: [number, number] } | null = null;
  private trail: InkPresenter | null = null;
  private trailReady: Promise<InkPresenter | null> | null = null;

  constructor(readonly aspect: number, private commit: (next: PageStroke[]) => void) {
    this.el.className = "pdf-ink";
    this.svg.setAttribute("class", "pdf-ink-svg");
    this.svg.setAttribute("viewBox", viewBox(aspect));
    this.svg.setAttribute("preserveAspectRatio", "none");
    this.svg.setAttribute("aria-hidden", "true");
    this.el.append(this.svg);
    this.el.addEventListener("pointerdown", (e) => this.down(e));
    this.el.addEventListener("pointermove", (e) => this.move(e));
    this.el.addEventListener("pointerup", (e) => this.up(e));
    this.el.addEventListener("pointercancel", (e) => this.cancel(e));
    this.el.addEventListener("contextmenu", (e) => e.preventDefault());
  }

  // show draws these strokes (the saved ones, or another set after undo).
  show(strokes: PageStroke[]): void {
    this.strokes = strokes;
    this.svg.replaceChildren(...strokes.map((k) => strokeEl(this.unit(k))));
    this.el.classList.toggle("has-ink", strokes.length > 0);
  }

  get busy(): boolean {
    return !!(this.cur || this.erasing);
  }

  private unit(k: PageStroke) {
    let u = this.units.get(k);
    if (!u) this.units.set(k, (u = unitStroke(k, this.aspect)));
    return u;
  }

  private rect(): DOMRect {
    return this.svg.getBoundingClientRect();
  }

  private at(e: PointerEvent): [number, number] {
    const r = this.rect();
    return [((e.clientX - r.left) / r.width) * INK_W, ((e.clientY - r.top) / r.height) * INK_W * this.aspect];
  }

  private point(e: PointerEvent, r: DOMRect): number[] {
    const pressure = e.pointerType === "pen" && e.pressure > 0 ? Math.max(0.001, e.pressure) : 0;
    return pagePoint(e.clientX, e.clientY, { x: r.left, y: r.top, w: r.width, h: r.height }, pressure);
  }

  // Pointer input ---------------------------------------------------------------------------------

  private down(e: PointerEvent): void {
    if (e.pointerType === "pen") notePen();
    if (palm(e)) {
      e.preventDefault();
      return;
    }
    const eraserEnd = e.pointerType === "pen" && (e.button === 5 || (e.buttons & 32) !== 0);
    if (this.cur || this.erasing || (e.button !== 0 && !eraserEnd)) return;
    e.preventDefault();
    e.stopPropagation();
    try {
      this.el.setPointerCapture(e.pointerId);
    } catch { /* a pointer that is no longer down */ }
    if (eraserEnd || pen.tool === "erase") {
      this.erasing = { id: e.pointerId, gone: new Set() };
      this.erase(e);
      return;
    }
    this.canvas = document.createElement("canvas");
    this.canvas.className = "ink-live";
    this.canvas.setAttribute("aria-hidden", "true");
    this.ctx = null;
    this.el.append(this.canvas);
    const stroke: PageStroke = { t: pen.tool, c: pen.colour, w: pen.width, p: [this.point(e, this.rect())] };
    const colour = getComputedStyle(this.el).getPropertyValue(stroke.c === "ink" ? "--text" : `--hl-${stroke.c}`).trim() || "#2a2622";
    this.cur = { stroke, id: e.pointerId, tail: [], frame: 0, colour };
    this.startTrail(e, stroke);
    this.refreshLive();
  }

  private move(e: PointerEvent): void {
    if (e.pointerType === "pen") notePen();
    if (this.erasing?.id === e.pointerId) {
      this.erase(e);
      return;
    }
    const c = this.cur;
    if (!c || c.id !== e.pointerId) return;
    const r = this.rect();
    const events = e.getCoalescedEvents?.() ?? [];
    for (const ev of events.length ? events : [e]) {
      const p = this.point(ev, r);
      const last = c.stroke.p[c.stroke.p.length - 1];
      const a = unitPoint(p, this.aspect), b = unitPoint(last, this.aspect);
      if (Math.hypot(a[0] - b[0], a[1] - b[1]) >= MIN_STEP) c.stroke.p.push(p);
    }
    const ahead = e.getPredictedEvents?.() ?? [];
    caps.predicted ||= ahead.length > 0;
    c.tail = ahead.map((ev) => this.point(ev, r));
    if (this.trail) this.trail.updateInkTrailStartPoint(events.length ? events[events.length - 1] : e, this.trailStyle(c.stroke, c.colour));
    this.refreshLive();
  }

  // The live stroke is drawn on a canvas, on one animation frame at most, and touches nothing saved.
  private refreshLive(): void {
    const c = this.cur;
    if (!c || c.frame) return;
    c.frame = requestAnimationFrame(() => {
      c.frame = 0;
      if (this.cur === c) this.paintLive(c);
    });
  }

  private paintLive(c: NonNullable<PageInkLayer["cur"]>): void {
    const cv = this.canvas;
    if (!cv) return;
    const r = this.rect();
    const dpr = window.devicePixelRatio || 1;
    const w = Math.max(1, Math.round(r.width * dpr)), h = Math.max(1, Math.round(r.height * dpr));
    if (!this.ctx) {
      this.ctx = cv.getContext("2d", { desynchronized: true });
      const attrs = this.ctx?.getContextAttributes?.() as { desynchronized?: boolean } | undefined;
      caps.desynchronized = attrs?.desynchronized === true;
    }
    const g = this.ctx;
    if (!g) return;
    if (cv.width !== w || cv.height !== h) {
      cv.width = w;
      cv.height = h;
    }
    g.setTransform(1, 0, 0, 1, 0, 0);
    g.clearRect(0, 0, w, h);
    const k = c.stroke;
    const pts = (c.tail.length ? [...k.p, ...c.tail] : k.p).map((p) => unitPoint(p, this.aspect));
    const o = strokeOutline(pts, strokeSize(k.t, k.w), k.t === "hl");
    if (o.length < 3) return;
    g.setTransform(w / INK_W, 0, 0, h / (INK_W * this.aspect), 0, 0);
    g.beginPath();
    g.moveTo(o[0][0], o[0][1]);
    for (let i = 1; i < o.length; i++) g.lineTo(o[i][0], o[i][1]);
    g.closePath();
    g.fillStyle = c.colour;
    g.fill();
  }

  private clearLive(): void {
    if (this.cur?.frame) cancelAnimationFrame(this.cur.frame);
    this.canvas?.remove(); // no canvas, and no memory for one, while idle
    this.canvas = null;
    this.ctx = null;
  }

  private trailStyle(k: PageStroke, colour: string): { color: string; diameter: number } {
    const px = this.rect().width / INK_W;
    return { color: colour, diameter: Math.max(1, strokeSize(k.t, k.w) * 0.9 * px) };
  }

  // startTrail asks the system to draw the pen tip itself where it can (not the highlighter, which is see-through).
  private startTrail(e: PointerEvent, k: PageStroke): void {
    this.trail = null;
    const api = inkApi();
    if (!api || e.pointerType === "mouse" || k.t === "hl") return;
    this.trailReady ??= api.requestPresenter({ presentationArea: this.el }).then((p) => {
      caps.delegated = true;
      return p;
    }, () => null);
    void this.trailReady.then((p) => {
      if (p && this.cur?.stroke === k) {
        this.trail = p;
        p.updateInkTrailStartPoint(e, this.trailStyle(k, this.cur.colour));
      }
    });
  }

  private up(e: PointerEvent): void {
    if (this.erasing?.id === e.pointerId) {
      const { gone } = this.erasing;
      this.erasing = null;
      if (gone.size) this.commit(this.strokes.filter((k) => !gone.has(k)));
      return;
    }
    const c = this.cur;
    if (!c || c.id !== e.pointerId) return;
    this.clearLive();
    this.cur = null;
    this.trail = null;
    this.commit([...this.strokes, c.stroke]);
  }

  private cancel(e: PointerEvent): void {
    if (this.erasing?.id === e.pointerId) {
      this.erasing = null;
      this.show(this.strokes);
    } else if (this.cur?.id === e.pointerId) {
      this.clearLive();
      this.cur = null;
      this.trail = null;
    }
  }

  // erase touches every stroke the pointer crossed since its last sample, not only those under it.
  private erase(e: PointerEvent): void {
    const er = this.erasing!;
    const [x, y] = this.at(e);
    const [x0, y0] = er.last ?? [x, y];
    er.last = [x, y];
    const steps = Math.max(1, Math.ceil(Math.hypot(x - x0, y - y0) / (ERASE_REACH / 2)));
    for (let s = 1; s <= steps; s++) {
      const px = x0 + ((x - x0) * s) / steps;
      const py = y0 + ((y - y0) * s) / steps;
      this.strokes.forEach((k, i) => {
        if (!er.gone.has(k) && hits(this.unit(k), px, py, ERASE_REACH)) {
          er.gone.add(k);
          (this.svg.children[i] as SVGElement | undefined)?.classList.add("ink-erased");
        }
      });
    }
  }
}
