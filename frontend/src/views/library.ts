// Library: adding books in one step (files, folders or drag and drop), the folders being watched,
// and the catalogue, where licences are shown in plain words and can be changed for one book or a
// selection. Every action is a library.* command.

import { AddResult, Facet, Folder, Item, ItemSummary, Message, pick, pickFiles, run } from "../api";
import { badge, button, clear, field, h, select } from "../dom";
import { changedFields, formatTime, pageLabel } from "../helpers";
import { licenceChooser, licenceLine, originText, plainName, termsText } from "../licence";
import { navigate } from "../state";
import { openBook } from "./book";

const PAGE = 50;

interface ScanResult {
  added: number; changed: number; removed: number; moved: number; restored: number; unchanged: number;
  errors: { rel_path: string; error: string }[];
}

export function libraryView() {
  let folders: Folder[] = [];
  const addMsg = new Message();
  const folderMsg = new Message();
  const folderList = h("div", { class: "table-wrap" });
  const items = catalogue(() => folders);

  // Adding books: one button, two ways.
  const menu = h("div", { class: "menu", role: "menu", hidden: true },
    button("Choose files…", () => { closeMenu(); chooseFiles(); }, { role: "menuitem" }),
    button("Choose a folder…", () => { closeMenu(); chooseFolder(); }, { role: "menuitem" }));
  const addButton = button("Add books", () => (menu.hidden ? openMenu() : closeMenu()),
    { class: "primary", "aria-haspopup": "menu", "aria-expanded": "false" });
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
    if (files?.length) add(files);
  }
  async function chooseFolder() {
    const dir = await addMsg.attempt(() => pick("folder", "Choose a folder of books"));
    if (dir) add([dir]);
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
    showFolders();
    items.refresh();
  }

  const el = h("div", { class: "view" },
    h("div", { class: "row between wrap" }, h("h1", {}, "Library"), menuWrap),
    h("p", { class: "hint" }, "Add EPUB, PDF, Word, HTML, Markdown or text files, or a whole folder. " +
      "You can also drop them on the window. Files you choose are copied into Documents/Xegesis Library; " +
      "folders stay where they are and are checked for new books every few minutes."),
    addMsg.el,
    items.el,
    h("section", { "aria-labelledby": "folders-h" },
      h("div", { class: "row between" }, h("h2", { id: "folders-h" }, "Folders"),
        button("Look for moved drives", async () => {
          if (await folderMsg.attempt(() => run("library.locate"))) {
            folderMsg.ok("Checked connected drives.");
            showFolders();
          }
        })),
      folderMsg.el, folderList));

  async function showFolders() {
    const fs = await folderMsg.attempt(() => run<Folder[]>("library.folders"));
    if (!fs) return;
    folders = fs;
    items.setFolders(fs);
    clear(folderList, fs.length === 0 ? h("p", { class: "empty" }, "No folders yet. Choose Add books to start.") :
      h("table", { class: "list" },
        h("thead", {}, h("tr", {}, ["Name", "Where", "Status", ""].map((t) => h("th", { scope: "col" }, t)))),
        h("tbody", {}, fs.map((f) => h("tr", {},
          h("td", {}, h("strong", {}, f.label || "Untitled")),
          h("td", { class: "path" }, f.path),
          h("td", {}, f.online ? badge("Online", "ok") : badge("Offline", "warn"),
            !f.online && f.last_seen_at ? h("small", { class: "hint" }, ` last seen ${formatTime(f.last_seen_at)}`) : null),
          h("td", { class: "actions" },
            button("Check now", () => scan(f), { disabled: !f.online, "aria-label": `Check ${f.label || f.path} for new books` }),
            button("Books", () => items.filterFolder(f.id), { "aria-label": `Show books in ${f.label || f.path}` }),
            button("Remove", () => removeFolder(f), { class: "danger", "aria-label": `Remove ${f.label || f.path}` })))))));
  }

  async function scan(f: Folder) {
    folderMsg.info(`Checking ${f.label || f.path}…`);
    const r = await folderMsg.attempt(() => run<ScanResult>("library.scan", { folder_id: f.id }));
    if (!r) return;
    clear(folderMsg.el,
      `Checked ${f.label || f.path}: ${r.added} added, ${r.changed} changed, ${r.moved} moved, ${r.restored} back, ${r.removed} gone.`,
      r.errors.length ? h("details", {}, h("summary", {}, `${r.errors.length} files could not be read`),
        h("ul", {}, r.errors.map((e) => h("li", {}, h("code", {}, e.rel_path), ` ${e.error}`)))) : null);
    folderMsg.el.className = "msg ok";
    items.refresh();
  }

  async function removeFolder(f: Folder) {
    if (!confirm(`Remove ${f.label || f.path} and its books from the library? Files on disk are left alone.`)) return;
    const r = await folderMsg.attempt(() => run<{ items: number; files: number }>("library.remove_folder", { id: f.id }));
    if (!r) return;
    folderMsg.ok(`Removed ${f.label || f.path} (${r.items} books).`);
    showFolders();
    items.refresh();
  }

  async function show() {
    await showFolders();
    items.refresh();
  }
  return { el, show, drop: add, changed: () => { showFolders(); items.refresh(); } };
}

