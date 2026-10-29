// A small rich-text editor for notes (and later sermons) built on Lexical, in plain TypeScript.
// People type formatted text directly; what is stored is Markdown (markdown.ts), so notes stay in an
// open format and export/import round-trips exactly.
//
//   const ed = createEditor(el, { markdown, onChange, placeholder });
//   ed.getMarkdown(); ed.setMarkdown(md); ed.focus(); ed.destroy();
//
// Extending: pass extra Lexical `nodes` and `setup` (called with the Lexical editor to register
// commands and transforms) to add custom blocks; `toBlocks` and `fromBlocks` below are the places a
// custom node would be read and written.

import {
  $createLineBreakNode,
  $createParagraphNode,
  $createTextNode,
  $getRoot,
  $getSelection,
  $isElementNode,
  $isLineBreakNode,
  $isRangeSelection,
  $isTextNode,
  COMMAND_PRIORITY_LOW,
  ElementNode,
  FORMAT_TEXT_COMMAND,
  KEY_DOWN_COMMAND,
  LexicalEditor,
  LexicalNode,
  REDO_COMMAND,
  SELECTION_CHANGE_COMMAND,
  UNDO_COMMAND,
  createEditor as createLexical,
  type Klass,
} from "lexical";
import { createEmptyHistoryState, registerHistory } from "@lexical/history";
import { $createLinkNode, $isLinkNode, LinkNode, TOGGLE_LINK_COMMAND, $toggleLink } from "@lexical/link";
import {
  $createListItemNode, $createListNode, $isListNode, INSERT_ORDERED_LIST_COMMAND, INSERT_UNORDERED_LIST_COMMAND,
  ListItemNode, ListNode, REMOVE_LIST_COMMAND, registerList,
} from "@lexical/list";
import {
  BOLD_ITALIC_STAR, BOLD_STAR, HEADING, INLINE_CODE, ITALIC_STAR, LINK, ORDERED_LIST, QUOTE, UNORDERED_LIST,
  registerMarkdownShortcuts,
} from "@lexical/markdown";
import {
  $createHeadingNode, $createQuoteNode, $isHeadingNode, $isQuoteNode, HeadingNode, QuoteNode, registerRichText,
} from "@lexical/rich-text";
import { $setBlocksType } from "@lexical/selection";
import { Block, Span, parseMarkdown, toMarkdown } from "./markdown";

export interface EditorOptions {
  markdown?: string;
  onChange?(markdown: string): void;
  placeholder?: string;
  label?: string; // accessible name of the text area
  nodes?: Klass<LexicalNode>[];
  setup?(editor: LexicalEditor): void | (() => void);
}

export interface RichEditor {
  getMarkdown(): string;
  setMarkdown(markdown: string): void;
  focus(): void;
  destroy(): void;
  lexical: LexicalEditor;
}


export function createEditor(host: HTMLElement, opts: EditorOptions = {}): RichEditor {
  const editor = createLexical({
    namespace: "xegesis-notes",
    nodes: [HeadingNode, QuoteNode, ListNode, ListItemNode, LinkNode, ...(opts.nodes ?? [])],
    onError: (e) => {
      throw e;
    },
  });
  const doc = host.ownerDocument;
  const content = doc.createElement("div");
  content.className = "rich-text";
  content.contentEditable = "true";
  content.setAttribute("role", "textbox");
  content.setAttribute("aria-multiline", "true");
  content.setAttribute("aria-label", opts.label ?? "Text");
  if (opts.placeholder) content.dataset.placeholder = opts.placeholder;
  const bar = toolbar(doc, editor, content);
  host.classList.add("rich-editor");
  host.replaceChildren(bar.el, bar.linkRow, content);
  editor.setRootElement(content);

  let source = opts.markdown ?? "";
  let dirty = false;
  let loading = false;

  load(source);

  const off = [
    registerRichText(editor),
    registerList(editor),
    editor.registerCommand(TOGGLE_LINK_COMMAND, (url) => {
      $toggleLink(typeof url === "string" && url ? url : null);
      return true;
    }, COMMAND_PRIORITY_LOW),
    registerHistory(editor, createEmptyHistoryState(), 300),
    registerMarkdownShortcuts(editor, [HEADING, QUOTE, UNORDERED_LIST, ORDERED_LIST, BOLD_ITALIC_STAR, BOLD_STAR, ITALIC_STAR, INLINE_CODE, LINK]),
    editor.registerUpdateListener(({ dirtyElements, dirtyLeaves, tags }) => {
      editor.getEditorState().read(() => {
        const root = $getRoot();
        const first = root.getFirstChild();
        content.toggleAttribute("data-empty", root.getChildrenSize() === 1 && !!first && first.getTextContentSize() === 0 && !$isListNode(first));
        bar.sync();
      });
      if (loading || tags.has("history-merge") || (dirtyElements.size === 0 && dirtyLeaves.size === 0)) return;
      dirty = true;
      opts.onChange?.(getMarkdown());
    }),
    editor.registerCommand(SELECTION_CHANGE_COMMAND, () => {
      bar.sync();
      return false;
    }, COMMAND_PRIORITY_LOW),
    editor.registerCommand(KEY_DOWN_COMMAND, (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === "k") {
        e.preventDefault();
        bar.askLink();
        return true;
      }
      return false;
    }, COMMAND_PRIORITY_LOW),
  ];
  const extra = opts.setup?.(editor);

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

  return {
    lexical: editor,
    getMarkdown,
    setMarkdown(md) {
      source = md;
      dirty = false;
      load(md);
    },
    focus: () => editor.focus(),
    destroy() {
      off.forEach((f) => f());
      if (typeof extra === "function") extra();
      editor.setRootElement(null);
      host.replaceChildren();
      host.classList.remove("rich-editor");
    },
  };
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
        const list = $createListNode(b.ordered ? "number" : "bullet");
        b.items.forEach((it) => list.append($createListItemNode().append(...spanNodes(it))));
        return list;
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
    if ($isListNode(n)) {
      return { type: "list", ordered: n.getListType() === "number",
        items: n.getChildren().map((li) => ($isElementNode(li) ? inline(li) : [{ text: li.getTextContent() }])) };
    }
    if ($isElementNode(n)) return { type: "paragraph", spans: inline(n) };
    return { type: "paragraph", spans: [{ text: n.getTextContent() }] };
  });
}

