// Library: adding books in one step (files, folders or drag and drop), the folders being watched,
// and the catalogue. The catalogue has one Filter button whose popover holds every facet (any
// choice within a facet, every facet across them), the active filters as removable chips with
// Clear all, a count, covers in a grid or rows in a list, and a detail pane with Read, Open in Study
// and the licence folded away. Every action is a library.* or facet.* command.

import { AddResult, CollectionNode, Facet, Folder, Item, ItemSummary, Message, Vocabulary, pick, pickFiles, run } from "../api";
import { chip, segmented } from "../controls";
import { badge, button, clear, field, h } from "../dom";
import { changedFields, formatTime } from "../helpers";
import { icon } from "../icons";
import { collectionsSidebar } from "../collections";
import { facetChooser, traditionsPanel } from "../facets";
import { countText, facetValue, reasonText } from "../facetwords";
import { licenceChooser, licenceLine, originText, plainName, termsText } from "../licence";
import { FACETS, FacetId, Filters, activeCount, applyFilters, chips, countLine, coverHue, initials, options, toggle } from "../librarywords";
import { draggableSpec } from "../document/drag";
import { openInStudy, readBook } from "../document/open";
import { closePopover, openPopover } from "../popover";
import { loadPref, savePref } from "../state";

const FETCH = 1000;
const SHOW = 120;
const VIEW_KEY = "xegesis.library.view";
const READABLE = new Set(["epub", "pdf", "docx", "html", "md", "txt"]);

interface ScanResult {
  added: number; changed: number; removed: number; moved: number; restored: number; unchanged: number;
  errors: { rel_path: string; error: string }[];
}

