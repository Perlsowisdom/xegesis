// The study grid: up to 3 columns of up to 3 panes, each pane with its own tabs, + for a new tab,
// a place box, a link button and a ⋯ menu. Tabs are dragged between panes (or onto an edge, to
// split one), and rows from the Library browser, My work and the sidebar's Related cards drag in
// too. The layout model is document/grid.ts; this file only draws it and turns gestures into
// changes. Only the current tab of each visible pane is mounted, so hidden PDFs are not kept drawn.
// Read mode draws the selected pane alone.

import { Ref, run } from "../api";
import { button, clear, h } from "../dom";
import { icon } from "../icons";
import { closePopover, openPopover, togglePopover } from "../popover";
import { loadPref, navigate, onPassage, savePref, setPassage, state } from "../state";
import {
  Grid, MAX_COLUMNS, MAX_PANES, MoveTarget, Pane, Tab, TabSpec, Zone, activeTab, applyPreset, closePane, closeTab, dropTab,
  equalise, findPane, findTab, insertTab, moveTab, openTab, paneRef, panes, resizeColumns, resizePanes, restore,
  selectPane, selectTab, setLinked, setRef, shape, updateTab,
} from "./grid";
import { DRAG_TYPE, dragging, setDragging } from "./drag";
import { setSelectedView } from "../gridhook";
import { mountDoc } from "./mount";
import { DocHost, DocView, followsPassage, tabLabel } from "./view";
import { paneMenu as paneMenuGroups } from "../docwords";
import { flash } from "../listpicker";

const GRID_KEY = "xegesis.grid";
const STEP = 0.05;

// zoneAt picks where a drop lands from the pointer's place in the pane: near an edge splits it,
// the middle (or the tab strip) moves the tab into it.
export function zoneAt(x: number, y: number, w: number, h: number, overBar: boolean): Zone {
  if (overBar) return "center";
  const fx = x / w;
  const fy = y / h;
  const edges: [Zone, number][] = [["left", fx], ["right", 1 - fx], ["top", fy], ["bottom", 1 - fy]];
  const [zone, d] = edges.reduce((a, b) => (b[1] < a[1] ? b : a));
  return d < 0.25 ? zone : "center";
}

interface PaneUI {
  el: HTMLElement;
  tabs: HTMLElement;
  place: HTMLElement;
  tools: HTMLElement;
  link: HTMLButtonElement;
  contents: HTMLButtonElement;
  more: HTMLButtonElement;
  body: HTMLElement;
  preview: HTMLElement;
}

interface Mounted {
  view: DocView;
  paneId: string;
}

export interface StudyGridOptions {
  mode(): "read" | "study";
  defaultTab(): TabSpec;
}

