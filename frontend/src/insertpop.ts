// What the note and sermon editors' Insert control needs from the app: pickers for a highlight and
// for a note or sermon to link to, checking a typed reference, opening a chip, and turning a
// dropped verse, highlight or Related card into Markdown. Bible text always goes through
// document.quote, and commentary text through licence.check, so each licence's rules apply.

import { Module, Note, Ref, run } from "./api";
import { installedBibles } from "./bibles";
import { clear, h } from "./dom";
import { openInStudy, studyPassage } from "./document/open";
import type { EditorOptions } from "./editor";
import type { HighlightRow } from "./highlightspanel";
import { highlightQuoteLines, highlightText, type CharRange } from "./insertwords";
import { parseStyle } from "./markingwords";
import { closePopover, openPopover } from "./popover";
import { setPassage, state } from "./state";
import { verseQuoteFor } from "./docpage";
import type { SermonDoc } from "./views/sermons";

// The Bible quotations and highlights are read from: the one being read, else the first installed.
async function currentBible(): Promise<string> {
  const mods = await installedBibles();
  return state.modules.find((m) => mods.some((x) => x.id === m)) ?? mods[0]?.id ?? "";
}

// highlightQuote is a highlight as a quotation with its place and a link back. A Bible highlight's
// words come through document.quote; a book highlight's are its own. range, when given, is the
// part of the highlighted words to quote (T34f: highlights within verses).
export async function highlightQuote(row: HighlightRow, range?: CharRange): Promise<{ md: string } | { reason: string }> {
  if (row.ref) {
    // Exact words are quoted from the Bible they were highlighted in, when it is still installed.
    const installed = row.range && row.module ? (await installedBibles()).some((m) => m.id === row.module) : false;
    const module = installed ? row.module! : await currentBible();
    if (!module) return { reason: "Install a Bible first, from Library → Get Bibles" };
    const words = installed ? highlightText(row, range) : "";
    return verseQuoteFor(row.ref, module, words);
  }
  const text = highlightText(row, range);
  if (!text) return { reason: "this highlight kept no words" };
  return { md: highlightQuoteLines(row, text).map((l) => (l ? `> ${l}` : ">")).join("\n") };
}

// A list picker in a popover: a search box over a list of choices, arrows and Enter to choose.
function pickerPopover<T>(anchor: HTMLElement, o: {
  title: string; placeholder: string; empty: string;
  load(query: string): Promise<T[]>;
  row(item: T): Node[];
  choose(item: T, say: (why: string) => void): Promise<void> | void;
  className: string;
}): void {
  const input = h("input", { type: "search", placeholder: o.placeholder, autocomplete: "off", spellcheck: "false", "aria-label": o.title,
    role: "combobox", "aria-expanded": "true", "aria-autocomplete": "list" });
  const list = h("ul", { class: "pick-list", role: "listbox", "aria-label": o.title, id: `pick-${Math.random().toString(36).slice(2)}` });
  input.setAttribute("aria-controls", list.id);
  const why = h("p", { class: "ref-note bad", role: "alert" });
  let items: T[] = [];
  let at = 0;
  let seq = 0;
  let timer = 0;
  const mark = () => {
    [...list.children].forEach((c, i) => c.setAttribute("aria-selected", String(i === at)));
    const cur = list.children[at] as HTMLElement | undefined;
    if (cur) input.setAttribute("aria-activedescendant", cur.id);
    cur?.scrollIntoView({ block: "nearest" });
  };
  const pick = async (i: number) => {
    const it = items[i];
    if (!it) return;
    why.textContent = "";
    await o.choose(it, (w) => (why.textContent = w));
  };
  const draw = () => {
    if (items.length === 0) {
      clear(list, h("li", { class: "pick-empty", role: "presentation" }, o.empty));
      return;
    }
    clear(list, items.map((it, i) => {
      const li = h("li", { role: "option", class: "pick-row", id: `${list.id}-${i}`, "aria-selected": String(i === at) }, o.row(it));
      li.addEventListener("mousedown", (e) => e.preventDefault());
      li.addEventListener("click", () => void pick(i));
      li.addEventListener("mousemove", () => { if (at !== i) { at = i; mark(); } });
      return li;
    }));
    mark();
  };
  const refresh = async () => {
    const n = ++seq;
    const got = await o.load(input.value.trim()).catch(() => [] as T[]);
    if (n !== seq) return;
    items = got;
    at = 0;
    draw();
  };
  input.addEventListener("input", () => {
    clearTimeout(timer);
    timer = window.setTimeout(() => void refresh(), 180);
  });
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      if (!items.length) return;
      at = (at + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length;
      mark();
    } else if (e.key === "Enter") {
      e.preventDefault();
      void pick(at);
    }
  });
  const body = h("div", { class: `pop-body pick-pop ${o.className}` }, h("p", { class: "pop-title" }, o.title), input, why, list);
  openPopover(anchor, body, { label: o.title, className: "doc-pop" });
  input.focus();
  void refresh();
}

