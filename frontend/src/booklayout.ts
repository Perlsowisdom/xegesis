// How books are turned: Pages (a section or page at a time) or Scroll (one continuous flow). The
// choice is kept on this device only; until it is made, narrow screens scroll.

import { Layout, effectiveLayout } from "./bookhelpers";

const KEY = "xegesis.book.layout";
export const LAYOUT_EVENT = "xegesis:book-layout";

export function chosenLayout(): Layout | null {
  try {
    const v = localStorage.getItem(KEY);
    return v === "pages" || v === "scroll" ? v : null;
  } catch {
    return null;
  }
}

export function currentLayout(): Layout {
  return effectiveLayout(chosenLayout(), window.innerWidth);
}

export function setLayout(l: Layout): void {
  try {
    localStorage.setItem(KEY, l);
  } catch {
    /* a convenience only */
  }
  document.dispatchEvent(new Event(LAYOUT_EVENT));
}