export function libraryView() {
  let folders: Folder[] = [];
  const addMsg = new Message();
  const folderMsg = new Message();
  const folderList = h("div", { class: "table-wrap" });
  let side!: ReturnType<typeof collectionsSidebar>;
  const items = catalogue(() => folders, () => side);
  side = collectionsSidebar({
    folders: () => folders, vocab: () => items.vocab(), picked: (n) => items.filterCollection(n),
    changed: () => items.refresh(), say: (t) => items.say(t),
  });
  items.slot.append(side.bar);
  const refreshAll = () => { void items.refresh(); void side.reload(); };

  // Adding books: one button, two ways.
  const menu = h("div", { class: "menu", role: "menu", hidden: true },
    button("Choose files…", () => { closeMenu(); void chooseFiles(); }, { role: "menuitem" }),
    button("Choose a folder…", () => { closeMenu(); void chooseFolder(); }, { role: "menuitem" }));
  const addButton = h("button", { type: "button", class: "primary new-btn", "aria-haspopup": "menu", "aria-expanded": "false" }, icon("plus"), h("span", {}, "Add books"));
  addButton.addEventListener("click", () => (menu.hidden ? openMenu() : closeMenu()));
  const menuWrap = h("div", { class: "menu-wrap" }, addButton, menu);
  function openMenu() {
    menu.hidden = false;
    addButton.setAttribute("aria-expanded", "true");
    menu.querySelector("button")?.focus();
  }
  function closeMenu() {
    menu.hidden = true;
    addButton.setAttribute("aria-expanded", "false");
  }
  menu.addEventListener("keydown", (e) => {
    if (e.key === "Escape") { closeMenu(); addButton.focus(); }
  });
  document.addEventListener("click", (e) => {
    if (!menu.hidden && !menuWrap.contains(e.target as Node)) closeMenu();
  });

  async function chooseFiles() {
    const files = await addMsg.attempt(() => pickFiles("Choose books to add"));
    if (files?.length) void add(files);
  }
  async function chooseFolder() {
    const dir = await addMsg.attempt(() => pick("folder", "Choose a folder of books"));
    if (dir) void add([dir]);
  }

  // add adds files and folders; a dropped mix goes through here too.
  async function add(paths: string[]) {
    addMsg.info(paths.length === 1 ? "Adding books… a large folder can take a while." : `Adding ${paths.length} items…`);
    const r = await addMsg.attempt(() => run<AddResult>("library.add", { paths }));
    if (!r) return;
    clear(addMsg.el, r.summary,
      r.added ? [" ", button("Review", () => items.review(r.since), { class: "link" })] : null,
      r.errors.length ? h("details", {}, h("summary", {}, "Why some files were not added"),
        h("ul", {}, r.errors.map((e) => h("li", {}, h("code", {}, e.rel_path), ` ${e.error}`)))) : null);
    addMsg.el.className = "msg ok";
    addMsg.el.setAttribute("role", "status");
    void showFolders();
    refreshAll();
  }

  const foldersBox = h("details", { class: "lib-folders" },
    h("summary", {}, h("span", { class: "lib-folders-title" }, "Folders on this computer"), h("span", { class: "hint" }, " · where your books are kept")),
    h("div", { class: "row between lib-folders-head" },
      h("p", { class: "hint" }, "Files you choose are copied into Documents/Xegesis Library. Folders stay where they are and are checked for new books every few minutes."),
      button("Look for moved drives", async () => {
        if (await folderMsg.attempt(() => run("library.locate"))) {
          folderMsg.ok("Checked connected drives.");
          void showFolders();
        }
      })),
    folderMsg.el, folderList);

  const el = h("div", { class: "view library-view" },
    h("div", { class: "work-head" },
      h("div", { class: "work-heading" }, h("h1", { class: "work-h1" }, "Library"),
        h("p", { class: "hint work-sub" }, "EPUB, PDF, Word, HTML, Markdown and text files. You can also drop them on the window.")),
      menuWrap),
    addMsg.el,
    h("div", { class: "lib-body" }, side.el, items.el),
    foldersBox);

  async function showFolders() {
    const fs = await folderMsg.attempt(() => run<Folder[]>("library.folders"));
    if (!fs) return;
    folders = fs;
    clear(folderList, fs.length === 0 ? h("p", { class: "empty" }, "No folders yet. Choose Add books to start.") :
      h("table", { class: "list" },
        h("thead", {}, h("tr", {}, ["Name", "Where", "Status", ""].map((t) => h("th", { scope: "col" }, t)))),
        h("tbody", {}, fs.map((f) => h("tr", {},
          h("td", {}, h("strong", {}, f.label || "Untitled")),
          h("td", { class: "path" }, f.path),
          h("td", {}, f.online ? badge("Online", "ok") : badge("Offline", "warn"),
            !f.online && f.last_seen_at ? h("small", { class: "hint" }, ` last seen ${formatTime(f.last_seen_at)}`) : null),
          h("td", { class: "actions" },
            button("Check now", () => void scan(f), { disabled: !f.online, "aria-label": `Check ${f.label || f.path} for new books` }),
            button("Books", () => items.filterFolder(f), { "aria-label": `Show books in ${f.label || f.path}` }),
            button("Remove", () => void removeFolder(f), { class: "danger", "aria-label": `Remove ${f.label || f.path}` })))))));
  }

  async function scan(f: Folder) {
    addMsg.clear();
    folderMsg.info(`Checking ${f.label || f.path}…`);
    const r = await folderMsg.attempt(() => run<ScanResult>("library.scan", { folder_id: f.id }));
    if (!r) return;
    const news = [[r.added, "added"], [r.changed, "changed"], [r.moved, "moved"], [r.restored, "back"], [r.removed, "gone"]]
      .filter(([n]) => Number(n) > 0).map(([n, w]) => `${n} ${w}`);
    clear(folderMsg.el,
      `Checked ${f.label || f.path}: ${news.length ? news.join(", ") : "no changes"}.`,
      r.errors.length ? h("details", {}, h("summary", {}, `${r.errors.length} files could not be read`),
        h("ul", {}, r.errors.map((e) => h("li", {}, h("code", {}, e.rel_path), ` ${e.error}`)))) : null);
    folderMsg.el.className = "msg ok";
    refreshAll();
  }

  async function removeFolder(f: Folder) {
    if (!confirm(`Remove ${f.label || f.path} and its books from the library? Files on disk are left alone.`)) return;
    const r = await folderMsg.attempt(() => run<{ items: number; files: number }>("library.remove_folder", { id: f.id }));
    if (!r) return;
    folderMsg.ok(`Removed ${f.label || f.path} (${r.items} books).`);
    void showFolders();
    refreshAll();
  }

  async function show() {
    await showFolders();
    refreshAll();
  }
  return { el, show, drop: add, changed: () => { void showFolders(); refreshAll(); } };
}

