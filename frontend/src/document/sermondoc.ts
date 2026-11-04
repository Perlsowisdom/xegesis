// A sermon, lesson or Bible study in a pane: the sermon page (sermonpage.ts), the same as in My
// work. In a narrow pane the outline column folds away; the headings are still the outline.

import { button, h } from "../dom";
import { sermonPage } from "../sermonpage";
import { untitled } from "../sermonwords";
import { navigate } from "../state";
import { DocHost, DocView } from "./view";

export function sermonDoc(host: DocHost): DocView {
  const id = host.tab().target;
  const el = h("div", { class: "doc doc-sermon" });
  if (!id) {
    el.append(h("p", { class: "empty" }, "No sermon chosen. Open one from the My work browser."));
    return { el, destroy() {} };
  }
  const page = sermonPage({
    id,
    onSaved: (d) => {
      const name = d.title || untitled(d.kind);
      if (host.tab().title !== name) host.update({ title: name });
    },
    onDeleted: () => el.replaceChildren(h("p", { class: "empty doc-gone" }, "This sermon went to the bin. Close the tab, or ",
      button("open the bin", () => navigate("bin"), { class: "link" }), ".")),
    more: [["Show in My work", () => navigate(`sermons/${id}`)]],
  });
  el.append(page.el);
  return { el, focus: () => page.focus(), destroy: () => page.destroy() };
}
