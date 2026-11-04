// The Sermons view of My work: sermons, lessons and Bible studies in a table of Title, Passage, Date
// preached and Theme, sorted by any column. Double-click or Enter opens one in the Study grid; "Edit
// outline and text" opens the builder here: its main passage, date preached and theme, an outline of
// points tied to verses, and the full text, with verses quoted in with their credit line
// (document.quote keeps each Bible's licence). Changes save on their own after a pause.

import { installedBibles } from "../bibles";
import { segmented } from "../controls";
import { openInStudy } from "../document/open";
import { openExport, printExportButton } from "../printexport";
import { Message, Ref, run } from "../api";
import { button, clear, field, h, select } from "../dom";
import { formatTime } from "../helpers";
import { icon } from "../icons";
import { flash } from "../listpicker";
import { openPopover } from "../popover";
import { loadPref, navigate, savePref, setPassage, state } from "../state";
import type { RichEditor } from "../editor";
import { DocKind, KIND_NAME, Point, cleanPoints, outlineMarkdown, untitled } from "../sermonwords";
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
const SAVE_AFTER = 1500;

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
  const detail = h("section", { class: "doc-detail", "aria-label": "Sermon or lesson", hidden: true });
  const el = h("div", { class: "view work-view" }, index, detail);

  let docs: SermonDoc[] = [];
  let openId: string | null = null;
  let selected = "";
  let sort = loadPref<{ key: SermonKey; dir: "asc" | "desc" }>(SORT_KEY, { key: "preached", dir: "desc" });
  let timer = 0;
  let editor: RichEditor | null = null;
  let flush: (() => Promise<boolean>) | null = null;

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
    if (flush) await flush();
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

  // The builder ---------------------------------------------------------------------------------------

  async function openBuilder(id: string, fresh = false) {
    if (flush && openId !== id) await flush();
    const d = await msg.attempt(() => run<SermonDoc>("document.get", { id }));
    if (!d) return;
    openId = id;
    selected = id;
    index.hidden = true;
    detail.hidden = false;
    renderDetail(d, fresh);
    detail.querySelector(".builder-bar")?.after(msg.el);
  }

  async function closeBuilder() {
    if (flush) await flush();
    editor?.destroy();
    editor = null;
    flush = null;
    openId = null;
    detail.hidden = true;
    index.hidden = false;
    clear(detail);
    table.before(msg.el);
    await refresh();
    table.querySelector<HTMLElement>("tr.on")?.focus();
  }

  function renderDetail(d: SermonDoc, fresh: boolean) {
    editor?.destroy();
    editor = null;
    let points: Point[] = d.points.map((p) => ({ ...p, level: p.level === 2 ? 2 : 1 }));
    let dirty = false;
    let saving: Promise<boolean> | null = null;
    let saveTimer = 0;
    const status = h("span", { class: "hint save-state", "aria-live": "polite" }, `Saved ${formatTime(d.updated_at)}`);

    const title = h("input", { type: "text", value: d.title === untitled(d.kind) && fresh ? "" : d.title, placeholder: untitled(d.kind), "aria-label": "Title", class: "doc-title" });
    const kind = select([["sermon", "Sermon"], ["lesson", "Lesson"], ["study", "Bible study"]], d.kind, { "aria-label": "Kind" });
    const passage = h("input", { type: "text", id: "doc-passage", value: d.passage?.display ?? "", placeholder: "e.g. Romans 8:28-39" });
    const readBtn = button("Read", () => openPassage(passage.value), { title: "Open the passage in the Reader" });
    const preached = h("input", { type: "date", id: "doc-preached", value: d.preached_on ?? "", class: "date-input" });
    const theme = h("input", { type: "text", id: "doc-theme", value: d.theme ?? "", placeholder: "e.g. Assurance", maxlength: "200" });

    const outline = h("ol", { class: "outline", "aria-label": "Outline" });
    const host = h("div", { class: "editor-host doc-text", "aria-busy": "true" }, h("p", { class: "hint" }, "Loading the editor…"));

    // Quoting verses.
    const qRef = h("input", { type: "text", placeholder: "e.g. Rom 8:31-32", "aria-label": "Verses to quote" });
    const qBible = select([], "", { "aria-label": "Bible to quote from" });
    const qForm = h("form", { class: "row wrap quote-row" }, qRef, qBible, h("button", { type: "submit" }, "Quote into text"));

    function changed() {
      dirty = true;
      status.textContent = "Not saved yet";
      clearTimeout(saveTimer);
      saveTimer = window.setTimeout(() => void save(), SAVE_AFTER);
    }

    async function save(): Promise<boolean> {
      clearTimeout(saveTimer);
      if (saving) await saving;
      if (!dirty) return true;
      dirty = false;
      status.textContent = "Saving…";
      saving = (async () => {
        const got = await msg.attempt(() => run<SermonDoc>("document.update", {
          id: d.id, title: title.value, kind: kind.value, passage: passage.value.trim(),
          preached_on: preached.value, theme: theme.value,
          points: cleanPoints(points), ...(editor ? { body: editor.getMarkdown() } : {}),
        }));
        if (!got) {
          dirty = true;
          status.textContent = "Not saved: see the message above";
          return false;
        }
        Object.assign(d, got);
        status.textContent = `Saved ${formatTime(got.updated_at)}`;
        const row = docs.find((x) => x.id === got.id);
        if (row) Object.assign(row, got);
        return true;
      })();
      const ok = await saving;
      saving = null;
      return ok;
    }
    flush = save;

    function renderOutline() {
      clear(outline, points.length === 0 ? h("li", { class: "empty outline-empty" }, "No points yet.") : points.map((p, i) => {
        const head = h("input", { type: "text", value: p.h, placeholder: p.level === 2 ? "Sub-point" : "Main point", "aria-label": `Point ${i + 1}` });
        const ref = h("input", { type: "text", value: p.ref ?? "", placeholder: "Verses", class: "point-ref", "aria-label": `Verses for point ${i + 1}` });
        head.addEventListener("input", () => { p.h = head.value; changed(); });
        ref.addEventListener("input", () => { p.ref = ref.value; changed(); });
        head.addEventListener("keydown", (e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            addPoint(i + 1, p.level);
          } else if (e.key === "Tab" && !e.ctrlKey && !e.altKey && (e.shiftKey ? p.level === 2 : p.level !== 2 && i > 0)) {
            e.preventDefault();
            indent(i, e.shiftKey ? 1 : 2);
          }
        });
        return h("li", { class: `point level-${p.level}` },
          head, ref,
          h("span", { class: "row small point-actions" },
            button(p.level === 2 ? "←" : "→", () => indent(i, p.level === 2 ? 1 : 2), { disabled: p.level !== 2 && i === 0, title: p.level === 2 ? "Make it a main point" : "Make it a sub-point", "aria-label": p.level === 2 ? `Make point ${i + 1} a main point` : `Make point ${i + 1} a sub-point` }),
            button("↑", () => move(i, -1), { disabled: i === 0, title: "Move up", "aria-label": `Move point ${i + 1} up` }),
            button("↓", () => move(i, 1), { disabled: i === points.length - 1, title: "Move down", "aria-label": `Move point ${i + 1} down` }),
            p.ref ? button("Quote", () => { qRef.value = p.ref ?? ""; qForm.requestSubmit(); }, { title: "Quote these verses into the text", "aria-label": `Quote the verses of point ${i + 1}` }) : null,
            button("×", () => remove(i), { class: "danger", title: "Remove", "aria-label": `Remove point ${i + 1}` })));
      }));
    }
    function focusPoint(i: number) {
      (outline.querySelectorAll<HTMLInputElement>("li.point > input:first-child")[i])?.focus();
    }
    function addPoint(at: number, level: 1 | 2 = 1) {
      points.splice(at, 0, { h: "", ref: "", level });
      renderOutline();
      focusPoint(at);
      changed();
    }
    function indent(i: number, level: 1 | 2) {
      points[i].level = level;
      renderOutline();
      focusPoint(i);
      changed();
    }
    function move(i: number, by: number) {
      [points[i], points[i + by]] = [points[i + by], points[i]];
      renderOutline();
      changed();
    }
    function remove(i: number) {
      points = points.filter((_, j) => j !== i);
      renderOutline();
      changed();
    }

    for (const c of [title, passage, theme]) c.addEventListener("input", changed);
    preached.addEventListener("change", changed);
    kind.addEventListener("change", () => {
      title.placeholder = untitled(kind.value as DocKind);
      changed();
    });

    qForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (!qRef.value.trim() || !qBible.value) {
        msg.info(qBible.value ? "Type the verses to quote, e.g. Rom 8:31." : "Install a Bible first (Library → Get Bibles) to quote verses.");
        return;
      }
      if (!(await save())) return;
      const r = await msg.attempt(() => run<{ allowed: boolean; reason?: string; document?: SermonDoc }>("document.quote", { id: d.id, ref: qRef.value.trim(), module: qBible.value }));
      if (!r) return;
      if (!r.allowed || !r.document) {
        msg.info(`Not quoted: ${r.reason ?? "the licence does not allow it"}.`);
        return;
      }
      Object.assign(d, r.document);
      editor?.setMarkdown(r.document.body);
      status.textContent = `Saved ${formatTime(r.document.updated_at)}`;
      flash(`Quoted ${qRef.value.trim()}.`);
      qRef.value = "";
    });

    const fromOutline = button("Add the outline to the text", () => {
      if (!editor) return;
      const md = outlineMarkdown(cleanPoints(points));
      if (!md) {
        msg.info("Write some points first.");
        return;
      }
      const cur = editor.getMarkdown().trimEnd();
      editor.setMarkdown(cur ? `${cur}\n\n${md}` : md);
      changed();
    }, { title: "Put each point in the text as a heading, ready to write under" });

    const del = button("Delete", async () => {
      await save();
      flush = null;
      await deleteSermon(msg, d);
      await closeBuilder();
    }, { class: "danger" });
    const back = button("", () => void closeBuilder(), { class: "back-link", title: "Back to the list of sermons" });
    back.append(icon("prev"), h("span", {}, "All sermons"));
    const inFolder = d.folder_id ? h("span", { class: "meta-chip folder-chip" }, icon("folder"), folderName(d.folder_id)) : null;

    clear(detail,
      h("div", { class: "row between builder-bar" }, back,
        h("div", { class: "row doc-actions" }, inFolder, status,
          button("Open in Study", () => openInStudy({ kind: "sermon", target: d.id, title: d.title || untitled(d.kind) }), { title: "Open it in a tab of the Study grid" }),
          printExportButton(() => ({ title: d.title, sources: [{ kind: "document", id: d.id }] })),
          del)),
      title,
      h("div", { class: "row wrap doc-props" },
        field("Kind", kind),
        h("div", { class: "field grow" }, h("label", { for: "doc-passage" }, "Main passage"), h("div", { class: "row" }, passage, readBtn)),
        field("Date preached", preached),
        h("div", { class: "field theme-field" }, h("label", { for: "doc-theme" }, "Theme"), theme)),
      h("div", { class: "doc-columns" },
        h("section", { class: "card doc-outline", "aria-label": "Outline" },
          h("div", { class: "row between" }, h("h2", {}, "Outline"), button("Add a point", () => addPoint(points.length, 1))),
          outline,
          h("p", { class: "hint" }, "Enter adds a point below; Tab makes a sub-point. Give a point its verses to quote them."),
          fromOutline,
          h("h3", {}, "Quote verses"),
          qForm),
        h("section", { class: "doc-body", "aria-label": "Text" }, h("h2", {}, "Text"), host)));

    renderOutline();
    loadBibles(qBible);

    import("../editor").then(({ createEditor }) => {
      if (openId !== d.id) return;
      clear(host);
      host.removeAttribute("aria-busy");
      editor = createEditor(host, { markdown: d.body, placeholder: "Write here…", label: "Text of the sermon or lesson", onChange: changed });
    });
    if (fresh) queueMicrotask(() => title.focus());
  }

  async function loadBibles(sel: HTMLSelectElement) {
    const mods = await installedBibles();
    const first = state.modules.find((m) => mods.some((x) => x.id === m)) ?? mods[0]?.id ?? "";
    clear(sel, mods.length === 0 ? h("option", { value: "" }, "No Bibles installed") :
      mods.map((m) => h("option", { value: m.id, selected: m.id === first }, m.abbreviation || m.name || m.id)));
    sel.value = first;
  }

  async function openPassage(ref: string) {
    if (!ref.trim()) return;
    const p = await msg.attempt(() => run<Ref>("verse.parse", { ref }));
    if (!p) return;
    navigate("reader");
    setPassage(p.display, p);
  }

  query.addEventListener("input", () => {
    clearTimeout(timer);
    timer = window.setTimeout(() => void refresh(), 220);
  });
  query.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && query.value) { e.preventDefault(); query.value = ""; void refresh(); }
  });
  document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s" && el.offsetParent !== null && flush) {
      e.preventDefault();
      void flush();
    }
  });
  window.addEventListener("beforeunload", () => void flush?.());
  onWorkChanged(() => { if (el.offsetParent !== null && !openId) void refresh(); });

  return {
    el,
    focus: () => (openId ? detail.querySelector<HTMLElement>(".doc-title") : query)?.focus(),
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
