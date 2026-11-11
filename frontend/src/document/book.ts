// A library book in one pane. Reflowable books (EPUB, DOCX, HTML, Markdown, text) are shown one
// section at a time as HTML the core has already sanitised; PDF pages are drawn by pdf.js from bytes
// the core sends (document/pdf.ts). Highlights and notes are anchored to paragraph markers in the
// HTML (or rectangles on a PDF page), copying goes through book.copy so the licence applies, and
// Bible references in the text move the pane's linked Bibles. A tab with no book shows a picker.

import type { Locator } from "./open";
import { DocHost, DocView } from "./view";
import { newCtx } from "./bookctx";
import { destroy, open, wirePaging } from "./bookpaging";
import { wireMarks } from "./bookmarks";
import { wireLinks } from "./booklinks";
import { bookMenu, choose } from "./bookmenu";

export type { Anchor } from "./bookinfo";

export function bookDoc(host: DocHost): DocView {
    const c = newCtx(host);
    wirePaging(c);
    wireLinks(c);
    wireMarks(c);
  const menu = bookMenu(c);

const t = host.tab();
if (t.itemId) void open(c, { itemId: t.itemId, fileId: t.fileId, locator: t.locator as Locator | undefined, section: t.locator ? undefined : t.section, anchor: t.anchor, offset: t.offset });
else choose(c);
if (t.locator || t.anchor) queueMicrotask(() => host.update({ locator: undefined, anchor: undefined, offset: undefined }));

  return {
    el: c.el, placeBox: c.placeBox, menu, focus: () => c.body.focus(),
    markScope: () => (c.book ? { kind: "item", itemId: c.book.item_id, title: c.book.title || "this book", section: c.section, unit: c.book.reflowable ? "section" : "page" } : null),
    destroy() {
      c.unwireMarks();
      destroy(c);
    },
  };
}
