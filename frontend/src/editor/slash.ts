
import { $createRangeSelection, $getSelection, $isRangeSelection, $isTextNode, $setSelection, COMMAND_PRIORITY_HIGH, FORMAT_TEXT_COMMAND, KEY_ARROW_DOWN_COMMAND, KEY_ARROW_UP_COMMAND, KEY_ENTER_COMMAND, KEY_ESCAPE_COMMAND, KEY_TAB_COMMAND, LexicalEditor } from "lexical";
import { INSERT_CHECK_LIST_COMMAND, INSERT_ORDERED_LIST_COMMAND, INSERT_UNORDERED_LIST_COMMAND } from "@lexical/list";
import { $createHeadingNode, $createQuoteNode } from "@lexical/rich-text";
import { $setBlocksType } from "@lexical/selection";
import { insertInk } from "./inknode";
import { filterSlash, SLASH_ITEMS, SlashId, slashQuery, tableSize } from "./slashwords";
import { insertTable } from "./table";
import { topBlock } from "./common";
import { type EditorOptions, type InsertId } from "./types";
import { insertDivider, INSERTS, insertsOf } from "./insert";

// Slash menu ---------------------------------------------------------------------------------------

export function slashMenu(doc: Document, editor: LexicalEditor, content: HTMLElement, opts: EditorOptions,
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
      case "ink":
        insertInk(editor, topBlock);
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
