// The picker for an empty tab, and the ⋯ menu: contents, search in the book, link and print.

import { openExport } from "../printexport";
import { ItemSummary, Message, run } from "../api";
import { tocOptions } from "../bookhelpers";
import { button, clear, h } from "../dom";
import { closePopover, togglePopover } from "../popover";
import { markSegments } from "../helpers";
import type { Locator } from "./open";
import { contentsPopover } from "./contentspop";
import type { BookCtx } from "./bookctx";
import { go, jump, open } from "./bookpaging";
import { selection } from "./bookmarks";
import { inkMenu, isPdf } from "./bookink";

// Choosing a book ---------------------------------------------------------------------------------

export function choose(c: BookCtx) {
  const q = h("input", { type: "search", placeholder: "Title", autocomplete: "off", "aria-label": "Find a book by title" });
  const list = h("ul", { class: "results" });
  const pmsg = new Message();
  async function find() {
    const r = await pmsg.attempt(() => run<{ items: ItemSummary[] }>("library.items", { text: q.value.trim(), limit: 30 }));
    if (!r || !c.alive) return;
    const books = r.items.filter((it) => it.kind !== "bible");
    clear(list, books.length === 0 ? h("li", { class: "empty" }, "No books found.") : books.map((it) =>
      h("li", {}, button(it.title || "Untitled", () => {
        c.picker.hidden = true;
        c.host.update({ itemId: it.id, title: it.title || "Untitled" });
        void open(c, { itemId: it.id });
      }, { class: "link", disabled: it.status !== "online" }),
      it.authors.length ? h("span", { class: "hint" }, ` ${it.authors.join(", ")}`) : null,
      it.status !== "online" ? h("span", { class: "badge warn" }, it.status) : null)));
  }
  const form = h("form", { role: "search", class: "row" }, q, h("button", { type: "submit", class: "primary" }, "Find"));
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    void find();
  });
  clear(c.picker, h("h3", {}, "Open a book"), form, pmsg.el, list);
  c.picker.hidden = false;
  queueMicrotask(() => q.focus());
  void find();
}

export function print(c: BookCtx) {
  if (!c.book) return;
  const r = selection(c);
  const where = c.book.reflowable ? `section ${c.section}` : `page ${c.section}`;
  openExport(r
    ? { title: c.book.title, sources: [{ kind: "book", item_id: c.book.item_id, file_id: c.book.file_id, text: r.toString() }] }
    : { title: `${c.book.title}, ${where}`, sources: [{ kind: "book", item_id: c.book.item_id, file_id: c.book.file_id, from: c.section, to: c.section }] });
}

export function bookMenu(c: BookCtx) {
  // The ⋯ menu ----------------------------------------------------------------------------------------

  const menu = {
    // ☰ Contents: the table of contents, narrowed by what is typed, and the places visited.
    contents(anchor: HTMLElement) {
      const unit = c.book?.reflowable ? "Section" : "Page";
      const entries = tocOptions(c.toc);
      contentsPopover(anchor, {
        placeholder: c.toc.length ? "Go to a chapter or section…" : `Go to a ${unit.toLowerCase()} number…`,
        go: (t) => {
          const n = Number(t);
          if (Number.isInteger(n) && n > 0) { go(c, n); return true; }
          const hit = entries.find(([, label]) => label.trim().toLowerCase().includes(t.toLowerCase()));
          const e = hit && c.toc[Number(hit[0])];
          if (!e) return false;
          jump(c, e.section, { anchor: e.anchor, flash: !!e.anchor });
          return true;
        },
        body: (q) => {
          const shown = entries.filter(([, label]) => !q || label.toLowerCase().includes(q.toLowerCase()));
          if (c.toc.length === 0) return h("p", { class: "hint" }, "This book has no contents list. Type a number to go there.");
          return h("div", { class: "menu-list toc-list" }, shown.length === 0 ? h("p", { class: "hint" }, "No heading has those words.")
            : shown.map(([v, label]) => button(label, () => {
              closePopover(true);
              const t = c.toc[Number(v)];
              if (t) jump(c, t.section, { anchor: t.anchor, flash: !!t.anchor });
            }, { class: Number(v) === c.tocAt ? "on" : "" })));
        },
        recent: [...c.back].reverse().map((p, i) => ({ label: `${unit} ${p.section}`, go: () => {
          c.back.splice(c.back.length - 1 - i);
          go(c, p.section, { para: p.para });
        } })),
      });
    },
    link: () => (c.book ? `#book/${encodeURIComponent(c.book.item_id)}?file=${encodeURIComponent(c.book.file_id)}${c.book.reflowable ? "" : `&page=${c.section}`}` : null),
    print: () => print(c),
    get ink() {
      return isPdf(c) ? inkMenu(c) : null;
    },
    search(anchor: HTMLElement) {
      const q = h("input", { type: "search", "aria-label": "Search in this book", placeholder: "Words to find", autocomplete: "off" });
      const form = h("form", { class: "pop-body", role: "search" }, h("h2", { class: "pop-title" }, "Search in this"), q);
      form.addEventListener("submit", (e) => { e.preventDefault(); void menu.find(q.value.trim(), anchor); });
      togglePopover(anchor, () => form, { label: "Search in this", side: "below" });
    },
    async find(q: string, anchor: HTMLElement) {
      if (!c.book || !q) return;
      const b = c.book;
      const r = await c.msg.attempt(() => run<{ hits: { heading?: string; snippet: string; locator: Locator }[] }>("search.query",
        { query: q, item_ids: [b.item_id], limit: 30 }));
      if (!r || !c.alive) return;
      closePopover();
      togglePopover(anchor, () => h("div", { class: "pop-body find-results" },
        h("p", { class: "hint", role: "status" }, r.hits.length ? `${r.hits.length} ${r.hits.length === 1 ? "match" : "matches"} in ${b.title}` : `No matches in ${b.title}.`),
        h("ol", { class: "results" }, r.hits.map((hit) => h("li", {},
          button(hit.locator.page ? `Page ${hit.locator.page}` : hit.heading || "Show", async () => {
            closePopover();
            const loc = await c.msg.attempt(() => run<{ section: number; para?: number; anchor?: string }>("book.locate",
              { item_id: b.item_id, file_id: b.file_id, locator: hit.locator }));
            if (loc && c.alive) jump(c, loc.section, { anchor: loc.anchor, para: loc.para, flash: true });
          }, { class: "link" }),
          h("p", {}, markSegments(hit.snippet).map((s) => (s.mark ? h("mark", {}, s.text) : s.text))))))),
      { label: "Matches", side: "below" });
    },
  };
  return menu;
}
