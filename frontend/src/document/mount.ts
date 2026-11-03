// mountDoc draws a tab's content: the one place that knows which renderer draws which kind of tab.

import { h } from "../dom";
import type { Tab } from "./grid";
import { bibleDoc } from "./bible";
import { bookDoc } from "./book";
import { libraryBrowser, workBrowser } from "./browsers";
import { commentaryDoc } from "./commentary";
import { listDoc } from "./listdoc";
import { noteDoc } from "./notedoc";
import { sermonDoc } from "./sermondoc";
import { DocHost, DocView, tabLabel } from "./view";

export function mountDoc(tab: Tab, host: DocHost): DocView {
  switch (tab.kind) {
    case "bible": return bibleDoc(host);
    case "commentary": return commentaryDoc(host);
    case "book": return bookDoc(host);
    case "note": return noteDoc(host);
    case "sermon": return sermonDoc(host);
    case "list": return listDoc(host);
    case "library": return libraryBrowser(host);
    case "work": return workBrowser(host);
    default:
      return { el: h("div", { class: "doc empty" }, h("p", {}, `${tabLabel(tab)} cannot be shown in a pane yet.`)), destroy() {} };
  }
}
