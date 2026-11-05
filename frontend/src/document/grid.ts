// The study grid's layout model: columns hold panes, panes hold tabs. Up to MAX_COLUMNS columns with
// up to MAX_PANES panes in each. Pure data and functions, no DOM, so it is unit-tested
// (grid.test.ts); document/studygrid.ts draws it. Every function returns a new grid and leaves the
// one it was given alone.

export const MAX_COLUMNS = 3;
export const MAX_PANES = 3;
// MIN_SIZE is the smallest share of the width (or of a column's height) a pane may be resized to.
export const MIN_SIZE = 0.12;

export type TabKind = "bible" | "commentary" | "book" | "note" | "sermon" | "list" | "library" | "work";
export const TAB_KINDS: TabKind[] = ["bible", "commentary", "book", "note", "sermon", "list", "library", "work"];

export interface Tab {
  id: string;
  kind: TabKind;
  title?: string;
  modules?: string[]; // Bibles shown, or the commentary module
  itemId?: string; // a library book
  fileId?: string;
  section?: number; // the book's section or PDF page
  para?: number;
  target?: string; // the note, sermon or list shown
  locator?: Record<string, unknown>; // where a book opens, used once and not saved
}

// TabSpec is a tab before it has an id, as a drop or a + creates it.
export type TabSpec = Omit<Tab, "id">;

export interface Pane {
  id: string;
  tabs: Tab[];
  active: string;
  linked: boolean; // linked panes follow the grid's passage; others keep their own
  ref: string; // the passage of an unlinked pane
  size: number;
}

export interface Column {
  id: string;
  panes: Pane[];
  size: number;
}

export interface Grid {
  columns: Column[];
  selected: string; // the highlighted pane, where + and the sidebar act
  passage: string; // the passage linked panes follow
  seq: number;
}

export type Zone = "center" | "left" | "right" | "top" | "bottom";

export function emptyGrid(passage = ""): Grid {
  return { columns: [], selected: "", passage, seq: 0 };
}

function clone(g: Grid): Grid {
  return JSON.parse(JSON.stringify(g)) as Grid;
}

function nextId(g: Grid, prefix: string): string {
  g.seq += 1;
  return `${prefix}${g.seq}`;
}

function newPane(g: Grid, tabs: TabSpec[] | Tab[], size = 1): Pane {
  const ts = tabs.map((t) => ({ ...t, id: nextId(g, "t") }) as Tab);
  return { id: nextId(g, "p"), tabs: ts, active: ts[0]?.id ?? "", linked: true, ref: g.passage, size };
}

// Finding --------------------------------------------------------------------------------------------

export function panes(g: Grid): Pane[] {
  return g.columns.flatMap((c) => c.panes);
}

export function findPane(g: Grid, paneId: string): { col: number; row: number; pane: Pane } | null {
  for (let col = 0; col < g.columns.length; col++) {
    const row = g.columns[col].panes.findIndex((p) => p.id === paneId);
    if (row >= 0) return { col, row, pane: g.columns[col].panes[row] };
  }
  return null;
}

export function findTab(g: Grid, tabId: string): { pane: Pane; index: number; tab: Tab } | null {
  for (const p of panes(g)) {
    const index = p.tabs.findIndex((t) => t.id === tabId);
    if (index >= 0) return { pane: p, index, tab: p.tabs[index] };
  }
  return null;
}

export function selectedPane(g: Grid): Pane | null {
  return findPane(g, g.selected)?.pane ?? panes(g)[0] ?? null;
}

export function activeTab(p: Pane): Tab | null {
  return p.tabs.find((t) => t.id === p.active) ?? p.tabs[0] ?? null;
}

// paneRef is the passage a pane shows: the grid's when linked, its own otherwise.
export function paneRef(g: Grid, p: Pane): string {
  return p.linked ? g.passage : p.ref || g.passage;
}

