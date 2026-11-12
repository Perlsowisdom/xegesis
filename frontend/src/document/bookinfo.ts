// The shapes a book pane works with, and the reading preferences it remembers.

import { Note } from "../api";
import { DEFAULT_PREFS, ReadingPrefs, tidyPrefs, TOCEntry } from "../bookhelpers";
import type { Locator } from "./open";

export interface BookInfo {
  item_id: string;
  file_id: string;
  title: string;
  authors: string[];
  format: string;
  reflowable: boolean;
  sections: number;
  toc: TOCEntry[];
  licence: string;
  attribution?: string;
  position?: { section: number; para?: number } | null;
}

export interface SectionData {
  section: number;
  sections: number;
  title?: string;
  html?: string;
  cfi?: string;
}

export interface Anchor {
  section: number;
  cfi?: string;
  para?: number;
  offset?: number;
  end_para?: number;
  end_offset?: number;
  page?: number;
  rects?: [number, number, number, number][];
  text?: string;
}

export interface Highlight {
  id: string;
  locator?: Anchor;
  style: string;
}

export type BookNote = Note & { item_id?: string; locator?: Anchor };

export interface Target {
  anchor?: string;
  para?: number;
  offset?: number;
  flash?: boolean;
}

const PREFS_KEY = "xegesis.book.prefs";

export function loadPrefs(): ReadingPrefs {
  try {
    return tidyPrefs(JSON.parse(localStorage.getItem(PREFS_KEY) ?? "null"));
  } catch {
    return { ...DEFAULT_PREFS };
  }
}

export interface OpenRequest {
  itemId: string;
  fileId?: string;
  locator?: Locator;
  section?: number;
  para?: number;
  anchor?: string;
  offset?: number;
}