// catalogue is the filtered list of books in a grid of covers or a list, with a detail pane.
function catalogue(getFolders: () => Folder[], getSide: () => ReturnType<typeof collectionsSidebar>) {
  const msg = new Message();
  const text = h("input", { type: "search", class: "work-search", placeholder: "Find a title or author", "aria-label": "Find a title or author", autocomplete: "off" });
  const filterBtn = h("button", { type: "button", class: "filter-btn", "aria-haspopup": "dialog", "aria-expanded": "false" },
    icon("filter"), h("span", {}, "Filter"), h("span", { class: "filter-count" }));
  const chipsRow = h("div", { class: "chips-row active-chips", "aria-label": "Active filters" });
  const count = h("span", { class: "lib-count", "aria-live": "polite" });
  let mode: "grid" | "list" = loadPref<string>(VIEW_KEY, "grid") === "list" ? "list" : "grid";
  const modeSwitch = segmented<"grid" | "list">({ label: "Show books as", small: true, value: mode, options: [["grid", "Covers"], ["list", "List"]],
    onChange: (v) => { mode = v; savePref(VIEW_KEY, v); render(); } });
  [...modeSwitch.querySelectorAll(".seg-opt > span")].forEach((s, i) => s.prepend(icon(i === 0 ? "covers" : "rows")));
  let vocab: Vocabulary = { types: [], traditions: [] };
  const results = h("div", { class: "lib-results", "aria-live": "polite" });
  const more = h("div", { class: "row center" });
  const detail = h("aside", { class: "lib-detail", "aria-label": "Book details", hidden: true });
  const scopeRow = h("div", { class: "lib-scope", hidden: true });
  const selection = h("div", { class: "selbar lib-selbar", hidden: true });
  const bulk = h("div", { class: "card", hidden: true });
  const slot = h("div", {});
  const rules = traditionsPanel(() => { void loadVocab().then(changedBooks); });
  rules.el.append(h("p", {}, button("Look again at books without a type", () => void suggestAll())));

  let all: ItemSummary[] = [];
  let filters: Filters = {};
  let shownMax = SHOW;
  let since = 0;
  let folder: Folder | null = null;
  let collection: CollectionNode | null = null;
  let selectedId = "";
  const ticked = new Set<string>();
  let seq = 0;

  const el = h("section", { class: "lib-catalogue", "aria-label": "Books" },
    slot,
    h("div", { class: "work-tools lib-tools" }, text, filterBtn, chipsRow, h("div", { class: "lib-right" }, count, modeSwitch)),
    scopeRow, msg.el, selection, bulk,
    h("div", { class: "lib-panes" }, h("div", { class: "lib-main" }, results, more), detail),
    rules.el);

  // Data ------------------------------------------------------------------------------------------

  async function loadVocab() {
    const v = await msg.attempt(() => run<Vocabulary>("facet.vocabulary"));
    if (!v) return;
    vocab = v;
    rules.reload();
  }

  // refresh loads every book in the collection, folder or new arrivals being shown; the facets then
  // filter them here, so counts and combinations answer at once.
  async function refresh() {
    const n = ++seq;
    if (!vocab.types.length) await loadVocab();
    const base: Record<string, unknown> = { limit: FETCH };
    if (collection) base.collection_id = collection.id;
    if (folder) base.folder_id = folder.id;
    if (since) base.added_since = since;
    const got: ItemSummary[] = [];
    for (let offset = 0; ; offset += FETCH) {
      const r = await msg.attempt(() => run<{ items: ItemSummary[]; total: number }>("library.items", { ...base, offset }));
      if (!r || n !== seq) return;
      got.push(...r.items);
      if (got.length >= r.total || r.items.length === 0) break;
    }
    all = got;
    for (const id of [...ticked]) if (!all.some((b) => b.id === id)) ticked.delete(id);
    render();
    if (selectedId && !detail.hidden && all.some((b) => b.id === selectedId)) void openDetail(selectedId);
  }

  function shown(): ItemSummary[] {
    return applyFilters(all, filters, text.value);
  }

  // Drawing ---------------------------------------------------------------------------------------

  function render() {
    const list = shown();
    count.textContent = countLine(list.length, all.length);
    if (selectedId && !list.some((b) => b.id === selectedId)) {
      selectedId = "";
      detail.hidden = true;
    }
    renderChips();
    renderScope();
    results.dataset.mode = mode;
    if (list.length === 0) {
      clear(results, all.length === 0
        ? emptyLib(since || folder || collection ? "No books here" : "No books yet", since || folder || collection ? "Nothing in this part of your library." : "Choose Add books, or drop files on the window.")
        : emptyLib("No books match", "Try fewer filters, or other words.", button("Clear all filters", () => clearAll(), { class: "primary" })));
      clear(more);
      updateSelection();
      return;
    }
    const page = list.slice(0, shownMax);
    clear(results, mode === "grid" ? grid(page) : table(page));
    clear(more, list.length > shownMax ? button(`Show ${Math.min(SHOW, list.length - shownMax)} more`, () => { shownMax += SHOW; render(); }) : null);
    updateSelection();
  }

  function emptyLib(title: string, hint: string, action?: HTMLElement): HTMLElement {
    return h("div", { class: "empty-state" }, h("span", { class: "empty-icon" }, icon("library")),
      h("p", { class: "empty-title" }, title), h("p", { class: "hint" }, hint), action ?? null);
  }

  function grid(page: ItemSummary[]): HTMLElement {
    return h("ul", { class: "cover-grid", "aria-label": "Books" }, page.map((b) => {
      const tile = h("li", {
        class: `book-tile${b.id === selectedId ? " on" : ""}`, tabindex: "0", "aria-selected": String(b.id === selectedId),
        "aria-label": `${b.title || "Untitled"}${b.authors.length ? ", " + b.authors.join(", ") : ""}`, title: "Double-click to read",
      }, cover(b), h("div", { class: "tile-caption" },
        h("span", { class: "tile-title" }, b.title || "Untitled"),
        h("span", { class: "tile-author" }, b.authors.join(", ") || " "),
        b.status !== "online" ? statusBadge(b.status) : null));
      tile.addEventListener("click", () => select(b.id, tile));
      tile.addEventListener("dblclick", () => void read(b));
      tile.addEventListener("keydown", (e) => {
        if (e.target !== tile) return;
        if (e.key === "Enter") { e.preventDefault(); void read(b); }
        else if (e.key === " ") { e.preventDefault(); select(b.id, tile); }
        else if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
          e.preventDefault();
          ((e.key === "ArrowRight" ? tile.nextElementSibling : tile.previousElementSibling) as HTMLElement | null)?.focus();
        }
      });
      draggableSpec(tile, () => ({ kind: "book", itemId: b.id, title: b.title || "Untitled" }));
      return tile;
    }));
  }

  function table(page: ItemSummary[]): HTMLElement {
    const allBox = h("input", { type: "checkbox", class: "tick-input", "aria-label": "Select every book shown", checked: page.length > 0 && page.every((b) => ticked.has(b.id)) });
    allBox.addEventListener("change", () => {
      for (const b of page) allBox.checked ? ticked.add(b.id) : ticked.delete(b.id);
      render();
    });
    return h("div", { class: "table-wrap lib-table-wrap" }, h("table", { class: "list lib-table" },
      h("thead", {}, h("tr", {}, h("th", { scope: "col", class: "tick-col" }, allBox),
        ["Title", "Authors", "Year", "Type", "Tradition", "Licence"].map((t, i) => h("th", { scope: "col", class: i >= 4 ? "opt-col" : null }, t)))),
      h("tbody", {}, page.map((b) => {
        const box = h("input", { type: "checkbox", class: "tick-input", value: b.id, checked: ticked.has(b.id), "aria-label": `Select ${b.title || "Untitled"}` });
        box.addEventListener("change", () => { box.checked ? ticked.add(b.id) : ticked.delete(b.id); updateSelection(); });
        box.addEventListener("click", (e) => e.stopPropagation());
        const tr = h("tr", { tabindex: "0", class: b.id === selectedId ? "on" : "", "aria-selected": String(b.id === selectedId), title: "Double-click to read" },
          h("td", { class: "tick-col" }, box),
          h("td", { class: "lib-title-cell" }, h("span", { class: "mini-cover", style: `--hue:${coverHue(b)}`, "aria-hidden": "true" }),
            h("span", {}, h("strong", {}, b.title || "Untitled"), b.subtitle ? h("small", { class: "hint" }, b.subtitle) : null,
              b.status !== "online" ? statusBadge(b.status) : null)),
          h("td", {}, b.authors.join(", ")),
          h("td", { class: "num" }, b.year || ""),
          h("td", {}, facetValue(b.type)),
          h("td", { class: "opt-col" }, facetValue(b.tradition)),
          h("td", { class: "opt-col" }, b.licence_name || plainName(b.licence)));
        tr.addEventListener("click", () => select(b.id, tr));
        tr.addEventListener("dblclick", () => void read(b));
        tr.addEventListener("keydown", (e) => {
          if (e.target !== tr) return;
          if (e.key === "Enter") { e.preventDefault(); void read(b); }
          else if (e.key === " ") { e.preventDefault(); select(b.id, tr); }
          else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            ((e.key === "ArrowDown" ? tr.nextElementSibling : tr.previousElementSibling) as HTMLElement | null)?.focus();
          }
        });
        draggableSpec(tr, () => ({ kind: "book", itemId: b.id, title: b.title || "Untitled" }));
        return tr;
      }))));
  }

  function select(id: string, node: HTMLElement) {
    selectedId = id;
    for (const x of results.querySelectorAll(".on")) { x.classList.remove("on"); x.setAttribute("aria-selected", "false"); }
    node.classList.add("on");
    node.setAttribute("aria-selected", "true");
    void openDetail(id);
  }

  // Filters ---------------------------------------------------------------------------------------

  function renderChips() {
    const cs = chips(filters);
    clear(chipsRow, cs.map((c) => h("span", { class: "chip filter-chip" }, c.label,
      button("×", () => setFilters(toggle(filters, c.facet, c.value, false)), { class: "chip-x", "aria-label": `Remove the filter ${c.label}` }))),
    cs.length ? button("Clear all", () => clearAll(), { class: "link small" }) : null);
    const n = activeCount(filters);
    filterBtn.querySelector(".filter-count")!.textContent = n ? String(n) : "";
    filterBtn.classList.toggle("on", n > 0);
    filterBtn.setAttribute("aria-label", n ? `Filter, ${n} chosen` : "Filter");
  }

  function setFilters(f: Filters) {
    filters = f;
    shownMax = SHOW;
    render();
    if (pop?.isConnected) drawPopover();
  }

  function clearAll() {
    text.value = "";
    setFilters({});
  }

  let pop: HTMLElement | null = null;
  const popBody = h("div", { class: "pop-body lib-filter-pop" });
  const authorFind = h("input", { type: "search", placeholder: "Find an author", "aria-label": "Find an author", class: "author-find" });
  authorFind.addEventListener("input", () => drawPopover());

  function drawPopover() {
    const base = applyFilters(all, {}, text.value);
    const focused = document.activeElement as HTMLInputElement | null;
    const keep = focused?.closest(".lib-filter-pop") ? focused.dataset.key : undefined;
    clear(popBody,
      h("div", { class: "row between pop-head" }, h("p", { class: "pop-title" }, "Filter books"),
        activeCount(filters) ? button("Clear all", () => setFilters({}), { class: "link small" }) : null),
      h("p", { class: "hint pop-note" }, "Any choice within a group; every group you use."),
      FACETS.map(({ id, label }) => facetGroup(id, label, base)),
      h("div", { class: "row pop-foot" }, h("span", { class: "hint" }, countLine(shown().length, all.length)),
        button("Done", () => closePopover(true), { class: "primary" })));
    if (keep) popBody.querySelector<HTMLElement>(`[data-key="${CSS.escape(keep)}"]`)?.focus();
  }

  function facetGroup(id: FacetId, label: string, base: ItemSummary[]): HTMLElement | null {
    const order = id === "type" ? vocab.types : id === "tradition" ? vocab.traditions : [];
    let opts = options(base, id, filters, order);
    if (opts.length === 0) return null;
    const many = id === "author" && opts.length > 12;
    const q = authorFind.value.trim().toLowerCase();
    if (many && q) opts = opts.filter((o) => o.value.toLowerCase().includes(q) || filters.author?.includes(o.value));
    const chosen = new Set(filters[id] ?? []);
    return h("fieldset", { class: "facet-group" }, h("legend", {}, label, chosen.size ? h("span", { class: "facet-chosen" }, ` · ${chosen.size} chosen`) : null),
      many ? authorFind : null,
      h("div", { class: "chips-row" }, opts.map((o) => {
        const c = chip({ label: o.value, checked: chosen.has(o.value), onChange: (on) => setFilters(toggle(filters, id, o.value, on)) });
        const input = c.querySelector("input")!;
        input.dataset.key = `${id}:${o.value}`;
        c.append(h("span", { class: "chip-count" }, String(o.count)));
        if (o.count === 0 && !chosen.has(o.value)) c.classList.add("zero");
        return c;
      })));
  }

  filterBtn.addEventListener("click", () => {
    if (pop?.isConnected) { closePopover(); return; }
    drawPopover();
    pop = openPopover(filterBtn, popBody, { label: "Filter books", className: "lib-filter", onClose: () => { pop = null; } });
  });

  let typing = 0;
  text.addEventListener("input", () => {
    clearTimeout(typing);
    typing = window.setTimeout(() => { shownMax = SHOW; render(); }, 120);
  });
  text.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && text.value) { e.preventDefault(); text.value = ""; render(); }
  });

  // The part of the library shown: a collection, a library folder or the books just added.
  function renderScope() {
    const parts: HTMLElement[] = [];
    if (since) parts.push(h("span", { class: "scope-chip" }, "Just added", button("×", () => { since = 0; void refresh(); }, { class: "chip-x", "aria-label": "Show all books" })));
    if (folder) {
      const f = folder;
      parts.push(h("span", { class: "scope-chip" }, icon("folder"), `In ${f.label || f.path}`, button("×", () => { folder = null; void refresh(); }, { class: "chip-x", "aria-label": "Show books in every folder" })));
    }
    scopeRow.hidden = parts.length === 0;
    clear(scopeRow, parts);
  }

  // Selection and bulk changes (list view) --------------------------------------------------------

  function updateSelection() {
    const ids = [...ticked];
    selection.hidden = ids.length === 0 || mode !== "list";
    clear(selection, h("span", { class: "sel-label" }, `${ids.length} selected`),
      button("Set licence", () => openBulk(), { class: "primary" }),
      button("Set type or tradition", () => openBulkFacets()),
      button("Add to collection", () => openAddTo()),
      collection?.kind === "manual" ? button("Take out of this collection", () => void getSide().removeFrom(ids)) : null,
      button("Clear selection", () => { ticked.clear(); render(); }, { class: "link small" }));
    if (!ids.length) bulk.hidden = true;
  }

  function openBulk() {
    const ids = [...ticked];
    const chooser = licenceChooser(`Licence for ${countText(ids.length)}`);
    const bmsg = new Message();
    clear(bulk, chooser.el, h("div", { class: "row" },
      button("Set licence", async () => {
        const r = await bmsg.attempt(() => run<{ updated: number }>("library.set_licence", { item_ids: ids, ...chooser.value() }));
        if (!r) return;
        bulk.hidden = true;
        msg.ok(`Licence set for ${countText(r.updated)}.`);
        void refresh();
      }, { class: "primary" }),
      button("Cancel", () => { bulk.hidden = true; })), bmsg.el);
    bulk.hidden = false;
    bulk.querySelector<HTMLInputElement>("input")?.focus();
  }

  function openAddTo() {
    clear(bulk, getSide().addTo([...ticked], () => { bulk.hidden = true; }));
    bulk.hidden = false;
    bulk.querySelector<HTMLElement>("select, button")?.focus();
  }

  function openBulkFacets() {
    const ids = [...ticked];
    const chooser = facetChooser(vocab, true);
    const bmsg = new Message();
    clear(bulk, h("fieldset", {}, h("legend", {}, `Type and tradition for ${countText(ids.length)}`), chooser.el),
      h("div", { class: "row" },
        button("Set", async () => {
          const p = chooser.value(ids);
          if (!p) return bmsg.info("Choose a type or a tradition first.");
          const r = await bmsg.attempt(() => run<{ updated: number }>("facet.set", p));
          if (!r) return;
          bulk.hidden = true;
          msg.ok(`Changed ${countText(r.updated)}.`);
          void loadVocab().then(changedBooks);
        }, { class: "primary" }),
        button("Cancel", () => { bulk.hidden = true; })), bmsg.el);
    bulk.hidden = false;
    bulk.querySelector<HTMLElement>("select")?.focus();
  }

  async function suggestAll() {
    const r = await msg.attempt(() => run<{ typed: number; traditions: number; unsure: number }>("facet.suggest", { redo: true }));
    if (!r) return;
    msg.ok(`Set ${countText(r.typed)} automatically (${r.unsure} had nothing to go on). Anything you chose yourself was left alone.`);
    changedBooks();
  }

  function changedBooks() {
    void refresh();
    void getSide().reload();
  }

  // The detail pane -------------------------------------------------------------------------------

  async function openDetail(id: string, editLicence = false, editFacets = false) {
    const it = await msg.attempt(() => run<Item>("library.item", { id }));
    if (!it || id !== selectedId) return;
    detail.hidden = false;
    const summary = all.find((b) => b.id === id);
    clear(detail, itemDetail(it, summary, getFolders(), vocab, editLicence, editFacets, close, () => { changedBooks(); }));
  }

  function close() {
    detail.hidden = true;
    const on = results.querySelector<HTMLElement>(".on");
    selectedId = "";
    on?.classList.remove("on");
    on?.focus();
  }

  async function read(b: ItemSummary) {
    const it = await msg.attempt(() => run<Item>("library.item", { id: b.id }));
    const f = it?.files?.find((x) => x.online && READABLE.has(x.format));
    if (!it || !f) return msg.info(`"${b.title || "This book"}" has no file that can be read here right now.`);
    readBook({ itemId: it.id, fileId: f.id });
  }

  return {
    el, refresh, slot, vocab: () => vocab, say: (t: string) => msg.ok(t),
    filterCollection(n: CollectionNode | null) {
      collection = n;
      since = 0;
      shownMax = SHOW;
      void refresh();
    },
    filterFolder(f: Folder) {
      folder = f;
      since = 0;
      shownMax = SHOW;
      void refresh();
      el.scrollIntoView({ block: "start" });
    },
    // review lists only the books added since t.
    review(t: number) {
      since = t;
      folder = null;
      collection = null;
      getSide().reset();
      text.value = "";
      filters = {};
      void refresh();
      el.scrollIntoView({ block: "start" });
    },
  };
}

