// Pure logic for My work: the folder tree, where an address points, the order of sermons and notes,
// and the words the bin and the + New menu use. No DOM here, so it is unit-tested (workwords.test.ts).

export interface FolderRow {
  id: string;
  name: string;
  parent_id?: string;
  sort: number;
  notes: number;
  documents: number;
}

export interface FolderNode extends FolderRow {
  children: FolderNode[];
  total: number; // notes and documents here and in its subfolders
}

const bySort = (a: FolderRow, b: FolderRow) => a.sort - b.sort || a.name.localeCompare(b.name, undefined, { sensitivity: "base" }) || a.id.localeCompare(b.id);

// folderTree nests the flat folder.list rows one level deep. A folder whose parent is missing (or is
// itself inside another) is shown at the top, so nothing is ever out of sight.
export function folderTree(rows: FolderRow[]): FolderNode[] {
  const top = rows.filter((r) => !r.parent_id || !rows.some((p) => p.id === r.parent_id && !p.parent_id));
  return [...top].sort(bySort).map((t) => {
    const children = rows.filter((r) => r.parent_id === t.id && !top.includes(r)).sort(bySort)
      .map((c) => ({ ...c, children: [], total: c.notes + c.documents }));
    return { ...t, children, total: t.notes + t.documents + children.reduce((n, c) => n + c.total, 0) };
  });
}

// canNestUnder says whether folder id may go inside parent: folders nest one level, so the parent
// must be at the top and the folder must hold no folders of its own.
export function canNestUnder(rows: FolderRow[], id: string, parent: string): boolean {
  if (id === parent) return false;
  const p = rows.find((r) => r.id === parent);
  if (!p || p.parent_id) return false;
  return !rows.some((r) => r.parent_id === id);
}

// folderPath is a folder's name with its parent's, as "Series / Week 1".
export function folderPath(rows: FolderRow[], id?: string): string {
  const f = rows.find((r) => r.id === id);
  if (!f) return "";
  const p = f.parent_id ? rows.find((r) => r.id === f.parent_id) : undefined;
  return p ? `${p.name} / ${f.name}` : f.name;
}

// moveTargets lists the folders an item can be moved to, in tree order, with "No folder" first.
export function moveTargets(rows: FolderRow[]): { id: string; label: string; depth: number }[] {
  const out = [{ id: "", label: "No folder", depth: 0 }];
  for (const t of folderTree(rows)) {
    out.push({ id: t.id, label: t.name, depth: 0 });
    for (const c of t.children) out.push({ id: c.id, label: c.name, depth: 1 });
  }
  return out;
}

// Addresses -----------------------------------------------------------------------------------

// Where the notes view points: every note, one folder or one tag.
export type Scope = { kind: "all" } | { kind: "folder"; id: string } | { kind: "tag"; tag: string };

// scopeOf reads #notes, #notes/folder/<id> and #notes/tag/<tag>.
export function scopeOf(hash: string): Scope {
  const m = /^#?notes\/(folder|tag)\/(.+)$/.exec(hash);
  if (!m) return { kind: "all" };
  const v = decodeURIComponent(m[2]);
  return m[1] === "folder" ? { kind: "folder", id: v } : { kind: "tag", tag: v };
}

export function scopeHash(s: Scope): string {
  if (s.kind === "folder") return `notes/folder/${encodeURIComponent(s.id)}`;
  if (s.kind === "tag") return `notes/tag/${encodeURIComponent(s.tag)}`;
  return "notes";
}

export function sameScope(a: Scope, b: Scope): boolean {
  return scopeHash(a) === scopeHash(b);
}

// Sermons -------------------------------------------------------------------------------------

export type SermonKey = "title" | "passage" | "preached" | "theme";

export interface SermonRow {
  id: string;
  title: string;
  passage?: { display: string; start: number };
  preached_on?: string;
  theme?: string;
  updated_at: number;
}

// sortSermons orders the sermon table by a column. Rows without a value go last either way, and ties
// keep the most recently changed first.
export function sortSermons<T extends SermonRow>(rows: T[], key: SermonKey, dir: "asc" | "desc"): T[] {
  const val = (r: T): string | number | undefined => {
    switch (key) {
      case "title": return r.title.toLocaleLowerCase();
      case "passage": return r.passage?.start;
      case "preached": return r.preached_on || undefined;
      case "theme": return r.theme?.toLocaleLowerCase() || undefined;
    }
  };
  const sign = dir === "asc" ? 1 : -1;
  return [...rows].sort((a, b) => {
    const x = val(a), y = val(b);
    if (x === undefined || y === undefined) return x === y ? b.updated_at - a.updated_at : x === undefined ? 1 : -1;
    const c = typeof x === "number" && typeof y === "number" ? x - y : String(x).localeCompare(String(y));
    return c * sign || b.updated_at - a.updated_at;
  });
}

