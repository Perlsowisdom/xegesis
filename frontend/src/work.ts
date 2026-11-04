// Pieces My work's views share: the folder list (folder.list, kept fresh), the "work changed" signal
// that keeps the column's counts and the open view in step, moving an item to a folder by menu or by
// drag and drop, and the question asked before something is deleted for good.

import { Message, run } from "./api";
import { button, h } from "./dom";
import { icon } from "./icons";
import { closePopover, openPopover } from "./popover";
import { DRAG_TYPE } from "./document/drag";
import type { TabSpec } from "./document/grid";
import { FolderRow, folderPath, moveTargets } from "./workwords";

export type ItemKind = "note" | "sermon";

export interface WorkItem {
  kind: ItemKind;
  id: string;
  title: string;
  folder_id?: string;
}

// The folders, loaded once and refreshed whenever work changes.
let folderRows: FolderRow[] = [];
let loading: Promise<FolderRow[]> | null = null;

export function folders(): FolderRow[] {
  return folderRows;
}

export function loadFolders(): Promise<FolderRow[]> {
  loading ??= run<FolderRow[]>("folder.list").then((f) => (folderRows = f)).finally(() => { loading = null; });
  return loading;
}

export function folderName(id?: string): string {
  return folderPath(folderRows, id);
}

const CHANGED = "xegesis:work-changed";

// workChanged tells My work's column and views that notes, sermons, lists or folders changed.
export function workChanged(): void {
  document.dispatchEvent(new Event(CHANGED));
}

export function onWorkChanged(fn: () => void): void {
  document.addEventListener(CHANGED, fn);
}

// moveItem files a note or sermon in a folder ("" for none) and says so.
export async function moveItem(msg: Message, item: WorkItem, folder: string): Promise<boolean> {
  if ((item.folder_id ?? "") === folder) return false;
  const ok = item.kind === "note"
    ? await msg.attempt(() => run("note.move", { id: item.id, folder_id: folder }))
    : await msg.attempt(() => run("document.move", { id: item.id, folder_id: folder }));
  if (ok === undefined) return false;
  await loadFolders();
  msg.ok(folder ? `Moved "${item.title}" to ${folderName(folder)}.` : `Took "${item.title}" out of its folder.`);
  workChanged();
  return true;
}

// moveMenu is the keyboard way to move an item: a menu of folders beside the button that opened it.
export function moveMenu(anchor: HTMLElement, msg: Message, item: WorkItem): void {
  const targets = moveTargets(folderRows);
  const body = h("div", { class: "menu-list move-menu" },
    h("p", { class: "pop-title" }, "Move to folder"),
    targets.length === 1 ? h("p", { class: "hint move-none" }, "No folders yet. Make one with + beside Folders.") : null,
    targets.map((t) => {
      const here = (item.folder_id ?? "") === t.id;
      return button(t.label, () => void moveItem(msg, item, t.id), {
        class: `move-target depth-${t.depth}${here ? " here" : ""}`, disabled: here, "aria-current": here ? "true" : null,
      });
    }));
  for (const [i, b] of [...body.querySelectorAll("button")].entries()) {
    b.prepend(icon(targets[i].id ? "folder" : "notes"));
  }
  openPopover(anchor, body, { label: `Move ${item.title} to a folder`, menu: true, className: "move-pop" });
}

// Dragging ------------------------------------------------------------------------------------------

export const ITEM_TYPE = "application/x-xegesis-work";

// draggableItem lets a card or row be dragged onto a folder, or into the study grid as a tab.
export function draggableItem(el: HTMLElement, item: () => WorkItem, spec: () => TabSpec): void {
  el.draggable = true;
  el.addEventListener("dragstart", (e) => {
    if (!e.dataTransfer) return;
    const it = item();
    e.dataTransfer.setData(ITEM_TYPE, JSON.stringify(it));
    e.dataTransfer.setData(DRAG_TYPE, JSON.stringify(spec()));
    e.dataTransfer.setData("text/plain", it.title);
    e.dataTransfer.effectAllowed = "copyMove";
    el.classList.add("dragging");
    document.body.classList.add("dragging-item");
  });
  el.addEventListener("dragend", () => {
    el.classList.remove("dragging");
    document.body.classList.remove("dragging-item");
  });
}

export function droppedItem(e: DragEvent): WorkItem | null {
  const raw = e.dataTransfer?.getData(ITEM_TYPE);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as WorkItem;
  } catch {
    return null;
  }
}

export function carriesItem(e: DragEvent): boolean {
  return !!e.dataTransfer?.types.includes(ITEM_TYPE);
}

// Asking first ---------------------------------------------------------------------------------------

// confirmDanger asks a yes-or-no question in a dialog, with the dangerous answer named plainly.
export function confirmDanger(title: string, text: string, yes: string): Promise<boolean> {
  return new Promise((resolve) => {
    const no = button("Cancel", () => done(false));
    const ok = button(yes, () => done(true), { class: "danger-solid" });
    const tid = "confirm-" + Math.random().toString(36).slice(2);
    const dlg = h("dialog", { class: "confirm", "aria-labelledby": tid },
      h("h2", { id: tid }, title), h("p", {}, text), h("div", { class: "row confirm-actions" }, no, ok));
    dlg.addEventListener("cancel", (e) => { e.preventDefault(); done(false); });
    document.body.append(dlg);
    closePopover();
    dlg.showModal();
    no.focus();
    function done(v: boolean) {
      dlg.close();
      dlg.remove();
      resolve(v);
    }
  });
}

// itemMenuButton is the ⋯ button that opens an item's menu.
export function moreButton(label: string, open: (anchor: HTMLElement) => void): HTMLButtonElement {
  const b = h("button", { type: "button", class: "icon-btn more-btn", title: "More", "aria-label": label, "aria-haspopup": "menu", "aria-expanded": "false" }, icon("more"));
  b.addEventListener("click", (e) => { e.stopPropagation(); open(b); });
  b.addEventListener("dblclick", (e) => e.stopPropagation());
  return b;
}

// menu opens a menu of actions beside anchor; null entries are left out and "-" draws a line.
export function actionMenu(anchor: HTMLElement, label: string, items: ([string, () => void, string?] | "-" | null)[]): void {
  const body = h("div", { class: "menu-list" });
  for (const it of items) {
    if (!it) continue;
    if (it === "-") body.append(h("hr", { class: "menu-sep" }));
    else body.append(button(it[0], it[1], it[2] ? { class: it[2] } : {}));
  }
  openPopover(anchor, body, { label, menu: true });
}

// emptyState is the friendly message for a view with nothing in it.
export function emptyState(iconName: string, title: string, hint: string, action?: HTMLElement): HTMLElement {
  return h("div", { class: "empty-state" }, h("span", { class: "empty-icon" }, icon(iconName)),
    h("p", { class: "empty-title" }, title), h("p", { class: "hint" }, hint), action ?? null);
}
