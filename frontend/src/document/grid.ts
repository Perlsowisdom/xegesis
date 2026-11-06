// The study grid's layout model: a tree of splits. A leaf is a pane (it holds tabs); a split lays its
// children side by side ("h") or one above the other ("v"). At most MAX_PANES panes in all, and at
// most MAX_SIBLINGS in one split, so the old 3 × 3 grid still fits and nothing gets crowded. Pure data
// and functions, no DOM, so it is unit-tested (grid.test.ts); document/studygrid.ts draws it. Every
// function returns a new grid and leaves the one it was given alone.

export const MAX_PANES = 9;
export const MAX_SIBLINGS = 3;
// The presets are up to PRESET_COLUMNS columns of up to PRESET_ROWS panes.
export const PRESET_COLUMNS = 3;
export const PRESET_ROWS = 3;
// MIN_SIZE is the smallest share of its split a child may be resized to.
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
  anchor?: string; // the element a book opens at, used once and not saved
  offset?: number; // with anchor: characters into that paragraph, used once and not saved
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

// A Split lays out its children: "h" side by side, "v" one above the other. size is its share of its
// parent split; the shares of a split's children add up to 1.
export interface Split {
  id: string;
  dir: "h" | "v";
  kids: GridNode[];
  size: number;
}

export type GridNode = Pane | Split;

export function isSplit(n: GridNode): n is Split {
  return "kids" in n;
}

export interface Grid {
  root: GridNode | null;
  selected: string; // the highlighted pane, where + and the sidebar act
  passage: string; // the passage linked panes follow
  seq: number;
}

export type Zone = "center" | "left" | "right" | "top" | "bottom";

