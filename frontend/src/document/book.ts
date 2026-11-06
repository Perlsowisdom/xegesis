// A library book in one pane. Reflowable books (EPUB, DOCX, HTML, Markdown, text) are shown one
// section at a time as HTML the core has already sanitised; PDF pages are drawn by pdf.js from bytes
// the core sends (document/pdf.ts). Highlights and notes are anchored to paragraph markers in the
// HTML (or rectangles on a PDF page), copying goes through book.copy so the licence applies, and
// Bible references in the text move the pane's linked Bibles. A tab with no book shows a picker.

import { openExport } from "../printexport";
import { ItemSummary, Message, Note, Ref, run } from "../api";
import {
  CopyResult, DEFAULT_PREFS, ReadingPrefs, TOCEntry, base64ToBytes, concatBytes, copyMessage, currentEntry,
  pageAtOffset, pageRects, renderPlan, tidyPrefs, tocOptions,
} from "../bookhelpers";
import { LAYOUT_EVENT, ZOOM_EVENT, bookZoom, currentLayout } from "../booklayout";
import { button, clear, h } from "../dom";
import { sidebar } from "../views/sidebar";
import { addToList } from "../listpicker";
import type { PdfDoc, RenderedPage } from "./pdf";
import { closePopover, togglePopover } from "../popover";
import { markSegments } from "../helpers";
import type { Locator } from "./open";
import { attachContextMenu } from "./contextmenu";
import { contentsPopover } from "./contentspop";
import { lookUpCard } from "../lookup";
import { attachMarking } from "../marking";
import { hlClasses, styleOf } from "../markingwords";
import { DocHost, DocView } from "./view";

interface BookInfo {
  item_id: string;
  file_id: string;
  title: string;
  authors: string[];
  format: string;
  reflowable: boolean;
  sections: number;
  toc: TOCEntry[];
  licence: string;
  attribution?: string;
  position?: { section: number; para?: number } | null;
}

interface SectionData {
  section: number;
  sections: number;
  title?: string;
  html?: string;
  cfi?: string;
}

export interface Anchor {
  section: number;
  cfi?: string;
  para?: number;
  offset?: number;
  end_para?: number;
  end_offset?: number;
  page?: number;
  rects?: [number, number, number, number][];
  text?: string;
}

interface Highlight {
  id: string;
  locator?: Anchor;
  style: string;
}

type BookNote = Note & { item_id?: string; locator?: Anchor };

interface Target {
  anchor?: string;
  para?: number;
  offset?: number;
  flash?: boolean;
}

const PREFS_KEY = "xegesis.book.prefs";

function loadPrefs(): ReadingPrefs {
  try {
    return tidyPrefs(JSON.parse(localStorage.getItem(PREFS_KEY) ?? "null"));
  } catch {
    return { ...DEFAULT_PREFS };
  }
}

interface OpenRequest {
  itemId: string;
  fileId?: string;
  locator?: Locator;
  section?: number;
  para?: number;
  anchor?: string;
  offset?: number;
}

