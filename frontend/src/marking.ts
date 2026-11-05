// Marking text: the small floating toolbar over selected text in any pane that shows text (a Bible,
// a book, a commentary). It offers the five highlight colours, Underline, Note, Look up and Copy.
// Clicking a highlight opens the same toolbar for it, with its colour ringed, Underline as a toggle
// and Remove. Alt+click on a word looks it up. After selecting with the keyboard, Tab (or Alt+F10)
// moves into the toolbar; Escape closes it. The right-click menu (contextmenu.ts) stays as well.
// Every action calls a command through the view that owns the text.

import { h } from "./dom";
import { icon } from "./icons";
import { COLOURS, Colour, Style, parseStyle, styleOf, toolbarPlace, wordAround } from "./markingwords";

export interface MarkTarget {
  id: string;
  style: string;
  el: Element; // where it is drawn, to place the toolbar
}

export interface MarkActions {
  highlight?(s: Style, r: Range): void | Promise<void>; // the selection
  restyle?(id: string, style: string): void | Promise<void>;
  remove?(id: string): void | Promise<void>;
  note?(text: string, r: Range | null): void;
  lookUp?(text: string, word: Element | null, at: DOMRect): void;
  copy?(text: string): void | Promise<void>;
  list?(r: Range): void; // add to a list
  at?(el: Element): MarkTarget | null; // the highlight drawn at a clicked element
}

export interface Marking {
  close(): void;
  // showFor opens the toolbar for something other than a text selection, such as picked verses.
  showFor(box: DOMRect, run: Partial<Pick<MarkActions, "highlight" | "note" | "list">> & { text: string }): void;
}

let open: { close(): void } | null = null;

// closeMarking closes the toolbar wherever it is open: going to another page leaves the selected
// text behind, and its toolbar must not stay over the new page.
export function closeMarking(): void {
  open?.close();
}

