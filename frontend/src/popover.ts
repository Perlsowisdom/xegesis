// A small popover anchored to a button: rail settings, help, search, the Reader's menus. One is open
// at a time. Escape or a click outside closes it, and focus goes back to the button that opened it.

import { h } from "./dom";

export interface PopoverOptions {
  label: string;
  // right: beside the anchor (the rail); below: under it (toolbars). Both stay inside the window.
  side?: "right" | "below";
  // A menu holds buttons only and is reached with the arrow keys as well as Tab.
  menu?: boolean;
  onClose?: () => void;
  className?: string;
}

interface Open {
  el: HTMLElement;
  anchor: HTMLElement;
  close: (refocus?: boolean) => void;
}

let current: Open | null = null;

export function popoverOpen(): boolean {
  return current !== null;
}

export function closePopover(refocus = false): void {
  current?.close(refocus);
}

// togglePopover opens content beside anchor, or closes it when it is already open there.
export function togglePopover(anchor: HTMLElement, content: () => Node, o: PopoverOptions): HTMLElement | null {
  if (current?.anchor === anchor) {
    current.close(false);
    return null;
  }
  return openPopover(anchor, content(), o);
}

export function openPopover(anchor: HTMLElement, content: Node, o: PopoverOptions): HTMLElement {
  current?.close(false);
  const el = h("div", {
    class: `popover ${o.className ?? ""}`, role: o.menu ? "menu" : "dialog", "aria-label": o.label, tabindex: "-1",
  }, content);
  if (o.menu) for (const b of el.querySelectorAll("button")) b.setAttribute("role", "menuitem");
  document.body.append(el);
  anchor.setAttribute("aria-expanded", "true");

  const place = () => {
    const r = anchor.getBoundingClientRect();
    const w = el.offsetWidth;
    const ht = el.offsetHeight;
    const vw = document.documentElement.clientWidth;
    const vh = document.documentElement.clientHeight;
    let left: number;
    let top: number;
    if ((o.side ?? "below") === "right") {
      left = r.right + 8;
      top = Math.min(r.top, vh - ht - 8);
    } else {
      left = Math.min(r.left, vw - w - 8);
      top = r.bottom + 4;
      if (top + ht > vh - 8 && r.top - ht - 4 > 8) top = r.top - ht - 4;
    }
    el.style.left = `${Math.max(8, left)}px`;
    el.style.top = `${Math.max(8, top)}px`;
  };
  place();

  const onKey = (e: KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      close(true);
      return;
    }
    if (o.menu && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
      const items = [...el.querySelectorAll<HTMLElement>("[role=menuitem]:not([disabled])")];
      const i = items.indexOf(document.activeElement as HTMLElement);
      const next = items[(i + (e.key === "ArrowDown" ? 1 : -1) + items.length) % items.length];
      if (next) {
        e.preventDefault();
        next.focus();
      }
    }
  };
  const onDown = (e: PointerEvent) => {
    const t = e.target as Node;
    if (!el.contains(t) && !anchor.contains(t)) close(false);
  };
  const onResize = () => close(false);
  el.addEventListener("keydown", onKey);
  document.addEventListener("pointerdown", onDown, true);
  window.addEventListener("resize", onResize);

  function close(refocus = false) {
    if (current?.el !== el) return;
    current = null;
    el.remove();
    anchor.setAttribute("aria-expanded", "false");
    document.removeEventListener("pointerdown", onDown, true);
    window.removeEventListener("resize", onResize);
    if (refocus) anchor.focus();
    o.onClose?.();
  }
  current = { el, anchor, close };
  // Clicking a menu item does its job and closes the menu.
  if (o.menu) el.addEventListener("click", (e) => { if ((e.target as Element).closest("button")) close(true); });

  const first = el.querySelector<HTMLElement>("input, select, textarea, button, a[href], [tabindex]:not([tabindex='-1'])");
  (first ?? el).focus();
  return el;
}
