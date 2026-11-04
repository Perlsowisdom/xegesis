// My work's notes view: every note, one folder (its notes and sermons together) or one tag, as
// cards with the title, first line, folder, tags and when it was edited. Double-click or Enter opens
// an item in the Study grid; its ⋯ menu moves it to a folder (the keyboard way to do what dragging
// onto a folder does), edits its details, shows its history or deletes it to the bin.

import { Message, Note, run } from "../api";
import { openInStudy } from "../document/open";
import { button, clear, h, select } from "../dom";
import { icon } from "../icons";
import { deleteNote, historyDrawer, noteForm, noteName, placeOf } from "../notes";
import { openPopover } from "../popover";
import { openExport } from "../printexport";
import { untitled } from "../sermonwords";
import { navigate } from "../state";
import {
  WorkItem, actionMenu, draggableItem, emptyState, folderName, loadFolders, moreButton, moveMenu, onWorkChanged, workChanged,
} from "../work";
import { Scope, editedText, firstLine, newChoices, preachedText, scopeOf } from "../workwords";
import type { SermonDoc } from "./sermons";

interface Sources {
  bible_books: { name: string; count: number }[];
  library_books: { item_id: string; title: string; count: number }[];
}

const PAGE = 60;

type Card = { kind: "note"; at: number; note: Note } | { kind: "sermon"; at: number; doc: SermonDoc };

