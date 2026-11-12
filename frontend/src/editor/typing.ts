// Typing, keys, paste and drop: what the editor does with the keyboard, the clipboard and dragged items.


import { $createParagraphNode, $createRangeSelection, $createTextNode, $getNodeByKey, $getRoot, $getSelection, $insertNodes, $isElementNode, $isParagraphNode, $isRangeSelection, $isTextNode, $setSelection, FORMAT_TEXT_COMMAND, LexicalEditor } from "lexical";
import { $isListItemNode } from "@lexical/list";
import { type ElementTransformer, type TextFormatTransformer } from "@lexical/markdown";
import { $findMatchingParent } from "@lexical/utils";
import { $isHeadingNode, $isQuoteNode } from "@lexical/rich-text";
import { parsePasted } from "./markdown";
import { $createDividerNode, $createRefChipNode, $isDividerNode, DividerNode } from "./nodes";
import { refCandidates } from "./refwords";
import { DROP_TYPES, topBlock } from "./common";
import { type EditorOptions, type InsertId } from "./types";
import { fromBlocks, spanNodes } from "./convert";
import { insertMarkdown } from "./insert";
import { shiftHeading } from "./headings";

// Markdown as you type ----------------------------------------------------------------------------

// UNDERLINE: __text__ underlines (not bold, as in some Markdown); it is stored as <u>text</u>.
export const UNDERLINE: TextFormatTransformer = { format: ["underline"], intraword: false, tag: "__", type: "text-format" };

// DIVIDER turns a line of "---" (or "***") into a divider when Enter or a space is typed after it.
export const DIVIDER: ElementTransformer = {
  dependencies: [DividerNode],
  export: (n) => ($isDividerNode(n) ? "---" : null),
  regExp: /^(?:---|\*\*\*) ?/,
  triggerOnEnter: true,
  replace(parent) {
    if (!/^(?:---|\*\*\*)$/.test(parent.getTextContent().trim()) || !$isParagraphNode(parent)) return false;
    const d = $createDividerNode();
    const p = $createParagraphNode();
    parent.replace(d);
    d.insertAfter(p);
    p.select();
  },
  type: "element",
};

// $plainAtStart is Backspace at the very start of a heading, quote or list item: it becomes plain
// text, as in other editors, instead of joining the line above.
export function $plainAtStart(): boolean {
  const s = $getSelection();
  if (!$isRangeSelection(s) || !s.isCollapsed() || s.anchor.offset !== 0) return false;
  const node = s.anchor.getNode();
  const li = $findMatchingParent(node, $isListItemNode);
  const block = li ?? node.getTopLevelElement();
  if (!block || !$isElementNode(block)) return false;
  const first = block.getFirstDescendant();
  if (node !== block && node !== first) return false;
  if (li && $isListItemNode(li)) {
    // A nested item moves out a level first, as Lexical does.
    return li.collapseAtStart(s);
  }
  if (!$isHeadingNode(block) && !$isQuoteNode(block)) return false;
  const p = $createParagraphNode();
  p.append(...block.getChildren());
  block.replace(p);
  p.selectStart();
  return true;
}

// pasteMarkdown puts pasted plain text in as Markdown, formatted. Copies with their own formatting
// (from a web page, or from this editor) are left to Lexical.
export function pasteMarkdown(e: ClipboardEvent | KeyboardEvent | InputEvent): boolean {
  const data = "clipboardData" in e ? e.clipboardData : null;
  if (!data || data.types.includes("text/html") || data.types.includes("application/x-lexical-editor") || data.files.length) return false;
  const text = data.getData("text/plain");
  if (!text) return false;
  const blocks = parsePasted(text);
  const s = $getSelection();
  if (!$isRangeSelection(s) || blocks.length === 0) return false;
  e.preventDefault();
  // One line of text goes in where the caret is, as words, not as a new paragraph.
  if (blocks.length === 1 && blocks[0].type === "paragraph") s.insertNodes(spanNodes(blocks[0].spans));
  else $insertNodes(fromBlocks(blocks));
  return true;
}
export interface KeyActions {
  askLink(): void;
  toggleList(type: "bullet" | "number" | "check"): void;
  setHeading(level: number): void;
  toggleQuote(): void;
  insertButton(): HTMLElement;
}

