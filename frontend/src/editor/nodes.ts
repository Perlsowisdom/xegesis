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

export function $createVerseQuoteNode(lines: string[]): VerseQuoteNode {
  return $applyNodeReplacement(new VerseQuoteNode(lines));
}

export function $isVerseQuoteNode(n: LexicalNode | null | undefined): n is VerseQuoteNode {
  return n instanceof VerseQuoteNode;
}
