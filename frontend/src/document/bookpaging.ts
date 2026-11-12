// Opening a book and moving in it: sections and PDF pages, the saved place, Pages and Scroll layout.

import { run } from "../api";
import { base64ToBytes, concatBytes, tidyPrefs } from "../bookhelpers";
import { bookZoom, currentLayout, LAYOUT_EVENT, ZOOM_EVENT } from "../booklayout";
import { clear, h } from "../dom";
import type { BookCtx } from "./bookctx";
import type { BookInfo, OpenRequest, SectionData, Target } from "./bookinfo";
import { buildSlots, stopPdfScroll, syncScroll } from "./bookscroll";
import { header } from "./bookwhere";
import { loadMarksFor } from "./bookmarks";
import { linkPdfRefs } from "./booklinks";
import { pointAt, secEl } from "./bookpos";

export function scrolling(c: BookCtx) {
  return c.layout === "scroll";
}

// Opening and moving ------------------------------------------------------------------------------

export async function open(c: BookCtx, r: OpenRequest) {
  c.picker.hidden = true;
  c.msg.clear();
  c.copyMsg.clear();
  const b = await c.msg.attempt(() => run<BookInfo>("book.open", { item_id: r.itemId, ...(r.fileId ? { file_id: r.fileId } : {}) }));
  if (!b || !c.alive) return;
  c.book = b;
  c.back.length = 0;
  c.reader.hidden = false;
  c.placeBox.hidden = false;
  const name = b.title || "Untitled";
  if (c.host.tab().title !== name || c.host.tab().fileId !== b.file_id) c.host.update({ title: name, fileId: b.file_id });
  c.byline.textContent = [b.authors.join(", "), b.format.toUpperCase()].filter(Boolean).join(" · ");
  c.attribution.textContent = b.attribution ?? "";
  c.attribution.hidden = !b.attribution;
  c.text.hidden = !b.reflowable;
  c.pdfBox.hidden = b.reflowable;
  c.toc = b.toc;
  if (b.format === "pdf") {
    if (!(await loadPdf(c, b))) return;
  }
  let target: Target = {};
  let start = r.section ?? b.position?.section ?? 1;
  if (r.locator) {
    const loc = await c.msg.attempt(() => run<{ section: number; para?: number; anchor?: string }>("book.locate",
      { item_id: b.item_id, file_id: b.file_id, locator: r.locator }));
    if (loc) {
      start = loc.section;
      target = { anchor: loc.anchor, para: loc.para, flash: true };
    }
  } else if (r.anchor) {
    target = { anchor: r.anchor, offset: r.offset, flash: true };
  } else if (r.para || b.position?.para) {
    target = { para: r.para ?? b.position?.para };
  }
  await show(c, Math.min(Math.max(1, start), b.sections), target);
}

export async function loadPdf(c: BookCtx, b: BookInfo): Promise<boolean> {
  if (c.pdf?.fileId === b.file_id) return true;
  c.pdf?.doc.destroy();
  c.pdf = null;
  c.msg.info("Loading the PDF…");
  const parts: Uint8Array[] = [];
  let offset = 0;
  for (;;) {
    const chunk = await c.msg.attempt(() => run<{ data: string; eof: boolean; size: number }>("book.file", { item_id: b.item_id, file_id: b.file_id, offset }));
    if (!chunk) return false;
    const bytes = base64ToBytes(chunk.data);
    parts.push(bytes);
    offset += bytes.length;
    if (chunk.eof || bytes.length === 0) break;
    c.msg.info(`Loading the PDF… ${Math.round((offset / chunk.size) * 100)}%`);
  }
  const ok = await c.msg.attempt(async () => {
    const view = await import("./pdf");
    const doc = await view.openPdf(concatBytes(parts));
    c.pdf = { fileId: b.file_id, doc };
    const items = await view.outline(doc);
    c.toc = items.map((o) => ({ title: o.title, section: o.page, level: o.level }));
    return true;
  });
  c.msg.clear();
  return !!ok;
}

