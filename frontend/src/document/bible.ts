// A Bible passage, drawn for one pane: one Bible or several side by side, with verse selection,
// highlights, notes and lists. The pane's place box is the passage box with previous and next
// chapter. Selecting text, or verse numbers, shows the marking toolbar (marking.ts); a word is
// looked up on Alt+click (lookup.ts). Words look like plain text unless Settings → Reading →
// "Show tagged words" is on.

import { Highlight, Message, Module, Ref, run } from "../api";
import { onBibles } from "../bibles";
import { bibleBooks as bibleTypedBooks, bookRows, openAt } from "../biblebooks";
import { chip } from "../controls";
import { button, clear, h } from "../dom";
import {
  Book, Column, Verse, alignColumns, markSegments, neighbourChapters, selectionRuns, verseParts, verseRangeText,
} from "../helpers";
import { icon } from "../icons";
import { addToList } from "../listpicker";
import { renderMarkup } from "../markup";
import { chapterOsis, chapterToggle } from "../chapterwords";
import { closePopover, togglePopover } from "../popover";
import { openExport } from "../printexport";
import { lookUpCard } from "../lookup";
import { attachMarking } from "../marking";
import { hlClasses, replaced, styleOf } from "../markingwords";
import { prefs, setModules, state } from "../state";
import { searchFor } from "../views/search";
import { sidebar } from "../views/sidebar";
import { attachContextMenu, selectedIn } from "./contextmenu";
import { chapterGrid, contentsPopover } from "./contentspop";
import { DocHost, DocView, bibleBooks, bibleModules } from "./view";
import { NodeLike, QuotePart, VerseRange, cpLength, nodeAt, paintFor, partsOf, pointOf, tidyRange, verseTextOf } from "../verserange";

interface Hit { id: number; module: string; ref: string; text: string; snippet?: string }