function statusBadge(s: string) {
  return s === "online" ? badge("Online", "ok") : s === "offline" ? badge("Offline", "warn") : badge("Missing", "bad");
}

// cover draws a book's cover from its type and title: there are no cover images in the library, so
// each book gets its own colour, title and author in a clear layout.
function cover(b: Pick<ItemSummary, "title" | "type" | "authors" | "year">, big = false): HTMLElement {
  return h("div", { class: `book-cover${big ? " big" : ""}`, style: `--hue:${coverHue({ title: b.title || "Untitled", type: b.type })}`, "aria-hidden": "true" },
    h("span", { class: "cover-type" }, b.type ? b.type.replace(/ \(.*\)$/, "") : ""),
    h("span", { class: "cover-title" }, b.title || "Untitled"),
    h("span", { class: "cover-rule" }),
    h("span", { class: "cover-author" }, b.authors.join(", ")),
    h("span", { class: "cover-mark" }, initials(b.title || "?")));
}

const EDITABLE = ["title", "subtitle", "kind", "year", "language", "publisher", "series", "series_no", "isbn"] as const;
const LABELS: Record<string, string> = {
  title: "Title", subtitle: "Subtitle", kind: "Kind", year: "Year", language: "Language", publisher: "Publisher",
  series: "Series", series_no: "Number in series", isbn: "ISBN",
};

