// Tables in notes and sermons, on @lexical/table: the size picker (a grid to point at), inserting a
// table, moving between cells with Tab, resizing columns by dragging their borders, and the menu of
// row, column and header actions (right-click in a cell, or the Table button while in one).
// The Markdown side (GitHub-style tables, with the column widths in a comment) is in markdown.ts.

import {
  $createParagraphNode, $getNearestNodeFromDOMNode, $getNodeByKey, $getRoot, $getSelection, $isParagraphNode, $isRangeSelection, COMMAND_PRIORITY_CRITICAL,
  KEY_TAB_COMMAND, LexicalEditor, LexicalNode,
} from "lexical";
import {
  $createTableNodeWithDimensions, $deleteTableColumnAtSelection, $deleteTableRowAtSelection, $findTableNode,
  $getTableCellNodeFromLexicalNode, $insertTableColumnAtSelection, $insertTableRowAtSelection, $isTableCellNode,
  $isTableNode, $isTableRowNode, $setTableRowIsHeader, TableCellHeaderStates, TableCellNode, TableNode,
  registerTablePlugin, registerTableSelectionObserver,
} from "@lexical/table";
import { h } from "../dom";
import { closePopover, openPopover } from "../popover";
import { MAX_TABLE, clampTable } from "./slashwords";

export const MIN_COL = 60; // px

// tablePicker is a 10 × 10 grid: pointing at a square chooses rows by columns ("3 × 2"); a click
// or Enter inserts. The arrow keys move the choice.
export function tablePicker(doc: Document, pick: (rows: number, cols: number) => void): HTMLElement {
  let rows = 3;
  let cols = 3;
  const label = h("div", { class: "tp-label", "aria-live": "polite" });
  const grid = h("div", { class: "tp-grid", role: "grid", tabindex: "0", "aria-label": "Rows and columns" });
  const cells: HTMLElement[] = [];
  for (let r = 1; r <= MAX_TABLE; r++) {
    for (let c = 1; c <= MAX_TABLE; c++) {
      const cell = h("span", { class: "tp-cell", "data-r": String(r), "data-c": String(c) });
      cell.addEventListener("mouseenter", () => set(r, c));
      cell.addEventListener("mousedown", (e) => e.preventDefault());
      cell.addEventListener("click", () => pick(r, c));
      cells.push(cell);
    }
  }
  grid.append(...cells);
  grid.addEventListener("keydown", (e) => {
    const d: Record<string, [number, number]> = { ArrowDown: [1, 0], ArrowUp: [-1, 0], ArrowRight: [0, 1], ArrowLeft: [0, -1] };
    if (d[e.key]) {
      e.preventDefault();
      set(rows + d[e.key][0], cols + d[e.key][1]);
    } else if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      pick(rows, cols);
    }
  });
  function set(r: number, c: number) {
    [rows, cols] = clampTable(r, c);
    for (const cell of cells) cell.classList.toggle("on", Number(cell.dataset.r) <= rows && Number(cell.dataset.c) <= cols);
    label.textContent = `${rows} × ${cols}`;
    grid.setAttribute("aria-valuetext", `${rows} rows by ${cols} columns`);
  }
  set(rows, cols);
  queueMicrotask(() => grid.focus());
  void doc;
  return h("div", { class: "pop-body table-picker" }, h("h2", { class: "pop-title" }, "Insert a table"), grid, label);
}

// insertTable puts a table with a header row after the caret's block (not inside another table)
// and puts the caret in its first cell.
export function insertTable(editor: LexicalEditor, rows: number, cols: number) {
  [rows, cols] = clampTable(rows, cols);
  editor.update(() => {
    const table = $createTableNodeWithDimensions(rows, cols, { rows: true, columns: false });
    const s = $getSelection();
    let at: LexicalNode | null = $isRangeSelection(s) ? s.anchor.getNode() : null;
    at = at ? ($findTableNode(at) ?? at.getTopLevelElement()) : null;
    if (at) {
      at.insertAfter(table);
      if ($isParagraphNode(at) && at.getTextContentSize() === 0) at.remove();
    } else $getRoot().append(table);
    if (!table.getNextSibling()) table.insertAfter($createParagraphNode());
    firstCell(table)?.selectStart();
  });
  editor.focus();
}

function firstCell(t: TableNode): TableCellNode | null {
  const row = t.getFirstChild();
  const cell = $isTableRowNode(row) ? row.getFirstChild() : null;
  return $isTableCellNode(cell) ? cell : null;
}

function $cellAtSelection(): TableCellNode | null {
  const s = $getSelection();
  return $isRangeSelection(s) ? $getTableCellNodeFromLexicalNode(s.anchor.getNode()) : null;
}

// inTable tells whether the caret is in a table cell.
export function inTable(editor: LexicalEditor): boolean {
  return editor.getEditorState().read(() => !!$cellAtSelection());
}

// The actions on the table holding the caret.
export type TableAction = "row-above" | "row-below" | "col-left" | "col-right" | "del-row" | "del-col" | "del-table" | "header";

