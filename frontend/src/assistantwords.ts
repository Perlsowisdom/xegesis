// The assistant's words and the pure parts of its chat: splitting a reply into text and drafts,
// and turning the Bible references found in it into links the chat shows as verse chips.

export interface Preset {
  id: string;
  label: string;
  base_url: string;
  local: boolean;
  needs_key: boolean;
}

export interface AssistantConfig {
  provider: string;
  label?: string;
  base_url?: string;
  model?: string;
  local: boolean;
  key_saved: boolean;
  needs_key: boolean;
  confirmed: boolean;
  presets: Preset[];
}

export interface ToolLine {
  id: string;
  name: string;
  args: string;
  label: string;
  content?: string;
  notice?: string;
}

export interface AssistantEvent {
  conversation_id: string;
  type: "start" | "text" | "tool" | "tool_done" | "done" | "error";
  text?: string;
  tool?: ToolLine;
  error?: string;
}

// The chips of Settings → Assistant → Model, in two groups.
export const ONLINE_CHOICES = ["claude", "openai", "gemini", "openrouter"];
export const LOCAL_CHOICES = ["claude-code", "codex", "local"];
// Claude Code and Codex on this computer come with T36b.
export const LATER: Record<string, string> = { "claude-code": "Claude Code", codex: "Codex" };

export function providerLine(c: AssistantConfig): string {
  if (c.provider === "off" || !c.label) return "Off";
  if (c.provider === "custom") return c.local ? "Custom, on this computer" : "Custom address";
  if (c.provider === "local") return c.local ? "Local model, on this computer" : "Local model, at another address";
  return `${c.label}, with your own key`;
}

export interface Segment {
  kind: "text" | "draft";
  text: string;
  open?: boolean; // a draft still being written
}

const FENCE = /^```draft[^\n]*\n/m;

// segments splits a reply into text and ```draft blocks.
export function segments(reply: string): Segment[] {
  const out: Segment[] = [];
  let rest = reply;
  for (;;) {
    const m = FENCE.exec(rest);
    if (!m) break;
    if (m.index > 0) out.push({ kind: "text", text: rest.slice(0, m.index) });
    rest = rest.slice(m.index + m[0].length);
    const end = /^```\s*$/m.exec(rest);
    if (!end) {
      out.push({ kind: "draft", text: rest, open: true });
      return out.filter((s) => s.kind === "draft" || s.text.trim());
    }
    out.push({ kind: "draft", text: rest.slice(0, end.index).replace(/\n$/, "") });
    rest = rest.slice(end.index + end[0].length);
  }
  if (rest) out.push({ kind: "text", text: rest });
  return out.filter((s) => s.kind === "draft" || s.text.trim());
}

// draftParts is a draft's title (its first heading, or its first words) and its body without
// that heading, for a new note.
export function draftParts(md: string): { title: string; body: string } {
  const lines = md.split("\n");
  const i = lines.findIndex((l) => l.trim());
  if (i >= 0) {
    const h = /^#{1,6}\s+(.*)$/.exec(lines[i].trim());
    if (h) return { title: h[1].trim(), body: lines.slice(i + 1).join("\n").trim() };
  }
  const words = md.replace(/[#>*_`[\]]/g, "").trim().split(/\s+/).slice(0, 8).join(" ");
  return { title: words || "Draft from the assistant", body: md.trim() };
}

export interface FoundRef {
  start: number;
  end: number;
  ref: string; // OSIS
}

// linkRefs turns found references into Markdown links to the passage, leaving ones already
// inside a link, or inside code, as they are.
export function linkRefs(text: string, refs: FoundRef[]): string {
  const skip: [number, number][] = [];
  for (const m of text.matchAll(/\[[^\]\n]*\]\([^)\n]*\)|`[^`\n]*`/g)) skip.push([m.index!, m.index! + m[0].length]);
  let out = "";
  let at = 0;
  for (const r of [...refs].sort((a, b) => a.start - b.start)) {
    if (r.start < at || skip.some(([s, e]) => r.start < e && r.end > s)) continue;
    out += text.slice(at, r.start) + `[${text.slice(r.start, r.end)}](#passage/${encodeURIComponent(r.ref)})`;
    at = r.end;
  }
  return out + text.slice(at);
}

// toolSummary is the short line a tool call shows while it runs.
export function runningLabel(name: string): string {
  const words: Record<string, string> = {
    search_library: "Searching the library", read_passage: "Reading the passage", list_bibles: "Listing your Bibles",
    read_book: "Reading the book", list_notes: "Looking through your notes", read_note: "Reading a note",
    list_sermons: "Looking through your sermons", read_sermon: "Reading a sermon", list_highlights: "Looking through your highlights",
    passage_report: "Gathering a passage report", word_report: "Gathering a word report", look_up_word: "Looking up the word",
  };
  return (words[name] ?? "Working") + "…";
}
