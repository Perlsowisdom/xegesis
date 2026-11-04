// Shows stored Markdown as read-only formatted text. Built from DOM nodes and text only, so nothing
// in a note can add markup or run script.

import { h } from "../dom";
import { Block, Span, parseMarkdown } from "./markdown";
import { verseQuoteEl } from "./versequote";

export function renderMarkdown(md: string): HTMLElement {
  return h("div", { class: "md" }, parseMarkdown(md).map(block));
}

function block(b: Block): Node {
  switch (b.type) {
    case "heading":
      return h(`h${Math.min(b.level + 2, 6)}` as "h3", { class: "md-h" }, inline(b.spans));
    case "quote":
      return h("blockquote", {}, inline(b.spans));
    case "list":
      return h(b.ordered ? "ol" : "ul", {}, b.items.map((it) => h("li", {}, inline(it))));
    case "divider":
      return h("hr", { class: "md-hr" });
    case "versequote":
      return verseQuoteEl(b.lines);
    default:
      return b.spans.length === 0 ? h("p", { class: "md-empty", "aria-hidden": "true" }) : h("p", {}, inline(b.spans));
  }
}

function inline(spans: Span[]): Node[] {
  return spans.map((s) => {
    if (s.br) return h("br");
    let n: Node = document.createTextNode(s.text);
    if (s.code) n = h("code", {}, n);
    if (s.underline) n = h("u", {}, n);
    if (s.italic) n = h("em", {}, n);
    if (s.bold) n = h("strong", {}, n);
    // Links to app addresses (#passage/…, as saved reports have) can be followed; others only show.
    if (s.link) n = s.link.startsWith("#") ? h("a", { class: "md-link", href: s.link }, n) : h("span", { class: "md-link", title: s.link }, n);
    return n;
  });
}