function itemDetail(it: Item, summary: ItemSummary | undefined, folders: Folder[], vocab: Vocabulary, editLicence: boolean, editFacets: boolean,
  close: () => void, saved: () => void) {
  const msg = new Message();
  const inputs: Record<string, HTMLInputElement> = {};
  for (const k of EDITABLE) {
    inputs[k] = h("input", { type: k === "year" ? "number" : "text", value: (it[k] as string | number) || "", autocomplete: "off" });
  }
  const authors = it.people.filter((p) => !p.role || p.role === "author").map((p) => p.name);
  const authorsBox = h("textarea", { rows: 2 }, authors.join("\n"));
  const removed: Facet[] = [];
  const facetList = h("ul", { class: "facets" });
  const renderFacets = () => clear(facetList, it.facets.filter((f) => !removed.includes(f)).map((f) =>
    h("li", {}, badge(`${f.facet}: ${f.value}`), button("×", () => { removed.push(f); renderFacets(); }, { "aria-label": `Remove ${f.facet} ${f.value}`, class: "icon" }))));
  renderFacets();
  const newFacet = h("input", { type: "text", placeholder: "subject", autocomplete: "off" });
  const newValue = h("input", { type: "text", placeholder: "e.g. Christology", autocomplete: "off" });

  const refreshed = async () => {
    const fresh = await msg.attempt(() => run<Item>("library.item", { id: it.id }));
    if (fresh) saved();
  };

  const chooser = licenceChooser("Change licence");
  chooser.set(it.licence);
  const licenceEditor = h("div", { hidden: !editLicence, class: "licence-edit" }, chooser.el,
    h("div", { class: "row" },
      button("Save licence", async () => {
        if (await msg.attempt(() => run("library.set_licence", { item_ids: [it.id], ...chooser.value() })) === undefined) return;
        await refreshed();
      }, { class: "primary" }),
      button("Cancel", () => { licenceEditor.hidden = true; })));

  const facetOf = (name: string) => it.facets.find((f) => f.facet === name);
  const typeNow = facetOf("type"), tradNow = facetOf("tradition");
  const by = (f?: Facet) => (f ? (f.origin === "user" ? "user" : "auto") : undefined);
  const facetChoice = facetChooser(vocab, false, { type: typeNow?.value, typeBy: by(typeNow), tradition: tradNow?.value, traditionBy: by(tradNow) });
  const facetEditor = h("div", { class: "facet-edit", hidden: !editFacets }, facetChoice.el,
    h("div", { class: "row" },
      button("Save", async () => {
        const p = facetChoice.value([it.id]);
        if (!p) { facetEditor.hidden = true; return; }
        if (await msg.attempt(() => run("facet.set", p)) === undefined) return;
        await refreshed();
      }, { class: "primary" }),
      button("Cancel", () => { facetEditor.hidden = true; })));
  const changeFacets = button("Change", () => { facetEditor.hidden = false; facetEditor.querySelector<HTMLElement>("select")?.focus(); },
    { class: "link small", "aria-label": "Change the type and tradition of this book" });

  const form = h("form", { "aria-label": "Edit book details" },
    h("div", { class: "grid2" }, EDITABLE.map((k) => field(LABELS[k], inputs[k]))),
    field("Authors", authorsBox, "One per line"),
    h("h4", {}, "Facets"), facetList,
    h("div", { class: "row wrap end" }, field("Facet", newFacet), field("Value", newValue)),
    h("div", { class: "row" }, h("button", { type: "submit", class: "primary" }, "Save details")));
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const before: Record<string, unknown> = {};
    const next: Record<string, unknown> = {};
    for (const k of EDITABLE) {
      before[k] = k === "year" ? it.year || 0 : it[k] || "";
      next[k] = k === "year" ? Number(inputs[k].value) || 0 : inputs[k].value.trim();
    }
    const p: Record<string, unknown> = { id: it.id, ...changedFields(before, next) };
    const nextAuthors = authorsBox.value.split("\n").map((a) => a.trim()).filter(Boolean);
    if (JSON.stringify(nextAuthors) !== JSON.stringify(authors)) p.authors = nextAuthors;
    if (removed.length) p.remove_facets = removed.map(({ facet, value, origin }) => ({ facet, value, ...(origin ? { origin } : {}) }));
    if (newFacet.value.trim() && newValue.value.trim()) p.add_facets = [{ facet: newFacet.value.trim(), value: newValue.value.trim(), origin: "user" }];
    if (Object.keys(p).length === 1) return msg.info("Nothing has changed.");
    if (await msg.attempt(() => run<Item>("library.update_item", p))) saved();
  });

  const folderName = (id: string) => folders.find((f) => f.id === id)?.label ?? "";
  const file = (it.files ?? []).find((f) => f.online && READABLE.has(f.format));
  const readBtn = h("button", { type: "button", class: "primary", disabled: !file, title: file ? "Read it on its own page" : "No file of this book can be opened right now" }, icon("reader"), h("span", {}, "Read"));
  readBtn.addEventListener("click", () => file && readBook({ itemId: it.id, fileId: file.id }));
  const studyBtn = h("button", { type: "button", disabled: !file, title: "Open it in a tab of the Study grid, beside what is open" }, icon("grid"), h("span", {}, "Open in Study"));
  studyBtn.addEventListener("click", () => file && openInStudy({ kind: "book", itemId: it.id, fileId: file.id, title: it.title || "Untitled" }));
  const facetLine = (label: string, f?: Facet) => [h("dt", {}, label), h("dd", {}, h("span", {}, facetValue(f?.value)),
    f ? h("small", { class: "hint" }, reasonText(by(f), f.reason)) : null)];
  const lic = it.licence;

  const pane = h("div", { class: "lib-detail-body" },
    h("div", { class: "lib-detail-top" },
      cover({ title: it.title, type: typeNow?.value ?? summary?.type, authors, year: it.year }, true),
      h("button", { type: "button", class: "icon-btn detail-close", title: "Close", "aria-label": "Close book details", onclick: close }, icon("close"))),
    h("h2", { class: "lib-detail-title", tabindex: "-1" }, it.title || "Untitled"),
    it.subtitle ? h("p", { class: "lib-detail-sub" }, it.subtitle) : null,
    h("p", { class: "lib-detail-by" }, [authors.join(", "), it.year ? String(it.year) : ""].filter(Boolean).join(" · ") || "No author given"),
    it.status !== "online" ? h("p", {}, statusBadge(it.status)) : null,
    h("div", { class: "row lib-detail-actions" }, readBtn, studyBtn),
    msg.el,
    h("dl", { class: "lib-facts" }, facetLine("Type", typeNow), facetLine("Tradition", tradNow),
      it.language ? [h("dt", {}, "Language"), h("dd", {}, it.language)] : null,
      it.publisher ? [h("dt", {}, "Publisher"), h("dd", {}, it.publisher)] : null,
      it.series ? [h("dt", {}, "Series"), h("dd", {}, it.series, it.series_no ? ` ${it.series_no}` : "")] : null),
    h("div", { class: "row lib-facts-change" }, changeFacets),
    facetEditor,
    h("details", { class: "lib-fold", open: editLicence },
      h("summary", {}, h("span", {}, "Licence"), h("span", { class: "fold-value" }, plainName(lic))),
      h("p", {}, licenceLine(lic, "book", () => { licenceEditor.hidden = false; licenceEditor.querySelector<HTMLInputElement>("input:checked")?.focus(); })),
      lic ? h("p", { class: "hint" }, termsText(lic)) : null,
      lic?.origin ? h("p", { class: "hint" }, originText(lic.origin, "book")) : null,
      licenceEditor),
    h("details", { class: "lib-fold" },
      h("summary", {}, h("span", {}, "Files"), h("span", { class: "fold-value" }, String((it.files ?? []).length))),
      h("ul", { class: "files" }, (it.files ?? []).map((f) => h("li", {},
        h("code", {}, f.rel_path || f.path), " ", badge(f.format || "file"), " ",
        f.online ? badge("Online", "ok") : badge("Offline", "warn"),
        f.folder_id && folderName(f.folder_id) ? h("small", { class: "hint" }, ` in ${folderName(f.folder_id)}`) : null)))),
    h("details", { class: "lib-fold" }, h("summary", {}, h("span", {}, "Edit details")), form));
  queueMicrotask(() => {
    if (editFacets) pane.querySelector<HTMLElement>(".facet-edit select")?.focus();
  });
  return pane;
}
