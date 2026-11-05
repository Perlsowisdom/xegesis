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

import {
  $createLineBreakNode,
  $createParagraphNode,
  $createRangeSelection,
  $createTextNode,
  $getNodeByKey,
  $getRoot,
  $getSelection,
  $insertNodes,
  $isElementNode,
  $isLineBreakNode,
  $isParagraphNode,
  $isRangeSelection,
  $isTextNode,
  $setSelection,
  COMMAND_PRIORITY_HIGH,
  COMMAND_PRIORITY_LOW,
  ElementNode,
  FORMAT_TEXT_COMMAND,
  KEY_ARROW_DOWN_COMMAND,
  KEY_ARROW_UP_COMMAND,
  KEY_BACKSPACE_COMMAND,
  KEY_DOWN_COMMAND,
  KEY_ENTER_COMMAND,
  KEY_ESCAPE_COMMAND,
  KEY_TAB_COMMAND,
  DRAGOVER_COMMAND,
  DROP_COMMAND,
  LexicalEditor,
  LexicalNode,
  PASTE_COMMAND,
  SELECTION_CHANGE_COMMAND,
  createEditor as createLexical,
  type Klass,
} from "lexical";
import { createEmptyHistoryState, registerHistory } from "@lexical/history";
import { followsLink } from "./linkclick";
import { $createLinkNode, $isLinkNode, LinkNode, TOGGLE_LINK_COMMAND, $toggleLink } from "@lexical/link";
import {
  $createListItemNode, $createListNode, $isListItemNode, $isListNode, INSERT_CHECK_LIST_COMMAND, INSERT_ORDERED_LIST_COMMAND, INSERT_UNORDERED_LIST_COMMAND,
  ListItemNode, ListNode, REMOVE_LIST_COMMAND, registerCheckList, registerList,
} from "@lexical/list";
import {
  BOLD_ITALIC_STAR, BOLD_STAR, CHECK_LIST, HEADING, INLINE_CODE, ITALIC_STAR,
  ITALIC_UNDERSCORE, LINK, ORDERED_LIST, QUOTE, UNORDERED_LIST, registerMarkdownShortcuts, type ElementTransformer,
  type TextFormatTransformer,
} from "@lexical/markdown";
import { $findMatchingParent } from "@lexical/utils";
import {
  $createHeadingNode, $createQuoteNode, $isHeadingNode, $isQuoteNode, HeadingNode, QuoteNode, registerRichText,
} from "@lexical/rich-text";
import { $setBlocksType } from "@lexical/selection";
import {
  $createTableCellNode, $createTableNode, $createTableRowNode, $isTableCellNode, $isTableNode, $isTableRowNode, TableCellHeaderStates,
  TableCellNode, TableNode, TableRowNode,
} from "@lexical/table";
import { h } from "../dom";
import { icon } from "../icons";
import { closePopover, openPopover } from "../popover";
import { Block, Span, parseMarkdown, parsePasted, toMarkdown } from "./markdown";
import {
  $createDividerNode, $createRefChipNode, $createVerseQuoteNode, $isDividerNode, $isRefChipNode, $isVerseQuoteNode, DividerNode,
  RefChipNode, VerseQuoteNode, chipKind,
} from "./nodes";
import { refCandidates } from "./refwords";
import { SLASH_ITEMS, SlashId, filterSlash, slashQuery, tableSize } from "./slashwords";
import { inTable, insertTable, registerTables, tableMenu, tablePicker } from "./table";
import { structure } from "./structure";

export interface HeadingInfo {
  key: string;
  level: number; // 1 to 6, as in Markdown
  text: string;
}

export interface EditorOptions {
  markdown?: string;
  onChange?(markdown: string): void;
  placeholder?: string;
  label?: string; // accessible name of the text area
  variant?: "page" | "compact"; // the full page, or the sidebar's smaller one
  tools?: HTMLElement[]; // extra toolbar buttons, after the built-in ones
  // The verse buttons need the app: a verse quotation goes through document.quote, a reference is
  // checked with verse.parse. Each is given where to show its popover.
  onVerseQuote?(anchor: HTMLElement): void;
  onVerseRef?(anchor: HTMLElement, selected: string, insert: (label: string, ref: string) => void): void;
  // Insert → Highlight: a picker of highlights; insert takes the quotation's Markdown.
  onHighlight?(anchor: HTMLElement, insert: (md: string) => void): void;
  // Insert → Note link: a picker of notes and sermons; insert takes the title and its address.
  onNoteLink?(anchor: HTMLElement, insert: (label: string, href: string) => void): void;
  // parseRef checks a typed reference (Rom 8:28 then Tab becomes a chip).
  parseRef?(text: string): Promise<{ display: string; osis: string } | null>;
  // A verse, a highlight or a Related card dropped on the text: the Markdown to put there.
  onDrop?(data: Record<string, string>): Promise<string | null>;
  // A chip or app link was clicked; by default the app goes to its address.
  onOpenLink?(href: string, label?: string): void;
  onHeadings?(): void; // the headings changed
  // Folded sections are remembered under this key (the note's or sermon's id) on this device.
  foldKey?: string;
  nodes?: Klass<LexicalNode>[];
  setup?(editor: LexicalEditor): void | (() => void);
}

