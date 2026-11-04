// A sermon, lesson or Bible study as a page: the note page's layout (title, a slim toolbar, the
// text) with its properties under the title (passage, date preached, theme, kind) and, in a narrow
// column at the left, its outline. The outline is the text's headings: clicking a point goes to it,
// dragging a point moves its whole section, Ctrl+] and Ctrl+[ make it a sub-point and back, and +
// adds one. The headings are saved as the outline too (document.update's points), for export.
// Outlines from before are added to the text as headings the first time a sermon opens.

import { Message, Ref, run } from "./api";
import { button, clear, h } from "./dom";
import {
  actionsMenu, calendarPopover, docChip, pageHead, quotePopover, refPopover, saveIndicator, titleBox, verseRefPopover,
} from "./docpage";
import { preachedChip } from "./docwords";
import type { RichEditor } from "./editor";
import { parseMarkdown } from "./editor/markdown";
import { icon } from "./icons";
import { openPopover } from "./popover";
import { openExport } from "./printexport";
import { DocKind, KIND_NAME, migrateOutline, outlineOf, pointsOf, untitled } from "./sermonwords";
import { outlineMarks } from "./editor/structurewords";
import type { SermonDoc } from "./views/sermons";
import { folderName, loadFolders, moveMenu, workChanged } from "./work";
import { deleteSermon } from "./views/notes";
import { editorInserts } from "./insertpop";

const SAVE_AFTER = 1000;

export interface SermonPageOptions {
  id: string;
  home?: Node; // the way back, as the first crumb
  fresh?: boolean; // just made: the title takes the keyboard
  more?: [string, () => void][];
  onSaved?(d: SermonDoc): void;
  onDeleted?(): void;
}

export interface SermonPage {
  el: HTMLElement;
  focus(): void;
  flush(): Promise<void>;
  destroy(): void;
  doc(): SermonDoc | null;
}