// Tabs -----------------------------------------------------------------------------------------------

// openTab adds a tab to a pane (the selected one by default) and makes it current. With no panes,
// it makes the first one.
export function openTab(g: Grid, spec: TabSpec, paneId = g.selected): Grid {
  const n = clone(g);
  let target = findPane(n, paneId)?.pane ?? selectedPane(n);
  if (!target) {
    target = newPane(n, []);
    n.columns.push({ id: nextId(n, "c"), panes: [target], size: 1 });
  }
  const tab = { ...spec, id: nextId(n, "t") } as Tab;
  target.tabs.push(tab);
  target.active = tab.id;
  n.selected = target.id;
  return n;
}

// openBookTab opens a book in the selected pane, or shows the tab already there for it when no
// particular place is asked for, so reading the same book again does not pile up tabs.
export function openBookTab(g: Grid, spec: TabSpec): Grid {
  const p = selectedPane(g);
  const same = !spec.locator && !spec.section && spec.itemId
    ? p?.tabs.find((t) => t.kind === "book" && t.itemId === spec.itemId && (!spec.fileId || !t.fileId || t.fileId === spec.fileId))
    : undefined;
  return same ? selectTab(g, same.id) : openTab(g, spec);
}

export function selectTab(g: Grid, tabId: string): Grid {
  const f = findTab(g, tabId);
  if (!f) return g;
  const n = clone(g);
  const p = findPane(n, f.pane.id)!.pane;
  p.active = tabId;
  n.selected = p.id;
  return n;
}

export function selectPane(g: Grid, paneId: string): Grid {
  if (!findPane(g, paneId) || g.selected === paneId) return g;
  return { ...g, selected: paneId };
}

// updateTab changes a tab's fields (its place in a book, its title, the Bibles it shows).
export function updateTab(g: Grid, tabId: string, patch: Partial<TabSpec>): Grid {
  const f = findTab(g, tabId);
  if (!f) return g;
  const n = clone(g);
  const t = findTab(n, tabId)!.tab;
  Object.assign(t, patch);
  return n;
}

// removeTab takes a tab out of its pane; an empty pane closes, and an empty column with it.
function removeTab(g: Grid, tabId: string): Tab | null {
  for (const c of g.columns) {
    for (const p of c.panes) {
      const i = p.tabs.findIndex((t) => t.id === tabId);
      if (i < 0) continue;
      const [tab] = p.tabs.splice(i, 1);
      if (p.active === tabId) p.active = (p.tabs[i] ?? p.tabs[i - 1])?.id ?? "";
      if (p.tabs.length === 0) removePane(g, p.id);
      return tab;
    }
  }
  return null;
}

function removePane(g: Grid, paneId: string) {
  const f = findPane(g, paneId);
  if (!f) return;
  const col = g.columns[f.col];
  col.panes.splice(f.row, 1);
  // The space goes to the neighbour, so the other panes keep their sizes.
  const heir = col.panes[f.row - 1] ?? col.panes[f.row];
  if (heir) heir.size += f.pane.size;
  if (col.panes.length === 0) {
    g.columns.splice(f.col, 1);
    const cheir = g.columns[f.col - 1] ?? g.columns[f.col];
    if (cheir) cheir.size += col.size;
  }
  if (g.selected === paneId) {
    const near = g.columns[Math.min(f.col, g.columns.length - 1)]?.panes[0];
    g.selected = (heir ?? near ?? panes(g)[0])?.id ?? "";
  }
}

export function closeTab(g: Grid, tabId: string): Grid {
  if (!findTab(g, tabId)) return g;
  const n = clone(g);
  removeTab(n, tabId);
  return n;
}

export function closePane(g: Grid, paneId: string): Grid {
  if (!findPane(g, paneId)) return g;
  const n = clone(g);
  removePane(n, paneId);
  return n;
}

// Linking and places ---------------------------------------------------------------------------------

