// The words of a highlight and its quotation lines, kept apart from the DOM so they are tested on
// their own (insertwords.test.ts).

import type { HighlightRow } from "./highlightspanel";

// CharRange is part of a highlight's words, as character offsets into them.
export interface CharRange {
  start: number;
  end: number;
}

// highlightText reads a highlight's words; with a range, only that part. Highlights are whole
// verses or whole book passages today; T34f gives them ranges within verses, read through here.
export function highlightText(row: Pick<HighlightRow, "text" | "locator">, range?: CharRange): string {
  const all = (row.text || row.locator?.text || "").replace(/\s+/g, " ").trim();
  if (!range) return all;
  const chars = [...all];
  const start = Math.max(0, Math.min(chars.length, range.start));
  const end = Math.max(start, Math.min(chars.length, range.end));
  return chars.slice(start, end).join("").trim();
}

// bookPlace is the app address of a highlight's place in a library book (reportwords.routeOf).
export function bookPlace(row: Pick<HighlightRow, "item_id" | "locator">): string {
  const q = new URLSearchParams();
  const loc = row.locator ?? {};
  if (loc.section) q.set("spine", String(loc.section));
  if (loc.page) q.set("page", String(loc.page));
  if (loc.para) q.set("para", String(loc.para));
  const qs = q.toString();
  return `#book/${encodeURIComponent(row.item_id ?? "")}${qs ? `?${qs}` : ""}`;
}

// highlightQuoteLines gives a book highlight's quotation: the words, then a credit line naming the
// book and place, linked back to it. (Quoted words are kept as they are; the editor reads only the
// marks it writes, so stray stars or brackets in a book stay text.)
export function highlightQuoteLines(row: Pick<HighlightRow, "item_id" | "item_title" | "locator">, text: string): string[] {
  const loc = row.locator ?? {};
  const place = loc.page ? `page ${loc.page}` : loc.section ? `section ${loc.section}` : "";
  const name = [row.item_title || "A book", place].filter(Boolean).join(", ").replace(/[[\]]/g, "");
  return [text, `— [${name}](${bookPlace(row)})`];
}