export function attachMarking(root: HTMLElement, a: MarkActions): Marking {
  let bar: HTMLElement | null = null;
  let mode: { kind: "selection"; range: Range } | { kind: "mark"; target: MarkTarget } | { kind: "custom"; run: Parameters<Marking["showFor"]>[1] } | null = null;
  let lastStyle: Style = { colour: "yellow", underline: false };

  const inRoot = (r: Range) => root.contains(r.commonAncestorContainer) || r.intersectsNode(root);
  const selection = (): Range | null => {
    const s = document.getSelection();
    if (!s || s.rangeCount === 0 || s.isCollapsed) return null;
    const r = s.getRangeAt(0);
    return inRoot(r) && r.toString().trim() ? r : null;
  };

  function close() {
    if (!bar) return;
    bar.remove();
    bar = null;
    mode = null;
    document.removeEventListener("pointerdown", onOutside, true);
    window.removeEventListener("resize", close);
    root.closest(".grid-body, main")?.removeEventListener("scroll", close);
    if (open?.close === close) open = null;
  }

  function onOutside(e: PointerEvent) {
    if (bar && !bar.contains(e.target as Node)) close();
  }

  function tool(label: string, content: Node | string, run: () => void, extra: Record<string, unknown> = {}): HTMLButtonElement {
    const b = h("button", { type: "button", class: "mark-btn", title: label, "aria-label": label, ...extra }, content);
    b.addEventListener("mousedown", (e) => e.preventDefault());
    b.addEventListener("click", () => run());
    return b;
  }

  function show(box: DOMRect) {
    open?.close();
    const current = mode?.kind === "mark" ? parseStyle(mode.target.style) : null;
    const text = mode?.kind === "selection" ? mode.range.toString().replace(/\s+/g, " ").trim()
      : mode?.kind === "custom" ? mode.run.text : "";
    const canColour = mode?.kind === "mark" ? !!a.restyle : mode?.kind === "custom" ? !!mode.run.highlight : !!a.highlight;
    const kids: Node[] = [];
    if (canColour) {
      kids.push(h("span", { class: "mark-swatches", role: "group", "aria-label": "Colour" }, COLOURS.map((c) =>
        tool(`Highlight ${c.label.toLowerCase()}`, h("span", { class: `mark-dot dot-${c.id}`, "aria-hidden": "true" }), () => void colour(c.id),
          { "aria-pressed": current ? String(current.colour === c.id) : null, class: `mark-btn mark-swatch${current?.colour === c.id ? " on" : ""}` }))));
      kids.push(h("span", { class: "mark-sep", "aria-hidden": "true" }));
      kids.push(tool(current?.underline ? "Underline (on)" : "Underline", h("span", { class: "mark-u", "aria-hidden": "true" }, "U"), () => void underline(),
        { "aria-pressed": current ? String(current.underline) : null, class: `mark-btn${current?.underline ? " on" : ""}` }));
    }
    const noteFn = mode?.kind === "custom" ? mode.run.note : a.note;
    if (noteFn) kids.push(tool("Add a note", h("span", { class: "mark-label" }, icon("pen"), "Note"), () => {
      const m = mode;
      const r = m?.kind === "selection" ? m.range : null;
      const t = text || (m?.kind === "mark" ? (m.target.el.textContent ?? "") : "");
      close();
      if (m?.kind === "custom") m.run.note?.(t, null);
      else a.note?.(t, r);
    }));
    if (a.lookUp && mode?.kind !== "custom") kids.push(tool("Look up (Alt+click a word)", h("span", { class: "mark-label" }, icon("search"), "Look up"), () => {
      const r = mode?.kind === "selection" ? mode.range : null;
      const t = text || (mode?.kind === "mark" ? (mode.target.el.textContent ?? "") : "");
      const at = r ? r.getBoundingClientRect() : box;
      const word = r ? (r.startContainer.parentElement?.closest(".w") ?? null) : null;
      close();
      document.getSelection()?.removeAllRanges();
      a.lookUp?.(t, word, at);
    }));
    if (a.copy && text) kids.push(tool("Copy, with the credit line", icon("copy"), () => { close(); void a.copy?.(text); }));
    const listFn = mode?.kind === "custom" ? mode.run.list : a.list;
    if (listFn && mode?.kind !== "mark") kids.push(tool("Add to a list", icon("list"), () => {
      const r = mode?.kind === "selection" ? mode.range : null;
      const run = mode?.kind === "custom" ? mode.run : null;
      close();
      if (run) run.list?.(r as Range);
      else if (r) a.list?.(r);
    }));
    if (mode?.kind === "mark" && a.remove) {
      const id = mode.target.id;
      kids.push(h("span", { class: "mark-sep", "aria-hidden": "true" }));
      kids.push(tool("Remove the highlight", "Remove", () => { close(); void a.remove?.(id); }, { class: "mark-btn mark-remove" }));
    }
    if (kids.length === 0) return;
    bar = h("div", { class: "mark-bar", role: "toolbar", "aria-label": mode?.kind === "mark" ? "Highlight" : "Selected text" }, kids);
    document.body.append(bar);
    const p = toolbarPlace(box, { width: bar.offsetWidth, height: bar.offsetHeight },
      { width: document.documentElement.clientWidth, height: document.documentElement.clientHeight });
    bar.style.left = `${p.left}px`;
    bar.style.top = `${p.top}px`;
    bar.dataset.below = String(p.below);
    // One stop on Tab; the arrow keys move along it.
    const items = [...bar.querySelectorAll<HTMLElement>("button")];
    items.forEach((b, i) => (b.tabIndex = i === 0 ? 0 : -1));
    bar.addEventListener("keydown", (e) => {
      const i = items.indexOf(document.activeElement as HTMLElement);
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        close();
        root.focus({ preventScroll: true });
        return;
      }
      const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
      if (!d || i < 0) return;
      e.preventDefault();
      const n = items[(i + d + items.length) % items.length];
      items.forEach((b) => (b.tabIndex = b === n ? 0 : -1));
      n.focus();
    });
    document.addEventListener("pointerdown", onOutside, true);
    window.addEventListener("resize", close);
    root.closest(".grid-body, main")?.addEventListener("scroll", close, { passive: true });
    open = { close };
  }

  async function colour(c: Colour) {
    if (!mode) return;
    if (mode.kind === "mark") {
      const cur = parseStyle(mode.target.style);
      const id = mode.target.id;
      close();
      if (cur.colour !== c) await a.restyle?.(id, styleOf({ ...cur, colour: c }));
      return;
    }
    lastStyle = { colour: c, underline: false };
    await apply(lastStyle);
  }

  async function underline() {
    if (!mode) return;
    if (mode.kind === "mark") {
      const cur = parseStyle(mode.target.style);
      const id = mode.target.id;
      close();
      await a.restyle?.(id, styleOf({ ...cur, underline: !cur.underline }));
      return;
    }
    await apply({ colour: lastStyle.colour, underline: true });
  }

  async function apply(s: Style) {
    const m = mode;
    close();
    if (m?.kind === "selection") {
      await a.highlight?.(s, m.range);
      document.getSelection()?.removeAllRanges();
    } else if (m?.kind === "custom") await m.run.highlight?.(s, document.createRange());
  }

  // The word under the pointer, for Alt+click.
  function wordAt(x: number, y: number): { text: string; rect: DOMRect; el: Element | null } | null {
    const doc = document as Document & { caretRangeFromPoint?(x: number, y: number): Range | null };
    const r = doc.caretRangeFromPoint?.(x, y);
    if (!r || r.startContainer.nodeType !== Node.TEXT_NODE || !root.contains(r.startContainer)) return null;
    const t = r.startContainer as Text;
    const span = wordAround(t.data, r.startOffset);
    if (!span) return null;
    const w = document.createRange();
    w.setStart(t, span[0]);
    w.setEnd(t, span[1]);
    return { text: w.toString(), rect: w.getBoundingClientRect(), el: t.parentElement?.closest(".w") ?? null };
  }

  root.addEventListener("mouseup", (e) => {
    if (e.button !== 0) return;
    if (e.altKey && a.lookUp) {
      const w = wordAt(e.clientX, e.clientY);
      if (w) {
        e.preventDefault();
        document.getSelection()?.removeAllRanges();
        close();
        a.lookUp(w.text, w.el, w.rect);
        return;
      }
    }
    setTimeout(() => {
      const r = selection();
      if (r) {
        mode = { kind: "selection", range: r.cloneRange() };
        show(r.getBoundingClientRect());
        return;
      }
      const t = a.at?.(e.target as Element);
      if (t) {
        mode = { kind: "mark", target: t };
        show(t.el.getBoundingClientRect());
      }
    }, 0);
  });
  // Alt+click should not start a selection.
  root.addEventListener("mousedown", (e) => { if (e.altKey && a.lookUp) e.preventDefault(); });
  root.addEventListener("keyup", (e) => {
    if (!e.shiftKey || !/^(Arrow|Home|End|Page)/.test(e.key)) return;
    const r = selection();
    if (r) {
      mode = { kind: "selection", range: r.cloneRange() };
      show(r.getBoundingClientRect());
    } else close();
  });
  root.addEventListener("keydown", (e) => {
    if (!bar) return;
    if ((e.key === "Tab" && !e.shiftKey) || (e.key === "F10" && e.altKey)) {
      e.preventDefault();
      bar.querySelector<HTMLElement>("button[tabindex='0']")?.focus();
    } else if (e.key === "Escape") close();
  });
  root.addEventListener("contextmenu", () => close());

  return {
    close,
    showFor(box, run) {
      mode = { kind: "custom", run };
      show(box);
    },
  };
}
