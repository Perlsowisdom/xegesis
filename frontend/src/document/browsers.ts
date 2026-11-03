// The compact browsers that sit in a pane: the Library browser (Bibles, commentaries and books) and
// the My work browser (notes, sermons and lists). A row opens in the same pane when clicked, and
// drags into any pane, onto its middle or an edge.

import { ItemSummary, Message, Module, Note, SavedList, run } from "../api";
import { button, clear, h } from "../dom";
import { noteName } from "../notes";
import { untitled } from "../sermonwords";
import type { SermonDoc } from "../views/sermons";
import { draggableSpec } from "./drag";
import type { TabSpec } from "./grid";
import { DocHost, DocView, bibleModules } from "./view";

const LIMIT = 40;

// row is one openable, draggable line.
function row(host: DocHost, label: string, hint: string, spec: TabSpec): HTMLElement {
  const b = button(label, () => host.open(spec), { class: "link browse-open", title: `Open ${label}. Drag it to put it elsewhere in the grid.` });
  const li = h("li", { class: "browse-row" }, b, hint ? h("span", { class: "hint" }, hint) : null);
  draggableSpec(li, () => spec);
  return li;
}

function section(title: string, list: HTMLElement, open = true): HTMLElement {
  return h("details", { class: "browse-section", open }, h("summary", {}, title), list);
}

function searchBox(label: string, onSearch: (q: string) => void): HTMLFormElement {
  const q = h("input", { type: "search", placeholder: label, "aria-label": label, autocomplete: "off" });
  const form = h("form", { role: "search", class: "browse-search" }, q);
  form.addEventListener("submit", (e) => { e.preventDefault(); onSearch(q.value.trim()); });
  let timer = 0;
  q.addEventListener("input", () => { clearTimeout(timer); timer = window.setTimeout(() => onSearch(q.value.trim()), 250); });
  return form;
}

export function libraryBrowser(host: DocHost): DocView {
  const msg = new Message();
  const modules = h("ul", { class: "plain browse-list" });
  const books = h("ul", { class: "plain browse-list" });
  let seq = 0;

  async function find(q: string) {
    const n = ++seq;
    const [ms, items] = await Promise.all([
      bibleModules(),
      msg.attempt(() => run<{ items: ItemSummary[] }>("library.items", { ...(q ? { text: q } : {}), limit: LIMIT })),
    ]);
    if (n !== seq) return;
    const needle = q.toLowerCase();
    const shown = ms.filter((m: Module) => (m.kind === "bible" || m.kind === "commentary" || !m.kind)
      && (!needle || `${m.name} ${m.abbreviation}`.toLowerCase().includes(needle)));
    clear(modules, shown.length === 0 ? h("li", { class: "empty" }, "None found.") : shown.map((m) => m.kind === "commentary"
      ? row(host, m.name, "Commentary", { kind: "commentary", modules: [m.id], title: m.name })
      : row(host, m.name, m.abbreviation || "Bible", { kind: "bible", modules: [m.id] })));
    const list = (items?.items ?? []).filter((it) => it.kind !== "bible" && it.status === "online");
    clear(books, list.length === 0 ? h("li", { class: "empty" }, q ? "No books found." : "No books in your library yet.")
      : list.map((it) => row(host, it.title || "Untitled", it.authors.join(", "), { kind: "book", itemId: it.id, title: it.title || "Untitled" })));
  }

  const el = h("div", { class: "doc doc-browse" },
    searchBox("Find in your library", (q) => void find(q)), msg.el,
    section("Bibles and commentaries", modules), section("Books", books));
  if (host.tab().title !== "Library") host.update({ title: "Library" });
  void find("");
  return { el, destroy: () => { seq++; } };
}

export function workBrowser(host: DocHost): DocView {
  const msg = new Message();
  const notes = h("ul", { class: "plain browse-list" });
  const sermons = h("ul", { class: "plain browse-list" });
  const lists = h("ul", { class: "plain browse-list" });
  let seq = 0;

  async function find(q: string) {
    const n = ++seq;
    const query = q ? { query: q } : {};
    const [ns, ds, ls] = await Promise.all([
      msg.attempt(() => run<Note[]>("note.search", { ...query, limit: LIMIT })),
      msg.attempt(() => run<SermonDoc[]>("document.list", query)),
      msg.attempt(() => run<SavedList[]>("list.list")),
    ]);
    if (n !== seq) return;
    const empty = (what: string) => h("li", { class: "empty" }, q ? "None found." : `No ${what} yet.`);
    clear(notes, !ns?.length ? empty("notes") : ns.map((x) => row(host, noteName(x), x.ref?.display ?? x.item_title ?? "", { kind: "note", target: x.id, title: noteName(x) })));
    clear(sermons, !ds?.length ? empty("sermons") : ds.slice(0, LIMIT).map((d) => row(host, d.title || untitled(d.kind), d.passage?.display ?? "",
      { kind: "sermon", target: d.id, title: d.title || untitled(d.kind) })));
    const needle = q.toLowerCase();
    const lf = (ls ?? []).filter((l) => !needle || l.name.toLowerCase().includes(needle));
    clear(lists, !lf.length ? empty("lists") : lf.map((l) => row(host, l.name, `${l.count}`, { kind: "list", target: l.id, title: l.name })));
  }

  const el = h("div", { class: "doc doc-browse" },
    h("div", { class: "row wrap" }, searchBox("Find in your work", (q) => void find(q)),
      button("New note", () => host.open({ kind: "note" }))),
    msg.el,
    section("Notes", notes), section("Sermons and lessons", sermons), section("Lists", lists));
  if (host.tab().title !== "My work") host.update({ title: "My work" });
  void find("");
  return { el, destroy: () => { seq++; } };
}
