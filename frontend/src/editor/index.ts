// The rich-text editor for notes and sermons, built on Lexical, in plain TypeScript. People type
// formatted text directly; what is stored is Markdown (markdown.ts), so writing stays in an open
// format and export and import round-trip exactly.
//
//   const ed = createEditor(el, { markdown, onChange, placeholder });
//   ed.getMarkdown(); ed.setMarkdown(md); ed.focus(); ed.destroy();
//
// It draws a slim toolbar of icon buttons (each with a tooltip and a shortcut) and a slash menu:
// typing "/" offers headings, lists, quotes, a verse quotation and a divider. Hosts add their own
// toolbar buttons with `tools`, and handle the verse buttons (which need the app's commands) with
// `onVerseQuote` and `onVerseRef`. For sermons it also reads the headings as an outline, moves a
// heading's section and changes a heading's level.

import { $createParagraphNode, $getRoot, $isElementNode, $isParagraphNode, COMMAND_PRIORITY_HIGH, COMMAND_PRIORITY_LOW, createEditor as createLexical, DRAGOVER_COMMAND, DROP_COMMAND, KEY_BACKSPACE_COMMAND, KEY_DOWN_COMMAND, KEY_TAB_COMMAND, PASTE_COMMAND, SELECTION_CHANGE_COMMAND } from "lexical";
import { createEmptyHistoryState, registerHistory } from "@lexical/history";
import { followsLink } from "./linkclick";
import { $toggleLink, LinkNode, TOGGLE_LINK_COMMAND } from "@lexical/link";
import { ListItemNode, ListNode, registerCheckList, registerList } from "@lexical/list";
import { BOLD_ITALIC_STAR, BOLD_STAR, CHECK_LIST, HEADING, INLINE_CODE, ITALIC_STAR, ITALIC_UNDERSCORE, LINK, ORDERED_LIST, QUOTE, registerMarkdownShortcuts, UNORDERED_LIST } from "@lexical/markdown";
import { $isHeadingNode, HeadingNode, QuoteNode, registerRichText } from "@lexical/rich-text";
import { $isTableNode, TableCellNode, TableNode, TableRowNode } from "@lexical/table";
import { parseMarkdown, toMarkdown } from "./markdown";
import { DividerNode, RefChipNode, VerseQuoteNode } from "./nodes";
import { InkNode } from "./inknode";
import { registerTables } from "./table";
import { structure } from "./structure";
import { topBlock } from "./common";
import { type EditorOptions, type RichEditor } from "./types";
import { fromBlocks, toBlocks } from "./convert";
import { toolbar } from "./toolbar";
import { slashMenu } from "./slash";
import { insertMarkdown, runInsert } from "./insert";
import { $plainAtStart, DIVIDER, dropOurs, dropped, editorKeys, pasteMarkdown, tabToChip, UNDERLINE } from "./typing";
import { addHeading, caretHeading, headingsOf, moveSection, revealHeading, setHeadingLevel } from "./headings";

export type { EditorOptions, HeadingInfo, InsertId, RichEditor } from "./types";
export { fromBlocks, toBlocks } from "./convert";

