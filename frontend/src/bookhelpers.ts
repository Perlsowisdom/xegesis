// Pure helpers for the book reader, with no DOM or app access, so they can be unit tested.

// base64ToBytes decodes one chunk from book.file.
export function base64ToBytes(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export function concatBytes(parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let at = 0;
  for (const p of parts) {
    out.set(p, at);
    at += p.length;
  }
  return out;
}

export interface Box {
  left: number;
  top: number;
  width: number;
  height: number;
}

// pageRects turns a selection's client rectangles into [x, y, width, height] fractions of the
// page, dropping empty ones and merging those on the same line that touch.
export function pageRects(rects: Box[], page: Box): [number, number, number, number][] {
  if (page.width <= 0 || page.height <= 0) return [];
  const round = (n: number) => Math.round(n * 10000) / 10000;
  const clamp = (n: number) => Math.min(1, Math.max(0, n));
  const out: [number, number, number, number][] = [];
  const sorted = rects.filter((r) => r.width > 0.5 && r.height > 0.5)
    .sort((a, b) => a.top - b.top || a.left - b.left);
  for (const r of sorted) {
    const x = clamp((r.left - page.left) / page.width);
    const y = clamp((r.top - page.top) / page.height);
    const w = Math.min(1 - x, r.width / page.width);
    const hh = Math.min(1 - y, r.height / page.height);
    const last = out[out.length - 1];
    if (last && Math.abs(last[1] - y) < hh / 2 && x <= last[0] + last[2] + 0.005) {
      const right = Math.max(last[0] + last[2], x + w);
      last[0] = Math.min(last[0], x);
      last[2] = right - last[0];
      last[3] = Math.max(last[3], hh);
      continue;
    }
    out.push([x, y, w, hh]);
  }
  return out.map((r) => r.map(round) as [number, number, number, number]);
}

export type Theme = "auto" | "light" | "sepia" | "dark";
export type Font = "serif" | "sans" | "mono";

export interface ReadingPrefs {
  font: Font;
  size: number; // percent
  spacing: number; // line height
  theme: Theme;
  zoom: number; // PDF, relative to the pane's width
}

export const DEFAULT_PREFS: ReadingPrefs = { font: "serif", size: 100, spacing: 1.7, theme: "auto", zoom: 1 };

// tidyPrefs fills in and bounds stored preferences.
export function tidyPrefs(p: Partial<ReadingPrefs> | null | undefined): ReadingPrefs {
  const v = { ...DEFAULT_PREFS, ...(p ?? {}) };
  const within = (n: unknown, lo: number, hi: number, d: number) =>
    typeof n === "number" && isFinite(n) ? Math.min(hi, Math.max(lo, n)) : d;
  return {
    font: (["serif", "sans", "mono"] as Font[]).includes(v.font) ? v.font : "serif",
    size: within(v.size, 70, 250, 100),
    spacing: within(v.spacing, 1.2, 2.4, 1.7),
    theme: (["auto", "light", "sepia", "dark"] as Theme[]).includes(v.theme) ? v.theme : "auto",
    zoom: within(v.zoom, 0.5, 3, 1),
  };
}

export interface CopyResult {
  allowed: boolean;
  text?: string;
  reason?: string;
  chars: number;
  copied_today: number;
  daily_limit?: number;
  confirm?: boolean;
}

// copyMessage says what happened to a copy.
export function copyMessage(r: CopyResult): string {
  if (!r.allowed) return `Not copied: ${r.reason ?? "the licence does not allow it"}.`;
  const left = r.daily_limit ? ` About ${Math.max(0, r.daily_limit - r.copied_today).toLocaleString()} more characters can be copied from this book today.` : "";
  const confirm = r.confirm ? " This book's licence has not been confirmed yet; check it in the Library." : "";
  return `Copied ${r.chars.toLocaleString()} characters${r.text && r.text.length > r.chars ? " with the attribution" : ""}.${left}${confirm}`;
}

export interface OutlineItem {
  title: string;
  page: number;
  level: number;
}

export interface TOCEntry {
  title: string;
  section: number;
  anchor?: string;
  level: number;
}

// tocOptions labels contents entries with indentation for a select box.
export function tocOptions(toc: TOCEntry[]): [string, string][] {
  return toc.map((t, i) => [String(i), `${" ".repeat(Math.max(0, Math.min(t.level, 6) - 1))}${t.title}`]);
}

// currentEntry is the last contents entry at or before a section.
export function currentEntry(toc: TOCEntry[], section: number): number {
  let best = -1;
  toc.forEach((t, i) => {
    if (t.section <= section && (best < 0 || t.section > toc[best].section)) best = i;
  });
  return best;
}
