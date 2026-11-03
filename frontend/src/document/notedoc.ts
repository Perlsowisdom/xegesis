// A note in a pane, in the shared editor (frontend/src/editor). A new note tab shows the note form,
// starting on the pane's passage; once created it becomes the note itself. Changes save on their
// own after a pause, through note.update.

import { Message, Note, run } from "../api";
import { button, clear, h } from "../dom";
import type { RichEditor } from "../editor";
import { formatTime } from "../helpers";
import { noteForm, noteName } from "../notes";
import { DocHost, DocView } from "./view";

const SAVE_AFTER = 1200;

export function noteDoc(host: DocHost): DocView {
  const msg = new Message();
  const body = h("div", { class: "note-doc" });
  const el = h("div", { class: "doc doc-note" }, msg.el, body);
  let editor: RichEditor | null = null;
  let alive = true;
  let flush: (() => Promise<void>) | null = null;

  async function show(id: string) {
    const n = await msg.attempt(() => run<Note>("note.get", { id }));
    if (!n || !alive) return;
    if (host.tab().title !== noteName(n)) host.update({ title: noteName(n) });
    let timer = 0;
    let dirty = false;
    const status = h("span", { class: "hint save-state", "aria-live": "polite" }, `Saved ${formatTime(n.updated_at)}`);
    const title = h("input", { type: "text", value: n.title, placeholder: "Untitled", "aria-label": "Title", class: "doc-title" });
    const where = n.ref ? button(n.ref.display, () => host.go(n.ref!.display), { class: "link", title: "Show this passage in the linked panes" })
      : n.item_title ? h("span", { class: "hint" }, `On ${n.item_title}`) : null;
    const tags = n.tags.length ? h("span", { class: "hint" }, n.tags.map((t) => `#${t}`).join(" ")) : null;
    const edHost = h("div", { class: "editor-host doc-text" });
    clear(body, h("div", { class: "row wrap note-head" }, title, where, tags, status), edHost);

    const changed = () => {
      dirty = true;
      status.textContent = "Not saved yet";
      clearTimeout(timer);
      timer = window.setTimeout(() => void save(), SAVE_AFTER);
    };
    async function save() {
      clearTimeout(timer);
      if (!dirty || !editor) return;
      dirty = false;
      status.textContent = "Saving…";
      const params = { id: n!.id, title: title.value, body: editor.getMarkdown(), tags: n!.tags };
      const got = await msg.attempt(() => run<Note>("note.update", params));
      if (!got) {
        dirty = true;
        status.textContent = "Not saved: see the message above";
        return;
      }
      status.textContent = `Saved ${formatTime(got.updated_at)}`;
      if (alive && host.tab().title !== noteName(got)) host.update({ title: noteName(got) });
    }
    flush = save;
    title.addEventListener("input", changed);
    const { createEditor } = await import("../editor");
    if (!alive) return;
    editor = createEditor(edHost, { markdown: n.body, placeholder: "Write your note…", label: "Text of the note", onChange: changed });
  }

  function create() {
    clear(body, noteForm(msg, {
      note: null,
      ref: host.ref(),
      onDone(saved) {
        if (!alive) return;
        if (!saved) return clear(body, h("p", { class: "empty" }, "No note was made. Close this tab, or ", button("start again", create, { class: "link" }), "."));
        host.update({ target: saved.id, title: noteName(saved) });
        void show(saved.id);
      },
    }));
  }

  const id = host.tab().target;
  if (id) void show(id);
  else create();

  return {
    el,
    focus: () => editor?.focus(),
    destroy() {
      void flush?.();
      alive = false;
      editor?.destroy();
      editor = null;
    },
  };
}
