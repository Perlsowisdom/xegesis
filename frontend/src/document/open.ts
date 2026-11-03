// Opening things in the study grid from anywhere in the app: a library book at a place, a passage,
// a note. Views call these; document/studygrid.ts registers the handler when it starts.

import type { TabSpec } from "./grid";

// Locator is where a search hit starts (search.query's locator).
export interface Locator {
  spine?: number;
  href?: string;
  cfi?: string;
  page?: number;
  para?: number;
  offset?: number;
}

export interface BookRequest {
  itemId: string;
  fileId?: string;
  locator?: Locator;
}

type Opener = (spec: TabSpec) => void;
let opener: Opener | null = null;
const waiting: TabSpec[] = [];

export function setOpener(fn: Opener): void {
  opener = fn;
  for (const s of waiting.splice(0)) fn(s);
}

// openInGrid opens a tab in the selected pane of the grid and shows the Reader.
export function openInGrid(spec: TabSpec): void {
  if (opener) opener(spec);
  else waiting.push(spec);
}

export function openBook(r: BookRequest): void {
  openInGrid({ kind: "book", itemId: r.itemId, fileId: r.fileId, locator: r.locator as Record<string, unknown> | undefined });
}