export function createEditor(host: HTMLElement, opts: EditorOptions = {}): RichEditor {
  const editor = createLexical({
    namespace: "xegesis-notes",
    nodes: [HeadingNode, QuoteNode, ListNode, ListItemNode, LinkNode, DividerNode, InkNode, VerseQuoteNode, RefChipNode, TableNode, TableRowNode, TableCellNode,
      ...(opts.nodes ?? [])],
    theme: {
      text: { underline: "rt-u", bold: "rt-b", italic: "rt-i", code: "rt-code" }, link: "rt-link",
      list: { checklist: "rt-checklist", listitemChecked: "rt-checked", listitemUnchecked: "rt-unchecked" },
      table: "rt-table", tableCell: "rt-cell", tableCellHeader: "rt-cell-head", tableRow: "rt-row", tableScrollableWrapper: "rt-table-wrap",
    },
    onError: (e) => {
      throw e;
    },
  });
  const doc = host.ownerDocument;
  const variant = opts.variant ?? "page";
  const content = doc.createElement("div");
  content.className = "rich-text";
  content.contentEditable = "true";
  content.spellcheck = true;
  content.setAttribute("role", "textbox");
  content.setAttribute("aria-multiline", "true");
  content.setAttribute("aria-label", opts.label ?? "Text");
  if (opts.placeholder) content.dataset.placeholder = opts.placeholder;
  const bar = toolbar(doc, editor, opts, variant, (id, anchor) => runInsert(editor, opts, id, anchor));
  host.classList.add("rich-editor", `rich-${variant}`);
  host.replaceChildren(bar.el, content);
  editor.setRootElement(content);
  // The toolbar sticks to the top: what is jumped to, and the outline strip, keep below it (style.css).
  const barSize = new ResizeObserver(() => host.style.setProperty("--rt-bar", `${bar.el.offsetHeight}px`));
  barSize.observe(bar.el);
  const slash = slashMenu(doc, editor, content, opts, (id, anchor) => runInsert(editor, opts, id, anchor), bar);
  const struct = structure({
    editor, host, content, strip: variant === "page",
    headings: () => headingsOf(editor),
    blockKeys: () => editor.getEditorState().read(() => $getRoot().getChildren().map((n) => ({
      key: n.getKey(), level: $isHeadingNode(n) ? Number(n.getTag().slice(1)) : 0 }))),
    reveal: (key) => revealHeading(editor, key),
    caretBlock: () => editor.getEditorState().read(() => topBlock()?.getKey() ?? null),
  });

  let source = opts.markdown ?? "";
  let dirty = false;
  let loading = false;
  let headingSig = "";

  load(source);

  const keys = editorKeys(editor, opts, bar, (id, anchor) => runInsert(editor, opts, id, anchor));
  const off = [
    registerRichText(editor),
    registerList(editor),
    registerCheckList(editor),
    editor.registerCommand(TOGGLE_LINK_COMMAND, (url) => {
      $toggleLink(typeof url === "string" && url ? url : null);
      return true;
    }, COMMAND_PRIORITY_LOW),
    registerHistory(editor, createEmptyHistoryState(), 300),
    registerMarkdownShortcuts(editor, [DIVIDER, HEADING, QUOTE, CHECK_LIST, UNORDERED_LIST, ORDERED_LIST,
      BOLD_ITALIC_STAR, BOLD_STAR, UNDERLINE, ITALIC_STAR, ITALIC_UNDERSCORE, INLINE_CODE, LINK]),
    editor.registerCommand(KEY_BACKSPACE_COMMAND, (e) => {
      if (!$plainAtStart()) return false;
      e?.preventDefault();
      return true;
    }, COMMAND_PRIORITY_HIGH),
    editor.registerCommand(PASTE_COMMAND, (e) => pasteMarkdown(e), COMMAND_PRIORITY_HIGH),
    editor.registerUpdateListener(({ dirtyElements, dirtyLeaves, tags }) => {
      syncEmpty();
      struct.apply();
      slash.check();
      const sig = JSON.stringify(headingsOf(editor).map((x) => [x.key, x.level, x.text]));
      if (sig !== headingSig) {
        headingSig = sig;
        opts.onHeadings?.();
      }
      if (loading || tags.has("history-merge") || (dirtyElements.size === 0 && dirtyLeaves.size === 0)) return;
      dirty = true;
      opts.onChange?.(getMarkdown());
    }),
    editor.registerCommand(SELECTION_CHANGE_COMMAND, () => {
      bar.sync();
      struct.caretMoved();
      return false;
    }, COMMAND_PRIORITY_LOW),
    editor.registerCommand(KEY_DOWN_COMMAND, keys, COMMAND_PRIORITY_LOW),
    editor.registerCommand(KEY_TAB_COMMAND, (e) => tabToChip(editor, opts, e), COMMAND_PRIORITY_LOW),
    registerTables(editor, content, (e) => tabToChip(editor, opts, e)),
    editor.registerCommand(DRAGOVER_COMMAND, (e) => {
      if (!dropOurs(opts, e)) return false;
      e.preventDefault();
      if (e.dataTransfer) e.dataTransfer.dropEffect = "copy";
      return true;
    }, COMMAND_PRIORITY_HIGH),
    editor.registerCommand(DROP_COMMAND, (e) => dropped(editor, opts, doc, e), COMMAND_PRIORITY_HIGH),
  ];
  // A chip or link opens what it names when clicked; with text selected, or Shift/Alt held, a click places the caret.
  const open = (href: string, label?: string) => (opts.onOpenLink ? opts.onOpenLink(href, label) : (location.hash = href));
  const onClick = (e: MouseEvent) => {
    const chip = (e.target as Element).closest?.<HTMLElement>(".ref-chip");
    if (chip?.dataset.href) {
      e.preventDefault();
      open(chip.dataset.href, chip.dataset.label);
      return;
    }
    const a = (e.target as Element).closest?.("a[href]");
    const href = a?.getAttribute("href");
    if (!a || !href) return;
    const sel = window.getSelection();
    const plain = followsLink({ button: e.button, shift: e.shiftKey, alt: e.altKey, hadSelection, hasSelection: !!sel && !sel.isCollapsed });
    if (!plain && !(e.ctrlKey || e.metaKey)) return;
    e.preventDefault();
    if (href.startsWith("#")) open(href);
    else if (/^(https?:|mailto:)/i.test(href)) {
      const rt = (window as unknown as { runtime?: { BrowserOpenURL?: (u: string) => void } }).runtime;
      if (rt?.BrowserOpenURL) rt.BrowserOpenURL(href);
      else window.open(href, "_blank", "noopener");
    }
  };
  let hadSelection = false;
  content.addEventListener("mousedown", () => {
    const s = window.getSelection();
    hadSelection = !!s && !s.isCollapsed;
  }, true);
  content.addEventListener("click", onClick);
  const extra = opts.setup?.(editor);
  syncEmpty();
  struct.setKey(opts.foldKey ?? "");

  // syncEmpty shows the placeholder while the text is one empty paragraph.
  function syncEmpty() {
    editor.getEditorState().read(() => {
      const root = $getRoot();
      const first = root.getFirstChild();
      content.toggleAttribute("data-empty", root.getChildrenSize() === 1 && !!first && $isParagraphNode(first) && first.getTextContentSize() === 0);
      bar.sync();
    });
  }


  function load(md: string) {
    loading = true;
    editor.update(() => {
      const root = $getRoot();
      root.clear();
      fromBlocks(parseMarkdown(md)).forEach((n) => root.append(n));
      // Text that ends with a block the caret cannot write in (a verse quote, a divider, a table)
      // gets a line under it to go on writing, as after inserting one. An empty line is not saved.
      const end = root.getLastChild();
      if (!end || !$isElementNode(end) || $isTableNode(end)) root.append($createParagraphNode());
    }, { discrete: true, tag: "history-merge" });
    loading = false;
  }

  function getMarkdown(): string {
    // Text that was never edited comes back exactly as it was given.
    if (!dirty) return source;
    return editor.getEditorState().read(() => toMarkdown(toBlocks($getRoot().getChildren())));
  }

  return {
    lexical: editor,
    toolbar: bar.el,
    getMarkdown,
    setMarkdown(md) {
      source = md;
      dirty = false;
      load(md);
    },
    focus: () => editor.focus(),
    insertMarkdown: (md) => insertMarkdown(editor, md),
    headings: () => headingsOf(editor),
    headingElement: (key) => editor.getElementByKey(key),
    revealHeading: (key) => revealHeading(editor, key),
    setFoldKey: (key) => struct.setKey(key),
    moveSection: (key, before) => moveSection(editor, key, before),
    setHeadingLevel: (key, level) => setHeadingLevel(editor, key, level),
    addHeading: (level) => addHeading(editor, level),
    caretHeading: () => caretHeading(editor),
    destroy() {
      off.forEach((f) => f());
      barSize.disconnect();
      bar.destroy();
      if (typeof extra === "function") extra();
      slash.destroy();
      struct.destroy();
      content.removeEventListener("click", onClick);
      editor.setRootElement(null);
      host.replaceChildren();
      host.classList.remove("rich-editor", `rich-${variant}`);
    },
  };
}
