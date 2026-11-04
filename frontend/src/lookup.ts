// The look-up card: a small card beside a word, opened only when asked (Alt+click, Look up in the
// selection toolbar, or the right-click menu). It shows the word's lemma, transliteration, Strong's
// number and a short gloss from the first lexicon that has it, names that lexicon, and leads on to
// the Word tool. A reference looks up as its passage instead. Escape or a click elsewhere closes it.

import { Ref, run } from "./api";
import { button, h } from "./dom";
import { WordInfo } from "./markup";
import { closePopover, openPopover } from "./popover";
import { searchFor } from "./views/search";

interface LexEntry {
  module: string; name: string; key: string; strong?: string; lemma?: string; transliteration?: string;
  gloss?: string; text: string;
}

export interface LookUpHooks {
  showWord?(w: WordInfo): void; // the Word tool
  go?(ref: string): void; // a passage
}

// wordInfo reads a tagged word's numbers from its element (markup.ts puts them there).
export function wordInfo(el: Element | null, text: string): WordInfo | null {
  const s = el?.getAttribute("data-strongs");
  if (!el || !s) return null;
  return { text: el.textContent?.trim() || text, strongs: s.split(" ").filter(Boolean),
    morph: el.getAttribute("data-morph") ?? undefined, gloss: el.getAttribute("data-gloss") ?? undefined };
}

// short is the first sentence or so of a lexicon's text.
function short(e: LexEntry): string {
  const t = (e.gloss || e.text || "").replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim();
  if (t.length <= 180) return t;
  const cut = t.slice(0, 180);
  const stop = Math.max(cut.lastIndexOf(". "), cut.lastIndexOf("; "));
  return (stop > 60 ? cut.slice(0, stop + 1) : `${cut.replace(/\s+\S*$/, "")}…`);
}

export async function lookUpCard(text: string, word: Element | null, at: DOMRect, hooks: LookUpHooks): Promise<void> {
  const info = wordInfo(word, text);
  const q = text.replace(/\s+/g, " ").trim().split(" ").slice(0, 3).join(" ");
  if (!info && /\d/.test(q) && q.length < 40 && hooks.go) {
    const p = await run<Ref>("verse.parse", { ref: q }).catch(() => null);
    if (p) return hooks.go(p.display);
  }
  const numbers = info?.strongs.filter((s) => !/^H9\d{3}/.test(s)) ?? [];
  const strong = numbers[numbers.length - 1] ?? info?.strongs[0];
  const entries = await run<LexEntry[]>("lexicon.lookup", strong ? { strong, limit: 1 } : { word: q, limit: 1 }).catch(() => [] as LexEntry[]);
  const e = entries[0];
  const anchor = h("span", { class: "ctx-anchor", style: `left:${at.left}px;top:${at.bottom}px` });
  document.body.append(anchor);
  const more = info && hooks.showWord
    ? button("More in the Word tool →", () => { closePopover(); hooks.showWord!(info); }, { class: "card-link" })
    : button("Search the library →", () => { closePopover(); searchFor(q); }, { class: "card-link" });
  const card = h("div", { class: "lookup-card" },
    h("div", { class: "lookup-top" },
      h("span", { class: "lookup-lemma", lang: strong?.startsWith("H") ? "he" : strong?.startsWith("G") ? "grc" : null }, e?.lemma || info?.text || q),
      e?.transliteration ? h("span", { class: "lookup-translit" }, e.transliteration) : null,
      h("span", { class: "spacer" }),
      strong ? h("span", { class: "lookup-num" }, strong) : null),
    h("p", { class: "lookup-gloss" }, e ? short(e) || info?.gloss || "" : info?.gloss || "No entry for this word in your lexicons and dictionaries."),
    h("div", { class: "lookup-foot" }, h("span", { class: "lookup-source" }, e?.name ?? (info ? "Tagged word" : "")), more));
  openPopover(anchor, card, { label: `Look up ${info?.text || q}`, side: "below", className: "doc-pop lookup-pop",
    onClose: () => setTimeout(() => anchor.remove(), 0) });
}
