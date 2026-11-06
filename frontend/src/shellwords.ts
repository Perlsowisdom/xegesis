// Pure logic for the interface shell: the places on the rail, which view belongs to which place,
// the per-device preferences for the rail, the right sidebar and reading, and their defaults. No DOM
// here, so it is unit-tested (shellwords.test.ts); main.ts, shell/*.ts and the views use it.

export type PlaceId = "search" | "library" | "reader" | "work" | "settings";
export type Width = "condensed" | "expanded";
export type ReaderMode = "read" | "study";
export type ToolId = "chat" | "related" | "notes" | "highlights" | "word" | "compare";

export interface Place {
  id: PlaceId;
  label: string;
  // The views reached inside this place, the first being where it opens. The Library shows its views
  // as tabs; My work shows them in its own column.
  views: { id: string; label: string }[];
  tabs: boolean;
}

// The places on the rail, top to bottom; Settings is reached from Rail settings, Help or Ctrl+,.
export const PLACES: Place[] = [
  { id: "search", label: "Search", views: [{ id: "search", label: "Search" }], tabs: false },
  { id: "library", label: "Library", views: [{ id: "library", label: "Your library" }, { id: "bibles", label: "Get Bibles" }], tabs: true },
  { id: "reader", label: "Reader", views: [{ id: "reader", label: "Reader" }], tabs: false },
  // My work shows its own column of folders, tags and writing (views/work.ts) instead of tabs.
  {
    id: "work", label: "My work", tabs: false,
    views: [{ id: "notes", label: "Notes" }, { id: "sermons", label: "Sermons" }, { id: "highlights", label: "Highlights" }, { id: "lists", label: "Lists" },
      { id: "plans", label: "Reading plans" }, { id: "reports", label: "Reports" }, { id: "bin", label: "Bin" }],
  },
  { id: "settings", label: "Settings", views: [{ id: "settings", label: "Settings" }, { id: "commands", label: "Commands" }], tabs: false },
];

// RAIL_PLACES are the places shown on the rail and in the phone dock; Alt+1 to Alt+4 reach them.
export const RAIL_PLACES: PlaceId[] = ["search", "library", "reader", "work"];
// Places the user may hide from the rail. The Reader always stays.
export const HIDEABLE: PlaceId[] = ["search", "library", "work"];

export function placeOf(view: string): Place {
  return PLACES.find((p) => p.views.some((v) => v.id === view)) ?? PLACES[2];
}

export function isView(view: string): boolean {
  return PLACES.some((p) => p.views.some((v) => v.id === view));
}

export function viewLabel(view: string): string {
  for (const p of PLACES) for (const v of p.views) if (v.id === view) return v.label;
  return "Reader";
}

// placeForKey maps Alt+1 to Alt+4 to a rail place; anything else gives null.
export function placeForKey(key: string): PlaceId | null {
  const n = Number(key);
  return Number.isInteger(n) && n >= 1 && n <= RAIL_PLACES.length ? RAIL_PLACES[n - 1] : null;
}

// Rail ---------------------------------------------------------------------------------------------

export interface RailPrefs {
  width: Width;
  autoHide: boolean;
  hidden: PlaceId[];
  gridOnRail: boolean;
}

export const RAIL_DEFAULTS: RailPrefs = { width: "condensed", autoHide: false, hidden: [], gridOnRail: true };

export function normalizeRail(v: unknown): RailPrefs {
  const o = (v && typeof v === "object" ? v : {}) as Partial<Record<keyof RailPrefs, unknown>>;
  return {
    width: o.width === "expanded" ? "expanded" : "condensed",
    autoHide: o.autoHide === true,
    hidden: Array.isArray(o.hidden) ? HIDEABLE.filter((p) => (o.hidden as unknown[]).includes(p)) : [],
    gridOnRail: o.gridOnRail !== false,
  };
}

export function railPlaces(p: RailPrefs): PlaceId[] {
  return RAIL_PLACES.filter((id) => !p.hidden.includes(id));
}

export function toggleHidden(p: RailPrefs, id: PlaceId, hide: boolean): RailPrefs {
  if (!HIDEABLE.includes(id)) return p;
  const hidden = p.hidden.filter((x) => x !== id);
  if (hide) hidden.push(id);
  return { ...p, hidden: HIDEABLE.filter((x) => hidden.includes(x)) };
}

// Where the grid presets button goes in Study mode: on the rail (the default), or in the Reader's
// top bar when the setting is off. Read mode has no grid.
export function gridButtonPlace(mode: ReaderMode, p: RailPrefs): "rail" | "toolbar" | null {
  if (mode !== "study") return null;
  return p.gridOnRail ? "rail" : "toolbar";
}

// Right sidebar ------------------------------------------------------------------------------------

export const TOOLS: { id: ToolId; label: string }[] = [
  { id: "chat", label: "AI chat" },
  { id: "related", label: "Related" },
  { id: "notes", label: "Notes" },
  { id: "highlights", label: "Highlights" },
  { id: "word", label: "Word" },
  { id: "compare", label: "Compare" },
];

export interface ToolSetting { id: ToolId; shown: boolean }

export interface SidebarPrefs {
  open: boolean;
  tool: ToolId;
  tools: ToolSetting[];
}

export const SIDEBAR_DEFAULTS: SidebarPrefs = {
  open: false,
  tool: "related",
  tools: TOOLS.map((t) => ({ id: t.id, shown: true })),
};

