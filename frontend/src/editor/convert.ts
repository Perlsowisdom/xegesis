// Lexical nodes <-> the Markdown blocks of markdown.ts, both ways.


import { $createLineBreakNode, $createParagraphNode, $createTextNode, $isElementNode, $isLineBreakNode, $isTextNode, ElementNode, LexicalNode } from "lexical";
import { $createLinkNode, $isLinkNode, LinkNode } from "@lexical/link";
import { $createListItemNode, $createListNode, $isListItemNode, $isListNode } from "@lexical/list";
import { $createHeadingNode, $createQuoteNode, $isHeadingNode, $isQuoteNode } from "@lexical/rich-text";
import { $createTableCellNode, $createTableNode, $createTableRowNode, $isTableCellNode, $isTableNode, $isTableRowNode, TableCellHeaderStates, TableNode } from "@lexical/table";
import { Block, Span } from "./markdown";
import { $createDividerNode, $createRefChipNode, $createVerseQuoteNode, $isDividerNode, $isRefChipNode, $isVerseQuoteNode, chipKind } from "./nodes";
import { $createInkNode, $isInkNode } from "./inknode";

// Lexical <-> blocks -----------------------------------------------------------------------------

// plainLink is a link run of one unformatted piece, which shows as a chip when it names a place in
// the app (verses, a note or a sermon).
function chipAt(spans: Span[], i: number): boolean {
  const s = spans[i];
  if (!s.link || !chipKind(s.link) || s.bold || s.italic || s.underline || s.code || !s.text.trim()) return false;
  return spans[i - 1]?.link !== s.link && spans[i + 1]?.link !== s.link;
}

export function spanNodes(spans: Span[]): LexicalNode[] {
  const out: LexicalNode[] = [];
  let link: { url: string; node: LinkNode } | null = null;
  for (const [i, s] of spans.entries()) {
    if (chipAt(spans, i)) {
      link = null;
      out.push($createRefChipNode(s.text, s.link!));
      continue;
    }
    if (s.br) {
      link = null;
      out.push($createLineBreakNode());
      continue;
    }
    const t = $createTextNode(s.text);
    if (s.bold) t.toggleFormat("bold");
    if (s.italic) t.toggleFormat("italic");
    if (s.underline) t.toggleFormat("underline");
    if (s.code) t.toggleFormat("code");
    if (s.link) {
      if (!link || link.url !== s.link) {
        link = { url: s.link, node: $createLinkNode(s.link) };
        out.push(link.node);
      }
      link.node.append(t);
    } else {
      link = null;
      out.push(t);
    }
  }
  return out;
}

export function fromBlocks(blocks: Block[]): LexicalNode[] {
  return blocks.map((b) => {
    switch (b.type) {
      case "heading":
        return $createHeadingNode(`h${b.level}` as "h1").append(...spanNodes(b.spans));
      case "quote":
        return $createQuoteNode().append(...spanNodes(b.spans));
      case "list": {
        const list = $createListNode(b.checked ? "check" : b.ordered ? "number" : "bullet");
        b.items.forEach((it, i) => list.append($createListItemNode(b.checked ? b.checked[i] : undefined).append(...spanNodes(it))));
        return list;
      }
      case "divider":
        return $createDividerNode();
      case "ink":
        return $createInkNode(b.ink);
      case "versequote":
        return $createVerseQuoteNode(b.lines);
      case "table": {
        const t = $createTableNode();
        if (b.widths) t.setColWidths(b.widths);
        b.rows.forEach((r, i) => t.append($createTableRowNode().append(...r.map((c) =>
          $createTableCellNode(b.header && i === 0 ? TableCellHeaderStates.ROW : TableCellHeaderStates.NO_STATUS)
            .append($createParagraphNode().append(...spanNodes(c)))))));
        return t;
      }
      default:
        return $createParagraphNode().append(...spanNodes(b.spans));
    }
  });
}

function inline(parent: ElementNode, link?: string): Span[] {
  const out: Span[] = [];
  for (const c of parent.getChildren()) {
    if ($isLineBreakNode(c)) out.push({ text: "", br: true });
    else if ($isTextNode(c)) {
      const s: Span = { text: c.getTextContent() };
      if (c.hasFormat("bold")) s.bold = true;
      if (c.hasFormat("italic")) s.italic = true;
      if (c.hasFormat("underline")) s.underline = true;
      if (c.hasFormat("code")) s.code = true;
      if (link) s.link = link;
      out.push(s);
    } else if ($isRefChipNode(c)) out.push({ text: c.getLabel(), link: c.getHref() });
    else if ($isLinkNode(c)) out.push(...inline(c, c.getURL()));
    else if ($isElementNode(c)) out.push(...inline(c, link));
    else out.push({ text: c.getTextContent(), ...(link ? { link } : {}) });
  }
  return out;
}

export function toBlocks(nodes: LexicalNode[]): Block[] {
  return nodes.map((n): Block => {
    if ($isHeadingNode(n)) return { type: "heading", level: Number(n.getTag().slice(1)), spans: inline(n) };
    if ($isQuoteNode(n)) return { type: "quote", spans: inline(n) };
    if ($isDividerNode(n)) return { type: "divider" };
    if ($isInkNode(n)) return { type: "ink", ink: n.getInk() };
    if ($isVerseQuoteNode(n)) return { type: "versequote", lines: n.getLines() };
    if ($isTableNode(n)) return tableBlock(n);
    if ($isListNode(n)) {
      const items = n.getChildren();
      return { type: "list", ordered: n.getListType() === "number",
        items: items.map((li) => ($isElementNode(li) ? inline(li) : [{ text: li.getTextContent() }])),
        ...(n.getListType() === "check" ? { checked: items.map((li) => $isListItemNode(li) && !!li.getChecked()) } : {}) };
    }
    if ($isElementNode(n)) return { type: "paragraph", spans: inline(n) };
    return { type: "paragraph", spans: [{ text: n.getTextContent() }] };
  });
}

// tableBlock reads a table: each cell's paragraphs become lines of one cell.
function tableBlock(t: TableNode): Block {
  const rows = t.getChildren().filter($isTableRowNode).map((r) => r.getChildren().filter($isTableCellNode).map((c) => {
    const out: Span[] = [];
    c.getChildren().forEach((p, i) => {
      if (i) out.push({ text: "", br: true });
      out.push(...($isElementNode(p) ? inline(p) : [{ text: p.getTextContent() }]));
    });
    return out;
  }));
  const first = t.getFirstChild();
  const header = $isTableRowNode(first) && first.getChildrenSize() > 0
    && first.getChildren().every((c) => $isTableCellNode(c) && c.hasHeaderState(TableCellHeaderStates.ROW));
  const cols = Math.max(0, ...rows.map((r) => r.length));
  const widths = t.getColWidths();
  return { type: "table", rows, header, ...(widths && widths.length === cols ? { widths: widths.map((w) => Math.round(w)) } : {}) };
}