// catalogue is the filtered, paged list of items with a detail pane and licence changes.
function catalogue(getFolders: () => Folder[]) {
  const msg = new Message();
  const text = h("input", { type: "search", autocomplete: "off" });
  const author = h("input", { type: "search", autocomplete: "off" });
  const kind = h("input", { type: "text", autocomplete: "off", placeholder: "e.g. book, commentary", list: "kinds" });
  const folder = select([["", "All folders"]], "");
  const table = h("div", { class: "table-wrap", "aria-live": "polite" });
  const pager = h("div", { class: "row" });
  const detail = h("aside", { class: "pane detail", "aria-label": "Book details", hidden: true });
  const reviewing = h("p", { class: "msg", role: "status", hidden: true });
  const selection = h("div", { class: "row wrap", hidden: true });
  const bulk = h("div", { class: "card", hidden: true });
  let offset = 0;
  let since = 0;

  const filters = h("form", { class: "filters", role: "search", "aria-label": "Filter books" },
    field("Title", text), field("Author", author), field("Kind", kind), field("Folder", folder),
    h("button", { type: "submit", class: "primary" }, "Filter"),
    h("datalist", { id: "kinds" }, ["book", "bible", "commentary", "dictionary", "journal"].map((k) => h("option", { value: k }))));
  filters.addEventListener("submit", (e) => {
    e.preventDefault();
    offset = 0;
    refresh();
  });
  folder.addEventListener("change", () => { offset = 0; refresh(); });

  const el = h("section", { "aria-labelledby": "books-h" },
    h("h2", { id: "books-h" }, "Books"),
    filters, reviewing, msg.el, selection, bulk,
    h("div", { class: "panes" }, h("div", { class: "pane" }, table, pager), detail));

  const checked = () => [...table.querySelectorAll<HTMLInputElement>("tbody input[type=checkbox]:checked")].map((b) => b.value);

  function updateSelection() {
    const ids = checked();
    selection.hidden = ids.length === 0;
    clear(selection, h("span", {}, `${ids.length} selected`),
      button("Set licence for selected", () => openBulk(), { class: "primary" }));
    if (!ids.length) bulk.hidden = true;
  }

  function openBulk() {
    const ids = checked();
    const chooser = licenceChooser(`Licence for ${ids.length} ${ids.length === 1 ? "book" : "books"}`);
    const bmsg = new Message();
    clear(bulk, chooser.el, h("div", { class: "row" },
      button("Set licence", async () => {
        const r = await bmsg.attempt(() => run<{ updated: number }>("library.set_licence", { item_ids: ids, ...chooser.value() }));
        if (!r) return;
        bulk.hidden = true;
        msg.ok(`Licence set for ${r.updated} ${r.updated === 1 ? "book" : "books"}.`);
        refresh();
      }, { class: "primary" }),
      button("Cancel", () => { bulk.hidden = true; })), bmsg.el);
    bulk.hidden = false;
    bulk.querySelector<HTMLInputElement>("input")?.focus();
  }

  async function refresh() {
    const p: Record<string, unknown> = { limit: PAGE, offset };
    if (text.value.trim()) p.text = text.value.trim();
    if (author.value.trim()) p.author = author.value.trim();
    if (kind.value.trim()) p.kind = kind.value.trim();
    if (folder.value) p.folder_id = folder.value;
    if (since) p.added_since = since;
    const r = await msg.attempt(() => run<{ items: ItemSummary[]; total: number }>("library.items", p));
    if (!r) return;
    reviewing.hidden = !since;
    clear(reviewing, since ? [`Showing the ${r.total} ${r.total === 1 ? "book" : "books"} just added. `,
      button("Show all books", () => { since = 0; offset = 0; refresh(); }, { class: "link" })] : null);
    const all = h("input", { type: "checkbox", "aria-label": "Select all books on this page" });
    all.addEventListener("change", () => {
      table.querySelectorAll<HTMLInputElement>("tbody input[type=checkbox]").forEach((b) => { b.checked = all.checked; });
      updateSelection();
    });
    clear(table, r.items.length === 0 ? h("p", { class: "empty" }, since ? "No books match." : "No books yet. Choose Add books, or drop files on the window.") :
      h("table", { class: "list" },
        h("thead", {}, h("tr", {}, h("th", { scope: "col" }, all),
          ["Title", "Authors", "Year", "Kind", "Status", "Licence"].map((t) => h("th", { scope: "col" }, t)))),
        h("tbody", { onchange: updateSelection }, r.items.map((it) => h("tr", {},
          h("td", {}, h("input", { type: "checkbox", value: it.id, "aria-label": `Select ${it.title || "Untitled"}` })),
          h("td", {}, button(it.title || "Untitled", () => open(it.id), { class: "link" }),
            it.subtitle ? h("small", { class: "hint" }, h("br"), it.subtitle) : null),
          h("td", {}, (it.authors ?? []).join(", ")),
          h("td", { class: "num" }, it.year || ""),
          h("td", {}, it.kind),
          h("td", {}, statusBadge(it.status)),
          h("td", {}, it.licence_name || plainName(it.licence), " · ",
            button("change", () => open(it.id, true), { class: "link", "aria-label": `Change the licence of ${it.title || "this book"}` }),
            it.licence_origin && it.licence_origin !== "default"
              ? h("small", { class: "hint" }, h("br"), originText(it.licence_origin)) : null))))));
    updateSelection();
    clear(pager, h("span", { class: "hint" }, pageLabel(offset, r.items.length, r.total)),
      button("Previous", () => { offset = Math.max(0, offset - PAGE); refresh(); }, { disabled: offset === 0 }),
      button("Next", () => { offset += PAGE; refresh(); }, { disabled: offset + PAGE >= r.total }));
  }

  async function open(id: string, editLicence = false) {
    const it = await msg.attempt(() => run<Item>("library.item", { id }));
    if (!it) return;
    detail.hidden = false;
    clear(detail, itemDetail(it, getFolders(), editLicence, () => { detail.hidden = true; }, (updated) => { refresh(); open(updated.id); }));
    detail.scrollIntoView({ block: "start" });
    const target = editLicence ? detail.querySelector<HTMLElement>(".licence input:checked") : detail.querySelector<HTMLElement>("h3");
    target?.focus({ preventScroll: true });
  }

  return {
    el, refresh,
    setFolders(fs: Folder[]) {
      const v = folder.value;
      clear(folder, h("option", { value: "" }, "All folders"), fs.map((f) => h("option", { value: f.id }, f.label || f.path)));
      folder.value = fs.some((f) => f.id === v) ? v : "";
    },
    filterFolder(id: string) {
      folder.value = id;
      since = 0;
      offset = 0;
      refresh();
      el.scrollIntoView({ block: "start" });
    },
    // review lists only the books added since t.
    review(t: number) {
      since = t;
      offset = 0;
      text.value = author.value = kind.value = folder.value = "";
      refresh();
      el.scrollIntoView({ block: "start" });
    },
  };
}

