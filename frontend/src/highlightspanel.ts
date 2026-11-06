// Lists of highlights: the right sidebar's Highlights tool (the selected pane's book, or the Bible
// book being read) and My work → Highlights (every highlight, with a search). Each row shows the
// colour, the words and where they are; choosing it goes there, × deletes it (with Undo). Colour
// chips filter the list. Everything goes through highlight.list, highlight.delete and
// highlight.create.

import { Message, Ref, run } from "./api";
import { installedBibles } from "./bibles";
import { button, clear, h } from "./dom";
import { openInGrid } from "./document/open";
import { icon } from "./icons";
import { COLOURS, Colour, parseStyle } from "./markingwords";
import { navigate, setPassage, state } from "./state";
import type { QuotePart, VerseRange } from "./verserange";

export interface HighlightRow {
  id: string;
  style: string;
  ref?: string;
  verse_start?: number;
  verse_end?: number;
  item_id?: string;
  item_title?: string;
  locator?: { section?: number; page?: number; para?: number; text?: string } & Record<string, unknown>;
  text?: string;
  created_at: number;
  // A Bible highlight on exact words (T34f): text is then those words.
  range?: VerseRange;
  module?: string;
  parts?: QuotePart[];
}

interface BookNoteRow {
  id: string;
  title?: string;
  body?: string;
  item_id?: string;
  locator?: HighlightRow["locator"];
}

// scopeUnit names a book section or page for a caption.
let unit = "section";
const scopeUnit = (n: number) => `${unit} ${n}`;

// The place the sidebar lists: a library book, or a book of the Bible.
export type MarkScope = { kind: "item"; itemId: string; title: string; section?: number; unit?: string } | { kind: "bible"; book: string };

export interface HighlightsOptions {
  page?: boolean; // My work's page: a search, every highlight
  scope?(): MarkScope | null;
  footer?: Node;
}

