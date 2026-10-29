// Reading the light markup of verses and reference entries (docs/modules.md, "Markup") without
// a DOM, so it can be tested. markup.ts turns the result into elements.

export interface MarkupNode {
  tag: string; // "" for text
  attrs: Record<string, string>;
  text: string;
  children: MarkupNode[];
}

const entities: Record<string, string> = { "&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": '"', "&apos;": "'" };

export function unescapeText(s: string): string {
  return s.replace(/&(amp|lt|gt|quot|apos);/g, (m) => entities[m] ?? m);
}

function attrsOf(s: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const m of s.matchAll(/([a-z]+)="([^"]*)"/g)) out[m[1]] = unescapeText(m[2]);
  return out;
}

// parseMarkup reads markup into a tree. Unknown or unbalanced tags are kept as best it can:
// a closing tag without an opening one is ignored, and open elements close at the end.
export function parseMarkup(s: string): MarkupNode[] {
  const root: MarkupNode = { tag: "#root", attrs: {}, text: "", children: [] };
  const stack: MarkupNode[] = [root];
  const re = /<(\/?)([a-z]+)([^>]*?)(\/?)>/g;
  let last = 0;
  const top = () => stack[stack.length - 1];
  for (const m of s.matchAll(re)) {
    if (m.index! > last) top().children.push(textNode(s.slice(last, m.index)));
    last = m.index! + m[0].length;
    const [, close, tag, attrs, self] = m;
    if (close) {
      const i = stack.map((n) => n.tag).lastIndexOf(tag);
      if (i > 0) stack.length = i;
      continue;
    }
    const node: MarkupNode = { tag, attrs: attrsOf(attrs), text: "", children: [] };
    top().children.push(node);
    if (!self) stack.push(node);
  }
  if (last < s.length) top().children.push(textNode(s.slice(last)));
  return root.children;
}

function textNode(s: string): MarkupNode {
  return { tag: "", attrs: {}, text: unescapeText(s), children: [] };
}

// plainText is the text of nodes, leaving out notes and headings.
export function plainText(nodes: MarkupNode[]): string {
  return nodes.map((n) => (n.tag === "" ? n.text : n.tag === "f" || n.tag === "x" || n.tag === "h" ? "" : plainText(n.children))).join("");
}

// hasWords reports whether markup holds tagged words, so the reader knows to draw them.
export function hasWords(markup: string | undefined): boolean {
  return !!markup && /<w s="/.test(markup);
}

// strongNumbers splits a word's s attribute into Strong's numbers.
export function strongNumbers(s: string | undefined): string[] {
  return (s ?? "").split(/\s+/).filter((n) => /^[HG]\d+[A-Za-z]?$/.test(n));
}

// kindLabel names a module kind in plain words.
export function kindLabel(kind: string): string {
  switch (kind) {
    case "commentary": return "Commentary";
    case "crossref": return "Cross-references";
    case "dictionary": return "Dictionary";
    case "lexicon": return "Lexicon";
    default: return "Bible";
  }
}
