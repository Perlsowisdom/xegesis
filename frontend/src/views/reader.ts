// Reader: a passage box, the Bible text (one Bible or parallel columns), an optional library book
// (views/book.ts) and the notes on that passage side by side.

import { Message, Module, Ref, run } from "../api";
import { button, clear, h } from "../dom";
import { Book, Column, Verse, alignColumns, neighbourChapters, verseParts } from "../helpers";
import { notesPanel } from "../notes";
import { todayCard } from "../plancard";
import { lastRef, onPassage, setModules, setPassage, state } from "../state";
import { bookPane } from "./book";
import { studyPanel } from "./study";
import { renderMarkup } from "../markup";

export function readerView() {
  const msg = new Message();
  const input = h("input", { type: "search", id: "passage", value: lastRef(), autocomplete: "off", spellcheck: "false",
    placeholder: "e.g. John 3:16 or Rom 8", "aria-keyshortcuts": "Control+L", "aria-describedby": "passage-parsed" });
  const parsed = h("span", { id: "passage-parsed", class: "hint", "aria-live": "polite" });
  const prev = button("‹ Previous chapter", () => step("prev"), { disabled: true, "aria-keyshortcuts": "Alt+ArrowLeft" });
  const next = button("Next chapter ›", () => step("next"), { disabled: true, "aria-keyshortcuts": "Alt+ArrowRight" });
  const picker = h("div", { class: "module-picker", role: "group", "aria-label": "Bibles to show" });
  const text = h("article", { class: "bible-text", "aria-live": "polite", tabindex: "-1", "aria-label": "Bible text" });
  const showNotes = h("input", { type: "checkbox", id: "show-notes", checked: true });
  const notes = notesPanel({
    heading: "Notes on this passage",
    scope: () => (state.parsed ? { ref: state.parsed.osis, display: state.parsed.display } : null),
  });
  const today = todayCard();
  const study = studyPanel((ref) => go(ref), () => layout());
  const book = bookPane((open) => panes.classList.toggle("with-book", open));
  const panes = h("div", { class: "panes" }, h("section", { class: "pane", "aria-label": "Text" }, text), book.el, h("aside", { class: "pane" }, study.el, notes.el));
  const words = { onWord: study.showWord, onRef: (osis: string) => go(osis) };
  let modules: Module[] = [];
  let books: Book[] = [];
  let neighbours: { prev: string | null; next: string | null } = { prev: null, next: null };

  const form = h("form", { class: "passage-bar", role: "search" },
    h("label", { for: "passage" }, "Passage"), input, h("button", { type: "submit", class: "primary" }, "Read"), parsed);
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    go(input.value);
  });
  // The side column holds the notes (when wanted) and the study panel (when it has something).
  function layout() {
    notes.el.hidden = !showNotes.checked;
    panes.classList.toggle("single", !showNotes.checked && study.empty());
  }
  showNotes.addEventListener("change", layout);

  const el = h("div", { class: "view" },
    h("h1", {}, "Reader"),
    form,
    h("div", { class: "row wrap" }, picker, h("span", { class: "spacer" }),
      h("label", { class: "check" }, showNotes, " Notes beside the text"),
      button("Open a book", () => book.choose(), { title: "Read a library book beside the Bible" }), prev, next),
    today.el, msg.el, panes);

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
    study.showPassage(p.osis);
    neighbours = books.length ? neighbourChapters(p.start, books) : { prev: null, next: null };
    prev.disabled = !neighbours.prev;
    next.disabled = !neighbours.next;
    const chosen = state.modules.filter((m) => modules.some((x) => x.id === m));
    if (chosen.length === 0) {
      clear(text, h("p", { class: "empty" }, modules.length
        ? "Choose a Bible above to read this passage."
        : "No Bibles are installed yet. Install one in the Bibles section."));
      return;
    }
    if (chosen.length === 1) {
      const r = await msg.attempt(() => run<Column>("bible.read", { module: chosen[0], ref: p.osis, markup: true }));
      if (r) renderSingle(r);
    } else {
      const r = await msg.attempt(() => run<{ ref: string; columns: Column[] }>("bible.parallel", { modules: chosen, ref: p.osis, markup: true }));
      if (r) renderParallel(r.columns);
    }
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

  function verseEl(v: Verse) {
    const n = verseParts(v.id).verse;
    const end = v.end && v.end !== v.id ? `–${verseParts(v.end).verse}` : "";
    return h("span", { class: "verse", id: `v${v.id}` }, h("sup", { class: "vnum", "aria-label": `verse ${n}${end}` }, `${n}${end}`), " ", content(v), " ");
  }

  function renderSingle(c: Column) {
    const last = { chapter: -1 };
    const m = modules.find((x) => x.id === c.module);
    clear(text,
      h("header", {}, h("h2", { class: "module-name" }, m?.name || c.module)),
      c.verses.length === 0 ? h("p", { class: "empty" }, "This Bible has no text for this passage.") : null,
      c.verses.map((v) => [heading(v, last), verseEl(v)]),
      c.attribution ? h("footer", { class: "attribution" }, c.attribution) : null);
  }

  function renderParallel(columns: Column[]) {
    const rows = alignColumns(columns);
    const names = columns.map((c) => modules.find((x) => x.id === c.module)?.abbreviation || c.module);
    clear(text, h("table", { class: "parallel" },
      h("caption", { class: "sr-only" }, `Parallel reading of ${state.parsed?.display ?? ""}`),
      h("thead", {}, h("tr", {}, h("th", { scope: "col" }, h("span", { class: "sr-only" }, "Verse")), names.map((n) => h("th", { scope: "col" }, n)))),
      h("tbody", {}, rows.map((r) =>
        h("tr", {},
          h("th", { scope: "row", class: "vnum" }, `${verseParts(r.base).chapter}:${verseParts(r.base).verse}`),
          r.cells.map((v) => h("td", { lang: "" }, v ? content(v) : "")))))),
    columns.some((c) => c.attribution)
      ? h("footer", { class: "attribution" }, columns.filter((c) => c.attribution).map((c) => h("p", {}, `${c.module}: ${c.attribution}`)))
      : null);
  }

  function renderPicker() {
    clear(picker, modules.length === 0 ? h("span", { class: "hint" }, "No Bibles installed.") : modules.map((m) => {
      const box = h("input", { type: "checkbox", value: m.id, checked: state.modules.includes(m.id) });
      box.addEventListener("change", () => {
        const ids = [...picker.querySelectorAll<HTMLInputElement>("input:checked")].map((b) => b.value);
        setModules(ids);
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
    notes.refresh();
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
