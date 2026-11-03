// Reader: a slim top bar (Read | Study, search in what is open, the passage box, the Bibles shown,
// reading settings and a ⋯ menu), today's reading as one line, then the Bible text, with a library
// book beside it in Study mode (views/book.ts). Study tools live in the right sidebar (sidebar.ts).

import { openExport } from "../printexport";
import { Highlight, Message, Module, Ref, run } from "../api";
import { button, clear, h } from "../dom";
import { openGridPresets } from "../gridhook";
import {
  Book, Column, Verse, alignColumns, highlightFor, markSegments, neighbourChapters, selectionRuns, verseParts, verseRangeText,
} from "../helpers";
import { icon } from "../icons";
import { addToList } from "../listpicker";
import { todayCard } from "../plancard";
import { closePopover, togglePopover } from "../popover";
import { ReaderMode, ReadingPrefs, TEXT_SIZES, gridButtonPlace, stepSize } from "../shellwords";
import { lastRef, navigate, onPassage, onPrefs, prefs, setModules, setPassage, setPrefs, state } from "../state";
import { bookPane } from "./book";
import { openReport } from "./report";
import { searchFor } from "./search";
import { sidebar } from "./sidebar";
import { renderMarkup } from "../markup";

interface Hit { id: number; module: string; ref: string; text: string; snippet?: string }

const FIND_LIMIT = 30;

function iconButton(name: string, label: string, onclick: (e: Event) => void, attrs: Record<string, unknown> = {}): HTMLButtonElement {
  return h("button", { type: "button", class: "icon-btn", "aria-label": label, title: label, ...attrs, onclick }, icon(name));
}