export function go(c: BookCtx, n: number, target: Target = {}) {
  if (!c.book || n < 1 || n > c.book.sections) return;
  show(c, n, target);
}

// jump moves somewhere else in the book, remembering where the user was.
export function jump(c: BookCtx, n: number, target: Target) {
  c.back.push({ section: c.section, para: visiblePara(c) });
  c.back.splice(0, c.back.length - 30);
  go(c, n, target);
}

export async function fetchSection(c: BookCtx, b: BookInfo, n: number): Promise<HTMLElement | null> {
  const s = await c.msg.attempt(() => run<SectionData>("book.section", { item_id: b.item_id, file_id: b.file_id, section: n }));
  if (!s) return null;
  // The core sanitised this HTML: no scripts, styles, event attributes or remote resources.
  const sec = h("div", { class: "book-sec", "data-sec": String(n), ...(s.cfi ? { "data-cfi": s.cfi } : {}) });
  sec.innerHTML = s.html || `<p class="empty">This section has no text.</p>`;
  return sec;
}

export async function show(c: BookCtx, n: number, target: Target) {
  if (!c.book) return;
  const b = c.book;
  const seq = ++c.loadSeq;
  header(c, n);
  c.store.clear();
  stopPdfScroll(c);
  c.pdfBox.classList.toggle("pdf-scroll", !b.reflowable && scrolling(c));
  if (!b.reflowable) c.lastWidth = c.body.clientWidth; // drawn at this width: no redraw for it
  if (b.reflowable) {
    const sec = await fetchSection(c, b, n);
    if (!sec || seq !== c.loadSeq || !c.alive) return;
    c.text.replaceChildren(sec);
    c.body.scrollTop = 0;
    await loadMarksFor(c, n);
  } else if (scrolling(c)) {
    await buildSlots(c, n, seq);
  } else {
    await drawPage(c, n);
    if (seq !== c.loadSeq) return;
    await loadMarksFor(c, n);
  }
  if (seq !== c.loadSeq) return;
  scrollTo(c, target);
  savePosition(c);
  if (b.reflowable && scrolling(c)) syncScroll(c);
}

export async function drawPage(c: BookCtx, n: number) {
  if (!c.pdf) return;
  c.page?.cancel();
  const doc = c.pdf.doc;
  const width = Math.max(200, c.body.clientWidth - 24);
  const r = await c.msg.attempt(async () => {
    const view = await import("./pdf");
    return view.renderPage(doc, n, width, c.prefs.zoom);
  });
  if (!r) return;
  c.page = r;
  clear(c.pdfBox, r.page);
  c.body.scrollTop = 0;
  linkPdfRefs(r.page);
}

export function scrollTo(c: BookCtx, t: Target) {
  const id = t.anchor || (t.para ? `bp-${t.para}` : "");
  if (c.book && !c.book.reflowable && scrolling(c)) {
    const sl = c.slots.get(c.section);
    if (sl) c.body.scrollTop = sl.offsetTop - 8;
    return;
  }
  const node = id ? secEl(c, c.section)?.querySelector<HTMLElement>(`[id="${CSS.escape(id)}"]`) : null;
  if (!node || !c.book?.reflowable) return;
  let top = node.getBoundingClientRect().top;
  const root = secEl(c, c.section);
  if (t.offset && root && node.dataset.p) {
    const pt = pointAt(root, Number(node.dataset.p), t.offset);
    if (pt) {
      const r = document.createRange();
      r.setStart(pt.node, pt.offset);
      r.collapse(true);
      const rc = r.getBoundingClientRect();
      if (rc.height > 0) top = rc.top;
    }
  }
  c.body.scrollTop += top - c.body.getBoundingClientRect().top - 8;
  if (t.flash) {
    const block = node.closest("p,li,div,blockquote,td,h1,h2,h3,h4,h5,h6") ?? node.parentElement;
    block?.classList.add("jump-flash");
    setTimeout(() => block?.classList.remove("jump-flash"), 2500);
  }
}

