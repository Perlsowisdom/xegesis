// A saved list in a pane: verses move the linked panes, excerpts open their book in a tab beside.
// Editing the list (renaming, reordering, removing) stays in My work → Lists.

import { ListEntry, Message, SavedList, run } from "../api";
import { button, clear, h } from "../dom";
import { navigate } from "../state";
import { KIND_NAME, emptyHint, entryBody } from "../views/lists";
import { DocHost, DocView } from "./view";

export function listDoc(host: DocHost): DocView {
  const msg = new Message();
  const body = h("div", { class: "list-doc" });
  const el = h("div", { class: "doc doc-list" }, msg.el, body);
  let alive = true;

  async function show(id: string) {
    const data = await msg.attempt(() => run<{ list: SavedList; entries: ListEntry[] }>("list.items", { id }));
    if (!data || !alive) return;
    const { list, entries } = data;
    if (host.tab().title !== list.name) host.update({ title: list.name });
    clear(body,
      h("div", { class: "row between" }, h("h2", { class: "doc-heading" }, list.name),
        button("Open in Lists", () => navigate("lists"), { class: "link", title: "Rename, reorder or remove entries" })),
      h("p", { class: "hint" }, `${KIND_NAME[list.kind]} · ${entries.length} ${entries.length === 1 ? "entry" : "entries"}`),
      entries.length === 0 ? h("p", { class: "empty" }, emptyHint(list.kind))
        : h("ol", { class: "entries" }, entries.map((e) => h("li", { class: "entry-row" },
          h("div", { class: "entry-main" }, entryBody(list.kind, e, (ref) => host.go(ref)))))));
  }

  const id = host.tab().target;
  if (id) void show(id);
  else clear(body, h("p", { class: "empty" }, "No list chosen. Open one from the My work browser."));
  return { el, destroy: () => { alive = false; } };
}