export function sermonPage(o: SermonPageOptions): SermonPage {
  const msg = new Message();
  const status = saveIndicator();
  const crumbs = h("span", { class: "crumb-path" });
  const printBtn = button("Print or save…", () => { if (d) openExport({ title: d.title || untitled(d.kind), sources: [{ kind: "document", id: d.id }] }); },
    { class: "ghost-btn text", title: "Print, or save as PDF, Word, a web page or Markdown" });
  const titleHost = h("div", {});
  const props = h("dl", { class: "page-props" });
  const edHost = h("div", { class: "page-text", "aria-busy": "true" });
  const outlineList = h("ol", { class: "outline-list", "aria-label": "Points" });
  const addPoint = h("button", { type: "button", class: "ghost-btn", "aria-label": "Add a point", title: "Add a point" }, icon("plus"));
  const aside = h("aside", { class: "outline-col", "aria-label": "Outline" },
    h("div", { class: "outline-head" }, h("span", { class: "outline-title" }, "Outline"), addPoint),
    outlineList,
    h("p", { class: "outline-hint" }, "The outline is the headings in the text. Drag a point to move its section; Ctrl+] and Ctrl+[ make it a sub-point and back."));
  const page = h("article", { class: "doc-page sermon-page", "aria-label": "Sermon" },
    pageHead({ crumbs: [crumbs], status: status.el, extra: [printBtn], menu: (b) => menu(b), menuLabel: "Sermon actions" }),
    msg.el, titleHost, props, edHost);
  const el = h("div", { class: "sermon-layout" }, aside, h("div", { class: "sermon-main" }, page));

  let d: SermonDoc | null = null;
  let editor: RichEditor | null = null;
  let title = "";
  let passage: Ref | null = null;
  let timer = 0;
  let dirty = false;
  let saving: Promise<void> | null = null;
  let alive = true;
  let editingTheme = false;
  let dragKey = "";

  const tbox = titleBox({ value: "", placeholder: "Untitled sermon", label: "Title", onInput: (v) => { title = v; changed(); }, onEnter: () => editor?.focus() });
  titleHost.append(tbox.el);
  addPoint.addEventListener("click", () => editor?.addHeading(2));

  function renderCrumbs() {
    const sep = () => h("span", { class: "crumb-sep", "aria-hidden": "true" }, "›");
    const where = h("span", { class: "crumb" }, d?.folder_id ? folderName(d.folder_id) || "Folder" : "Sermons");
    clear(crumbs, o.home ?? h("span", { class: "crumb" }, "My work"), sep(), where);
  }

  // Properties ------------------------------------------------------------------------------------

  function renderProps() {
    if (!d) return;
    const doc = d;
    const row = (label: string, ...value: Node[]) => [h("dt", {}, label), h("dd", {}, value)];
    const passageChip = passage
      ? docChip({ kind: "verse", label: [h("span", { class: "star", "aria-hidden": "true" }, "✦"), passage.display], title: "Change the passage", onClick: editPassage })
      : docChip({ kind: "add", label: "+ passage", title: "Choose the main passage", onClick: editPassage });
    const preached = docChip({ kind: doc.preached_on ? "plain" : "add", label: [icon("calendar"), preachedChip(doc.preached_on)], title: "When it was preached or taught",
      onClick: (b) => calendarPopover(b, { value: doc.preached_on ?? "", noneText: "Not preached yet", onPick: (iso) => setFact({ preached_on: iso }) }) });
    let theme: Node;
    if (editingTheme) {
      const input = h("input", { type: "text", class: "inline-chip", value: doc.theme ?? "", maxlength: "200", placeholder: "e.g. Assurance", "aria-label": "Theme" });
      let done = false;
      const finish = (keep: boolean) => {
        if (done) return;
        done = true;
        editingTheme = false;
        if (keep && input.value.trim() !== (doc.theme ?? "")) setFact({ theme: input.value.trim() });
        else renderProps();
        props.querySelector<HTMLElement>("[data-prop=theme]")?.focus();
      };
      input.addEventListener("keydown", (e) => {
        if (e.key === "Enter") { e.preventDefault(); finish(true); }
        if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); finish(false); }
      });
      input.addEventListener("blur", () => finish(true));
      theme = input;
      queueMicrotask(() => input.select());
    } else {
      theme = docChip({ kind: doc.theme ? "plain" : "add", label: doc.theme || "+ theme", title: "The theme or topic, in a few words",
        onClick: () => { editingTheme = true; renderProps(); } });
      (theme as HTMLElement).dataset.prop = "theme";
    }
    const kind = docChip({ kind: "plain", label: [KIND_NAME[doc.kind] ?? "Sermon", icon("chevron")], title: "Sermon, lesson or Bible study",
      onClick: (b) => openPopover(b, h("div", { class: "menu-list kind-menu" }, (["sermon", "lesson", "study"] as DocKind[]).map((k) =>
        button(KIND_NAME[k], () => { if (k !== doc.kind) setFact({ kind: k }); }, { class: k === doc.kind ? "on" : "", "aria-checked": String(k === doc.kind) }))),
      { label: "Kind", menu: true }) });
    clear(props, row("Passage", passageChip), row("Preached", preached), row("Theme", theme), row("Kind", kind));
  }

  function editPassage(b: HTMLElement) {
    refPopover(b, {
      title: "Main passage", value: passage?.display ?? "", action: passage ? "Change" : "Choose",
      onSave: (r) => { passage = r; setFact({ passage: r.display }); },
      onRemove: passage ? () => { passage = null; setFact({ passage: "" }); } : undefined, removeText: "No main passage",
    });
  }

  // setFact saves one property at once, with whatever else is waiting.
  function setFact(p: Record<string, string>) {
    if (!d) return;
    Object.assign(d, p);
    if (p.kind) tbox.el.dataset.placeholder = untitled(p.kind as DocKind);
    renderProps();
    dirty = true;
    void save();
  }

  // Saving ----------------------------------------------------------------------------------------

  function changed() {
    dirty = true;
    status.set("dirty");
    clearTimeout(timer);
    timer = window.setTimeout(() => void save(), SAVE_AFTER);
  }

  async function save(): Promise<void> {
    clearTimeout(timer);
    if (saving) await saving;
    if (!dirty || !d) return;
    dirty = false;
    status.set("saving");
    const doc = d;
    const body = editor?.getMarkdown() ?? doc.body;
    saving = (async () => {
      const before = doc.title;
      const got = await msg.attempt(() => run<SermonDoc>("document.update", {
        id: doc.id, title, kind: doc.kind, passage: passage?.display ?? "", preached_on: doc.preached_on ?? "", theme: doc.theme ?? "",
        body, points: pointsOf(parseMarkdown(body)),
      }));
      if (!got) {
        dirty = true;
        status.set("error");
        return;
      }
      Object.assign(doc, got);
      status.set("saved", got.updated_at);
      if (got.title !== before) workChanged();
      o.onSaved?.(got);
    })();
    await saving;
    saving = null;
  }

  // The outline -----------------------------------------------------------------------------------

  function renderOutline() {
    if (!editor) return;
    const heads = editor.headings();
    const points = outlineOf(parseMarkdown(editor.getMarkdown()));
    const here = editor.caretHeading();
    const marks = outlineMarks(heads.map((x) => x.level));
    clear(outlineList, heads.length === 0
      ? h("li", { class: "outline-empty" }, "No points yet. Add a heading to the text, or choose +.")
      : heads.map((hd, i) => {
        const { depth, label } = marks[i];
        const refs = points[i]?.refs.map((r) => r.label).join("; ") ?? "";
        const b = h("button", { type: "button", class: `outline-point depth-${depth}${hd.key === here ? " on" : ""}`, draggable: "true",
          "data-key": hd.key, "data-level": String(hd.level), "aria-current": hd.key === here ? "true" : null,
          title: `${["Large", "Medium", "Small"][hd.level - 1] ?? "Small"} heading. Go to this point; drag to move its section.` },
        h("span", { class: "outline-num", "aria-hidden": "true" }, label),
        h("span", { class: "outline-label" }, hd.text.replace(/^\d{1,3}[.)]\s+/, "") || h("span", { class: "untitled" }, "Untitled point")),
        refs ? h("span", { class: "outline-ref" }, refs) : null);
        b.addEventListener("click", () => editor?.revealHeading(hd.key));
        b.addEventListener("keydown", (e) => pointKey(e, hd.key, i));
        b.addEventListener("dragstart", (e) => {
          dragKey = hd.key;
          e.dataTransfer?.setData("text/plain", hd.text);
          if (e.dataTransfer) e.dataTransfer.effectAllowed = "move";
          b.classList.add("dragging");
        });
        b.addEventListener("dragend", () => { dragKey = ""; b.classList.remove("dragging"); clearDrop(); });
        b.addEventListener("dragover", (e) => {
          if (!dragKey) return;
          e.preventDefault();
          clearDrop();
          const r = b.getBoundingClientRect();
          b.classList.add(e.clientY < r.top + r.height / 2 ? "drop-before" : "drop-after");
        });
        b.addEventListener("drop", (e) => {
          if (!dragKey) return;
          e.preventDefault();
          const after = b.classList.contains("drop-after");
          clearDrop();
          // Dropping after a point puts the section after that point's whole section.
          const target = after ? nextSectionStart(i) : hd.key;
          if (dragKey !== hd.key) editor?.moveSection(dragKey, target);
          dragKey = "";
        });
        return h("li", {}, b);
      }));
  }

  // nextSectionStart is the heading after the section of the i-th heading, or null at the end.
  function nextSectionStart(i: number): string | null {
    const heads = editor?.headings() ?? [];
    for (let j = i + 1; j < heads.length; j++) if (heads[j].level <= heads[i].level) return heads[j].key;
    return null;
  }

  function clearDrop() {
    for (const x of outlineList.querySelectorAll(".drop-before, .drop-after")) x.classList.remove("drop-before", "drop-after");
  }

  function pointKey(e: KeyboardEvent, key: string, i: number) {
    const heads = editor?.headings() ?? [];
    const cur = heads[i];
    const mod = e.ctrlKey || e.metaKey;
    const refocus = () => queueMicrotask(() => outlineList.querySelector<HTMLElement>(`[data-key="${editor?.headings().find((x) => x.text === cur.text)?.key ?? key}"]`)?.focus());
    if (mod && (e.code === "BracketRight" || e.code === "BracketLeft")) {
      e.preventDefault();
      editor?.setHeadingLevel(key, Math.min(3, Math.max(1, cur.level + (e.code === "BracketRight" ? 1 : -1))));
      refocus();
    } else if (e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
      // Alt+↑ and Alt+↓ move the point's section, the keyboard way to drag it.
      e.preventDefault();
      if (e.key === "ArrowUp") {
        let j = i - 1;
        while (j >= 0 && heads[j].level > cur.level) j--;
        if (j >= 0) editor?.moveSection(key, heads[j].key);
      } else {
        const after = nextSectionStart(i);
        const k = after ? heads.findIndex((x) => x.key === after) : -1;
        if (k >= 0) editor?.moveSection(key, nextSectionStart(k));
      }
      refocus();
    } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const all = [...outlineList.querySelectorAll<HTMLElement>(".outline-point")];
      all[Math.max(0, Math.min(all.length - 1, i + (e.key === "ArrowDown" ? 1 : -1)))]?.focus();
    }
  }

  // The menu --------------------------------------------------------------------------------------

  async function moveTo(b: HTMLElement) {
    if (!d) return;
    await loadFolders();
    moveMenu(b, msg, { kind: "sermon", id: d.id, title: d.title || untitled(d.kind), folder_id: d.folder_id });
  }

  function menu(b: HTMLElement) {
    actionsMenu(b, "Sermon actions", [
      ...(o.more ?? []),
      ["Move to folder…", () => void moveTo(b)],
      ["Print or export…", () => printBtn.click()],
      "-",
      ["Delete", () => void remove(), "danger"],
    ]);
  }

  async function remove() {
    if (!d) return;
    clearTimeout(timer);
    await save();
    dirty = false;
    await deleteSermon(msg, d);
    o.onDeleted?.();
  }

  // Opening ---------------------------------------------------------------------------------------

  async function start() {
    await loadFolders().catch(() => []);
    const got = await msg.attempt(() => run<SermonDoc>("document.get", { id: o.id }));
    if (!got || !alive) return;
    d = got;
    title = got.title === untitled(got.kind) && o.fresh ? "" : got.title;
    passage = got.passage ? { display: got.passage.display, osis: got.passage.osis, start: got.passage.start, end: got.passage.end } : null;
    tbox.el.textContent = title;
    tbox.el.toggleAttribute("data-empty", !title);
    tbox.el.dataset.placeholder = untitled(got.kind);
    status.set("saved", got.updated_at);
    renderCrumbs();
    renderProps();
    // An outline from before becomes headings in the text, once.
    let body = got.body;
    const missing = migrateOutline(got.points, parseMarkdown(body));
    if (missing) body = body.trim() ? `${body.trimEnd()}\n\n${missing}` : missing;
    const { createEditor } = await import("./editor");
    if (!alive) return;
    edHost.removeAttribute("aria-busy");
    editor = createEditor(edHost, {
      markdown: got.body, placeholder: "Write here… Headings become the outline. Type / for more.", label: "Text of the sermon or lesson",
      onChange: changed, onHeadings: renderOutline, foldKey: got.id,
      onVerseRef: verseRefPopover,
      ...editorInserts({
        say: (w) => msg.info(w), passage: () => passage?.display ?? "", self: () => d?.id,
        quote: (anchor) => quotePopover(anchor, passage?.display ?? "", (md) => editor?.insertMarkdown(md)),
      }),
    });
    if (missing) {
      editor.setMarkdown(body);
      changed();
    }
    renderOutline();
    if (o.fresh) tbox.focus();
  }

  // The current point follows the caret.
  const onSel = () => {
    if (!editor || !el.contains(document.activeElement)) return;
    const here = editor.caretHeading();
    for (const b of outlineList.querySelectorAll<HTMLElement>(".outline-point")) {
      const on = b.dataset.key === here;
      b.classList.toggle("on", on);
      if (on) b.setAttribute("aria-current", "true");
      else b.removeAttribute("aria-current");
    }
  };
  document.addEventListener("selectionchange", onSel);
  const onUnload = () => void save();
  window.addEventListener("beforeunload", onUnload);
  el.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
      e.preventDefault();
      dirty = true;
      void save();
    }
  });
  renderCrumbs();
  void start();

  return {
    el,
    doc: () => d,
    focus: () => (title ? editor?.focus() : tbox.focus()),
    flush: save,
    destroy() {
      alive = false;
      document.removeEventListener("selectionchange", onSel);
      window.removeEventListener("beforeunload", onUnload);
      void save().finally(() => {
        editor?.destroy();
        editor = null;
      });
    },
  };
}