// Toolbar ------------------------------------------------------------------------------------------

function toolbar(doc: Document, editor: LexicalEditor, content: HTMLElement) {
  const el = doc.createElement("div");
  el.className = "rich-toolbar";
  el.setAttribute("role", "toolbar");
  el.setAttribute("aria-label", "Formatting");
  const buttons: { b: HTMLButtonElement; active?: () => boolean }[] = [];

  const add = (label: string, title: string, run: () => void, active?: () => boolean) => {
    const b = doc.createElement("button");
    b.type = "button";
    b.textContent = label;
    b.title = title;
    b.setAttribute("aria-label", title);
    if (active) b.setAttribute("aria-pressed", "false");
    // Keep the selection in the text while a button is used.
    b.addEventListener("mousedown", (e) => e.preventDefault());
    b.addEventListener("click", () => {
      run();
      editor.focus();
    });
    el.append(b);
    buttons.push({ b, active });
  };
  const sel = () => {
    const s = $getSelection();
    return $isRangeSelection(s) ? s : null;
  };
  const block = (): LexicalNode | null => {
    const s = sel();
    return s ? s.anchor.getNode().getTopLevelElement() : null;
  };
  const headingTag = (tag: "h2" | "h3") => () => {
    editor.update(() => {
      const s = sel();
      if (!s) return;
      const b = block();
      const same = b && $isHeadingNode(b) && b.getTag() === tag;
      $setBlocksType(s, () => (same ? $createParagraphNode() : $createHeadingNode(tag)));
    });
  };
  const isHeading = (tag: string) => () => {
    const b = block();
    return !!b && $isHeadingNode(b) && b.getTag() === tag;
  };
  const isFormat = (f: "bold" | "italic") => () => {
    const s = sel();
    return !!s && s.hasFormat(f);
  };
  const isList = (type: string) => () => {
    const b = block();
    return !!b && $isListNode(b) && b.getListType() === type;
  };

  add("B", "Bold (Ctrl+B)", () => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "bold"), isFormat("bold"));
  add("I", "Italic (Ctrl+I)", () => editor.dispatchCommand(FORMAT_TEXT_COMMAND, "italic"), isFormat("italic"));
  add("Heading", "Heading", headingTag("h2"), isHeading("h2"));
  add("Subheading", "Subheading", headingTag("h3"), isHeading("h3"));
  add("Bullets", "Bulleted list", () => {
    const b = editor.getEditorState().read(isList("bullet"));
    editor.dispatchCommand(b ? REMOVE_LIST_COMMAND : INSERT_UNORDERED_LIST_COMMAND, undefined);
  }, isList("bullet"));
  add("Numbers", "Numbered list", () => {
    const b = editor.getEditorState().read(isList("number"));
    editor.dispatchCommand(b ? REMOVE_LIST_COMMAND : INSERT_ORDERED_LIST_COMMAND, undefined);
  }, isList("number"));
  add("Quote", "Quote", () => {
    editor.update(() => {
      const s = sel();
      if (!s) return;
      const b = block();
      $setBlocksType(s, () => (b && $isQuoteNode(b) ? $createParagraphNode() : $createQuoteNode()));
    });
  }, () => {
    const b = block();
    return !!b && $isQuoteNode(b);
  });
  add("Link", "Link (Ctrl+K)", () => askLink(), () => {
    const s = sel();
    return !!s && $isLinkNode(s.anchor.getNode().getParent());
  });
  add("Undo", "Undo (Ctrl+Z)", () => editor.dispatchCommand(UNDO_COMMAND, undefined));
  add("Redo", "Redo (Ctrl+Y)", () => editor.dispatchCommand(REDO_COMMAND, undefined));

  // The link address is typed in a small row under the toolbar.
  const linkRow = doc.createElement("form");
  linkRow.className = "rich-link";
  linkRow.hidden = true;
  const input = doc.createElement("input");
  input.type = "text";
  input.placeholder = "https://… (leave empty to remove the link)";
  input.setAttribute("aria-label", "Link address");
  const ok = doc.createElement("button");
  ok.type = "submit";
  ok.textContent = "Set link";
  linkRow.append(input, ok);
  linkRow.addEventListener("submit", (e) => {
    e.preventDefault();
    const url = input.value.trim();
    linkRow.hidden = true;
    editor.dispatchCommand(TOGGLE_LINK_COMMAND, url || null);
    editor.focus();
  });
  linkRow.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      linkRow.hidden = true;
      editor.focus();
    }
  });
  function askLink() {
    let current = "";
    editor.getEditorState().read(() => {
      const s = sel();
      const p = s?.anchor.getNode().getParent();
      if (p && $isLinkNode(p)) current = p.getURL();
    });
    input.value = current;
    linkRow.hidden = false;
    input.focus();
    input.select();
  }

  function sync() {
    for (const { b, active } of buttons) {
      if (active) b.setAttribute("aria-pressed", String(active()));
    }
    void content;
  }
  return { el, linkRow, sync, askLink };
}
