// Search: tabs for words in the installed Bibles, words in the library's books, and books that
// cite a passage (views/librarysearch.ts).

import { installedBibles } from "../bibles";
import { bibleBooks, type BibleBook } from "../biblebooks";
import { openBook, type Locator } from "../document/open";
import { chip } from "../controls";
import { Message, Module, Ref, paramEnum, run } from "../api";
import { clear, field, h, select } from "../dom";
import { markSegments } from "../helpers";
import { navigate, setModules, setPassage, state } from "../state";
import { citingPanel, librarySearchPanel } from "./librarysearch";

interface Hit {
  id: number;
  end: number;
  module: string;
  ref: string;
  text: string;
  snippet?: string;
  score: number;
}

const BOOK = "book:";

interface BookHit {
  item_id: string;
  file_id: string;
  title: string;
  heading?: string;
  locator: Locator;
  snippet: string;
}

const MODE_LABELS: Record<string, string> = {
  all: "All the words", any: "Any of the words", phrase: "Exact phrase", fts: "Search syntax",
};

// Other parts of the app (the rail's search flyout, the Reader's search box) search through
// searchFor; the Search view runs the words when it is next shown.
let pending: string | null = null;

export function searchFor(words: string) {
  if (!words.trim()) return;
  pending = words.trim();
  navigate("search");
}

export function searchView() {
  const msg = new Message();
  const query = h("input", { type: "search", id: "search-q", autocomplete: "off", placeholder: "e.g. love one another" });
  const mode = select([], "all");
  paramEnum("bible.search", "mode").then((ms) => ms.forEach((m) => mode.append(h("option", { value: m, selected: m === "all" }, MODE_LABELS[m] ?? m))));
  const within = h("input", { type: "text", placeholder: "e.g. John, or Rom–Gal", autocomplete: "off" });
  const strong = h("input", { type: "text", placeholder: "e.g. G26", autocomplete: "off" });
  const modules = h("div", { class: "module-picker", role: "group", "aria-label": "Bibles to search" });
  const results = h("ol", { class: "results" });
  const count = h("p", { class: "hint", role: "status", "aria-live": "polite" });
  let installed: Module[] = [];
  let books: BibleBook[] = [];

  const form = h("form", { class: "card", role: "search", "aria-label": "Search the Bibles" },
    h("div", { class: "grid3" },
      field("Words", query),
      field("Match", mode),
      field("Only in", within, "Optional reference range")),
    h("div", { class: "grid3" }, field("Strong's number", strong, "Optional; for Bibles tagged with Strong's numbers")),
    modules,
    h("div", { class: "row" }, h("button", { type: "submit", class: "primary" }, "Search")));
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    search();
  });

  const bibleTab = h("div", {},
    h("p", { class: "hint" }, "Searches the text of your installed Bibles, and of library books typed Bible."),
    form, msg.el, count, results);
  const library = librarySearchPanel();
  const citing = citingPanel();
  const tabs = searchTabs([
    { id: "bibles", label: "Bibles", el: bibleTab, focus: () => query.focus() },
    { id: "library", label: "Library", el: library.el, focus: library.focus, show: library.show },
    { id: "citing", label: "Books citing a passage", el: citing.el, focus: citing.focus, show: citing.show },
  ]);
  const el = h("div", { class: "view" }, h("h1", {}, "Search"), tabs.el);

  async function search() {
    const p: Record<string, unknown> = { mode: mode.value || "all", limit: 200 };
    if (query.value.trim()) p.query = query.value.trim();
    if (strong.value.trim()) p.strong = strong.value.trim();
    if (within.value.trim()) p.ref = within.value.trim();
    const chosen = [...modules.querySelectorAll<HTMLInputElement>("input:checked")].map((b) => b.value);
    const bookIds = chosen.filter((v) => v.startsWith(BOOK)).map((v) => v.slice(BOOK.length));
    const mods = chosen.filter((v) => !v.startsWith(BOOK));
    if (mods.length) p.modules = mods;
    count.textContent = "Searching…";
    // Books are searched by their words only: a verse range or Strong's number applies to Bibles.
    const inBooks = bookIds.length > 0 && !!query.value.trim() && !strong.value.trim() && !within.value.trim();
    const hits = bookIds.length > 0 && mods.length === 0 ? [] : await msg.attempt(() => run<Hit[]>("bible.search", p));
    if (!hits) return void (count.textContent = "");
    const bookHits = inBooks ? ((await msg.attempt(() => run<{ hits: BookHit[] }>("search.query", { query: query.value.trim(), mode: mode.value || "all", item_ids: bookIds, limit: 50 })))?.hits ?? []) : [];
    const total = hits.length + bookHits.length;
    count.textContent = total === 0 ? "No matches." : `${total} matches${hits.length === 200 ? " (showing the best 200 Bible verses)" : ""}.`;
    clear(results, [...bookHits.map((hit) => {
      const open = h("button", { type: "button", class: "link" }, [hit.title, hit.heading].filter(Boolean).join(" · "));
      open.addEventListener("click", () => {
        navigate("reader");
        openBook({ itemId: hit.item_id, fileId: hit.file_id, locator: hit.locator });
      });
      return h("li", {}, h("div", { class: "row" }, open, h("span", { class: "badge" }, "book")),
        h("p", {}, markSegments(hit.snippet).map((s) => (s.mark ? h("mark", {}, s.text) : s.text))));
    }), ...hits.map((hit) => {
      const name = installed.find((m) => m.id === hit.module)?.abbreviation || hit.module;
      const segs = markSegments(hit.snippet || hit.text);
      const open = h("button", { type: "button", class: "link" }, hit.ref);
      open.addEventListener("click", () => openRef(hit));
      return h("li", {},
        h("div", { class: "row" }, open, h("span", { class: "badge" }, name)),
        h("p", {}, segs.map((s) => (s.mark ? h("mark", {}, s.text) : s.text))));
    })]);
  }

  async function openRef(hit: Hit) {
    const p = await msg.attempt(() => run<Ref>("verse.parse", { ref: hit.ref }));
    if (!p) return;
    if (!state.modules.includes(hit.module)) setModules([hit.module, ...state.modules]);
    navigate("reader");
    setPassage(p.display, p);
  }

  async function show() {
    const asked = pending;
    pending = null;
    if (asked) {
      tabs.select(0);
      query.value = asked;
    }
    installed = await installedBibles();
    books = await bibleBooks();
    const checked = new Set([...modules.querySelectorAll<HTMLInputElement>("input:checked")].map((b) => b.value));
    clear(modules, h("span", { class: "hint" }, installed.length || books.length ? "Bibles:" : "No Bibles installed."),
      installed.map((m) => chip({ label: m.abbreviation || m.id, title: m.name, value: m.id, checked: checked.size ? checked.has(m.id) : true })),
      books.map((b) => chip({ label: `${b.title} · book`, title: `${b.title}, a library book typed Bible`, value: BOOK + b.id, checked: checked.size ? checked.has(BOOK + b.id) : true })));
    if (asked) await search();
  }

  return { el, show, focus: tabs.focus };
}

