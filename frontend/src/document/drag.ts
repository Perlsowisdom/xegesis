// What is being dragged into or within the study grid. HTML5 drag and drop only shows a drag's data
// on drop, so it is kept here too, for the dashed preview while dragging.

import type { TabSpec } from "./grid";

export const DRAG_TYPE = "application/x-xegesis-tab";

export let dragging: { tabId?: string; spec?: TabSpec } | null = null;

export function setDragging(d: typeof dragging): void {
  dragging = d;
}

// draggableSpec makes an element outside the grid (a Library row, a Related card) drag a new tab in.
export function draggableSpec(el: HTMLElement, spec: () => TabSpec | null): void {
  el.draggable = true;
  el.classList.add("drags-tab");
  el.addEventListener("dragstart", (e) => {
    const s = spec();
    if (!s || !e.dataTransfer) return;
    e.stopPropagation();
    dragging = { spec: s };
    e.dataTransfer.setData(DRAG_TYPE, JSON.stringify(s));
    e.dataTransfer.setData("text/plain", s.title ?? "");
    e.dataTransfer.effectAllowed = "copyMove";
  });
  el.addEventListener("dragend", () => { dragging = null; });
}
