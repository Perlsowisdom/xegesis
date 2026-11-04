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
  LexicalEditor,
  LexicalNode,
  PASTE_COMMAND,
  SELECTION_CHANGE_COMMAND,
  createEditor as createLexical,
  type Klass,
} from "lexical";
import { createEmptyHistoryState, registerHistory } from "@lexical/history";
import { $createLinkNode, $isLinkNode, LinkNode, TOGGLE_LINK_COMMAND, $toggleLink } from "@lexical/link";
import {
  $createListItemNode, $createListNode, $isListItemNode, $isListNode, INSERT_ORDERED_LIST_COMMAND, INSERT_UNORDERED_LIST_COMMAND,
  ListItemNode, ListNode, REMOVE_LIST_COMMAND, registerCheckList, registerList,
} from "@lexical/list";
import {
  BOLD_ITALIC_STAR, BOLD_ITALIC_UNDERSCORE, BOLD_STAR, BOLD_UNDERSCORE, CHECK_LIST, HEADING, INLINE_CODE, ITALIC_STAR,
  ITALIC_UNDERSCORE, LINK, ORDERED_LIST, QUOTE, UNORDERED_LIST, registerMarkdownShortcuts, type ElementTransformer,
} from "@lexical/markdown";
import { $findMatchingParent } from "@lexical/utils";
import {
  $createHeadingNode, $createQuoteNode, $isHeadingNode, $isQuoteNode, HeadingNode, QuoteNode, registerRichText,
} from "@lexical/rich-text";
import { $setBlocksType } from "@lexical/selection";
import { icon } from "../icons";
import { closePopover, openPopover } from "../popover";
import { Block, Span, parseMarkdown, parsePasted, toMarkdown } from "./markdown";
import { $createDividerNode, $createVerseQuoteNode, $isDividerNode, $isVerseQuoteNode, DividerNode, VerseQuoteNode } from "./nodes";
import { SLASH_ITEMS, SlashId, filterSlash, slashQuery } from "./slashwords";

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
  onHeadings?(): void; // the headings changed
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
}

const MOD = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl+";

