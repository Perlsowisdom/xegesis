// Two tools of the right sidebar (views/sidebar.ts): Related, what the installed commentaries and
// cross-references say about the passage; and Word, a clicked word's lexicon entries and where else
// it occurs. Each says plainly when it has nothing to show.

import { run } from "../api";
import { button, clear, h } from "../dom";
import { addToList } from "../listpicker";
import { WordInfo, paragraphs } from "../markup";
import { openReport } from "./report";
import { draggableSpec } from "../document/drag";
import type { TabSpec } from "../document/grid";

interface Commentary { module: string; name: string; attribution?: string; entries: { ref: string; markup?: string; text: string }[] }
interface CrossRef { from: number; from_ref: string; to: string; to_ref: string; sources: string[] }
interface ForPassage { commentaries: Commentary[]; cross_references: CrossRef[] }
interface LexEntry {
  module: string; name: string; attribution?: string; key: string; strong?: string; lemma?: string;
  transliteration?: string; morphology?: string; gloss?: string; text: string; markup?: string;
}
interface Rendering { text: string; count: number; refs: string[] }
interface StudyBible { module: string; name: string; occurrences: number; verses: number; renderings: Rendering[] }
interface Study { strong: string; occurrences: number; bibles: StudyBible[] }

const MAX_REFS = 12;

// card lets a Related card be dragged into the study grid, where it opens as a tab.
function card(el: HTMLElement, spec: TabSpec): HTMLElement {
  draggableSpec(el, () => spec);
  el.title = "Drag into a pane to open it there";
  return el;
}

export function relatedPanel(go: (ref: string) => void) {
  const passageBox = h("div", { class: "study-passage", "aria-live": "polite" });
  const el = h("div", { class: "study" }, passageBox);
  let passageSeq = 0;
  const link = (ref: string, label = ref) =>
    h("a", { href: "#", class: "ref", onclick: (e: Event) => { e.preventDefault(); go(ref); } }, label);
  const handlers = { onRef: (osis: string) => go(osis) };

  // showPassage lists commentary entries and cross-references for the passage.
  async function showPassage(ref: string | null, display = "") {
    const seq = ++passageSeq;
    if (!ref) {
      clear(passageBox, h("p", { class: "empty" }, "Open a passage in the Reader to see what your library says about it."));
      return;
    }
    clear(passageBox, h("p", { class: "hint" }, `Looking for ${display || ref}…`));
    const r = await run<ForPassage>("reference.for_passage", { ref }).catch(() => null);
    if (seq !== passageSeq) return;
    if (!r || (r.commentaries.length === 0 && r.cross_references.length === 0)) {
      clear(passageBox, h("p", { class: "empty" }, `Nothing yet for ${display || ref}.`),
        h("p", { class: "hint" }, "Commentaries and cross-references show here when your library has them. Add them from Library → Get Bibles."));
      return;
    }
    const byVerse = new Map<string, CrossRef[]>();
    for (const x of r.cross_references) byVerse.set(x.from_ref, [...(byVerse.get(x.from_ref) ?? []), x]);
    clear(passageBox,
      h("h3", {}, display ? `On ${display}` : "On this passage"),
      r.commentaries.map((c, i) => card(h("details", { class: "commentary", open: i === 0 },
        h("summary", {}, c.name, h("span", { class: "sr-only" }, " (drag into the grid to open it in a pane)")),
        c.entries.map((e) => h("div", { class: "entry" },
          h("h4", {}, e.ref),
          paragraphs(e.markup || e.text, handlers))),
        c.attribution ? h("p", { class: "attribution" }, c.attribution) : null), { kind: "commentary", modules: [c.module], title: c.name })),
      byVerse.size ? h("details", { class: "crossrefs", open: r.commentaries.length === 0 },
        h("summary", {}, `Cross-references (${r.cross_references.length})`),
        h("dl", {}, [...byVerse].map(([from, xs]) => [
          h("dt", {}, from),
          h("dd", {}, joined(xs.slice(0, MAX_REFS).map((x) => link(x.to, x.to_ref))),
            xs.length > MAX_REFS ? h("small", { class: "hint" }, ` and ${xs.length - MAX_REFS} more`) : null),
        ]))) : null);
  }

  return { el, showPassage };
}

