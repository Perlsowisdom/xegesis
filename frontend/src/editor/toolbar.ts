
import { $createParagraphNode, $getSelection, $isRangeSelection, FORMAT_TEXT_COMMAND, LexicalEditor } from "lexical";
import { $isLinkNode, TOGGLE_LINK_COMMAND } from "@lexical/link";
import { $isListNode, INSERT_CHECK_LIST_COMMAND, INSERT_ORDERED_LIST_COMMAND, INSERT_UNORDERED_LIST_COMMAND, REMOVE_LIST_COMMAND } from "@lexical/list";
import { $createHeadingNode, $createQuoteNode, $isHeadingNode, $isQuoteNode } from "@lexical/rich-text";
import { $setBlocksType } from "@lexical/selection";
import { h } from "../dom";
import { icon } from "../icons";
import { closePopover, openPopover } from "../popover";
import { insertInk } from "./inknode";
import { insertTable, inTable, tableMenu, tablePicker } from "./table";
import { MOD, topBlock } from "./common";
import { type EditorOptions, type InsertId } from "./types";
import { insertDivider, insertsOf } from "./insert";

// Toolbar ------------------------------------------------------------------------------------------

type Tool = {
  id: string; label: string; keys?: string; glyph?: string; icon?: string; cls?: string; menu?: boolean;
  run(anchor: HTMLElement): void; active?(): boolean;
};

export function toolbar(doc: Document, editor: LexicalEditor, opts: EditorOptions, variant: "page" | "compact",
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
    { id: "ink", label: "Handwriting", icon: "pen", run: () => insertInk(editor, topBlock) },
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
