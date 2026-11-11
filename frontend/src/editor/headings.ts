
import { $getNodeByKey, $getRoot, $isElementNode, $isParagraphNode, LexicalEditor } from "lexical";
import { $createHeadingNode, $isHeadingNode } from "@lexical/rich-text";
import { topBlock } from "./common";
import { type HeadingInfo } from "./types";

// The headings as an outline: reading them, moving a section, changing a level, revealing one.

export function headingsOf(editor: LexicalEditor): HeadingInfo[] {
  return editor.getEditorState().read(() => $getRoot().getChildren().filter($isHeadingNode).map((n) => ({
    key: n.getKey(), level: Number(n.getTag().slice(1)), text: n.getTextContent(),
  })));
}

export function shiftHeading(editor: LexicalEditor, key: string, by: number) {
  const cur = headingsOf(editor).find((x) => x.key === key);
  if (cur) setHeadingLevel(editor, key, Math.min(3, Math.max(1, cur.level + by)));
}

export function setHeadingLevel(editor: LexicalEditor, key: string, level: number) {
  editor.update(() => {
    const n = $getNodeByKey(key);
    if (!n || !$isHeadingNode(n) || n.getTag() === `h${level}`) return;
    const next = $createHeadingNode(`h${level}` as "h2");
    next.append(...n.getChildren());
    n.replace(next);
    next.selectEnd();
  });
}

export function moveSection(editor: LexicalEditor, key: string, beforeKey: string | null) {
  editor.update(() => {
    const kids = $getRoot().getChildren();
    const levels = kids.map((n) => ($isHeadingNode(n) ? Number(n.getTag().slice(1)) : 0));
    const from = kids.findIndex((n) => n.getKey() === key);
    if (from < 0 || !levels[from]) return;
    let end = from + 1;
    while (end < kids.length && !(levels[end] && levels[end] <= levels[from])) end++;
    const target = beforeKey ? kids.find((n) => n.getKey() === beforeKey) ?? null : null;
    const ti = target ? kids.indexOf(target) : kids.length;
    if (ti >= from && ti <= end) return;
    const moving = kids.slice(from, end);
    if (target) for (const n of moving) target.insertBefore(n);
    else for (const n of moving) $getRoot().append(n);
  });
}

export function addHeading(editor: LexicalEditor, level = 2) {
  editor.update(() => {
    const root = $getRoot();
    const last = root.getLastChild();
    const hd = $createHeadingNode(`h${level}` as "h2");
    if (last && $isParagraphNode(last) && last.getTextContentSize() === 0) last.replace(hd);
    else root.append(hd);
    hd.select();
  });
  editor.focus();
}

export function revealHeading(editor: LexicalEditor, key: string) {
  const el = editor.getElementByKey(key);
  if (!el) return;
  // The caret goes there first, then the heading is scrolled to the top (putting the caret in
  // the text scrolls on its own, which would undo the scroll the other way round).
  editor.update(() => {
    const n = $getNodeByKey(key);
    if (n && $isElementNode(n)) n.selectEnd();
  }, { discrete: true });
  editor.focus();
  requestAnimationFrame(() => el.scrollIntoView({ block: "start", behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" }));
  el.classList.remove("rt-flash");
  void el.offsetWidth;
  el.classList.add("rt-flash");
}

export function caretHeading(editor: LexicalEditor): string | null {
  return editor.getEditorState().read(() => {
    let b = topBlock();
    while (b && !$isHeadingNode(b)) b = b.getPreviousSibling();
    return b ? b.getKey() : null;
  });
}
