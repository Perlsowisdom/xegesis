// Scroll mode: reflowable sections that load as the reader nears either end, and PDF pages drawn near the view.

import { pageAtOffset, renderPlan } from "../bookhelpers";
import { h } from "../dom";
import type { BookCtx } from "./bookctx";
import { fetchSection, scrolling } from "./bookpaging";
import { header } from "./bookwhere";
import { loadMarksFor } from "./bookmarks";
import { linkPdfRefs } from "./booklinks";

// Scroll mode: the next section loads below when the end is near, the previous above when the
// start is, and the view is kept where it was.
export async function grow(c: BookCtx, dir: 1 | -1) {
  if (c.growing || !c.book?.reflowable) return;
  const edge = (dir > 0 ? c.text.lastElementChild : c.text.firstElementChild) as HTMLElement | null;
  const n = Number(edge?.dataset.sec) + dir;
  if (!edge || n < 1 || n > c.book.sections) return;
  const seq = c.loadSeq;
  c.growing = true;
  try {
    const sec = await fetchSection(c, c.book, n);
    if (!sec || seq !== c.loadSeq || !c.alive) return;
    if (dir > 0) c.text.append(sec);
    else {
      // Keep what is being read exactly where it was, whatever margins the new section brings.
      const was = edge.getBoundingClientRect().top;
      c.text.prepend(sec);
      c.body.scrollTop += edge.getBoundingClientRect().top - was;
    }
    await loadMarksFor(c, n);
  } finally {
    c.growing = false;
  }
  if (seq === c.loadSeq) syncScroll(c);
}

// syncScroll updates the place shown while scrolling, and loads or draws what is coming.
export function syncScroll(c: BookCtx) {
  if (!c.book || !scrolling(c)) return;
  const bt = c.body.getBoundingClientRect().top;
  if (c.book.reflowable) {
    let cur = 0;
    for (const child of c.text.children) {
      if (child.getBoundingClientRect().top <= bt + c.body.clientHeight * 0.35) cur = Number((child as HTMLElement).dataset.sec);
      else break;
    }
    if (cur && cur !== c.section) {
      header(c, cur);
    }
    const room = c.body.clientHeight * 1.5;
    if (c.body.scrollHeight - c.body.scrollTop - c.body.clientHeight < room) void grow(c, 1);
    if (c.body.scrollTop < room) void grow(c, -1);
  } else if (c.slots.size) {
    const y = bt - c.pdfBox.getBoundingClientRect().top + c.body.clientHeight * 0.35;
    const n = pageAtOffset([...c.slots.values()].map((sl) => sl.offsetTop), y);
    if (n && n !== c.section) {
      header(c, n);
    }
  }
}

// PDF Scroll mode -------------------------------------------------------------------------------

export function stopPdfScroll(c: BookCtx) {
  c.pdfSeq++;
  c.observer?.disconnect();
  c.observer = null;
  clearTimeout(c.planTimer);
  for (const r of c.drawn.values()) r.cancel();
  c.drawn.clear();
  c.pending.clear();
  c.near.clear();
  c.slots.clear();
}

export async function buildSlots(c: BookCtx, start: number, seq: number) {
  if (!c.pdf || !c.book) return;
  c.page?.cancel();
  c.page = null;
  const doc = c.pdf.doc;
  const width = Math.max(200, c.body.clientWidth - 24);
  const p1 = await doc.getPage(1);
  if (seq !== c.loadSeq || !c.alive || !c.book) return;
  const vp = p1.getViewport({ scale: 1 });
  const scale = Math.max(0.2, (width / vp.width) * c.prefs.zoom);
  const w = Math.floor(vp.width * scale);
  const hh = Math.floor(vp.height * scale);
  const mine = ++c.pdfSeq;
  const list: HTMLElement[] = [];
  for (let n = 1; n <= c.book.sections; n++) {
    const sl = h("div", { class: "pdf-slot", "data-page": String(n), style: `width:${w}px;height:${hh}px` });
    c.slots.set(n, sl);
    list.push(sl);
  }
  c.pdfBox.replaceChildren(...list);
  const first = c.slots.get(start);
  c.body.scrollTop = first ? first.offsetTop - 8 : 0;
  c.observer = new IntersectionObserver((entries) => {
    for (const e of entries) {
      const n = Number((e.target as HTMLElement).dataset.page);
      if (e.isIntersecting) c.near.add(n);
      else c.near.delete(n);
    }
    clearTimeout(c.planTimer);
    c.planTimer = window.setTimeout(() => applyPlan(c, mine), 60);
  }, { root: c.body, rootMargin: "100% 0px" });
  for (const sl of list) c.observer.observe(sl);
}

export function applyPlan(c: BookCtx, mine: number) {
  if (mine !== c.pdfSeq || !c.alive) return;
  const plan = renderPlan([...c.near], [...c.drawn.keys()], 2);
  for (const n of plan.free) {
    c.drawn.get(n)?.cancel();
    c.drawn.delete(n);
    c.slots.get(n)?.replaceChildren();
    c.store.delete(n);
  }
  for (const n of plan.render) if (!c.pending.has(n)) void drawSlot(c, n, mine);
  syncScroll(c);
}

export async function drawSlot(c: BookCtx, n: number, mine: number) {
  if (!c.pdf) return;
  c.pending.add(n);
  const doc = c.pdf.doc;
  const width = Math.max(200, c.body.clientWidth - 24);
  try {
    const view = await import("./pdf");
    const r = await view.renderPage(doc, n, width, c.prefs.zoom);
    const sl = c.slots.get(n);
    if (mine !== c.pdfSeq || !sl) return r.cancel();
    const was = sl.offsetHeight;
    sl.replaceChildren(r.page);
    sl.style.width = r.page.style.width;
    sl.style.height = r.page.style.height;
    // A page above the view that turns out taller or shorter must not move the text being read.
    if (sl.offsetTop + was <= c.body.scrollTop + c.pdfBox.offsetTop) c.body.scrollTop += sl.offsetHeight - was;
    c.drawn.set(n, r);
    void linkPdfRefs(r.page);
    void loadMarksFor(c, n);
  } catch (e) {
    if (mine === c.pdfSeq && !(e instanceof Error && /cancel/i.test(e.name + e.message))) c.msg.attempt(() => Promise.reject(e));
  } finally {
    c.pending.delete(n);
  }
}