export const TABLE_ACTIONS: { id: TableAction; label: string; danger?: boolean }[] = [
  { id: "row-above", label: "Add a row above" },
  { id: "row-below", label: "Add a row below" },
  { id: "col-left", label: "Add a column to the left" },
  { id: "col-right", label: "Add a column to the right" },
  { id: "header", label: "Header row" },
  { id: "del-row", label: "Delete the row", danger: true },
  { id: "del-col", label: "Delete the column", danger: true },
  { id: "del-table", label: "Delete the table", danger: true },
];

export function tableAction(editor: LexicalEditor, a: TableAction) {
  editor.update(() => {
    const cell = $cellAtSelection();
    const table = cell ? $findTableNode(cell) : null;
    if (!cell || !table) return;
    const widths = table.getColWidths();
    const col = cell.getIndexWithinParent();
    switch (a) {
      case "row-above": $insertTableRowAtSelection(false); break;
      case "row-below": $insertTableRowAtSelection(true); break;
      case "col-left":
      case "col-right":
        $insertTableColumnAtSelection(a === "col-right");
        if (widths) {
          const w = [...widths];
          w.splice(a === "col-right" ? col + 1 : col, 0, widths[col] ?? 120);
          table.setColWidths(w);
        }
        break;
      case "del-row":
        if (table.getChildrenSize() <= 1) return removeTable(table);
        $deleteTableRowAtSelection();
        break;
      case "del-col":
        if (cell.getParentOrThrow().getChildrenSize() <= 1) return removeTable(table);
        $deleteTableColumnAtSelection();
        if (widths) table.setColWidths(widths.filter((_, i) => i !== col));
        break;
      case "del-table":
        removeTable(table);
        break;
      case "header": {
        const first = table.getFirstChild();
        const on = $isTableRowNode(first) && first.getChildren().every((c) => $isTableCellNode(c) && c.hasHeaderState(TableCellHeaderStates.ROW));
        $setTableRowIsHeader(table, 0, !on);
        break;
      }
    }
  });
  editor.focus();
}

function removeTable(t: TableNode) {
  const p = $createParagraphNode();
  t.replace(p);
  p.select();
}

export function isHeaderOn(editor: LexicalEditor): boolean {
  return editor.getEditorState().read(() => {
    const cell = $cellAtSelection();
    const t = cell ? $findTableNode(cell) : null;
    const first = t?.getFirstChild();
    return !!first && $isTableRowNode(first) && first.getChildren().every((c) => $isTableCellNode(c) && c.hasHeaderState(TableCellHeaderStates.ROW));
  });
}

// tableMenu opens the actions for the table holding the caret.
export function tableMenu(editor: LexicalEditor, anchor: HTMLElement) {
  const header = isHeaderOn(editor);
  const list = h("div", { class: "menu-list table-menu" }, TABLE_ACTIONS.map((x, i) => {
    const b = h("button", { type: "button", "data-action": x.id, class: x.danger ? "danger" : "",
      ...(x.id === "header" ? { role: "menuitemcheckbox", "aria-checked": String(header) } : {}) },
    x.id === "header" ? `${header ? "✓ " : ""}${x.label}` : x.label);
    b.addEventListener("mousedown", (e) => e.preventDefault());
    b.addEventListener("click", () => {
      closePopover();
      tableAction(editor, x.id);
    });
    return [i === 5 ? h("hr", { class: "menu-sep" }) : null, b];
  }).flat().filter(Boolean) as HTMLElement[]);
  openPopover(anchor, list, { label: "Table", menu: true, side: "below" });
  list.querySelector<HTMLElement>("button")?.focus();
}

