// The section and page field: where the reader is, and a place typed in.

import { currentEntry, tocOptions } from "../bookhelpers";
import { button, h } from "../dom";
import { closePopover, togglePopover } from "../popover";
import type { BookCtx } from "./bookctx";
import { go, jump } from "./bookpaging";

// buildPlace makes the section / page field: type a number and press Enter, or words to find a heading in the contents.
export function buildPlace(c: BookCtx) {
  c.prev = button("‹", () => go(c, c.section - 1), { "aria-label": "Previous section", title: "Previous section (Alt+Page Up)", "aria-keyshortcuts": "Alt+PageUp" });
  c.next = button("›", () => go(c, c.section + 1), { "aria-label": "Next section", title: "Next section (Alt+Page Down)", "aria-keyshortcuts": "Alt+PageDown" });
  c.unitText = h("span", { class: "book-unit" }, "Section");
  c.whereInput = h("input", { type: "text", class: "book-where-input", autocomplete: "off", spellcheck: "false",
    "aria-label": "Go to a section number, or words from a heading", title: "Type a number, or words from a heading, then press Enter" });
  c.ofText = h("span", { class: "book-of" });
  const where = h("span", { class: "hint book-where" }, c.unitText, c.whereInput, c.ofText);
  c.placeBox = h("div", { class: "book-nav", role: "group", "aria-label": "Place in the book", hidden: true }, c.prev, where, c.next);
  wireWhere(c);
}

// typedPlace handles Enter in the place field: a number goes there, words list the matching headings.
export function typedPlace(c: BookCtx) {
  const t = c.whereInput.value.trim();
  c.typing = false;
  if (!c.book || !t) return;
  const n = Number(t);
  if (/^\d+$/.test(t)) {
    if (n >= 1 && n <= c.book.sections) { jump(c, n, {}); c.whereInput.blur(); }
    else { c.whereInput.value = String(c.section); c.whereInput.title = `Choose a number from 1 to ${c.book.sections}`; c.whereInput.select(); }
    return;
  }
  const found = tocOptions(c.toc).filter(([, label]) => label.toLowerCase().includes(t.toLowerCase()));
  closePopover();
  togglePopover(c.whereInput, () => h("div", { class: "pop-body" }, found.length === 0
    ? h("p", { class: "hint", role: "status" }, "No heading has those words.")
    : h("div", { class: "menu-list toc-list" }, found.map(([v, label]) => button(label, () => {
      closePopover(true);
      const e = c.toc[Number(v)];
      if (e) jump(c, e.section, { anchor: e.anchor, flash: !!e.anchor });
    })))), { label: "Matching headings", side: "below", menu: false });
}

// header shows where the reader is: the place label, the contents entry and the pane's saved place.
export function header(c: BookCtx, n: number) {
  if (!c.book) return;
  c.section = n;
  c.unitText.textContent = c.book.format === "pdf" ? "Page" : "Section";
  c.ofText.textContent = `of ${c.book.sections}`;
  c.whereInput.setAttribute("aria-label", c.book.format === "pdf" ? "Go to a page number" : "Go to a section number, or words from a heading");
  if (!c.typing) c.whereInput.value = String(n);
  if (c.host.tab().section !== n) c.host.update({ section: n });
  c.prev.disabled = n <= 1;
  c.next.disabled = n >= c.book.sections;
  c.tocAt = currentEntry(c.toc, n);
  document.dispatchEvent(new Event("xegesis:book-section"));
}

export function wireWhere(c: BookCtx) {
  c.whereInput.addEventListener("input", () => { c.typing = true; });
  c.whereInput.addEventListener("focus", () => c.whereInput.select());
  c.whereInput.addEventListener("keydown", (e) => {
    e.stopPropagation();
    if (e.key === "Enter") { e.preventDefault(); typedPlace(c); }
    else if (e.key === "Escape") { c.typing = false; c.whereInput.value = String(c.section); c.whereInput.blur(); }
  });
  c.whereInput.addEventListener("blur", () => { c.typing = false; c.whereInput.value = String(c.section); });
}
