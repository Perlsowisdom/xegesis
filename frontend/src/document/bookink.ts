// Ink on the pages of a PDF in a book pane: writing on a page, showing or hiding a book's ink, undo
// and redo, and saving each page's strokes through ink.page.set. The strokes are pinned to the page
// (pdfinkmap.ts), so every pane showing the book, at any zoom and in Pages or Scroll mode, draws them
// in the same place. A page's ink is listed with the highlights as "Ink · page N".

import { run } from "../api";
import { h } from "../dom";
import { icon } from "../icons";
import { INK_COLOURS, INK_COLOUR_LABEL } from "../editor/ink";
import { bars, pen, refreshBars } from "../editor/inkpen";
import type { BookCtx } from "./bookctx";
import { PageInkLayer } from "./pdfink";
import type { PageStroke } from "./pdfinkmap";
import type { RenderedPage } from "./pdf";

interface PageState {
  id: string;
  aspect: number;
  strokes: PageStroke[];
  undo: PageStroke[][];
  redo: PageStroke[][];
  saving: number; // saves not finished yet
  chain: Promise<unknown>;
}

export interface InkCtl {
  writing: boolean;
  hidden: boolean;
  pages: Map<number, PageState>;
  layers: Map<number, PageInkLayer>;
  last: number; // the page written on last
  bar: HTMLElement | null;
  hideBtn: HTMLButtonElement | null;
}

export const newInk = (): InkCtl => ({ writing: false, hidden: false, pages: new Map(), layers: new Map(), last: 0, bar: null, hideBtn: null });

export const isPdf = (c: BookCtx) => !!c.book && !c.book.reflowable && c.book.format === "pdf";

const hideKey = (itemId: string) => `xegesis.ink.hidden.${itemId}`;
function readHidden(itemId: string): boolean {
  try {
    return localStorage.getItem(hideKey(itemId)) === "1";
  } catch {
    return false;
  }
}
function writeHidden(itemId: string, hidden: boolean) {
  try {
    if (hidden) localStorage.setItem(hideKey(itemId), "1");
    else localStorage.removeItem(hideKey(itemId));
  } catch { /* the choice lasts until the pane closes */ }
}

// inkOpened is called when a book is opened: its show/hide choice, and nothing left from the last book.
export function inkOpened(c: BookCtx) {
  const k = c.ink;
  k.pages.clear();
  k.layers.clear();
  k.last = 0;
  k.hidden = c.book ? readHidden(c.book.item_id) : false;
  if (!isPdf(c)) setWriting(c, false);
  applyMode(c);
}

// attachInk puts an ink layer over a page that has just been drawn.
export function attachInk(c: BookCtx, n: number, r: RenderedPage) {
  if (!isPdf(c)) return;
  const layer = new PageInkLayer(r.aspect, (next) => commit(c, n, r.aspect, next));
  c.ink.layers.set(n, layer);
  r.page.append(layer.el);
  // Writing leaves the focus in the pane, so Ctrl+Z and Ctrl+Y reach it.
  layer.el.addEventListener("pointerdown", () => c.body.focus({ preventScroll: true }));
  layer.el.hidden = c.ink.hidden;
  layer.show(c.ink.pages.get(n)?.strokes ?? []);
}

// dropInk forgets the layer of a page that was freed (Scroll mode draws only the pages near the view).
export function dropInk(c: BookCtx, n?: number) {
  if (n === undefined) c.ink.layers.clear();
  else c.ink.layers.delete(n);
}

// loadInk fetches a page's saved strokes and draws them, unless this pane is saving or drawing there.
export async function loadInk(c: BookCtx, n: number) {
  if (!isPdf(c) || !c.book) return;
  const have = c.ink.pages.get(n);
  if (have && have.saving > 0) return;
  const item = c.book.item_id;
  const res = await c.msg.attempt(() => run<{ id: string; ink: { aspect: number; strokes: PageStroke[] } }>("ink.page.get", { item_id: item, page: n }));
  if (!res || !c.alive || !c.book || c.book.item_id !== item) return;
  const st = c.ink.pages.get(n);
  const layer = c.ink.layers.get(n);
  if ((st && st.saving > 0) || layer?.busy) return;
  const strokes = res.ink.strokes ?? [];
  if (st && sameStrokes(st.strokes, strokes)) {
    st.id = res.id;
    return;
  }
  c.ink.pages.set(n, { id: res.id, aspect: res.ink.aspect || layer?.aspect || 1.4, strokes, undo: st?.undo ?? [], redo: st?.redo ?? [], saving: 0, chain: Promise.resolve() });
  layer?.show(strokes);
}

