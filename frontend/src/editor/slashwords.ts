// The slash menu's items and how typing narrows them, kept apart from the editor so they are tested
// on their own (slashwords.test.ts). Typing "/" in the text opens the menu; the letters after it
// filter the items by the start of any word in their name or their other names. The menu lists
// every format and insert, in alphabetical order.

export type SlashId =
  | "bullets" | "checks" | "code" | "divider" | "h1" | "h2" | "h3" | "highlight" | "ink" | "link" | "notelink" | "numbers" | "quote"
  | "table" | "verseref" | "versequote";

export interface SlashItem {
  id: SlashId;
  label: string;
  hint: string;
  icon: string; // a short glyph shown beside the name
  also: string[]; // other words that find it
}

// The largest table the picker and /table make: 10 rows by 10 columns.
export const MAX_TABLE = 10;

const ITEMS: SlashItem[] = [
  { id: "bullets", label: "Bulleted list", hint: "A simple list", icon: "•", also: ["unordered", "list"] },
  { id: "checks", label: "Checkbox list", hint: "Things to tick off", icon: "☐", also: ["todo", "task", "check", "list"] },
  { id: "code", label: "Code", hint: "Words in a fixed-width face", icon: "<>", also: ["monospace", "inline"] },
  { id: "divider", label: "Divider", hint: "A line across the page", icon: "—", also: ["line", "rule", "separator"] },
  { id: "ink", label: "Handwriting", hint: "Write or draw with a pen, a mouse or a finger", icon: "✎", also: ["ink", "draw", "sketch", "pen", "write", "scribble"] },
  { id: "h1", label: "Heading large", hint: "A main section title", icon: "H1", also: ["title", "section", "point", "big"] },
  { id: "h2", label: "Heading medium", hint: "A section title", icon: "H2", also: ["title", "section", "point", "subheading"] },
  { id: "h3", label: "Heading small", hint: "A small section title", icon: "H3", also: ["title", "section", "point", "subheading"] },
  { id: "highlight", label: "Highlight", hint: "Words you highlighted, with where they are", icon: "▍", also: ["marked"] },
  { id: "link", label: "Link", hint: "A link to a web address", icon: "↗", also: ["url", "web", "address"] },
  { id: "notelink", label: "Link to a note or sermon", hint: "A chip that opens it", icon: "¶", also: ["note", "sermon"] },
  { id: "numbers", label: "Numbered list", hint: "A list with numbers", icon: "1.", also: ["ordered", "list"] },
  { id: "quote", label: "Quote", hint: "Someone else's words", icon: "”", also: ["citation", "blockquote"] },
  { id: "table", label: "Table", hint: "Rows and columns: /table3x2 makes 3 rows by 2 columns", icon: "▦", also: ["grid", "rows", "columns"] },
  { id: "verseref", label: "Verse reference", hint: "A chip that opens the passage", icon: "✦", also: ["reference", "ref", "chip"] },
  { id: "versequote", label: "Verse text", hint: "Bible text, with its credit line", icon: "❝", also: ["quote", "verses", "bible", "scripture", "passage"] },
];

export const SLASH_ITEMS: SlashItem[] = [...ITEMS].sort((a, b) => a.label.localeCompare(b.label));

// tableSize reads "/table", "/table3x2" or "/table 3×2": rows by columns, each at most 10. A plain
// "table" (or any start of it) is 3 by 3; anything else is null.
export function tableSize(query: string): { rows: number; cols: number } | null {
  const q = query.trim().toLowerCase();
  const m = /^table\s*(\d{1,3})\s*[x×]\s*(\d{1,3})$/.exec(q);
  if (m) {
    const [rows, cols] = clampTable(Number(m[1]), Number(m[2]));
    return { rows, cols };
  }
  return q && "table".startsWith(q) ? { rows: 3, cols: 3 } : null;
}

export function clampTable(rows: number, cols: number): [number, number] {
  const c = (n: number) => Math.min(MAX_TABLE, Math.max(1, Math.round(n) || 1));
  return [c(rows), c(cols)];
}

// filterSlash keeps the items whose name, or one of whose other words, has a word starting with
// query (ignoring case), in alphabetical order. "table3x2" finds the table.
export function filterSlash(items: SlashItem[], query: string): SlashItem[] {
  let q = query.trim().toLowerCase();
  if (!q) return items;
  if (/^table\s*\d/.test(q) && tableSize(q)) q = "table";
  const words = (s: string) => s.toLowerCase().split(/\s+/);
  return items.filter((it) => words(it.label).some((w) => w.startsWith(q)) || it.label.toLowerCase().startsWith(q)
    || it.also.some((a) => words(a).some((w) => w.startsWith(q))));
}

// slashQuery finds a slash command being typed just before the caret: "/" at the start of the text
// or after a space, then letters (and, for a table's size, digits and x or ×). It gives back what
// was typed after the slash, or null.
export function slashQuery(before: string): string | null {
  const m = /(?:^|\s)\/([A-Za-z]{0,20}(?:\d{1,3}[x×]\d{0,3}|\d{1,3})?)$/.exec(before);
  return m ? m[1] : null;
}
