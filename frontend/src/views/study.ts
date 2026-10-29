// Beside the Bible text: what the installed commentaries and cross-references say about the
// passage, and, when a tagged word is clicked, its lexicon entry and where else it occurs. Each part
// shows only when it has something to show.

import { Message, run } from "../api";
import { button, clear, h } from "../dom";
import { WordInfo, paragraphs } from "../markup";

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

export function studyPanel(go: (ref: string) => void, changed: () => void) {
  const msg = new Message();
  const wordBox = h("section", { class: "study-word", "aria-label": "Word", "aria-live": "polite", hidden: true });
  const passageBox = h("section", { class: "study-passage", "aria-label": "On this passage", hidden: true });
  const el = h("div", { class: "study" }, msg.el, wordBox, passageBox);
  let passageSeq = 0;
  let wordSeq = 0;

  const link = (ref: string, label = ref) =>
    h("a", { href: "#", class: "ref", onclick: (e: Event) => { e.preventDefault(); go(ref); } }, label);
  const handlers = { onRef: (osis: string) => go(osis) };

  // showPassage lists commentary entries and cross-references; it hides itself when there are none.
  async function showPassage(ref: string) {
    const seq = ++passageSeq;
    const r = await run<ForPassage>("reference.for_passage", { ref }).catch(() => null);
    if (seq !== passageSeq) return;
    if (!r || (r.commentaries.length === 0 && r.cross_references.length === 0)) {
      clear(passageBox);
      passageBox.hidden = true;
      changed();
      return;
    }
    const byVerse = new Map<string, CrossRef[]>();
    for (const x of r.cross_references) byVerse.set(x.from_ref, [...(byVerse.get(x.from_ref) ?? []), x]);
    clear(passageBox,
      h("h3", {}, "On this passage"),
      r.commentaries.map((c, i) => h("details", { class: "commentary", open: i === 0 },
        h("summary", {}, c.name),
        c.entries.map((e) => h("div", { class: "entry" },
          h("h4", {}, e.ref),
          paragraphs(e.markup || e.text, handlers))),
        c.attribution ? h("p", { class: "attribution" }, c.attribution) : null)),
      byVerse.size ? h("details", { class: "crossrefs", open: r.commentaries.length === 0 },
        h("summary", {}, `Cross-references (${r.cross_references.length})`),
        h("dl", {}, [...byVerse].map(([from, xs]) => [
          h("dt", {}, from),
          h("dd", {}, joined(xs.slice(0, MAX_REFS).map((x) => link(x.to, x.to_ref))),
            xs.length > MAX_REFS ? h("small", { class: "hint" }, ` and ${xs.length - MAX_REFS} more`) : null),
        ]))) : null);
    passageBox.hidden = false;
    changed();
  }

  // showWord looks a clicked word up in the lexicons and finds where else it occurs.
  // A word can carry several numbers ("God" is often tagged G3588 G2316, the article then the noun);
  // the last one is the word itself, and the others are offered as "also tagged".
  async function showWord(w: WordInfo, chosen?: string) {
    const seq = ++wordSeq;
    const numbers = w.strongs.filter((s) => !/^H9\d{3}/.test(s));
    const number = chosen ?? numbers[numbers.length - 1] ?? w.strongs[0];
    const others = numbers.filter((s) => s !== number);
    wordBox.hidden = false;
    clear(wordBox, h("p", { class: "hint" }, `Looking up ${w.text}…`));
    changed();
    const [entries, study] = await Promise.all([
      run<LexEntry[]>("lexicon.lookup", { strong: number, limit: 5 }).catch(() => [] as LexEntry[]),
      run<Study>("word.study", { strong: number, limit: MAX_REFS }).catch(() => null),
    ]);
    if (seq !== wordSeq) return;
    const head = entries[0];
    clear(wordBox,
      h("div", { class: "row between" },
        h("h3", {}, head?.lemma || w.text),
        button("Close", () => closeWord(), { "aria-label": "Close the word panel" })),
      h("p", { class: "word-facts" },
        [head?.transliteration, w.gloss || head?.gloss, number, w.morph].filter(Boolean).join(" · ")),
      others.length ? h("p", { class: "hint" }, "Also tagged: ",
        others.map((n, i) => [i ? ", " : "", button(n, () => showWord(w, n), { class: "link" })])) : null,
      entries.length === 0 ? h("p", { class: "hint" }, "No lexicon installed has this word. Lexicons are in Bibles → Get Bibles, under Dictionaries and lexicons.") :
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
    changed();
  }

  function closeWord() {
    wordSeq++;
    clear(wordBox);
    wordBox.hidden = true;
    changed();
  }

  return { el, showPassage, showWord, closeWord, empty: () => wordBox.hidden && passageBox.hidden };
}

function joined(nodes: Node[]): Node[] {
  const out: Node[] = [];
  nodes.forEach((n, i) => {
    if (i) out.push(document.createTextNode("; "));
    out.push(n);
  });
  return out;
}
