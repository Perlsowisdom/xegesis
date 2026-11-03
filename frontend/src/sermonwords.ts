// Words and small rules for the Sermons section, kept apart from the view so they can be tested.

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