// editorKeys is the editor's keyboard shortcuts: a handler for KEY_DOWN_COMMAND.
export function editorKeys(editor: LexicalEditor, opts: EditorOptions, bar: KeyActions, insert: (id: InsertId, anchor: HTMLElement) => void) {
  return (e: KeyboardEvent): boolean => {
    const mod = e.ctrlKey || e.metaKey;
    if (!mod) return false;
    const k = e.key.toLowerCase();
    const run = (fn: () => void) => {
      e.preventDefault();
      fn();
      return true;
    };
    // Ctrl+End and Ctrl+Home go to the end or start of the text, also from inside a verse quote,
    // where the browser would leave the caret.
    if (!e.shiftKey && !e.altKey && (e.key === "End" || e.key === "Home")) {
      return run(() => editor.update(() => (e.key === "End" ? $getRoot().selectEnd() : $getRoot().selectStart())));
    }
    if (!e.shiftKey && !e.altKey && k === "k") return run(() => bar.askLink());
    if (e.shiftKey && !e.altKey && k === "k" && opts.onVerseRef) return run(() => insert("verseref", bar.insertButton()));
    if (!e.shiftKey && !e.altKey && k === "u") return run(() => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "underline"));
    if (!e.shiftKey && !e.altKey && k === "e") return run(() => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "code"));
    if (e.shiftKey && !e.altKey && k === "x") return run(() => bar.toggleList("check"));
    // Ctrl+Alt+1, 2 and 3 make a large, medium or small heading; Ctrl+Alt+0 plain text.
    const digit = /^Digit([0-3])$/.exec(e.code);
    if (e.altKey && !e.shiftKey && digit) return run(() => bar.setHeading(Number(digit[1])));
    if (e.shiftKey && !e.altKey && (e.code === "Digit8" || e.code === "Digit7")) return run(() => bar.toggleList(e.code === "Digit8" ? "bullet" : "number"));
    if (e.shiftKey && !e.altKey && e.code === "Digit9") return run(() => bar.toggleQuote());
    // Ctrl+] and Ctrl+[ make a heading deeper or higher (a point becomes a sub-point and back).
    if (!e.shiftKey && !e.altKey && (e.code === "BracketRight" || e.code === "BracketLeft")) {
      const key = editor.getEditorState().read(() => {
        const b = topBlock();
        return b && $isHeadingNode(b) ? b.getKey() : null;
      });
      if (!key) return false;
      return run(() => shiftHeading(editor, key, e.code === "BracketRight" ? 1 : -1));
    }
    return false;
  };
}

// tabToChip turns a reference typed just before the caret (Rom 8:28) into a chip on Tab.
export function tabToChip(editor: LexicalEditor, opts: EditorOptions, e: KeyboardEvent): boolean {
  if (!opts.parseRef || e.shiftKey) return false;
  const found = editor.getEditorState().read(() => {
    const s = $getSelection();
    if (!$isRangeSelection(s) || !s.isCollapsed()) return null;
    const node = s.anchor.getNode();
    if (!$isTextNode(node)) return null;
    const before = node.getTextContent().slice(0, s.anchor.offset);
    const cands = refCandidates(before);
    return cands.length ? { key: node.getKey(), end: s.anchor.offset, cands } : null;
  });
  if (!found) return false;
  e.preventDefault();
  void (async () => {
    for (const c of found.cands) {
      const p = await opts.parseRef!(c.text);
      if (!p) continue;
      editor.update(() => {
        const n = $getNodeByKey(found.key);
        if (!$isTextNode(n) || n.getTextContent().slice(c.start, found.end) !== c.text) return;
        const r = $createRangeSelection();
        r.anchor.set(found.key, c.start, "text");
        r.focus.set(found.key, found.end, "text");
        $setSelection(r);
        const chip = $createRefChipNode(p.display, `#passage/${encodeURIComponent(p.osis)}`);
        const space = $createTextNode(" ");
        r.insertNodes([chip, space]);
        space.select(1, 1);
      });
      return;
    }
  })();
  return true;
  return true;
}

export function dropOurs(opts: EditorOptions, e: DragEvent): boolean {
  return !!opts.onDrop && !!e.dataTransfer && [...e.dataTransfer.types].some((t) => DROP_TYPES.includes(t));
}

// dropped puts what was dragged in where it was dropped: the host turns it into Markdown.
export function dropped(editor: LexicalEditor, opts: EditorOptions, doc: Document, e: DragEvent): boolean {
  if (!dropOurs(opts, e)) return false;
  e.preventDefault();
  const data: Record<string, string> = {};
  for (const t of DROP_TYPES) {
    const v = e.dataTransfer!.getData(t);
    if (v) data[t] = v;
  }
  const range = doc.caretRangeFromPoint?.(e.clientX, e.clientY);
  if (range) {
    editor.update(() => {
      const r = $createRangeSelection();
      r.applyDOMRange(range);
      $setSelection(r);
    }, { discrete: true });
  }
  void opts.onDrop!(data).then((md) => {
    if (md) insertMarkdown(editor, md);
  });
  return true;
}