interface Tab {
  id: string;
  label: string;
  el: HTMLElement;
  focus(): void;
  show?(): void;
}

// searchTabs is a tab list (arrow keys move between tabs) over one panel per tab.
function searchTabs(tabs: Tab[]) {
  let current = 0;
  const buttons = tabs.map((t, i) => {
    const b = h("button", { type: "button", role: "tab", id: `search-tab-${t.id}`, "aria-controls": `search-panel-${t.id}` }, t.label);
    b.addEventListener("click", () => select(i, true));
    b.addEventListener("keydown", (e) => {
      const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
      if (!d) return;
      e.preventDefault();
      select((current + d + tabs.length) % tabs.length, false);
      buttons[current].focus();
    });
    t.el.id = `search-panel-${t.id}`;
    t.el.setAttribute("role", "tabpanel");
    t.el.setAttribute("aria-labelledby", b.id);
    return b;
  });
  function select(i: number, focus: boolean) {
    current = i;
    tabs.forEach((t, j) => {
      t.el.hidden = j !== i;
      buttons[j].setAttribute("aria-selected", String(j === i));
      buttons[j].tabIndex = j === i ? 0 : -1;
    });
    tabs[i].show?.();
    if (focus) tabs[i].focus();
  }
  select(0, false);
  const el = h("div", {}, h("div", { class: "tabs", role: "tablist", "aria-label": "What to search" }, buttons), tabs.map((t) => t.el));
  return { el, focus: () => tabs[current].focus(), select: (i: number) => select(i, false) };
}
