// How books are turned: Pages (a section or page at a time) or Scroll (one continuous flow). The
// choice is kept on this device only; until it is made, narrow screens scroll.

import { Layout, effectiveLayout, tidyPrefs } from "./bookhelpers";

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

// The zoom for PDF pages is kept with the other book reading choices, on this device only.
const PREFS_KEY = "xegesis.book.prefs";
export const ZOOM_EVENT = "xegesis:book-zoom";

export function bookZoom(): number {
  try {
    return tidyPrefs(JSON.parse(localStorage.getItem(PREFS_KEY) ?? "null")).zoom;
  } catch {
    return 1;
  }
}

export function setBookZoom(z: number): number {
  try {
    const p = tidyPrefs({ ...tidyPrefs(JSON.parse(localStorage.getItem(PREFS_KEY) ?? "null")), zoom: z });
    localStorage.setItem(PREFS_KEY, JSON.stringify(p));
    document.dispatchEvent(new Event(ZOOM_EVENT));
    return p.zoom;
  } catch {
    return bookZoom();
  }
}
