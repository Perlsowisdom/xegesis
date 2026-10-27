// Library: folders of books, scanning them, answering licence questions, and the catalogue.

import { Facet, Folder, Item, ItemSummary, Licence, Message, pick, run } from "../api";
import { badge, button, clear, field, h, select } from "../dom";
import { changedFields, formatTime, pageLabel } from "../helpers";
import { licenceForm, termsText, typeLabel } from "../licence";

const PAGE = 50;

interface ScanResult {
  added: number; changed: number; removed: number; moved: number; restored: number; unchanged: number;
  errors: { rel_path: string; error: string }[];
  needs_licence_total: number;
}

export function libraryView() {
  let folders: Folder[] = [];
  const folderMsg = new Message();
  const folderList = h("div", { class: "table-wrap" });
  const questions = licenceQuestions(() => items.refresh());
  const items = catalogue(() => folders);

  // Adding a folder
  const path = h("input", { type: "text", id: "folder-path", spellcheck: "false", placeholder: "Choose a folder", required: true });
  const label = h("input", { type: "text", autocomplete: "off", placeholder: "Optional" });
  const addForm = h("form", { class: "row wrap end", "aria-label": "Add a library folder" },
    h("div", { class: "field grow" }, h("label", { for: "folder-path" }, "Folder"),
      h("div", { class: "row" }, path, button("Choose…", async () => {
        const p = await folderMsg.attempt(() => pick("folder", "Choose a folder of books"));
        if (p) path.value = p;
      }))),
    field("Name", label),
    h("button", { type: "submit", class: "primary" }, "Add folder"));
  addForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    const f = await folderMsg.attempt(() => run<Folder>("library.add_folder", { path: path.value.trim(), ...(label.value.trim() ? { label: label.value.trim() } : {}) }));
    if (!f) return;
    path.value = label.value = "";
    folderMsg.ok(`Added ${f.label || f.path}. Scan it to find its books.`);
    showFolders();
  });

  const el = h("div", { class: "view" },
    h("h1", {}, "Library"),
    h("section", { "aria-labelledby": "folders-h" },
      h("div", { class: "row between" }, h("h2", { id: "folders-h" }, "Folders"),
        button("Look for moved drives", async () => {
          if (await folderMsg.attempt(() => run("library.locate"))) {
            folderMsg.ok("Checked mounted drives.");
            showFolders();
          }
        })),
      folderList, addForm, folderMsg.el),
    questions.el,
    items.el);

  async function showFolders() {
    const fs = await folderMsg.attempt(() => run<Folder[]>("library.folders"));
    if (!fs) return;
    folders = fs;
    items.setFolders(fs);
    clear(folderList, fs.length === 0 ? h("p", { class: "empty" }, "No folders yet. Add a folder of PDF, EPUB or other book files.") :
      h("table", { class: "list" },
        h("thead", {}, h("tr", {}, ["Name", "Where", "Status", ""].map((t) => h("th", { scope: "col" }, t)))),
        h("tbody", {}, fs.map((f) => h("tr", {},
          h("td", {}, h("strong", {}, f.label || "Untitled")),
          h("td", { class: "path" }, f.path),
          h("td", {}, f.online ? badge("Online", "ok") : badge("Offline", "warn"),
            !f.online && f.last_seen_at ? h("small", { class: "hint" }, ` last seen ${formatTime(f.last_seen_at)}`) : null),
          h("td", { class: "actions" },
            button("Scan", () => scan(f), { disabled: !f.online, "aria-label": `Scan ${f.label || f.path}` }),
            button("Books", () => items.filterFolder(f.id), { "aria-label": `Show books in ${f.label || f.path}` }),
            button("Licences", () => questions.open(f), { "aria-label": `Licence questions for ${f.label || f.path}` }),
            button("Remove", () => removeFolder(f), { class: "danger", "aria-label": `Remove ${f.label || f.path}` })))))));
  }

  async function scan(f: Folder) {
    folderMsg.info(`Scanning ${f.label || f.path}… this can take a while for large folders.`);
    const r = await folderMsg.attempt(() => run<ScanResult>("library.scan", { folder_id: f.id }));
    if (!r) return;
    clear(folderMsg.el,
      `Scanned ${f.label || f.path}: ${r.added} added, ${r.changed} changed, ${r.moved} moved, ${r.restored} restored, ${r.removed} removed, ${r.unchanged} unchanged.`,
      r.errors.length ? h("details", {}, h("summary", {}, `${r.errors.length} files could not be read`),
        h("ul", {}, r.errors.map((e) => h("li", {}, h("code", {}, e.rel_path), ` ${e.error}`)))) : null,
      r.needs_licence_total ? h("span", {}, ` ${r.needs_licence_total} books need a licence decision.`) : null);
    folderMsg.el.className = "msg ok";
    if (r.needs_licence_total) questions.open(f);
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
  return { el, show };
}