export function readerView() {
  const msg = new Message();
  const side = sidebar();
  const input = h("input", { type: "search", id: "passage", value: lastRef(), autocomplete: "off", spellcheck: "false",
    placeholder: "e.g. John 3:16 or Rom 8", "aria-label": "Passage", "aria-keyshortcuts": "Control+L", "aria-describedby": "passage-parsed" });
  const parsed = h("span", { id: "passage-parsed", class: "sr-only", "aria-live": "polite" });
  const prev = iconButton("prev", "Previous chapter", () => step("prev"), { disabled: true, "aria-keyshortcuts": "Alt+ArrowLeft", title: "Previous chapter (Alt+←)" });
  const next = iconButton("next", "Next chapter", () => step("next"), { disabled: true, "aria-keyshortcuts": "Alt+ArrowRight", title: "Next chapter (Alt+→)" });
  const picker = h("div", { class: "module-picker", role: "group", "aria-label": "Bibles to show" });
  const text = h("article", { class: "bible-text", "aria-live": "polite", tabindex: "-1", "aria-label": "Bible text" });
  const selbar = h("div", { class: "selbar", role: "toolbar", "aria-label": "Selected verses", hidden: true });
  const today = todayCard();
  const panes = h("div", { class: "reader-panes" }, h("section", { class: "pane", "aria-label": "Text" }, selbar, text));
  const book = bookPane((open) => {
    panes.classList.toggle("with-book", open);
    // A book opens beside the Bible, which is Study mode.
    if (open && prefs.reading.mode === "read") setMode("study");
  });
  panes.append(book.el);
  const words = { onWord: side.showWord, onRef: (osis: string) => go(osis) };
  let modules: Module[] = [];
  let books: Book[] = [];
  let neighbours: { prev: string | null; next: string | null } = { prev: null, next: null };
  // Verses are selected and highlighted by their English-numbered ID (base_id), so a highlight shows
  // in every Bible that has those verses, whatever its own numbering.
  const selected = new Set<number>();
  let shown: number[] = [];
  let highlights: Highlight[] = [];

  // Read | Study ---------------------------------------------------------------------------------
  const modes: ReaderMode[] = ["read", "study"];
  const modeBtns = modes.map((m) => {
    const b = h("button", { type: "button", "aria-pressed": "false" }, m === "read" ? "Read" : "Study");
    b.addEventListener("click", () => setMode(m));
    return b;
  });
  const modeSwitch = h("div", { class: "segmented", role: "group", "aria-label": "Mode" }, modeBtns);
  function setMode(m: ReaderMode) {
    if (prefs.reading.mode !== m) setPrefs({ reading: { ...prefs.reading, mode: m } });
  }

  // Search in what is open ------------------------------------------------------------------------
  const findInput = h("input", { type: "search", "aria-label": "Search in the open Bibles", placeholder: "Search in what is open", autocomplete: "off" });
  const findForm = h("form", { class: "find", role: "search", hidden: true }, findInput);
  const findBtn = iconButton("search", "Search in what is open", () => {
    findForm.hidden = !findForm.hidden;
    findBtn.setAttribute("aria-expanded", String(!findForm.hidden));
    if (!findForm.hidden) findInput.focus();
  }, { "aria-expanded": "false" });
  findInput.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !findInput.value) {
      findForm.hidden = true;
      findBtn.setAttribute("aria-expanded", "false");
      findBtn.focus();
    }
  });
  findForm.addEventListener("submit", (e) => {
    e.preventDefault();
    void find(findInput.value.trim());
  });

  async function find(q: string) {
    if (!q) return;
    const chosen = state.modules.filter((m) => modules.some((x) => x.id === m));
    const hits = await msg.attempt(() => run<Hit[]>("bible.search", { query: q, mode: "all", limit: FIND_LIMIT, ...(chosen.length ? { modules: chosen } : {}) }));
    if (!hits) return;
    const names = chosen.map((id) => modules.find((m) => m.id === id)?.abbreviation || id).join(", ") || "your Bibles";
    closePopover();
    togglePopover(findInput, () => h("div", { class: "pop-body find-results" },
      h("p", { class: "hint", role: "status" }, hits.length === 0 ? `No matches in ${names}.`
        : `${hits.length === FIND_LIMIT ? `The first ${FIND_LIMIT} matches` : `${hits.length} ${hits.length === 1 ? "match" : "matches"}`} in ${names}`),
      h("ol", { class: "results" }, hits.map((hit) => h("li", {},
        button(hit.ref, () => { closePopover(); void go(hit.ref); }, { class: "link" }),
        h("p", {}, markSegments(hit.snippet || hit.text).map((s) => (s.mark ? h("mark", {}, s.text) : s.text)))))),
      button("Search with more options…", () => { closePopover(); searchFor(q); }, { class: "link" })),
    { label: "Matches", side: "below" });
  }

  // Bibles, reading settings, grid and the ⋯ menu --------------------------------------------------
  const biblesBtn = h("button", { type: "button", class: "bibles-btn", "aria-haspopup": "dialog", "aria-expanded": "false", title: "Choose the Bibles to show" }, "Bibles");
  biblesBtn.addEventListener("click", () => togglePopover(biblesBtn, () => h("div", { class: "pop-body" },
    h("h2", { class: "pop-title" }, "Bibles to show"),
    h("p", { class: "hint" }, "Choose two or more to read them side by side."),
    picker), { label: "Bibles to show", side: "below" }));

  const aaBtn = h("button", { type: "button", class: "aa-btn", "aria-haspopup": "dialog", "aria-expanded": "false", "aria-label": "Reading settings", title: "Reading settings" }, "Aa");
  aaBtn.addEventListener("click", () => togglePopover(aaBtn, () => readingControls(), { label: "Reading settings", side: "below" }));

  const gridBtn: HTMLButtonElement = iconButton("grid", "Grid presets", () => openGridPresets(gridBtn), { "aria-haspopup": "dialog", "aria-expanded": "false" });

  const moreBtn: HTMLButtonElement = iconButton("more", "More", () => togglePopover(moreBtn, () => h("div", { class: "menu-list" },
    button("Passage report", () => state.parsed && openReport("passage", state.parsed.osis),
      { title: "Everything your library holds about this passage, in one place" }),
    button("Notes on this passage", () => side.open("notes")),
    button("Print or save…", () => printPassage(), { title: "Print, or save as PDF, Word, a web page or Markdown" }),
    button("Open a book beside the Bible", () => book.choose()),
    button("Reading plans", () => navigate("plans"))), { label: "More", side: "below", menu: true }),
  { "aria-haspopup": "menu", "aria-expanded": "false" });

  const sideBtn = iconButton("panel", "Right sidebar", () => side.toggle(), { "aria-controls": "rightbar", title: "Right sidebar (Ctrl+.)", "aria-keyshortcuts": "Control+." });

  function printPassage() {
    const chosen = state.modules.filter((m) => modules.some((x) => x.id === m));
    if (!state.parsed || chosen.length === 0) {
      msg.info("Choose a passage and at least one Bible to print.");
      return;
    }
    openExport({ title: state.parsed.display, sources: [{ kind: "passage", ref: state.parsed.osis, modules: chosen }] });
  }

  const form = h("form", { class: "passage-box" }, prev, input, next, parsed);
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    go(input.value);
  });

  const bar = h("div", { class: "topbar", role: "group", "aria-label": "Reader" },
    modeSwitch, findBtn, findForm, form, biblesBtn, aaBtn, gridBtn, h("span", { class: "spacer" }), moreBtn, sideBtn);

  const el = h("div", { class: "view reader" },
    h("h1", { class: "sr-only" }, "Reader"),
    bar, today.el, msg.el, panes);

  // applyPrefs shows the mode, the reading settings and where the grid button goes.
  function applyPrefs() {
    const r = prefs.reading;
    el.dataset.mode = r.mode;
    modeBtns.forEach((b, i) => b.setAttribute("aria-pressed", String(modes[i] === r.mode)));
    aaBtn.hidden = r.mode !== "read";
    gridBtn.hidden = gridButtonPlace(r.mode, prefs.rail) !== "toolbar";
    text.style.setProperty("--text-size", `${r.size / 100}`);
    text.dataset.font = r.font;
    text.dataset.width = r.width;
    sideBtn.setAttribute("aria-pressed", String(prefs.sidebar.open));
  }
  onPrefs(applyPrefs);
  applyPrefs();

  async function go(ref: string) {
    ref = ref.trim();
    if (!ref) return;
    const p = await msg.attempt(() => run<Ref>("verse.parse", { ref }));
    if (!p) {
      parsed.textContent = "";
      return;
    }
    input.value = p.display;
    parsed.textContent = p.display === ref ? "" : `Reading ${p.display}`;
    setPassage(p.display, p);
  }

  async function read() {
    const p = state.parsed;
    if (!p) return;
    neighbours = books.length ? neighbourChapters(p.start, books) : { prev: null, next: null };
    prev.disabled = !neighbours.prev;
    next.disabled = !neighbours.next;
    const chosen = state.modules.filter((m) => modules.some((x) => x.id === m));
    selected.clear();
    updateSelbar();
    if (chosen.length === 0) {
      shown = [];
      clear(text, h("p", { class: "empty" }, modules.length
        ? "Choose a Bible to read this passage, from the Bibles button in the bar above."
        : "No Bibles are installed yet. Get one in Library → Get Bibles."));
      return;
    }
    await loadHighlights();
    const r = await msg.attempt(() => run<{ ref: string; columns: Column[] }>("bible.parallel", { modules: chosen, ref: p.osis, markup: true }));
    if (!r) return;
    if (chosen.length === 1) renderSingle(r.columns[0]);
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
    paint();
    updateSelbar();
  }

  function verseEl(v: Verse) {
    const n = verseParts(v.id).verse;
    const end = v.end && v.end !== v.id ? `–${verseParts(v.end).verse}` : "";
    const base = v.base_id || v.id;
    const num = h("button", { type: "button", class: "vnum", "aria-label": `verse ${n}${end}`, "aria-pressed": "false",
      title: "Select this verse to highlight it, note it or add it to a list" }, `${n}${end}`);
    num.addEventListener("click", () => pick(base));
    return h("span", { class: "verse", id: `v${v.id}`, "data-base": base }, num, " ", content(v), " ");
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
      h("caption", { class: "sr-only" }, `Parallel reading of ${state.parsed?.display ?? ""}`),
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
      e.className = e.className.replace(/hl(-w+)?|picked/g, "").trim();
      const hl = highlightFor(highlights, base);
      if (hl) e.classList.add("hl", `hl-${hl.style}`);
      const on = selected.has(base);
      if (on) e.classList.add("picked");
      e.querySelector(".vnum")?.setAttribute("aria-pressed", String(on));
    }
  }

  async function loadHighlights() {
    const p = state.parsed;
    highlights = p ? (await run<Highlight[]>("highlight.list", { ref: p.osis }).catch(() => [])) : [];
    paint();
  }

  function runs() {
    return selectionRuns(shown, selected);
  }

  async function colour(style: string) {
    const rs = runs();
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

  async function unhighlight() {
    const ok = await msg.attempt(async () => {
      for (const hl of highlights) {
        if ([...selected].some((v) => hl.verse_start !== undefined && hl.verse_end !== undefined && hl.verse_start <= v && v <= hl.verse_end)) {
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
    const rs = runs();
    const label = rs.map(([a, b]) => verseRangeText(a, b, books)).join("; ");
    const swatch = (style: string, name: string) =>
      h("button", { type: "button", class: `swatch-btn hl-${style}`, title: `Highlight ${name}`, "aria-label": `Highlight ${name}`, onclick: () => colour(style) }, style === "underline" ? "U" : "");
    clear(selbar,
      h("span", { class: "sel-label" }, `Selected: ${label}`),
      h("span", { class: "row", role: "group", "aria-label": "Highlight colour" },
        swatch("yellow", "yellow"), swatch("green", "green"), swatch("blue", "blue"), swatch("pink", "pink"), swatch("purple", "purple"), swatch("underline", "with an underline")),
      button("Remove highlight", () => unhighlight()),
      button("Note", () => side.newNote(verseRangeText(rs[0][0], rs[0][1], books))),
      button("Add to list", () => addToList("verses", "these verses", rs.map(([a, b]) => ({ verse_start: a, verse_end: b })))),
      button("Clear selection", () => { selected.clear(); updateSelbar(); }));
  }

  function renderPicker() {
    const chosen = state.modules.filter((id) => modules.some((m) => m.id === id))
      .map((id) => modules.find((m) => m.id === id)?.abbreviation || id);
    biblesBtn.textContent = chosen.length ? chosen.join(", ") : "Bibles";
    clear(picker, modules.length === 0 ? h("span", { class: "hint" }, "No Bibles installed.") : modules.map((m) => {
      const box = h("input", { type: "checkbox", value: m.id, checked: state.modules.includes(m.id) });
      box.addEventListener("change", () => {
        const ids = [...picker.querySelectorAll<HTMLInputElement>("input:checked")].map((b) => b.value);
        setModules(ids);
        renderPicker();
        read();
      });
      return h("label", { class: "check chip", title: m.name }, box, " ", m.abbreviation || m.id);
    }));
  }

  function step(dir: "prev" | "next") {
    const ref = neighbours[dir];
    if (ref) go(ref);
  }

  el.addEventListener("keydown", (e) => {
    if (e.altKey && (e.key === "ArrowLeft" || e.key === "ArrowRight")) {
      e.preventDefault();
      step(e.key === "ArrowLeft" ? "prev" : "next");
    }
  });

  onPassage(() => {
    input.value = state.ref;
    read();
  });

  async function show() {
    void today.refresh();
    const [ms, bs] = await Promise.all([
      msg.attempt(() => run<Module[]>("bible.modules")),
      books.length ? Promise.resolve(books) : msg.attempt(() => run<Book[]>("verse.books")),
    ]);
    modules = ms ?? [];
    books = bs ?? [];
    // Forget remembered Bibles that are no longer installed; with none left, use the first one.
    const kept = state.modules.filter((m) => modules.some((x) => x.id === m));
    if (kept.length !== state.modules.length || (kept.length === 0 && modules.length)) {
      setModules(kept.length ? kept : modules.length ? [modules[0].id] : []);
    }
    renderPicker();
    if (!state.parsed) await go(input.value);
    else read();
  }

  return { el, show, focus: () => input.select() };
}

// readingControls are the Read mode settings, in the Aa popover and in Settings → Appearance.
let radios = 0;
export function readingControls(): HTMLElement {
  const size = h("output", { "aria-live": "polite" }, `${prefs.reading.size}%`);
  const set = (patch: Partial<ReadingPrefs>) => {
    setPrefs({ reading: { ...prefs.reading, ...patch } });
    size.textContent = `${prefs.reading.size}%`;
  };
  const group = ++radios;
  const choice = (key: "font" | "width", label: string, options: [string, string][]) =>
    h("fieldset", {}, h("legend", {}, label), options.map(([v, l]) => {
      const b = h("input", { type: "radio", name: `reading-${key}-${group}`, checked: prefs.reading[key] === v });
      b.addEventListener("change", () => set({ [key]: v } as Partial<ReadingPrefs>));
      return h("label", { class: "check" }, b, " ", l);
    }));
  return h("div", { class: "pop-body reading-controls" },
    h("h2", { class: "pop-title" }, "Reading"),
    h("div", { class: "row", role: "group", "aria-label": "Text size" },
      h("span", {}, "Text size"),
      button("A−", () => set({ size: stepSize(prefs.reading.size, -1) }), { "aria-label": "Smaller text", title: `Smaller (down to ${TEXT_SIZES[0]}%)` }),
      size,
      button("A+", () => set({ size: stepSize(prefs.reading.size, 1) }), { "aria-label": "Larger text", title: `Larger (up to ${TEXT_SIZES[TEXT_SIZES.length - 1]}%)` })),
    choice("font", "Typeface", [["serif", "Book (serif)"], ["sans", "Plain (sans serif)"]]),
    choice("width", "Line length", [["narrow", "Comfortable"], ["wide", "Full width"]]));
}
