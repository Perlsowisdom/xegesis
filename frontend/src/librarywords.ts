// Pure logic for the Library's Filter popover: which facets a book has, how choices combine (any
// choice within a facet, every facet across them), the chips for active filters, the count line and
// the colours of the drawn covers. No DOM here, so it is unit-tested (librarywords.test.ts).

import { NOT_SET } from "./facetwords";

export type FacetId = "type" | "tradition" | "author" | "era" | "licence";

export const FACETS: { id: FacetId; label: string }[] = [
  { id: "type", label: "Type" },
  { id: "tradition", label: "Tradition" },
  { id: "author", label: "Author" },
  { id: "era", label: "Era" },
  { id: "licence", label: "Licence" },
];

// The fields of a book (library.items) that filtering reads.
export interface Book {
  id: string;
  title: string;
  subtitle?: string;
  authors: string[];
  year?: number;
  type?: string;
  tradition?: string;
  licence?: string;
  licence_name?: string;
}

// Filters holds the chosen values of each facet; a facet with none chosen does not filter.
export type Filters = Partial<Record<FacetId, string[]>>;

// ERAS are the year spans books are grouped into, oldest first.
export const ERAS: { label: string; until: number }[] = [
  { label: "Before 1500", until: 1499 },
  { label: "1500 to 1699", until: 1699 },
  { label: "1700 to 1899", until: 1899 },
  { label: "1900 to 1999", until: 1999 },
  { label: "2000 and later", until: Infinity },
];
export const NO_YEAR = "Year not known";
export const NO_AUTHOR = "No author given";

export function eraOf(year?: number): string {
  if (!year) return NO_YEAR;
  return ERAS.find((e) => year <= e.until)!.label;
}

// valuesOf is what a book counts as under a facet. A book has one type, tradition, era and licence,
// and any number of authors.
export function valuesOf(b: Book, f: FacetId): string[] {
  switch (f) {
    case "type": return [b.type || NOT_SET];
    case "tradition": return [b.tradition || NOT_SET];
    case "author": return b.authors.length ? b.authors : [NO_AUTHOR];
    case "era": return [eraOf(b.year)];
    case "licence": return [b.licence_name || "Licence not set"];
  }
}

// matchesText is true when every word of q is in the title, subtitle or an author.
export function matchesText(b: Book, q: string): boolean {
  const words = q.toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return true;
  const hay = [b.title, b.subtitle ?? "", ...b.authors].join(" ").toLowerCase();
  return words.every((w) => hay.includes(w));
}

// matches combines the filters: within a facet any chosen value will do (OR), and a book must pass
// every facet that has choices (AND). skip leaves one facet out, to count that facet's options.
export function matches(b: Book, filters: Filters, skip?: FacetId): boolean {
  for (const { id } of FACETS) {
    const want = filters[id];
    if (id === skip || !want?.length) continue;
    if (!valuesOf(b, id).some((v) => want.includes(v))) return false;
  }
  return true;
}

export function applyFilters<T extends Book>(books: T[], filters: Filters, q = ""): T[] {
  return books.filter((b) => matchesText(b, q) && matches(b, filters));
}

export interface Option {
  value: string;
  count: number; // books it would show, given the other facets' choices
}

// options lists a facet's values in the books, with counts that take the other facets into account,
// so choosing one never leads to an empty list. Chosen values stay listed even at zero.
export function options(books: Book[], f: FacetId, filters: Filters, order: string[] = []): Option[] {
  const counts = new Map<string, number>();
  for (const b of books) {
    if (!matches(b, filters, f)) continue;
    for (const v of new Set(valuesOf(b, f))) counts.set(v, (counts.get(v) ?? 0) + 1);
  }
  for (const v of filters[f] ?? []) if (!counts.has(v)) counts.set(v, 0);
  const rank = (v: string) => {
    if (f === "era") return v === NO_YEAR ? ERAS.length : ERAS.findIndex((e) => e.label === v);
    const i = order.indexOf(v);
    return i < 0 ? Infinity : i;
  };
  const last = new Set([NOT_SET, NO_AUTHOR, NO_YEAR, "Licence not set"]);
  return [...counts].map(([value, count]) => ({ value, count })).sort((a, b) =>
    Number(last.has(a.value)) - Number(last.has(b.value))
    || rank(a.value) - rank(b.value)
    || (f === "author" || f === "tradition" ? a.value.localeCompare(b.value) : b.count - a.count || a.value.localeCompare(b.value)));
}

// toggle chooses or unchooses one value, leaving the filters it was given alone.
export function toggle(filters: Filters, f: FacetId, value: string, on: boolean): Filters {
  const now = (filters[f] ?? []).filter((v) => v !== value);
  if (on) now.push(value);
  const out: Filters = { ...filters, [f]: now };
  if (!now.length) delete out[f];
  return out;
}

export interface Chip {
  facet: FacetId;
  value: string;
  label: string;
}

// chips are the active filters in facet order, each removable on its own.
export function chips(filters: Filters): Chip[] {
  const out: Chip[] = [];
  for (const { id, label } of FACETS) {
    for (const v of filters[id] ?? []) out.push({ facet: id, value: v, label: id === "type" || id === "tradition" || id === "licence" ? v : `${label}: ${v}` });
  }
  return out;
}

export function activeCount(filters: Filters): number {
  return FACETS.reduce((n, f) => n + (filters[f.id]?.length ?? 0), 0);
}

// countLine reads "12 of 140 books", or "140 books" when nothing is filtered out.
export function countLine(shown: number, total: number): string {
  const noun = (n: number) => (n === 1 ? "book" : "books");
  return shown === total ? `${total.toLocaleString("en")} ${noun(total)}` : `${shown.toLocaleString("en")} of ${total.toLocaleString("en")} ${noun(total)}`;
}

// Covers --------------------------------------------------------------------------------------

// A drawn cover's colours come from the book's type, so a shelf reads at a glance; the shade varies
// with the title so neighbours differ.
const COVER_HUES: [RegExp, number][] = [
  [/^Bible/, 8], [/^Commentary/, 28], [/^Dictionary|^Lexicon/, 205], [/^Theology/, 262],
  [/^Church history|^Biography/, 150], [/^Devotional|^Sermons/, 340], [/^Fiction/, 185],
];

export function coverHue(b: Pick<Book, "type" | "title">): number {
  const base = COVER_HUES.find(([re]) => re.test(b.type ?? ""))?.[1] ?? 35;
  let h = 0;
  for (const c of b.title) h = (h * 31 + c.charCodeAt(0)) % 997;
  return (base + (h % 17) - 8 + 360) % 360;
}

// initials are up to two letters for a cover's mark: the first letters of the first two long words.
export function initials(title: string): string {
  const words = title.replace(/[^\p{L}\p{N}\s]/gu, " ").split(/\s+/).filter((w) => w.length > 2 && !/^(the|and|of|on|for|with)$/i.test(w));
  return (words.slice(0, 2).map((w) => w[0]).join("") || title.slice(0, 1)).toUpperCase();
}