// licenceQuestions lists books whose licence is undecided and records the user's answers.
function licenceQuestions(changed: () => void) {
  const msg = new Message();
  const list = h("div", {});
  const pager = h("div", { class: "row" });
  const title = h("h2", { id: "lq-h" }, "Licence questions");
  const form = licenceForm("Licence to record", true);
  let folder: Folder | null = null;
  let offset = 0;
  let rows: ItemSummary[] = [];

  const el = h("section", { class: "card", "aria-labelledby": "lq-h", hidden: true },
    h("div", { class: "row between" }, title, button("Close", () => { el.hidden = true; })),
    h("p", { class: "hint" }, "Tell Xegesis what you may do with each book. A suggestion is shown where the book's own details name a licence."),
    msg.el, list, pager,
    h("div", { class: "row wrap" },
      button("Use the suggestion for selected", () => apply("selected", true), { class: "primary" }),
      button("Use the suggestion for all undecided", () => apply("all", true))),
    h("details", {}, h("summary", {}, "Choose a licence myself"),
      form.el,
      h("div", { class: "row wrap" },
        button("Apply to selected", () => apply("selected", false), { class: "primary" }),
        button("Apply to all undecided", () => apply("all", false)))));

  async function load() {
    const p: Record<string, unknown> = { needs_licence: true, limit: PAGE, offset };
    if (folder) p.folder_id = folder.id;
    const r = await msg.attempt(() => run<{ items: ItemSummary[]; total: number }>("library.items", p));
    if (!r) return;
    rows = r.items;
    title.textContent = `Licence questions${folder ? ` · ${folder.label || folder.path}` : ""} (${r.total})`;
    const all = h("input", { type: "checkbox", "aria-label": "Select all on this page" });
    all.addEventListener("change", () => list.querySelectorAll<HTMLInputElement>("tbody input").forEach((b) => { b.checked = all.checked; }));
    clear(list, rows.length === 0 ? h("p", { class: "empty" }, "Every book has a licence. Nothing to answer.") :
      h("div", { class: "table-wrap" }, h("table", { class: "list" },
        h("thead", {}, h("tr", {}, h("th", { scope: "col" }, all), ["Book", "Suggested licence"].map((t) => h("th", { scope: "col" }, t)))),
        h("tbody", {}, rows.map((it) => h("tr", {},
          h("td", {}, h("input", { type: "checkbox", value: it.id, "aria-label": `Select ${it.title}` })),
          h("td", {}, h("strong", {}, it.title || "Untitled"), it.authors?.length ? h("small", { class: "hint" }, ` ${it.authors.join(", ")}`) : null),
          h("td", {}, it.suggested_licence ? suggestion(it.suggested_licence) : h("span", { class: "hint" }, "None"))))))));
    clear(pager, h("span", { class: "hint" }, pageLabel(offset, rows.length, r.total)),
      button("Previous", () => { offset = Math.max(0, offset - PAGE); load(); }, { disabled: offset === 0 }),
      button("Next", () => { offset += PAGE; load(); }, { disabled: offset + PAGE >= r.total }));
  }

  async function apply(which: "selected" | "all", suggested: boolean) {
    const p: Record<string, unknown> = {};
    if (which === "selected") {
      const ids = [...list.querySelectorAll<HTMLInputElement>("tbody input:checked")].map((b) => b.value);
      if (!ids.length) return msg.info("Select at least one book first.");
      p.item_ids = ids;
    } else {
      p.pending = true;
      if (folder) p.folder_id = folder.id;
    }
    if (suggested) p.use_suggested = true;
    else Object.assign(p, form.value());
    const r = await msg.attempt(() => run<{ updated: number }>("library.set_licence", p));
    if (!r) return;
    msg.ok(`Recorded a licence for ${r.updated} books.`);
    offset = 0;
    load();
    changed();
  }

  return {
    el,
    open(f: Folder | null) {
      folder = f;
      offset = 0;
      el.hidden = false;
      load();
      el.scrollIntoView({ block: "start" });
    },
  };
}

function suggestion(l: Licence) {
  return h("span", {}, typeLabel(l.type), l.spdx ? h("small", { class: "hint" }, ` ${l.spdx}`) : null);
}

