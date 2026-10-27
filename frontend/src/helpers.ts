// Pure helpers with no DOM or app access, so they can be unit tested.

export interface CommandError {
  code: string;
  message: string;
}

export interface Schema {
  type: string;
  description?: string;
  properties?: Record<string, Schema>;
  required?: string[];
  items?: Schema;
  enum?: string[];
}

const KNOWN_CODES = ["bad_params", "not_found", "unknown_command", "unavailable", "failed", "bad_request"];

// parseError turns whatever a rejected App.Run gives ("code: message", an Error, or an object)
// into a code and message.
export function parseError(e: unknown): CommandError {
  if (e && typeof e === "object" && "code" in e && "message" in e) {
    const o = e as { code: unknown; message: unknown };
    return { code: String(o.code), message: String(o.message) };
  }
  const text = e instanceof Error ? e.message : String(e);
  const i = text.indexOf(": ");
  if (i > 0 && KNOWN_CODES.includes(text.slice(0, i))) {
    return { code: text.slice(0, i), message: text.slice(i + 2) };
  }
  return { code: "failed", message: text };
}

const CODE_TITLES: Record<string, string> = {
  bad_params: "Please check the details",
  bad_request: "Please check the details",
  not_found: "Not found",
  unknown_command: "This action is not available",
  unavailable: "Not available right now",
  failed: "Something went wrong",
};

// errorText is a sentence for people: a title for the code, then the command's message with the
// "command.id: " prefix removed.
export function errorText(err: CommandError): string {
  const title = CODE_TITLES[err.code] ?? "Something went wrong";
  const msg = err.message.replace(/^[a-z_]+\.[a-z_.]+: /, "");
  return `${title}: ${msg}`;
}

// exampleParams fills each required parameter with a placeholder.
export function exampleParams(schema: Schema | null | undefined): string {
  const o: Record<string, unknown> = {};
  for (const [k, p] of Object.entries(schema?.properties ?? {})) {
    if (!(schema?.required ?? []).includes(k)) continue;
    o[k] = p.enum ? p.enum[0] : p.type === "integer" || p.type === "number" ? 0
      : p.type === "object" ? {} : p.type === "array" ? [] : p.type === "boolean" ? false : "";
  }
  return JSON.stringify(o, null, 2);
}

// Verse IDs are integers BBCCCVVV.
export function verseParts(id: number): { book: number; chapter: number; verse: number } {
  return { book: Math.floor(id / 1_000_000), chapter: Math.floor(id / 1000) % 1000, verse: id % 1000 };
}

export interface Book {
  number: number;
  osis: string;
  name: string;
  chapters: number;
}

// neighbourChapters returns references for the chapters before and after the one a verse is in,
// crossing book boundaries, or null at either end of the canon.
export function neighbourChapters(id: number, books: Book[]): { prev: string | null; next: string | null } {
  const sorted = [...books].sort((a, b) => a.number - b.number);
  const { book, chapter } = verseParts(id);
  const i = sorted.findIndex((b) => b.number === book);
  if (i < 0) return { prev: null, next: null };
  const b = sorted[i];
  let prev: string | null = null;
  let next: string | null = null;
  if (chapter > 1) prev = `${b.osis} ${chapter - 1}`;
  else if (i > 0) prev = `${sorted[i - 1].osis} ${sorted[i - 1].chapters}`;
  if (chapter < b.chapters) next = `${b.osis} ${chapter + 1}`;
  else if (i < sorted.length - 1) next = `${sorted[i + 1].osis} 1`;
  return { prev, next };
}

export interface Verse {
  id: number;
  base_id: number;
  end: number;
  ref: string;
  text: string;
}

export interface Column {
  module: string;
  ref: string;
  attribution: string;
  verses: Verse[];
}

// alignColumns lines parallel columns up by base verse, keeping canonical order. Each row has one
// cell per column; a cell is empty where that Bible has no such verse.
export function alignColumns(columns: Column[]): { base: number; cells: (Verse | null)[] }[] {
  const ids = new Set<number>();
  for (const c of columns) for (const v of c.verses) ids.add(v.base_id || v.id);
  const order = [...ids].sort((a, b) => a - b);
  const lookup = columns.map((c) => {
    const m = new Map<number, Verse>();
    for (const v of c.verses) if (!m.has(v.base_id || v.id)) m.set(v.base_id || v.id, v);
    return m;
  });
  return order.map((base) => ({ base, cells: lookup.map((m) => m.get(base) ?? null) }));
}

// splitList reads a comma- or line-separated list, dropping blanks.
export function splitList(text: string): string[] {
  return text.split(/[\n,;]/).map((s) => s.trim()).filter(Boolean);
}

// changedFields returns the fields of next that differ from before.
export function changedFields<T extends Record<string, unknown>>(before: T, next: T): Partial<T> {
  const out: Partial<T> = {};
  for (const k of Object.keys(next) as (keyof T)[]) {
    if (JSON.stringify(before[k] ?? "") !== JSON.stringify(next[k] ?? "")) out[k] = next[k];
  }
  return out;
}

// formatTime shows a millisecond Unix time in the user's locale.
export function formatTime(ms: number): string {
  if (!ms) return "";
  return new Date(ms).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}

// pageLabel describes one page of a paged list.
export function pageLabel(offset: number, count: number, total: number): string {
  if (total === 0) return "No items";
  return `${offset + 1}–${offset + count} of ${total}`;
}

// summarise shortens a value from the change history for display.
export function summarise(v: unknown, max = 80): string {
  if (v === null || v === undefined) return "—";
  const s = typeof v === "string" ? v : JSON.stringify(v);
  return s.length > max ? s.slice(0, max - 1) + "…" : s;
}

// markSegments splits a search snippet with <mark></mark> around matches into plain segments, so
// it can be shown without treating the text as HTML.
export function markSegments(snippet: string): { text: string; mark: boolean }[] {
  const out: { text: string; mark: boolean }[] = [];
  const re = /<mark>([\s\S]*?)<\/mark>/g;
  let last = 0;
  for (let m; (m = re.exec(snippet)); ) {
    if (m.index > last) out.push({ text: snippet.slice(last, m.index), mark: false });
    out.push({ text: m[1], mark: true });
    last = m.index + m[0].length;
  }
  if (last < snippet.length) out.push({ text: snippet.slice(last), mark: false });
  return out;
}
