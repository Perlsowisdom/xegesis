// Markdown <-> a small block model, following any's canonical Markdown (docs/tasks/T29-export-backup.md,
// internal/backup/markdown.go): blocks are runs of non-blank lines, separated by one blank line; every
// further blank line is an empty paragraph; at the edges every blank line is an empty paragraph.
//
// The reader is deliberately conservative. It recognises only what the writer emits in exactly that
// form (headings "# ", bullets "- ", check items "- [ ] ", numbers "1. ", quotes "> ", a divider "---", tables,
// **bold**, *italic*, <u>underline</u>, `code`, [text](url)); anything else stays literal text, so a note written by hand
// is not rewritten. A quote whose last line is a credit ("> — [Rom 8:28](#passage/…) (KJV)…", as
// document.quote writes it) is a verse quotation, kept line for line.
// An ink block is a fenced ```ink block of one line of JSON (ink.ts).
// This file has no dependency on the editor, so it is tested on its own.

import { InkDoc, parseInk, serializeInk } from "./ink";

export interface Span {
  text: string;
  bold?: boolean;
  italic?: boolean;
  code?: boolean;
  underline?: boolean;
  link?: string;
  br?: boolean; // a line break inside a block; text is ""
}

export type Block =
  | { type: "paragraph"; spans: Span[] }
  | { type: "heading"; level: number; spans: Span[] }
  | { type: "quote"; spans: Span[] }
  // A check list ("- [ ] " and "- [x] ") has checked, one flag per item.
  | { type: "list"; ordered: boolean; items: Span[][]; checked?: boolean[] }
  | { type: "divider" }
  | { type: "ink"; ink: InkDoc }
  // lines are the quotation's lines without their "> " (a bare ">" is ""); the last is the credit.
  | { type: "versequote"; lines: string[] }
  // A table: rows of cells of spans. header: the first row is a header row. widths: each column's
  // width in pixels, once set by dragging.
  | { type: "table"; rows: Span[][][]; header: boolean; widths?: number[] };

// splitBlocks mirrors backup.ParseBlocks. An empty paragraph is "".
export function splitBlocks(md: string): string[] {
  if (md === "") return [];
  const blocks: string[] = [];
  let run: string[] = [];
  let blanks = 0;
  let started = false;
  const flush = (edge: boolean) => {
    for (let i = 0, n = edge ? blanks : blanks - 1; i < n; i++) blocks.push("");
    blanks = 0;
  };
  for (const line of md.split("\n")) {
    if (line === "") {
      if (run.length) {
        blocks.push(run.join("\n"));
        run = [];
      }
      blanks++;
      continue;
    }
    if (blanks > 0) flush(!started);
    started = true;
    run.push(line);
  }
  if (run.length) blocks.push(run.join("\n"));
  if (blanks > 0) flush(true);
  return blocks;
}

// joinBlocks mirrors backup.RenderBlocks.
export function joinBlocks(blocks: string[]): string {
  let out = "";
  let empties = 0;
  let seen = false;
  for (const b of blocks) {
    if (b === "") {
      empties++;
      continue;
    }
    out += "\n".repeat(seen ? empties + 2 : empties) + b;
    seen = true;
    empties = 0;
  }
  if (seen) out += "\n".repeat(empties);
  else if (empties > 1) out += "\n".repeat(empties - 1);
  return out;
}

// Reading -------------------------------------------------------------------------------------

export function parseMarkdown(md: string): Block[] {
  return splitBlocks(md).map(parseBlock);
}

