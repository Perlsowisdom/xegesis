// Small things the editor's modules share.


import { $getSelection, $isRangeSelection, LexicalNode } from "lexical";

export const MOD = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl+";

export const DROP_TYPES = ["application/x-xegesis-verse", "application/x-xegesis-highlight", "application/x-xegesis-related"];

// topBlock is the top-level block holding the caret.
export function topBlock(): LexicalNode | null {
  const s = $getSelection();
  return $isRangeSelection(s) ? s.anchor.getNode().getTopLevelElement() : null;
}