// nextSort is the column's next order when its heading is clicked: a new column starts with what
// reads best (newest preached first, A to Z otherwise); the same column flips.
export function nextSort(cur: { key: SermonKey; dir: "asc" | "desc" }, key: SermonKey): { key: SermonKey; dir: "asc" | "desc" } {
  if (cur.key === key) return { key, dir: cur.dir === "asc" ? "desc" : "asc" };
  return { key, dir: key === "preached" ? "desc" : "asc" };
}

// preachedText shows a YYYY-MM-DD date in the reader's way, without shifting it a day by time zone.
export function preachedText(iso?: string, locale?: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso ?? "");
  if (!m) return "";
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3])).toLocaleDateString(locale, { day: "numeric", month: "short", year: "numeric" });
}

// Notes ---------------------------------------------------------------------------------------

// firstLine is a note's first line of text without Markdown marks, for its card.
export function firstLine(md: string, max = 160): string {
  for (const raw of md.split("\n")) {
    const line = raw.replace(/^\s{0,3}(#{1,6}\s+|>\s?|[-*+]\s+|\d+[.)]\s+)/, "")
      .replace(/!\[[^\]]*\]\([^)]*\)/g, "").replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
      .replace(/[*_`~]+/g, "").trim();
    if (line) return line.length > max ? line.slice(0, max - 1).trimEnd() + "…" : line;
  }
  return "";
}

// Edited is how long ago something changed, in a few words.
export function editedText(ms: number, now = Date.now()): string {
  const mins = Math.floor((now - ms) / 60000);
  if (mins < 1) return "Edited just now";
  if (mins < 60) return `Edited ${mins} min ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `Edited ${hours} ${hours === 1 ? "hour" : "hours"} ago`;
  const days = Math.round(hours / 24);
  if (days === 1) return "Edited yesterday";
  if (days < 7) return `Edited ${days} days ago`;
  return `Edited ${new Date(ms).toLocaleDateString(undefined, { day: "numeric", month: "short", year: new Date(ms).getFullYear() === new Date(now).getFullYear() ? undefined : "numeric" })}`;
}

// The bin and + New -----------------------------------------------------------------------------

export interface BinEntry {
  kind: "folder" | "note" | "document" | "list";
  id: string;
  title: string;
  sort?: string;
  deleted_at: number;
  folders?: number;
  notes?: number;
  documents?: number;
}

const DOC_NAMES: Record<string, string> = { sermon: "Sermon", lesson: "Lesson", study: "Bible study" };
const LIST_NAMES: Record<string, string> = { verses: "Verse list", words: "Word list", excerpts: "Excerpt list" };

export function binKind(e: BinEntry): string {
  if (e.kind === "folder") return "Folder";
  if (e.kind === "note") return "Note";
  if (e.kind === "document") return DOC_NAMES[e.sort ?? ""] ?? "Sermon";
  return LIST_NAMES[e.sort ?? ""] ?? "List";
}

export function binTitle(e: BinEntry): string {
  return e.title || (e.kind === "note" ? "Untitled note" : "Untitled");
}

const count = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// binContents says what a deleted folder took with it, or "" when it was empty.
export function binContents(e: BinEntry): string {
  if (e.kind !== "folder") return "";
  const parts: string[] = [];
  if (e.folders) parts.push(count(e.folders, "folder", "folders"));
  if (e.notes) parts.push(count(e.notes, "note", "notes"));
  if (e.documents) parts.push(count(e.documents, "sermon", "sermons"));
  if (!parts.length) return "Empty";
  return parts.length === 1 ? `With ${parts[0]}` : `With ${parts.slice(0, -1).join(", ")} and ${parts[parts.length - 1]}`;
}

// purgeQuestion is what "Delete for good" asks first.
export function purgeQuestion(e: BinEntry): string {
  const what = `"${binTitle(e)}"`;
  const inside = binContents(e);
  return e.kind === "folder" && inside !== "Empty"
    ? `Delete the folder ${what} and everything in it for good? ${inside.replace(/^With/, "That is")}. This cannot be undone.`
    : `Delete ${what} for good? This cannot be undone.`;
}

export type NewKind = "note" | "sermon";

// newChoices are the + New menu's entries: the thing that matches the view first, each in the
// folder being shown when there is one.
export function newChoices(view: "notes" | "sermons", folderName?: string): { kind: NewKind; label: string }[] {
  const where = folderName ? ` in ${folderName}` : "";
  const note = { kind: "note" as const, label: `Note${where}` };
  const sermon = { kind: "sermon" as const, label: `Sermon${where}` };
  return view === "sermons" ? [sermon, note] : [note, sermon];
}