export function highlightsPanel(o: HighlightsOptions) {
  const msg = new Message();
  let colour: Colour | "" = "";
  let rows: HighlightRow[] = [];
  let notes: BookNoteRow[] = [];
  let whole = false; // a book: every section, not just the one being read
  let seq = 0;
  let timer = 0;
  const caption = h("p", { class: "side-caption" });
  const scopeBtn = button("", () => { whole = !whole; void refresh(); }, { class: "link side-scope", hidden: true });
  const query = o.page ? h("input", { type: "search", class: "work-search", placeholder: "Search the highlighted words and places", "aria-label": "Search highlights" }) : null;
  const chips = h("div", { class: "hl-filters", role: "group", "aria-label": "Colour" });
  const list = h("ul", { class: "hl-list", "aria-live": "polite" });
  const el = h("div", { class: `hl-panel${o.page ? " page" : ""}` },
    o.page ? null : caption, o.page ? null : scopeBtn,
    h("div", { class: "hl-tools" }, query, chips), msg.el, list, o.footer ?? null);

  function drawChips() {
    const chip = (id: Colour | "", label: Node | string, name: string) => {
      const b = h("button", { type: "button", class: `hl-chip${colour === id ? " on" : ""}`, "aria-pressed": String(colour === id), title: name, "aria-label": name }, label);
      b.addEventListener("click", () => { colour = id; drawChips(); void refresh(); });
      return b;
    };
    clear(chips, chip("", "All", "All colours"),
      COLOURS.map((c) => chip(c.id, h("span", { class: `mark-dot dot-${c.id}`, "aria-hidden": "true" }), c.label)));
  }

  async function module(): Promise<string> {
    const mods = await installedBibles();
    return state.modules.find((m) => mods.some((x) => x.id === m)) ?? mods[0]?.id ?? "";
  }

  async function refresh() {
    const n = ++seq;
    const scope = o.scope?.() ?? null;
    if (!o.page && !scope) {
      caption.textContent = "";
      clear(list, h("li", { class: "side-empty" }, "Open a Bible or a book to see its highlights."));
      return;
    }
    unit = scope?.kind === "item" && scope.unit ? scope.unit : "section";
    const inSection = scope?.kind === "item" && scope.section !== undefined && !whole;
    caption.textContent = scope ? (scope.kind === "bible" ? `In ${scope.book}` : `In ${scope.title}${inSection ? `, ${scopeUnit(scope.section ?? 0)}` : ""}`) : "";
    scopeBtn.hidden = !(scope?.kind === "item" && scope.section !== undefined);
    scopeBtn.textContent = whole ? "Show this section only" : "Show the whole book";
    const params: Record<string, unknown> = { module: await module() };
    if (scope?.kind === "bible") params.book = scope.book;
    if (scope?.kind === "item") params.item_id = scope.itemId;
    if (inSection) params.section = scope.section;
    if (colour) params.colour = colour;
    if (query?.value.trim()) params.query = query.value.trim();
    if (o.page) params.newest = true;
    const noteParams = { item_id: scope?.kind === "item" ? scope.itemId : "", ...(inSection ? { section: scope.section } : {}), limit: 200 };
    const [got, ns] = await Promise.all([
      msg.attempt(() => run<HighlightRow[]>("highlight.list", params)),
      scope?.kind === "item" && !colour && !query?.value.trim() ? msg.attempt(() => run<BookNoteRow[]>("note.list", noteParams)) : Promise.resolve([]),
    ]);
    if (n !== seq || !got) return;
    rows = got;
    notes = ns ?? [];
    draw();
  }

  function where(r: HighlightRow): string {
    if (!r.item_id && r.ref) return r.ref;
    const loc = r.locator;
    const place = loc?.page ? `page ${loc.page}` : loc?.section ? `section ${loc.section}` : "";
    return [r.item_title || "A book", r.ref || place].filter(Boolean).join(" · ");
  }

  function drawNote(nt: BookNoteRow) {
    const words = `${nt.title || "Untitled"}${nt.body ? `: ${nt.body}` : ""}`;
    const go = h("button", { type: "button", class: "hl-row-main", title: "Go there" },
      h("span", { class: "hl-text" }, words.length > 220 ? `${words.slice(0, 220)}…` : words),
      h("span", { class: "hl-where" }, "Note", nt.locator?.section ? ` · ${scopeUnit(nt.locator.section)}` : ""));
    go.addEventListener("click", () => void jump({ id: nt.id, style: "", item_id: nt.item_id, locator: nt.locator, created_at: 0 }));
    const x = button("", async () => {
      if (await msg.attempt(() => run("note.delete", { id: nt.id })) === undefined) return;
      notes = notes.filter((y) => y.id !== nt.id);
      draw();
      msg.ok("Note deleted. It can be restored from its history.");
      document.dispatchEvent(new Event("xegesis:marks-changed"));
    }, { class: "ghost-btn hl-x", "aria-label": `Delete the note ${nt.title || "Untitled"}`, title: "Delete note" });
    x.append(icon("close"));
    return h("li", { class: "hl-row" }, h("span", { class: "hl-bar bar-note", "aria-hidden": "true" }), go, x);
  }

  function draw() {
    if (rows.length === 0 && notes.length === 0) {
      const filtered = !!colour || !!query?.value.trim();
      clear(list, h("li", { class: "side-empty" }, filtered ? "No highlights match."
        : "No highlights here yet. Select words in the text and pick a colour."));
      return;
    }
    clear(list, [...notes.map(drawNote), ...rows.map((r) => {
      const s = parseStyle(r.style);
      const words = (r.text ?? "").trim();
      const go = h("button", { type: "button", class: "hl-row-main", title: "Go there" },
        h("span", { class: `hl-text${s.underline ? " u" : ""}` }, words ? `“${words.length > 220 ? `${words.slice(0, 220)}…` : words}”` : h("span", { class: "untitled" }, "(no words kept)")),
        h("span", { class: "hl-where" }, where(r), s.underline ? " · underline" : ""));
      go.addEventListener("click", () => void jump(r));
      const x = button("", () => void remove(r), { class: "ghost-btn hl-x", "aria-label": `Delete the highlight at ${where(r)}`, title: "Delete highlight" });
      x.append(icon("close"));
      const li = h("li", { class: "hl-row", draggable: "true", title: "Drag into a note or sermon to quote it" },
        h("span", { class: `hl-bar bar-${s.colour}`, "aria-hidden": "true" }), go, x);
      li.addEventListener("dragstart", (e) => {
        if (!e.dataTransfer) return;
        e.dataTransfer.setData("application/x-xegesis-highlight", JSON.stringify(r));
        e.dataTransfer.setData("text/plain", words);
        e.dataTransfer.effectAllowed = "copy";
      });
      return li;
    })]);
  }

  async function jump(r: HighlightRow) {
    if (r.ref && !r.item_id) {
      const p = await run<Ref>("verse.parse", { ref: r.ref }).catch(() => null);
      if (!p) return;
      navigate("reader");
      setPassage(p.display, p);
      return;
    }
    if (r.item_id) {
      navigate("reader");
      openInGrid({ kind: "book", itemId: r.item_id, section: r.locator?.section ?? r.locator?.page, para: r.locator?.para });
    }
  }

  async function remove(r: HighlightRow) {
    if (await msg.attempt(() => run("highlight.delete", { id: r.id })) === undefined) return;
    rows = rows.filter((x) => x.id !== r.id);
    draw();
    clear(msg.el, "Highlight deleted. ", button("Undo", async () => {
      const again = r.range ? { range: r.range, module: r.module, ...(r.parts?.length ? { parts: r.parts } : { quote: r.text }), style: r.style }
        : r.item_id ? { item_id: r.item_id, locator: r.locator, style: r.style }
        : { verse_start: r.verse_start, verse_end: r.verse_end, style: r.style };
      if (await msg.attempt(() => run("highlight.create", again)) !== undefined) {
        msg.clear();
        void refresh();
        document.dispatchEvent(new Event("xegesis:marks-changed"));
      }
    }, { class: "link" }));
    msg.el.className = "msg ok";
    document.dispatchEvent(new Event("xegesis:marks-changed"));
  }

  query?.addEventListener("input", () => {
    clearTimeout(timer);
    timer = window.setTimeout(() => void refresh(), 220);
  });
  query?.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && query.value) { e.preventDefault(); query.value = ""; void refresh(); }
  });
  drawChips();
  return { el, refresh, focus: () => (query ?? chips.querySelector("button"))?.focus() };
}

// highlightsView is My work → Highlights.
export function highlightsView() {
  const panel = highlightsPanel({ page: true });
  const el = h("div", { class: "view work-view" },
    h("div", { class: "work-head" }, h("div", { class: "work-heading" },
      h("h1", { class: "work-h1" }, "Highlights"),
      h("p", { class: "hint work-sub" }, "Every highlight in your Bibles and books, newest first. Choose one to go there."))),
    panel.el);
  document.addEventListener("xegesis:marks-changed", () => { if (el.offsetParent !== null) void panel.refresh(); });
  return { el, show: () => panel.refresh(), focus: panel.focus };
}
