// Insert: a verse reference, verse text, a highlight or a link to a note, put in at the caret.


import { $createParagraphNode, $createTextNode, $getRoot, $getSelection, $isParagraphNode, $isRangeSelection, LexicalEditor, LexicalNode } from "lexical";
import { parseMarkdown } from "./markdown";
import { $createDividerNode, $createRefChipNode } from "./nodes";
import { MOD, topBlock } from "./common";
import { type EditorOptions, type InsertId } from "./types";
import { fromBlocks } from "./convert";

// The things the Insert control and the slash menu put in, each when the host can do it.
export const INSERTS: { id: InsertId; label: string; hint: string; glyph: string; keys?: string }[] = [
  { id: "verseref", label: "Verse reference", hint: "A chip that opens the passage", glyph: "✦", keys: `${MOD}Shift+K` },
  { id: "versequote", label: "Verse text", hint: "The verses, with the Bible's credit line", glyph: "❝" },
  { id: "highlight", label: "Highlight", hint: "Words you highlighted, with where they are", glyph: "▍" },
  { id: "notelink", label: "Link to a note or sermon", hint: "A chip that opens it", glyph: "¶" },
];

export function insertsOf(opts: EditorOptions) {
  const can: Record<InsertId, boolean> = {
    verseref: !!opts.onVerseRef, versequote: !!opts.onVerseQuote, highlight: !!opts.onHighlight, notelink: !!opts.onNoteLink,
  };
  return INSERTS.filter((it) => can[it.id]);
}


// insertDivider puts a divider line after the caret's block (in place of it when it is empty).
export function insertDivider(editor: LexicalEditor) {
  editor.update(() => {
    const b = topBlock();
    const d = $createDividerNode();
    if (!b) return;
    if ($isParagraphNode(b) && b.getTextContentSize() === 0) {
      b.insertBefore(d);
      b.selectStart();
    } else {
      b.insertAfter(d);
      const p = $createParagraphNode();
      d.insertAfter(p);
      p.select();
    }
  });
}

// insertChip puts a reference chip where the caret is (in place of any selected words).
export function insertChip(editor: LexicalEditor, label: string, href: string) {
  editor.update(() => {
    let s = $getSelection();
    if (!$isRangeSelection(s)) {
      $getRoot().selectEnd();
      s = $getSelection();
    }
    if (!$isRangeSelection(s)) return;
    const chip = $createRefChipNode(label, href);
    const space = $createTextNode(" ");
    s.insertNodes([chip, space]);
    space.select(1, 1);
  });
  editor.focus();
}

// insertMarkdown puts blocks after the block with the caret (replacing it when it is empty).
export function insertMarkdown(editor: LexicalEditor, md: string) {
  editor.update(() => {
    const nodes = fromBlocks(parseMarkdown(md));
    if (nodes.length === 0) return;
    const at = topBlock();
    const root = $getRoot();
    if (at) {
      let prev: LexicalNode = at;
      for (const n of nodes) {
        prev.insertAfter(n);
        prev = n;
      }
      if ($isParagraphNode(at) && at.getTextContentSize() === 0) at.remove();
    } else {
      for (const n of nodes) root.append(n);
    }
    // Leave a line to go on writing under what was put in.
    const last = nodes[nodes.length - 1];
    const after = last.getNextSibling();
    if (after && $isParagraphNode(after)) after.selectStart();
    else {
      const p = $createParagraphNode();
      last.insertAfter(p);
      p.select();
    }
  });
  editor.focus();
}

export function runInsert(editor: LexicalEditor, opts: EditorOptions, id: InsertId, anchor: HTMLElement) {
  switch (id) {
    case "verseref": {
      const selected = editor.getEditorState().read(() => {
        const s = $getSelection();
        return $isRangeSelection(s) ? s.getTextContent() : "";
      });
      opts.onVerseRef?.(anchor, selected.trim(), (label, ref) => insertChip(editor, label, `#passage/${encodeURIComponent(ref)}`));
      break;
    }
    case "versequote":
      opts.onVerseQuote?.(anchor);
      break;
    case "highlight":
      opts.onHighlight?.(anchor, (md) => insertMarkdown(editor, md));
      break;
    case "notelink":
      opts.onNoteLink?.(anchor, (label, href) => insertChip(editor, label, href));
      break;
  }
}
