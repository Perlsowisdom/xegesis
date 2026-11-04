// Drawing verse and reference markup as elements. Tagged words keep their Strong's numbers (but look
// like plain text); references become links to the passage. Notes and headings inside the markup are left out.

import { h } from "./dom";
import { MarkupNode, parseMarkup, strongNumbers } from "./markupwords";

export interface WordInfo {
  text: string;
  strongs: string[];
  morph?: string;
  gloss?: string;
}

export interface MarkupHandlers {
  onRef?: (osis: string, label: string) => void;
}

export function renderMarkup(markup: string, handlers: MarkupHandlers = {}): Node[] {
  return draw(parseMarkup(markup), handlers);
}

function draw(nodes: MarkupNode[], hd: MarkupHandlers): Node[] {
  const out: Node[] = [];
  for (const n of nodes) {
    switch (n.tag) {
      case "":
        out.push(document.createTextNode(n.text));
        break;
      case "f": case "x": case "h":
        break;
      case "p":
        if (out.length) out.push(document.createTextNode(" "));
        break;
      case "w":
        out.push(word(n, hd));
        break;
      case "j":
        out.push(h("span", { class: "woc" }, draw(n.children, hd)));
        break;
      case "add":
        out.push(h("em", {}, draw(n.children, hd)));
        break;
      case "r": {
        const label = textOf(n);
        if (n.attrs.o && hd.onRef) {
          out.push(h("a", { href: "#", class: "ref", onclick: (e: Event) => { e.preventDefault(); hd.onRef!(n.attrs.o, label); } }, draw(n.children, hd)));
        } else out.push(...draw(n.children, hd));
        break;
      }
      default:
        out.push(...draw(n.children, hd));
    }
  }
  return out;
}

function textOf(n: MarkupNode): string {
  return n.tag === "" ? n.text : n.children.map(textOf).join("");
}

// word draws a tagged word as plain text that carries its numbers, so it can be looked up when
// asked (marking.ts, lookup.ts) without looking clickable.
function word(n: MarkupNode, hd: MarkupHandlers): Node {
  const strongs = strongNumbers(n.attrs.s);
  const children = draw(n.children, hd);
  if (!strongs.length) return h("span", {}, children);
  return h("span", { class: "w", "data-strongs": strongs.join(" "), "data-morph": n.attrs.m || null, "data-gloss": n.attrs.g || null }, children);
}

// paragraphs draws reference-work markup, where <p/> starts a new paragraph.
export function paragraphs(markup: string, handlers: MarkupHandlers = {}): HTMLElement[] {
  return markup.split("<p/>").filter((p) => p.trim()).map((p) => h("p", {}, renderMarkup(p, handlers)));
}
