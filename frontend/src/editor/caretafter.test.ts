import { describe, expect, it } from "vitest";
import { $getRoot, $getSelection, $isRangeSelection, $createParagraphNode, $createTextNode, createEditor } from "lexical";
import { $caretAfterBlock, $createVerseQuoteNode, VerseQuoteNode } from "./nodes";

function make() {
  return createEditor({ nodes: [VerseQuoteNode], onError: (e) => { throw e; } });
}

describe("caret after a verse quote", () => {
  it("goes to the start of the next line", () => {
    const ed = make();
    let nextKey = "";
    ed.update(() => {
      const q = $createVerseQuoteNode(["**1** In the beginning", "— Genesis 1:1"]);
      const p = $createParagraphNode().append($createTextNode("after"));
      nextKey = p.getKey();
      $getRoot().append(q, p);
      $caretAfterBlock(q.getKey());
    }, { discrete: true });
    ed.getEditorState().read(() => {
      const s = $getSelection();
      expect($isRangeSelection(s) && s.anchor.getNode().getParentOrThrow().getKey() === nextKey || ($isRangeSelection(s) && s.anchor.key === nextKey)).toBe(true);
    });
  });
  it("adds a line when the quote is last", () => {
    const ed = make();
    ed.update(() => {
      const q = $createVerseQuoteNode(["x", "— Y"]);
      $getRoot().append(q);
      $caretAfterBlock(q.getKey());
    }, { discrete: true });
    ed.getEditorState().read(() => {
      expect($getRoot().getChildrenSize()).toBe(2);
      expect($getRoot().getLastChild()?.getType()).toBe("paragraph");
      expect($isRangeSelection($getSelection())).toBe(true);
    });
  });
});
