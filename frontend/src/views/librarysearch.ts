// Searching the library's books (search.query) and finding books that cite a passage
// (search.refs). Each hit opens in the reader's book pane at its locator.

import { Message, paramEnum, run } from "../api";
import { collectionPicker } from "../collections";
import { clear, field, h, select } from "../dom";
import { markSegments } from "../helpers";
import { navigate, state } from "../state";
import { Locator, openBook } from "./book";

interface BookHit {
  item_id: string;
  title: string;
  authors: string[];
  file_id: string;
  format: string;
  online: boolean;
  heading?: string;
  locator: Locator;
  snippet: string;
  cites?: string;
}

interface QueryResult {
  hits: BookHit[];
  more: boolean;
  incomplete: string[];
}

const MODE_LABELS: Record<string, string> = {
  all: "All the words", any: "Any of the words", phrase: "Exact phrase", fts: "Search syntax",
};

const PAGE = 50;

function hitList(hits: BookHit[]) {
  return hits.map((hit) => {
    const open = h("button", { type: "button", class: "link", disabled: !hit.online }, hit.title || "Untitled");
    open.addEventListener("click", () => {
      navigate("reader");
      openBook({ itemId: hit.item_id, fileId: hit.file_id, locator: hit.locator });
    });
    const where = hit.locator.page ? `page ${hit.locator.page}` : hit.heading ?? "";
    return h("li", {},
      h("div", { class: "row wrap" }, open,
        hit.authors.length ? h("span", { class: "hint" }, hit.authors.join(", ")) : null,
        where ? h("span", { class: "badge" }, where) : null,
        hit.cites ? h("span", { class: "badge ok" }, hit.cites) : null,
        hit.online ? null : h("span", { class: "badge warn", title: "Its library folder's drive is not connected" }, "offline")),
      h("p", {}, markSegments(hit.snippet).map((s) => (s.mark ? h("mark", {}, s.text) : s.text))));
  });
}

function summary(r: QueryResult, offset: number): string {
  if (r.hits.length === 0 && offset === 0) return "No matches.";
  const shown = `Showing ${offset + 1}–${offset + r.hits.length}${r.more ? " (more follow)" : ""}.`;
  return r.incomplete.length ? `${shown} Some library folders are still being indexed, so results may be missing.` : shown;
}

// pager shows Previous and Next for a paged search.
function pager(offset: number, more: boolean, go: (offset: number) => void) {
  const prev = h("button", { type: "button", disabled: offset === 0 }, "Previous");
  const next = h("button", { type: "button", disabled: !more }, "Next");
  prev.addEventListener("click", () => go(Math.max(0, offset - PAGE)));
  next.addEventListener("click", () => go(offset + PAGE));
  return h("div", { class: "row" }, prev, next);
}

export function librarySearchPanel() {
  const msg = new Message();
  const query = h("input", { type: "search", autocomplete: "off", placeholder: "e.g. justification by faith" });
  const mode = select([], "all");
  paramEnum("search.query", "mode").then((ms) => ms.forEach((m) => mode.append(h("option", { value: m, selected: m === "all" }, MODE_LABELS[m] ?? m))));
  const author = h("input", { type: "text", autocomplete: "off", placeholder: "Optional" });
  const where = collectionPicker();
  const count = h("p", { class: "hint", role: "status", "aria-live": "polite" });
  const results = h("ol", { class: "results" });
  const more = h("div", {});
  const form = h("form", { class: "card", role: "search", "aria-label": "Search the library" },
    h("div", { class: "grid3" }, field("Words", query), field("Match", mode), field("Author", author)),
    h("div", { class: "grid3" }, where.el),
    h("div", { class: "row" }, h("button", { type: "submit", class: "primary" }, "Search books")));
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    search(0);
  });

  async function search(offset: number) {
    if (!query.value.trim()) return;
    count.textContent = "Searching…";
    const p: Record<string, unknown> = { query: query.value.trim(), mode: mode.value || "all", limit: PAGE, offset };
    if (author.value.trim()) p.author = author.value.trim();
    if (where.value()) p.collection_id = where.value();
    const r = await msg.attempt(() => run<QueryResult>("search.query", p));
    if (!r) return void (count.textContent = "");
    count.textContent = summary(r, offset);
    clear(results, hitList(r.hits));
    results.start = offset + 1;
    clear(more, r.more || offset > 0 ? pager(offset, r.more, search) : null);
  }

  const el = h("div", {},
    h("p", { class: "hint" }, "Searches the text of the books in your library folders. Index a folder in the Library section first."),
    form, msg.el, count, results, more);
  return { el, focus: () => query.focus(), show: where.reload };
}

export function citingPanel() {
  const msg = new Message();
  const ref = h("input", { type: "search", autocomplete: "off", placeholder: "e.g. Rom 8:28" });
  const where = collectionPicker();
  const count = h("p", { class: "hint", role: "status", "aria-live": "polite" });
  const results = h("ol", { class: "results" });
  const more = h("div", {});
  const form = h("form", { class: "card", role: "search", "aria-label": "Find books citing a passage" },
    h("div", { class: "grid3" }, field("Passage", ref, "Books that cite verses in this passage"), where.el),
    h("div", { class: "row" }, h("button", { type: "submit", class: "primary" }, "Find books")));
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    search(0);
  });

  async function search(offset: number) {
    if (!ref.value.trim()) return;
    count.textContent = "Searching…";
    const r = await msg.attempt(() => run<QueryResult>("search.refs", { ref: ref.value.trim(), limit: PAGE, offset, ...(where.value() ? { collection_id: where.value() } : {}) }));
    if (!r) return void (count.textContent = "");
    count.textContent = summary(r, offset);
    clear(results, hitList(r.hits));
    results.start = offset + 1;
    clear(more, r.more || offset > 0 ? pager(offset, r.more, search) : null);
  }

  const el = h("div", {}, form, msg.el, count, results, more);
  return {
    el,
    // show fills in the passage being read, if the box is empty.
    show() {
      if (!ref.value && state.ref) ref.value = state.ref;
      where.reload();
    },
    focus: () => ref.focus(),
  };
}
