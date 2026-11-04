// The Sermons view of My work: sermons, lessons and Bible studies in a table of Title, Passage, Date
// preached and Theme, sorted by any column. Double-click or Enter opens one in the Study grid; "Edit
// outline and text" opens it here as a page (sermonpage.ts).

import { segmented } from "../controls";
import { openInStudy } from "../document/open";
import { openExport } from "../printexport";
import { Message, Ref, run } from "../api";
import { button, clear, h } from "../dom";
import { icon } from "../icons";
import { openPopover } from "../popover";
import { loadPref, navigate, savePref } from "../state";
import { DocKind, KIND_NAME, Point, untitled } from "../sermonwords";
import { SermonPage, sermonPage } from "../sermonpage";
import { WorkItem, actionMenu, draggableItem, emptyState, folderName, loadFolders, moreButton, moveMenu, onWorkChanged, workChanged } from "../work";
import { SermonKey, nextSort, preachedText, sortSermons } from "../workwords";
import { deleteSermon } from "./notes";

export interface SermonDoc {
  id: string;
  kind: DocKind;
  title: string;
  passage?: Ref & { start: number; end: number };
  points: Point[];
  body: string;
  words: number;
  preached_on?: string;
  theme?: string;
  folder_id?: string;
  created_at: number;
  updated_at: number;
  deleted_at?: number;
}

const KINDS: [DocKind | "", string][] = [["", "All"], ["sermon", "Sermons"], ["lesson", "Lessons"], ["study", "Bible studies"]];
const COLUMNS: [SermonKey, string][] = [["title", "Title"], ["passage", "Passage"], ["preached", "Date preached"], ["theme", "Theme"]];
const NEW: [DocKind | "note", string][] = [["sermon", "Sermon"], ["lesson", "Lesson"], ["study", "Bible study"], ["note", "Note"]];
const SORT_KEY = "xegesis.sermons.sort";

