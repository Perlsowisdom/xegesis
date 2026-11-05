// Highlights on exact words in Bible verses: the maths, kept apart from the page so it is tested on
// its own (verserange.test.ts). A place in a verse is its verse ID and a count of characters
// (Unicode code points) into the verse's text as drawn, without the verse number or notes. Each
// verse's text is drawn in its own element (span.vt with data-tb, the verse's English-numbered
// ID), so a place is found by counting the text inside that element.

export interface TextPoint { verse: number; offset: number }
export interface VerseRange { start: TextPoint; end: TextPoint }
export interface QuotePart { verse: number; text: string }

// The parts of a DOM node the maths reads, so tests can give plain objects.
export interface NodeLike {
  nodeType: number;
  data?: string;
  childNodes: ArrayLike<NodeLike>;
  parentNode: NodeLike | null;
  dataset?: Record<string, string | undefined>;
}

const TEXT = 3;

export const cpLength = (s: string): number => {
  let n = 0;
  for (const _ of s) n++;
  return n;
};

// unitOffset turns a count of code points into a JavaScript string index.
export function unitOffset(s: string, cp: number): number {
  let i = 0;
  let n = 0;
  for (const ch of s) {
    if (n >= cp) break;
    i += ch.length;
    n++;
  }
  return i;
}

// cpOffset turns a JavaScript string index into a count of code points.
export const cpOffset = (s: string, unit: number): number => cpLength(s.slice(0, unit));

export function compare(a: TextPoint, b: TextPoint): number {
  return a.verse - b.verse || a.offset - b.offset;
}

// verseTextOf finds the verse-text element a node is in, or null.
export function verseTextOf(n: NodeLike | null): NodeLike | null {
  for (let x = n; x; x = x.parentNode) if (x.dataset?.tb) return x;
  return null;
}

function textLength(n: NodeLike): number {
  if (n.nodeType === TEXT) return cpLength(n.data ?? "");
  let len = 0;
  for (const c of Array.from(n.childNodes)) len += textLength(c);
  return len;
}

// pointOf maps a DOM place (a node and an offset in it, as a selection gives) inside a verse's text
// to the verse and a character offset. It is null outside verse text.
export function pointOf(node: NodeLike, offset: number): TextPoint | null {
  const vt = verseTextOf(node);
  if (!vt) return null;
  let count = node.nodeType === TEXT ? cpOffset(node.data ?? "", offset)
    : Array.from(node.childNodes).slice(0, offset).reduce((n, c) => n + textLength(c), 0);
  // Add the text before node, up to the verse text element.
  for (let x = node; x !== vt && x.parentNode; x = x.parentNode) {
    for (const s of Array.from(x.parentNode.childNodes)) {
      if (s === x) break;
      count += textLength(s);
    }
  }
  return { verse: Number(vt.dataset!.tb), offset: count };
}

// nodeAt maps a character offset in a verse's text element back to a text node and an index in it.
// An offset past the end gives the end of the last text.
export function nodeAt(vt: NodeLike, offset: number): { node: NodeLike; offset: number } | null {
  let left = offset;
  let last: NodeLike | null = null;
  const walk = (n: NodeLike): { node: NodeLike; offset: number } | null => {
    if (n.nodeType === TEXT) {
      const len = cpLength(n.data ?? "");
      if (len > 0 && left <= len) return { node: n, offset: unitOffset(n.data ?? "", left) };
      left -= len;
      last = n;
      return null;
    }
    for (const c of Array.from(n.childNodes)) {
      const r = walk(c);
      if (r) return r;
    }
    return null;
  };
  const r = walk(vt);
  if (r) return r;
  const l = last as NodeLike | null;
  return l ? { node: l, offset: (l.data ?? "").length } : null;
}