export function bookDoc(host: DocHost): DocView {
  const msg = new Message();
  const copyMsg = new Message();
  let prefs = loadPrefs();
  let book: BookInfo | null = null;
  let section = 1;
  let toc: TOCEntry[] = [];
  let pdf: { fileId: string; doc: PdfDoc } | null = null;
  let page: RenderedPage | null = null;
  // Highlights and notes by section (or PDF page): one entry in Pages mode, every loaded one in Scroll.
  const store = new Map<number, { hs: Highlight[]; ns: BookNote[] }>();
  let layout = currentLayout();
  const scrolling = () => layout === "scroll";
  let growing = false;
  let syncFrame = 0;
  // PDF Scroll mode: a slot for every page, drawn only while near the view.
  const slots = new Map<number, HTMLElement>();
  const drawn = new Map<number, RenderedPage>();
  const pending = new Set<number>();
  const near = new Set<number>();
  let observer: IntersectionObserver | null = null;
  let planTimer = 0;
  let pdfSeq = 0;
  const back: { section: number; para: number }[] = [];
  let saveTimer = 0;
  let loadSeq = 0;
  let alive = true;

  const byline = h("p", { class: "hint book-byline" });
  let tocAt = -1; // the contents entry the reader is in
  const prev = button("‹", () => go(section - 1), { "aria-label": "Previous section", title: "Previous section (Alt+Page Up)", "aria-keyshortcuts": "Alt+PageUp" });
  const next = button("›", () => go(section + 1), { "aria-label": "Next section", title: "Next section (Alt+Page Down)", "aria-keyshortcuts": "Alt+PageDown" });
  // The place is a field: type a number and press Enter, or words to find a heading in the contents.
  const unitText = h("span", { class: "book-unit" }, "Section");
  const whereInput = h("input", { type: "text", class: "book-where-input", autocomplete: "off", spellcheck: "false",
    "aria-label": "Go to a section number, or words from a heading", title: "Type a number, or words from a heading, then press Enter" });
  const ofText = h("span", { class: "book-of" });
  const where = h("span", { class: "hint book-where" }, unitText, whereInput, ofText);
  const placeBox = h("div", { class: "book-nav", role: "group", "aria-label": "Place in the book", hidden: true }, prev, where, next);
  const text = h("article", { class: "book-text doc-text", lang: "" });
  const pdfBox = h("div", { class: "pdf-box doc-text" });
  const body = h("div", { class: "book-body", tabindex: "0", role: "document", "aria-label": "Book text" }, text, pdfBox);
  const attribution = h("footer", { class: "attribution" });
  const editor = h("div", {});
  const picker = h("div", { class: "card book-picker", hidden: true });

  function print() {
    if (!book) return;
    const r = selection();
    const where = book.reflowable ? `section ${section}` : `page ${section}`;
    openExport(r
      ? { title: book.title, sources: [{ kind: "book", item_id: book.item_id, file_id: book.file_id, text: r.toString() }] }
      : { title: `${book.title}, ${where}`, sources: [{ kind: "book", item_id: book.item_id, file_id: book.file_id, from: section, to: section }] });
  }

  const reader = h("div", { class: "book-reader", hidden: true }, byline, editor, copyMsg.el, body, attribution);
  const el = h("div", { class: "doc doc-book" }, picker, msg.el, reader);

  // Choosing a book ---------------------------------------------------------------------------------

  function choose() {
    const q = h("input", { type: "search", placeholder: "Title", autocomplete: "off", "aria-label": "Find a book by title" });
    const list = h("ul", { class: "results" });
    const pmsg = new Message();
    async function find() {
      const r = await pmsg.attempt(() => run<{ items: ItemSummary[] }>("library.items", { text: q.value.trim(), limit: 30 }));
      if (!r || !alive) return;
      const books = r.items.filter((it) => it.kind !== "bible");
      clear(list, books.length === 0 ? h("li", { class: "empty" }, "No books found.") : books.map((it) =>
        h("li", {}, button(it.title || "Untitled", () => {
          picker.hidden = true;
          host.update({ itemId: it.id, title: it.title || "Untitled" });
          void open({ itemId: it.id });
        }, { class: "link", disabled: it.status !== "online" }),
        it.authors.length ? h("span", { class: "hint" }, ` ${it.authors.join(", ")}`) : null,
        it.status !== "online" ? h("span", { class: "badge warn" }, it.status) : null)));
    }
    const form = h("form", { role: "search", class: "row" }, q, h("button", { type: "submit", class: "primary" }, "Find"));
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      void find();
    });
    clear(picker, h("h3", {}, "Open a book"), form, pmsg.el, list);
    picker.hidden = false;
    queueMicrotask(() => q.focus());
    void find();
  }

  // Opening and moving ------------------------------------------------------------------------------

  async function open(r: OpenRequest) {
    picker.hidden = true;
    msg.clear();
    copyMsg.clear();
    const b = await msg.attempt(() => run<BookInfo>("book.open", { item_id: r.itemId, ...(r.fileId ? { file_id: r.fileId } : {}) }));
    if (!b || !alive) return;
    book = b;
    back.length = 0;
    reader.hidden = false;
    placeBox.hidden = false;
    const name = b.title || "Untitled";
    if (host.tab().title !== name || host.tab().fileId !== b.file_id) host.update({ title: name, fileId: b.file_id });
    byline.textContent = [b.authors.join(", "), b.format.toUpperCase()].filter(Boolean).join(" · ");
    attribution.textContent = b.attribution ?? "";
    attribution.hidden = !b.attribution;
    text.hidden = !b.reflowable;
    pdfBox.hidden = b.reflowable;
    toc = b.toc;
    if (b.format === "pdf") {
      if (!(await loadPdf(b))) return;
    }
    let target: Target = {};
    let start = r.section ?? b.position?.section ?? 1;
    if (r.locator) {
      const loc = await msg.attempt(() => run<{ section: number; para?: number; anchor?: string }>("book.locate",
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
    await show(Math.min(Math.max(1, start), b.sections), target);
  }

  async function loadPdf(b: BookInfo): Promise<boolean> {
    if (pdf?.fileId === b.file_id) return true;
    pdf?.doc.destroy();
    pdf = null;
    msg.info("Loading the PDF…");
    const parts: Uint8Array[] = [];
    let offset = 0;
    for (;;) {
      const c = await msg.attempt(() => run<{ data: string; eof: boolean; size: number }>("book.file", { item_id: b.item_id, file_id: b.file_id, offset }));
      if (!c) return false;
      const bytes = base64ToBytes(c.data);
      parts.push(bytes);
      offset += bytes.length;
      if (c.eof || bytes.length === 0) break;
      msg.info(`Loading the PDF… ${Math.round((offset / c.size) * 100)}%`);
    }
    const ok = await msg.attempt(async () => {
      const view = await import("./pdf");
      const doc = await view.openPdf(concatBytes(parts));
      pdf = { fileId: b.file_id, doc };
      const items = await view.outline(doc);
      toc = items.map((o) => ({ title: o.title, section: o.page, level: o.level }));
      return true;
    });
    msg.clear();
    return !!ok;
  }

  function go(n: number, target: Target = {}) {
    if (!book || n < 1 || n > book.sections) return;
    show(n, target);
  }

  // jump moves somewhere else in the book, remembering where the user was.
  function jump(n: number, target: Target) {
    back.push({ section, para: visiblePara() });
    back.splice(0, back.length - 30);
    go(n, target);
  }


  // typedPlace handles Enter in the place field: a number goes there, words list the matching headings.
  function typedPlace() {
    const t = whereInput.value.trim();
    typing = false;
    if (!book || !t) return;
    const n = Number(t);
    if (/^\d+$/.test(t)) {
      if (n >= 1 && n <= book.sections) { jump(n, {}); whereInput.blur(); }
      else { whereInput.value = String(section); whereInput.title = `Choose a number from 1 to ${book.sections}`; whereInput.select(); }
      return;
    }
    const found = tocOptions(toc).filter(([, label]) => label.toLowerCase().includes(t.toLowerCase()));
    closePopover();
    togglePopover(whereInput, () => h("div", { class: "pop-body" }, found.length === 0
      ? h("p", { class: "hint", role: "status" }, "No heading has those words.")
      : h("div", { class: "menu-list toc-list" }, found.map(([v, label]) => button(label, () => {
        closePopover(true);
        const e = toc[Number(v)];
        if (e) jump(e.section, { anchor: e.anchor, flash: !!e.anchor });
      })))), { label: "Matching headings", side: "below", menu: false });
  }
  let typing = false; // the reader is typing in the field: a place change must not overwrite it
  whereInput.addEventListener("input", () => { typing = true; });
  whereInput.addEventListener("focus", () => whereInput.select());
  whereInput.addEventListener("keydown", (e) => {
    e.stopPropagation();
    if (e.key === "Enter") { e.preventDefault(); typedPlace(); }
    else if (e.key === "Escape") { typing = false; whereInput.value = String(section); whereInput.blur(); }
  });
  whereInput.addEventListener("blur", () => { typing = false; whereInput.value = String(section); });

  // header shows where the reader is: the place label, the contents entry and the pane's saved place.
  function header(n: number) {
    if (!book) return;
    section = n;
    unitText.textContent = book.format === "pdf" ? "Page" : "Section";
    ofText.textContent = `of ${book.sections}`;
    whereInput.setAttribute("aria-label", book.format === "pdf" ? "Go to a page number" : "Go to a section number, or words from a heading");
    if (!typing) whereInput.value = String(n);
    if (host.tab().section !== n) host.update({ section: n });
    prev.disabled = n <= 1;
    next.disabled = n >= book.sections;
    tocAt = currentEntry(toc, n);
    document.dispatchEvent(new Event("xegesis:book-section"));
  }

  async function fetchSection(b: BookInfo, n: number): Promise<HTMLElement | null> {
    const s = await msg.attempt(() => run<SectionData>("book.section", { item_id: b.item_id, file_id: b.file_id, section: n }));
    if (!s) return null;
    // The core sanitised this HTML: no scripts, styles, event attributes or remote resources.
    const sec = h("div", { class: "book-sec", "data-sec": String(n), ...(s.cfi ? { "data-cfi": s.cfi } : {}) });
    sec.innerHTML = s.html || `<p class="empty">This section has no text.</p>`;
    return sec;
  }

  async function show(n: number, target: Target) {
    if (!book) return;
    const b = book;
    const seq = ++loadSeq;
    header(n);
    store.clear();
    stopPdfScroll();
    pdfBox.classList.toggle("pdf-scroll", !b.reflowable && scrolling());
    if (!b.reflowable) lastWidth = body.clientWidth; // drawn at this width: no redraw for it
    if (b.reflowable) {
      const sec = await fetchSection(b, n);
      if (!sec || seq !== loadSeq || !alive) return;
      text.replaceChildren(sec);
      body.scrollTop = 0;
      await loadMarksFor(n);
    } else if (scrolling()) {
      await buildSlots(n, seq);
    } else {
      await drawPage(n);
      if (seq !== loadSeq) return;
      await loadMarksFor(n);
    }
    if (seq !== loadSeq) return;
    scrollTo(target);
    savePosition();
    if (b.reflowable && scrolling()) syncScroll();
  }

  // Scroll mode: the next section loads below when the end is near, the previous above when the
  // start is, and the view is kept where it was.
  async function grow(dir: 1 | -1) {
    if (growing || !book?.reflowable) return;
    const edge = (dir > 0 ? text.lastElementChild : text.firstElementChild) as HTMLElement | null;
    const n = Number(edge?.dataset.sec) + dir;
    if (!edge || n < 1 || n > book.sections) return;
    const seq = loadSeq;
    growing = true;
    try {
      const sec = await fetchSection(book, n);
      if (!sec || seq !== loadSeq || !alive) return;
      if (dir > 0) text.append(sec);
      else {
        // Keep what is being read exactly where it was, whatever margins the new section brings.
        const was = edge.getBoundingClientRect().top;
        text.prepend(sec);
        body.scrollTop += edge.getBoundingClientRect().top - was;
      }
      await loadMarksFor(n);
    } finally {
      growing = false;
    }
    if (seq === loadSeq) syncScroll();
  }

  // syncScroll updates the place shown while scrolling, and loads or draws what is coming.
  function syncScroll() {
    if (!book || !scrolling()) return;
    const bt = body.getBoundingClientRect().top;
    if (book.reflowable) {
      let cur = 0;
      for (const c of text.children) {
        if (c.getBoundingClientRect().top <= bt + body.clientHeight * 0.35) cur = Number((c as HTMLElement).dataset.sec);
        else break;
      }
      if (cur && cur !== section) {
        header(cur);
      }
      const room = body.clientHeight * 1.5;
      if (body.scrollHeight - body.scrollTop - body.clientHeight < room) void grow(1);
      if (body.scrollTop < room) void grow(-1);
    } else if (slots.size) {
      const y = bt - pdfBox.getBoundingClientRect().top + body.clientHeight * 0.35;
      const n = pageAtOffset([...slots.values()].map((sl) => sl.offsetTop), y);
      if (n && n !== section) {
        header(n);
      }
    }
  }

  // PDF Scroll mode -------------------------------------------------------------------------------

  function stopPdfScroll() {
    pdfSeq++;
    observer?.disconnect();
    observer = null;
    clearTimeout(planTimer);
    for (const r of drawn.values()) r.cancel();
    drawn.clear();
    pending.clear();
    near.clear();
    slots.clear();
  }

  async function buildSlots(start: number, seq: number) {
    if (!pdf || !book) return;
    page?.cancel();
    page = null;
    const doc = pdf.doc;
    const width = Math.max(200, body.clientWidth - 24);
    const p1 = await doc.getPage(1);
    if (seq !== loadSeq || !alive || !book) return;
    const vp = p1.getViewport({ scale: 1 });
    const scale = Math.max(0.2, (width / vp.width) * prefs.zoom);
    const w = Math.floor(vp.width * scale);
    const hh = Math.floor(vp.height * scale);
    const mine = ++pdfSeq;
    const list: HTMLElement[] = [];
    for (let n = 1; n <= book.sections; n++) {
      const sl = h("div", { class: "pdf-slot", "data-page": String(n), style: `width:${w}px;height:${hh}px` });
      slots.set(n, sl);
      list.push(sl);
    }
    pdfBox.replaceChildren(...list);
    const first = slots.get(start);
    body.scrollTop = first ? first.offsetTop - 8 : 0;
    observer = new IntersectionObserver((entries) => {
      for (const e of entries) {
        const n = Number((e.target as HTMLElement).dataset.page);
        if (e.isIntersecting) near.add(n);
        else near.delete(n);
      }
      clearTimeout(planTimer);
      planTimer = window.setTimeout(() => applyPlan(mine), 60);
    }, { root: body, rootMargin: "100% 0px" });
    for (const sl of list) observer.observe(sl);
  }

  function applyPlan(mine: number) {
    if (mine !== pdfSeq || !alive) return;
    const plan = renderPlan([...near], [...drawn.keys()], 2);
    for (const n of plan.free) {
      drawn.get(n)?.cancel();
      drawn.delete(n);
      slots.get(n)?.replaceChildren();
      store.delete(n);
    }
    for (const n of plan.render) if (!pending.has(n)) void drawSlot(n, mine);
    syncScroll();
  }

  async function drawSlot(n: number, mine: number) {
    if (!pdf) return;
    pending.add(n);
    const doc = pdf.doc;
    const width = Math.max(200, body.clientWidth - 24);
    try {
      const view = await import("./pdf");
      const r = await view.renderPage(doc, n, width, prefs.zoom);
      const sl = slots.get(n);
      if (mine !== pdfSeq || !sl) return r.cancel();
      const was = sl.offsetHeight;
      sl.replaceChildren(r.page);
      sl.style.width = r.page.style.width;
      sl.style.height = r.page.style.height;
      // A page above the view that turns out taller or shorter must not move the text being read.
      if (sl.offsetTop + was <= body.scrollTop + pdfBox.offsetTop) body.scrollTop += sl.offsetHeight - was;
      drawn.set(n, r);
      void linkPdfRefs(r.page);
      void loadMarksFor(n);
    } catch (e) {
      if (mine === pdfSeq && !(e instanceof Error && /cancel/i.test(e.name + e.message))) msg.attempt(() => Promise.reject(e));
    } finally {
      pending.delete(n);
    }
  }

  async function drawPage(n: number) {
    if (!pdf) return;
    page?.cancel();
    const doc = pdf.doc;
    const width = Math.max(200, body.clientWidth - 24);
    const r = await msg.attempt(async () => {
      const view = await import("./pdf");
      return view.renderPage(doc, n, width, prefs.zoom);
    });
    if (!r) return;
    page = r;
    clear(pdfBox, r.page);
    body.scrollTop = 0;
    linkPdfRefs(r.page);
  }

  // linkPdfRefs turns Bible references in a PDF page's text layer into the same links EPUB chapters
  // get, so clicking one opens it in the Bible pane. A reference split across lines stays plain.
  async function linkPdfRefs(pageEl: HTMLElement) {
    const spans = [...pageEl.querySelectorAll<HTMLElement>(".textLayer span")]
      .filter((sp) => sp.childElementCount === 0 && /\d/.test(sp.textContent ?? ""));
    if (spans.length === 0) return;
    const found = await run<{ start: number; end: number; ref: string }[][]>("book.refs",
      { texts: spans.map((sp) => sp.textContent ?? "") }).catch(() => null);
    if (!found || !pageEl.isConnected) return;
    spans.forEach((sp, i) => {
      const refs = found[i];
      if (!refs?.length) return;
      const t = sp.textContent ?? "";
      const parts: (string | Node)[] = [];
      let pos = 0;
      for (const f of refs) {
        parts.push(t.slice(pos, f.start), h("a", { href: "#", class: "vref", "data-ref": f.ref }, t.slice(f.start, f.end)));
        pos = f.end;
      }
      parts.push(t.slice(pos));
      sp.replaceChildren(...parts);
    });
  }

  function scrollTo(t: Target) {
    const id = t.anchor || (t.para ? `bp-${t.para}` : "");
    if (book && !book.reflowable && scrolling()) {
      const sl = slots.get(section);
      if (sl) body.scrollTop = sl.offsetTop - 8;
      return;
    }
    const node = id ? secEl(section)?.querySelector<HTMLElement>(`[id="${CSS.escape(id)}"]`) : null;
    if (!node || !book?.reflowable) return;
    let top = node.getBoundingClientRect().top;
    const root = secEl(section);
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
    body.scrollTop += top - body.getBoundingClientRect().top - 8;
    if (t.flash) {
      const block = node.closest("p,li,div,blockquote,td,h1,h2,h3,h4,h5,h6") ?? node.parentElement;
      block?.classList.add("jump-flash");
      setTimeout(() => block?.classList.remove("jump-flash"), 2500);
    }
  }

  const secEl = (n: number) => text.querySelector<HTMLElement>(`.book-sec[data-sec="${n}"]`);
  const secOf = (node: Node) => (node.nodeType === Node.ELEMENT_NODE ? (node as Element) : node.parentElement)?.closest<HTMLElement>(".book-sec") ?? null;

  // visiblePara is the paragraph at the top of the view.
  function visiblePara(): number {
    if (!book?.reflowable) return 0;
    const top = body.getBoundingClientRect().top;
    let para = 0;
    for (const m of secEl(section)?.querySelectorAll<HTMLElement>("span.bp") ?? []) {
      if (m.getBoundingClientRect().top > top + 4) break;
      para = Number(m.dataset.p);
    }
    return para || 1;
  }

  function savePosition() {
    clearTimeout(saveTimer);
    saveTimer = window.setTimeout(() => {
      if (!book) return;
      run("book.set_position", { item_id: book.item_id, file_id: book.file_id, section, para: visiblePara() }).catch(() => {});
    }, 1500);
  }
  body.addEventListener("scroll", () => {
    savePosition();
    if (!scrolling() || syncFrame) return;
    syncFrame = requestAnimationFrame(() => {
      syncFrame = 0;
      syncScroll();
    });
  }, { passive: true });

  // Space and the page keys move by a screen in Scroll mode; the arrow keys scroll natively.
  body.addEventListener("keydown", (e) => {
    if (!scrolling() || e.target !== body || e.altKey || e.ctrlKey || e.metaKey) return;
    const dir = e.key === "PageDown" || (e.key === " " && !e.shiftKey) ? 1 : e.key === "PageUp" || (e.key === " " && e.shiftKey) ? -1 : 0;
    if (!dir) return;
    e.preventDefault();
    body.scrollBy({ top: dir * body.clientHeight * 0.9 });
  });

  // The Pages / Scroll choice changed (in the Aa popover, or this pane's own setting).
  const onLayout = () => {
    const l = currentLayout();
    if (!alive || l === layout) return;
    layout = l;
    if (book) void show(section, { para: book.reflowable ? visiblePara() : 0 });
  };
  document.addEventListener(LAYOUT_EVENT, onLayout);

  // destroy frees the pane's book: a PDF's pages and the document pdf.js holds go with it.
  function destroy() {
    if (book) run("book.set_position", { item_id: book.item_id, file_id: book.file_id, section, para: visiblePara() }).catch(() => {});
    alive = false;
    loadSeq++;
    clearTimeout(saveTimer);
    resizer.disconnect();
    document.removeEventListener(LAYOUT_EVENT, onLayout);
    document.removeEventListener(ZOOM_EVENT, onZoom);
    cancelAnimationFrame(syncFrame);
    stopPdfScroll();
    book = null;
    page?.cancel();
    page = null;
    void pdf?.doc.destroy();
    pdf = null;
    clear(text);
    clear(pdfBox);
  }

  // Highlights and notes ----------------------------------------------------------------------------

  // loadMarksFor fetches one section's (or page's) highlights and notes and paints them.
  async function loadMarksFor(n: number) {
    if (!book) return;
    const p = { item_id: book.item_id, section: n };
    const [hs, ns] = await Promise.all([
      msg.attempt(() => run<Highlight[]>("highlight.list", p)),
      msg.attempt(() => run<BookNote[]>("note.list", { ...p, limit: 200 })),
    ]);
    if (!book || !alive) return;
    store.set(n, { hs: hs ?? [], ns: ns ?? [] });
    paintMarks(n);
  }

  async function loadMarks() {
    if (!book) return;
    await Promise.all([...(store.size ? store.keys() : [section])].map((n) => loadMarksFor(n)));
  }

  const allHighlights = () => [...store.values()].flatMap((v) => v.hs);

  function paintMarks(n: number) {
    if (!book) return;
    const m = store.get(n);
    if (!m) return;
    if (book.reflowable) {
      const sec = secEl(n);
      if (!sec) return;
      for (const x of [...sec.querySelectorAll("mark.hl")]) x.replaceWith(...x.childNodes);
      sec.normalize();
      for (const hl of m.hs) if (hl.locator) markText(sec, hl.locator, hlClasses(hl.style).join(" "), hl.id);
      for (const nt of m.ns) if (nt.locator) markText(sec, nt.locator, "hl hl-note", nt.id);
    } else {
      const pg = scrolling() ? drawn.get(n) : page;
      if (pg) clear(pg.marks, m.hs.map((hl) => rectsEl(hl.locator, `pdf-hl ${hlClasses(hl.style).slice(1).join(" ")}`)),
        m.ns.map((nt) => rectsEl(nt.locator, "pdf-hl hl-note")));
    }
  }

  function rectsEl(a: Anchor | undefined, cls: string) {
    return (a?.rects ?? []).map(([x, y, w, hh]) => h("div", { class: cls,
      style: `left:${x * 100}%;top:${y * 100}%;width:${w * 100}%;height:${hh * 100}%` }));
  }

  function markText(root: HTMLElement, a: Anchor, cls: string, id: string) {
    const start = pointAt(root, a.para ?? 0, a.offset ?? 0);
    const end = pointAt(root, a.end_para ?? a.para ?? 0, a.end_offset ?? a.offset ?? 0);
    if (!start || !end) return;
    const r = document.createRange();
    r.setStart(start.node, start.offset);
    r.setEnd(end.node, end.offset);
    if (r.collapsed) return;
    const parts: [Text, number, number][] = [];
    for (const t of textNodes(root)) {
      if (!r.intersectsNode(t)) continue;
      const s = t === r.startContainer ? r.startOffset : 0;
      const e = t === r.endContainer ? r.endOffset : t.length;
      if (s < e && t.data.slice(s, e).trim()) parts.push([t, s, e]);
    }
    for (const [t, s, e] of parts) {
      let mid = t;
      if (s > 0) mid = t.splitText(s);
      if (e - s < mid.length) mid.splitText(e - s);
      const m = h("mark", { class: cls, "data-mark": id });
      mid.replaceWith(m);
      m.append(mid);
    }
  }

  function selection(): Range | null {
    const sel = document.getSelection();
    if (!sel || sel.rangeCount === 0 || sel.isCollapsed) return null;
    const r = sel.getRangeAt(0).cloneRange();
    if (!r.intersectsNode(body)) return null;
    // A triple-click selects to the start of whatever follows the paragraph, which may lie outside
    // the book text; trim the range back to the text inside it.
    const nodes = textNodes(body).filter((t) => r.intersectsNode(t) && t.data.trim());
    if (nodes.length === 0) return null;
    if (!body.contains(r.startContainer)) r.setStart(nodes[0], 0);
    if (!body.contains(r.endContainer) || (r.endContainer.nodeType !== Node.TEXT_NODE && r.endOffset === 0)) {
      const last = nodes[nodes.length - 1];
      r.setEnd(last, last.length);
    }
    return r.collapsed || !r.toString().trim() ? null : r;
  }

  function anchorOf(r: Range): Anchor | null {
    if (!book) return null;
    const quote = r.toString().replace(/\s+/g, " ").trim().slice(0, 2000);
    if (book.reflowable) {
      const sec = secOf(r.startContainer);
      if (!sec) return null;
      const s = posOf(sec, r.startContainer, r.startOffset);
      // A selection that runs on into the next section is cut at the end of this one.
      let endNode = r.endContainer;
      let endOffset = r.endOffset;
      if (secOf(endNode) !== sec) {
        const last = textNodes(sec).pop();
        if (!last) return null;
        endNode = last;
        endOffset = last.length;
      }
      const e = posOf(sec, endNode, endOffset);
      if (!s || !e || !s.para) return null;
      const cfi = sec.dataset.cfi;
      return { section: Number(sec.dataset.sec), ...(cfi ? { cfi } : {}), para: s.para, offset: s.offset, end_para: e.para, end_offset: e.offset, text: quote };
    }
    const pg = (r.startContainer.nodeType === Node.ELEMENT_NODE ? (r.startContainer as Element) : r.startContainer.parentElement)?.closest<HTMLElement>(".pdf-page");
    if (!pg) return null;
    const n = Number(pg.parentElement?.dataset.page || section);
    const box = pg.getBoundingClientRect();
    const rects = pageRects([...r.getClientRects()].filter((c) => c.top >= box.top - 1 && c.bottom <= box.bottom + 1), box);
    return rects.length ? { section: n, page: n, rects, text: quote } : null;
  }

  async function addHighlight(style: string, given?: Range) {
    const r = given ?? selection();
    const a = r && anchorOf(r);
    if (!book || !a) return msg.info("Select some text in the book first.");
    const ok = await msg.attempt(() => run("highlight.create", { item_id: book!.item_id, locator: a, style }));
    if (ok !== undefined) {
      document.getSelection()?.removeAllRanges();
      marksChanged();
    }
  }

  // addNote starts a note on the selection, in the sidebar's Notes tool; it is kept with its first words.
  function addNote() {
    const r = selection();
    const a = r && anchorOf(r);
    if (!book || !a) return msg.info("Select some text in the book first.");
    sidebar().newNoteOn({ itemId: book.item_id, title: book.title, locator: a as unknown as Record<string, unknown> });
  }

  // Selected text goes into a list of excerpts as a copy: the book's licence limits and credit apply.
  function addExcerpt() {
    const r = selection();
    const a = r && anchorOf(r);
    if (!book || !a) return msg.info("Select some text in the book first.");
    addToList("excerpts", "this text", { item_id: book.item_id, file_id: book.file_id, locator: a, text: a.text ?? r!.toString() });
  }

  // Copying, links and keys -------------------------------------------------------------------------

  let passCopy = false;
  body.addEventListener("copy", (e) => {
    if (passCopy || !book) return;
    const r = selection();
    if (!r) return;
    e.preventDefault(); // nothing reaches the clipboard unless the licence allows it
    copyText(r.toString());
  });
  body.addEventListener("dragstart", (e) => e.preventDefault());

  async function copyText(selected: string) {
    if (!book || !selected.trim()) return;
    const res = await copyMsg.attempt(() => run<CopyResult>("book.copy", { item_id: book!.item_id, file_id: book!.file_id, text: selected }));
    if (!res) return;
    if (res.allowed && res.text) await writeClipboard(res.text);
    if (res.allowed) return copyMsg.ok(copyMessage(res));
    copyMsg.info(copyMessage(res));
    copyMsg.el.className = "msg error";
    copyMsg.el.setAttribute("role", "alert");
  }

  async function writeClipboard(s: string) {
    try {
      await navigator.clipboard.writeText(s);
      return;
    } catch {
      /* fall back to a copy command the handler lets through */
    }
    const ta = h("textarea", { class: "sr-only", "aria-hidden": "true" }, s);
    body.append(ta);
    ta.select();
    passCopy = true;
    try {
      document.execCommand("copy");
    } finally {
      passCopy = false;
      ta.remove();
    }
  }

  body.addEventListener("click", async (e) => {
    const a = (e.target as HTMLElement).closest("a");
    if (!a || !body.contains(a)) return;
    e.preventDefault();
    if (a.classList.contains("vref") && a.dataset.ref) {
      const p = await msg.attempt(() => run<Ref>("verse.parse", { ref: a.dataset.ref }));
      if (p) host.go(p.display);
    } else if (a.dataset.section) {
      jump(Number(a.dataset.section), { anchor: a.dataset.anchor, flash: !!a.dataset.anchor });
    } else if (a.dataset.url) {
      msg.info(`This link goes to ${a.dataset.url}. Xegesis does not open web pages from books.`);
    }
  });

  el.addEventListener("keydown", (e) => {
    if (!e.altKey || (e.key !== "PageDown" && e.key !== "PageUp")) return;
    e.preventDefault();
    go(section + (e.key === "PageDown" ? 1 : -1));
  });

  // A PDF page is redrawn to fit when the pane changes width.
  let resizeTimer = 0;
  let lastWidth = 0;
  const resizer = new ResizeObserver(() => {
    const w = body.clientWidth;
    if (!book || book.reflowable || !pdf || Math.abs(w - lastWidth) < 24) return;
    lastWidth = w;
    clearTimeout(resizeTimer);
    resizeTimer = window.setTimeout(() => alive && show(section, {}), 300);
  });
  resizer.observe(body);

  // The PDF zoom is changed in the Aa popover.
  const onZoom = () => {
    const z = bookZoom();
    if (!alive || z === prefs.zoom) return;
    prefs = tidyPrefs({ ...prefs, zoom: z });
    if (book && !book.reflowable) void show(section, {});
  };
  document.addEventListener(ZOOM_EVENT, onZoom);

  attachContextMenu(body, {
    highlight: (style) => addHighlight(style),
    list: () => addExcerpt(),
    showWord: (w) => sidebar().showWord(w),
    note: () => addNote(),
    copy: (t) => copyText(t),
    go: (ref) => host.go(ref),
  });

  // This pane repaints itself, so it ignores its own announcement (a second repaint would replace the marks again).
  let announcing = false;
  function marksChanged() {
    void loadMarks();
    announcing = true;
    document.dispatchEvent(new Event("xegesis:marks-changed"));
    announcing = false;
  }
  const onMarks = () => { if (alive && book && !announcing) void loadMarks(); };
  document.addEventListener("xegesis:marks-changed", onMarks);

  const marking = attachMarking(body, {
    highlight: (st, r) => addHighlight(styleOf(st), r),
    restyle: async (id, style) => {
      if (await msg.attempt(() => run("highlight.update", { id, style })) !== undefined) marksChanged();
    },
    remove: async (id) => {
      if (await msg.attempt(() => run("highlight.delete", { id })) !== undefined) marksChanged();
    },
    note: () => addNote(),
    lookUp: (t, word, at) => void lookUpCard(t, word, at, { showWord: (w) => sidebar().showWord(w), go: (ref) => host.go(ref) }),
    copy: (t) => copyText(t),
    list: () => addExcerpt(),
    at: (target) => {
      const m = target.closest<HTMLElement>("mark[data-mark]");
      const hl = m && allHighlights().find((x) => x.id === m.dataset.mark);
      return m && hl ? { id: hl.id, style: hl.style, el: m } : null;
    },
  });

  // The ⋯ menu ----------------------------------------------------------------------------------------

  const menu = {
    // ☰ Contents: the table of contents, narrowed by what is typed, and the places visited.
    contents(anchor: HTMLElement) {
      const unit = book?.reflowable ? "Section" : "Page";
      const entries = tocOptions(toc);
      contentsPopover(anchor, {
        placeholder: toc.length ? "Go to a chapter or section…" : `Go to a ${unit.toLowerCase()} number…`,
        go: (t) => {
          const n = Number(t);
          if (Number.isInteger(n) && n > 0) { go(n); return true; }
          const hit = entries.find(([, label]) => label.trim().toLowerCase().includes(t.toLowerCase()));
          const e = hit && toc[Number(hit[0])];
          if (!e) return false;
          jump(e.section, { anchor: e.anchor, flash: !!e.anchor });
          return true;
        },
        body: (q) => {
          const shown = entries.filter(([, label]) => !q || label.toLowerCase().includes(q.toLowerCase()));
          if (toc.length === 0) return h("p", { class: "hint" }, "This book has no contents list. Type a number to go there.");
          return h("div", { class: "menu-list toc-list" }, shown.length === 0 ? h("p", { class: "hint" }, "No heading has those words.")
            : shown.map(([v, label]) => button(label, () => {
              closePopover(true);
              const t = toc[Number(v)];
              if (t) jump(t.section, { anchor: t.anchor, flash: !!t.anchor });
            }, { class: Number(v) === tocAt ? "on" : "" })));
        },
        recent: [...back].reverse().map((p, i) => ({ label: `${unit} ${p.section}`, go: () => {
          back.splice(back.length - 1 - i);
          go(p.section, { para: p.para });
        } })),
      });
    },
    link: () => (book ? `#book/${encodeURIComponent(book.item_id)}?file=${encodeURIComponent(book.file_id)}${book.reflowable ? "" : `&page=${section}`}` : null),
    print,
    search(anchor: HTMLElement) {
      const q = h("input", { type: "search", "aria-label": "Search in this book", placeholder: "Words to find", autocomplete: "off" });
      const form = h("form", { class: "pop-body", role: "search" }, h("h2", { class: "pop-title" }, "Search in this"), q);
      form.addEventListener("submit", (e) => { e.preventDefault(); void menu.find(q.value.trim(), anchor); });
      togglePopover(anchor, () => form, { label: "Search in this", side: "below" });
    },
    async find(q: string, anchor: HTMLElement) {
      if (!book || !q) return;
      const b = book;
      const r = await msg.attempt(() => run<{ hits: { heading?: string; snippet: string; locator: Locator }[] }>("search.query",
        { query: q, item_ids: [b.item_id], limit: 30 }));
      if (!r || !alive) return;
      closePopover();
      togglePopover(anchor, () => h("div", { class: "pop-body find-results" },
        h("p", { class: "hint", role: "status" }, r.hits.length ? `${r.hits.length} ${r.hits.length === 1 ? "match" : "matches"} in ${b.title}` : `No matches in ${b.title}.`),
        h("ol", { class: "results" }, r.hits.map((hit) => h("li", {},
          button(hit.locator.page ? `Page ${hit.locator.page}` : hit.heading || "Show", async () => {
            closePopover();
            const loc = await msg.attempt(() => run<{ section: number; para?: number; anchor?: string }>("book.locate",
              { item_id: b.item_id, file_id: b.file_id, locator: hit.locator }));
            if (loc && alive) jump(loc.section, { anchor: loc.anchor, para: loc.para, flash: true });
          }, { class: "link" }),
          h("p", {}, markSegments(hit.snippet).map((s) => (s.mark ? h("mark", {}, s.text) : s.text))))))),
      { label: "Matches", side: "below" });
    },
  };

  const t = host.tab();
  if (t.itemId) void open({ itemId: t.itemId, fileId: t.fileId, locator: t.locator as Locator | undefined, section: t.locator ? undefined : t.section, anchor: t.anchor, offset: t.offset });
  else choose();
  if (t.locator || t.anchor) queueMicrotask(() => host.update({ locator: undefined, anchor: undefined, offset: undefined }));

  return {
    el, placeBox, menu, focus: () => body.focus(),
    markScope: () => (book ? { kind: "item", itemId: book.item_id, title: book.title || "this book", section, unit: book.reflowable ? "section" : "page" } : null),
    destroy() {
      marking.close();
      document.removeEventListener("xegesis:marks-changed", onMarks);
      destroy();
    },
  };
}

// Positions in sanitised book HTML ------------------------------------------------------------------
// A position is a paragraph number (the data-p of the span.bp marker that starts it) and a count of
// characters of text after that marker. The core sends the same HTML for a section every time, so
// positions found when highlighting are found again later.

function textNodes(root: Node): Text[] {
  const out: Text[] = [];
  const w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let n = w.nextNode(); n; n = w.nextNode()) out.push(n as Text);
  return out;
}

function posOf(root: HTMLElement, node: Node, offset: number): { para: number; offset: number } | null {
  const point = document.createRange();
  point.setStart(node, offset);
  const w = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT);
  let para = 0;
  let count = 0;
  let last: { para: number; offset: number } | null = null;
  for (let n = w.nextNode(); n; n = w.nextNode()) {
    if (n.nodeType === Node.ELEMENT_NODE) {
      const el = n as HTMLElement;
      if (el.classList.contains("bp")) {
        para = Number(el.dataset.p);
        count = 0;
      }
      continue;
    }
    const t = n as Text;
    if (point.comparePoint(t, t.length) >= 0) {
      if (t === node) return { para, offset: count + offset };
      return { para, offset: count + (point.comparePoint(t, 0) >= 0 ? 0 : t.length) };
    }
    count += t.length;
    last = { para, offset: count };
  }
  return last;
}

function pointAt(root: HTMLElement, para: number, offset: number): { node: Text; offset: number } | null {
  const w = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT);
  let inside = false;
  let count = 0;
  let last: Text | null = null;
  for (let n = w.nextNode(); n; n = w.nextNode()) {
    if (n.nodeType === Node.ELEMENT_NODE) {
      const el = n as HTMLElement;
      if (el.classList.contains("bp")) {
        if (inside) break;
        inside = Number(el.dataset.p) === para;
      }
      continue;
    }
    if (!inside) continue;
    const t = n as Text;
    if (count + t.length >= offset) return { node: t, offset: offset - count };
    count += t.length;
    last = t;
  }
  return last ? { node: last, offset: last.length } : null;
}