const FIND_LIMIT = 30;

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
  const picker = h("div", { class: "module-picker", role: "group", "aria-label": "Bibles to show" });
  const bookBox = h("div");
  const biblesBtn = h("button", { type: "button", class: "bibles-btn", "aria-haspopup": "dialog", "aria-expanded": "false", title: "Choose the Bibles to show" }, "Bibles");
  const openBibles = (anchor: HTMLElement) => togglePopover(anchor, () => h("div", { class: "pop-body" },
    h("h2", { class: "pop-title" }, "Bibles to show"),
    h("p", { class: "hint" }, "Choose two or more to read them side by side."),
    picker, bookBox), { label: "Bibles to show", side: "below" });
  const loadBooks = async () => {
    const bs = await bibleTypedBooks();
    clear(bookBox, bs.length ? bookRows(bs, (b) => { closePopover(); void openAt(b, host.ref(), msg); }) : null);
  };
  biblesBtn.addEventListener("click", () => { void loadBooks(); openBibles(biblesBtn); });
  const el = h("div", { class: "doc doc-bible" }, msg.el, text);

  const words = { onRef: (osis: string) => void go(osis) };
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
  // The whole chapter is shown instead of the verses asked for (this pane only; reset on a new passage).
  let expanded = false;
  let expandedFor = "";
  let chapter: { ref: string; start: number; end: number } | null = null;

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
    if (p.display !== expandedFor) expanded = false;
    const co = chapterOsis(p, books);
    chapter = null;
    if (co) {
      const c = await run<Ref>("verse.parse", { ref: co }).catch(() => null);
      if (n !== seq || !alive) return;
      if (c) chapter = { ref: c.osis, start: c.start, end: c.end };
    }
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
    const r = await msg.attempt(() => run<{ ref: string; columns: Column[] }>("bible.parallel", { modules: ids, ref: expanded && chapter ? chapter.ref : p.osis, markup: true }));
    if (!r || n !== seq || !alive) return;
    if (ids.length === 1) renderSingle(r.columns[0]);
    else renderParallel(r.columns);
    if (expanded) markChosen(p);
  }

  // chapterButton sits one line below the passage: show the whole chapter, or only the verses asked for.
  function chapterButton(): HTMLElement | null {
    const t = parsed && chapter ? chapterToggle(parsed, chapter, expanded, books) : null;
    if (!t || !parsed) return null;
    const display = parsed.display;
    return h("p", { class: "chapter-toggle-row" }, button(t.label, () => {
      expanded = t.expand;
      expandedFor = display;
      void read(host.ref());
    }, { class: "chapter-toggle" }));
  }

  // markChosen scrolls the verses asked for into view in the whole chapter and marks them briefly.
  function markChosen(p: Ref) {
    const els = [...text.querySelectorAll<HTMLElement>("[data-base]")].filter((e) => Number(e.dataset.base) >= p.start && Number(e.dataset.base) <= p.end);
    for (const e of els) e.classList.add("jumped");
    els[0]?.scrollIntoView({ block: "center" });
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

  // The marking toolbar ---------------------------------------------------------------------------

  const lookUp = (t: string, word: Element | null, at: DOMRect) => void lookUpCard(t, word, at, { showWord: side.showWord, go: (r) => void go(r) });
  const marking = attachMarking(text, {
    highlight: (s, r) => colourWords(styleOf(s), r),
    restyle: async (id, style) => {
      document.getSelection()?.removeAllRanges();
      if (await msg.attempt(() => run("highlight.update", { id, style })) !== undefined) await changed();
    },
    remove: async (id) => {
      document.getSelection()?.removeAllRanges();
      if (await msg.attempt(() => run("highlight.delete", { id })) !== undefined) await changed();
    },
    note: (_t, r) => {
      const rs = r ? selectionRuns(shown, versesIn(r)) : [];
      side.newNote(rs.length ? verseRangeText(rs[0][0], rs[0][1], books) : parsed?.display ?? "");
    },
    lookUp,
    copy: (t) => copy(t),
    list: (r) => {
      const rs = selectionRuns(shown, versesIn(r));
      if (rs.length) addToList("verses", "these verses", rs.map(([a, b]) => ({ verse_start: a, verse_end: b })));
    },
    at: (target) => {
      const v = target.closest<HTMLElement>("[data-hl]");
      if (!v || !text.contains(v) || target.closest(".vnum")) return null;
      const hl = highlights.find((x) => x.id === v.dataset.hl);
      if (!hl) return null;
      selectWhole(hl.id);
      return { id: hl.id, style: hl.style, el: v, text: hl.text || undefined };
    },
  });
  // With "Show tagged words" on, a click on a tagged word looks it up too.
  text.addEventListener("click", (e) => {
    const w = (e.target as Element).closest(".w");
    if (!w || !prefs.reading.taggedWords || !document.getSelection()?.isCollapsed) return;
    lookUp(w.textContent ?? "", w, w.getBoundingClientRect());
  });

  function verseEl(v: Verse, module: string) {
    const n = verseParts(v.id).verse;
    const end = v.end && v.end !== v.id ? `–${verseParts(v.end).verse}` : "";
    const base = v.base_id || v.id;
    const num = h("button", { type: "button", class: "vnum", "aria-label": `verse ${n}${end}`, "aria-pressed": "false",
      title: "Select this verse to highlight it, note it or add it to a list" }, `${n}${end}`);
    num.addEventListener("click", () => pick(base));
    // A verse number can be dragged into a note or sermon, where the verse is quoted.
    num.draggable = true;
    num.addEventListener("dragstart", (e) => {
      if (!e.dataTransfer) return;
      e.dataTransfer.setData("application/x-xegesis-verse", JSON.stringify({ ref: v.ref, module }));
      e.dataTransfer.setData("text/plain", v.ref);
      e.dataTransfer.effectAllowed = "copy";
    });
    return h("span", { class: "verse", "data-verse": v.id, "data-base": base }, num, " ", verseText(base, module, content(v)), " ");
  }

  function renderSingle(c: Column) {
    const last = { chapter: -1 };
    shown = c.verses.map((v) => v.base_id || v.id);
    const m = modules.find((x) => x.id === c.module);
    clear(text,
      h("header", {}, h("h2", { class: "module-name" }, m?.name || c.module)),
      c.verses.length === 0 ? h("p", { class: "empty" }, "This Bible has no text for this passage.") : null,
      c.verses.map((v) => [heading(v, last), verseEl(v, c.module)]),
      chapterButton(),
      c.attribution ? h("footer", { class: "attribution" }, c.attribution) : null);
    paint();
    paintMarks();
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
          r.cells.map((v, i) => h("td", { lang: "" }, v ? verseText(r.base, columns[i].module, content(v)) : "")))))),
    chapterButton(),
    columns.some((c) => c.attribution)
      ? h("footer", { class: "attribution" }, columns.filter((c) => c.attribution).map((c) => h("p", {}, `${c.module}: ${c.attribution}`)))
      : null);
    paint();
    paintMarks();
  }

  function verseButton(base: number) {
    const label = `${verseParts(base).chapter}:${verseParts(base).verse}`;
    const b = h("button", { type: "button", class: "vnum", "aria-pressed": "false", "aria-label": `verse ${label}`,
      title: "Select this verse to highlight it, note it or add it to a list" }, label);
    b.addEventListener("click", () => pick(base));
    return b;
  }

  // verseText holds one verse's words in one Bible: highlights on exact words count characters in it.
  function verseText(base: number, module: string, kids: Node[] | string) {
    return h("span", { class: "vt", "data-tb": base, "data-module": module }, kids);
  }

  // paint marks the picked verses; it runs after the text is drawn and on every change.
  function paint() {
    for (const e of text.querySelectorAll<HTMLElement>("[data-base]")) {
      const on = selected.has(Number(e.dataset.base));
      e.classList.toggle("picked", on);
      e.querySelector(".vnum")?.setAttribute("aria-pressed", String(on));
    }
  }

  // paintMarks draws the highlights: on a whole verse (its cell, side by side), on exact words, or
  // faintly on the whole verse for words highlighted in another Bible. It runs when the text is
  // drawn and when the highlights change, never while text is being selected.
  function paintMarks() {
    for (const m of [...text.querySelectorAll("mark[data-hl]")]) m.replaceWith(...m.childNodes);
    text.normalize();
    for (const e of text.querySelectorAll<HTMLElement>("[data-hl], .hl")) {
      e.classList.remove(...[...e.classList].filter((c) => c === "hl" || c.startsWith("hl-")));
      delete e.dataset.hl;
      if (e.classList.contains("vt")) e.removeAttribute("title");
    }
    for (const vt of text.querySelectorAll<HTMLElement>("[data-tb]")) {
      const module = vt.dataset.module ?? "";
      const p = paintFor(highlights, Number(vt.dataset.tb), cpLength(vt.textContent ?? ""), module);
      const host = vt.closest<HTMLElement>(".verse, td") ?? vt;
      if (p.whole) {
        host.classList.add(...hlClasses(p.whole.style));
        host.dataset.hl = p.whole.id;
      }
      if (p.faint) {
        vt.classList.add(...hlClasses(p.faint.style), "hl-faint");
        vt.dataset.hl = p.faint.id;
        const where = modules.find((m) => m.id === p.faint!.module)?.abbreviation || p.faint.module;
        vt.title = `Words highlighted in ${where}. In this Bible the whole verse is shown, faintly.`;
      }
      for (const s of p.segments) wrapWords(vt, s.start, s.end, hlClasses(s.hl.style), s.hl.id);
    }
  }

  // wrapWords draws characters start..end of a verse's text as a highlight.
  function wrapWords(vt: HTMLElement, start: number, end: number, cls: string[], id: string) {
    const a = nodeAt(vt as unknown as NodeLike, start);
    const b = nodeAt(vt as unknown as NodeLike, end);
    if (!a || !b) return;
    const r = document.createRange();
    r.setStart(a.node as unknown as Text, a.offset);
    r.setEnd(b.node as unknown as Text, b.offset);
    if (r.collapsed) return;
    const parts: [Text, number, number][] = [];
    const w = document.createTreeWalker(vt, NodeFilter.SHOW_TEXT);
    for (let n = w.nextNode() as Text | null; n; n = w.nextNode() as Text | null) {
      if (!r.intersectsNode(n)) continue;
      const s = n === r.startContainer ? r.startOffset : 0;
      const e = n === r.endContainer ? r.endOffset : n.length;
      if (s < e) parts.push([n, s, e]);
    }
    for (const [t, s, e] of parts) {
      let mid = t;
      if (s > 0) mid = t.splitText(s);
      if (e - s < mid.length) mid.splitText(e - s);
      const m = h("mark", { class: cls.join(" "), "data-hl": id });
      mid.replaceWith(m);
      m.append(mid);
    }
  }

  // selectWhole selects all of a highlight's words, so a click on any part of it shows it whole.
  function selectWhole(id: string) {
    const els = [...text.querySelectorAll<HTMLElement>("[data-hl]")].filter((e) => e.dataset.hl === id)
      .map((e) => (e.matches("mark, .vt") ? e : e.querySelector<HTMLElement>(".vt") ?? e));
    const sel = document.getSelection();
    if (!els.length || !sel) return;
    const r = document.createRange();
    r.setStartBefore(els[0]);
    r.setEndAfter(els[els.length - 1]);
    sel.removeAllRanges();
    sel.addRange(r);
  }

  async function loadHighlights() {
    highlights = parsed ? (await run<Highlight[]>("highlight.list", { ref: parsed.osis }).catch(() => [])) : [];
    if (alive) paintMarks();
  }

  // changed reloads the highlights after a change here, and tells the Highlights tool.
  async function changed() {
    await loadHighlights();
    document.dispatchEvent(new Event("xegesis:marks-changed"));
  }
  const onMarks = () => { if (alive) void loadHighlights(); };
  document.addEventListener("xegesis:marks-changed", onMarks);

  // colour highlights the given verse runs (the selection by default).
  async function colour(style: string, rs = selectionRuns(shown, selected)) {
    const ok = await msg.attempt(async () => {
      for (const [a, b] of rs) {
        // A highlight wholly inside the selection is replaced by the new colour.
        for (const hl of replaced(highlights, a, b)) await run("highlight.delete", { id: hl.id });
        await run("highlight.create", { verse_start: a, verse_end: b, style });
      }
      return true;
    });
    if (ok) {
      selected.clear();
      await changed();
      updateSelbar();
    }
  }

  // colourWords highlights exactly the selected words; a selection with no verse words in it (only
  // verse numbers, say) highlights the verses it touches.
  async function colourWords(style: string, r: Range) {
    const w = wordsIn(r);
    if (!w) return colour(style, selectionRuns(shown, versesIn(r)));
    const ok = await msg.attempt(() => run("highlight.create", { range: w.range, module: w.module, parts: w.parts, style }));
    if (ok === undefined) return;
    document.getSelection()?.removeAllRanges();
    await changed();
  }

  // wordsIn reads a text selection as a range of verse words in one Bible (the one it starts in).
  function wordsIn(r: Range): { range: VerseRange; module: string; parts: QuotePart[] } | null {
    const all = [...text.querySelectorAll<HTMLElement>("[data-tb]")].filter((vt) => r.intersectsNode(vt) && vt.textContent);
    if (!all.length) return null;
    const node = (n: Node) => n as unknown as NodeLike;
    const vtOf = (n: Node) => verseTextOf(node(n)) as unknown as HTMLElement | null;
    const startVt = vtOf(r.startContainer);
    const module = (startVt && all.includes(startVt) ? startVt : all[0]).dataset.module ?? "";
    const vts = all.filter((v) => (v.dataset.module ?? "") === module);
    const first = vts[0];
    const last = vts[vts.length - 1];
    const start = startVt && vts.includes(startVt) ? pointOf(node(r.startContainer), r.startOffset) : { verse: Number(first.dataset.tb), offset: 0 };
    const endVt = vtOf(r.endContainer);
    const end = endVt && vts.includes(endVt) ? pointOf(node(r.endContainer), r.endOffset)
      : { verse: Number(last.dataset.tb), offset: cpLength(last.textContent ?? "") };
    if (!start || !end) return null;
    const texts = vts.map((v) => ({ verse: Number(v.dataset.tb), text: v.textContent ?? "" }));
    const range = tidyRange({ start, end }, texts.map((t) => ({ verse: t.verse, length: cpLength(t.text) })));
    return range ? { range, module, parts: partsOf(range, texts) } : null;
  }

  // updateSelbar marks the picked verses and offers the marking toolbar over them.
  function updateSelbar() {
    paint();
    if (selected.size === 0) return marking.close();
    const rs = selectionRuns(shown, selected);
    const els = [...text.querySelectorAll<HTMLElement>("[data-base]")].filter((e) => selected.has(Number(e.dataset.base)));
    if (els.length === 0) return;
    const boxes = els.map((e) => e.getBoundingClientRect());
    const top = Math.min(...boxes.map((b) => b.top));
    const first = boxes.find((b) => b.top === top)!;
    marking.showFor(new DOMRect(first.left, top, first.width, first.height), {
      text: rs.map(([a, b]) => verseRangeText(a, b, books)).join("; "),
      highlight: (s) => colour(styleOf(s)),
      note: () => side.newNote(verseRangeText(rs[0][0], rs[0][1], books)),
      list: () => addToList("verses", "these verses", rs.map(([a, b]) => ({ verse_start: a, verse_end: b }))),
    });
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
      if (r) return colourWords(style, r);
    },
    list: () => {
      const r = selectedIn(text);
      const rs = r ? selectionRuns(shown, versesIn(r)) : [];
      if (rs.length) addToList("verses", "these verses", rs.map(([a, b]) => ({ verse_start: a, verse_end: b })));
    },
    showWord: side.showWord,
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
    // ☰ Contents: go to a book or chapter, the chapters of this book, and recent passages.
    contents(anchor: HTMLElement) {
      const at = parsed ? verseParts(parsed.start) : null;
      const here = at ? books.find((b) => b.number === at.book) : undefined;
      contentsPopover(anchor, {
        placeholder: "Go to a book or chapter…",
        go: async (t) => {
          const p = await run<Ref>("verse.parse", { ref: t }).catch(() => null);
          if (!p) return false;
          void go(p.display);
          return true;
        },
        body: (q) => {
          const want = q.toLowerCase();
          const match = q && !/\d\s*$/.test(q) ? books.filter((b) => b.name.toLowerCase().startsWith(want) || b.osis.toLowerCase().startsWith(want)).slice(0, 8) : [];
          if (match.length) {
            return h("div", { class: "book-matches" }, match.map((b) => button(b.name, () => { closePopover(true); void go(`${b.name} 1`); }, { class: "doc-chip chip-plain" })));
          }
          return here ? chapterGrid(here.name, here.chapters, at!.chapter, (c) => void go(`${here.name} ${c}`)) : h("p", { class: "hint" }, "Type a book of the Bible.");
        },
        recent: history.map((r) => ({ label: r, go: () => void go(r) })),
      });
    },
    link: () => (parsed ? `#passage/${parsed.osis}` : null),
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
    markScope: () => {
      const b = parsed ? books.find((x) => x.number === verseParts(parsed!.start).book) : undefined;
      return b ? { kind: "bible", book: b.name } : null;
    },
    focus: () => input.select(),
    destroy() {
      alive = false;
      seq++;
      stopBibles();
      marking.close();
      document.removeEventListener("xegesis:marks-changed", onMarks);
    },
  };
}
