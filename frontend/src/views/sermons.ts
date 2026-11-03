// The Sermons section: sermons, lessons and Bible studies. Each has a main passage, an outline of
// points tied to verses, and the full text; verses are quoted into the text with their credit line
// (document.quote keeps each Bible's licence). Changes save on their own after a pause.

import { printExportButton } from "../printexport";
import { Message, Module, Ref, run } from "../api";
import { button, clear, field, h, select } from "../dom";
import { formatTime } from "../helpers";
import { flash } from "../listpicker";
import { navigate, setPassage, state } from "../state";
import type { RichEditor } from "../editor";
import { DocKind, Point, cleanPoints, docSummary, outlineMarkdown, untitled } from "../sermonwords";

export interface SermonDoc {
  id: string;
  kind: DocKind;
  title: string;
  passage?: Ref & { start: number; end: number };
  points: Point[];
  body: string;
  words: number;
  created_at: number;
  updated_at: number;
  deleted_at?: number;
}

const KINDS: [DocKind | "", string][] = [["", "All kinds"], ["sermon", "Sermons"], ["lesson", "Lessons"], ["study", "Bible studies"]];
const SAVE_AFTER = 1500;

export function sermonsView() {
  const msg = new Message();
  const query = h("input", { type: "search", placeholder: "Search titles, passages and text", "aria-label": "Search sermons and lessons" });
  const kindFilter = select(KINDS as [string, string][], "", { "aria-label": "Kind" });
  const index = h("ul", { class: "plain doc-index", "aria-label": "Sermons and lessons" });
  const newKind = select([["sermon", "Sermon"], ["lesson", "Lesson"], ["study", "Bible study"]], "sermon", { "aria-label": "Kind of the new document" });
  const startForm = h("form", { class: "row" }, newKind, h("button", { type: "submit", class: "primary" }, "New"));
  const trashToggle = h("button", { type: "button", class: "link", "aria-pressed": "false" }, "Recently deleted");
  const sidebar = h("div", { class: "lists-index" },
    h("div", { class: "doc-filters" }, query, kindFilter), index, startForm, trashToggle);
  const detail = h("section", { class: "list-detail doc-detail", "aria-label": "Sermon or lesson" });
  const el = h("div", { class: "view" },
    h("h1", {}, "Sermons and lessons"),
    h("p", { class: "hint" }, "Plan a sermon, lesson or Bible study: its main passage, an outline of points with their verses, and the full text. Quote verses into the text with their credit line."),
    msg.el, h("div", { class: "lists-layout" }, sidebar, detail));

  let docs: SermonDoc[] = [];
  let openId: string | null = null;
  let trash = false;
  let timer = 0;
  let editor: RichEditor | null = null;
  let flush: (() => Promise<boolean>) | null = null;

  async function refresh() {
    const params: Record<string, unknown> = {};
    if (query.value.trim()) params.query = query.value.trim();
    if (kindFilter.value) params.kind = kindFilter.value;
    const got = await msg.attempt(() => run<SermonDoc[]>(trash ? "document.deleted" : "document.list", trash ? {} : params));
    if (!got) return;
    docs = got;
    renderIndex();
    if (!trash && !openId) {
      clear(detail, h("div", { class: "empty doc-empty" },
        h("p", {}, docs.length ? "Choose a sermon or lesson on the left, or start a new one." : "No sermons or lessons yet."),
        docs.length ? null : h("p", { class: "hint" }, "Choose New to start one. Give it a passage, jot down the points, then write.")));
    }
  }

  function renderIndex() {
    if (trash) {
      clear(index, docs.length === 0 ? h("li", { class: "empty" }, "Nothing has been deleted.") : docs.map((d) => h("li", { class: "doc-row" },
        h("strong", {}, d.title), h("span", { class: "hint" }, ` Deleted ${formatTime(d.deleted_at ?? d.updated_at)}`),
        button("Restore", async () => {
          const back = await msg.attempt(() => run<SermonDoc>("document.restore", { id: d.id }));
          if (back) {
            flash(`Restored "${back.title}".`);
            setTrash(false);
            open(back.id);
          }
        }, { "aria-label": `Restore ${d.title}` }))));
      return;
    }
    clear(index, docs.length === 0 ? h("li", { class: "empty" }, query.value || kindFilter.value ? "None match." : "") :
      docs.map((d) => h("li", {},
        h("button", { type: "button", class: "list-pick", "aria-current": d.id === openId ? "true" : null, onclick: () => open(d.id) },
          h("strong", {}, d.title), h("span", { class: "hint doc-meta" }, docSummary(d))))));
  }

  function setTrash(on: boolean) {
    trash = on;
    trashToggle.setAttribute("aria-pressed", String(on));
    trashToggle.textContent = on ? "Back to your documents" : "Recently deleted";
    startForm.hidden = on;
    refresh();
  }
  trashToggle.addEventListener("click", async () => {
    if (flush) await flush();
    setTrash(!trash);
  });

  startForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    if (flush) await flush();
    const d = await msg.attempt(() => run<SermonDoc>("document.create", { kind: newKind.value }));
    if (d) {
      query.value = "";
      kindFilter.value = "";
      await open(d.id, true);
    }
  });

  async function open(id: string, fresh = false) {
    if (flush && openId !== id) await flush();
    const d = await msg.attempt(() => run<SermonDoc>("document.get", { id }));
    if (!d) return;
    openId = id;
    await refresh();
    renderDetail(d, fresh);
  }

  // The editor ----------------------------------------------------------------------------------------

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
        if (row) {
          Object.assign(row, got);
          renderIndex();
        }
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

    for (const c of [title, passage]) c.addEventListener("input", changed);
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
      if ((await msg.attempt(() => run("document.delete", { id: d.id }))) === undefined) return;
      editor?.destroy();
      editor = null;
      flush = null;
      openId = null;
      clear(msg.el, `Deleted "${d.title}". It stays under Recently deleted. `, button("Undo", async () => {
        if ((await msg.attempt(() => run("document.restore", { id: d.id }))) !== undefined) {
          msg.ok("Restored.");
          open(d.id);
        }
      }));
      msg.el.className = "msg ok";
      msg.el.setAttribute("role", "status");
      refresh();
    }, { class: "danger" });

    clear(detail,
      title,
      h("div", { class: "row wrap doc-props" },
        field("Kind", kind),
        h("div", { class: "field grow" }, h("label", { for: "doc-passage" }, "Main passage"), h("div", { class: "row" }, passage, readBtn)),
        h("div", { class: "row doc-actions" }, status,
          printExportButton(() => ({ title: d.title, sources: [{ kind: "document", id: d.id }] })),
          del)),
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
    const mods = (await run<Module[]>("bible.modules").catch(() => [])) ?? [];
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
    timer = window.setTimeout(() => refresh(), 250);
  });
  kindFilter.addEventListener("change", () => refresh());
  document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s" && el.offsetParent !== null && flush) {
      e.preventDefault();
      void flush();
    }
  });
  window.addEventListener("beforeunload", () => void flush?.());

  return {
    el,
    show: async () => {
      // #sermons/<id> opens one document, so links and scripts can point at it.
      const asked = /^#sermons\/(.+)$/.exec(location.hash)?.[1];
      if (asked) {
        history.replaceState(null, "", "#sermons");
        if (trash) setTrash(false);
        await open(decodeURIComponent(asked));
        return;
      }
      await refresh();
    },
  };
}
