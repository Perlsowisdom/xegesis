// The pane's ☰ Contents popover: a "Go to…" field at the top, the places to choose (a Bible
// book's chapters as a grid, or a book's table of contents), and a line of recent places.

import { button, clear, h } from "../dom";
import { closePopover, togglePopover } from "../popover";

export interface ContentsOptions {
  placeholder: string;
  // go is given what was typed; it says whether it found somewhere to go.
  go(text: string): boolean | Promise<boolean>;
  // body draws the choices for what is typed so far ("" at first).
  body(query: string): Node;
  recent: { label: string; go(): void }[];
}

export function contentsPopover(anchor: HTMLElement, o: ContentsOptions): void {
  togglePopover(anchor, () => {
    const input = h("input", { type: "text", placeholder: o.placeholder, autocomplete: "off", spellcheck: "false", "aria-label": o.placeholder.replace(/…$/, "") });
    const note = h("p", { class: "ref-note bad", "aria-live": "polite" });
    const body = h("div", { class: "contents-body" }, o.body(""));
    const form = h("form", { class: "contents-form" }, input);
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      const v = input.value.trim();
      if (!v) return;
      if (await o.go(v)) closePopover(true);
      else note.textContent = "Nothing by that name here.";
    });
    input.addEventListener("input", () => {
      note.textContent = "";
      clear(body, o.body(input.value.trim()));
    });
    return h("div", { class: "pop-body contents-pop" }, form, note, body,
      o.recent.length ? h("div", { class: "contents-recent" }, h("span", { class: "contents-recent-label" }, "Recent"),
        o.recent.slice(0, 6).map((r) => button(r.label, () => { closePopover(true); r.go(); }, { class: "recent-link" }))) : null);
  }, { label: "Contents", side: "below", className: "doc-pop" });
}

// chapterGrid is a Bible book's chapters as buttons, the current one marked.
export function chapterGrid(book: string, chapters: number, current: number, go: (ch: number) => void): HTMLElement {
  return h("div", {},
    h("p", { class: "contents-title" }, book),
    h("div", { class: "chapter-grid", role: "group", "aria-label": `Chapters of ${book}` },
      Array.from({ length: chapters }, (_, i) => i + 1).map((c) => button(String(c), () => { closePopover(true); go(c); },
        { class: `chapter-btn${c === current ? " on" : ""}`, "aria-current": c === current ? "true" : null, "aria-label": `${book} ${c}` }))));
}
