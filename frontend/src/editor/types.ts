// Types shared by the editor's modules (see index.ts).


import type { Klass, LexicalEditor, LexicalNode } from "lexical";

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

// The things the Insert control and the slash menu put in, each when the host can do it.
export type InsertId = "verseref" | "versequote" | "highlight" | "notelink";
