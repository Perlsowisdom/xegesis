// Small rules for the note and sermon pages and the pane menu, kept apart from the DOM so they are
// tested (docwords.test.ts): the save indicator's words, the month calendar's days, and the pane ⋯
// menu's groups.

export type SaveStatus = "saved" | "dirty" | "saving" | "error" | "new";

export function saveText(s: SaveStatus): string {
  return { saved: "Saved", dirty: "Not saved yet", saving: "Saving…", error: "Not saved", new: "Not saved yet" }[s];
}

// Calendar --------------------------------------------------------------------------------------

export interface CalDay {
  day: number; // 0 for a blank before the 1st
  iso: string; // YYYY-MM-DD, "" for a blank
  sunday: boolean;
}

const pad = (n: number) => String(n).padStart(2, "0");
export const isoDate = (y: number, m: number, d: number) => `${y}-${pad(m + 1)}-${pad(d)}`;

// monthDays lays out a month (m from 0) in weeks starting on Sunday: blanks, then each day.
export function monthDays(y: number, m: number): CalDay[] {
  const first = new Date(y, m, 1).getDay();
  const n = new Date(y, m + 1, 0).getDate();
  const out: CalDay[] = [];
  for (let i = 0; i < first; i++) out.push({ day: 0, iso: "", sunday: false });
  for (let d = 1; d <= n; d++) out.push({ day: d, iso: isoDate(y, m, d), sunday: (first + d - 1) % 7 === 0 });
  return out;
}

export function addMonths(y: number, m: number, by: number): [number, number] {
  const t = y * 12 + m + by;
  return [Math.floor(t / 12), ((t % 12) + 12) % 12];
}

// preachedChip is the Preached chip's words: "Sun 12 Oct 2026", or "Not preached yet".
export function preachedChip(iso: string | undefined): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso ?? "");
  if (!m) return "Not preached yet";
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  return d.toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short", year: "numeric" }).replace(",", "");
}

// The pane ⋯ menu ----------------------------------------------------------------------------------

export interface MenuItem {
  id: string;
  label: string;
  keys?: string;
  disabled?: boolean;
  danger?: boolean;
}

export interface MenuGroup {
  head: string;
  items: MenuItem[];
}

export interface PaneMenuState {
  search: boolean; // the pane's document can be searched
  print: boolean;
  link: boolean; // it has a place to link to
  splitRight: boolean; // there is room beside it
  splitDown: boolean; // there is room under it
  bibles?: boolean; // the Bibles button is folded away in a narrow pane
}

// paneMenu is the ⋯ menu of a pane, grouped: what this pane shows, then the layout.
export function paneMenu(s: PaneMenuState): MenuGroup[] {
  const pane: MenuItem[] = [
    { id: "search", label: "Search in this book", keys: "Ctrl+F", disabled: !s.search },
    { id: "print", label: "Print or export…", keys: "Ctrl+P", disabled: !s.print },
    { id: "copylink", label: "Copy link to this place", disabled: !s.link },
  ];
  if (s.bibles) pane.push({ id: "bibles", label: "Bibles to show…" });
  return [
    { head: "This pane", items: pane },
    { head: "Layout", items: [
      { id: "split-right", label: "Split right", disabled: !s.splitRight },
      { id: "split-down", label: "Split down", disabled: !s.splitDown },
      { id: "move", label: "Move to…" },
      { id: "close", label: "Close pane", keys: "Ctrl+W", danger: true },
    ] },
  ];
}
