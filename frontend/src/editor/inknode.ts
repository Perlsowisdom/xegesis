// The ink block of the editor: a page you write or draw on with a pen, a mouse or a finger. It is one
// whole block like the divider: the caret steps over it, and it keeps its strokes as vector data
// (ink.ts), which are saved in the note's Markdown. The block draws and edits itself; every change
// goes into the editor as one update, so saving, undo history and sync work as for text.

import {
  $applyNodeReplacement, $getNodeByKey, $isParagraphNode, DecoratorNode, type EditorConfig, type LexicalEditor, type LexicalNode,
  type NodeKey, type SerializedLexicalNode, type Spread,
} from "lexical";
import { icon } from "../icons";
import {
  INK_COLOURS, INK_COLOUR_LABEL, INK_W, InkColour, InkDoc, InkStroke, InkTool, emptyInk, growTo, hits, inkOpacity, outlinePath,
} from "./ink";
import { $caretAfterBlock } from "./nodes";

type SerializedInk = Spread<{ ink: InkDoc }, SerializedLexicalNode>;

export class InkNode extends DecoratorNode<null> {
  __ink: InkDoc;

  static getType(): string {
    return "xg-ink";
  }
  static clone(n: InkNode): InkNode {
    return new InkNode(n.__ink, n.__key);
  }
  static importJSON(j: SerializedInk): InkNode {
    return $createInkNode(j.ink);
  }
  constructor(ink: InkDoc = emptyInk(), key?: NodeKey) {
    super(key);
    this.__ink = ink;
  }
  exportJSON(): SerializedInk {
    return { type: "xg-ink", version: 1, ink: this.getInk() };
  }
  getInk(): InkDoc {
    return this.getLatest().__ink;
  }
  setInk(ink: InkDoc): void {
    this.getWritable().__ink = ink;
  }
  getTextContent(): string {
    return "";
  }
  createDOM(_config: EditorConfig, editor: LexicalEditor): HTMLElement {
    const ctl = new InkBlock(editor, this.__key, this.__ink);
    live.set(ctl.el, ctl);
    return ctl.el;
  }
  updateDOM(prev: InkNode, dom: HTMLElement): false {
    // Undo and redo of the editor bring back another set of strokes: the block draws it.
    if (prev.__ink !== this.__ink) live.get(dom)?.show(this.__ink);
    return false;
  }
  isInline(): false {
    return false;
  }
  decorate(): null {
    return null;
  }
}

export function $createInkNode(ink: InkDoc = emptyInk()): InkNode {
  return $applyNodeReplacement(new InkNode(ink));
}

export function $isInkNode(n: LexicalNode | null | undefined): n is InkNode {
  return n instanceof InkNode;
}

// insertInk puts a new ink block after the caret's block (in place of it when it is empty).
export function insertInk(editor: LexicalEditor, topBlock: () => LexicalNode | null): void {
  editor.update(() => {
    const b = topBlock();
    if (!b) return;
    const n = $createInkNode();
    if ($isParagraphNode(b) && b.getTextContentSize() === 0) b.replace(n);
    else b.insertAfter(n);
    $caretAfterBlock(n.getKey());
  });
}

// The pen, shared by every ink block on the screen.
type Pen = { tool: InkTool | "erase"; colour: InkColour; width: 1 | 2 | 3 };
const pen: Pen = { tool: "pen", colour: "ink", width: 2 };
const bars = new Set<() => void>();
const live = new WeakMap<HTMLElement, InkBlock>();

// Once a pen has touched the screen, touches count as a resting palm for a while.
const PEN_PRESENCE_MS = 120_000;
let penAt = -Infinity;
const palm = (e: PointerEvent) => e.pointerType === "touch" && performance.now() - penAt < PEN_PRESENCE_MS;

const NS = "http://www.w3.org/2000/svg";
const MIN_STEP = 0.8; // page units between points kept
const ERASE_REACH = 7;

// inkView is the read-only picture of a page of ink (saved reports, the chat).
export function inkView(doc: InkDoc): HTMLElement {
  const el = document.createElement("div");
  el.className = "ink-block ink-still";
  const svg = document.createElementNS(NS, "svg");
  svg.setAttribute("class", "ink-page");
  svg.setAttribute("viewBox", `0 0 ${INK_W} ${doc.h}`);
  svg.setAttribute("role", "img");
  svg.setAttribute("aria-label", "Handwriting");
  svg.style.aspectRatio = `${INK_W} / ${doc.h}`;
  svg.append(...doc.s.map((k) => strokeEl(k)));
  el.append(svg);
  return el;
}

