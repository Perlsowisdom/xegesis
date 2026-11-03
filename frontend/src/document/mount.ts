// mountDoc draws a tab's content: the one place that knows which renderer draws which kind of tab.

import { h } from "../dom";
import type { Tab } from "./grid";
import { bibleDoc } from "./bible";
import { bookDoc } from "./book";
import { commentaryDoc } from "./commentary";
import { DocHost, DocView, tabLabel } from "./view";

export function mountDoc(tab: Tab, host: DocHost): DocView {
  switch (tab.kind) {
    case "bible": return bibleDoc(host);
    case "commentary": return commentaryDoc(host);
    case "book": return bookDoc(host);
    default:
      return { el: h("div", { class: "doc empty" }, h("p", {}, `${tabLabel(tab)} cannot be shown in a pane yet.`)), destroy() {} };
  }
}