function parseBlock(src: string): Block {
  if (src === "") return { type: "paragraph", spans: [] };
  const lines = src.split("\n");
  if (lines.length === 1) {
    const m = /^(#{1,6}) (\S.*)$/.exec(src);
    if (m) return { type: "heading", level: m[1].length, spans: parseInline(m[2]) };
    if (src === "---") return { type: "divider" };
  }
  if (lines.length > 1 && lines.every((l) => l === ">" || l.startsWith("> ")) && /^> — \S/.test(lines[lines.length - 1])) {
    return { type: "versequote", lines: lines.map((l) => l.slice(2)) };
  }
  const ink = lines.length === 3 ? parseInk(lines) : null;
  if (ink) return { type: "ink", ink };
  const table = parseTable(lines);
  if (table) return table;
  if (lines.every((l) => /^- \[[ x]\] \S/.test(l))) {
    return { type: "list", ordered: false, items: lines.map((l) => parseInline(l.slice(6))), checked: lines.map((l) => l[3] === "x") };
  }
  if (lines.every((l) => /^- \S/.test(l))) {
    return { type: "list", ordered: false, items: lines.map((l) => parseInline(l.slice(2))) };
  }
  if (lines.every((l, i) => l.startsWith(`${i + 1}. `) && /\S/.test(l.slice(`${i + 1}. `.length)))) {
    return { type: "list", ordered: true, items: lines.map((l, i) => parseInline(l.slice(`${i + 1}. `.length))) };
  }
  if (lines.every((l) => /^> \S/.test(l))) {
    return { type: "quote", spans: joinLines(lines.map((l) => parseInline(l.slice(2)))) };
  }
  return { type: "paragraph", spans: joinLines(lines.map((l) => parseInline(l))) };
}

// Tables are GitHub-style: a first row, a separator row ("| --- |"), then the other rows; a "|" in a
// cell is written "\|" and a line break "<br>". A comment line straight above keeps what the table
// itself cannot say: <!-- table widths=120,200 header=no --> (either part may be left out).
const TABLE_NOTE = /^<!-- table((?: (?:widths=\d{1,4}(?:,\d{1,4})*|header=no))*) -->$/;
const TABLE_SEP = /^\|(?: *:?-{3,}:? *\|)+$/;

function parseTable(lines: string[]): Block | null {
  let note = "";
  let rest = lines;
  const m = TABLE_NOTE.exec(lines[0] ?? "");
  if (m) {
    note = m[1];
    rest = lines.slice(1);
  }
  if (rest.length < 2 || !TABLE_SEP.test(rest[1]) || !rest.every((l) => l.length > 1 && l.startsWith("|") && l.endsWith("|"))) return null;
  const cols = rowCells(rest[1]).length;
  const rows = [rest[0], ...rest.slice(2)].map(rowCells);
  if (rows.some((r) => r.length !== cols)) return null;
  const widths = /widths=([\d,]+)/.exec(note)?.[1].split(",").map(Number);
  return {
    type: "table", header: !/header=no/.test(note),
    rows: rows.map((r) => r.map((c) => joinLines(c.split("<br>").map((part) => parseInline(part))))),
    ...(widths && widths.length === cols ? { widths } : {}),
  };
}

// rowCells splits "| a | b\|c |" into ["a", "b|c"].
function rowCells(line: string): string[] {
  const inner = line.slice(1, -1);
  const cells: string[] = [];
  let cur = "";
  for (let i = 0; i < inner.length; i++) {
    if (inner[i] === "\\" && inner[i + 1] === "|") {
      cur += "|";
      i++;
    } else if (inner[i] === "|") {
      cells.push(cur);
      cur = "";
    } else cur += inner[i];
  }
  cells.push(cur);
  return cells.map((c) => c.replace(/^ /, "").replace(/ $/, ""));
}

function writeTable(b: Extract<Block, { type: "table" }>): string {
  const cols = Math.max(1, ...b.rows.map((r) => r.length));
  const cell = (spans: Span[] | undefined) => writeInline(spans ?? []).replace(/\|/g, "\\|").replace(/\n/g, "<br>");
  const row = (r: Span[][]) => `| ${Array.from({ length: cols }, (_, i) => cell(r[i])).join(" | ")} |`;
  const parts = [
    ...(b.widths && b.widths.length === cols ? [` widths=${b.widths.map((w) => Math.round(w)).join(",")}`] : []),
    ...(b.header ? [] : [" header=no"]),
  ];
  const rows = b.rows.length ? b.rows : [[]];
  return [
    ...(parts.length ? [`<!-- table${parts.join("")} -->`] : []),
    row(rows[0]), `| ${Array.from({ length: cols }, () => "---").join(" | ")} |`, ...rows.slice(1).map(row),
  ].join("\n");
}

function joinLines(lines: Span[][]): Span[] {
  const out: Span[] = [];
  lines.forEach((l, i) => {
    if (i) out.push({ text: "", br: true });
    out.push(...l);
  });
  return out;
}

type Marks = Pick<Span, "bold" | "italic" | "code" | "underline" | "link">;

export function parseInline(s: string, marks: Marks = {}): Span[] {
  const out: Span[] = [];
  let lit = "";
  const flush = () => {
    if (lit) out.push({ text: lit, ...marks });
    lit = "";
  };
  let i = 0;
  while (i < s.length) {
    const rest = s.slice(i);
    let m: { len: number; spans: Span[] } | null = null;
    if (rest[0] === "`") m = codeAt(rest, marks);
    else if (rest.startsWith("***")) m = emphasisAt(rest, "***", { ...marks, bold: true, italic: true }) ?? emphasisAt(rest, "**", { ...marks, bold: true });
    else if (rest.startsWith("**")) m = emphasisAt(rest, "**", { ...marks, bold: true });
    else if (rest[0] === "*") m = emphasisAt(rest, "*", { ...marks, italic: true });
    else if (rest[0] === "[" && !marks.link) m = linkAt(rest, marks);
    else if (rest.startsWith("<u>") && !marks.underline) m = underlineAt(rest, marks);
    if (m) {
      flush();
      out.push(...m.spans);
      i += m.len;
    } else {
      lit += s[i++];
    }
  }
  flush();
  return out;
}

function codeAt(rest: string, marks: Marks): { len: number; spans: Span[] } | null {
  const end = rest.indexOf("`", 1);
  if (end < 2 || rest.slice(1, end).trim() !== rest.slice(1, end)) return null;
  return { len: end + 1, spans: [{ text: rest.slice(1, end), ...marks, code: true }] };
}

function emphasisAt(rest: string, mark: string, marks: Marks): { len: number; spans: Span[] } | null {
  const body = rest.slice(mark.length);
  if (!body || /^\s/.test(body) || body[0] === "*") return null;
  let from = 0;
  for (;;) {
    const end = body.indexOf(mark, from);
    if (end < 1) return null;
    const inner = body.slice(0, end);
    const after = body[end + mark.length];
    if (!/\s$/.test(inner) && !(mark === "*" && after === "*") && !(mark === "**" && after === "*")) {
      return { len: mark.length * 2 + end, spans: parseInline(inner, marks) };
    }
    from = end + 1;
  }
}

function linkAt(rest: string, marks: Marks): { len: number; spans: Span[] } | null {
  const m = /^\[([^\]\n]+)\]\(([^\s()]+)\)/.exec(rest);
  if (!m) return null;
  return { len: m[0].length, spans: parseInline(m[1], { ...marks, link: m[2] }) };
}

