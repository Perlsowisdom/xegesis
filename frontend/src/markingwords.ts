// Rules for marking text, kept apart from the DOM so they are tested (markingwords.test.ts): the
// five highlight colours and how a style names a colour and an underline, which highlights a new one
// replaces, and where the floating selection toolbar goes.

export type Colour = "yellow" | "green" | "blue" | "pink" | "purple";

export const COLOURS: { id: Colour; label: string }[] = [
  { id: "yellow", label: "Yellow" }, { id: "green", label: "Green" }, { id: "blue", label: "Blue" },
  { id: "pink", label: "Pink" }, { id: "purple", label: "Purple" },
];

export interface Style {
  colour: Colour;
  underline: boolean;
}

// parseStyle reads a stored style: "green", "green-underline", or the older "underline" (yellow).
export function parseStyle(style: string): Style {
  if (style === "underline") return { colour: "yellow", underline: true };
  const underline = style.endsWith("-underline");
  const c = style.replace(/-underline$/, "") as Colour;
  return { colour: COLOURS.some((x) => x.id === c) ? c : "yellow", underline };
}

export function styleOf(s: Style): string {
  return s.underline ? `${s.colour}-underline` : s.colour;
}

// hlClasses are the classes that draw a highlight of a style.
export function hlClasses(style: string): string[] {
  const s = parseStyle(style);
  return s.underline ? ["hl", `hl-${s.colour}`, "hl-u"] : ["hl", `hl-${s.colour}`];
}

// replaced lists the verse highlights a new one over a..b takes the place of: those wholly inside it.
export function replaced<T extends { id: string; verse_start?: number; verse_end?: number }>(list: T[], a: number, b: number): T[] {
  return list.filter((x) => x.verse_start !== undefined && x.verse_end !== undefined && x.verse_start >= a && x.verse_end <= b);
}

export interface Box { left: number; top: number; width: number; height: number }

// toolbarPlace puts a toolbar of size bar just above the target, centred on it, kept inside the
// view with a margin; when there is no room above it goes just below.
export function toolbarPlace(target: Box, bar: { width: number; height: number }, view: { width: number; height: number }, gap = 8, margin = 8): { left: number; top: number; below: boolean } {
  const centre = target.left + target.width / 2;
  const left = Math.round(Math.min(Math.max(margin, centre - bar.width / 2), Math.max(margin, view.width - bar.width - margin)));
  const above = target.top - gap - bar.height;
  if (above >= margin) return { left, top: Math.round(above), below: false };
  const below = Math.min(target.top + target.height + gap, view.height - bar.height - margin);
  return { left, top: Math.round(Math.max(margin, below)), below: true };
}

// wordAround finds the word in text that covers offset: letters, marks, digits and inner
// apostrophes or hyphens. It returns [start, end), or null between words.
export function wordAround(text: string, offset: number): [number, number] | null {
  const isWord = (ch: string) => /[\p{L}\p{M}\p{N}]/u.test(ch);
  const inner = (i: number) => /['’-]/.test(text[i] ?? "") && isWord(text[i - 1] ?? "") && isWord(text[i + 1] ?? "");
  let at = offset;
  if (!isWord(text[at] ?? "") && !inner(at)) {
    if (at > 0 && isWord(text[at - 1])) at -= 1;
    else return null;
  }
  let a = at;
  let b = at;
  while (a > 0 && (isWord(text[a - 1]) || inner(a - 1))) a--;
  while (b < text.length && (isWord(text[b]) || inner(b))) b++;
  return a < b ? [a, b] : null;
}