export function setLinked(g: Grid, paneId: string, linked: boolean): Grid {
  const f = findPane(g, paneId);
  if (!f || f.pane.linked === linked) return g;
  const n = clone(g);
  const p = findPane(n, paneId)!.pane;
  // Leaving the group keeps the passage it showed; joining it takes the group's.
  if (!linked) p.ref = n.passage;
  p.linked = linked;
  return n;
}

// setRef moves a pane to a passage: every linked pane follows when it is linked.
export function setRef(g: Grid, paneId: string, ref: string): Grid {
  const f = findPane(g, paneId);
  const n = clone(g);
  if (!f || f.pane.linked) n.passage = ref;
  else findPane(n, paneId)!.pane.ref = ref;
  return n;
}

// Drag and drop --------------------------------------------------------------------------------------

// place puts a tab into the grid at a pane's middle or edge. It returns false when the grid is full
// in that direction.
function place(g: Grid, tab: Tab, targetId: string, zone: Zone): boolean {
  const f = findPane(g, targetId);
  if (!f) return false;
  const col = g.columns[f.col];
  if (zone === "center") {
    f.pane.tabs.push(tab);
    f.pane.active = tab.id;
    g.selected = f.pane.id;
    return true;
  }
  if (zone === "left" || zone === "right") {
    if (g.columns.length >= MAX_COLUMNS) return false;
    const size = col.size / 2;
    col.size = size;
    const p = newPane(g, []);
    p.tabs.push(tab);
    p.active = tab.id;
    const c: Column = { id: nextId(g, "c"), panes: [p], size };
    g.columns.splice(zone === "left" ? f.col : f.col + 1, 0, c);
    g.selected = p.id;
    return true;
  }
  if (col.panes.length >= MAX_PANES) return false;
  const size = f.pane.size / 2;
  f.pane.size = size;
  const p = newPane(g, [], size);
  p.tabs.push(tab);
  p.active = tab.id;
  col.panes.splice(zone === "top" ? f.row : f.row + 1, 0, p);
  g.selected = p.id;
  return true;
}

// dropTab moves a tab onto another pane's middle (it joins that pane) or edge (a new pane or column
// is made). It returns null when the drop is not allowed or would change nothing.
export function dropTab(g: Grid, tabId: string, targetId: string, zone: Zone): Grid | null {
  const src = findTab(g, tabId);
  const dst = findPane(g, targetId);
  if (!src || !dst) return null;
  if (src.pane.id === targetId && (zone === "center" || src.pane.tabs.length === 1)) return null;
  const n = clone(g);
  const tab = removeTab(n, tabId)!;
  if (!findPane(n, targetId)) return null;
  return place(n, tab, targetId, zone) ? n : null;
}

// insertTab drops a new tab (from the Library browser or a Related card) onto a pane.
export function insertTab(g: Grid, spec: TabSpec, targetId: string, zone: Zone): Grid | null {
  if (!findPane(g, targetId)) return null;
  const n = clone(g);
  const tab = { ...spec, id: nextId(n, "t") } as Tab;
  return place(n, tab, targetId, zone) ? n : null;
}

export function canDrop(g: Grid, tabId: string | null, targetId: string, zone: Zone): boolean {
  return tabId ? dropTab(g, tabId, targetId, zone) !== null : insertTab(g, { kind: "library" }, targetId, zone) !== null;
}

// Move to… is the keyboard way to drag: beside, above or below a pane, or into it.
export type MoveTarget = "left" | "right" | "above" | "below" | "into";

export function moveTab(g: Grid, tabId: string, targetId: string, where: MoveTarget): Grid | null {
  const zone: Zone = where === "above" ? "top" : where === "below" ? "bottom" : where === "into" ? "center" : where;
  return dropTab(g, tabId, targetId, zone);
}

// Presets --------------------------------------------------------------------------------------------

