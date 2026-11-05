// A note in a pane: the note page (notepage.ts), the same as in My work. A new note tab starts on
// the pane's passage and becomes the note itself with its first change.

import { button, h } from "../dom";
import { noteName } from "../notes";
import { notePage } from "../notepage";
import { navigate } from "../state";
import { DocHost, DocView } from "./view";

export function noteDoc(host: DocHost): DocView {
  const id = host.tab().target;
  const el = h("div", { class: "doc doc-note" });
  const page = notePage({
    id: id || undefined,
    ref: id ? undefined : host.ref(),
    onSaved: (n) => host.update({ target: n.id, title: noteName(n) }),
    onTurned: (d) => host.replace({ kind: "sermon", target: d.id, title: d.title }),
    onDeleted: () => el.replaceChildren(h("p", { class: "empty doc-gone" }, "This note went to the bin. Close the tab, or ",
      button("open the bin", () => navigate("bin"), { class: "link" }), ".")),
    more: [["Show in My work", () => navigate(page.note() ? `notes/note/${page.note()!.id}` : "notes")]],
  });
  el.append(page.el);
  return {
    el,
    focus: () => page.focus(),
    destroy: () => page.destroy(),
  };
}
