// The book pane: a library book beside the Bible text. Reflowable books (EPUB, DOCX, HTML,
// Markdown, text) are shown one section at a time as HTML the core has already sanitised; PDF pages
// are drawn by pdf.js from bytes the core sends. Highlights and notes are anchored to paragraph
// markers in the HTML (or rectangles on a PDF page), copying goes through book.copy so the licence
// applies, and Bible references in the text open in the Bible pane.

import { ItemSummary, Message, Note, Ref, run } from "../api";
import {
  CopyResult, DEFAULT_PREFS, ReadingPrefs, TOCEntry, base64ToBytes, concatBytes, copyMessage, currentEntry,
  pageRects, tidyPrefs, tocOptions,
} from "../bookhelpers";
import { button, clear, field, h, select } from "../dom";
import { setPassage } from "../state";
import type { PdfDoc, RenderedPage } from "../pdfview";

// Locator is where a search hit starts (search.query's locator).
export interface Locator {
  spine?: number;
  href?: string;
  cfi?: string;
  page?: number;
  para?: number;
  offset?: number;
}

export interface BookRequest {
  itemId: string;
  fileId?: string;
  locator?: Locator;
}

// Other views open books through openBook; the reader's book pane takes the request.
let listener: ((r: BookRequest) => void) | null = null;
let pending: BookRequest | null = null;
export function openBook(r: BookRequest) {
  if (listener) listener(r);
  else pending = r;
}

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
  flash?: boolean;
}

const STYLES = ["yellow", "green", "blue", "pink", "purple", "underline"];
const PREFS_KEY = "xegesis.book.prefs";

function loadPrefs(): ReadingPrefs {
  try {
    return tidyPrefs(JSON.parse(localStorage.getItem(PREFS_KEY) ?? "null"));
  } catch {
    return { ...DEFAULT_PREFS };
  }
}

function savePrefs(p: ReadingPrefs) {
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(p));
  } catch {
    /* preferences are a convenience */
  }
}

