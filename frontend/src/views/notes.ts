// The Notes section: every note, newest first, with search, filters and Recently deleted.

import { Message, Note, TagCount, run } from "../api";
import { button, clear, field, h, select } from "../dom";
import { formatTime } from "../helpers";
import { CardActions, deleteNote, historyDrawer, noteCard, noteForm, noteName, placeOf } from "../notes";

interface Sources {
  bible_books: { name: string; count: number }[];
  library_books: { item_id: string; title: string; count: number }[];
  free: number;
  deleted: number;
}

const PAGE = 50;

export function notesSectionView() {
  const msg = new Message();
  const query = h("input", { type: "search", placeholder: "Search titles, text and tags", "aria-label": "Search notes" });
  const tag = select([["", "Any tag"]], "", { "aria-label": "Tag" });
  const bible = select([["", "Any book of the Bible"]], "", { "aria-label": "Book of the Bible" });
  const library = select([["", "Any library book"]], "", { "aria-label": "Library book" });
  const on = select([["", "Attached anywhere"], ["verses", "On Bible verses"], ["books", "In library books"], ["nothing", "Not attached"]], "", { "aria-label": "Attached to" });
  const list = h("ul", { class: "notes", "aria-live": "polite" });
  const editor = h("div", { class: "note-editor" });
  const summary = h("p", { class: "hint", "aria-live": "polite" });
  const more = h("div", {});
  const filters = h("div", { class: "filters row wrap" }, query, tag, bible, library, on, button("Clear", reset));
  const trash = h("div", { hidden: true });
  const main = h("div", {}, filters, summary, editor, list, more);
  const drawer = historyDrawer(msg, () => search());
  const showDeleted = h("button", { type: "button", "aria-pressed": "false" }, "Recently deleted");

  const el = h("div", { class: "view" },
    h("div", { class: "row between" }, h("h1", {}, "Notes"),
      h("div", { class: "row" }, showDeleted, button("New note", () => edit(null), { class: "primary", "aria-keyshortcuts": "Alt+N" }))),
    msg.el, main, trash, drawer.el);

  let shown: Note[] = [];
  let timer = 0;
  let deleted = false;

  const actions: CardActions = {
    edit: (n) => edit(n),
    history: (n) => drawer.open(n),
    remove: (n) => deleteNote(msg, n, () => { search(); sources(); }),
  };

  function params() {
    const p: Record<string, unknown> = {};
    if (query.value.trim()) p.query = query.value.trim();
    if (tag.value) p.tag = tag.value;
    if (bible.value) p.book = bible.value;
    if (library.value) p.item_id = library.value;
    if (on.value) p.on = on.value;
    return p;
  }

  function filtered() {
    return Object.keys(params()).length > 0;
  }

  async function search(append = false) {
    const offset = append ? shown.length : 0;
    const got = await msg.attempt(() => run<Note[]>("note.search", { ...params(), limit: PAGE + 1, offset }));
    if (!got) return;
    const page = got.slice(0, PAGE);
    shown = append ? [...shown, ...page] : page;
    clear(list, shown.length === 0
      ? h("li", { class: "empty" }, filtered() ? "No notes match." : "No notes yet. Select New note to write one.")
      : shown.map((n) => noteCard(n, actions)));
    clear(more, got.length > PAGE ? button("Show more", () => search(true)) : null);
    summary.textContent = shown.length === 0 ? "" : `${shown.length}${got.length > PAGE ? "+" : ""} ${shown.length === 1 ? "note" : "notes"}${filtered() ? " found" : ""}, newest first.`;
  }

  async function sources() {
    const [s, tags] = await Promise.all([
      msg.attempt(() => run<Sources>("note.sources")),
      msg.attempt(() => run<TagCount[]>("note.tags")),
    ]);
    if (!s || !tags) return;
    fill(tag, [["", "Any tag"], ...tags.map((t): [string, string] => [t.tag, `${t.tag} (${t.count})`])]);
    fill(bible, [["", "Any book of the Bible"], ...s.bible_books.map((b): [string, string] => [b.name, `${b.name} (${b.count})`])]);
    fill(library, [["", "Any library book"], ...s.library_books.map((b): [string, string] => [b.item_id, `${b.title} (${b.count})`])]);
    showDeleted.textContent = s.deleted ? `Recently deleted (${s.deleted})` : "Recently deleted";
  }

  function fill(sel: HTMLSelectElement, options: [string, string][]) {
    const keep = sel.value;
    clear(sel, options.map(([v, l]) => h("option", { value: v, selected: v === keep }, l)));
    sel.value = options.some(([v]) => v === keep) ? keep : "";
  }

  function reset() {
    query.value = "";
    for (const s of [tag, bible, library, on]) s.value = "";
    search();
  }

  function edit(n: Note | null) {
    setDeleted(false);
    clear(editor, noteForm(msg, {
      note: n,
      onDone: (saved) => {
        clear(editor);
        if (saved) {
          msg.ok(n ? "Note saved." : "Note created.");
          search();
          sources();
        }
      },
    }));
  }

  // Recently deleted --------------------------------------------------------------------------------

  async function loadDeleted() {
    const got = await msg.attempt(() => run<Note[]>("note.deleted", { limit: 200 }));
    if (!got) return;
    clear(trash,
      h("h2", {}, "Recently deleted"),
      h("p", { class: "hint" }, "Deleted notes stay here until you restore them."),
      h("ul", { class: "notes" }, got.length === 0 ? h("li", { class: "empty" }, "Nothing has been deleted.") : got.map((n) =>
        h("li", { class: "note" },
          h("div", { class: "row between" }, h("strong", {}, noteName(n)), placeOf(n) ? h("span", { class: "badge" }, placeOf(n)) : null),
          h("p", { class: "note-body" }, n.body.length > 300 ? n.body.slice(0, 300) + "…" : n.body),
          h("div", { class: "row small" },
            h("span", { class: "hint" }, `Deleted ${formatTime(n.deleted_at ?? n.updated_at)}`),
            button("Restore", async () => {
              if ((await msg.attempt(() => run("note.restore", { id: n.id }))) !== undefined) {
                msg.ok(`Restored "${noteName(n)}".`);
                loadDeleted();
                sources();
              }
            }, { "aria-label": `Restore ${noteName(n)}` }),
            button("History", () => drawer.open(n), { "aria-label": `History of ${noteName(n)}` }))))));
  }

  function setDeleted(on: boolean) {
    deleted = on;
    trash.hidden = !on;
    main.hidden = on;
    showDeleted.setAttribute("aria-pressed", String(on));
    showDeleted.textContent = on ? "Back to notes" : showDeleted.textContent!.replace(/^Back to notes$/, "Recently deleted");
    if (on) loadDeleted();
    else search();
  }
  showDeleted.addEventListener("click", () => {
    setDeleted(!deleted);
    if (!deleted) sources();
  });

  for (const c of [tag, bible, library, on]) c.addEventListener("change", () => search());
  query.addEventListener("input", () => {
    clearTimeout(timer);
    timer = window.setTimeout(() => search(), 250);
  });
  filters.addEventListener("submit", (e) => e.preventDefault());

  document.addEventListener("keydown", (e) => {
    if (e.altKey && !e.ctrlKey && e.key.toLowerCase() === "n" && el.offsetParent !== null) {
      e.preventDefault();
      edit(null);
    }
  });
  void field;

  return {
    el,
    show: async () => {
      await sources();
      if (deleted) await loadDeleted();
      else await search();
    },
  };
}