export function notesSectionView() {
  const msg = new Message();
  const title = h("h1", { class: "work-h1" });
  const sub = h("p", { class: "hint work-sub", "aria-live": "polite" });
  const query = h("input", { type: "search", placeholder: "Search titles, text and tags", "aria-label": "Search", class: "work-search" });
  const bible = select([["", "Any book of the Bible"]], "", { "aria-label": "Book of the Bible" });
  const library = select([["", "Any library book"]], "", { "aria-label": "Library book" });
  const on = select([["", "Attached anywhere"], ["verses", "On Bible verses"], ["books", "In library books"], ["nothing", "Not attached"]], "", { "aria-label": "Attached to" });
  const filterBtn = h("button", { type: "button", class: "filter-btn", "aria-haspopup": "dialog", "aria-expanded": "false" }, icon("filter"), h("span", {}, "Filter"), h("span", { class: "filter-count" }));
  const chipsRow = h("div", { class: "chips-row active-chips" });
  const newBtn = h("button", { type: "button", class: "primary new-btn", "aria-haspopup": "menu", "aria-expanded": "false", "aria-keyshortcuts": "Alt+N" }, icon("plus"), h("span", {}, "New"));
  const list = h("ul", { class: "work-cards", "aria-label": "Notes and sermons" });
  const editor = h("div", { class: "note-editor" });
  const more = h("div", { class: "row center" });
  const drawer = historyDrawer(msg, () => void refresh());
  const el = h("div", { class: "view work-view" },
    h("div", { class: "work-head" }, h("div", { class: "work-heading" }, title, sub), newBtn),
    h("div", { class: "work-tools" }, query, filterBtn, chipsRow),
    msg.el, editor, list, more, drawer.el);

  let scope: Scope = { kind: "all" };
  let cards: Card[] = [];
  let selected = "";
  let limit = PAGE;
  let timer = 0;
  let seq = 0;

  const params = () => {
    const p: Record<string, unknown> = {};
    if (query.value.trim()) p.query = query.value.trim();
    if (bible.value) p.book = bible.value;
    if (library.value) p.item_id = library.value;
    if (on.value) p.on = on.value;
    if (scope.kind === "folder") p.folder_id = scope.id;
    if (scope.kind === "tag") p.tag = scope.tag;
    return p;
  };
  const filtered = () => !!(query.value.trim() || bible.value || library.value || on.value);

  async function refresh() {
    const n = ++seq;
    const folder = scope.kind === "folder" ? scope.id : "";
    // Sermons sit in folders beside notes; the note-only filters leave them out.
    const withSermons = folder && !bible.value && !library.value && !on.value;
    const [notes, docs] = await Promise.all([
      msg.attempt(() => run<Note[]>("note.search", { ...params(), limit: limit + 1 })),
      withSermons
        ? msg.attempt(() => run<SermonDoc[]>("document.list", { folder_id: folder, ...(query.value.trim() ? { query: query.value.trim() } : {}) }))
        : Promise.resolve([] as SermonDoc[]),
      loadFolders(),
    ]);
    if (n !== seq || !notes) return;
    cards = [
      ...notes.slice(0, limit).map((note): Card => ({ kind: "note", at: note.updated_at, note })),
      ...(docs ?? []).map((doc): Card => ({ kind: "sermon", at: doc.updated_at, doc })),
    ].sort((a, b) => b.at - a.at);
    renderHead(notes.length > limit, docs?.length ?? 0);
    renderCards();
    clear(more, notes.length > limit ? button("Show more", () => { limit += PAGE; void refresh(); }) : null);
  }

  function scopeName(): string {
    if (scope.kind === "folder") return folderName(scope.id) || "Folder";
    if (scope.kind === "tag") return scope.tag;
    return "All notes";
  }

  function renderHead(moreNotes: boolean, sermons: number) {
    clear(title, scope.kind === "all" ? "All notes" : [icon(scope.kind === "tag" ? "tag" : "folder"), h("span", {}, scopeName())]);
    const notes = cards.length - sermons;
    const parts = [`${notes}${moreNotes ? "+" : ""} ${notes === 1 ? "note" : "notes"}`];
    if (scope.kind === "folder") parts.push(`${sermons} ${sermons === 1 ? "sermon" : "sermons"}`);
    sub.textContent = `${parts.join(" and ")}${filtered() ? " found" : ""}, most recently edited first`;
    query.placeholder = scope.kind === "folder" ? `Search ${scopeName()}` : scope.kind === "tag" ? `Search notes tagged ${scope.tag}` : "Search titles, text and tags";
    const chips: [string, () => void][] = [];
    if (bible.value) chips.push([bible.value, () => { bible.value = ""; }]);
    if (library.value) chips.push([library.selectedOptions[0]?.textContent?.replace(/ \(\d+\)$/, "") ?? "", () => { library.value = ""; }]);
    if (on.value) chips.push([on.selectedOptions[0]?.textContent ?? "", () => { on.value = ""; }]);
    clear(chipsRow, chips.map(([label, off]) => h("span", { class: "chip filter-chip" }, label,
      button("×", () => { off(); void refresh(); }, { class: "chip-x", "aria-label": `Remove the filter ${label}` }))),
    chips.length > 1 ? button("Clear all", () => { bible.value = library.value = on.value = ""; void refresh(); }, { class: "link small" }) : null);
    filterBtn.querySelector(".filter-count")!.textContent = chips.length ? String(chips.length) : "";
    filterBtn.classList.toggle("on", chips.length > 0);
  }

  function renderCards() {
    if (cards.length === 0) {
      const what = scope.kind === "folder" ? "This folder is empty" : scope.kind === "tag" ? "No notes with this tag" : "No notes yet";
      clear(list, h("li", { class: "work-empty" }, filtered()
        ? emptyState("search", "Nothing matches", "Try other words, or clear the filters.")
        : emptyState(scope.kind === "folder" ? "folder" : "notes", what,
          scope.kind === "folder" ? "Make a note or a sermon here with New, or drag notes and sermons onto the folder."
            : "Select New to write one. Notes you write beside the Bible text show here too.")));
      return;
    }
    clear(list, cards.map((c) => (c.kind === "note" ? noteCard(c.note) : sermonCard(c.doc))));
  }

  // card is one item's card: focusable, selected by a click, opened by a double-click or Enter.
  function card(item: WorkItem, kindLabel: string, iconName: string, open: () => void, menu: (b: HTMLElement) => void, ...body: (Node | null)[]): HTMLElement {
    const li = h("li", {
      class: `work-card${selected === item.id ? " on" : ""}`, tabindex: "0", "data-kind": item.kind,
      "aria-label": `${kindLabel}: ${item.title}`, "aria-selected": String(selected === item.id), title: "Double-click to open in Study",
    },
    h("div", { class: "card-top" },
      h("span", { class: `kind-badge kind-${item.kind}` }, icon(iconName), kindLabel),
      moreButton(`Actions for ${item.title}`, menu)),
    ...body);
    li.addEventListener("click", () => pick(item.id, li));
    li.addEventListener("dblclick", open);
    li.addEventListener("focus", () => pick(item.id, li));
    li.addEventListener("keydown", (e) => {
      if (e.target !== li) return;
      if (e.key === "Enter") { e.preventDefault(); open(); }
      else if (e.key === "ContextMenu" || (e.shiftKey && e.key === "F10")) { e.preventDefault(); menu(li.querySelector<HTMLElement>(".more-btn")!); }
      else if (e.key === "ArrowDown" || e.key === "ArrowRight") { e.preventDefault(); (li.nextElementSibling as HTMLElement | null)?.focus(); }
      else if (e.key === "ArrowUp" || e.key === "ArrowLeft") { e.preventDefault(); (li.previousElementSibling as HTMLElement | null)?.focus(); }
    });
    return li;
  }

  function pick(id: string, li: HTMLElement) {
    selected = id;
    for (const x of list.querySelectorAll(".work-card.on")) { x.classList.remove("on"); x.setAttribute("aria-selected", "false"); }
    li.classList.add("on");
    li.setAttribute("aria-selected", "true");
  }

  function meta(...bits: (Node | null)[]): HTMLElement | null {
    const shown = bits.filter((b): b is Node => !!b);
    return shown.length ? h("div", { class: "card-meta" }, shown) : null;
  }

  function folderChip(id?: string): Node | null {
    if (!id || (scope.kind === "folder" && scope.id === id)) return null;
    return h("span", { class: "meta-chip folder-chip", title: "Folder" }, icon("folder"), folderName(id));
  }

  function noteCard(n: Note): HTMLElement {
    const item: WorkItem = { kind: "note", id: n.id, title: noteName(n), folder_id: n.folder_id };
    const open = () => openInStudy({ kind: "note", target: n.id, title: noteName(n) });
    const line = firstLine(n.body);
    const place = placeOf(n);
    const li = card(item, "Note", "notes", open, (b) => actionMenu(b, `Actions for ${noteName(n)}`, [
      ["Open in Study", open],
      ["Edit details", () => edit(n)],
      ["Move to folder…", () => moveMenu(b, msg, item)],
      ["History", () => void drawer.open(n)],
      ["Print or save…", () => openExport({ title: noteName(n), sources: [{ kind: "note", id: n.id }] })],
      "-",
      ["Delete", () => void deleteNote(msg, n, () => workChanged()), "danger"],
    ]),
    h("h3", { class: "card-title" }, n.title || h("span", { class: "untitled" }, "Untitled")),
    h("p", { class: `card-line${line ? "" : " none"}` }, line || "No text yet"),
    meta(place ? h("span", { class: "meta-chip place-chip" }, place) : null, folderChip(n.folder_id),
      ...n.tags.map((t) => h("span", { class: "meta-chip tag-chip" }, `#${t}`))),
    h("p", { class: "card-foot" }, editedText(n.updated_at)));
    draggableItem(li, () => item, () => ({ kind: "note", target: n.id, title: noteName(n) }));
    return li;
  }

  function sermonCard(d: SermonDoc): HTMLElement {
    const name = d.title || untitled(d.kind);
    const item: WorkItem = { kind: "sermon", id: d.id, title: name, folder_id: d.folder_id };
    const open = () => openInStudy({ kind: "sermon", target: d.id, title: name });
    const kind = { sermon: "Sermon", lesson: "Lesson", study: "Bible study" }[d.kind] ?? "Sermon";
    const points = d.points.filter((p) => p.level !== 2).map((p) => p.h).filter(Boolean).join(" · ");
    const line = points || firstLine(d.body);
    const li = card(item, kind, "sermon", open, (b) => actionMenu(b, `Actions for ${name}`, [
      ["Open in Study", open],
      ["Edit outline and text", () => navigate(`sermons/${d.id}`)],
      ["Move to folder…", () => moveMenu(b, msg, item)],
      ["Print or save…", () => openExport({ title: name, sources: [{ kind: "document", id: d.id }] })],
      "-",
      ["Delete", () => void deleteSermon(msg, d), "danger"],
    ]),
    h("h3", { class: "card-title" }, name),
    h("p", { class: `card-line${line ? "" : " none"}` }, line || "No outline or text yet"),
    meta(d.passage ? h("span", { class: "meta-chip place-chip" }, d.passage.display) : null,
      d.preached_on ? h("span", { class: "meta-chip" }, `Preached ${preachedText(d.preached_on)}`) : null,
      d.theme ? h("span", { class: "meta-chip theme-chip" }, d.theme) : null, folderChip(d.folder_id)),
    h("p", { class: "card-foot" }, editedText(d.updated_at)));
    draggableItem(li, () => item, () => ({ kind: "sermon", target: d.id, title: name }));
    return li;
  }

  // Editing and making things -----------------------------------------------------------------------

  function edit(n: Note | null) {
    clear(editor, noteForm(msg, {
      note: n,
      folderId: !n && scope.kind === "folder" ? scope.id : undefined,
      tags: !n && scope.kind === "tag" ? [scope.tag] : undefined,
      onDone: (saved) => {
        clear(editor);
        if (!saved) return;
        msg.ok(n ? "Note saved." : "Note made. Double-click it to open it in Study.");
        selected = saved.id;
        workChanged();
      },
    }));
    editor.scrollIntoView({ block: "nearest" });
  }

  async function newSermon() {
    const folder = scope.kind === "folder" ? scope.id : undefined;
    const d = await msg.attempt(() => run<SermonDoc>("document.create", { kind: "sermon", ...(folder ? { folder_id: folder } : {}) }));
    if (!d) return;
    workChanged();
    navigate(`sermons/${d.id}`);
  }

  newBtn.addEventListener("click", () => {
    const choices = newChoices("notes", scope.kind === "folder" ? scopeName() : undefined);
    const body = h("div", { class: "menu-list" }, choices.map((c) =>
      button("", () => (c.kind === "note" ? edit(null) : void newSermon()), {})));
    [...body.querySelectorAll("button")].forEach((b, i) => b.append(icon(choices[i].kind === "note" ? "notes" : "sermon"), h("span", {}, choices[i].label)));
    openPopover(newBtn, body, { label: "New", menu: true });
  });

  filterBtn.addEventListener("click", () => {
    const pop = h("div", { class: "pop-body note-filters" },
      h("p", { class: "pop-title" }, "Filter notes"),
      h("label", { class: "field" }, h("span", { class: "field-label" }, "Book of the Bible"), bible),
      h("label", { class: "field" }, h("span", { class: "field-label" }, "Library book"), library),
      h("label", { class: "field" }, h("span", { class: "field-label" }, "Attached to"), on));
    openPopover(filterBtn, pop, { label: "Filter notes", className: "filter-pop" });
  });

  async function loadSources() {
    const s = await msg.attempt(() => run<Sources>("note.sources"));
    if (!s) return;
    fill(bible, [["", "Any book of the Bible"], ...s.bible_books.map((b): [string, string] => [b.name, `${b.name} (${b.count})`])]);
    fill(library, [["", "Any library book"], ...s.library_books.map((b): [string, string] => [b.item_id, `${b.title} (${b.count})`])]);
  }

  function fill(sel: HTMLSelectElement, options: [string, string][]) {
    const keep = sel.value;
    clear(sel, options.map(([v, l]) => h("option", { value: v }, l)));
    sel.value = options.some(([v]) => v === keep) ? keep : "";
  }

  for (const c of [bible, library, on]) c.addEventListener("change", () => void refresh());
  query.addEventListener("input", () => {
    clearTimeout(timer);
    timer = window.setTimeout(() => void refresh(), 220);
  });
  query.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && query.value) { e.preventDefault(); query.value = ""; void refresh(); }
  });
  document.addEventListener("keydown", (e) => {
    if (e.altKey && !e.ctrlKey && e.key.toLowerCase() === "n" && el.offsetParent !== null) {
      e.preventDefault();
      edit(null);
    }
  });
  onWorkChanged(() => { if (el.offsetParent !== null) void refresh(); });

  return {
    el,
    focus: () => query.focus(),
    show: async () => {
      const next = scopeOf(location.hash);
      if (JSON.stringify(next) !== JSON.stringify(scope)) {
        scope = next;
        limit = PAGE;
        query.value = "";
        clear(editor);
      }
      await Promise.all([loadSources(), refresh()]);
    },
  };
}

// deleteSermon puts a sermon in the bin and offers Undo.
export async function deleteSermon(msg: Message, d: SermonDoc): Promise<void> {
  if (await msg.attempt(() => run("document.delete", { id: d.id })) === undefined) return;
  clear(msg.el, `"${d.title}" went to the bin. `, button("Undo", async () => {
    if (await msg.attempt(() => run("document.restore", { id: d.id })) !== undefined) {
      msg.ok("Restored.");
      workChanged();
    }
  }, { class: "link" }));
  msg.el.className = "msg ok";
  msg.el.setAttribute("role", "status");
  workChanged();
}