export function bookPane(onState: (open: boolean) => void) {
  const msg = new Message();
  const copyMsg = new Message();
  let prefs = loadPrefs();
  let book: BookInfo | null = null;
  let section = 1;
  let cfi = "";
  let toc: TOCEntry[] = [];
  let pdf: { fileId: string; doc: PdfDoc } | null = null;
  let page: RenderedPage | null = null;
  let highlights: Highlight[] = [];
  let notes: BookNote[] = [];
  const back: { section: number; para: number }[] = [];
  let saveTimer = 0;
  let loadSeq = 0;

  const title = h("h2", { class: "book-title", tabindex: "-1" }, "Book");
  const byline = h("p", { class: "hint" });
  const contents = select([], "", { "aria-label": "Contents", class: "book-toc" });
  const prev = button("‹", () => go(section - 1), { "aria-label": "Previous section", title: "Previous section (Alt+Page Up)", "aria-keyshortcuts": "Alt+PageUp" });
  const next = button("›", () => go(section + 1), { "aria-label": "Next section", title: "Next section (Alt+Page Down)", "aria-keyshortcuts": "Alt+PageDown" });
  const where = h("span", { class: "hint book-where", "aria-live": "polite" });
  const backBtn = button("Back", () => goBack(), { disabled: true, title: "Return to where you were before the last jump" });
  const text = h("article", { class: "book-text", lang: "" });
  const pdfBox = h("div", { class: "pdf-box" });
  const body = h("div", { class: "book-body", tabindex: "0", role: "document", "aria-label": "Book text" }, text, pdfBox);
  const attribution = h("footer", { class: "attribution" });
  const styleSel = select(STYLES, "yellow", { "aria-label": "Highlight colour" });
  const hlBtn = button("Highlight", () => addHighlight(), { disabled: true });
  const noteBtn = button("Note", () => addNote(), { disabled: true });
  const actions = h("div", { class: "row wrap book-actions", role: "group", "aria-label": "Selected text" },
    h("span", { class: "hint" }, "Selection:"), styleSel, hlBtn, noteBtn);
  const editor = h("div", {});
  const marks = h("ul", { class: "book-marks" });
  const picker = h("div", { class: "card book-picker", hidden: true });

  const settings = textSettings();
  const el = h("section", { class: "pane book-pane", "aria-label": "Book", hidden: true },
    h("div", { class: "row between" }, title, button("Close", close, { "aria-label": "Close the book" })),
    byline,
    picker,
    h("div", { class: "row wrap book-nav" }, contents, prev, where, next, backBtn),
    settings.el,
    msg.el,
    actions, editor, copyMsg.el,
    body,
    attribution,
    h("details", { class: "book-marks-box" }, h("summary", {}, "Highlights and notes in this section"), marks));

  contents.addEventListener("change", () => {
    const t = toc[Number(contents.value)];
    if (t) jump(t.section, { anchor: t.anchor, flash: !!t.anchor });
  });

  // Choosing a book ---------------------------------------------------------------------------------

  function choose() {
    el.hidden = false;
    onState(true);
    const q = h("input", { type: "search", placeholder: "Title", autocomplete: "off", "aria-label": "Find a book by title" });
    const list = h("ul", { class: "results" });
    const pmsg = new Message();
    async function find() {
      const r = await pmsg.attempt(() => run<{ items: ItemSummary[] }>("library.items", { text: q.value.trim(), limit: 30 }));
      if (!r) return;
      const books = r.items.filter((it) => it.kind !== "bible");
      clear(list, books.length === 0 ? h("li", { class: "empty" }, "No books found.") : books.map((it) =>
        h("li", {}, button(it.title || "Untitled", () => {
          picker.hidden = true;
          open({ itemId: it.id });
        }, { class: "link", disabled: it.status !== "online" }),
        it.authors.length ? h("span", { class: "hint" }, ` ${it.authors.join(", ")}`) : null,
        it.status !== "online" ? h("span", { class: "badge warn" }, it.status) : null)));
    }
    const form = h("form", { role: "search", class: "row" }, q, h("button", { type: "submit", class: "primary" }, "Find"),
      button("Cancel", () => {
        picker.hidden = true;
        if (!book) close();
      }));
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      find();
    });
    clear(picker, h("h3", {}, "Open a book"), form, pmsg.el, list);
    picker.hidden = false;
    q.focus();
    find();
  }

  // Opening and moving ------------------------------------------------------------------------------

  async function open(r: BookRequest) {
    el.hidden = false;
    onState(true);
    picker.hidden = true;
    msg.clear();
    copyMsg.clear();
    const b = await msg.attempt(() => run<BookInfo>("book.open", { item_id: r.itemId, ...(r.fileId ? { file_id: r.fileId } : {}) }));
    if (!b) return;
    book = b;
    back.length = 0;
    backBtn.disabled = true;
    title.textContent = b.title || "Untitled";
    byline.textContent = [b.authors.join(", "), b.format.toUpperCase()].filter(Boolean).join(" · ");
    attribution.textContent = b.attribution ?? "";
    attribution.hidden = !b.attribution;
    text.hidden = !b.reflowable;
    pdfBox.hidden = b.reflowable;
    settings.update();
    toc = b.toc;
    if (b.format === "pdf") {
      if (!(await loadPdf(b))) return;
    }
    renderTOC();
    let target: Target = {};
    let start = b.position?.section ?? 1;
    if (r.locator) {
      const loc = await msg.attempt(() => run<{ section: number; para?: number; anchor?: string }>("book.locate",
        { item_id: b.item_id, file_id: b.file_id, locator: r.locator }));
      if (loc) {
        start = loc.section;
        target = { anchor: loc.anchor, para: loc.para, flash: true };
      }
    } else if (b.position?.para) {
      target = { para: b.position.para };
    }
    await show(Math.min(Math.max(1, start), b.sections), target);
    title.focus();
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
      const view = await import("../pdfview");
      const doc = await view.openPdf(concatBytes(parts));
      pdf = { fileId: b.file_id, doc };
      const items = await view.outline(doc);
      toc = items.map((o) => ({ title: o.title, section: o.page, level: o.level }));
      return true;
    });
    msg.clear();
    return !!ok;
  }

  function renderTOC() {
    clear(contents, h("option", { value: "" }, toc.length ? "Contents" : "No contents"),
      tocOptions(toc).map(([v, label]) => h("option", { value: v }, label)));
    contents.disabled = toc.length === 0;
  }

  function go(n: number, target: Target = {}) {
    if (!book || n < 1 || n > book.sections) return;
    show(n, target);
  }

  // jump moves somewhere else in the book, remembering where the user was.
  function jump(n: number, target: Target) {
    back.push({ section, para: visiblePara() });
    backBtn.disabled = false;
    go(n, target);
  }

  function goBack() {
    const p = back.pop();
    backBtn.disabled = back.length === 0;
    if (p) go(p.section, { para: p.para });
  }

  async function show(n: number, target: Target) {
    if (!book) return;
    const b = book;
    const seq = ++loadSeq;
    section = n;
    const unit = b.format === "pdf" ? "Page" : "Section";
    where.textContent = `${unit} ${n} of ${b.sections}`;
    prev.disabled = n <= 1;
    next.disabled = n >= b.sections;
    const ci = currentEntry(toc, n);
    contents.value = ci >= 0 ? String(ci) : "";
    if (b.reflowable) {
      const s = await msg.attempt(() => run<SectionData>("book.section", { item_id: b.item_id, file_id: b.file_id, section: n }));
      if (!s || seq !== loadSeq) return;
      cfi = s.cfi ?? "";
      // The core sanitised this HTML: no scripts, styles, event attributes or remote resources.
      text.innerHTML = s.html || `<p class="empty">This section has no text.</p>`;
      body.scrollTop = 0;
    } else {
      await drawPage(n);
      if (seq !== loadSeq) return;
    }
    await loadMarks();
    if (seq !== loadSeq) return;
    scrollTo(target);
    savePosition();
  }

  async function drawPage(n: number) {
    if (!pdf) return;
    page?.cancel();
    const doc = pdf.doc;
    const width = Math.max(200, body.clientWidth - 24);
    const r = await msg.attempt(async () => {
      const view = await import("../pdfview");
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
    const node = id ? text.querySelector<HTMLElement>(`[id="${CSS.escape(id)}"]`) : null;
    if (!node || !book?.reflowable) return;
    body.scrollTop += node.getBoundingClientRect().top - body.getBoundingClientRect().top - 8;
    if (t.flash) {
      const block = node.closest("p,li,div,blockquote,td,h1,h2,h3,h4,h5,h6") ?? node.parentElement;
      block?.classList.add("flash");
      setTimeout(() => block?.classList.remove("flash"), 2500);
    }
  }

  // visiblePara is the paragraph at the top of the view.
  function visiblePara(): number {
    if (!book?.reflowable) return 0;
    const top = body.getBoundingClientRect().top;
    let para = 0;
    for (const m of text.querySelectorAll<HTMLElement>("span.bp")) {
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
  body.addEventListener("scroll", savePosition, { passive: true });

  function close() {
    savePosition();
    el.hidden = true;
    book = null;
    page?.cancel();
    pdf?.doc.destroy();
    pdf = null;
    clear(text);
    clear(pdfBox);
    onState(false);
  }

  // Highlights and notes ----------------------------------------------------------------------------

  async function loadMarks() {
    if (!book) return;
    const p = { item_id: book.item_id, section };
    const [hs, ns] = await Promise.all([
      msg.attempt(() => run<Highlight[]>("highlight.list", p)),
      msg.attempt(() => run<BookNote[]>("note.list", { ...p, limit: 200 })),
    ]);
    highlights = hs ?? [];
    notes = ns ?? [];
    paintMarks();
    listMarks();
  }

  function paintMarks() {
    if (!book) return;
    if (book.reflowable) {
      for (const m of [...text.querySelectorAll("mark.hl")]) m.replaceWith(...m.childNodes);
      text.normalize();
      for (const hl of highlights) if (hl.locator) markText(hl.locator, `hl hl-${hl.style}`, hl.id);
      for (const n of notes) if (n.locator) markText(n.locator, "hl hl-note", n.id);
    } else if (page) {
      clear(page.marks, highlights.map((hl) => rectsEl(hl.locator, `pdf-hl hl-${hl.style}`)),
        notes.map((n) => rectsEl(n.locator, "pdf-hl hl-note")));
    }
  }

  function rectsEl(a: Anchor | undefined, cls: string) {
    return (a?.rects ?? []).map(([x, y, w, hh]) => h("div", { class: cls,
      style: `left:${x * 100}%;top:${y * 100}%;width:${w * 100}%;height:${hh * 100}%` }));
  }

  function listMarks() {
    const items = [
      ...highlights.map((hl) => ({ kind: "Highlight", id: hl.id, a: hl.locator, label: hl.locator?.text ?? "", style: hl.style })),
      ...notes.map((n) => ({ kind: "Note", id: n.id, a: n.locator, label: `${n.title || "Untitled"}: ${n.body}`, style: "note" })),
    ];
    clear(marks, items.length === 0 ? h("li", { class: "empty" }, "None yet. Select text to highlight it or add a note.") : items.map((m) =>
      h("li", { class: "row small" },
        h("span", { class: `swatch hl-${m.style}`, "aria-hidden": "true" }),
        h("span", { class: "grow" }, h("strong", {}, `${m.kind}: `), m.label.length > 120 ? m.label.slice(0, 120) + "…" : m.label),
        book?.reflowable && m.a?.para ? button("Show", () => scrollTo({ para: m.a?.para, flash: true }), { "aria-label": `Show ${m.kind.toLowerCase()}` }) : null,
        button("Delete", () => removeMark(m.kind, m.id), { class: "danger", "aria-label": `Delete ${m.kind.toLowerCase()}` }))));
  }

  async function removeMark(kind: string, id: string) {
    const ok = await msg.attempt(() => run(kind === "Note" ? "note.delete" : "highlight.delete", { id }));
    if (ok !== undefined) {
      msg.ok(`${kind} deleted. It can be restored from its history.`);
      loadMarks();
    }
  }

  function markText(a: Anchor, cls: string, id: string) {
    const start = pointAt(text, a.para ?? 0, a.offset ?? 0);
    const end = pointAt(text, a.end_para ?? a.para ?? 0, a.end_offset ?? a.offset ?? 0);
    if (!start || !end) return;
    const r = document.createRange();
    r.setStart(start.node, start.offset);
    r.setEnd(end.node, end.offset);
    if (r.collapsed) return;
    const parts: [Text, number, number][] = [];
    for (const t of textNodes(text)) {
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

  document.addEventListener("selectionchange", () => {
    const has = !!book && !!selection();
    hlBtn.disabled = !has;
    noteBtn.disabled = !has;
  });

  function anchorOf(r: Range): Anchor | null {
    if (!book) return null;
    const quote = r.toString().replace(/\s+/g, " ").trim().slice(0, 2000);
    if (book.reflowable) {
      const s = posOf(text, r.startContainer, r.startOffset);
      const e = posOf(text, r.endContainer, r.endOffset);
      if (!s || !e || !s.para) return null;
      return { section, ...(cfi ? { cfi } : {}), para: s.para, offset: s.offset, end_para: e.para, end_offset: e.offset, text: quote };
    }
    if (!page) return null;
    const rects = pageRects([...r.getClientRects()], page.page.getBoundingClientRect());
    return rects.length ? { section, page: section, rects, text: quote } : null;
  }

  async function addHighlight() {
    const r = selection();
    const a = r && anchorOf(r);
    if (!book || !a) return msg.info("Select some text in the book first.");
    const ok = await msg.attempt(() => run("highlight.create", { item_id: book!.item_id, locator: a, style: styleSel.value }));
    if (ok !== undefined) {
      document.getSelection()?.removeAllRanges();
      loadMarks();
    }
  }

  function addNote() {
    const r = selection();
    const a = r && anchorOf(r);
    if (!book || !a) return msg.info("Select some text in the book first.");
    const t = h("input", { type: "text" });
    const b = h("textarea", { rows: 4 });
    const form = h("form", { class: "card", "aria-label": "New note on the selection" },
      h("p", { class: "hint" }, `On: “${(a.text ?? "").slice(0, 160)}”`),
      field("Title", t), field("Text", b),
      h("div", { class: "row" }, h("button", { type: "submit", class: "primary" }, "Create note"), button("Cancel", () => clear(editor))));
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      const ok = await msg.attempt(() => run<Note>("note.create", { title: t.value, body: b.value, item_id: book!.item_id, locator: a }));
      if (ok) {
        clear(editor);
        msg.ok("Note created.");
        loadMarks();
      }
    });
    form.addEventListener("keydown", (e) => {
      if (e.key === "Escape") clear(editor);
      if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) form.requestSubmit();
    });
    clear(editor, form);
    t.focus();
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
      if (p) setPassage(p.display, p);
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

  let resizeTimer = 0;
  window.addEventListener("resize", () => {
    if (book && !book.reflowable && !el.hidden) {
      clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(() => show(section, {}), 300);
    }
  });

  // Text settings -----------------------------------------------------------------------------------

  function textSettings() {
    const font = select([["serif", "Serif"], ["sans", "Sans-serif"], ["mono", "Monospace"]], prefs.font);
    const theme = select([["auto", "Match the app"], ["light", "Light"], ["sepia", "Sepia"], ["dark", "Dark"]], prefs.theme);
    const spacing = select([["1.4", "Tight"], ["1.7", "Normal"], ["2.1", "Loose"]], String(prefs.spacing));
    const size = h("output", {}, `${prefs.size}%`);
    const zoom = h("output", {}, `${Math.round(prefs.zoom * 100)}%`);
    const reflow = h("div", { class: "row wrap" },
      field("Font", font), field("Line spacing", spacing), field("Theme", theme),
      h("div", { class: "field" }, h("span", { class: "label" }, "Text size"),
        h("div", { class: "row" }, button("A−", () => set({ size: prefs.size - 10 }), { "aria-label": "Smaller text" }), size,
          button("A+", () => set({ size: prefs.size + 10 }), { "aria-label": "Larger text" }))));
    const fixed = h("div", { class: "row" }, h("span", {}, "Zoom"),
      button("−", () => set({ zoom: Math.round((prefs.zoom - 0.1) * 10) / 10 }), { "aria-label": "Zoom out" }), zoom,
      button("+", () => set({ zoom: Math.round((prefs.zoom + 0.1) * 10) / 10 }), { "aria-label": "Zoom in" }));
    font.addEventListener("change", () => set({ font: font.value as ReadingPrefs["font"] }));
    theme.addEventListener("change", () => set({ theme: theme.value as ReadingPrefs["theme"] }));
    spacing.addEventListener("change", () => set({ spacing: Number(spacing.value) }));

    function set(p: Partial<ReadingPrefs>) {
      const zoomed = p.zoom !== undefined && p.zoom !== prefs.zoom;
      prefs = tidyPrefs({ ...prefs, ...p });
      savePrefs(prefs);
      apply();
      if (zoomed && book && !book.reflowable) show(section, {});
    }
    function apply() {
      size.textContent = `${prefs.size}%`;
      zoom.textContent = `${Math.round(prefs.zoom * 100)}%`;
      text.dataset.font = prefs.font;
      body.dataset.theme = prefs.theme;
      text.style.fontSize = `${(prefs.size / 100) * 1.06}rem`;
      text.style.lineHeight = String(prefs.spacing);
    }
    apply();
    const el = h("details", { class: "book-settings" }, h("summary", {}, "Text settings"), reflow, fixed);
    return {
      el,
      update() {
        reflow.hidden = !book?.reflowable;
        fixed.hidden = !!book?.reflowable;
      },
    };
  }

  listener = open;
  if (pending) {
    const r = pending;
    pending = null;
    queueMicrotask(() => open(r));
  }

  return { el, open, choose, isOpen: () => !!book };
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
