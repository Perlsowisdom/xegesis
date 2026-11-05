// Words and small rules for the Sermons section, kept apart from the view so they can be tested.

import type { Block, Span } from "./editor/markdown";

export type DocKind = "sermon" | "lesson" | "study";

export interface Point {
  h: string;
  ref?: string;
  level?: 1 | 2;
}

export const KIND_NAME: Record<DocKind, string> = { sermon: "Sermon", lesson: "Lesson", study: "Bible study" };

export function untitled(kind: DocKind): string {
  return { sermon: "Untitled sermon", lesson: "Untitled lesson", study: "Untitled study" }[kind] ?? "Untitled";
}

// sermonAddress reads #sermons/<id>, the address that opens one document. A document just made
// elsewhere (My work → New → Sermon) is opened with ?new, so its title starts empty and takes the keyboard.
export function sermonAddress(hash: string): { id: string; fresh: boolean } | null {
  const m = /^#sermons\/([^?]+)(\?new)?$/.exec(hash);
  return m ? { id: decodeURIComponent(m[1]), fresh: !!m[2] } : null;
}

// docSummary is the line under a document's title in the list: kind, passage and length.
export function docSummary(d: { kind: DocKind; passage?: { display: string }; points: Point[]; words: number }): string {
  const parts: string[] = [KIND_NAME[d.kind] ?? d.kind];
  if (d.passage?.display) parts.push(d.passage.display);
  if (d.points.length) parts.push(`${d.points.length} ${d.points.length === 1 ? "point" : "points"}`);
  parts.push(`${d.words.toLocaleString("en")} ${d.words === 1 ? "word" : "words"}`);
  return parts.join(" · ");
}

// cleanPoints drops empty points and trims the rest, as the core will.
export function cleanPoints(points: Point[]): Point[] {
  return points
    .map((p) => ({ h: p.h.trim().replace(/\s+/g, " "), ref: (p.ref ?? "").trim(), level: p.level === 2 ? 2 as const : 1 as const }))
    .filter((p) => p.h || p.ref);
}

// outlineMarkdown turns the outline into headings to write under: main points become "##"
// headings and sub-points "###", each followed by its verses.
export function outlineMarkdown(points: Point[]): string {
  const out: string[] = [];
  let main = 0;
  for (const p of points) {
    if (p.level === 2) {
      out.push(`### ${p.h || p.ref}`);
    } else {
      main++;
      out.push(`## ${main}. ${p.h || p.ref}`);
    }
    if (p.ref && p.h) out.push(`*${p.ref}*`);
  }
  return out.join("\n\n");
}

// The outline is the text's headings ------------------------------------------------------------
// A sermon's outline is read from its headings: "#" and "##" are main points, deeper ones
// sub-points. A point's verses are the passage links in its section (a quotation's credit line, or a
// verse reference), up to the next heading. These work on the Markdown block model, so the same
// rules give the outline column, the saved outline (document.update's points, kept for export)
// and the migration of older outlines.


export interface OutlinePoint {
  index: number; // the heading's block
  h: string;
  level: 1 | 2;
  refs: { label: string; ref: string }[];
}

const MAX_POINT = 300;

// headingText is a heading's words without a leading "1." or "2)" the writer may have typed.
export function headingText(spans: Span[]): string {
  const t = spans.filter((s) => !s.br).map((s) => s.text).join("").replace(/\s+/g, " ").trim();
  return t.replace(/^\d{1,3}[.)]\s+(?=\S)/, "").slice(0, MAX_POINT);
}

// passageLinks finds links to passages ("#passage/…") in a block, as label and reference.
export function passageLinks(b: Block): { label: string; ref: string }[] {
  const out: { label: string; ref: string }[] = [];
  const add = (label: string, url: string) => {
    if (!url.startsWith("#passage/")) return;
    let ref = url.slice("#passage/".length);
    try { ref = decodeURIComponent(ref); } catch { /* keep it as it is */ }
    if (ref && !out.some((x) => x.ref === ref)) out.push({ label: label.trim() || ref, ref });
  };
  const spans = b.type === "list" ? b.items.flat() : b.type === "paragraph" || b.type === "heading" || b.type === "quote" ? b.spans : [];
  let url = "";
  let text = "";
  for (const s of [...spans, { text: "" } as Span]) {
    if (url && s.link !== url) {
      add(text, url);
      url = "";
    }
    if (s.link) {
      text = url ? text + s.text : s.text;
      url = s.link;
    }
  }
  if (b.type === "versequote") {
    for (const m of b.lines[b.lines.length - 1].matchAll(/\[([^\]\n]+)\]\(([^\s()]+)\)/g)) add(m[1], m[2]);
  }
  return out;
}

// outlineOf lists the headings of a text as an outline.
export function outlineOf(blocks: Block[]): OutlinePoint[] {
  const out: OutlinePoint[] = [];
  blocks.forEach((b, i) => {
    if (b.type === "heading") {
      out.push({ index: i, h: headingText(b.spans), level: b.level <= 2 ? 1 : 2, refs: [] });
      return;
    }
    const cur = out[out.length - 1];
    if (cur) for (const r of passageLinks(b)) if (!cur.refs.some((x) => x.ref === r.ref)) cur.refs.push(r);
  });
  return out;
}

// pointsOf is the outline as document.update saves it: each point's first verses only.
export function pointsOf(blocks: Block[]): Point[] {
  return outlineOf(blocks).filter((p) => p.h || p.refs.length).map((p) => ({ h: p.h, ref: p.refs[0]?.ref ?? "", level: p.level }));
}

// sectionEnd is where the section that starts with the heading at i ends: the next heading of the
// same or a higher level, or the end of the text. levels[j] is a block's heading level, or 0.
export function sectionEnd(levels: number[], i: number): number {
  for (let j = i + 1; j < levels.length; j++) if (levels[j] && levels[j] <= levels[i]) return j;
  return levels.length;
}

// moveSection is the order of the blocks after the section at from moves before block to (or to
// the end when to is levels.length). Moving a section into itself changes nothing.
export function moveSection(levels: number[], from: number, to: number): number[] {
  const order = levels.map((_, i) => i);
  const end = sectionEnd(levels, from);
  if (to >= from && to <= end) return order;
  const moving = order.slice(from, end);
  const rest = order.filter((i) => i < from || i >= end);
  const at = to >= levels.length ? rest.length : rest.indexOf(to);
  rest.splice(at, 0, ...moving);
  return rest;
}

// migrateOutline is the Markdown to add to a text so that an older, separate outline lives on as
// headings: every point not already a heading, with a link to its verses. "" when nothing is missing.
export function migrateOutline(points: Point[], blocks: Block[]): string {
  const have = new Set(outlineOf(blocks).map((p) => p.h.toLowerCase()));
  const out: string[] = [];
  for (const p of cleanPoints(points)) {
    const h = (p.h || p.ref || "").replace(/^\d{1,3}[.)]\s+/, "");
    if (!h || have.has(h.toLowerCase())) continue;
    out.push(`${p.level === 2 ? "###" : "##"} ${h}`);
    if (p.ref && p.h) out.push(`[${p.ref}](#passage/${encodeURIComponent(p.ref)})`);
  }
  return out.join("\n\n");
}
