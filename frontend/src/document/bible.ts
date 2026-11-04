// A Bible passage, drawn for one pane: one Bible or several side by side, with verse selection,
// highlights, notes and lists. The pane's place box is the passage box with previous and next
// chapter. Words tagged with Strong's numbers open in the sidebar's Word tool.

import { Highlight, Message, Module, Ref, run } from "../api";
import { onBibles } from "../bibles";
import { chip } from "../controls";
import { button, clear, h } from "../dom";
import {
  Book, Column, Verse, alignColumns, highlightFor, markSegments, neighbourChapters, selectionRuns, verseParts, verseRangeText,
} from "../helpers";
import { icon } from "../icons";
import { addToList } from "../listpicker";
import { renderMarkup } from "../markup";
import { closePopover, togglePopover } from "../popover";
import { openExport } from "../printexport";
import { setModules, state } from "../state";
import { searchFor } from "../views/search";
import { sidebar } from "../views/sidebar";
import { attachContextMenu, selectedIn } from "./contextmenu";
import { DocHost, DocView, bibleBooks, bibleModules } from "./view";

interface Hit { id: number; module: string; ref: string; text: string; snippet?: string }

const FIND_LIMIT = 30;
const SWATCHES: [string, string][] = [["yellow", "yellow"], ["green", "green"], ["blue", "blue"], ["pink", "pink"], ["purple", "purple"], ["underline", "with an underline"]];

function iconButton(name: string, label: string, onclick: (e: Event) => void, attrs: Record<string, unknown> = {}): HTMLButtonElement {
  return h("button", { type: "button", class: "icon-btn", "aria-label": label, title: label, ...attrs, onclick }, icon(name));
}