export function toolLabel(id: ToolId): string {
  return TOOLS.find((t) => t.id === id)?.label ?? id;
}

// normalizeTools keeps the saved order, drops unknown or repeated tools and adds any new tool, shown,
// after the tool it follows in TOOLS (Highlights after Notes).
export function normalizeTools(v: unknown): ToolSetting[] {
  const out: ToolSetting[] = [];
  const known = new Set<string>(TOOLS.map((t) => t.id));
  if (Array.isArray(v)) {
    for (const x of v) {
      const id = x && typeof x === "object" ? (x as { id?: unknown }).id : undefined;
      if (typeof id !== "string" || !known.has(id) || out.some((t) => t.id === id)) continue;
      out.push({ id: id as ToolId, shown: (x as { shown?: unknown }).shown !== false });
    }
  }
  TOOLS.forEach((t, i) => {
    if (out.some((o) => o.id === t.id)) return;
    const after = i > 0 ? out.findIndex((o) => o.id === TOOLS[i - 1].id) : -1;
    out.splice(after + 1, 0, { id: t.id, shown: true });
  });
  return out;
}

export function normalizeSidebar(v: unknown): SidebarPrefs {
  const o = (v && typeof v === "object" ? v : {}) as Partial<Record<keyof SidebarPrefs, unknown>>;
  const tools = normalizeTools(o.tools);
  const p: SidebarPrefs = { open: o.open === true, tool: "related", tools };
  p.tool = activeTool(tools, typeof o.tool === "string" ? (o.tool as ToolId) : "related");
  return p;
}

export function visibleTools(tools: ToolSetting[]): ToolId[] {
  return tools.filter((t) => t.shown).map((t) => t.id);
}

// activeTool is the wanted tool when it is shown, else the first shown one ("related" if none are).
export function activeTool(tools: ToolSetting[], wanted: ToolId): ToolId {
  const shown = visibleTools(tools);
  return shown.includes(wanted) ? wanted : shown[0] ?? "related";
}

export function moveTool(tools: ToolSetting[], id: ToolId, by: -1 | 1): ToolSetting[] {
  const i = tools.findIndex((t) => t.id === id);
  const j = i + by;
  if (i < 0 || j < 0 || j >= tools.length) return tools;
  const out = tools.slice();
  [out[i], out[j]] = [out[j], out[i]];
  return out;
}

export function showTool(tools: ToolSetting[], id: ToolId, shown: boolean): ToolSetting[] {
  return tools.map((t) => (t.id === id ? { ...t, shown } : t));
}

// Reading ------------------------------------------------------------------------------------------

export interface ReadingPrefs {
  mode: ReaderMode;
  size: number; // text size in percent
  font: "serif" | "sans";
  width: "narrow" | "wide";
  spacing: Spacing;
  theme: PageTheme; // the page colour in Read mode; "auto" follows the app
  taggedWords: boolean; // mark words tagged with Strong's numbers, and look them up on a click
}

export type Spacing = "tight" | "normal" | "loose";
export type PageTheme = "auto" | "light" | "sepia" | "dark";
export const SPACINGS: Record<Spacing, number> = { tight: 1.45, normal: 1.75, loose: 2.1 };
const THEMES: PageTheme[] = ["auto", "light", "sepia", "dark"];

export const READING_DEFAULTS: ReadingPrefs = { mode: "read", size: 100, font: "serif", width: "narrow", spacing: "normal", theme: "auto", taggedWords: false };
export const TEXT_SIZES = [85, 100, 115, 130, 150];

export function normalizeReading(v: unknown): ReadingPrefs {
  const o = (v && typeof v === "object" ? v : {}) as Partial<Record<keyof ReadingPrefs, unknown>>;
  return {
    mode: o.mode === "study" ? "study" : "read",
    size: TEXT_SIZES.includes(o.size as number) ? (o.size as number) : 100,
    font: o.font === "sans" ? "sans" : "serif",
    width: o.width === "wide" ? "wide" : "narrow",
    spacing: o.spacing === "tight" || o.spacing === "loose" ? o.spacing : "normal",
    theme: THEMES.includes(o.theme as PageTheme) ? (o.theme as PageTheme) : "auto",
    taggedWords: o.taggedWords === true,
  };
}

// stepSize moves one text size up or down, staying within TEXT_SIZES.
export function stepSize(size: number, by: -1 | 1): number {
  const i = TEXT_SIZES.indexOf(size);
  const j = Math.min(TEXT_SIZES.length - 1, Math.max(0, (i < 0 ? 1 : i) + by));
  return TEXT_SIZES[j];
}

// Assistant ----------------------------------------------------------------------------------------

// The app's colours: Match the system (the default), or always Light or Dark. Read mode's page
// colour (ReadingPrefs.theme) is separate.
export type AppTheme = "system" | "light" | "dark";
export const APP_THEMES: [AppTheme, string][] = [["system", "Match the system"], ["light", "Light"], ["dark", "Dark"]];

export function normalizeTheme(v: unknown): AppTheme {
  return v === "light" || v === "dark" ? v : "system";
}

// Recent searches ----------------------------------------------------------------------------------

export const MAX_RECENT = 8;

// addRecent puts a search first, without repeats (ignoring case and spacing), keeping MAX_RECENT.
export function addRecent(list: string[], query: string): string[] {
  const q = query.trim().replace(/\s+/g, " ");
  if (!q) return list;
  return [q, ...list.filter((x) => x.toLowerCase() !== q.toLowerCase())].slice(0, MAX_RECENT);
}
