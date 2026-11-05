// The text's headings as structure: an arrow beside each heading folds its section away, and a
// slim strip at the right edge has a mark per heading (their names on hover, a click goes there).
// Both are drawn outside the editable text, over it, so Lexical's own DOM is never changed beyond
// a class on hidden blocks. Folding is for display only: the text and its Markdown are untouched,
// and which sections are folded is remembered per document on this device (localStorage).

import type { LexicalEditor } from "lexical";
import { carryFolds, foldedRange, foldId } from "./structurewords";

export interface StructureHeading {
  key: string;
  level: number;
  text: string;
}

export interface StructureOptions {
  editor: LexicalEditor;
  host: HTMLElement; // .rich-editor, positioned
  content: HTMLElement; // the editable text
  strip: boolean; // the outline strip (the full page only)
  headings(): StructureHeading[];
  blockKeys(): { key: string; level: number }[]; // every top-level block, 0 for a non-heading
  reveal(key: string): void;
  caretBlock(): string | null; // the top-level block holding the caret
}

const STORE = "xegesis.folds.";

export function structure(o: StructureOptions) {
  const doc = o.host.ownerDocument;
  const gutter = doc.createElement("div");
  gutter.className = "rt-fold-gutter";
  const strip = doc.createElement("nav");
  strip.className = "rt-strip";
  strip.setAttribute("aria-label", "Headings");
  const marks = doc.createElement("div");
  marks.className = "rt-strip-marks";
  const names = doc.createElement("div");
  names.className = "rt-strip-names";
  const inner = doc.createElement("div");
  inner.className = "rt-strip-inner";
  inner.append(marks, names);
  strip.append(inner);
  o.host.append(gutter);
  if (o.strip) o.host.append(strip);

  let key = ""; // the document's fold key; "" keeps folds for this visit only
  let folded = new Set<string>();
  let hover: string | null = null;
  let last = "";
  let prevIds = new Map<string, string>(); // each heading's fold id at the last apply, by node key

  function load(k: string) {
    key = k;
    folded = new Set();
    prevIds = new Map();
    if (!k) return;
    try {
      const v = JSON.parse(localStorage.getItem(STORE + k) ?? "[]");
      if (Array.isArray(v)) folded = new Set(v.filter((x) => typeof x === "string"));
    } catch {
      // storage unavailable: folds last for this visit
    }
  }

  function save() {
    if (!key) return;
    try {
      if (folded.size) localStorage.setItem(STORE + key, JSON.stringify([...folded]));
      else localStorage.removeItem(STORE + key);
    } catch {
      // storage unavailable
    }
  }

  // ids names each heading by its level, words and how many alike come before it.
  function ids(hs: StructureHeading[]): Map<string, string> {
    const seen = new Map<string, number>();
    const out = new Map<string, string>();
    for (const hd of hs) {
      const base = `${hd.level}|${hd.text}`;
      const n = seen.get(base) ?? 0;
      seen.set(base, n + 1);
      out.set(hd.key, foldId(hd.level, hd.text, n));
    }
    return out;
  }

  function apply() {
    const hs = o.headings();
    const idOf = ids(hs);
    // A heading edited while folded keeps its fold: the fold follows the heading node.
    if (carryFolds(prevIds, idOf, folded)) save();
    prevIds = idOf;
    const blocks = o.blockKeys();
    const isFolded = blocks.map((b) => b.level > 0 && folded.has(idOf.get(b.key) ?? ""));
    const hidden = foldedRange(blocks.map((b) => b.level), isFolded);
    blocks.forEach((b, i) => {
      const el = o.editor.getElementByKey(b.key);
      if (!el) return;
      el.classList.toggle("rt-folded-away", hidden[i]);
      el.classList.toggle("rt-folded", isFolded[i]);
    });
    drawGutter(hs, idOf, new Set(blocks.filter((_, i) => hidden[i]).map((b) => b.key)));
    drawStrip(hs, idOf);
  }

  function toggle(id: string) {
    if (folded.has(id)) folded.delete(id);
    else folded.add(id);
    save();
    apply();
  }

  function drawGutter(hs: StructureHeading[], idOf: Map<string, string>, hiddenKeys: Set<string>) {
    const base = o.content.offsetTop;
    const kids: HTMLElement[] = [];
    for (const hd of hs) {
      const el = o.editor.getElementByKey(hd.key);
      if (!el || hiddenKeys.has(hd.key)) continue;
      const id = idOf.get(hd.key)!;
      const on = folded.has(id);
      const b = doc.createElement("button");
      b.type = "button";
      b.className = `rt-fold${on ? " closed" : ""}${hover === hd.key ? " hover" : ""}`;
      b.dataset.key = hd.key;
      b.setAttribute("aria-expanded", String(!on));
      b.setAttribute("aria-label", `${on ? "Unfold" : "Fold"} “${hd.text || "Untitled"}”`);
      b.title = on ? "Show this section" : "Fold this section away";
      b.tabIndex = -1;
      b.style.top = `${base + el.offsetTop + (el.offsetHeight - 22) / 2}px`;
      b.addEventListener("mousedown", (e) => e.preventDefault());
      b.addEventListener("click", () => toggle(id));
      kids.push(b);
    }
    gutter.replaceChildren(...kids);
  }

  function drawStrip(hs: StructureHeading[], idOf: Map<string, string>) {
    if (!o.strip) return;
    const sig = JSON.stringify(hs.map((x) => [x.key, x.level, x.text, folded.has(idOf.get(x.key)!)]));
    strip.hidden = hs.length === 0;
    if (sig === last) return mark();
    last = sig;
    const min = Math.min(...hs.map((x) => x.level));
    const mk = (hd: StructureHeading, kind: "mark" | "name") => {
      const b = doc.createElement("button");
      b.type = "button";
      b.className = `rt-strip-${kind} depth-${Math.min(hd.level - min, 2)}`;
      b.dataset.key = hd.key;
      if (kind === "mark") {
        b.setAttribute("aria-label", hd.text || "Untitled heading");
        b.tabIndex = -1;
      } else b.textContent = hd.text || "Untitled heading";
      b.addEventListener("mousedown", (e) => e.preventDefault());
      b.addEventListener("click", () => go(hd.key, idOf));
      return b;
    };
    marks.replaceChildren(...hs.map((hd) => mk(hd, "mark")));
    names.replaceChildren(...hs.map((hd) => mk(hd, "name")));
    mark();
  }

  // go shows a heading, unfolding whatever hides it, and puts the caret there.
  function go(k: string, idOf: Map<string, string>) {
    const blocks = o.blockKeys();
    const i = blocks.findIndex((b) => b.key === k);
    let changed = false;
    if (i >= 0) {
      let level = blocks[i].level;
      for (let j = i - 1; j >= 0 && level > 1; j--) {
        const b = blocks[j];
        if (b.level > 0 && b.level < level) {
          const id = idOf.get(b.key);
          if (id && folded.delete(id)) changed = true;
          level = b.level;
        }
      }
    }
    if (changed) {
      save();
      apply();
    }
    o.reveal(k);
  }

  // mark shows which heading the caret is under.
  function mark() {
    const at = o.caretBlock();
    const hs = o.headings();
    const blocks = o.blockKeys();
    let here: string | null = null;
    for (const b of at ? blocks : []) {
      if (b.level > 0) here = b.key;
      if (b.key === at) break;
    }
    if (!hs.length) here = null;
    for (const b of strip.querySelectorAll<HTMLElement>("[data-key]")) b.classList.toggle("on", b.dataset.key === here);
  }

  // A caret moved into a folded section opens it, so nothing is typed out of sight.
  function caretMoved() {
    const at = o.caretBlock();
    if (at && o.editor.getElementByKey(at)?.classList.contains("rt-folded-away")) {
      const blocks = o.blockKeys();
      const idOf = ids(o.headings());
      const i = blocks.findIndex((b) => b.key === at);
      for (let j = i - 1; j >= 0; j--) {
        const id = idOf.get(blocks[j].key);
        if (id && folded.delete(id)) {
          save();
          apply();
          return;
        }
      }
    }
    if (o.strip) mark();
  }

  const onMove = (e: MouseEvent) => {
    const hd = (e.target as Element).closest?.("h1, h2, h3, h4, h5, h6");
    const k = hd ? keyOfElement(hd as HTMLElement) : null;
    if (k === hover) return;
    hover = k;
    for (const b of gutter.querySelectorAll<HTMLElement>(".rt-fold")) b.classList.toggle("hover", b.dataset.key === hover);
  };
  const keyOfElement = (el: HTMLElement): string | null => {
    for (const hd of o.headings()) if (o.editor.getElementByKey(hd.key) === el) return hd.key;
    return null;
  };
  const onLeave = () => {
    hover = null;
    for (const b of gutter.querySelectorAll<HTMLElement>(".rt-fold.hover")) b.classList.remove("hover");
  };
  o.content.addEventListener("mousemove", onMove);
  o.host.addEventListener("mouseleave", onLeave);
  const resize = new ResizeObserver(() => apply());
  resize.observe(o.content);

  return {
    apply,
    caretMoved,
    setKey(k: string) {
      if (k === key) return;
      const keep = !key && k ? folded : null;
      load(k);
      if (keep && keep.size) {
        folded = keep;
        save();
      }
      apply();
    },
    destroy() {
      resize.disconnect();
      o.content.removeEventListener("mousemove", onMove);
      o.host.removeEventListener("mouseleave", onLeave);
      gutter.remove();
      strip.remove();
    },
  };
}
