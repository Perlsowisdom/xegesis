// Drawing a verse quotation (a quote block whose last line is its credit, as document.quote writes
// it): the verses in the reading face with small verse numbers, then the credit line in small sans
// type. Used by the editor's verse quote block and by the read-only Markdown view. Built from text
// nodes only, so nothing in a note can add markup.

import { h } from "../dom";
import { Span, parseInline } from "./markdown";

function spans(list: Span[], plain = false): Node[] {
  return list.map((s) => {
    if (s.br) return h("br");
    // A bold number at the start of a verse is its verse number.
    if (!plain && s.bold && /^\d+(-\d+)?$/.test(s.text.trim())) return h("sup", { class: "vq-num" }, s.text.trim());
    let n: Node = document.createTextNode(s.text);
    if (s.italic) n = h("em", {}, n);
    if (s.bold) n = h("strong", {}, n);
    return n;
  });
}

// verseQuoteParts draws the verses and the credit of a quotation from its lines.
export function verseQuoteParts(lines: string[]): { text: HTMLElement; credit: HTMLElement } {
  const body = lines.slice(0, -1).filter((l) => l.trim());
  const credit = lines[lines.length - 1].replace(/^—\s*/, "");
  const text = h("div", { class: "vq-text" });
  body.forEach((l, i) => {
    if (i) text.append(" ");
    text.append(...spans(parseInline(l)));
  });
  return { text, credit: h("small", { class: "vq-credit" }, spans(parseInline(credit), true)) };
}

export function verseQuoteEl(lines: string[]): HTMLElement {
  const { text, credit } = verseQuoteParts(lines);
  return h("blockquote", { class: "verse-quote" }, text, credit);
}