function statusBadge(s: string) {
  return s === "online" ? badge("Online", "ok") : s === "offline" ? badge("Offline", "warn") : badge("Missing", "bad");
}

const EDITABLE = ["title", "subtitle", "kind", "year", "language", "publisher", "series", "series_no", "isbn"] as const;
const LABELS: Record<string, string> = {
  title: "Title", subtitle: "Subtitle", kind: "Kind", year: "Year", language: "Language", publisher: "Publisher",
  series: "Series", series_no: "Number in series", isbn: "ISBN",
};

const READABLE = new Set(["epub", "pdf", "docx", "html", "md", "txt"]);

function itemDetail(it: Item, folders: Folder[], editLicence: boolean, close: () => void, saved: (it: Item) => void) {
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

  const chooser = licenceChooser("Change licence");
  chooser.set(it.licence);
  const editor = h("div", { hidden: !editLicence }, chooser.el,
    h("div", { class: "row" },
      button("Save licence", async () => {
        const r = await msg.attempt(() => run("library.set_licence", { item_ids: [it.id], ...chooser.value() }));
        if (!r) return;
        const fresh = await msg.attempt(() => run<Item>("library.item", { id: it.id }));
        if (fresh) saved(fresh);
      }, { class: "primary" }),
      button("Cancel", () => { editor.hidden = true; })));

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
    const r = await msg.attempt(() => run<Item>("library.update_item", p));
    if (r) saved(r);
  });

  const folderName = (id: string) => folders.find((f) => f.id === id)?.label ?? "";

  return h("div", {},
    h("div", { class: "row between" }, h("h3", { tabindex: "-1" }, it.title || "Untitled"), button("Close", close, { "aria-label": "Close book details" })),
    h("p", {}, statusBadge(it.status)),
    h("p", {}, licenceLine(it.licence, "book", () => {
      editor.hidden = false;
      editor.querySelector<HTMLInputElement>("input:checked")?.focus();
    }), it.licence ? h("small", { class: "hint" }, h("br"), termsText(it.licence)) : null),
    editor,
    msg.el,
    h("h4", {}, "Files"),
    h("ul", { class: "files" }, (it.files ?? []).map((f) => h("li", {},
      h("code", {}, f.rel_path || f.path), " ", badge(f.format || "file"), " ",
      f.online ? badge("Online", "ok") : badge("Offline", "warn"),
      f.folder_id && folderName(f.folder_id) ? h("small", { class: "hint" }, ` in ${folderName(f.folder_id)}`) : null,
      f.online && READABLE.has(f.format) ? [" ", button("Read", () => {
        navigate("reader");
        openBook({ itemId: it.id, fileId: f.id });
      }, { "aria-label": `Read ${f.rel_path || it.title}` })] : null))),
    h("details", {}, h("summary", {}, "Edit details"), form));
}