export function wordPanel(go: (ref: string) => void) {
  const wordBox = h("div", { class: "study-word", "aria-live": "polite" });
  const el = h("div", { class: "study" }, wordBox);
  let wordSeq = 0;
  const link = (ref: string, label = ref) =>
    h("a", { href: "#", class: "ref", onclick: (e: Event) => { e.preventDefault(); go(ref); } }, label);
  const handlers = { onRef: (osis: string) => go(osis) };
  const empty = () => clear(wordBox, h("p", { class: "empty" }, "No word chosen."),
    h("p", { class: "hint" }, "Click a word in a Bible tagged with Strong's numbers to look it up here."));
  empty();

  // showWord looks a clicked word up in the lexicons and finds where else it occurs.
  // A word can carry several numbers ("God" is often tagged G3588 G2316, the article then the noun);
  // the last one is the word itself, and the others are offered as "also tagged".
  async function showWord(w: WordInfo, chosen?: string) {
    const seq = ++wordSeq;
    const numbers = w.strongs.filter((s) => !/^H9\d{3}/.test(s));
    const number = chosen ?? numbers[numbers.length - 1] ?? w.strongs[0];
    const others = numbers.filter((s) => s !== number);
    clear(wordBox, h("p", { class: "hint" }, `Looking up ${w.text}…`));
    const [entries, study] = await Promise.all([
      run<LexEntry[]>("lexicon.lookup", { strong: number, limit: 5 }).catch(() => [] as LexEntry[]),
      run<Study>("word.study", { strong: number, limit: MAX_REFS }).catch(() => null),
    ]);
    if (seq !== wordSeq) return;
    const head = entries[0];
    clear(wordBox,
      h("div", { class: "row between" },
        h("h3", {}, head?.lemma || w.text),
        button("Clear", () => { wordSeq++; empty(); }, { "aria-label": "Clear the word" })),
      h("p", { class: "word-facts" },
        [head?.transliteration, w.gloss || head?.gloss, number, w.morph].filter(Boolean).join(" · ")),
      h("div", { class: "row wrap" },
        button("Word report", () => openReport("word", number), { title: "Everything your library holds about this word, in one place" }),
        button("Add to a word list", () => addToList("words", `“${head?.lemma || w.text}”`, {
          word: w.text, strong: number, lemma: head?.lemma ?? "", language: number.startsWith("H") ? "hbo" : "grc" }),
          { title: "Keep this word, with its Strong's number, in a list" })),
      others.length ? h("p", { class: "hint" }, "Also tagged: ",
        others.map((n, i) => [i ? ", " : "", button(n, () => showWord(w, n), { class: "link" })])) : null,
      entries.length === 0 ? h("p", { class: "hint" }, "No lexicon installed has this word. Lexicons are in Library → Get Bibles, under Dictionaries and lexicons.") :
        entries.map((e) => h("div", { class: "lex-entry" },
          h("h4", {}, `${e.name}${e.key && e.key !== e.strong ? ` (${e.key})` : ""}`),
          paragraphs(e.markup || e.text, handlers),
          e.attribution ? h("p", { class: "attribution" }, e.attribution) : null)),
      study && study.bibles.length ? h("div", { class: "occurrences" },
        h("h4", {}, `Where it occurs (${study.occurrences.toLocaleString()} times)`),
        study.bibles.map((b) => h("details", { open: study.bibles.length === 1 },
          h("summary", {}, `${b.name}: ${b.occurrences.toLocaleString()} in ${b.verses.toLocaleString()} verses`),
          h("ul", { class: "renderings" }, b.renderings.slice(0, 15).map((r) => h("li", {},
            h("strong", {}, r.text), ` ${r.count}× `,
            joined(r.refs.map((ref) => link(ref))))))))) : null);
  }

  return { el, showWord };
}

function joined(nodes: Node[]): Node[] {
  const out: Node[] = [];
  nodes.forEach((n, i) => {
    if (i) out.push(document.createTextNode("; "));
    out.push(n);
  });
  return out;
}