// tidyRange keeps a selection's range to the verses it has words in: an end at the very start of a
// verse moves back to the end of the verse before, and a start at the very end of a verse moves on
// to the start of the next. verses are the shown verses' IDs and text lengths, in reading order.
export function tidyRange(r: VerseRange, verses: { verse: number; length: number }[]): VerseRange | null {
  let { start, end } = r;
  const i = verses.findIndex((v) => v.verse === start.verse);
  if (i >= 0 && start.offset >= verses[i].length && verses[i + 1] && verses[i + 1].verse <= end.verse) start = { verse: verses[i + 1].verse, offset: 0 };
  const j = verses.findIndex((v) => v.verse === end.verse);
  if (j > 0 && end.offset === 0 && verses[j - 1].verse >= start.verse) end = { verse: verses[j - 1].verse, offset: verses[j - 1].length };
  return compare(start, end) < 0 ? { start, end } : null;
}

// partsOf cuts each verse's words of a range out of the verses' texts (in reading order).
export function partsOf(r: VerseRange, texts: { verse: number; text: string }[]): QuotePart[] {
  const out: QuotePart[] = [];
  for (const t of texts) {
    if (t.verse < r.start.verse || t.verse > r.end.verse) continue;
    const chars = [...t.text];
    const a = t.verse === r.start.verse ? r.start.offset : 0;
    const b = t.verse === r.end.verse ? r.end.offset : chars.length;
    const s = chars.slice(a, b).join("");
    if (s) out.push({ verse: t.verse, text: s });
  }
  return out;
}

// A highlight as the page paints it.
export interface Painted {
  id: string;
  style: string;
  created_at?: number;
  verse_start?: number;
  verse_end?: number;
  range?: VerseRange;
  module?: string;
}

export interface Segment { start: number; end: number; hl: Painted }

// VersePaint is how one verse is drawn in one Bible: a highlight on the whole verse, or the
// highlighted words, or (for words highlighted in another Bible) the whole verse, faintly.
export interface VersePaint {
  whole: Painted | null;
  segments: Segment[];
  faint: Painted | null;
}

const age = (h: Painted) => h.created_at ?? 0;

// cover lays b over the segments: b wins where they overlap, so two colours never stack.
export function cover(segs: Segment[], b: Segment): Segment[] {
  const out: Segment[] = [];
  for (const s of segs) {
    if (s.end <= b.start || s.start >= b.end) { out.push(s); continue; }
    if (s.start < b.start) out.push({ ...s, end: b.start });
    if (s.end > b.end) out.push({ ...s, start: b.end });
  }
  out.push(b);
  return out.sort((x, y) => x.start - y.start);
}

// paintFor works out how verse is drawn in module, whose text there is length characters long.
// A whole-verse highlight is drawn on the verse as before, unless words in it are highlighted
// later: then it becomes words too, under the newer ones.
export function paintFor(list: Painted[], verse: number, length: number, module: string): VersePaint {
  const on = list.filter((h) => (h.range ? h.range.start.verse <= verse && h.range.end.verse >= verse
    : (h.verse_start ?? 0) <= verse && (h.verse_end ?? 0) >= verse)).sort((a, b) => age(a) - age(b));
  const wholes = on.filter((h) => !h.range);
  const whole = wholes[wholes.length - 1] ?? null;
  const mine = on.filter((h) => h.range && h.module === module && (!whole || age(h) >= age(whole)));
  const others = on.filter((h) => h.range && h.module !== module);
  let segs: Segment[] = [];
  for (const h of mine) {
    const r = h.range!;
    const start = r.start.verse === verse ? Math.min(r.start.offset, length) : 0;
    const end = r.end.verse === verse ? Math.min(r.end.offset, length) : length;
    if (start < end) segs = cover(segs, { start, end, hl: h });
  }
  if (whole && segs.length) {
    segs = segs.reduce((acc, s) => cover(acc, s), [{ start: 0, end: length, hl: whole }]);
    return { whole: null, segments: segs, faint: null };
  }
  return { whole, segments: segs, faint: !whole && !segs.length ? others[others.length - 1] ?? null : null };
}