export function bibleDoc(host: DocHost): DocView {
  const msg = new Message();
  const side = sidebar();
  const input = h("input", { type: "search", value: host.ref(), autocomplete: "off", spellcheck: "false", class: "place-input",
    placeholder: "e.g. John 3:16 or Rom 8", "aria-label": "Passage", "aria-keyshortcuts": "Control+L" });
  const prev = iconButton("prev", "Previous chapter", () => step("prev"), { disabled: true, title: "Previous chapter (Alt+←)" });
  const next = iconButton("next", "Next chapter", () => step("next"), { disabled: true, title: "Next chapter (Alt+→)" });
  const placeBox = h("form", { class: "passage-box" }, prev, input, next);
  placeBox.addEventListener("submit", (e) => {
    e.preventDefault();
    void go(input.value);
  });
  const text = h("article", { class: "bible-text doc-text", tabindex: "-1", "aria-label": "Bible text" });
  const selbar = h("div", { class: "selbar", role: "toolbar", "aria-label": "Selected verses", hidden: true });
  const picker = h("div", { class: "module-picker", role: "group", "aria-label": "Bibles to show" });
  const biblesBtn = h("button", { type: "button", class: "bibles-btn", "aria-haspopup": "dialog", "aria-expanded": "false", title: "Choose the Bibles to show" }, "Bibles");
  const openBibles = (anchor: HTMLElement) => togglePopover(anchor, () => h("div", { class: "pop-body" },
    h("h2", { class: "pop-title" }, "Bibles to show"),
    h("p", { class: "hint" }, "Choose two or more to read them side by side."),
    picker), { label: "Bibles to show", side: "below" });
  biblesBtn.addEventListener("click", () => openBibles(biblesBtn));
  const el = h("div", { class: "doc doc-bible" }, msg.el, selbar, text);

  const words = { onWord: side.showWord, onRef: (osis: string) => void go(osis) };
  let modules: Module[] = [];
  let books: Book[] = [];
  let parsed: Ref | null = null;
  let neighbours: { prev: string | null; next: string | null } = { prev: null, next: null };
  // Verses are selected and highlighted by their English-numbered ID (base_id), so a highlight shows
  // in every Bible that has those verses, whatever its own numbering.
  const selected = new Set<number>();
  let shown: number[] = [];
  let highlights: Highlight[] = [];
  const history: string[] = [];
  let seq = 0;
  let wanted: string | null = null;
  let alive = true;

  function chosen(): string[] {
    const want = host.tab().modules?.length ? host.tab().modules! : state.modules;
    const ids = want.filter((m) => modules.some((x) => x.id === m));
    return ids.length || !modules.length ? ids : [modules[0].id];
  }

  async function go(ref: string) {
    ref = ref.trim();
    if (!ref) return;
    const p = await msg.attempt(() => run<Ref>("verse.parse", { ref }));
    if (!p || !alive) return;
    if (parsed && parsed.display !== p.display) history.unshift(parsed.display);
    history.splice(20);
    host.go(p.display);
  }

  async function read(ref: string) {
    const n = ++seq;
    wanted = ref;
    input.value = ref;
    if (!ref) return void clear(text, h("p", { class: "empty" }, "Type a passage in the box above."));
    const p = await msg.attempt(() => run<Ref>("verse.parse", { ref }));
    if (!p || n !== seq || !alive) return;
    parsed = p;
    input.value = p.display;
    neighbours = books.length ? neighbourChapters(p.start, books) : { prev: null, next: null };
    prev.disabled = !neighbours.prev;
    next.disabled = !neighbours.next;
    const ids = chosen();
    selected.clear();
    updateSelbar();
    const names = ids.map((id) => modules.find((m) => m.id === id)?.abbreviation || id);
    biblesBtn.textContent = names.length ? names.join(", ") : "Bibles";
    const title = names.length ? `${names.join(", ")} · ${p.display}` : p.display;
    if (host.tab().title !== title) host.update({ title });
    if (ids.length === 0) {
      shown = [];
      clear(text, h("p", { class: "empty" }, modules.length
        ? "Choose a Bible to read this passage, from the Bibles button above."
        : "No Bibles are installed yet. Get one in Library → Get Bibles."));
      return;
    }
    await loadHighlights();
    const r = await msg.attempt(() => run<{ ref: string; columns: Column[] }>("bible.parallel", { modules: ids, ref: p.osis, markup: true }));
    if (!r || n !== seq || !alive) return;
    if (ids.length === 1) renderSingle(r.columns[0]);
    else renderParallel(r.columns);
  }

  function heading(v: Verse, last: { chapter: number }) {
    const { book, chapter } = verseParts(v.id);
    if (chapter === last.chapter) return null;
    last.chapter = chapter;
    const name = books.find((b) => b.number === book)?.name;
    return h("h3", { class: "chapter" }, name ? `${name} ${chapter}` : `Chapter ${chapter}`);
  }

  function content(v: Verse): Node[] | string {
    return v.markup ? renderMarkup(v.markup, words) : v.text;
  }

  function pick(base: number) {
    if (selected.has(base)) selected.delete(base);
    else selected.add(base);
    updateSelbar();
  }

  function verseEl(v: Verse) {
    const n = verseParts(v.id).verse;
    const end = v.end && v.end !== v.id ? `–${verseParts(v.end).verse}` : "";
    const base = v.base_id || v.id;
    const num = h("button", { type: "button", class: "vnum", "aria-label": `verse ${n}${end}`, "aria-pressed": "false",
      title: "Select this verse to highlight it, note it or add it to a list" }, `${n}${end}`);
    num.addEventListener("click", () => pick(base));
    return h("span", { class: "verse", "data-verse": v.id, "data-base": base }, num, " ", content(v), " ");
  }

  function renderSingle(c: Column) {
    const last = { chapter: -1 };
    shown = c.verses.map((v) => v.base_id || v.id);
    const m = modules.find((x) => x.id === c.module);
    clear(text,
      h("header", {}, h("h2", { class: "module-name" }, m?.name || c.module)),
      c.verses.length === 0 ? h("p", { class: "empty" }, "This Bible has no text for this passage.") : null,
      c.verses.map((v) => [heading(v, last), verseEl(v)]),
      c.attribution ? h("footer", { class: "attribution" }, c.attribution) : null);
    paint();
  }

  function renderParallel(columns: Column[]) {
    const rows = alignColumns(columns);
    shown = rows.map((r) => r.base);
    const names = columns.map((c) => modules.find((x) => x.id === c.module)?.abbreviation || c.module);
    clear(text, h("table", { class: "parallel" },
      h("caption", { class: "sr-only" }, `Parallel reading of ${parsed?.display ?? ""}`),
      h("thead", {}, h("tr", {}, h("th", { scope: "col" }, h("span", { class: "sr-only" }, "Verse")), names.map((n) => h("th", { scope: "col" }, n)))),
      h("tbody", {}, rows.map((r) =>
        h("tr", { "data-base": r.base },
          h("th", { scope: "row" }, verseButton(r.base)),
          r.cells.map((v) => h("td", { lang: "" }, v ? content(v) : "")))))),
    columns.some((c) => c.attribution)
      ? h("footer", { class: "attribution" }, columns.filter((c) => c.attribution).map((c) => h("p", {}, `${c.module}: ${c.attribution}`)))
      : null);
    paint();
  }

  function verseButton(base: number) {
    const label = `${verseParts(base).chapter}:${verseParts(base).verse}`;
    const b = h("button", { type: "button", class: "vnum", "aria-pressed": "false", "aria-label": `verse ${label}`,
      title: "Select this verse to highlight it, note it or add it to a list" }, label);
    b.addEventListener("click", () => pick(base));
    return b;
  }

  // paint marks selected and highlighted verses; it runs after the text is drawn and on every change.
  function paint() {
    for (const e of text.querySelectorAll<HTMLElement>("[data-base]")) {
      const base = Number(e.dataset.base);
      e.className = e.className.replace(/hl(-\w+)?|picked/g, "").trim();
      const hl = highlightFor(highlights, base);
      if (hl) e.classList.add("hl", `hl-${hl.style}`);
      const on = selected.has(base);
      if (on) e.classList.add("picked");
      e.querySelector(".vnum")?.setAttribute("aria-pressed", String(on));
    }
  }

  async function loadHighlights() {
    highlights = parsed ? (await run<Highlight[]>("highlight.list", { ref: parsed.osis }).catch(() => [])) : [];
    if (alive) paint();
  }

  // colour highlights the given verse runs (the selection by default).
  async function colour(style: string, rs = selectionRuns(shown, selected)) {
    const ok = await msg.attempt(async () => {
      for (const [a, b] of rs) {
        // A highlight wholly inside the selection is replaced by the new colour.
        for (const hl of highlights) {
          if (hl.verse_start !== undefined && hl.verse_end !== undefined && hl.verse_start >= a && hl.verse_end <= b) {
            await run("highlight.delete", { id: hl.id });
          }
        }
        await run("highlight.create", { verse_start: a, verse_end: b, style });
      }
      return true;
    });
    if (ok) {
      selected.clear();
      await loadHighlights();
      updateSelbar();
    }
  }

  async function unhighlight(verses: number[] = [...selected]) {
    const ok = await msg.attempt(async () => {
      for (const hl of highlights) {
        if (verses.some((v) => hl.verse_start !== undefined && hl.verse_end !== undefined && hl.verse_start <= v && v <= hl.verse_end)) {
          await run("highlight.delete", { id: hl.id });
        }
      }
      return true;
    });
    if (ok) {
      selected.clear();
      await loadHighlights();
      updateSelbar();
    }
  }

  function updateSelbar() {
    paint();
    selbar.hidden = selected.size === 0;
    if (selected.size === 0) return clear(selbar);
    const rs = selectionRuns(shown, selected);
    const label = rs.map(([a, b]) => verseRangeText(a, b, books)).join("; ");
    const swatch = (style: string, name: string) =>
      h("button", { type: "button", class: `swatch-btn hl-${style}`, title: `Highlight ${name}`, "aria-label": `Highlight ${name}`, onclick: () => colour(style) }, style === "underline" ? "U" : "");
    clear(selbar,
      h("span", { class: "sel-label" }, `Selected: ${label}`),
      h("span", { class: "row", role: "group", "aria-label": "Highlight colour" }, SWATCHES.map(([s, n]) => swatch(s, n))),
      button("Remove highlight", () => unhighlight()),
      button("Note", () => side.newNote(verseRangeText(rs[0][0], rs[0][1], books))),
      button("Add to list", () => addToList("verses", "these verses", rs.map(([a, b]) => ({ verse_start: a, verse_end: b })))),
      button("Clear selection", () => { selected.clear(); updateSelbar(); }));
  }

  function renderPicker() {
    const on = new Set(chosen());
    clear(picker, modules.length === 0 ? h("span", { class: "hint" }, "No Bibles installed.") : modules.map((m) => {
      return chip({ label: m.abbreviation || m.id, title: m.name, value: m.id, checked: on.has(m.id), onChange: () => {
        const ids = [...picker.querySelectorAll<HTMLInputElement>("input:checked")].map((b) => b.value);
        // The choice is remembered for this tab, and as the default for new Bible tabs.
        setModules(ids);
        host.update({ modules: ids });
        void read(host.ref());
      } });
    }));
  }

  function step(dir: "prev" | "next") {
    const ref = neighbours[dir];
    if (ref) void go(ref);
  }

  el.addEventListener("keydown", (e) => {
    if (e.altKey && (e.key === "ArrowLeft" || e.key === "ArrowRight")) {
      e.preventDefault();
      step(e.key === "ArrowLeft" ? "prev" : "next");
    }
  });

  // Selected text: the right-click menu, and copying within each Bible's licence ----------------

  // versesIn finds the verses a text selection touches.
  function versesIn(r: Range): Set<number> {
    const out = new Set<number>();
    for (const e of text.querySelectorAll<HTMLElement>("[data-base]")) if (r.intersectsNode(e)) out.add(Number(e.dataset.base));
    const at = (r.startContainer.parentElement?.closest("[data-base]") as HTMLElement | null);
    if (at) out.add(Number(at.dataset.base));
    return out;
  }

  async function copy(t: string) {
    const ids = chosen();
    const decisions = await msg.attempt(() => Promise.all(ids.map((id) => {
      const m = modules.find((x) => x.id === id);
      return run<{ allowed: boolean; attribution?: string; reason?: string }>("licence.check", { licence: m?.licence ?? {}, request: { action: "share" } });
    })));
    if (!decisions) return;
    const refused = decisions.find((d) => !d.allowed);
    if (refused) return msg.info(`This Bible's licence does not allow copying${refused.reason ? `: ${refused.reason}` : "."}`);
    const credit = [...new Set(decisions.map((d) => d.attribution).filter(Boolean))].join("\n");
    const out = credit ? `${t}\n\n${credit}` : t;
    try {
      await navigator.clipboard.writeText(out);
      msg.ok(credit ? "Copied, with the Bible's credit line." : "Copied.");
    } catch {
      msg.info("The clipboard is not available here.");
    }
  }

  text.addEventListener("copy", (e) => {
    const r = selectedIn(text);
    if (!r) return;
    e.preventDefault(); // nothing reaches the clipboard unless the licence allows it
    void copy(r.toString());
  });

  attachContextMenu(text, {
    highlight: (style) => {
      const r = selectedIn(text);
      if (r) return colour(style, selectionRuns(shown, versesIn(r)));
    },
    note: () => {
      const r = selectedIn(text);
      const rs = r ? selectionRuns(shown, versesIn(r)) : [];
      side.newNote(rs.length ? verseRangeText(rs[0][0], rs[0][1], books) : parsed?.display ?? "");
    },
    copy: (t) => copy(t),
    go: (ref) => void go(ref),
  });

  // The ⋯ menu ---------------------------------------------------------------------------------------

  async function find(q: string, anchor: HTMLElement) {
    if (!q) return;
    const ids = chosen();
    const hits = await msg.attempt(() => run<Hit[]>("bible.search", { query: q, mode: "all", limit: FIND_LIMIT, ...(ids.length ? { modules: ids } : {}) }));
    if (!hits) return;
    const names = ids.map((id) => modules.find((m) => m.id === id)?.abbreviation || id).join(", ") || "your Bibles";
    closePopover();
    togglePopover(anchor, () => h("div", { class: "pop-body find-results" },
      h("p", { class: "hint", role: "status" }, hits.length === 0 ? `No matches in ${names}.`
        : `${hits.length === FIND_LIMIT ? `The first ${FIND_LIMIT} matches` : `${hits.length} ${hits.length === 1 ? "match" : "matches"}`} in ${names}`),
      h("ol", { class: "results" }, hits.map((hit) => h("li", {},
        button(hit.ref, () => { closePopover(); void go(hit.ref); }, { class: "link" }),
        h("p", {}, markSegments(hit.snippet || hit.text).map((s) => (s.mark ? h("mark", {}, s.text) : s.text)))))),
      button("Search with more options…", () => { closePopover(); searchFor(q); }, { class: "link" })),
    { label: "Matches", side: "below" });
  }

  const menu = {
    find,
    bibles: openBibles,
    search(anchor: HTMLElement) {
      const q = h("input", { type: "search", "aria-label": "Search in these Bibles", placeholder: "Words to find", autocomplete: "off" });
      const form = h("form", { class: "pop-body", role: "search" }, h("h2", { class: "pop-title" }, "Search in this"), q);
      form.addEventListener("submit", (e) => { e.preventDefault(); void find(q.value.trim(), anchor); });
      togglePopover(anchor, () => form, { label: "Search in this", side: "below" });
    },
    contents(anchor: HTMLElement) {
      togglePopover(anchor, () => h("div", { class: "pop-body bible-contents" },
        h("h2", { class: "pop-title" }, "Books"),
        h("div", { class: "row wrap" }, books.map((b) => button(b.name, () => { closePopover(); void go(`${b.name} 1`); }, { class: "chip" })))),
      { label: "Books of the Bible", side: "below" });
    },
    history(anchor: HTMLElement) {
      togglePopover(anchor, () => h("div", { class: "menu-list" }, history.length === 0
        ? h("p", { class: "hint pop-body" }, "No other passages yet.")
        : history.map((r) => button(r, () => { void go(r); }))), { label: "History", side: "below", menu: true });
    },
    print() {
      const ids = chosen();
      if (!parsed || ids.length === 0) return msg.info("Choose a passage and at least one Bible to print.");
      openExport({ title: parsed.display, sources: [{ kind: "passage", ref: parsed.osis, modules: ids }] });
    },
  };

  // A Bible installed or removed while this pane is open shows at once.
  const stopBibles = onBibles((list) => {
    if (!alive) return;
    modules = list;
    renderPicker();
    void read(host.ref());
  });

  void (async () => {
    [modules, books] = await Promise.all([bibleModules(), bibleBooks()]);
    if (!alive) return;
    renderPicker();
    await read(host.ref());
  })();

  return {
    el,
    placeBox,
    tools: biblesBtn,
    menu,
    refChanged(ref) {
      if (ref !== wanted) void read(ref);
    },
    focus: () => input.select(),
    destroy() {
      alive = false;
      seq++;
      stopBibles();
    },
  };
}