function underlineAt(rest: string, marks: Marks): { len: number; spans: Span[] } | null {
  const end = rest.indexOf("</u>", 3);
  if (end < 4) return null;
  return { len: end + 4, spans: parseInline(rest.slice(3, end), { ...marks, underline: true }) };
}

// Writing -------------------------------------------------------------------------------------

export function toMarkdown(blocks: Block[]): string {
  return joinBlocks(blocks.map(writeBlock));
}

function writeBlock(b: Block): string {
  switch (b.type) {
    case "paragraph":
      return writeInline(b.spans);
    case "heading":
      // An empty heading would read back as the text "##"; it is an empty paragraph instead.
      return b.spans.length === 0 ? "" : `${"#".repeat(b.level)} ${writeInline(b.spans)}`;
    case "quote":
      return writeInline(b.spans).split("\n").map((l) => `> ${l}`).join("\n");
    case "list":
      if (b.checked) return b.items.map((it, i) => `- [${b.checked![i] ? "x" : " "}] ${writeInline(it)}`).join("\n");
      return b.items.map((it, i) => `${b.ordered ? `${i + 1}. ` : "- "}${writeInline(it)}`).join("\n");
    case "divider":
      return "---";
    case "ink":
      return serializeInk(b.ink);
    case "versequote":
      return b.lines.map((l) => (l ? `> ${l}` : ">")).join("\n");
    case "table":
      return writeTable(b);
  }
}

type Mark = "link" | "bold" | "italic" | "underline" | "code";
const ORDER: Mark[] = ["link", "bold", "italic", "underline", "code"];

const has = (s: Span, m: Mark) => (m === "link" ? !!s.link : !!s[m]);
const same = (a: Span, b: Span, m: Mark) => !b.br && has(b, m) && (m !== "link" || a.link === b.link);