// highlightPicker lists the user's highlights, newest first, with their colours; choosing one
// inserts it as a quotation.
export function highlightPicker(anchor: HTMLElement, insert: (md: string) => void): void {
  pickerPopover<HighlightRow>(anchor, {
    title: "Insert a highlight", placeholder: "Search the highlighted words and places", className: "hl-pick",
    empty: "No highlights yet. Select words in a Bible or book and pick a colour.",
    async load(query) {
      const module = await currentBible();
      return run<HighlightRow[]>("highlight.list", { newest: true, limit: 100, ...(module ? { module } : {}), ...(query ? { query } : {}) });
    },
    row(r) {
      const s = parseStyle(r.style);
      const words = (r.text ?? "").replace(/\s+/g, " ").trim();
      const where = r.ref || [r.item_title || "A book", r.locator?.page ? `page ${r.locator.page}` : ""].filter(Boolean).join(" · ");
      return [h("span", { class: `mark-dot dot-${s.colour}`, "aria-hidden": "true" }),
        h("span", { class: "pick-main" }, h("span", { class: "pick-text" }, words ? `“${words.length > 120 ? `${words.slice(0, 120)}…` : words}”` : "(no words kept)"),
          h("span", { class: "pick-where" }, where, ` · ${s.colour}`))];
    },
    async choose(r, say) {
      const got = await highlightQuote(r);
      if ("reason" in got) return say(`Not inserted: ${got.reason}.`);
      closePopover();
      insert(got.md);
    },
  });
}

type LinkItem = { kind: "note" | "sermon"; id: string; title: string; when: number };

// noteLinkPicker lists notes and sermons by title, most recent first; choosing one inserts a chip.
export function noteLinkPicker(anchor: HTMLElement, insert: (label: string, href: string) => void, except?: () => string | undefined): void {
  let all: LinkItem[] | null = null;
  pickerPopover<LinkItem>(anchor, {
    title: "Link to a note or sermon", placeholder: "Search by title", className: "note-pick",
    empty: "Nothing with that title.",
    async load(query) {
      if (!all) {
        const [notes, docs] = await Promise.all([
          run<Note[]>("note.list", { limit: 500 }).catch(() => [] as Note[]),
          run<SermonDoc[]>("document.list", {}).catch(() => [] as SermonDoc[]),
        ]);
        all = [
          ...notes.map((n): LinkItem => ({ kind: "note", id: n.id, title: n.title || "Untitled note", when: n.updated_at })),
          ...docs.map((d): LinkItem => ({ kind: "sermon", id: d.id, title: d.title || "Untitled sermon", when: (d as unknown as { updated_at: number }).updated_at ?? 0 })),
        ].sort((a, b) => b.when - a.when);
      }
      const skip = except?.();
      const words = query.toLowerCase().split(/\s+/).filter(Boolean);
      return all.filter((x) => x.id !== skip && words.every((w) => x.title.toLowerCase().includes(w))).slice(0, 60);
    },
    row: (x) => [h("span", { class: "pick-kind", "aria-hidden": "true" }, x.kind === "sermon" ? "§" : "¶"),
      h("span", { class: "pick-main" }, h("span", { class: "pick-text" }, x.title), h("span", { class: "pick-where" }, x.kind === "sermon" ? "Sermon" : "Note"))],
    choose(x) {
      closePopover();
      insert(x.title, x.kind === "sermon" ? `#sermons/${x.id}` : `#notes/note/${x.id}`);
    },
  });
}

