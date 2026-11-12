// Positions in sanitised book HTML: a paragraph number and a count of characters after its marker.

import type { BookCtx } from "./bookctx";

export function textNodes(root: Node): Text[] {
  const out: Text[] = [];
  const w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let n = w.nextNode(); n; n = w.nextNode()) out.push(n as Text);
  return out;
}

export function posOf(root: HTMLElement, node: Node, offset: number): { para: number; offset: number } | null {
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

export function pointAt(root: HTMLElement, para: number, offset: number): { node: Text; offset: number } | null {
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


export function secEl(c: BookCtx, n: number) {
  return c.text.querySelector<HTMLElement>(`.book-sec[data-sec="${n}"]`);
}

export function secOf(node: Node) {
  return (node.nodeType === Node.ELEMENT_NODE ? (node as Element) : node.parentElement)?.closest<HTMLElement>(".book-sec") ?? null;
}
