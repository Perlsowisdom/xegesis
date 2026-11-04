// The shared document renderer's contract. Every kind of tab (a Bible, a commentary, a book or PDF, a
// note, a sermon, a list, the compact browsers) is drawn by a DocView, mounted once per pane that
// shows it. The pane (document/studygrid.ts) gives each view a DocHost to reach the grid with.
// Only the current tab of each visible pane is mounted; destroy() frees everything, PDFs included.

import { Module, run } from "../api";
import { installedBibles } from "../bibles";
import type { Book } from "../helpers";
import type { Tab, TabSpec } from "./grid";
import type { MarkScope } from "../highlightspanel";

export interface DocHost {
  tab(): Tab;
  ref(): string; // the passage this pane shows
  go(ref: string): void; // move this pane, and the panes linked to it
  update(patch: Partial<TabSpec>): void; // remember the view's state in its tab
  open(spec: TabSpec): void; // open another tab in this pane
  mode(): "read" | "study";
}

// The pane's bar offers whichever of these a view has: ☰ Contents (a place to go, with the recent
// places), and in ⋯ searching, printing and copying a link to the place shown.
export interface DocMenu {
  contents?(anchor: HTMLElement): void;
  link?(): string | null; // an app address of the place shown, e.g. #passage/John.1
  search?(anchor: HTMLElement): void;
  find?(query: string, anchor: HTMLElement): void | Promise<void>; // the Reader bar's "search in what is open"
  print?(): void;
  bibles?(anchor: HTMLElement): void; // the Bibles picker, offered in ⋯ when the pane is too narrow for its button
}

export interface DocView {
  el: HTMLElement;
  placeBox?: HTMLElement; // the passage or chapter box in the pane's bar
  tools?: HTMLElement; // extra controls in the pane's bar
  menu?: DocMenu;
  refChanged?(ref: string): void;
  markScope?(): MarkScope | null; // where its highlights are, for the sidebar's Highlights tool
  focus?(): void;
  destroy(): void;
}

// The books of the Bible, fetched once. The installed Bibles come from bibles.ts, which keeps
// them current as Bibles are installed and removed.
let booksP: Promise<Book[]> | null = null;

export function bibleModules(): Promise<Module[]> {
  return installedBibles();
}

export function bibleBooks(): Promise<Book[]> {
  if (!booksP) booksP = run<Book[]>("verse.books").catch(() => { booksP = null; return []; });
  return booksP;
}

export function tabLabel(t: Tab): string {
  if (t.title) return t.title;
  switch (t.kind) {
    case "bible": return t.modules?.join(", ") || "Bible";
    case "commentary": return "Commentary";
    case "book": return "Book";
    case "note": return "Note";
    case "sermon": return "Sermon";
    case "list": return "List";
    case "library": return "Library";
    case "work": return "My work";
  }
}

// Passage-led tabs follow the pane's passage; the place box shows it.
export function followsPassage(t: Tab): boolean {
  return t.kind === "bible" || t.kind === "commentary";
}