export function writeInline(spans: Span[]): string {
  return write(spans, new Set());
}

// Marks nest in a fixed order (link, bold, italic, underline, code); neighbouring pieces that share a mark are
// written inside one pair of marks, so marks are not closed and reopened between them.
function write(spans: Span[], used: Set<Mark>): string {
  let out = "";
  for (let i = 0; i < spans.length; ) {
    const s = spans[i];
    if (s.br) {
      out += "\n";
      i++;
      continue;
    }
    const m = ORDER.find((x) => !used.has(x) && has(s, x));
    if (!m) {
      out += s.text;
      i++;
      continue;
    }
    let j = i + 1;
    while (j < spans.length && same(s, spans[j], m)) j++;
    const inner = write(spans.slice(i, j), new Set([...used, m]));
    out += wrap(m, inner, s.link);
    i = j;
  }
  return out;
}

// wrap puts marks round text without its edge spaces, which Markdown does not allow inside them.
function wrap(m: Mark, text: string, url?: string): string {
  const lead = /^\s*/.exec(text)![0];
  const body = text.slice(lead.length).replace(/\s+$/, "");
  const trail = text.slice(lead.length + body.length);
  if (body === "") return text;
  const marked = m === "bold" ? `**${body}**` : m === "italic" ? `*${body}*` : m === "code" ? `\`${body}\``
    : m === "underline" ? `<u>${body}</u>` : `[${body}](${url})`;
  return m === "link" ? marked : lead + marked + trail;
}

// Pasting ---------------------------------------------------------------------------------------

// parsePasted reads Markdown pasted as plain text, more freely than stored notes are read: each
// line is looked at on its own, so "# Title" straight above a paragraph is still a heading; "*" and
// "+" bullets, "1)" numbers, "__underline__" and "_italic_" are understood; each other line is a
// paragraph of its own. Blank lines only separate.
export function parsePasted(text: string): Block[] {
  const out: Block[] = [];
  let list: Extract<Block, { type: "list" }> | null = null;
  let quote: Span[][] | null = null;
  const end = () => {
    if (quote) out.push({ type: "quote", spans: joinLines(quote) });
    list = null;
    quote = null;
  };
  const item = (ordered: boolean, check: boolean | null, body: string) => {
    if (quote) end();
    const kind = (l: typeof list) => !!l && l.ordered === ordered && !!l.checked === (check !== null);
    if (!kind(list)) {
      end();
      list = { type: "list", ordered, items: [], ...(check !== null ? { checked: [] } : {}) };
      out.push(list);
    }
    list!.items.push(pastedInline(body));
    if (check !== null) list!.checked!.push(check);
  };
  const rule = /^\s*([-*_])(\s*\1){2,}\s*$/;
  for (const raw of text.replace(/\r\n?/g, "\n").split("\n")) {
    const line = raw.replace(/\s+$/, "");
    let m: RegExpExecArray | null;
    if (!line.trim()) end();
    else if ((m = /^\s*[-*+] \[([ xX])\]\s+(.*)$/.exec(line))) item(false, m[1] !== " ", m[2]);
    else if ((m = /^\s*[-*+]\s+(.*)$/.exec(line)) && !rule.test(line)) item(false, null, m[1]);
    else if ((m = /^\s*\d{1,3}[.)]\s+(.*)$/.exec(line))) item(true, null, m[1]);
    else if ((m = /^>\s?(.*)$/.exec(line))) {
      if (list) end();
      (quote ??= []).push(pastedInline(m[1]));
    } else {
      end();
      if ((m = /^(#{1,6})\s+(.+)$/.exec(line))) out.push({ type: "heading", level: m[1].length, spans: pastedInline(m[2]) });
      else if (rule.test(line)) out.push({ type: "divider" });
      else out.push({ type: "paragraph", spans: pastedInline(line) });
    }
  }
  end();
  return out;
}

// pastedInline also reads underscores for underline and italic, but only round whole words.
function pastedInline(s: string): Span[] {
  return parseInline(s
    .replace(/(^|[^\w_])__(?=\S)([^_]*?\S)__(?![\w_])/g, "$1<u>$2</u>")
    .replace(/(^|[^\w_])_(?=[^\s_])([^_]*?[^\s_])_(?![\w_])/g, "$1*$2*"));
}
