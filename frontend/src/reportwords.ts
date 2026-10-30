// Reports (views/report.ts): the result's types, the app addresses a report and its saved note link
// to, and the small decisions the view makes. No DOM here, so it is tested on its own.

export type ReportKind = "passage" | "word" | "topic";

export interface ReportSource { module?: string; name: string; attribution?: string; licence?: string; spdx?: string }
export interface ReportLink { ref: string; label: string }
export interface ReportBook {
  item_id: string; file_id: string; title: string; authors: string[]; heading?: string;
  locator: { spine?: number; href?: string; cfi?: string; page?: number; para?: number; offset?: number }; online: boolean;
}
export interface ReportEntry {
  heading?: string; text?: string; markup?: string; snippet?: string; ref?: string; ref_label?: string;
  strong?: string; lemma?: string; transliteration?: string; gloss?: string; morphology?: string; count?: number;
  links?: ReportLink[]; book?: ReportBook; note_id?: string; list_id?: string; plan_id?: string; day?: number;
  done?: boolean; topic?: string; module?: string; extra?: string[];
}
export interface Bar { label: string; book: string; count: number }
export interface ReportSection {
  id: string; group: string; title: string; sources: ReportSource[]; entries: ReportEntry[];
  chart?: Bar[]; total: number; more?: string; hint?: string; error?: string;
}
export interface Report {
  kind: ReportKind; query: string; subject: string; title: string; ref?: string; strong?: string;
  candidates?: ReportEntry[]; sections: ReportSection[];
}

export const KINDS: [ReportKind, string, string, string][] = [
  ["passage", "Passage", "e.g. Romans 8:28-30", "ref"],
  ["word", "Word", "e.g. G25, ἀγαπάω or love", "word"],
  ["topic", "Topic", "e.g. grace or covenant", "topic"],
];

// paramFor is the report command's name for what the report is about.
export function paramFor(kind: ReportKind): string {
  return KINDS.find((k) => k[0] === kind)?.[3] ?? "ref";
}

// Where an app address leads: a passage, a report, a book place or another section.
export type Route =
  | { to: "passage"; ref: string }
  | { to: "report"; kind: ReportKind; query: string }
  | { to: "book"; itemId: string; fileId?: string; locator: ReportBook["locator"] }
  | { to: "section"; section: string };

const SECTIONS: Record<string, string> = { note: "notes", list: "lists", plan: "plans" };

// routeOf reads the addresses that reports and their saved notes use: #passage/Rom.8.28,
// #word/G25, #topic/grace, #report/passage/Rom.8.28, #book/<item>?file=…&spine=2&para=1,
// #note/<id>, #list/<id> and #plan/<id>. It returns null for anything else, such as a section name.
export function routeOf(hash: string): Route | null {
  const h = hash.replace(/^#/, "");
  const slash = h.indexOf("/");
  if (slash < 0) return null;
  const head = h.slice(0, slash);
  let rest = h.slice(slash + 1);
  const dec = (s: string) => {
    try {
      return decodeURIComponent(s);
    } catch {
      return s;
    }
  };
  if (!rest) return null;
  switch (head) {
    case "passage":
      return { to: "passage", ref: dec(rest) };
    case "word":
    case "topic":
      return { to: "report", kind: head, query: dec(rest) };
    case "report": {
      const i = rest.indexOf("/");
      const kind = rest.slice(0, i) as ReportKind;
      if (i < 0 || !KINDS.some((k) => k[0] === kind) || !rest.slice(i + 1)) return null;
      return { to: "report", kind, query: dec(rest.slice(i + 1)) };
    }
    case "book": {
      const q = rest.indexOf("?");
      const params = new URLSearchParams(q < 0 ? "" : rest.slice(q + 1));
      rest = q < 0 ? rest : rest.slice(0, q);
      const locator: ReportBook["locator"] = {};
      for (const k of ["spine", "page", "para", "offset"] as const) {
        const n = Number(params.get(k));
        if (n > 0) locator[k] = n;
      }
      if (params.get("href")) locator.href = params.get("href")!;
      return { to: "book", itemId: dec(rest), fileId: params.get("file") || undefined, locator };
    }
  }
  return SECTIONS[head] ? { to: "section", section: SECTIONS[head] } : null;
}

// runs groups neighbouring sections of one group, so a passage's text in every Bible is shown side
// by side.
export function runs(sections: ReportSection[]): { group: string; sections: ReportSection[] }[] {
  const out: { group: string; sections: ReportSection[] }[] = [];
  for (const s of sections) {
    const last = out[out.length - 1];
    if (last && last.group === s.group) last.sections.push(s);
    else out.push({ group: s.group, sections: [s] });
  }
  return out;
}

// sourceText credits a section's sources: "From Easton's Bible Dictionary" or, with an
// attribution, "From TBESG. STEPBible.org, CC BY 4.0".
export function sourceText(sources: ReportSource[]): string {
  const named = sources.filter((s) => s.name);
  if (named.length === 0) return "";
  return "From " + named.map((s) => (s.attribution && s.attribution !== s.name ? `${s.name}. ${s.attribution}` : s.name)).join("; ");
}

// countText is how many entries a section holds, in words.
export function countText(s: ReportSection): string {
  const n = Math.max(s.total, s.entries.length);
  if (n <= 1) return "";
  if (!s.more) return String(n);
  return s.total > s.entries.length ? `${s.entries.length} of ${s.total}` : `${s.entries.length}+`;
}

// barWidths scales a chart's bars to the longest, as percentages of at least 2 so a bar of one shows.
export function barWidths(chart: Bar[]): number[] {
  const most = Math.max(1, ...chart.map((b) => b.count));
  return chart.map((b) => Math.max(2, Math.round((b.count / most) * 100)));
}

// wordFacts is the line under a word: lemma, transliteration, gloss, number and morphology.
export function wordFacts(e: ReportEntry): string {
  return [e.lemma && e.lemma !== e.heading ? e.lemma : "", e.transliteration, e.gloss, e.strong, e.morphology]
    .filter(Boolean).join(" · ");
}

// openByDefault decides which sections start open: the first few, and every short one.
export function openByDefault(index: number, s: ReportSection): boolean {
  return index < 4 || s.entries.length <= 3;
}