// registerTables wires tables into an editor: Lexical's table handling, Tab between cells (Tab in
// the last cell adds a row), the right-click menu and column resizing.
export function registerTables(editor: LexicalEditor, content: HTMLElement, tabFirst: (e: KeyboardEvent) => boolean): () => void {
  const doc = content.ownerDocument;
  const offs = [
    editor.registerCommand(KEY_TAB_COMMAND, (e) => {
      const target = editor.getEditorState().read(() => {
        const cell = $cellAtSelection();
        const table = cell ? $findTableNode(cell) : null;
        if (!cell || !table) return null;
        const all = table.getChildren().flatMap((r) => ($isTableRowNode(r) ? r.getChildren() : [])).filter($isTableCellNode);
        return { i: all.findIndex((c) => c.is(cell)), keys: all.map((c) => c.getKey()) };
      });
      if (!target) return false;
      // A reference typed just before the caret becomes a chip first (Rom 8:28 then Tab).
      if (!e.shiftKey && tabFirst(e)) return true;
      e.preventDefault();
      const to = target.i + (e.shiftKey ? -1 : 1);
      editor.update(() => {
        if (to < 0) return;
        if (to >= target.keys.length) {
          const row = $insertTableRowAtSelection(true);
          const first = row?.getFirstChild();
          if ($isTableCellNode(first)) first.selectStart();
          return;
        }
        const n = $getNodeByKey(target.keys[to]);
        if ($isTableCellNode(n)) n.selectEnd();
      });
      return true;
    }, COMMAND_PRIORITY_CRITICAL),
    registerTablePlugin(editor),
    registerTableSelectionObserver(editor, false),
  ];

  // Right-click in a cell: the table's menu, where the pointer is.
  const onMenu = (e: MouseEvent) => {
    const td = (e.target as Element).closest?.("td, th");
    if (!td || !content.contains(td)) return;
    e.preventDefault();
    e.stopPropagation();
    // Put the caret in the cell that was clicked, then show the menu there.
    const range = (doc as Document & { caretRangeFromPoint?(x: number, y: number): Range | null }).caretRangeFromPoint?.(e.clientX, e.clientY);
    if (range) {
      const sel = doc.getSelection();
      sel?.removeAllRanges();
      sel?.addRange(range);
    }
    const anchor = h("span", { class: "ctx-anchor", style: `left:${e.clientX}px;top:${e.clientY}px` });
    doc.body.append(anchor);
    const watch = new MutationObserver(() => {
      if (anchor.getAttribute("aria-expanded") === "false") {
        anchor.remove();
        watch.disconnect();
      }
    });
    watch.observe(anchor, { attributes: true, attributeFilter: ["aria-expanded"] });
    setTimeout(() => tableMenu(editor, anchor), 0);
  };
  content.addEventListener("contextmenu", onMenu);

  // Column resizing: the pointer near a cell's right border shows a resize cursor; dragging sets
  // that column's width, which is saved with the table.
  let edge: { table: string; col: number; x: number; widths: number[] } | null = null;
  let drag: { table: string; col: number; x: number; widths: number[]; start: number } | null = null;
  const guide = h("div", { class: "rt-col-guide", hidden: true });
  const edgeAt = (e: MouseEvent) => {
    const td = (e.target as Element).closest?.("td, th") as HTMLTableCellElement | null;
    if (!td || !content.contains(td)) return null;
    const r = td.getBoundingClientRect();
    const tableEl = td.closest("table")!;
    let cell = td;
    let col = td.cellIndex;
    if (e.clientX - r.left < 5 && col > 0) {
      cell = td.parentElement!.children[col - 1] as HTMLTableCellElement;
      col -= 1;
    } else if (r.right - e.clientX > 5) return null;
    const key = editor.read(() => {
      const n = $getNearestNodeFromDOMNode(cell);
      return (n && $findTableNode(n)?.getKey()) ?? null;
    });
    if (!key) return null;
    const first = tableEl.rows[0];
    const widths = [...(first?.cells ?? [])].map((c) => Math.round(c.getBoundingClientRect().width));
    return { table: key, col, x: cell.getBoundingClientRect().right, widths };
  };
  const onMove = (e: MouseEvent) => {
    if (drag) return;
    edge = edgeAt(e);
    content.classList.toggle("rt-col-resize", !!edge);
  };
  const onDown = (e: MouseEvent) => {
    if (!edge || e.button !== 0) return;
    e.preventDefault();
    drag = { ...edge, start: e.clientX };
    const host = content.parentElement!;
    host.append(guide);
    const hr = host.getBoundingClientRect();
    const tr = (e.target as Element).closest("table")!.getBoundingClientRect();
    guide.style.top = `${tr.top - hr.top}px`;
    guide.style.height = `${tr.height}px`;
    guide.style.left = `${drag.x - hr.left}px`;
    guide.hidden = false;
    doc.addEventListener("mousemove", onDrag);
    doc.addEventListener("mouseup", onUp, { once: true });
  };
  const onDrag = (e: MouseEvent) => {
    if (!drag) return;
    const hr = content.parentElement!.getBoundingClientRect();
    const w = Math.max(MIN_COL, drag.widths[drag.col] + e.clientX - drag.start);
    guide.style.left = `${drag.x - drag.widths[drag.col] + w - hr.left}px`;
  };
  const onUp = (e: MouseEvent) => {
    doc.removeEventListener("mousemove", onDrag);
    guide.hidden = true;
    const d = drag;
    drag = null;
    if (!d) return;
    const widths = [...d.widths];
    widths[d.col] = Math.max(MIN_COL, Math.round(d.widths[d.col] + e.clientX - d.start));
    editor.update(() => {
      const t = $getNodeByKey(d.table);
      if ($isTableNode(t)) t.setColWidths(widths);
    });
  };
  content.addEventListener("mousemove", onMove);
  content.addEventListener("mousedown", onDown, true);

  return () => {
    offs.forEach((f) => f());
    content.removeEventListener("contextmenu", onMenu);
    content.removeEventListener("mousemove", onMove);
    content.removeEventListener("mousedown", onDown, true);
    doc.removeEventListener("mousemove", onDrag);
    guide.remove();
  };
}
