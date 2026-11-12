// Highlights and notes, the selection and copying: everything that works on text the reader picks.

import { run } from "../api";
import { copyMessage, CopyResult, pageRects } from "../bookhelpers";
import { clear, h } from "../dom";
import { sidebar } from "../views/sidebar";
import { addToList } from "../listpicker";
import { attachContextMenu } from "./contextmenu";
import { lookUpCard } from "../lookup";
import { attachMarking } from "../marking";
import { hlClasses, styleOf } from "../markingwords";
import type { BookCtx } from "./bookctx";
import type { Anchor, BookNote, Highlight } from "./bookinfo";
import { scrolling } from "./bookpaging";
import { isPdf, loadInk } from "./bookink";
import { pointAt, posOf, secEl, secOf, textNodes } from "./bookpos";

// Highlights and notes ----------------------------------------------------------------------------

// loadMarksFor fetches one section's (or page's) highlights and notes and paints them.
export async function loadMarksFor(c: BookCtx, n: number) {
  if (!c.book) return;
  const p = { item_id: c.book.item_id, section: n };
  const [hs, ns] = await Promise.all([
    c.msg.attempt(() => run<Highlight[]>("highlight.list", p)),
    c.msg.attempt(() => run<BookNote[]>("note.list", { ...p, limit: 200 })),
  ]);
  if (!c.book || !c.alive) return;
  c.store.set(n, { hs: (hs ?? []).filter((x) => x.style !== "ink"), ns: ns ?? [] });
  paintMarks(c, n);
  if (isPdf(c)) await loadInk(c, n);
}

export async function loadMarks(c: BookCtx) {
  if (!c.book) return;
  await Promise.all([...(c.store.size ? c.store.keys() : [c.section])].map((n) => loadMarksFor(c, n)));
}

export function allHighlights(c: BookCtx) {
  return [...c.store.values()].flatMap((v) => v.hs);
}

export function paintMarks(c: BookCtx, n: number) {
  if (!c.book) return;
  const m = c.store.get(n);
  if (!m) return;
  if (c.book.reflowable) {
    const sec = secEl(c, n);
    if (!sec) return;
    for (const x of [...sec.querySelectorAll("mark.hl")]) x.replaceWith(...x.childNodes);
    sec.normalize();
    for (const hl of m.hs) if (hl.locator) markText(sec, hl.locator, hlClasses(hl.style).join(" "), hl.id);
    for (const nt of m.ns) if (nt.locator) markText(sec, nt.locator, "hl hl-note", nt.id);
  } else {
    const pg = scrolling(c) ? c.drawn.get(n) : c.page;
    if (pg) clear(pg.marks, m.hs.map((hl) => rectsEl(hl.locator, `pdf-hl ${hlClasses(hl.style).slice(1).join(" ")}`)),
      m.ns.map((nt) => rectsEl(nt.locator, "pdf-hl hl-note")));
  }
}

export function rectsEl(a: Anchor | undefined, cls: string) {
  return (a?.rects ?? []).map(([x, y, w, hh]) => h("div", { class: cls,
    style: `left:${x * 100}%;top:${y * 100}%;width:${w * 100}%;height:${hh * 100}%` }));
}

export function markText(root: HTMLElement, a: Anchor, cls: string, id: string) {
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

export function selection(c: BookCtx): Range | null {
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0 || sel.isCollapsed) return null;
  const r = sel.getRangeAt(0).cloneRange();
  if (!r.intersectsNode(c.body)) return null;
  // A triple-click selects to the start of whatever follows the paragraph, which may lie outside
  // the book text; trim the range back to the text inside it.
  const nodes = textNodes(c.body).filter((t) => r.intersectsNode(t) && t.data.trim());
  if (nodes.length === 0) return null;
  if (!c.body.contains(r.startContainer)) r.setStart(nodes[0], 0);
  if (!c.body.contains(r.endContainer) || (r.endContainer.nodeType !== Node.TEXT_NODE && r.endOffset === 0)) {
    const last = nodes[nodes.length - 1];
    r.setEnd(last, last.length);
  }
  return r.collapsed || !r.toString().trim() ? null : r;
}

