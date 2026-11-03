// The right-click menu on selected text, the same in every pane that shows text (Bibles, books,
// PDFs, commentaries) and in both modes: Look up, Highlight, Add a note and Copy within the licence.
// Every item calls a command through the view that owns the text. The menu key and Shift+F10 open it
// from the keyboard, beside the selection.

import { Ref, run } from "../api";
import { button, h } from "../dom";
import { paragraphs } from "../markup";
import { closePopover, openPopover } from "../popover";
import { searchFor } from "../views/search";

export interface TextActions {
  highlight?(style: string): void | Promise<void>;
  note?(text: string): void;
  copy?(text: string): void | Promise<void>;
  go?(ref: string): void; // a reference that is looked up opens here
}

export const HIGHLIGHTS: [string, string][] = [
  ["yellow", "Yellow"], ["green", "Green"], ["blue", "Blue"], ["pink", "Pink"], ["purple", "Purple"], ["underline", "Underline"],
];

interface LexEntry { name: string; key: string; lemma?: string; gloss?: string; text: string; markup?: string }

// selectedIn is the selection inside root, trimmed to text that is really there.
export function selectedIn(root: HTMLElement): Range | null {
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0 || sel.isCollapsed) return null;
  const r = sel.getRangeAt(0);
  if (!root.contains(r.commonAncestorContainer) && !r.intersectsNode(root)) return null;
  return r.toString().trim() ? r : null;
}

export function attachContextMenu(root: HTMLElement, actions: TextActions): void {
  root.addEventListener("contextmenu", (e) => {
    const r = selectedIn(root);
    if (!r) return;
    e.preventDefault();
    let x = e.clientX;
    let y = e.clientY;
    if (!x && !y) {
      const box = r.getBoundingClientRect();
      x = box.left;
      y = box.bottom;
    }
    open(r, x, y, actions);
  });
}

function open(r: Range, x: number, y: number, a: TextActions) {
  const text = r.toString().replace(/\s+/g, " ").trim();
  const short = text.length > 24 ? `${text.slice(0, 24)}…` : text;
  const word = (r.startContainer.parentElement?.closest(".w") ?? null) as HTMLElement | null;
  // An invisible anchor at the pointer, so the menu opens where the click was.
  const anchor = h("span", { class: "ctx-anchor", style: `left:${x}px;top:${y}px` });
  document.body.append(anchor);
  const swatches = a.highlight ? h("div", { class: "row ctx-swatches", role: "group", "aria-label": "Highlight" },
    HIGHLIGHTS.map(([style, name]) => h("button", { type: "button", class: `swatch-btn hl-${style}`, title: `Highlight ${name.toLowerCase()}`,
      "aria-label": `Highlight ${name.toLowerCase()}`, onclick: () => void a.highlight!(style) }, style === "underline" ? "U" : ""))) : null;
  const menu = h("div", { class: "menu-list ctx-menu" },
    button(`Look up “${short}”`, () => setTimeout(() => void lookUp(text, word, a, anchor), 0)),
    swatches ? h("div", { class: "ctx-label" }, "Highlight") : null, swatches,
    a.note ? button("Add a note", () => a.note!(text)) : null,
    a.copy ? button("Copy", () => void a.copy!(text)) : null);
  openPopover(anchor, menu, { label: "Selected text", side: "below", menu: true, className: "ctx-pop", onClose: () => setTimeout(() => anchor.remove(), 0) });
}

// lookUp opens a reference in the Bible, a tagged word in the Word tool, or else looks the word up
// in the lexicons and dictionaries, offering a library search when they have nothing.
async function lookUp(text: string, word: HTMLElement | null, a: TextActions, anchor: HTMLElement) {
  if (word) {
    word.click();
    return;
  }
  if (/\d/.test(text) && text.length < 40) {
    const p = await run<Ref>("verse.parse", { ref: text }).catch(() => null);
    if (p && a.go) return a.go(p.display);
  }
  const q = text.split(" ").slice(0, 3).join(" ");
  const entries = await run<LexEntry[]>("lexicon.lookup", { word: q, limit: 3 }).catch(() => [] as LexEntry[]);
  const back = h("span", { class: "ctx-anchor", style: anchor.getAttribute("style") ?? "" });
  document.body.append(back);
  openPopover(back, h("div", { class: "pop-body lookup" },
    h("h2", { class: "pop-title" }, `Look up “${q}”`),
    entries.length === 0 ? h("p", { class: "hint" }, "Nothing in your lexicons and dictionaries.") :
      entries.map((e) => h("div", { class: "lex-entry" }, h("h3", {}, `${e.lemma || e.key} · ${e.name}`),
        e.gloss ? h("p", { class: "hint" }, e.gloss) : null, paragraphs(e.markup || e.text, { onRef: (o: string) => a.go?.(o) }))),
    button("Search the library for it", () => { closePopover(); searchFor(q); }, { class: "link" })),
  { label: "Look up", side: "below", onClose: () => setTimeout(() => back.remove(), 0) });
}
