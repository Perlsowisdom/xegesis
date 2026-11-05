// A sermon, lesson or Bible study as a page: the note page's layout (title, a slim toolbar, the
// text) with its properties under the title (passage, date preached, theme, kind). Its outline is
// the text's headings, shown by the editor's strip at the right edge as in notes; Ctrl+] and Ctrl+[
// make a heading a sub-point and back. The headings are saved as the outline too (document.update's
// points), for export. Outlines from before are added to the text as headings the first time a
// sermon opens.

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
import { registerDoc } from "./activedoc";
import { DocKind, KIND_NAME, migrateOutline, pointsOf, untitled } from "./sermonwords";
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
  const page = h("article", { class: "doc-page sermon-page", "aria-label": "Sermon" },
    pageHead({ crumbs: [crumbs], status: status.el, extra: [printBtn], menu: (b) => menu(b), menuLabel: "Sermon actions" }),
    msg.el, titleHost, props, edHost);
  const el = h("div", { class: "sermon-layout" }, page);

  let d: SermonDoc | null = null;
  let editor: RichEditor | null = null;
  let title = "";
  let passage: Ref | null = null;
  let timer = 0;
  let dirty = false;
  let saving: Promise<void> | null = null;
  let alive = true;
  let editingTheme = false;

  const tbox = titleBox({ value: "", placeholder: "Untitled sermon", label: "Title", onInput: (v) => { title = v; changed(); }, onEnter: () => editor?.focus() });
  titleHost.append(tbox.el);

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
      onChange: changed, foldKey: got.id,
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
    if (o.fresh) tbox.focus();
    unregister = registerDoc({ kind: "sermon", el, id: () => d?.id ?? null, title: () => title || untitled(d?.kind ?? "sermon"), insert: (md) => editor?.insertMarkdown(md) });
  }
  let unregister = () => {};

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
      unregister();
      window.removeEventListener("beforeunload", onUnload);
      void save().finally(() => {
        editor?.destroy();
        editor = null;
      });
    },
  };
}