// parseRef checks a typed reference.
export async function parseRef(text: string): Promise<{ display: string; osis: string } | null> {
  const p = await run<Ref>("verse.parse", { ref: text }).catch(() => null);
  return p ? { display: p.display, osis: p.osis } : null;
}

// openAppLink opens what a chip names: verses in the study grid, a note or sermon in a tab there.
export async function openAppLink(href: string, title = ""): Promise<void> {
  const dec = (s: string) => { try { return decodeURIComponent(s); } catch { return s; } };
  if (href.startsWith("#passage/")) {
    const p = await run<Ref>("verse.parse", { ref: dec(href.slice("#passage/".length)) }).catch(() => null);
    if (!p) return;
    setPassage(p.display, p);
    studyPassage();
  } else if (href.startsWith("#notes/note/")) {
    openInStudy({ kind: "note", target: dec(href.slice("#notes/note/".length)), title });
  } else if (href.startsWith("#sermons/")) {
    openInStudy({ kind: "sermon", target: dec(href.slice("#sermons/".length)), title });
  } else location.hash = href;
}

// dropToMarkdown turns a dropped verse, highlight or Related card into the Markdown to insert.
export async function dropToMarkdown(data: Record<string, string>, say: (why: string) => void): Promise<string | null> {
  const json = (t: string) => { try { return JSON.parse(data[t] ?? "null"); } catch { return null; } };
  const verse = json("application/x-xegesis-verse") as { ref: string; module: string } | null;
  if (verse?.ref) {
    const got = await verseQuoteFor(verse.ref, verse.module || await currentBible());
    if ("reason" in got) { say(`Not quoted: ${got.reason}.`); return null; }
    return got.md;
  }
  const hl = json("application/x-xegesis-highlight") as HighlightRow | null;
  if (hl?.id) {
    const got = await highlightQuote(hl);
    if ("reason" in got) { say(`Not inserted: ${got.reason}.`); return null; }
    return got.md;
  }
  const rel = json("application/x-xegesis-related") as { module: string; name: string; ref: string; osis?: string; text: string } | null;
  if (rel?.module) {
    const mods = await run<Module[]>("bible.modules", { kind: "commentary" }).catch(() => [] as Module[]);
    const m = mods.find((x) => x.id === rel.module);
    const d = await run<{ allowed: boolean; attribution?: string; reason?: string }>("licence.check", { licence: m?.licence ?? {}, request: { action: "share" } })
      .catch((e: Error) => ({ allowed: false, reason: e.message, attribution: "" }));
    const place = rel.osis ? `[${rel.ref}](#passage/${encodeURIComponent(rel.osis)})` : rel.ref;
    if (!d.allowed || !rel.text.trim()) {
      // The licence does not allow copying: the card's place goes in as a chip instead.
      if (!d.allowed) say(`${rel.name}'s licence does not allow copying its words, so a link to the passage went in instead.`);
      return rel.osis ? place : null;
    }
    const credit = [`— ${rel.name} on ${place}`, d.attribution].filter(Boolean).join(". ");
    return [...rel.text.trim().split(/\n+/).map((l) => `> ${l}`), `> ${credit}`].join("\n");
  }
  return null;
}

// editorInserts is what a writing page hands its editor for the Insert control.
export function editorInserts(o: { say(why: string): void; passage(): string; self?(): string | undefined; quote(anchor: HTMLElement): void }):
  Pick<EditorOptions, "onHighlight" | "onNoteLink" | "parseRef" | "onDrop" | "onOpenLink" | "onVerseQuote"> {
  return {
    onVerseQuote: o.quote,
    onHighlight: (anchor, insert) => highlightPicker(anchor, insert),
    onNoteLink: (anchor, insert) => noteLinkPicker(anchor, insert, o.self),
    parseRef,
    onDrop: (data) => dropToMarkdown(data, o.say),
    onOpenLink: (href, label) => void openAppLink(href, label),
  };
}
