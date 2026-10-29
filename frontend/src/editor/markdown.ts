// Markdown <-> a small block model, following any's canonical Markdown (docs/tasks/T29-export-backup.md,
// internal/backup/markdown.go): blocks are runs of non-blank lines, separated by one blank line; every
// further blank line is an empty paragraph; at the edges every blank line is an empty paragraph.
//
// The reader is deliberately conservative. It recognises only what the writer emits in exactly that
// form (headings "# ", bullets "- ", numbers "1. ", quotes "> ", **bold**, *italic*, `code`,
// [text](url)); anything else stays literal text, so a note written by hand is not rewritten.
// This file has no dependency on the editor, so it is tested on its own.

export interface Span {
  text: string;
  bold?: boolean;
  italic?: boolean;
  code?: boolean;
  link?: string;
  br?: boolean; // a line break inside a block; text is ""
}

export type Block =
  | { type: "paragraph"; spans: Span[] }
  | { type: "heading"; level: number; spans: Span[] }
  | { type: "quote"; spans: Span[] }
  | { type: "list"; ordered: boolean; items: Span[][] };

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

function joinLines(lines: Span[][]): Span[] {
  const out: Span[] = [];
  lines.forEach((l, i) => {
    if (i) out.push({ text: "", br: true });
    out.push(...l);
  });
  return out;
}

type Marks = Pick<Span, "bold" | "italic" | "code" | "link">;

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

// Writing -------------------------------------------------------------------------------------

export function toMarkdown(blocks: Block[]): string {
  return joinBlocks(blocks.map(writeBlock));
}

function writeBlock(b: Block): string {
  switch (b.type) {
    case "paragraph":
      return writeInline(b.spans);
    case "heading":
      return `${"#".repeat(b.level)} ${writeInline(b.spans)}`;
    case "quote":
      return writeInline(b.spans).split("\n").map((l) => `> ${l}`).join("\n");
    case "list":
      return b.items.map((it, i) => `${b.ordered ? `${i + 1}. ` : "- "}${writeInline(it)}`).join("\n");
  }
}

type Mark = "link" | "bold" | "italic" | "code";
const ORDER: Mark[] = ["link", "bold", "italic", "code"];

const has = (s: Span, m: Mark) => (m === "link" ? !!s.link : !!s[m]);
const same = (a: Span, b: Span, m: Mark) => !b.br && has(b, m) && (m !== "link" || a.link === b.link);

export function writeInline(spans: Span[]): string {
  return write(spans, new Set());
}

// Marks nest in a fixed order (link, bold, italic, code); neighbouring pieces that share a mark are
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
  const marked = m === "bold" ? `**${body}**` : m === "italic" ? `*${body}*` : m === "code" ? `\`${body}\`` : `[${body}](${url})`;
  return m === "link" ? marked : lead + marked + trail;
}