export function createEditor(host: HTMLElement, opts: EditorOptions = {}): RichEditor {
  const editor = createLexical({
    namespace: "xegesis-notes",
    nodes: [HeadingNode, QuoteNode, ListNode, ListItemNode, LinkNode, DividerNode, VerseQuoteNode, ...(opts.nodes ?? [])],
    theme: {
      text: { underline: "rt-u", bold: "rt-b", italic: "rt-i", code: "rt-code" }, link: "rt-link",
      list: { checklist: "rt-checklist", listitemChecked: "rt-checked", listitemUnchecked: "rt-unchecked" },
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
  const bar = toolbar(doc, editor, opts, variant);
  host.classList.add("rich-editor", `rich-${variant}`);
  host.replaceChildren(bar.el, content);
  editor.setRootElement(content);
  const slash = slashMenu(doc, editor, content, opts);

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
      BOLD_ITALIC_STAR, BOLD_ITALIC_UNDERSCORE, BOLD_STAR, BOLD_UNDERSCORE, ITALIC_STAR, ITALIC_UNDERSCORE, INLINE_CODE, LINK]),
    editor.registerCommand(KEY_BACKSPACE_COMMAND, (e) => {
      if (!$plainAtStart()) return false;
      e?.preventDefault();
      return true;
    }, COMMAND_PRIORITY_HIGH),
    editor.registerCommand(PASTE_COMMAND, (e) => pasteMarkdown(e), COMMAND_PRIORITY_HIGH),
    editor.registerUpdateListener(({ dirtyElements, dirtyLeaves, tags }) => {
      syncEmpty();
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
      return false;
    }, COMMAND_PRIORITY_LOW),
    editor.registerCommand(KEY_DOWN_COMMAND, (e: KeyboardEvent) => shortcut(e), COMMAND_PRIORITY_LOW),
  ];
  // Ctrl+click follows a link to a passage or a place in the app.
  const onClick = (e: MouseEvent) => {
    const a = (e.target as Element).closest?.("a[href^='#']");
    if (a && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      location.hash = a.getAttribute("href")!;
    }
  };
  content.addEventListener("click", onClick);
  const extra = opts.setup?.(editor);
  syncEmpty();

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
    if (!e.shiftKey && !e.altKey && k === "k") return run(() => bar.askLink());
    if (e.shiftKey && !e.altKey && k === "k" && opts.onVerseRef) return run(() => bar.askVerse());
    if (!e.shiftKey && !e.altKey && k === "u") return run(() => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "underline"));
    if (e.altKey && !e.shiftKey && (e.code === "Digit2" || e.code === "Digit3")) return run(() => bar.toggleHeading(e.code === "Digit2" ? "h2" : "h3"));
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

  function shiftHeading(key: string, by: number) {
    const cur = headings().find((x) => x.key === key);
    if (cur) setHeadingLevel(key, Math.min(3, Math.max(2, cur.level + by)));
  }

  function load(md: string) {
    loading = true;
    editor.update(() => {
      const root = $getRoot();
      root.clear();
      fromBlocks(parseMarkdown(md)).forEach((n) => root.append(n));
      if (!root.getFirstChild()) root.append($createParagraphNode());
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
    revealHeading(key) {
      const el = editor.getElementByKey(key);
      if (!el) return;
      el.scrollIntoView({ block: "start", behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" });
      editor.update(() => {
        const n = $getNodeByKey(key);
        if (n && $isElementNode(n)) n.selectEnd();
      });
      el.classList.remove("rt-flash");
      void el.offsetWidth;
      el.classList.add("rt-flash");
    },
    moveSection,
    setHeadingLevel,
    addHeading,
    caretHeading,
    destroy() {
      off.forEach((f) => f());
      if (typeof extra === "function") extra();
      slash.destroy();
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

function spanNodes(spans: Span[]): LexicalNode[] {
  const out: LexicalNode[] = [];
  let link: { url: string; node: LinkNode } | null = null;
  for (const s of spans) {
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
    } else if ($isLinkNode(c)) out.push(...inline(c, c.getURL()));
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

// Toolbar ------------------------------------------------------------------------------------------

type Tool = { id: string; label: string; keys?: string; glyph?: string; icon?: string; cls?: string; run(): void; active?(): boolean };

function toolbar(doc: Document, editor: LexicalEditor, opts: EditorOptions, variant: "page" | "compact") {
  const el = doc.createElement("div");
  el.className = "rich-toolbar";
  el.setAttribute("role", "toolbar");
  el.setAttribute("aria-label", "Formatting");
  const buttons: { b: HTMLButtonElement; active?: () => boolean }[] = [];

  const sel = () => {
    const s = $getSelection();
    return $isRangeSelection(s) ? s : null;
  };
  const isFormat = (f: "bold" | "italic" | "underline") => () => !!sel()?.hasFormat(f);
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

  function toggleHeading(tag: "h2" | "h3") {
    editor.update(() => {
      const s = sel();
      if (!s) return;
      const b = topBlock();
      const same = b && $isHeadingNode(b) && (b.getTag() === tag || tag === "h2");
      $setBlocksType(s, () => (same ? $createParagraphNode() : $createHeadingNode(tag)));
    });
  }
  function toggleList(type: "bullet" | "number") {
    const on = editor.getEditorState().read(isList(type));
    editor.dispatchCommand(on ? REMOVE_LIST_COMMAND : type === "bullet" ? INSERT_UNORDERED_LIST_COMMAND : INSERT_ORDERED_LIST_COMMAND, undefined);
  }
  function toggleQuote() {
    editor.update(() => {
      const s = sel();
      if (!s) return;
      const b = topBlock();
      $setBlocksType(s, () => (b && $isQuoteNode(b) ? $createParagraphNode() : $createQuoteNode()));
    });
  }

  const tools: (Tool | "|")[] = [
    { id: "bold", label: "Bold", keys: `${MOD}B`, glyph: "B", cls: "t-bold", run: () => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "bold"), active: isFormat("bold") },
    { id: "italic", label: "Italic", keys: `${MOD}I`, glyph: "I", cls: "t-italic", run: () => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "italic"), active: isFormat("italic") },
    { id: "underline", label: "Underline", keys: `${MOD}U`, glyph: "U", cls: "t-underline", run: () => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "underline"), active: isFormat("underline") },
    "|",
    { id: "heading", label: "Heading", keys: `${MOD}Alt+2`, glyph: "H", cls: "t-heading", run: () => toggleHeading("h2"), active: isHeading },
    { id: "bullets", label: "Bulleted list", keys: `${MOD}Shift+8`, icon: "bullets", run: () => toggleList("bullet"), active: isList("bullet") },
    { id: "numbers", label: "Numbered list", keys: `${MOD}Shift+7`, icon: "numbers", run: () => toggleList("number"), active: isList("number") },
    { id: "quote", label: "Quote", keys: `${MOD}Shift+9`, icon: "quote", run: () => toggleQuote(), active: isQuote },
    "|",
    { id: "link", label: "Link", keys: `${MOD}K`, icon: "link", run: () => askLink(), active: () => { const u = linkUrl(); return !!u && !u.startsWith("#passage/"); } },
    ...(opts.onVerseRef ? [{ id: "verse", label: "Verse reference", keys: `${MOD}Shift+K`, glyph: "✦", cls: "t-verse", run: () => askVerse(),
      active: () => !!linkUrl()?.startsWith("#passage/") } as Tool] : []),
  ];

  for (const t of tools) {
    if (t === "|") {
      el.append(Object.assign(doc.createElement("span"), { className: "rich-sep" }));
      continue;
    }
    const b = doc.createElement("button");
    b.type = "button";
    b.className = `rich-btn ${t.cls ?? ""}`;
    b.dataset.tool = t.id;
    b.title = t.keys ? `${t.label} (${t.keys})` : t.label;
    b.setAttribute("aria-label", t.label);
    if (t.keys) b.setAttribute("aria-keyshortcuts", t.keys.replace("⌘", "Meta+").replace("Ctrl+", "Control+"));
    if (t.icon) b.append(icon(t.icon));
    else b.textContent = t.glyph ?? "";
    if (t.active) b.setAttribute("aria-pressed", "false");
    // Keep the selection in the text while a button is used.
    b.addEventListener("mousedown", (e) => e.preventDefault());
    b.addEventListener("click", () => {
      t.run();
      if (t.id !== "link" && t.id !== "verse") editor.focus();
    });
    el.append(b);
    buttons.push({ b, active: t.active });
  }
  if (opts.tools?.length) {
    el.append(Object.assign(doc.createElement("span"), { className: "rich-sep" }), ...opts.tools);
  }
  const hint = doc.createElement("span");
  hint.className = "rich-hint";
  hint.textContent = "Type / for more";
  hint.setAttribute("aria-hidden", "true");
  if (variant === "page") el.append(hint);

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

  const linkBtn = () => el.querySelector<HTMLElement>("[data-tool=link]")!;
  const verseBtn = () => el.querySelector<HTMLElement>("[data-tool=verse]") ?? linkBtn();

  // The link address is typed in a small popover under the Link button.
  function askLink() {
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
    openPopover(linkBtn(), form, { label: "Link", side: "below" });
    input.select();
  }

  function askVerse() {
    if (!opts.onVerseRef) return;
    const selected = editor.getEditorState().read(() => sel()?.getTextContent() ?? "");
    opts.onVerseRef(verseBtn(), selected.trim(), (label, ref) => {
      const url = `#passage/${encodeURIComponent(ref)}`;
      const collapsed = editor.getEditorState().read(() => sel()?.isCollapsed() ?? true);
      if (!collapsed) {
        editor.dispatchCommand(TOGGLE_LINK_COMMAND, url);
        editor.focus();
      } else {
        insertLinkAtCaret(editor, label, url);
      }
    });
  }

  function sync() {
    for (const { b, active } of buttons) {
      if (active) b.setAttribute("aria-pressed", String(active()));
    }
  }
  return { el, sync, askLink, askVerse, toggleHeading, toggleList, toggleQuote };
}

function insertLinkAtCaret(editor: LexicalEditor, label: string, url: string) {
  editor.update(() => {
    let s = $getSelection();
    if (!$isRangeSelection(s)) {
      $getRoot().selectEnd();
      s = $getSelection();
    }
    if (!$isRangeSelection(s)) return;
    const link = $createLinkNode(url).append($createTextNode(label));
    s.insertNodes([link]);
    const space = $createTextNode(" ");
    link.insertAfter(space);
    space.select(1, 1);
  });
  editor.focus();
}

// Slash menu ---------------------------------------------------------------------------------------

function slashMenu(doc: Document, editor: LexicalEditor, content: HTMLElement, opts: EditorOptions) {
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

  const shown = () => items.filter((it) => it.id !== "versequote" || !!opts.onVerseQuote);

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
      n.textContent = it.label;
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
    run(it.id);
  }

  function run(id: SlashId) {
    switch (id) {
      case "heading":
        editor.update(() => {
          const s = $getSelection();
          if ($isRangeSelection(s)) $setBlocksType(s, () => $createHeadingNode("h2"));
        });
        break;
      case "bullets":
        editor.dispatchCommand(INSERT_UNORDERED_LIST_COMMAND, undefined);
        break;
      case "numbers":
        editor.dispatchCommand(INSERT_ORDERED_LIST_COMMAND, undefined);
        break;
      case "quote":
        editor.update(() => {
          const s = $getSelection();
          if ($isRangeSelection(s)) $setBlocksType(s, () => $createQuoteNode());
        });
        break;
      case "divider":
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
        break;
      case "versequote": {
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
        setTimeout(() => opts.onVerseQuote?.(anchor), 30);
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