export function sermonsView() {
  const msg = new Message();
  const sub = h("p", { class: "hint work-sub", "aria-live": "polite" });
  const query = h("input", { type: "search", placeholder: "Search titles, passages, themes and text", "aria-label": "Search sermons", class: "work-search" });
  let kindValue: DocKind | "" = "";
  const kindPills = segmented({ label: "Kind", small: true, value: kindValue, options: KINDS, onChange: (v) => { kindValue = v; void refresh(); } });
  const newBtn = h("button", { type: "button", class: "primary new-btn", "aria-haspopup": "menu", "aria-expanded": "false" }, icon("plus"), h("span", {}, "New"));
  const table = h("div", { class: "table-wrap sermon-table-wrap" });
  const index = h("div", { class: "sermon-index" },
    h("div", { class: "work-head" }, h("div", { class: "work-heading" }, h("h1", { class: "work-h1" }, "Sermons"), sub), newBtn),
    h("div", { class: "work-tools" }, query, kindPills),
    msg.el, table);
  const detail = h("section", { class: "doc-detail work-page", "aria-label": "Sermon or lesson", hidden: true });
  const el = h("div", { class: "view work-view" }, index, detail);

  let docs: SermonDoc[] = [];
  let openId: string | null = null;
  let selected = "";
  let sort = loadPref<{ key: SermonKey; dir: "asc" | "desc" }>(SORT_KEY, { key: "preached", dir: "desc" });
  let timer = 0;
  let page: SermonPage | null = null;

  async function refresh() {
    const params: Record<string, unknown> = {};
    if (query.value.trim()) params.query = query.value.trim();
    if (kindValue) params.kind = kindValue;
    const [got] = await Promise.all([msg.attempt(() => run<SermonDoc[]>("document.list", params)), loadFolders()]);
    if (!got) return;
    docs = got;
    renderTable();
  }

  function renderTable() {
    const rows = sortSermons(docs, sort.key, sort.dir);
    const filtered = !!(query.value.trim() || kindValue);
    sub.textContent = rows.length
      ? `${rows.length} ${rows.length === 1 ? "item" : "items"}${filtered ? " found" : ""}. Double-click one to open it in Study.`
      : "";
    if (rows.length === 0) {
      clear(table, filtered ? emptyState("search", "Nothing matches", "Try other words, or choose All.")
        : emptyState("sermon", "No sermons yet", "Start one with New: give it a passage, jot down the points, then write.",
          button("New sermon", () => void create("sermon"), { class: "primary" })));
      return;
    }
    const head = h("tr", {}, COLUMNS.map(([key, label]) => {
      const on = sort.key === key;
      const b = h("button", { type: "button", class: "sort-btn" }, label,
        h("span", { class: "sort-mark", "aria-hidden": "true" }, on ? (sort.dir === "asc" ? "▲" : "▼") : ""));
      b.addEventListener("click", () => {
        sort = nextSort(sort, key);
        savePref(SORT_KEY, sort);
        renderTable();
        table.querySelector<HTMLElement>(`th[data-key="${key}"] button`)?.focus();
      });
      return h("th", { scope: "col", "data-key": key, "aria-sort": on ? (sort.dir === "asc" ? "ascending" : "descending") : "none" }, b);
    }), h("th", { scope: "col" }, h("span", { class: "sr-only" }, "Actions")));
    clear(table, h("table", { class: "list sermon-table" }, h("thead", {}, head), h("tbody", {}, rows.map(row))));
  }

  function row(d: SermonDoc): HTMLElement {
    const name = d.title || untitled(d.kind);
    const item: WorkItem = { kind: "sermon", id: d.id, title: name, folder_id: d.folder_id };
    const open = () => openInStudy({ kind: "sermon", target: d.id, title: name });
    const menu = (b: HTMLElement) => actionMenu(b, `Actions for ${name}`, [
      ["Open in Study", open],
      ["Edit outline and text", () => void openBuilder(d.id)],
      ["Move to folder…", () => moveMenu(b, msg, item)],
      ["Print or save…", () => openExport({ title: name, sources: [{ kind: "document", id: d.id }] })],
      "-",
      ["Delete", () => void deleteSermon(msg, d), "danger"],
    ]);
    const where = [d.kind !== "sermon" ? KIND_NAME[d.kind] : "", d.folder_id ? folderName(d.folder_id) : ""].filter(Boolean);
    const none = () => h("span", { class: "none", "aria-label": "None" }, "—");
    const tr = h("tr", { tabindex: "0", class: selected === d.id ? "on" : "", "aria-selected": String(selected === d.id), title: "Double-click to open in Study" },
      h("td", { class: "sermon-title" }, h("strong", {}, name), where.length ? h("small", { class: "hint" }, where.join(" · ")) : null),
      h("td", { class: "nowrap" }, d.passage?.display ?? none()),
      h("td", { class: "nowrap" }, d.preached_on ? preachedText(d.preached_on) : none()),
      h("td", { class: "theme-cell" }, d.theme || none()),
      h("td", { class: "row-actions" }, moreButton(`Actions for ${name}`, menu)));
    const pick = () => {
      selected = d.id;
      for (const x of table.querySelectorAll("tr.on")) { x.classList.remove("on"); x.setAttribute("aria-selected", "false"); }
      tr.classList.add("on");
      tr.setAttribute("aria-selected", "true");
    };
    tr.addEventListener("click", pick);
    tr.addEventListener("focus", pick);
    tr.addEventListener("dblclick", open);
    tr.addEventListener("keydown", (e) => {
      if (e.target !== tr) return;
      if (e.key === "Enter") { e.preventDefault(); open(); }
      else if (e.key === "ContextMenu" || (e.shiftKey && e.key === "F10")) { e.preventDefault(); menu(tr.querySelector<HTMLElement>(".more-btn")!); }
      else if (e.key === "ArrowDown") { e.preventDefault(); (tr.nextElementSibling as HTMLElement | null)?.focus(); }
      else if (e.key === "ArrowUp") { e.preventDefault(); (tr.previousElementSibling as HTMLElement | null)?.focus(); }
    });
    draggableItem(tr, () => item, () => ({ kind: "sermon", target: d.id, title: name }));
    return tr;
  }

  async function create(kind: DocKind) {
    await closePage(false);
    const d = await msg.attempt(() => run<SermonDoc>("document.create", { kind }));
    if (!d) return;
    workChanged();
    await openBuilder(d.id, true);
  }

  // + New offers a sermon first here, the other kinds, and a note (made in the notes view).
  newBtn.addEventListener("click", () => {
    const body = h("div", { class: "menu-list" }, NEW.map(([k, label]) =>
      h("button", { type: "button", onclick: () => (k === "note" ? navigate("notes") : void create(k)) },
        icon(k === "note" ? "notes" : "sermon"), h("span", {}, label))));
    openPopover(newBtn, body, { label: "New", menu: true });
  });

  // The page ----------------------------------------------------------------------------------------
  // "Edit outline and text" (and #sermons/<id>) shows one document as a page here, in place of the table.

  async function openBuilder(id: string, fresh = false) {
    if (page && openId === id) return;
    await closePage(false);
    openId = id;
    selected = id;
    const back = button("", () => void closeBuilder(), { class: "crumb crumb-back", title: "Back to the list of sermons" });
    back.append(icon("prev"), h("span", {}, "My work"));
    page = sermonPage({
      id, fresh, home: back,
      onDeleted: () => void closeBuilder(),
      more: [["Open in Study", () => {
        const d = page?.doc();
        if (d) openInStudy({ kind: "sermon", target: d.id, title: d.title || untitled(d.kind) });
      }]],
    });
    clear(detail, page.el);
    index.hidden = true;
    detail.hidden = false;
    el.closest("main")?.scrollTo({ top: 0 });
  }

  async function closePage(_show: boolean) {
    const p = page;
    page = null;
    openId = null;
    if (p) {
      await p.flush();
      p.destroy();
    }
  }

  async function closeBuilder() {
    await closePage(true);
    detail.hidden = true;
    index.hidden = false;
    clear(detail);
    await refresh();
    table.querySelector<HTMLElement>("tr.on")?.focus();
  }

  query.addEventListener("input", () => {
    clearTimeout(timer);
    timer = window.setTimeout(() => void refresh(), 220);
  });
  query.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && query.value) { e.preventDefault(); query.value = ""; void refresh(); }
  });
  onWorkChanged(() => { if (el.offsetParent !== null && !openId) void refresh(); });

  return {
    el,
    focus: () => (page ? page.focus() : query.focus()),
    show: async () => {
      // #sermons/<id> opens one document's builder, so links and scripts can point at it.
      const asked = /^#sermons\/(.+)$/.exec(location.hash)?.[1];
      if (asked) {
        history.replaceState(null, "", "#sermons");
        await openBuilder(decodeURIComponent(asked));
        return;
      }
      if (!openId) await refresh();
    },
  };
}