// catalogue is the filtered, paged list of items with an editable detail pane.
function catalogue(getFolders: () => Folder[]) {
  const msg = new Message();
  const text = h("input", { type: "search", autocomplete: "off" });
  const author = h("input", { type: "search", autocomplete: "off" });
  const kind = h("input", { type: "text", autocomplete: "off", placeholder: "e.g. book, commentary", list: "kinds" });
  const folder = select([["", "All folders"]], "");
  const needs = h("input", { type: "checkbox" });
  const table = h("div", { class: "table-wrap", "aria-live": "polite" });
  const pager = h("div", { class: "row" });
  const detail = h("aside", { class: "pane detail", "aria-label": "Book details", hidden: true });
  let offset = 0;

  const filters = h("form", { class: "filters", role: "search", "aria-label": "Filter books" },
    field("Title", text), field("Author", author), field("Kind", kind), field("Folder", folder),
    h("label", { class: "check" }, needs, " Licence undecided"),
    h("button", { type: "submit", class: "primary" }, "Filter"),
    h("datalist", { id: "kinds" }, ["book", "bible", "commentary", "dictionary", "journal"].map((k) => h("option", { value: k }))));
  filters.addEventListener("submit", (e) => {
    e.preventDefault();
    offset = 0;
    refresh();
  });
  folder.addEventListener("change", () => { offset = 0; refresh(); });
  needs.addEventListener("change", () => { offset = 0; refresh(); });

  const el = h("section", { "aria-labelledby": "books-h" },
    h("h2", { id: "books-h" }, "Books"),
    filters, msg.el,
    h("div", { class: "panes" }, h("div", { class: "pane" }, table, pager), detail));

  async function refresh() {
    const p: Record<string, unknown> = { limit: PAGE, offset };
    if (text.value.trim()) p.text = text.value.trim();
    if (author.value.trim()) p.author = author.value.trim();
    if (kind.value.trim()) p.kind = kind.value.trim();
    if (folder.value) p.folder_id = folder.value;
    if (needs.checked) p.needs_licence = true;
    const r = await msg.attempt(() => run<{ items: ItemSummary[]; total: number }>("library.items", p));
    if (!r) return;
    clear(table, r.items.length === 0 ? h("p", { class: "empty" }, "No books match.") :
      h("table", { class: "list" },
        h("thead", {}, h("tr", {}, ["Title", "Authors", "Year", "Kind", "Status", "Licence"].map((t) => h("th", { scope: "col" }, t)))),
        h("tbody", {}, r.items.map((it) => h("tr", {},
          h("td", {}, button(it.title || "Untitled", () => open(it.id), { class: "link" }),
            it.subtitle ? h("small", { class: "hint" }, h("br"), it.subtitle) : null),
          h("td", {}, (it.authors ?? []).join(", ")),
          h("td", { class: "num" }, it.year || ""),
          h("td", {}, it.kind),
          h("td", {}, statusBadge(it.status)),
          h("td", {}, it.licence ? typeLabel(it.licence) : h("span", { class: "hint" }, "Undecided")))))));
    clear(pager, h("span", { class: "hint" }, pageLabel(offset, r.items.length, r.total)),
      button("Previous", () => { offset = Math.max(0, offset - PAGE); refresh(); }, { disabled: offset === 0 }),
      button("Next", () => { offset += PAGE; refresh(); }, { disabled: offset + PAGE >= r.total }));
  }

  async function open(id: string) {
    const it = await msg.attempt(() => run<Item>("library.item", { id }));
    if (!it) return;
    detail.hidden = false;
    clear(detail, itemDetail(it, getFolders(), () => { detail.hidden = true; }, (updated) => { refresh(); open(updated.id); }));
    detail.querySelector<HTMLElement>("h3")?.focus();
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
      offset = 0;
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

function itemDetail(it: Item, folders: Folder[], close: () => void, saved: (it: Item) => void) {
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
  const licence = licenceForm("Change licence");
  if (it.licence) licence.set(it.licence);

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
    h("p", {}, statusBadge(it.status), " ", h("span", { class: "hint" }, it.id)),
    msg.el, form,
    h("h4", {}, "Licence"),
    h("p", {}, it.licence ? [typeLabel(it.licence.type), it.licence.spdx ? ` (${it.licence.spdx})` : "", h("br"), h("small", { class: "hint" }, termsText(it.licence))]
      : h("span", { class: "hint" }, it.suggested_licence ? `Undecided. Suggested: ${typeLabel(it.suggested_licence.type)}` : "Undecided")),
    h("details", {}, h("summary", {}, "Change licence"), licence.el,
      button("Record licence", async () => {
        const r = await msg.attempt(() => run("library.set_licence", { item_ids: [it.id], ...licence.value() }));
        if (r) {
          msg.ok("Licence recorded.");
          const fresh = await msg.attempt(() => run<Item>("library.item", { id: it.id }));
          if (fresh) saved(fresh);
        }
      }, { class: "primary" })),
    h("h4", {}, "Files"),
    h("ul", { class: "files" }, (it.files ?? []).map((f) => h("li", {},
      h("code", {}, f.rel_path || f.path), " ", badge(f.format || "file"), " ",
      f.online ? badge("Online", "ok") : badge("Offline", "warn"),
      f.folder_id && folderName(f.folder_id) ? h("small", { class: "hint" }, ` in ${folderName(f.folder_id)}`) : null))));
}