// visiblePara is the paragraph at the top of the view.
export function visiblePara(c: BookCtx): number {
  if (!c.book?.reflowable) return 0;
  const top = c.body.getBoundingClientRect().top;
  let para = 0;
  for (const m of secEl(c, c.section)?.querySelectorAll<HTMLElement>("span.bp") ?? []) {
    if (m.getBoundingClientRect().top > top + 4) break;
    para = Number(m.dataset.p);
  }
  return para || 1;
}

export function savePosition(c: BookCtx) {
  clearTimeout(c.saveTimer);
  c.saveTimer = window.setTimeout(() => {
    if (!c.book) return;
    run("book.set_position", { item_id: c.book.item_id, file_id: c.book.file_id, section: c.section, para: visiblePara(c) }).catch(() => {});
  }, 1500);
}

// destroy frees the pane's book: a PDF's pages and the document pdf.js holds go with it.
export function destroy(c: BookCtx) {
  if (c.book) run("book.set_position", { item_id: c.book.item_id, file_id: c.book.file_id, section: c.section, para: visiblePara(c) }).catch(() => {});
  c.alive = false;
  c.loadSeq++;
  clearTimeout(c.saveTimer);
  c.unwirePaging();
  cancelAnimationFrame(c.syncFrame);
  stopPdfScroll(c);
  c.book = null;
  c.page?.cancel();
  c.page = null;
  void c.pdf?.doc.destroy();
  c.pdf = null;
  clear(c.text);
  clear(c.pdfBox);
}

// wirePaging listens for scrolling, keys, the Pages / Scroll choice, the pane's width and the PDF zoom.
export function wirePaging(c: BookCtx) {
  c.body.addEventListener("scroll", () => {
    savePosition(c);
    if (!scrolling(c) || c.syncFrame) return;
    c.syncFrame = requestAnimationFrame(() => {
      c.syncFrame = 0;
      syncScroll(c);
    });
  }, { passive: true });
  // Space and the page keys move by a screen in Scroll mode; the arrow keys scroll natively.
  c.body.addEventListener("keydown", (e) => {
    if (!scrolling(c) || e.target !== c.body || e.altKey || e.ctrlKey || e.metaKey) return;
    const dir = e.key === "PageDown" || (e.key === " " && !e.shiftKey) ? 1 : e.key === "PageUp" || (e.key === " " && e.shiftKey) ? -1 : 0;
    if (!dir) return;
    e.preventDefault();
    c.body.scrollBy({ top: dir * c.body.clientHeight * 0.9 });
  });
  // The Pages / Scroll choice changed (in the Aa popover, or this pane's own setting).
  const onLayout = () => {
    const l = currentLayout();
    if (!c.alive || l === c.layout) return;
    c.layout = l;
    if (c.book) void show(c, c.section, { para: c.book.reflowable ? visiblePara(c) : 0 });
  };
  document.addEventListener(LAYOUT_EVENT, onLayout);
  const resizer = new ResizeObserver(() => {
    const w = c.body.clientWidth;
    if (!c.book || c.book.reflowable || !c.pdf || Math.abs(w - c.lastWidth) < 24) return;
    c.lastWidth = w;
    clearTimeout(c.resizeTimer);
    c.resizeTimer = window.setTimeout(() => c.alive && show(c, c.section, {}), 300);
  });
  resizer.observe(c.body);
  // The PDF zoom is changed in the Aa popover.
  const onZoom = () => {
    const z = bookZoom();
    if (!c.alive || z === c.prefs.zoom) return;
    c.prefs = tidyPrefs({ ...c.prefs, zoom: z });
    if (c.book && !c.book.reflowable) void show(c, c.section, {});
  };
  document.addEventListener(ZOOM_EVENT, onZoom);
  c.unwirePaging = () => {
    resizer.disconnect();
    document.removeEventListener(LAYOUT_EVENT, onLayout);
    document.removeEventListener(ZOOM_EVENT, onZoom);
  };
}