// applyPreset lays the grid out as cols columns of rows panes. Open tabs are spread out: panes with
// several tabs are split before new panes get a default tab, and panes beyond the layout are merged
// into its last pane.
export function applyPreset(g: Grid, cols: number, rows: number, makeTab: () => TabSpec): Grid {
  cols = Math.min(MAX_COLUMNS, Math.max(1, Math.round(cols)));
  rows = Math.min(MAX_PANES, Math.max(1, Math.round(rows)));
  const want = cols * rows;
  const n = clone(g);
  const list = panes(n);
  while (list.length < want) {
    const i = list.findIndex((p) => p.tabs.length > 1);
    if (i < 0) {
      list.push(newPane(n, [makeTab()]));
      continue;
    }
    const from = list[i];
    const tab = from.tabs.pop()!;
    if (from.active === tab.id) from.active = from.tabs[from.tabs.length - 1].id;
    const p = newPane(n, []);
    p.tabs.push(tab);
    p.active = tab.id;
    p.linked = from.linked;
    p.ref = from.ref;
    list.splice(i + 1, 0, p);
  }
  if (list.length > want) {
    const last = list[want - 1];
    for (const extra of list.splice(want)) last.tabs.push(...extra.tabs);
    if (list.every((p) => p.id !== n.selected)) n.selected = last.id;
  }
  n.columns = [];
  for (let c = 0; c < cols; c++) {
    const ps = list.slice(c * rows, c * rows + rows);
    ps.forEach((p) => (p.size = 1 / rows));
    n.columns.push({ id: nextId(n, "c"), panes: ps, size: 1 / cols });
  }
  if (!findPane(n, n.selected)) n.selected = list[0].id;
  return n;
}

// equalise makes every column the same width and every pane in a column the same height.
export function equalise(g: Grid): Grid {
  const n = clone(g);
  for (const c of n.columns) {
    c.size = 1 / n.columns.length;
    for (const p of c.panes) p.size = 1 / c.panes.length;
  }
  return n;
}

// shape is [columns, most panes in a column], to mark the current preset.
export function shape(g: Grid): [number, number] {
  return [g.columns.length, Math.max(0, ...g.columns.map((c) => c.panes.length))];
}

// Resizing -------------------------------------------------------------------------------------------

// resizeBetween moves delta (a share of the whole) from sizes[i+1] to sizes[i], keeping both at
// least min. Sizes are shares that add up to 1.
export function resizeBetween(sizes: number[], i: number, delta: number, min = MIN_SIZE): number[] {
  if (i < 0 || i + 1 >= sizes.length) return sizes;
  const out = normalise(sizes);
  const pair = out[i] + out[i + 1];
  const a = Math.min(pair - min, Math.max(min, out[i] + delta));
  out[i] = a;
  out[i + 1] = pair - a;
  return out;
}

function normalise(sizes: number[]): number[] {
  const ok = sizes.map((s) => (Number.isFinite(s) && s > 0 ? s : 1 / sizes.length));
  const sum = ok.reduce((a, b) => a + b, 0);
  return ok.map((s) => s / sum);
}

export function resizeColumns(g: Grid, i: number, delta: number): Grid {
  const n = clone(g);
  const sizes = resizeBetween(n.columns.map((c) => c.size), i, delta);
  n.columns.forEach((c, j) => (c.size = sizes[j]));
  return n;
}

export function resizePanes(g: Grid, col: number, i: number, delta: number): Grid {
  const n = clone(g);
  const c = n.columns[col];
  if (!c) return g;
  const sizes = resizeBetween(c.panes.map((p) => p.size), i, delta);
  c.panes.forEach((p, j) => (p.size = sizes[j]));
  return n;
}

// Saving ---------------------------------------------------------------------------------------------

const VERSION = 1;

export function serialise(g: Grid): string {
  return JSON.stringify({ v: VERSION, ...g });
}