export function anchorOf(c: BookCtx, r: Range): Anchor | null {
  if (!c.book) return null;
  const quote = r.toString().replace(/\s+/g, " ").trim().slice(0, 2000);
  if (c.book.reflowable) {
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
  const n = Number(pg.parentElement?.dataset.page || c.section);
  const box = pg.getBoundingClientRect();
  const rects = pageRects([...r.getClientRects()].filter((c) => c.top >= box.top - 1 && c.bottom <= box.bottom + 1), box);
  return rects.length ? { section: n, page: n, rects, text: quote } : null;
}

export async function addHighlight(c: BookCtx, style: string, given?: Range) {
  const r = given ?? selection(c);
  const a = r && anchorOf(c, r);
  if (!c.book || !a) return c.msg.info("Select some text in the book first.");
  const ok = await c.msg.attempt(() => run("highlight.create", { item_id: c.book!.item_id, locator: a, style }));
  if (ok !== undefined) {
    document.getSelection()?.removeAllRanges();
    marksChanged(c);
  }
}

// addNote starts a note on the selection, in the sidebar's Notes tool; it is kept with its first words.
export function addNote(c: BookCtx) {
  const r = selection(c);
  const a = r && anchorOf(c, r);
  if (!c.book || !a) return c.msg.info("Select some text in the book first.");
  sidebar().newNoteOn({ itemId: c.book.item_id, title: c.book.title, locator: a as unknown as Record<string, unknown> });
}

// Selected text goes into a list of excerpts as a copy: the book's licence limits and credit apply.
export function addExcerpt(c: BookCtx) {
  const r = selection(c);
  const a = r && anchorOf(c, r);
  if (!c.book || !a) return c.msg.info("Select some text in the book first.");
  addToList("excerpts", "this text", { item_id: c.book.item_id, file_id: c.book.file_id, locator: a, text: a.text ?? r!.toString() });
}

export async function copyText(c: BookCtx, selected: string) {
  if (!c.book || !selected.trim()) return;
  const res = await c.copyMsg.attempt(() => run<CopyResult>("book.copy", { item_id: c.book!.item_id, file_id: c.book!.file_id, text: selected }));
  if (!res) return;
  if (res.allowed && res.text) await writeClipboard(c, res.text);
  if (res.allowed) return c.copyMsg.ok(copyMessage(res));
  c.copyMsg.info(copyMessage(res));
  c.copyMsg.el.className = "msg error";
  c.copyMsg.el.setAttribute("role", "alert");
}

export async function writeClipboard(c: BookCtx, s: string) {
  try {
    await navigator.clipboard.writeText(s);
    return;
  } catch {
    /* fall back to a copy command the handler lets through */
  }
  const ta = h("textarea", { class: "sr-only", "aria-hidden": "true" }, s);
  c.body.append(ta);
  ta.select();
  c.passCopy = true;
  try {
    document.execCommand("copy");
  } finally {
    c.passCopy = false;
    ta.remove();
  }
}

export function marksChanged(c: BookCtx) {
  void loadMarks(c);
  c.announcing = true;
  document.dispatchEvent(new Event("xegesis:marks-changed"));
  c.announcing = false;
}

// wireMarks listens for copying (which goes through the licence), the context menu and marking, and
// for other panes changing the highlights.
export function wireMarks(c: BookCtx) {
  c.body.addEventListener("copy", (e) => {
    if (c.passCopy || !c.book) return;
    const r = selection(c);
    if (!r) return;
    e.preventDefault(); // nothing reaches the clipboard unless the licence allows it
    copyText(c, r.toString());
  });
  c.body.addEventListener("dragstart", (e) => e.preventDefault());
  attachContextMenu(c.body, {
    highlight: (style) => addHighlight(c, style),
    list: () => addExcerpt(c),
    showWord: (w) => sidebar().showWord(w),
    note: () => addNote(c),
    copy: (t) => copyText(c, t),
    go: (ref) => c.host.go(ref),
  });
  const onMarks = () => { if (c.alive && c.book && !c.announcing) void loadMarks(c); };
  document.addEventListener("xegesis:marks-changed", onMarks);
  const marking = attachMarking(c.body, {
    highlight: (st, r) => addHighlight(c, styleOf(st), r),
    restyle: async (id, style) => {
      if (await c.msg.attempt(() => run("highlight.update", { id, style })) !== undefined) marksChanged(c);
    },
    remove: async (id) => {
      if (await c.msg.attempt(() => run("highlight.delete", { id })) !== undefined) marksChanged(c);
    },
    note: () => addNote(c),
    lookUp: (t, word, at) => void lookUpCard(t, word, at, { showWord: (w) => sidebar().showWord(w), go: (ref) => c.host.go(ref) }),
    copy: (t) => copyText(c, t),
    list: () => addExcerpt(c),
    at: (target) => {
      const m = target.closest<HTMLElement>("mark[data-mark]");
      const hl = m && allHighlights(c).find((x) => x.id === m.dataset.mark);
      return m && hl ? { id: hl.id, style: hl.style, el: m } : null;
    },
  });
  c.unwireMarks = () => {
    marking.close();
    document.removeEventListener("xegesis:marks-changed", onMarks);
  };
}