const sameStrokes = (a: PageStroke[], b: PageStroke[]) => a.length === b.length && JSON.stringify(a) === JSON.stringify(b);

// commit is a change a layer made: kept for undo, shown and saved.
function commit(c: BookCtx, n: number, aspect: number, next: PageStroke[]) {
  const st = state(c, n, aspect);
  st.undo.push(st.strokes);
  st.undo.splice(0, st.undo.length - 100);
  st.redo = [];
  c.ink.last = n;
  set(c, n, st, next);
}

function state(c: BookCtx, n: number, aspect: number): PageState {
  let st = c.ink.pages.get(n);
  if (!st) c.ink.pages.set(n, (st = { id: "", aspect, strokes: [], undo: [], redo: [], saving: 0, chain: Promise.resolve() }));
  return st;
}

function set(c: BookCtx, n: number, st: PageState, next: PageStroke[]) {
  st.strokes = next;
  c.ink.layers.get(n)?.show(next);
  refreshBars();
  const item = c.book?.item_id;
  if (!item) return;
  st.saving++;
  st.chain = st.chain.then(async () => {
    try {
      const res = await c.msg.attempt(() => run<{ id: string }>("ink.page.set", { item_id: item, page: n, aspect: st.aspect, strokes: st.strokes }));
      if (res) {
        st.id = res.id;
        announce(c);
      }
    } finally {
      st.saving--;
    }
  });
}

// announce tells the other panes (and the Highlights tool) that marks changed, not this pane.
function announce(c: BookCtx) {
  c.announcing = true;
  document.dispatchEvent(new Event("xegesis:marks-changed"));
  c.announcing = false;
}

function target(c: BookCtx): [number, PageState] | null {
  const n = c.ink.last || c.section;
  const st = c.ink.pages.get(n);
  return st ? [n, st] : null;
}

function step(c: BookCtx, from: "undo" | "redo") {
  const t = target(c);
  if (!t) return;
  const [n, st] = t;
  const prev = st[from].pop();
  if (!prev) return;
  (from === "undo" ? st.redo : st.undo).push(st.strokes);
  set(c, n, st, prev);
}

export const undoInk = (c: BookCtx) => step(c, "undo");
export const redoInk = (c: BookCtx) => step(c, "redo");

// Writing and showing -----------------------------------------------------------------------------

export function setWriting(c: BookCtx, on: boolean) {
  const k = c.ink;
  on = on && isPdf(c);
  if (on && k.hidden) setHidden(c, false);
  k.writing = on;
  applyMode(c);
}

export function setHidden(c: BookCtx, hidden: boolean) {
  const k = c.ink;
  if (!c.book) return;
  k.hidden = hidden;
  writeHidden(c.book.item_id, hidden);
  if (hidden) k.writing = false;
  applyMode(c);
}

function applyMode(c: BookCtx) {
  const k = c.ink;
  c.pdfBox.classList.toggle("ink-writing", k.writing);
  c.pdfBox.classList.toggle("ink-hidden", k.hidden);
  for (const l of k.layers.values()) l.el.hidden = k.hidden;
  if (k.bar) k.bar.hidden = !k.writing;
  if (k.hideBtn) k.hideBtn.textContent = k.hidden ? "Show ink" : "Hide ink";
  refreshBars();
}

// The strip of tools over the page while writing --------------------------------------------------