// restore reads a saved grid. Anything damaged or unknown is dropped, the 3 × 3 limit is applied, ids
// are made unique, and sizes are made to add up. It never throws: a bad save gives an empty grid.
export function restore(saved: unknown): Grid {
  let o: unknown = saved;
  if (typeof saved === "string") {
    try {
      o = JSON.parse(saved);
    } catch {
      return emptyGrid();
    }
  }
  if (!o || typeof o !== "object") return emptyGrid();
  const s = o as Partial<Grid> & { v?: number };
  const g = emptyGrid(typeof s.passage === "string" ? s.passage : "");
  const seen = new Set<string>();
  const cols: Column[] = [];
  for (const c of Array.isArray(s.columns) ? s.columns : []) {
    const ps: Pane[] = [];
    for (const p of Array.isArray(c?.panes) ? c.panes : []) {
      const tabs: Tab[] = [];
      for (const t of Array.isArray(p?.tabs) ? p.tabs : []) {
        if (!t || typeof t !== "object" || !TAB_KINDS.includes(t.kind)) continue;
        const id = typeof t.id === "string" && !seen.has(t.id) ? t.id : "";
        const tab = { ...tidyTab(t), id: id || nextId(g, "t") } as Tab;
        seen.add(tab.id);
        tabs.push(tab);
      }
      if (tabs.length === 0) continue;
      const pid = typeof p.id === "string" && !seen.has(p.id) ? p.id : nextId(g, "p");
      seen.add(pid);
      const active = tabs.some((t) => t.id === p.active) ? p.active : tabs[0].id;
      ps.push({ id: pid, tabs, active, linked: p.linked !== false, ref: typeof p.ref === "string" ? p.ref : "", size: num(p.size) });
    }
    if (ps.length === 0) continue;
    const cid = typeof c.id === "string" && !seen.has(c.id) ? c.id : nextId(g, "c");
    seen.add(cid);
    cols.push({ id: cid, panes: ps, size: num(c.size) });
  }
  // Too many panes in a column, or too many columns, are merged into the last one allowed.
  for (const c of cols) {
    if (c.panes.length > MAX_PANES) {
      const last = c.panes[MAX_PANES - 1];
      for (const extra of c.panes.splice(MAX_PANES)) last.tabs.push(...extra.tabs);
    }
  }
  if (cols.length > MAX_COLUMNS) {
    const last = cols[MAX_COLUMNS - 1];
    const lastPane = last.panes[last.panes.length - 1];
    for (const extra of cols.splice(MAX_COLUMNS)) for (const p of extra.panes) lastPane.tabs.push(...p.tabs);
  }
  const csizes = normalise(cols.map((c) => c.size));
  cols.forEach((c, i) => {
    c.size = csizes[i];
    const ps = normalise(c.panes.map((p) => p.size));
    c.panes.forEach((p, j) => (p.size = ps[j]));
  });
  g.columns = cols;
  const max = [...seen].map((id) => Number(id.replace(/^\D+/, ""))).filter(Number.isFinite);
  g.seq = Math.max(num(s.seq, 0), 0, ...max);
  g.selected = typeof s.selected === "string" && findPane(g, s.selected) ? s.selected : panes(g)[0]?.id ?? "";
  return g;
}

function num(v: unknown, fallback = 0): number {
  return typeof v === "number" && Number.isFinite(v) && v > 0 ? v : fallback;
}

function tidyTab(t: Partial<Tab>): TabSpec {
  const out: TabSpec = { kind: t.kind as TabKind };
  if (typeof t.title === "string") out.title = t.title;
  if (Array.isArray(t.modules)) out.modules = t.modules.filter((m) => typeof m === "string");
  for (const k of ["itemId", "fileId", "target"] as const) if (typeof t[k] === "string") out[k] = t[k];
  for (const k of ["section", "para"] as const) if (typeof t[k] === "number" && Number.isFinite(t[k])) out[k] = t[k];
  return out;
}
