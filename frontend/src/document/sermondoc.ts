// A sermon, lesson or Bible study in a pane: its passage, kind and last change at the top, its
// outline, then its text in the shared editor. Text changes save on their own (document.update);
// the full builder, with quoting and outline editing, is in My work → Sermons.

import { Message, run } from "../api";
import { button, clear, h } from "../dom";
import type { RichEditor } from "../editor";
import { formatTime } from "../helpers";
import { navigate } from "../state";
import { untitled } from "../sermonwords";
import type { SermonDoc } from "../views/sermons";
import { DocHost, DocView } from "./view";

const SAVE_AFTER = 1500;
const KIND_NAME: Record<string, string> = { sermon: "Sermon", lesson: "Lesson", study: "Bible study" };

export function sermonDoc(host: DocHost): DocView {
  const msg = new Message();
  const body = h("div", { class: "sermon-doc" });
  const el = h("div", { class: "doc doc-sermon" }, msg.el, body);
  let editor: RichEditor | null = null;
  let alive = true;
  let flush: (() => Promise<void>) | null = null;

  async function show(id: string) {
    const d = await msg.attempt(() => run<SermonDoc>("document.get", { id }));
    if (!d || !alive) return;
    const name = d.title || untitled(d.kind);
    if (host.tab().title !== name) host.update({ title: name });
    let timer = 0;
    let dirty = false;
    const status = h("span", { class: "hint save-state", "aria-live": "polite" }, `Saved ${formatTime(d.updated_at)}`);
    // The date preached and theme are edited here too; each saves when it changes.
    const preached = h("input", { type: "date", value: d.preached_on ?? "", class: "fact-input", "aria-label": "Date preached" });
    const theme = h("input", { type: "text", value: d.theme ?? "", placeholder: "Add a theme", class: "fact-input", "aria-label": "Theme", maxlength: "200" });
    const saveFact = async (params: Record<string, string>) => {
      status.textContent = "Saving…";
      const got = await msg.attempt(() => run<SermonDoc>("document.update", { id: d.id, ...params }));
      status.textContent = got ? `Saved ${formatTime(got.updated_at)}` : "Not saved: see the message above";
    };
    preached.addEventListener("change", () => void saveFact({ preached_on: preached.value }));
    theme.addEventListener("change", () => void saveFact({ theme: theme.value }));
    theme.addEventListener("keydown", (e) => { if (e.key === "Enter") theme.blur(); });
    const facts = h("dl", { class: "sermon-facts" },
      h("dt", {}, "Passage"), h("dd", {}, d.passage ? button(d.passage.display, () => host.go(d.passage!.display), { class: "link", title: "Show this passage in the linked panes" }) : "None yet"),
      h("dt", {}, "Kind"), h("dd", {}, KIND_NAME[d.kind] ?? d.kind),
      h("dt", {}, "Preached"), h("dd", {}, preached),
      h("dt", {}, "Theme"), h("dd", {}, theme),
      h("dt", {}, "Changed"), h("dd", {}, formatTime(d.updated_at)));
    const outline = d.points.length === 0 ? null : h("ol", { class: "outline-view", "aria-label": "Outline" }, d.points.map((p) =>
      h("li", { class: `level-${p.level === 2 ? 2 : 1}` }, p.h || "Untitled point",
        p.ref ? [" ", button(p.ref, () => host.go(p.ref!), { class: "link small" })] : null)));
    const edHost = h("div", { class: "editor-host doc-text" });
    clear(body,
      h("div", { class: "row between" }, h("h2", { class: "doc-heading" }, name),
        button("Open in Sermons", () => navigate(`sermons/${d.id}`), { class: "link", title: "Edit the outline, quote verses and more" })),
      facts, outline ? [h("h3", {}, "Outline"), outline] : null,
      h("div", { class: "row between" }, h("h3", {}, "Text"), status), edHost);

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
      const params = { id: d!.id, body: editor.getMarkdown() };
      const got = await msg.attempt(() => run<SermonDoc>("document.update", params));
      if (!got) {
        dirty = true;
        status.textContent = "Not saved: see the message above";
        return;
      }
      status.textContent = `Saved ${formatTime(got.updated_at)}`;
    }
    flush = save;
    const { createEditor } = await import("../editor");
    if (!alive) return;
    editor = createEditor(edHost, { markdown: d.body, placeholder: "Write the text…", label: "Text", onChange: changed });
  }

  const id = host.tab().target;
  if (id) void show(id);
  else clear(body, h("p", { class: "empty" }, "No sermon chosen. Open one from the My work browser."));

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