function buildBar(c: BookCtx, mine: (() => void)[]): HTMLElement {
  const bar = h("div", { class: "ink-bar pdf-ink-bar", role: "toolbar", "aria-label": "Handwriting tools", hidden: true });
  const btn = (cls: string, label: string, content: Node | string, run: () => void, on?: () => boolean) => {
    const b = h("button", { type: "button", class: `ink-btn ${cls}`, title: label, "aria-label": label }, content) as HTMLButtonElement;
    b.addEventListener("mousedown", (e) => e.preventDefault());
    b.addEventListener("click", () => {
      run();
      refreshBars();
    });
    if (on) {
      const f = () => {
        b.setAttribute("aria-pressed", String(on()));
        b.classList.toggle("on", on());
      };
      mine.push(f);
      f();
    }
    bar.append(b);
    return b;
  };
  const sep = () => bar.append(h("span", { class: "ink-sep" }));
  btn("ink-pen", "Pen", icon("pen"), () => (pen.tool = "pen"), () => pen.tool === "pen");
  btn("ink-hl", "Highlighter", icon("highlight"), () => (pen.tool = "hl"), () => pen.tool === "hl");
  btn("ink-erase", "Eraser (removes a whole stroke)", icon("eraser"), () => (pen.tool = "erase"), () => pen.tool === "erase");
  sep();
  for (const col of INK_COLOURS) {
    btn("ink-colour", INK_COLOUR_LABEL[col], h("span", { class: `ink-dot ink-dot-${col}` }), () => {
      pen.colour = col;
      if (pen.tool === "erase") pen.tool = "pen";
    }, () => pen.colour === col && pen.tool !== "erase");
  }
  sep();
  for (const w of [1, 2, 3] as const) btn("ink-size", ["Thin", "Medium", "Thick"][w - 1], h("span", { class: `ink-width ink-width-${w}` }), () => (pen.width = w), () => pen.width === w);
  sep();
  const undo = btn("ink-undo", "Undo last stroke", icon("undo"), () => undoInk(c));
  const redo = btn("ink-redo", "Redo", icon("redo"), () => redoInk(c));
  mine.push(() => {
    const st = target(c)?.[1];
    undo.disabled = !st || st.undo.length === 0;
    redo.disabled = !st || st.redo.length === 0;
  });
  undo.disabled = redo.disabled = true;
  bar.append(h("span", { class: "ink-grow" }));
  c.ink.hideBtn = h("button", { type: "button", class: "ink-text-btn ink-hide", title: "Hide this book's ink" }, "Hide ink") as HTMLButtonElement;
  c.ink.hideBtn.addEventListener("click", () => setHidden(c, !c.ink.hidden));
  const done = h("button", { type: "button", class: "ink-text-btn ink-done", title: "Stop writing" }, "Done");
  done.addEventListener("click", () => setWriting(c, false));
  bar.append(c.ink.hideBtn, done);
  return bar;
}

// wireInk adds the strip and keys to a book pane.
export function wireInk(c: BookCtx) {
  const mine: (() => void)[] = [];
  c.ink.bar = buildBar(c, mine);
  mine.forEach((f) => bars.add(f));
  c.reader.insertBefore(c.ink.bar, c.body);
  const onKey = (e: KeyboardEvent) => {
    if (!c.ink.writing || !(e.ctrlKey || e.metaKey) || e.altKey) return;
    if ((e.target as HTMLElement).closest("input, textarea, [contenteditable]")) return;
    const k = e.key.toLowerCase();
    if (k === "z" && !e.shiftKey) undoInk(c);
    else if (k === "y" || (k === "z" && e.shiftKey)) redoInk(c);
    else return;
    e.preventDefault();
    e.stopPropagation();
  };
  c.body.addEventListener("keydown", onKey);
  c.unwireInk = () => {
    c.body.removeEventListener("keydown", onKey);
    c.ink.layers.clear();
    mine.forEach((f) => bars.delete(f));
  };
}

// inkMenu is what the pane's ⋯ menu offers for a PDF.
export function inkMenu(c: BookCtx) {
  return {
    writing: () => c.ink.writing,
    shown: () => !c.ink.hidden,
    toggleWriting: () => setWriting(c, !c.ink.writing),
    toggleShown: () => setHidden(c, !c.ink.hidden),
  };
}
