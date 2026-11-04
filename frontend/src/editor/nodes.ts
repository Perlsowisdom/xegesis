// Two blocks of the editor that are not text to type in: a divider line, and a verse quotation
// (Bible text with its credit line, quoted through document.quote so its licence applies). Both are
// whole blocks: the caret steps over them and Backspace removes them. A verse quotation is kept
// exactly as it was quoted, since its credit belongs with its words.

import {
  $applyNodeReplacement, $getNodeByKey, DecoratorNode, type EditorConfig, type LexicalEditor, type LexicalNode, type NodeKey,
  type SerializedLexicalNode, type Spread,
} from "lexical";
import { verseQuoteParts } from "./versequote";

export class DividerNode extends DecoratorNode<null> {
  static getType(): string {
    return "xg-divider";
  }
  static clone(n: DividerNode): DividerNode {
    return new DividerNode(n.__key);
  }
  static importJSON(): DividerNode {
    return $createDividerNode();
  }
  exportJSON(): SerializedLexicalNode {
    return { type: "xg-divider", version: 1 };
  }
  createDOM(): HTMLElement {
    const el = document.createElement("div");
    el.className = "rich-divider";
    el.contentEditable = "false";
    el.append(document.createElement("hr"));
    return el;
  }
  updateDOM(): false {
    return false;
  }
  isInline(): false {
    return false;
  }
  decorate(): null {
    return null;
  }
}

export function $createDividerNode(): DividerNode {
  return $applyNodeReplacement(new DividerNode());
}

export function $isDividerNode(n: LexicalNode | null | undefined): n is DividerNode {
  return n instanceof DividerNode;
}

type SerializedVerseQuote = Spread<{ lines: string[] }, SerializedLexicalNode>;

export class VerseQuoteNode extends DecoratorNode<null> {
  __lines: string[];

  static getType(): string {
    return "xg-versequote";
  }
  static clone(n: VerseQuoteNode): VerseQuoteNode {
    return new VerseQuoteNode(n.__lines, n.__key);
  }
  static importJSON(j: SerializedVerseQuote): VerseQuoteNode {
    return $createVerseQuoteNode(j.lines);
  }
  constructor(lines: string[] = [], key?: NodeKey) {
    super(key);
    this.__lines = lines;
  }
  exportJSON(): SerializedVerseQuote {
    return { type: "xg-versequote", version: 1, lines: this.getLines() };
  }
  getLines(): string[] {
    return this.getLatest().__lines;
  }
  createDOM(_config: EditorConfig, editor: LexicalEditor): HTMLElement {
    const { text, credit } = verseQuoteParts(this.__lines);
    const el = document.createElement("blockquote");
    el.className = "verse-quote";
    el.contentEditable = "false";
    const key = this.__key;
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "vq-remove";
    remove.title = "Remove this quotation";
    remove.setAttribute("aria-label", "Remove this quotation");
    remove.textContent = "×";
    remove.addEventListener("mousedown", (e) => e.preventDefault());
    remove.addEventListener("click", () => editor.update(() => $getNodeByKey(key)?.remove()));
    el.append(text, credit, remove);
    return el;
  }
  updateDOM(): false {
    return false;
  }
  isInline(): false {
    return false;
  }
  decorate(): null {
    return null;
  }
}

// A reference chip: a link to verses ("#passage/Rom.8.28"), a note ("#notes/note/<id>") or a
// sermon ("#sermons/<id>"), shown as a small rounded chip inside the text. It is one piece: the
// caret steps over it and Backspace removes it. Clicking it opens what it names (the editor
// handles the click). In Markdown it is an ordinary link, [Romans 8:28](#passage/Rom.8.28).

export type ChipKind = "verse" | "note" | "sermon";

export function chipKind(href: string): ChipKind | null {
  if (href.startsWith("#passage/")) return "verse";
  if (href.startsWith("#notes/note/")) return "note";
  if (href.startsWith("#sermons/")) return "sermon";
  return null;
}

type SerializedRefChip = Spread<{ label: string; href: string }, SerializedLexicalNode>;

export class RefChipNode extends DecoratorNode<null> {
  __label: string;
  __href: string;

  static getType(): string {
    return "xg-refchip";
  }
  static clone(n: RefChipNode): RefChipNode {
    return new RefChipNode(n.__label, n.__href, n.__key);
  }
  static importJSON(j: SerializedRefChip): RefChipNode {
    return $createRefChipNode(j.label, j.href);
  }
  constructor(label = "", href = "", key?: NodeKey) {
    super(key);
    this.__label = label;
    this.__href = href;
  }
  exportJSON(): SerializedRefChip {
    return { type: "xg-refchip", version: 1, label: this.getLabel(), href: this.getHref() };
  }
  getLabel(): string {
    return this.getLatest().__label;
  }
  getHref(): string {
    return this.getLatest().__href;
  }
  getTextContent(): string {
    return this.getLabel();
  }
  createDOM(): HTMLElement {
    const kind = chipKind(this.__href) ?? "note";
    const el = document.createElement("span");
    el.className = `ref-chip chip-${kind}`;
    el.contentEditable = "false";
    el.dataset.href = this.__href;
    el.dataset.label = this.__label;
    el.setAttribute("role", "link");
    el.title = kind === "verse" ? `Open ${this.__label} in the study grid` : `Open “${this.__label}”`;
    const mark = document.createElement("span");
    mark.className = "ref-chip-mark";
    mark.setAttribute("aria-hidden", "true");
    mark.textContent = kind === "verse" ? "✦" : kind === "sermon" ? "§" : "¶";
    el.append(mark, document.createTextNode(this.__label));
    return el;
  }
  updateDOM(): false {
    return false;
  }
  isInline(): true {
    return true;
  }
  decorate(): null {
    return null;
  }
}

export function $createRefChipNode(label: string, href: string): RefChipNode {
  return $applyNodeReplacement(new RefChipNode(label, href));
}

export function $isRefChipNode(n: LexicalNode | null | undefined): n is RefChipNode {
  return n instanceof RefChipNode;
}

export function $createVerseQuoteNode(lines: string[]): VerseQuoteNode {
  return $applyNodeReplacement(new VerseQuoteNode(lines));
}

export function $isVerseQuoteNode(n: LexicalNode | null | undefined): n is VerseQuoteNode {
  return n instanceof VerseQuoteNode;
}
