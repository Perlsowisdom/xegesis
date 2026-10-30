// Plain words and small rules for the print and export dialog, kept free of the DOM so they are
// tested on their own.

export interface ExportSource {
  kind: "passage" | "book" | "note" | "list" | "plan" | "report";
  [key: string]: unknown;
}

export interface ExportOptions {
  page_size?: "a4" | "letter";
  font_size?: number;
  verse_numbers?: boolean;
  notes?: boolean;
  layout?: "side_by_side" | "one_after_another";
  header?: boolean;
  footer?: boolean;
  credits?: "each" | "end";
}

export interface PieceInfo {
  title: string;
  licence: string;
  credits?: string[];
  pages: number;
  cut?: { reason: string; kept_pages: number; total_pages: number; rest?: ExportSource };
  confirm?: boolean;
}

export interface ExportPreview {
  title: string;
  format: string;
  pieces: PieceInfo[];
  cuts: string[];
  warnings: string[];
  body: string;
  css: string;
  html?: string;
  path?: string;
  bytes?: number;
}

export const FORMATS: { id: string; label: string }[] = [
  { id: "pdf", label: "PDF" },
  { id: "docx", label: "Word" },
  { id: "html", label: "Web page" },
  { id: "md", label: "Markdown" },
];

export function formatLabel(id: string): string {
  return FORMATS.find((f) => f.id === id)?.label ?? id;
}

// fileName suggests a file name for a title, without characters file systems refuse.
export function fileName(title: string, format: string): string {
  const base = title.replace(/[<>:"/\\|?*\x00-\x1f]+/g, " ").replace(/\s+/g, " ").trim().slice(0, 80) || "Xegesis export";
  return `${base}.${format}`;
}

// summary says in one line what the result holds.
export function summary(p: ExportPreview): string {
  const pages = p.pieces.reduce((n, x) => n + x.pages, 0);
  const parts = p.pieces.length === 1 ? "1 item" : `${p.pieces.length} items`;
  const size = pages < 0.1 ? "" : `, about ${pages < 1 ? "1 page" : `${Math.round(pages)} page${Math.round(pages) === 1 ? "" : "s"}`} of text`;
  const cut = p.cuts.length ? ` · ${p.cuts.length === 1 ? "1 item shortened" : `${p.cuts.length} items shortened`} by its licence` : "";
  return `${parts}${size}${cut}`;
}

// rests lists the parts left out that can be printed another day.
export function rests(p: ExportPreview): { title: string; source: ExportSource }[] {
  return p.pieces.filter((x) => x.cut?.rest).map((x) => ({ title: x.title, source: x.cut!.rest! }));
}

// Waiting: parts kept to print another day, remembered on this computer only.

export interface Waiting {
  title: string;
  sources: ExportSource[];
  options: ExportOptions;
  saved: string; // YYYY-MM-DD
}

const KEY = "xegesis.printLater";

export function loadWaiting(store: Pick<Storage, "getItem"> = localStorage): Waiting[] {
  try {
    const v = JSON.parse(store.getItem(KEY) ?? "[]");
    return Array.isArray(v) ? v : [];
  } catch {
    return [];
  }
}

export function saveWaiting(list: Waiting[], store: Pick<Storage, "setItem"> = localStorage) {
  try {
    store.setItem(KEY, JSON.stringify(list.slice(-20)));
  } catch {
    // Storage may be unavailable; the list is only a convenience.
  }
}