export function studyGrid(o: StudyGridOptions) {
  let g: Grid = restore(loadPref(GRID_KEY, null));
  if (!g.passage) g = { ...g, passage: state.ref || loadPref("xegesis.ref", "John 1") };
  if (panes(g).length === 0) g = openTab(g, o.defaultTab());

  const el = h("div", { class: "study-grid", role: "region", "aria-label": "Study grid" });
  const uis = new Map<string, PaneUI>();
  const colEls = new Map<string, HTMLElement>();
  const mounted = new Map<string, Mounted>();
  let syncing = false;
  let rendering = false;
  let again = false;

  // setGrid saves a change and redraws. A change made while drawing (a view naming its tab as it
  // mounts) is drawn straight after.
  function setGrid(n: Grid | null) {
    if (!n || n === g) return;
    g = n;
    savePref(GRID_KEY, g);
    render();
  }

  function render() {
    if (rendering) {
      again = true;
      return;
    }
    rendering = true;
    try {
      do {
        again = false;
        draw();
      } while (again);
    } finally {
      rendering = false;
    }
  }

  // select highlights a pane without redrawing, so a click that selects it still lands.
  function select(paneId: string) {
    const n = selectPane(g, paneId);
    if (n === g) return;
    g = n;
    savePref(GRID_KEY, g);
    for (const [id, ui] of uis) ui.el.classList.toggle("selected", id === paneId);
    syncPassage();
    document.dispatchEvent(new Event("xegesis:pane-changed"));
  }

  // Hosts ----------------------------------------------------------------------------------------------

  function hostFor(tabId: string, initial: Tab): DocHost {
    return {
      tab: () => findTab(g, tabId)?.tab ?? initial,
      ref: () => {
        const f = findTab(g, tabId);
        return f ? paneRef(g, f.pane) : g.passage;
      },
      go: (ref) => {
        const f = findTab(g, tabId);
        if (f) setGrid(setRef(g, f.pane.id, ref));
      },
      update: (patch) => setGrid(updateTab(g, tabId, patch)),
      open: (spec) => {
        const f = findTab(g, tabId);
        setGrid(openTab(g, spec, f?.pane.id));
      },
      mode: o.mode,
    };
  }

  // Drawing --------------------------------------------------------------------------------------------

  function setChildren(parent: Element, kids: Node[]) {
    const now = [...parent.childNodes];
    if (now.length === kids.length && now.every((n, i) => n === kids[i])) return;
    parent.replaceChildren(...kids);
  }

  function draw() {
    const read = o.mode() === "read";
    el.dataset.mode = read ? "read" : "study";
    const sel = findPane(g, g.selected)?.pane ?? panes(g)[0];
    if (!sel) {
      for (const [id, m] of mounted) { m.view.destroy(); mounted.delete(id); }
      clear(el, h("div", { class: "grid-empty" },
        h("p", {}, "Nothing is open."),
        h("div", { class: "row wrap" },
          button("Open a Bible", () => setGrid(openTab(g, { kind: "bible" })), { class: "primary" }),
          button("Open the Library browser", () => setGrid(openTab(g, { kind: "library" }))))));
      return;
    }
    // Which tabs are on screen: each pane's current tab, or in Read mode the selected pane's only.
    const visible = new Map<string, Pane>();
    const cols = read ? [{ id: "read", panes: [sel], size: 1 }] : g.columns;
    for (const c of cols) for (const p of c.panes) visible.set(activeTab(p)!.id, p);
    for (const [id, m] of mounted) {
      const p = visible.get(id);
      if (!p || p.id !== m.paneId) {
        m.view.destroy();
        mounted.delete(id);
      }
    }
    for (const [id] of uis) if (!findPane(g, id)) uis.delete(id);

    const colNodes: Node[] = [];
    cols.forEach((c, ci) => {
      let cel = colEls.get(c.id);
      if (!cel) {
        cel = h("div", { class: "grid-col" });
        colEls.set(c.id, cel);
      }
      cel.style.flexGrow = String(c.size);
      const kids: Node[] = [];
      c.panes.forEach((p, pi) => {
        if (pi > 0) kids.push(gutter("row", ci, pi - 1));
        kids.push(drawPane(p, sel.id === p.id, ci, pi));
      });
      setChildren(cel, kids);
      if (ci > 0) colNodes.push(gutter("col", ci - 1, 0));
      colNodes.push(cel);
    });
    for (const id of [...colEls.keys()]) if (!g.columns.some((c) => c.id === id)) colEls.delete(id);
    setChildren(el, colNodes);
    for (const m of mounted.values()) {
      const p = findPane(g, m.paneId)?.pane;
      if (p) m.view.refChanged?.(paneRef(g, p));
    }
    syncPassage();
    document.dispatchEvent(new Event("xegesis:pane-changed"));
  }

  function drawPane(p: Pane, selected: boolean, ci: number, pi: number): HTMLElement {
    let ui = uis.get(p.id);
    if (!ui) {
      ui = paneUI(p.id);
      uis.set(p.id, ui);
    }
    const tab = activeTab(p)!;
    ui.el.classList.toggle("selected", selected);
    ui.el.style.flexGrow = String(p.size);
    ui.el.setAttribute("aria-label", `Pane ${paneNumber(p.id)}: ${tabLabel(tab)}`);
    ui.el.dataset.col = String(ci);
    ui.el.dataset.row = String(pi);

    clear(ui.tabs, p.tabs.map((t) => tabButton(p, t)));
    const strip = ui.tabs.parentElement!;
    const on = ui.tabs.querySelector<HTMLElement>(".grid-tab.on");
    // Keep the current tab in view when the strip is too narrow for all of them.
    if (on) requestAnimationFrame(() => {
      const a = on.offsetLeft - strip.offsetLeft;
      if (a < strip.scrollLeft || a + on.offsetWidth > strip.scrollLeft + strip.clientWidth) strip.scrollLeft = a - 8;
    });
    ui.link.setAttribute("aria-pressed", String(p.linked));
    ui.link.replaceChildren(icon(p.linked ? "link" : "unlink"));
    const linkText = p.linked ? "Linked: follows the shared passage. Press to keep its own." : "Not linked: keeps its own passage. Press to follow the shared one.";
    ui.link.title = linkText;
    ui.link.setAttribute("aria-label", p.linked ? "Linked to the other panes" : "Not linked to the other panes");

    let m = mounted.get(tab.id);
    if (!m) {
      m = { view: mountDoc(tab, hostFor(tab.id, tab)), paneId: p.id };
      mounted.set(tab.id, m);
    }
    setChildren(ui.body, [m.view.el]);
    setChildren(ui.place, m.view.placeBox ? [m.view.placeBox] : []);
    setChildren(ui.tools, m.view.tools ? [m.view.tools] : []);
    ui.link.hidden = !followsPassage(tab) && !p.tabs.some(followsPassage);
    ui.contents.hidden = !m.view.menu?.contents;
    return ui.el;
  }

  function paneNumber(id: string): number {
    return panes(g).findIndex((p) => p.id === id) + 1;
  }

  function tabButton(p: Pane, t: Tab): HTMLElement {
    const on = t.id === p.active;
    const label = tabLabel(t);
    const b = h("button", { type: "button", role: "tab", class: "grid-tab-btn", "aria-selected": String(on), tabindex: on ? "0" : "-1",
      title: label, draggable: "true", "data-tab": t.id }, h("span", { class: "grid-tab-label" }, label));
    b.addEventListener("click", () => setGrid(selectTab(g, t.id)));
    b.addEventListener("auxclick", (e) => { if (e.button === 1) setGrid(closeTab(g, t.id)); });
    b.addEventListener("keydown", (e) => {
      const i = p.tabs.findIndex((x) => x.id === t.id);
      const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
      if (d) {
        e.preventDefault();
        const nt = p.tabs[(i + d + p.tabs.length) % p.tabs.length];
        setGrid(selectTab(g, nt.id));
        uis.get(p.id)?.tabs.querySelector<HTMLElement>(`[data-tab="${nt.id}"]`)?.focus();
      } else if (e.key === "Delete") {
        e.preventDefault();
        setGrid(closeTab(g, t.id));
      }
    });
    b.addEventListener("dragstart", (e) => {
      setDragging({ tabId: t.id });
      e.dataTransfer?.setData(DRAG_TYPE, t.id);
      e.dataTransfer?.setData("text/plain", label);
      if (e.dataTransfer) e.dataTransfer.effectAllowed = "move";
    });
    b.addEventListener("dragend", endDrag);
    const x = h("button", { type: "button", class: "grid-tab-close", "aria-label": `Close ${label}`, title: "Close (Delete)", tabindex: "-1" }, icon("close"));
    x.addEventListener("click", (e) => {
      e.stopPropagation();
      setGrid(closeTab(g, t.id));
    });
    return h("span", { class: `grid-tab${on ? " on" : ""}`, role: "presentation" }, b, x);
  }

  function paneUI(paneId: string): PaneUI {
    const tabs = h("div", { class: "grid-tabs", role: "tablist", "aria-label": "Tabs" });
    const plus = h("button", { type: "button", class: "icon-btn grid-plus", "aria-label": "New tab", title: "New tab", "aria-haspopup": "menu", "aria-expanded": "false" }, icon("plus"));
    plus.addEventListener("click", () => togglePopover(plus, () => newTabMenu(paneId), { label: "New tab", side: "below", menu: true }));
    const place = h("div", { class: "grid-place" });
    const tools = h("div", { class: "grid-tools" });
    const link = h("button", { type: "button", class: "icon-btn grid-link", "aria-pressed": "true" });
    link.addEventListener("click", () => {
      const p = findPane(g, paneId)?.pane;
      if (p) setGrid(setLinked(g, paneId, !p.linked));
    });
    const contents: HTMLButtonElement = h("button", { type: "button", class: "icon-btn grid-contents", "aria-label": "Contents", title: "Contents: go to a book, chapter or section",
      "aria-haspopup": "dialog", "aria-expanded": "false" }, icon("contents"));
    contents.addEventListener("click", () => viewOf(paneId)?.menu?.contents?.(contents));
    const more: HTMLButtonElement = h("button", { type: "button", class: "icon-btn", "aria-label": "Pane menu", title: "More", "aria-haspopup": "menu", "aria-expanded": "false" }, icon("more"));
    more.addEventListener("click", () => togglePopover(more, () => paneMenu(paneId, more), { label: "Pane menu", side: "below", menu: true, className: "pane-menu" }));
    const bar = h("div", { class: "grid-bar" }, h("div", { class: "grid-tabstrip" }, tabs, plus), place, tools, link, contents, more);
    const body = h("div", { class: "grid-body" });
    const preview = h("div", { class: "grid-preview", hidden: true, "aria-hidden": "true" });
    const pel = h("section", { class: "grid-pane", "data-pane": paneId }, bar, body, preview);
    pel.addEventListener("pointerdown", () => select(paneId), true);
    pel.addEventListener("focusin", () => select(paneId));
    // The ⋯ menu's shortcuts work while the keyboard is in the pane.
    pel.addEventListener("keydown", (e) => {
      if (!(e.ctrlKey || e.metaKey) || e.altKey || e.shiftKey) return;
      const k = e.key.toLowerCase();
      const menu = viewOf(paneId)?.menu;
      if (k === "f" && menu?.search) { e.preventDefault(); menu.search(more); }
      else if (k === "p" && menu?.print) { e.preventDefault(); menu.print(); }
      else if (k === "w") { e.preventDefault(); setGrid(closePane(g, paneId)); focusSelected(); }
    });

    pel.addEventListener("dragover", (e) => {
      if (!dragging) return;
      const r = pel.getBoundingClientRect();
      const zone = zoneAt(e.clientX - r.left, e.clientY - r.top, r.width, r.height, bar.contains(e.target as Node));
      const ok = dragging.tabId ? dropTab(g, dragging.tabId, paneId, zone) !== null : insertTab(g, dragging.spec!, paneId, zone) !== null;
      if (!ok) {
        preview.hidden = true;
        return;
      }
      e.preventDefault();
      if (e.dataTransfer) e.dataTransfer.dropEffect = dragging.tabId ? "move" : "copy";
      preview.hidden = false;
      preview.dataset.zone = zone;
    });
    pel.addEventListener("dragleave", (e) => {
      if (!pel.contains(e.relatedTarget as Node)) preview.hidden = true;
    });
    pel.addEventListener("drop", (e) => {
      if (!dragging) return;
      e.preventDefault();
      const zone = (preview.dataset.zone as Zone) || "center";
      const d = dragging;
      endDrag();
      setGrid(d.tabId ? dropTab(g, d.tabId, paneId, zone) : insertTab(g, d.spec!, paneId, zone));
    });
    return { el: pel, tabs, place, tools, link, contents, more, body, preview };
  }

  function endDrag() {
    setDragging(null);
    for (const ui of uis.values()) ui.preview.hidden = true;
  }

  // Menus ----------------------------------------------------------------------------------------------

  function newTabMenu(paneId: string): HTMLElement {
    const add = (spec: TabSpec) => () => setGrid(openTab(g, spec, paneId));
    return h("div", { class: "menu-list" },
      button("Bible", add({ kind: "bible" })),
      button("Book from your library…", add({ kind: "book" })),
      button("Library browser", add({ kind: "library" })),
      button("My work browser", add({ kind: "work" })),
      button("New note", add({ kind: "note" })));
  }

  function viewOf(paneId: string): DocView | undefined {
    const p = findPane(g, paneId)?.pane;
    return p ? mounted.get(activeTab(p)!.id)?.view : undefined;
  }

  // paneMenu is ⋯: what this pane shows, then the layout (docwords.ts paneMenu).
  function paneMenu(paneId: string, anchor: HTMLElement): HTMLElement {
    const p = findPane(g, paneId)?.pane;
    const tab = p && activeTab(p);
    const menu = viewOf(paneId)?.menu ?? {};
    // A menu item that opens another popover waits for the menu to close first.
    const later = (fn: () => void) => () => setTimeout(fn, 0);
    // A narrow pane hides its Bibles button (style.css); it is offered here instead.
    const ui = uis.get(paneId);
    const folded = (x?: HTMLElement) => !!x && !x.hidden && getComputedStyle(x).display === "none";
    const spec = (): TabSpec | null => (tab ? (({ id: _id, ...rest }) => rest)(tab) : null);
    const groups = paneMenuGroups({
      search: !!menu.search, print: !!menu.print, link: !!menu.link?.(),
      splitRight: !!tab && insertTab(g, spec()!, paneId, "right") !== null,
      splitDown: !!tab && insertTab(g, spec()!, paneId, "bottom") !== null,
      bibles: !!menu.bibles && folded(ui?.tools),
    });
    const run: Record<string, () => void> = {
      search: later(() => menu.search?.(anchor)),
      print: () => menu.print?.(),
      copylink: () => {
        const l = menu.link?.();
        if (l) void navigator.clipboard.writeText(`${location.origin}${location.pathname}${l}`).then(() => flash("Link copied."), () => flash("The clipboard is not available here."));
      },
      bibles: later(() => menu.bibles?.(anchor)),
      "split-right": () => { const sp = spec(); if (sp) setGrid(insertTab(g, sp, paneId, "right")); },
      "split-down": () => { const sp = spec(); if (sp) setGrid(insertTab(g, sp, paneId, "bottom")); },
      move: later(() => moveDialog(paneId, anchor)),
      close: () => setGrid(closePane(g, paneId)),
    };
    return h("div", { class: "menu-list grouped-menu" }, groups.map((gr) => [
      h("div", { class: "menu-head", role: "presentation" }, gr.head),
      gr.items.map((it) => h("button", { type: "button", class: `menu-item${it.danger ? " danger" : ""}`, disabled: !!it.disabled, onclick: run[it.id],
        "aria-keyshortcuts": it.keys ? it.keys.replace("Ctrl+", "Control+") : null },
      h("span", {}, it.label), it.keys ? h("kbd", { class: "menu-keys" }, it.keys) : null)),
    ]));
  }

  // moveDialog is the keyboard way to drag the current tab: beside, above or below a pane, or into it.
  function moveDialog(paneId: string, anchor: HTMLElement) {
    const p = findPane(g, paneId)?.pane;
    const tab = p && activeTab(p);
    if (!p || !tab) return;
    const all = panes(g);
    const target = h("select", { "aria-label": "Pane" }, all.map((q, i) =>
      h("option", { value: q.id, selected: q.id === paneId }, `Pane ${i + 1}${q.id === paneId ? " (this one)" : ""}: ${tabLabel(activeTab(q)!)}`)));
    const wheres: [MoveTarget, string][] = [["into", "Into it, as a tab"], ["left", "To its left"], ["right", "To its right"], ["above", "Above it"], ["below", "Below it"]];
    const radios = h("fieldset", {}, h("legend", {}, "Where"));
    const go = h("button", { type: "submit", class: "primary" }, "Move");
    const note = h("p", { class: "hint", role: "status" });
    function refresh() {
      let first: HTMLInputElement | null = null;
      clear(radios, h("legend", {}, "Where"), wheres.map(([w, label]) => {
        const ok = moveTab(g, tab!.id, target.value, w) !== null;
        const r = h("input", { type: "radio", name: "move-where", value: w, disabled: !ok });
        if (ok && !first) first = r;
        return h("label", { class: "check" }, r, " ", label);
      }));
      if (first) (first as HTMLInputElement).checked = true;
      go.disabled = !first;
      note.textContent = first ? "" : `There is no room there: the grid holds at most ${MAX_COLUMNS} columns of ${MAX_PANES} panes.`;
    }
    target.addEventListener("change", refresh);
    refresh();
    const form = h("form", { class: "pop-body move-to" }, h("h2", { class: "pop-title" }, `Move “${tabLabel(tab)}”`), target, radios, note, h("div", { class: "row" }, go));
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      const w = (radios.querySelector<HTMLInputElement>("input:checked")?.value ?? "into") as MoveTarget;
      const n = moveTab(g, tab.id, target.value, w);
      closePopover();
      setGrid(n);
      focusSelected();
    });
    openPopover(anchor, form, { label: "Move to", side: "below" });
  }

  // presetPicker is the 3 × 3 chooser, and Make panes equal.
  function presetPicker(): HTMLElement {
    const [cols, rows] = shape(g);
    const cells: HTMLButtonElement[] = [];
    const caption = h("p", { class: "hint", "aria-live": "polite" }, `Now ${cols} × ${rows}`);
    const mark = (c: number, r: number) => {
      for (const b of cells) b.classList.toggle("lit", Number(b.dataset.c) <= c && Number(b.dataset.r) <= r);
      caption.textContent = `${c} ${c === 1 ? "column" : "columns"} of ${r} ${r === 1 ? "pane" : "panes"}`;
    };
    const chooser = h("div", { class: "preset-grid", role: "group", "aria-label": "Columns and panes" });
    for (let r = 1; r <= MAX_PANES; r++) {
      for (let c = 1; c <= MAX_COLUMNS; c++) {
        const b = h("button", { type: "button", class: "preset-cell", "data-c": c, "data-r": r, "aria-label": `${c} × ${r}`,
          "aria-pressed": String(c === cols && r === rows), title: `${c} × ${r}` });
        b.addEventListener("mouseenter", () => mark(c, r));
        b.addEventListener("focus", () => mark(c, r));
        b.addEventListener("click", () => {
          closePopover(true);
          setGrid(applyPreset(g, c, r, o.defaultTab));
        });
        cells.push(b);
        chooser.append(b);
      }
    }
    chooser.addEventListener("mouseleave", () => mark(cols, rows));
    mark(cols, rows);
    return h("div", { class: "pop-body presets" },
      h("h2", { class: "pop-title" }, "Grid"),
      chooser, caption,
      button("Make panes equal", () => { closePopover(true); setGrid(equalise(g)); }));
  }

  // Resizing -------------------------------------------------------------------------------------------

  function gutter(kind: "col" | "row", col: number, i: number): HTMLElement {
    const label = kind === "col" ? `Resize columns ${col + 1} and ${col + 2}` : `Resize panes ${i + 1} and ${i + 2} in column ${col + 1}`;
    const gt = h("div", { class: `grid-gutter ${kind}`, role: "separator", tabindex: "0", "aria-label": label,
      "aria-orientation": kind === "col" ? "vertical" : "horizontal", title: "Drag to resize. Arrow keys work too." });
    const apply = (base: Grid, delta: number) => kind === "col" ? resizeColumns(base, col, delta) : resizePanes(base, col, i, delta);
    gt.addEventListener("pointerdown", (e) => {
      e.preventDefault();
      gt.setPointerCapture(e.pointerId);
      const start = g;
      const total = kind === "col" ? el.clientWidth : (colEls.get(g.columns[col].id)?.clientHeight ?? 1);
      const from = kind === "col" ? e.clientX : e.clientY;
      let last = start;
      const move = (ev: PointerEvent) => {
        last = apply(start, ((kind === "col" ? ev.clientX : ev.clientY) - from) / Math.max(1, total));
        // Draw sizes as the pointer moves; save once it is let go.
        if (kind === "col") last.columns.forEach((c) => { const ce = colEls.get(c.id); if (ce) ce.style.flexGrow = String(c.size); });
        else last.columns[col].panes.forEach((p) => { const pe = uis.get(p.id)?.el; if (pe) pe.style.flexGrow = String(p.size); });
      };
      const up = () => {
        gt.removeEventListener("pointermove", move);
        gt.removeEventListener("pointerup", up);
        gt.removeEventListener("pointercancel", up);
        setGrid(last);
      };
      gt.addEventListener("pointermove", move);
      gt.addEventListener("pointerup", up);
      gt.addEventListener("pointercancel", up);
    });
    gt.addEventListener("keydown", (e) => {
      const back = kind === "col" ? "ArrowLeft" : "ArrowUp";
      const fwd = kind === "col" ? "ArrowRight" : "ArrowDown";
      if (e.key !== back && e.key !== fwd) return;
      e.preventDefault();
      setGrid(apply(g, e.key === fwd ? STEP : -STEP));
      el.querySelector<HTMLElement>(`.grid-gutter[aria-label="${label}"]`)?.focus();
    });
    return gt;
  }

  // The passage --------------------------------------------------------------------------------------
  // The sidebar follows the selected pane's passage, and a passage chosen elsewhere (a link in the
  // sidebar, a report, a reading plan) goes to the selected pane.

  let parseSeq = 0;
  function syncPassage() {
    const sel = findPane(g, g.selected)?.pane;
    if (!sel) return;
    const ref = paneRef(g, sel);
    if (!ref || ref === state.ref) return;
    const n = ++parseSeq;
    void run<Ref>("verse.parse", { ref }).then((p) => {
      if (n !== parseSeq || state.ref === p.display) return;
      syncing = true;
      try {
        setPassage(p.display, p);
      } finally {
        syncing = false;
      }
    }).catch(() => {});
  }

  onPassage(() => {
    if (syncing) return;
    const sel = findPane(g, g.selected)?.pane;
    if (!sel) return setGrid(openTab({ ...g, passage: state.ref }, { kind: "bible" }));
    let n = setRef(g, sel.id, state.ref);
    // Make sure something shows the passage.
    const shown = n.columns.some((c) => c.panes.some((p) => followsPassage(activeTab(p)!) && (p.linked || p.id === sel.id)));
    if (!shown) n = openTab(n, { kind: "bible" }, sel.id);
    setGrid(n);
  });

  function selectedView(): DocView | null {
    const p = findPane(g, g.selected)?.pane;
    return p ? mounted.get(activeTab(p)!.id)?.view ?? null : null;
  }

  function focusSelected() {
    queueMicrotask(() => {
      const v = selectedView();
      if (v?.focus) v.focus();
      else uis.get(g.selected)?.tabs.querySelector<HTMLElement>("[aria-selected=true]")?.focus();
    });
  }

  render();
  setSelectedView(selectedView);

  return {
    el,
    render,
    // open shows a tab in the selected pane; a book already open there is reused.
    open(spec: TabSpec) {
      navigate("reader");
      setGrid(openTab(g, spec));
      focusSelected();
    },
    presets(anchor: HTMLElement) {
      togglePopover(anchor, presetPicker, { label: "Grid presets", side: anchor.closest(".rail") ? "right" : "below" });
    },
    selectedView,
    focus: focusSelected,
    empty: () => panes(g).length === 0,
  };
}
