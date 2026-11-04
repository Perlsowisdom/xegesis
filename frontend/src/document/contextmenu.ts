// The right-click menu on selected text, the same in every pane that shows text (Bibles, books,
// PDFs, commentaries) and in both modes: Look up, Highlight, Add a note and Copy within the licence.
// It is the keyboard and right-click way to what the floating toolbar (marking.ts) offers.
// Every item calls a command through the view that owns the text. The menu key and Shift+F10 open it
// from the keyboard, beside the selection.

import { button, h } from "../dom";
import { lookUpCard } from "../lookup";
import type { WordInfo } from "../markup";
import { COLOURS } from "../markingwords";
import { openPopover } from "../popover";

export interface TextActions {
  highlight?(style: string): void | Promise<void>;
  note?(text: string): void;
  copy?(text: string): void | Promise<void>;
  list?(): void; // add the selection to a list
  go?(ref: string): void; // a reference that is looked up opens here
  showWord?(w: WordInfo): void; // the Word tool, for "More in the Word tool"
}

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
  const word = r.startContainer.parentElement?.closest(".w") ?? null;
  const at = r.getBoundingClientRect();
  // An invisible anchor at the pointer, so the menu opens where the click was.
  const anchor = h("span", { class: "ctx-anchor", style: `left:${x}px;top:${y}px` });
  document.body.append(anchor);
  const swatches = a.highlight ? h("div", { class: "row ctx-swatches", role: "group", "aria-label": "Highlight" },
    COLOURS.map((c) => h("button", { type: "button", class: "mark-btn mark-swatch", title: `Highlight ${c.label.toLowerCase()}`,
      "aria-label": `Highlight ${c.label.toLowerCase()}`, onclick: () => void a.highlight!(c.id) }, h("span", { class: `mark-dot dot-${c.id}`, "aria-hidden": "true" }))),
    h("button", { type: "button", class: "mark-btn", title: "Underline", "aria-label": "Underline", onclick: () => void a.highlight!("yellow-underline") },
      h("span", { class: "mark-u", "aria-hidden": "true" }, "U"))) : null;
  const menu = h("div", { class: "menu-list ctx-menu" },
    button(`Look up “${short}”`, () => setTimeout(() => void lookUpCard(text, word, at, { go: a.go, showWord: a.showWord }), 0)),
    swatches ? h("div", { class: "ctx-label" }, "Highlight") : null, swatches,
    a.note ? button("Add a note", () => a.note!(text)) : null,
    a.list ? button("Add to a list", () => a.list!()) : null,
    a.copy ? button("Copy", () => void a.copy!(text)) : null);
  openPopover(anchor, menu, { label: "Selected text", side: "below", menu: true, className: "ctx-pop", onClose: () => setTimeout(() => anchor.remove(), 0) });
}
