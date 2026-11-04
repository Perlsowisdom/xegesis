// The slash menu's items and how typing narrows them, kept apart from the editor so they are tested
// on their own (slashwords.test.ts). Typing "/" in the text opens the menu; the letters after it
// filter the items by the start of any word in their name or their other names.

export type SlashId = "heading" | "bullets" | "numbers" | "quote" | "verseref" | "versequote" | "highlight" | "notelink" | "divider";

export interface SlashItem {
  id: SlashId;
  label: string;
  hint: string;
  icon: string; // a short glyph shown beside the name
  also: string[]; // other words that find it
}

// The order is the menu's order when nothing is typed; "/verse", "/quote", "/highlight" and
// "/link" each put their own item first.
export const SLASH_ITEMS: SlashItem[] = [
  { id: "heading", label: "Heading", hint: "A section title", icon: "H", also: ["title", "section", "point"] },
  { id: "bullets", label: "Bulleted list", hint: "A simple list", icon: "•", also: ["unordered", "list"] },
  { id: "numbers", label: "Numbered list", hint: "A list with numbers", icon: "1.", also: ["ordered", "list"] },
  { id: "verseref", label: "Verse reference", hint: "A chip that opens the passage", icon: "✦", also: ["reference", "ref", "chip"] },
  { id: "versequote", label: "Quote verses", hint: "Bible text, with its credit line", icon: "❝", also: ["verse text", "bible", "scripture", "passage"] },
  { id: "highlight", label: "Highlight", hint: "Words you highlighted, with where they are", icon: "▍", also: ["marked"] },
  { id: "notelink", label: "Link to a note or sermon", hint: "A chip that opens it", icon: "¶", also: ["note", "sermon"] },
  { id: "quote", label: "Block quote", hint: "Someone else's words", icon: "”", also: ["citation", "blockquote"] },
  { id: "divider", label: "Divider", hint: "A line across the page", icon: "—", also: ["line", "rule", "separator"] },
];

// filterSlash keeps the items whose name, or one of whose other words, has a word starting with
// query (ignoring case); items whose name starts with it come first.
export function filterSlash(items: SlashItem[], query: string): SlashItem[] {
  const q = query.trim().toLowerCase();
  if (!q) return items;
  const words = (s: string) => s.toLowerCase().split(/\s+/);
  const scored = items.map((it) => {
    const name = it.label.toLowerCase();
    if (name.startsWith(q)) return { it, score: 0 };
    if (words(it.label).some((w) => w.startsWith(q))) return { it, score: 1 };
    if (it.also.some((a) => words(a).some((w) => w.startsWith(q)))) return { it, score: 2 };
    return { it, score: -1 };
  });
  return scored.filter((s) => s.score >= 0).sort((a, b) => a.score - b.score).map((s) => s.it);
}

// slashQuery finds a slash command being typed just before the caret: "/" at the start of the text
// or after a space, then letters only. It returns the letters typed, or null.
export function slashQuery(before: string): string | null {
  const m = /(?:^|\s)\/([A-Za-z]{0,20})$/.exec(before);
  return m ? m[1] : null;
}