function strokeEl(k: InkStroke): SVGPathElement {
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

class InkBlock {
  readonly el: HTMLElement;
  private doc: InkDoc;
  private undo: InkDoc[] = [];
  private redo: InkDoc[] = [];
  private page = document.createElementNS(NS, "svg");
  private undoBtn!: HTMLButtonElement;
  private redoBtn!: HTMLButtonElement;
  private cur: { stroke: InkStroke; path: SVGPathElement; id: number } | null = null;
  private erasing: { id: number; gone: Set<InkStroke> } | null = null;

  constructor(private editor: LexicalEditor, private key: NodeKey, doc: InkDoc) {
    this.doc = doc;
    this.el = document.createElement("div");
    this.el.className = "ink-block";
    this.el.contentEditable = "false";
    this.el.append(this.buildBar(), this.page);
    this.page.setAttribute("class", "ink-page");
    this.page.setAttribute("role", "img");
    this.page.setAttribute("aria-label", "Handwriting area. Write or draw here with a pen, a mouse or a finger.");
    this.page.setAttribute("tabindex", "0");
    this.page.addEventListener("pointerdown", (e) => this.down(e));
    this.page.addEventListener("pointermove", (e) => this.move(e));
    this.page.addEventListener("pointerup", (e) => this.up(e));
    this.page.addEventListener("pointercancel", (e) => this.cancel(e));
    this.page.addEventListener("keydown", (e) => this.key_(e));
    this.page.addEventListener("contextmenu", (e) => e.preventDefault());
    this.draw();
  }

  // show swaps in another set of strokes from outside (the editor's undo).
  show(doc: InkDoc): void {
    if (doc === this.doc) return;
    this.doc = doc;
    this.draw();
  }

  // Drawing ---------------------------------------------------------------------------------------

  private draw(): void {
    this.page.setAttribute("viewBox", `0 0 ${INK_W} ${this.doc.h}`);
    this.page.style.aspectRatio = `${INK_W} / ${this.doc.h}`;
    this.page.replaceChildren(...this.doc.s.map((k) => this.pathEl(k)));
    this.el.classList.toggle("ink-empty", this.doc.s.length === 0);
    if (this.undoBtn) {
      this.undoBtn.disabled = this.undo.length === 0;
      this.redoBtn.disabled = this.redo.length === 0;
    }
  }

  private pathEl(k: InkStroke): SVGPathElement {
    return strokeEl(k);
  }

  private at(e: PointerEvent): [number, number] {
    const r = this.page.getBoundingClientRect();
    return [((e.clientX - r.left) / r.width) * INK_W, ((e.clientY - r.top) / r.width) * INK_W];
  }

  private point(e: PointerEvent): number[] {
    const [x, y] = this.at(e);
    const pressure = e.pointerType === "pen" && e.pressure > 0 ? Math.max(1, Math.round(e.pressure * 100)) : 0;
    return [x, y, pressure];
  }

  // Pointer input ---------------------------------------------------------------------------------

  private down(e: PointerEvent): void {
    if (e.pointerType === "pen") penAt = performance.now();
    if (palm(e)) {
      e.preventDefault();
      return;
    }
    const eraserEnd = e.pointerType === "pen" && (e.button === 5 || (e.buttons & 32) !== 0);
    if (this.cur || this.erasing || (e.button !== 0 && !eraserEnd)) return;
    e.preventDefault();
    this.page.focus({ preventScroll: true });
    this.page.setPointerCapture(e.pointerId);
    if (eraserEnd || pen.tool === "erase") {
      this.erasing = { id: e.pointerId, gone: new Set() };
      this.erase(e);
      return;
    }
    const stroke: InkStroke = { t: pen.tool, c: pen.colour, w: pen.width, p: [this.point(e)] };
    const path = this.pathEl(stroke);
    this.page.append(path);
    this.cur = { stroke, path, id: e.pointerId };
    this.refreshLive();
  }

  private move(e: PointerEvent): void {
    if (e.pointerType === "pen") penAt = performance.now();
    if (this.erasing?.id === e.pointerId) {
      this.erase(e);
      return;
    }
    const c = this.cur;
    if (!c || c.id !== e.pointerId) return;
    const events = e.getCoalescedEvents?.() ?? [];
    for (const ev of events.length ? events : [e]) {
      const p = this.point(ev);
      const last = c.stroke.p[c.stroke.p.length - 1];
      if (Math.hypot(p[0] - last[0], p[1] - last[1]) >= MIN_STEP) c.stroke.p.push(p);
    }
    const y = c.stroke.p[c.stroke.p.length - 1][1];
    const h = growTo(this.doc.h, y);
    if (h !== this.doc.h) {
      this.doc = { ...this.doc, h };
      this.page.setAttribute("viewBox", `0 0 ${INK_W} ${h}`);
      this.page.style.aspectRatio = `${INK_W} / ${h}`;
    }
    this.refreshLive();
  }

  private refreshLive(): void {
    if (this.cur) this.cur.path.setAttribute("d", outlinePath(this.cur.stroke));
  }

  private up(e: PointerEvent): void {
    if (this.erasing?.id === e.pointerId) {
      const { gone } = this.erasing;
      this.erasing = null;
      if (gone.size) this.commit({ h: this.doc.h, s: this.doc.s.filter((k) => !gone.has(k)) });
      return;
    }
    const c = this.cur;
    if (!c || c.id !== e.pointerId) return;
    this.cur = null;
    pathCache.delete(c.stroke); // drawn while it grew; the finished shape is made fresh
    this.commit({ h: this.doc.h, s: [...this.doc.s, c.stroke] });
  }

  private cancel(e: PointerEvent): void {
    if (this.erasing?.id === e.pointerId) {
      this.erasing = null;
      this.draw();
    } else if (this.cur?.id === e.pointerId) {
      this.cur.path.remove();
      this.cur = null;
    }
  }

  private erase(e: PointerEvent): void {
    const er = this.erasing!;
    const [x, y] = this.at(e);
    this.doc.s.forEach((k, i) => {
      if (!er.gone.has(k) && hits(k, x, y, ERASE_REACH)) {
        er.gone.add(k);
        (this.page.children[i] as SVGElement | undefined)?.classList.add("ink-erased");
      }
    });
  }

  // History and the editor ------------------------------------------------------------------------

  private commit(next: InkDoc): void {
    this.undo.push(this.doc);
    this.redo = [];
    this.set(next);
  }

  private set(next: InkDoc): void {
    this.doc = next;
    this.draw();
    this.editor.update(() => {
      const n = $getNodeByKey(this.key);
      if ($isInkNode(n)) n.setInk(next);
    });
  }

  private stepUndo(): void {
    const prev = this.undo.pop();
    if (!prev) return;
    this.redo.push(this.doc);
    this.set(prev);
  }

  private stepRedo(): void {
    const next = this.redo.pop();
    if (!next) return;
    this.undo.push(this.doc);
    this.set(next);
  }

  private key_(e: KeyboardEvent): void {
    if (!(e.ctrlKey || e.metaKey) || e.altKey) return;
    const k = e.key.toLowerCase();
    if (k === "z" && !e.shiftKey) this.stepUndo();
    else if (k === "y" || (k === "z" && e.shiftKey)) this.stepRedo();
    else return;
    e.preventDefault();
    e.stopPropagation();
  }

  // The tool strip ---------------------------------------------------------------------------------

  private buildBar(): HTMLElement {
    const bar = document.createElement("div");
    bar.className = "ink-bar";
    bar.setAttribute("role", "toolbar");
    bar.setAttribute("aria-label", "Handwriting tools");
    const refresh: (() => void)[] = [];
    const btn = (cls: string, label: string, content: Node | string, run: () => void, on?: () => boolean) => {
      const b = document.createElement("button");
      b.type = "button";
      b.className = `ink-btn ${cls}`;
      b.title = label;
      b.setAttribute("aria-label", label);
      b.append(content);
      b.addEventListener("mousedown", (e) => e.preventDefault());
      b.addEventListener("click", () => {
        run();
        bars.forEach((f) => f());
      });
      if (on) {
        refresh.push(() => {
          b.setAttribute("aria-pressed", String(on()));
          b.classList.toggle("on", on());
        });
      }
      bar.append(b);
      return b;
    };
    const sep = () => bar.append(Object.assign(document.createElement("span"), { className: "ink-sep" }));

    btn("ink-pen", "Pen", icon("pen"), () => (pen.tool = "pen"), () => pen.tool === "pen");
    btn("ink-hl", "Highlighter", icon("highlight"), () => (pen.tool = "hl"), () => pen.tool === "hl");
    btn("ink-erase", "Eraser (removes a whole stroke)", icon("eraser"), () => (pen.tool = "erase"), () => pen.tool === "erase");
    sep();
    for (const c of INK_COLOURS) {
      const dot = document.createElement("span");
      dot.className = `ink-dot ink-dot-${c}`;
      btn("ink-colour", INK_COLOUR_LABEL[c], dot, () => {
        pen.colour = c;
        if (pen.tool === "erase") pen.tool = "pen";
      }, () => pen.colour === c && pen.tool !== "erase");
    }
    sep();
    for (const w of [1, 2, 3] as const) {
      const line = document.createElement("span");
      line.className = `ink-width ink-width-${w}`;
      btn("ink-size", ["Thin", "Medium", "Thick"][w - 1], line, () => (pen.width = w), () => pen.width === w);
    }
    sep();
    this.undoBtn = btn("ink-undo", "Undo last stroke", icon("undo"), () => this.stepUndo());
    this.redoBtn = btn("ink-redo", "Redo", icon("redo"), () => this.stepRedo());
    const grow = document.createElement("span");
    grow.className = "ink-grow";
    bar.append(grow);
    btn("ink-remove", "Remove this handwriting", icon("close"), () => {
      const key = this.key;
      this.editor.update(() => $getNodeByKey(key)?.remove());
    });

    let seen = false;
    const all = () => {
      if (!bar.isConnected && seen) bars.delete(all); // the block is gone
      seen ||= bar.isConnected;
      refresh.forEach((f) => f());
    };
    bars.add(all);
    all();
    return bar;
  }
}