export function emptyGrid(passage = ""): Grid {
  return { root: null, selected: "", passage, seq: 0 };
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

function leaves(n: GridNode | null): Pane[] {
  if (!n) return [];
  return isSplit(n) ? n.kids.flatMap(leaves) : [n];
}

export function panes(g: Grid): Pane[] {
  return leaves(g.root);
}

// findPane gives a pane with the split it sits in (null when it is the whole grid) and its place there.
export function findPane(g: Grid, paneId: string): { parent: Split | null; index: number; pane: Pane } | null {
  const walk = (n: GridNode, parent: Split | null, index: number): { parent: Split | null; index: number; pane: Pane } | null => {
    if (!isSplit(n)) return n.id === paneId ? { parent, index, pane: n } : null;
    for (let i = 0; i < n.kids.length; i++) {
      const f = walk(n.kids[i], n, i);
      if (f) return f;
    }
    return null;
  };
  return g.root ? walk(g.root, null, 0) : null;
}

// findSplit finds a split by id.
export function findSplit(g: Grid, id: string): Split | null {
  const walk = (n: GridNode): Split | null => {
    if (!isSplit(n)) return null;
    if (n.id === id) return n;
    for (const k of n.kids) {
      const f = walk(k);
      if (f) return f;
    }
    return null;
  };
  return g.root ? walk(g.root) : null;
}

// tidy keeps the tree in its simplest form: a split with one child is replaced by it, a split inside
// a split of the same direction is merged into it, and every split's shares add up to 1.
function tidy(n: GridNode): GridNode {
  if (!isSplit(n)) return n;
  const kids: GridNode[] = [];
  for (const raw of n.kids) {
    const k = tidy(raw);
    if (isSplit(k) && k.dir === n.dir) {
      for (const kk of k.kids) kids.push({ ...kk, size: kk.size * k.size });
    } else kids.push(k);
  }
  if (kids.length === 1) return { ...kids[0], size: n.size };
  const sizes = normalise(kids.map((k) => k.size));
  kids.forEach((k, i) => (k.size = sizes[i]));
  return { ...n, kids };
}

function tidyGrid(g: Grid) {
  g.root = g.root ? tidy(g.root) : null;
  if (g.root && !isSplit(g.root)) g.root.size = 1;
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
    n.root = target;
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

// removeTab takes a tab out of its pane; an empty pane closes, and a split left with one child goes.
function removeTab(g: Grid, tabId: string): Tab | null {
  for (const p of panes(g)) {
    const i = p.tabs.findIndex((t) => t.id === tabId);
    if (i < 0) continue;
    const [tab] = p.tabs.splice(i, 1);
    if (p.active === tabId) p.active = (p.tabs[i] ?? p.tabs[i - 1])?.id ?? "";
    if (p.tabs.length === 0) removePane(g, p.id);
    return tab;
  }
  return null;
}

function removePane(g: Grid, paneId: string) {
  const f = findPane(g, paneId);
  if (!f) return;
  if (!f.parent) {
    g.root = null;
    g.selected = "";
    return;
  }
  const kids = f.parent.kids;
  kids.splice(f.index, 1);
  // The space goes to the neighbour, so the other panes keep their sizes.
  const heir = kids[f.index - 1] ?? kids[f.index];
  heir.size += f.pane.size;
  tidyGrid(g);
  if (g.selected === paneId) g.selected = leaves(heir)[0]?.id ?? panes(g)[0]?.id ?? "";
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

// place puts a tab into the grid at a pane's middle or edge: the pane is split side by side (left,
// right) or one above the other (top, bottom). It returns false when the grid is full that way.
function place(g: Grid, tab: Tab, targetId: string, zone: Zone): boolean {
  const f = findPane(g, targetId);
  if (!f) return false;
  if (zone === "center") {
    f.pane.tabs.push(tab);
    f.pane.active = tab.id;
    g.selected = f.pane.id;
    return true;
  }
  if (panes(g).length >= MAX_PANES) return false;
  const dir = zone === "left" || zone === "right" ? "h" : "v";
  const before = zone === "left" || zone === "top";
  const size = f.pane.size / 2;
  const p = newPane(g, [], size);
  p.tabs.push(tab);
  p.active = tab.id;
  if (f.parent && f.parent.dir === dir) {
    if (f.parent.kids.length >= MAX_SIBLINGS) return false;
    f.pane.size = size;
    f.parent.kids.splice(before ? f.index : f.index + 1, 0, p);
  } else {
    const old: Pane = { ...f.pane, size: 0.5 };
    p.size = 0.5;
    const s: Split = { id: nextId(g, "s"), dir, kids: before ? [p, old] : [old, p], size: f.pane.size };
    if (f.parent) f.parent.kids[f.index] = s;
    else g.root = s;
  }
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
  cols = Math.min(PRESET_COLUMNS, Math.max(1, Math.round(cols)));
  rows = Math.min(PRESET_ROWS, Math.max(1, Math.round(rows)));
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
  const columns: GridNode[] = [];
  for (let c = 0; c < cols; c++) {
    const ps = list.slice(c * rows, c * rows + rows);
    ps.forEach((p) => (p.size = 1 / rows));
    columns.push(ps.length === 1 ? { ...ps[0], size: 1 / cols } : { id: nextId(n, "s"), dir: "v", kids: ps, size: 1 / cols });
  }
  n.root = columns.length === 1 ? { ...columns[0], size: 1 } : { id: nextId(n, "s"), dir: "h", kids: columns, size: 1 };
  tidyGrid(n);
  if (!findPane(n, n.selected)) n.selected = list[0].id;
  return n;
}

// equalise makes every split's children the same size.
export function equalise(g: Grid): Grid {
  const n = clone(g);
  const walk = (x: GridNode) => {
    if (!isSplit(x)) return;
    x.kids.forEach((k) => { k.size = 1 / x.kids.length; walk(k); });
  };
  if (n.root) walk(n.root);
  return n;
}

// shape is [columns, most panes in a column] when the layout is one the presets make (a row of
// columns, each a stack of panes), to mark the current preset; [0, 0] for any other layout.
export function shape(g: Grid): [number, number] {
  const r = g.root;
  if (!r) return [0, 0];
  if (!isSplit(r)) return [1, 1];
  const stack = (k: GridNode) => (!isSplit(k) ? 1 : k.dir === "v" && k.kids.every((x) => !isSplit(x)) ? k.kids.length : 0);
  if (r.dir === "v") return r.kids.every((k) => !isSplit(k)) ? [1, r.kids.length] : [0, 0];
  const rows = r.kids.map(stack);
  return rows.every((x) => x > 0) ? [r.kids.length, Math.max(...rows)] : [0, 0];
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

// resizeSplit moves the divider after child i of a split by delta (a share of the split).
export function resizeSplit(g: Grid, splitId: string, i: number, delta: number): Grid {
  const n = clone(g);
  const s = findSplit(n, splitId);
  if (!s) return g;
  const sizes = resizeBetween(s.kids.map((k) => k.size), i, delta);
  s.kids.forEach((k, j) => (k.size = sizes[j]));
  return n;
}

// Saving ---------------------------------------------------------------------------------------------

const VERSION = 2;

export function serialise(g: Grid): string {
  return JSON.stringify({ v: VERSION, ...g });
}

// restore reads a saved grid. Anything damaged or unknown is dropped, the pane limits are applied, ids
// are made unique, and sizes are made to add up. It never throws: a bad save gives an empty grid.
// Layouts saved before splits (columns of panes) open as a row of columns.
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
  const s = o as { root?: unknown; columns?: unknown; passage?: unknown; seq?: unknown; selected?: unknown };
  const g = emptyGrid(typeof s.passage === "string" ? s.passage : "");
  const seen = new Set<string>();
  const fresh = (id: unknown, prefix: string) => {
    const v = typeof id === "string" && id && !seen.has(id) ? id : nextId(g, prefix);
    seen.add(v);
    return v;
  };
  const pane = (p: Partial<Pane> | null | undefined): Pane | null => {
    const tabs: Tab[] = [];
    for (const t of Array.isArray(p?.tabs) ? p!.tabs : []) {
      if (!t || typeof t !== "object" || !TAB_KINDS.includes(t.kind)) continue;
      const tab = { ...tidyTab(t), id: fresh(t.id, "t") } as Tab;
      tabs.push(tab);
    }
    if (tabs.length === 0) return null;
    const active = tabs.some((t) => t.id === p!.active) ? p!.active! : tabs[0].id;
    return { id: fresh(p!.id, "p"), tabs, active, linked: p!.linked !== false, ref: typeof p!.ref === "string" ? p!.ref : "", size: num(p!.size) };
  };
  const node = (raw: unknown, depth: number): GridNode | null => {
    if (!raw || typeof raw !== "object" || depth > 12) return null;
    const r = raw as { kids?: unknown; dir?: unknown; id?: unknown; size?: unknown };
    if (!Array.isArray(r.kids)) return pane(raw as Partial<Pane>);
    const kids = r.kids.map((k) => node(k, depth + 1)).filter((k): k is GridNode => !!k);
    if (kids.length === 0) return null;
    return { id: fresh(r.id, "s"), dir: r.dir === "v" ? "v" : "h", kids, size: num(r.size) };
  };
  let root: GridNode | null = null;
  if (s.root) root = node(s.root, 0);
  else if (Array.isArray(s.columns)) {
    // From before splits: a row of columns, each a stack of panes.
    const cols = (s.columns as { id?: unknown; panes?: unknown[]; size?: unknown }[]).map((c) =>
      node({ id: c?.id, dir: "v", size: c?.size, kids: Array.isArray(c?.panes) ? c.panes : [] }, 1)).filter((c): c is GridNode => !!c);
    if (cols.length) root = { id: fresh(undefined, "s"), dir: "h", kids: cols, size: 1 };
  }
  g.root = root;
  if (g.root) {
    limit(g);
    tidyGrid(g);
  }
  const max = [...seen].map((id) => Number(id.replace(/^\D+/, ""))).filter(Number.isFinite);
  g.seq = Math.max(num(s.seq, 0), 0, ...max);
  g.selected = typeof s.selected === "string" && findPane(g, s.selected) ? s.selected : panes(g)[0]?.id ?? "";
  return g;
}

// limit applies the pane limits to a saved grid: a split keeps its first MAX_SIBLINGS children and
// the grid its first MAX_PANES panes; the tabs of what is left out go into the last pane kept.
function limit(g: Grid) {
  const spill: Tab[] = [];
  const cutKids = (n: GridNode) => {
    if (!isSplit(n)) return;
    const extra = n.kids.splice(MAX_SIBLINGS).flatMap(leaves).flatMap((p) => p.tabs);
    if (extra.length) (l => l[l.length - 1])(leaves(n.kids[n.kids.length - 1])).tabs.push(...extra);
    n.kids.forEach(cutKids);
  };
  const without = (n: GridNode, id: string): GridNode | null => {
    if (!isSplit(n)) return n.id === id ? null : n;
    n.kids = n.kids.map((k) => without(k, id)).filter((k): k is GridNode => !!k);
    return n.kids.length ? n : null;
  };
  cutKids(g.root!);
  let all = leaves(g.root);
  while (all.length > MAX_PANES) {
    const last = all[all.length - 1];
    spill.push(...last.tabs);
    g.root = without(g.root!, last.id);
    all = leaves(g.root);
  }
  all[all.length - 1]?.tabs.push(...spill);
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