export interface RichEditor {
  getMarkdown(): string;
  setMarkdown(markdown: string): void;
  focus(): void;
  destroy(): void;
  lexical: LexicalEditor;
  toolbar: HTMLElement;
  // insertMarkdown puts blocks after the block with the caret (replacing it when it is empty).
  insertMarkdown(md: string): void;
  headings(): HeadingInfo[];
  headingElement(key: string): HTMLElement | null;
  revealHeading(key: string): void;
  moveSection(key: string, beforeKey: string | null): void;
  setHeadingLevel(key: string, level: number): void;
  addHeading(level?: number): void;
  caretHeading(): string | null; // the heading the caret is in or under
  setFoldKey(key: string): void; // once a new note has an id
}

const MOD = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl+";

export function createEditor(host: HTMLElement, opts: EditorOptions = {}): RichEditor {
  const editor = createLexical({
    namespace: "xegesis-notes",
    nodes: [HeadingNode, QuoteNode, ListNode, ListItemNode, LinkNode, DividerNode, VerseQuoteNode, RefChipNode, TableNode, TableRowNode, TableCellNode,
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
  const bar = toolbar(doc, editor, opts, variant, (id, anchor) => runInsert(id, anchor));
  host.classList.add("rich-editor", `rich-${variant}`);
  host.replaceChildren(bar.el, content);
  editor.setRootElement(content);
  // The toolbar sticks to the top: what is jumped to, and the outline strip, keep below it (style.css).
  const barSize = new ResizeObserver(() => host.style.setProperty("--rt-bar", `${bar.el.offsetHeight}px`));
  barSize.observe(bar.el);
  const slash = slashMenu(doc, editor, content, opts, (id, anchor) => runInsert(id, anchor), bar);
  const struct = structure({
    editor, host, content, strip: variant === "page",
    headings: () => headings(),
    blockKeys: () => editor.getEditorState().read(() => $getRoot().getChildren().map((n) => ({
      key: n.getKey(), level: $isHeadingNode(n) ? Number(n.getTag().slice(1)) : 0 }))),
    reveal: (key) => reveal(key),
    caretBlock: () => editor.getEditorState().read(() => topBlock()?.getKey() ?? null),
  });

  let source = opts.markdown ?? "";
  let dirty = false;
  let loading = false;
  let headingSig = "";

  load(source);

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
      const sig = JSON.stringify(headings().map((x) => [x.key, x.level, x.text]));
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
    editor.registerCommand(KEY_DOWN_COMMAND, (e: KeyboardEvent) => shortcut(e), COMMAND_PRIORITY_LOW),
    editor.registerCommand(KEY_TAB_COMMAND, (e) => tabToChip(e), COMMAND_PRIORITY_LOW),
    registerTables(editor, content, (e) => tabToChip(e)),
    editor.registerCommand(DRAGOVER_COMMAND, (e) => {
      if (!ours(e)) return false;
      e.preventDefault();
      if (e.dataTransfer) e.dataTransfer.dropEffect = "copy";
      return true;
    }, COMMAND_PRIORITY_HIGH),
    editor.registerCommand(DROP_COMMAND, (e) => dropped(e), COMMAND_PRIORITY_HIGH),
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

  function shortcut(e: KeyboardEvent): boolean {
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
    if (e.shiftKey && !e.altKey && k === "k" && opts.onVerseRef) return run(() => runInsert("verseref", bar.insertButton()));
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
      return run(() => shiftHeading(key, e.code === "BracketRight" ? 1 : -1));
    }
    return false;
  }

  // Insert ----------------------------------------------------------------------------------------

  function runInsert(id: InsertId, anchor: HTMLElement) {
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
        opts.onHighlight?.(anchor, (md) => insertMarkdown(md));
        break;
      case "notelink":
        opts.onNoteLink?.(anchor, (label, href) => insertChip(editor, label, href));
        break;
    }
  }

  // tabToChip turns a reference typed just before the caret (Rom 8:28) into a chip on Tab.
  function tabToChip(e: KeyboardEvent): boolean {
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
  }

  function ours(e: DragEvent): boolean {
    return !!opts.onDrop && !!e.dataTransfer && [...e.dataTransfer.types].some((t) => DROP_TYPES.includes(t));
  }

  // dropped puts what was dragged in where it was dropped: the host turns it into Markdown.
  function dropped(e: DragEvent): boolean {
    if (!ours(e)) return false;
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
      if (md) insertMarkdown(md);
    });
    return true;
  }

  function shiftHeading(key: string, by: number) {
    const cur = headings().find((x) => x.key === key);
    if (cur) setHeadingLevel(key, Math.min(3, Math.max(1, cur.level + by)));
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

  function headings(): HeadingInfo[] {
    return editor.getEditorState().read(() => $getRoot().getChildren().filter($isHeadingNode).map((n) => ({
      key: n.getKey(), level: Number(n.getTag().slice(1)), text: n.getTextContent(),
    })));
  }

  function setHeadingLevel(key: string, level: number) {
    editor.update(() => {
      const n = $getNodeByKey(key);
      if (!n || !$isHeadingNode(n) || n.getTag() === `h${level}`) return;
      const next = $createHeadingNode(`h${level}` as "h2");
      next.append(...n.getChildren());
      n.replace(next);
      next.selectEnd();
    });
  }

  function moveSection(key: string, beforeKey: string | null) {
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

  function addHeading(level = 2) {
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

  function insertMarkdown(md: string) {
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

  function reveal(key: string) {
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

  function caretHeading(): string | null {
    return editor.getEditorState().read(() => {
      let b = topBlock();
      while (b && !$isHeadingNode(b)) b = b.getPreviousSibling();
      return b ? b.getKey() : null;
    });
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
    insertMarkdown,
    headings,
    headingElement: (key) => editor.getElementByKey(key),
    revealHeading: (key) => reveal(key),
    setFoldKey: (key) => struct.setKey(key),
    moveSection,
    setHeadingLevel,
    addHeading,
    caretHeading,
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

// topBlock is the top-level block holding the caret.
function topBlock(): LexicalNode | null {
  const s = $getSelection();
  return $isRangeSelection(s) ? s.anchor.getNode().getTopLevelElement() : null;
}

// Markdown as you type ----------------------------------------------------------------------------

// UNDERLINE: __text__ underlines (not bold, as in some Markdown); it is stored as <u>text</u>.
const UNDERLINE: TextFormatTransformer = { format: ["underline"], intraword: false, tag: "__", type: "text-format" };

// DIVIDER turns a line of "---" (or "***") into a divider when Enter or a space is typed after it.
const DIVIDER: ElementTransformer = {
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
function $plainAtStart(): boolean {
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
function pasteMarkdown(e: ClipboardEvent | KeyboardEvent | InputEvent): boolean {
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

// Lexical <-> blocks -----------------------------------------------------------------------------

// plainLink is a link run of one unformatted piece, which shows as a chip when it names a place in
// the app (verses, a note or a sermon).
function chipAt(spans: Span[], i: number): boolean {
  const s = spans[i];
  if (!s.link || !chipKind(s.link) || s.bold || s.italic || s.underline || s.code || !s.text.trim()) return false;
  return spans[i - 1]?.link !== s.link && spans[i + 1]?.link !== s.link;
}

function spanNodes(spans: Span[]): LexicalNode[] {
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

// Toolbar ------------------------------------------------------------------------------------------

type Tool = {
  id: string; label: string; keys?: string; glyph?: string; icon?: string; cls?: string; menu?: boolean;
  run(anchor: HTMLElement): void; active?(): boolean;
};

function toolbar(doc: Document, editor: LexicalEditor, opts: EditorOptions, variant: "page" | "compact",
  insert: (id: InsertId, anchor: HTMLElement) => void) {
  const el = doc.createElement("div");
  el.className = "rich-toolbar";
  el.setAttribute("role", "toolbar");
  el.setAttribute("aria-label", "Formatting");
  const buttons: { b: HTMLButtonElement; active?: () => boolean }[] = [];

  const sel = () => {
    const s = $getSelection();
    return $isRangeSelection(s) ? s : null;
  };
  const isFormat = (f: "bold" | "italic" | "underline" | "code") => () => !!sel()?.hasFormat(f);
  const isHeading = () => {
    const b = topBlock();
    return !!b && $isHeadingNode(b);
  };
  const isList = (type: string) => () => {
    const b = topBlock();
    return !!b && $isListNode(b) && b.getListType() === type;
  };
  const isQuote = () => {
    const b = topBlock();
    return !!b && $isQuoteNode(b);
  };
  const linkUrl = () => {
    const p = sel()?.anchor.getNode().getParent();
    return p && $isLinkNode(p) ? p.getURL() : null;
  };

  // setHeading makes the caret's block a heading of a size, 0 being plain text. Choosing the size
  // it already has makes it plain text again.
  function setHeading(level: number) {
    editor.update(() => {
      const s = sel();
      if (!s) return;
      const b = topBlock();
      const same = !level || (b && $isHeadingNode(b) && b.getTag() === `h${level}`);
      $setBlocksType(s, () => (same ? $createParagraphNode() : $createHeadingNode(`h${level}` as "h1")));
    });
  }
  const headingLevel = () => {
    const b = topBlock();
    return b && $isHeadingNode(b) ? Number(b.getTag().slice(1)) : 0;
  };

  // The heading button offers the three sizes and plain text.
  function askHeading(anchor: HTMLElement) {
    const now = editor.getEditorState().read(headingLevel);
    const sizes: [number, string, string, string][] = [
      [1, "Large heading", "h-large", `${MOD}Alt+1`], [2, "Medium heading", "h-medium", `${MOD}Alt+2`],
      [3, "Small heading", "h-small", `${MOD}Alt+3`], [0, "Normal text", "h-normal", `${MOD}Alt+0`],
    ];
    const list = h("div", { class: "menu-list heading-menu" }, sizes.map(([lv, name, cls, keys]) => {
      const b = h("button", { type: "button", class: `${cls}${lv === now ? " on" : ""}`, "aria-checked": String(lv === now), "data-level": String(lv) },
        h("span", { class: "hm-name" }, name), h("kbd", { class: "hm-keys" }, keys));
      b.addEventListener("mousedown", (e) => e.preventDefault());
      b.addEventListener("click", () => {
        closePopover();
        if (lv !== now) setHeading(lv);
        editor.focus();
      });
      return b;
    }));
    openPopover(anchor, list, { label: "Heading size", menu: true, side: "below" });
    list.querySelector<HTMLElement>(".on, button")?.focus();
  }
  function toggleList(type: "bullet" | "number" | "check") {
    const on = editor.getEditorState().read(isList(type));
    editor.dispatchCommand(on ? REMOVE_LIST_COMMAND
      : type === "bullet" ? INSERT_UNORDERED_LIST_COMMAND : type === "number" ? INSERT_ORDERED_LIST_COMMAND : INSERT_CHECK_LIST_COMMAND, undefined);
  }
  function toggleQuote() {
    editor.update(() => {
      const s = sel();
      if (!s) return;
      const b = topBlock();
      $setBlocksType(s, () => (b && $isQuoteNode(b) ? $createParagraphNode() : $createQuoteNode()));
    });
  }

  // The buttons, in groups: text formats, block kinds, then what is put in.
  const tools: (Tool | "|")[] = [
    { id: "bold", label: "Bold", keys: `${MOD}B`, glyph: "B", cls: "t-bold", run: () => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "bold"), active: isFormat("bold") },
    { id: "italic", label: "Italic", keys: `${MOD}I`, glyph: "I", cls: "t-italic", run: () => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "italic"), active: isFormat("italic") },
    { id: "underline", label: "Underline", keys: `${MOD}U`, glyph: "U", cls: "t-underline", run: () => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "underline"), active: isFormat("underline") },
    { id: "code", label: "Code", keys: `${MOD}E`, icon: "code", run: () => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "code"), active: isFormat("code") },
    "|",
    { id: "heading", label: "Heading size", keys: `${MOD}Alt+1, 2 or 3`, glyph: "H", cls: "t-heading", menu: true, run: (a) => askHeading(a), active: isHeading },
    "|",
    { id: "bullets", label: "Bulleted list", keys: `${MOD}Shift+8`, icon: "bullets", run: () => toggleList("bullet"), active: isList("bullet") },
    { id: "numbers", label: "Numbered list", keys: `${MOD}Shift+7`, icon: "numbers", run: () => toggleList("number"), active: isList("number") },
    { id: "checks", label: "Checkbox list", keys: `${MOD}Shift+X`, icon: "checklist", run: () => toggleList("check"), active: isList("check") },
    "|",
    { id: "quote", label: "Quote", keys: `${MOD}Shift+9`, icon: "quote", run: () => toggleQuote(), active: isQuote },
    { id: "divider", label: "Divider", icon: "divider", run: () => insertDivider(editor) },
    { id: "link", label: "Link", keys: `${MOD}K`, icon: "link", menu: true, run: (a) => askLink(a), active: () => { const u = linkUrl(); return !!u && !u.startsWith("#"); } },
    { id: "table", label: "Table", icon: "table", menu: true, run: (a) => askTable(a) },
    ...(insertsOf(opts).length ? [{ id: "insert", label: "Insert a verse, a highlight or a link to a note", icon: "book-plus", cls: "t-insert", menu: true, run: (a: HTMLElement) => askInsert(a) } as Tool] : []),
  ];

  const sep = () => Object.assign(doc.createElement("span"), { className: "rich-sep" });
  const byId = new Map<string, Tool>();
  for (const t of tools) {
    if (t === "|") {
      el.append(sep());
      continue;
    }
    byId.set(t.id, t);
    const b = doc.createElement("button");
    b.type = "button";
    b.className = `rich-btn ${t.cls ?? ""}`;
    b.dataset.tool = t.id;
    b.title = t.keys ? `${t.label} (${t.keys})` : t.label;
    b.setAttribute("aria-label", t.label);
    if (t.keys) b.setAttribute("aria-keyshortcuts", t.keys.replace("⌘", "Meta+").replace("Ctrl+", "Control+"));
    if (t.icon) b.append(icon(t.icon));
    else b.textContent = t.glyph ?? "";
    if (t.active && t.id !== "heading") b.setAttribute("aria-pressed", "false");
    if (t.menu && t.id !== "link") b.setAttribute("aria-haspopup", "menu");
    // Keep the selection in the text while a button is used.
    b.addEventListener("mousedown", (e) => e.preventDefault());
    b.addEventListener("click", () => {
      t.run(b);
      if (!t.menu) editor.focus();
    });
    el.append(b);
    buttons.push({ b, active: t.active });
  }
  if (opts.tools?.length) el.append(sep(), ...opts.tools);

  // ⋯ More holds whatever does not fit on the row, in order; it is never cut off.
  const more = doc.createElement("button");
  more.type = "button";
  more.className = "rich-btn rich-more";
  more.dataset.tool = "more";
  more.title = "More";
  more.setAttribute("aria-label", "More formatting");
  more.setAttribute("aria-haspopup", "menu");
  more.append(icon("more"));
  more.hidden = true;
  more.addEventListener("mousedown", (e) => e.preventDefault());
  more.addEventListener("click", () => askMore());
  const hint = doc.createElement("span");
  hint.className = "rich-hint";
  hint.textContent = "Type / for more";
  hint.setAttribute("aria-hidden", "true");
  if (variant === "page") el.append(hint);
  el.append(more);

  // The row's items that can move into ⋯ More, in order (separators go with what follows them).
  const movable = () => [...el.children].filter((c) => c !== more && c !== hint) as HTMLElement[];
  function fit() {
    const items = movable();
    for (const c of items) c.hidden = false;
    hint.hidden = false;
    more.hidden = true;
    if (el.scrollWidth <= el.clientWidth + 1) return;
    hint.hidden = true;
    if (el.scrollWidth <= el.clientWidth + 1) return;
    more.hidden = false;
    for (let i = items.length - 1; i >= 0 && el.scrollWidth > el.clientWidth + 1; i--) {
      items[i].hidden = true;
      // A separator never ends the row.
      while (i > 0 && items[i - 1].classList.contains("rich-sep")) items[--i].hidden = true;
    }
    rove(items.find((x) => !x.hidden && x.matches("button")) ?? more);
  }
  const fitter = new ResizeObserver(() => fit());
  fitter.observe(el);

  function askMore() {
    const hidden = movable().filter((c) => c.hidden && c.matches("button"));
    const list = h("div", { class: "menu-list more-menu" }, hidden.map((b) => {
      const t = byId.get(b.dataset.tool ?? "");
      const item = h("button", { type: "button", "data-tool": b.dataset.tool ?? "" },
        h("span", { class: "mm-glyph", "aria-hidden": "true" }, [...b.childNodes].map((n) => n.cloneNode(true))),
        h("span", { class: "hm-name" }, b.getAttribute("aria-label") ?? b.textContent ?? ""),
        t?.keys ? h("kbd", { class: "hm-keys" }, t.keys) : null);
      item.addEventListener("mousedown", (e) => e.preventDefault());
      item.addEventListener("click", () => {
        closePopover();
        if (!t) {
          b.click();
          return;
        }
        // A tool with its own popover opens it from ⋯ More, after this menu has gone.
        if (t.menu) setTimeout(() => t.run(more), 0);
        else {
          t.run(more);
          editor.focus();
        }
      });
      return item;
    }));
    openPopover(more, list, { label: "More formatting", menu: true, side: "below" });
    list.querySelector<HTMLElement>("button")?.focus();
  }

  // One stop on Tab; the arrow keys move along the toolbar.
  const items = () => [...el.querySelectorAll<HTMLElement>("button")].filter((x) => !x.hidden);
  const rove = (to: HTMLElement | undefined) => items().forEach((x) => (x.tabIndex = x === to ? 0 : -1));
  rove(items()[0]);
  el.addEventListener("keydown", (e) => {
    const list = items();
    const i = list.indexOf(doc.activeElement as HTMLElement);
    if (i < 0) return;
    let j = -1;
    if (e.key === "ArrowRight") j = (i + 1) % list.length;
    else if (e.key === "ArrowLeft") j = (i - 1 + list.length) % list.length;
    else if (e.key === "Home") j = 0;
    else if (e.key === "End") j = list.length - 1;
    else if (e.key === "Escape") {
      e.preventDefault();
      editor.focus();
      return;
    }
    if (j < 0) return;
    e.preventDefault();
    rove(list[j]);
    list[j].focus();
  });
  el.addEventListener("focusin", (e) => {
    if (e.target instanceof HTMLElement && e.target.matches("button")) rove(e.target);
  });

  // anchorFor is where a tool's popover opens: its button, or ⋯ More when the button is in there.
  const anchorFor = (id: string) => {
    const b = el.querySelector<HTMLElement>(`[data-tool=${id}]`);
    return b && !b.hidden ? b : more;
  };
  const insertButton = () => anchorFor(el.querySelector("[data-tool=insert]") ? "insert" : "link");

  // The Insert control: one menu for verses, highlights and links to notes.
  function askInsert(anchor: HTMLElement) {
    const list = h("div", { class: "menu-list insert-menu" }, insertsOf(opts).map((it) => {
      const b = h("button", { type: "button", "data-insert": it.id },
        h("span", { class: `im-glyph g-${it.id}`, "aria-hidden": "true" }, it.glyph),
        h("span", { class: "im-text" }, h("span", { class: "im-name" }, it.label), h("span", { class: "im-hint" }, it.hint)),
        it.keys ? h("kbd", { class: "hm-keys" }, it.keys) : null);
      b.addEventListener("mousedown", (e) => e.preventDefault());
      b.addEventListener("click", () => {
        closePopover();
        // After the menu has gone, so the next popover takes the keyboard.
        setTimeout(() => insert(it.id, anchor), 0);
      });
      return b;
    }));
    openPopover(anchor, list, { label: "Insert", menu: true, side: "below" });
    list.querySelector<HTMLElement>("button")?.focus();
  }

  // The table button: a grid to point at for the size, up to 10 by 10.
  function askTable(anchor: HTMLElement) {
    // In a table, the button offers its rows, columns and header instead.
    if (inTable(editor)) return tableMenu(editor, anchor);
    openPopover(anchor, tablePicker(doc, (rows, cols) => {
      closePopover();
      insertTable(editor, rows, cols);
    }), { label: "Table size", side: "below" });
  }

  // The link address is typed in a small popover under the Link button.
  function askLink(anchor: HTMLElement = anchorFor("link")) {
    const current = editor.getEditorState().read(() => linkUrl()) ?? "";
    const input = doc.createElement("input");
    input.type = "url";
    input.placeholder = "https://…";
    input.value = current;
    input.setAttribute("aria-label", "Link address");
    const form = doc.createElement("form");
    form.className = "pop-body rich-pop";
    const ok = doc.createElement("button");
    ok.type = "submit";
    ok.className = "primary";
    ok.textContent = current ? "Change" : "Add link";
    const row = doc.createElement("div");
    row.className = "row";
    row.append(input, ok);
    form.append(row);
    if (current) {
      const rm = doc.createElement("button");
      rm.type = "button";
      rm.className = "link small";
      rm.textContent = "Remove the link";
      rm.addEventListener("click", () => {
        closePopover();
        editor.dispatchCommand(TOGGLE_LINK_COMMAND, null);
        editor.focus();
      });
      form.append(rm);
    }
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      const url = input.value.trim();
      closePopover();
      editor.dispatchCommand(TOGGLE_LINK_COMMAND, url || null);
      editor.focus();
    });
    openPopover(anchor, form, { label: "Link", side: "below" });
    input.select();
  }

  function sync() {
    for (const { b, active } of buttons) {
      if (active) b.setAttribute("aria-pressed", String(active()));
    }
  }
  return {
    el, sync, insertButton, setHeading, toggleList, toggleQuote,
    askLink: () => askLink(), askTable: (a?: HTMLElement) => askTable(a ?? anchorFor("table")),
    destroy: () => fitter.disconnect(),
  };
}

// The things the Insert control and the slash menu put in, each when the host can do it.
export type InsertId = "verseref" | "versequote" | "highlight" | "notelink";

const INSERTS: { id: InsertId; label: string; hint: string; glyph: string; keys?: string }[] = [
  { id: "verseref", label: "Verse reference", hint: "A chip that opens the passage", glyph: "✦", keys: `${MOD}Shift+K` },
  { id: "versequote", label: "Verse text", hint: "The verses, with the Bible's credit line", glyph: "❝" },
  { id: "highlight", label: "Highlight", hint: "Words you highlighted, with where they are", glyph: "▍" },
  { id: "notelink", label: "Link to a note or sermon", hint: "A chip that opens it", glyph: "¶" },
];

function insertsOf(opts: EditorOptions) {
  const can: Record<InsertId, boolean> = {
    verseref: !!opts.onVerseRef, versequote: !!opts.onVerseQuote, highlight: !!opts.onHighlight, notelink: !!opts.onNoteLink,
  };
  return INSERTS.filter((it) => can[it.id]);
}

const DROP_TYPES = ["application/x-xegesis-verse", "application/x-xegesis-highlight", "application/x-xegesis-related"];

// insertDivider puts a divider line after the caret's block (in place of it when it is empty).
function insertDivider(editor: LexicalEditor) {
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
function insertChip(editor: LexicalEditor, label: string, href: string) {
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

// Slash menu ---------------------------------------------------------------------------------------

function slashMenu(doc: Document, editor: LexicalEditor, content: HTMLElement, opts: EditorOptions,
  insert: (id: InsertId, anchor: HTMLElement) => void, links: { askLink(): void }) {
  const box = doc.createElement("div");
  box.className = "slash-menu";
  box.setAttribute("role", "listbox");
  box.setAttribute("aria-label", "Insert");
  box.id = `slash-${Math.random().toString(36).slice(2)}`;
  box.hidden = true;
  doc.body.append(box);
  let items = SLASH_ITEMS;
  let at = 0;
  let open = false;
  let query = "";
  // A slash dismissed with Escape stays plain text until the caret leaves it.
  let dismissed: string | null = null;

  const can = new Set<string>(insertsOf(opts).map((it) => it.id));
  const shown = () => items.filter((it) => !INSERTS.some((x) => x.id === it.id) || can.has(it.id));

  function check() {
    const found = editor.getEditorState().read(() => {
      const s = $getSelection();
      if (!$isRangeSelection(s) || !s.isCollapsed()) return null;
      const node = s.anchor.getNode();
      if (!$isTextNode(node)) return null;
      const before = node.getTextContent().slice(0, s.anchor.offset);
      const q = slashQuery(before);
      return q === null ? null : { q, key: node.getKey() + ":" + (s.anchor.offset - q.length - 1) };
    });
    if (!found || found.key === dismissed || doc.activeElement !== content) {
      if (!found) dismissed = null;
      return close();
    }
    query = found.q;
    items = filterSlash(SLASH_ITEMS, query);
    if (shown().length === 0) return close();
    if (!open) at = 0;
    at = Math.min(at, shown().length - 1);
    open = true;
    draw();
    place();
  }

  function draw() {
    box.replaceChildren(...shown().map((it, i) => {
      const o = doc.createElement("div");
      o.className = "slash-item";
      o.id = `${box.id}-${i}`;
      o.setAttribute("role", "option");
      o.setAttribute("aria-selected", String(i === at));
      const g = doc.createElement("span");
      g.className = `slash-glyph g-${it.id}`;
      g.textContent = it.icon;
      const t = doc.createElement("span");
      t.className = "slash-text";
      const n = doc.createElement("span");
      n.className = "slash-name";
      const size = it.id === "table" ? tableSize(query) : null;
      n.textContent = size && /\d/.test(query) ? `Table ${size.rows} × ${size.cols}` : it.label;
      const hint = doc.createElement("span");
      hint.className = "slash-hint";
      hint.textContent = it.hint;
      t.append(n, hint);
      o.append(g, t);
      o.addEventListener("mousedown", (e) => {
        e.preventDefault();
        choose(i);
      });
      o.addEventListener("mousemove", () => {
        if (at !== i) {
          at = i;
          mark();
        }
      });
      return o;
    }));
    box.hidden = false;
    content.setAttribute("aria-controls", box.id);
    content.setAttribute("aria-activedescendant", `${box.id}-${at}`);
  }

  function mark() {
    [...box.children].forEach((c, i) => c.setAttribute("aria-selected", String(i === at)));
    content.setAttribute("aria-activedescendant", `${box.id}-${at}`);
    (box.children[at] as HTMLElement | undefined)?.scrollIntoView({ block: "nearest" });
  }

  function caretRect(): DOMRect | null {
    const s = doc.getSelection();
    if (!s || s.rangeCount === 0) return null;
    const r = s.getRangeAt(0).cloneRange();
    const rects = r.getClientRects();
    if (rects.length) return rects[rects.length - 1];
    const n = r.startContainer;
    return (n instanceof Element ? n : n.parentElement)?.getBoundingClientRect() ?? null;
  }

  function place() {
    const r = caretRect();
    if (!r) return;
    const vh = doc.documentElement.clientHeight;
    const vw = doc.documentElement.clientWidth;
    const ht = box.offsetHeight;
    let top = r.bottom + 6;
    if (top + ht > vh - 8) top = Math.max(8, r.top - ht - 6);
    box.style.top = `${top}px`;
    box.style.left = `${Math.max(8, Math.min(r.left - 6, vw - box.offsetWidth - 8))}px`;
  }

  function close() {
    if (!open) return;
    open = false;
    box.hidden = true;
    content.removeAttribute("aria-activedescendant");
    content.removeAttribute("aria-controls");
  }

  function choose(i: number) {
    const it = shown()[i];
    if (!it) return;
    close();
    const typed = query;
    // Take the "/query" out of the text first.
    editor.update(() => {
      const s = $getSelection();
      if (!$isRangeSelection(s)) return;
      const node = s.anchor.getNode();
      if (!$isTextNode(node)) return;
      const end = s.anchor.offset;
      const start = end - query.length - 1;
      const r = $createRangeSelection();
      r.anchor.set(node.getKey(), start, "text");
      r.focus.set(node.getKey(), end, "text");
      $setSelection(r);
      r.removeText();
    }, { discrete: true });
    query = typed;
    run(it.id);
  }

  function run(id: SlashId) {
    switch (id) {
      case "h1":
      case "h2":
      case "h3":
        editor.update(() => {
          const s = $getSelection();
          if ($isRangeSelection(s)) $setBlocksType(s, () => $createHeadingNode(id));
        });
        break;
      case "bullets":
        editor.dispatchCommand(INSERT_UNORDERED_LIST_COMMAND, undefined);
        break;
      case "numbers":
        editor.dispatchCommand(INSERT_ORDERED_LIST_COMMAND, undefined);
        break;
      case "checks":
        editor.dispatchCommand(INSERT_CHECK_LIST_COMMAND, undefined);
        break;
      case "code":
        editor.dispatchCommand(FORMAT_TEXT_COMMAND, "code");
        break;
      case "table": {
        const size = tableSize(query) ?? { rows: 3, cols: 3 };
        insertTable(editor, size.rows, size.cols);
        return;
      }
      case "link":
        setTimeout(() => links.askLink(), 30);
        return;
      case "quote":
        editor.update(() => {
          const s = $getSelection();
          if ($isRangeSelection(s)) $setBlocksType(s, () => $createQuoteNode());
        });
        break;
      case "divider":
        insertDivider(editor);
        break;
      case "verseref":
      case "versequote":
      case "highlight":
      case "notelink": {
        // The popover opens where the caret is, on an anchor that goes when it closes.
        const r = caretRect();
        const anchor = doc.createElement("span");
        anchor.className = "ctx-anchor";
        anchor.style.left = `${r?.left ?? 0}px`;
        anchor.style.top = `${r?.bottom ?? 0}px`;
        doc.body.append(anchor);
        const watch = new MutationObserver(() => {
          if (anchor.getAttribute("aria-expanded") === "false") {
            anchor.remove();
            watch.disconnect();
          }
        });
        watch.observe(anchor, { attributes: true, attributeFilter: ["aria-expanded"] });
        // After Lexical has put the caret back, so the popover keeps the keyboard.
        setTimeout(() => insert(id, anchor), 30);
        return;
      }
    }
    editor.focus();
  }

  const off = [
    editor.registerCommand(KEY_ARROW_DOWN_COMMAND, (e) => {
      if (!open) return false;
      e?.preventDefault();
      at = (at + 1) % shown().length;
      mark();
      return true;
    }, COMMAND_PRIORITY_HIGH),
    editor.registerCommand(KEY_ARROW_UP_COMMAND, (e) => {
      if (!open) return false;
      e?.preventDefault();
      at = (at - 1 + shown().length) % shown().length;
      mark();
      return true;
    }, COMMAND_PRIORITY_HIGH),
    editor.registerCommand(KEY_ENTER_COMMAND, (e) => {
      if (!open) return false;
      e?.preventDefault();
      choose(at);
      return true;
    }, COMMAND_PRIORITY_HIGH),
    editor.registerCommand(KEY_TAB_COMMAND, (e) => {
      if (!open) return false;
      e.preventDefault();
      choose(at);
      return true;
    }, COMMAND_PRIORITY_HIGH),
    editor.registerCommand(KEY_ESCAPE_COMMAND, () => {
      if (!open) return false;
      dismissed = editor.getEditorState().read(() => {
        const s = $getSelection();
        if (!$isRangeSelection(s)) return null;
        return s.anchor.getNode().getKey() + ":" + (s.anchor.offset - query.length - 1);
      });
      close();
      return true;
    }, COMMAND_PRIORITY_HIGH),
  ];
  const onBlur = () => setTimeout(() => {
    if (doc.activeElement !== content) close();
  }, 0);
  content.addEventListener("blur", onBlur);
  const onScroll = () => { if (open) place(); };
  doc.addEventListener("scroll", onScroll, true);

  return {
    check,
    destroy() {
      off.forEach((f) => f());
      content.removeEventListener("blur", onBlur);
      doc.removeEventListener("scroll", onScroll, true);
      box.remove();
    },
  };
}
