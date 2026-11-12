// One book pane's state and elements, shared by the modules that make it up (see book.ts).

import { Message } from "../api";
import { ReadingPrefs, TOCEntry } from "../bookhelpers";
import { currentLayout } from "../booklayout";
import { h } from "../dom";
import type { PdfDoc, RenderedPage } from "./pdf";
import { DocHost } from "./view";
import type { BookInfo, BookNote, Highlight } from "./bookinfo";
import { loadPrefs } from "./bookinfo";
import { buildPlace } from "./bookwhere";
import { InkCtl, newInk } from "./bookink";

export interface BookCtx {
  host: DocHost;
  msg: Message;
  copyMsg: Message;
  prefs: ReadingPrefs;
  book: BookInfo | null;
  section: number;
  toc: TOCEntry[];
  pdf: { fileId: string; doc: PdfDoc } | null;
  page: RenderedPage | null;
  // Highlights and notes by section (or PDF page): one entry in Pages mode, every loaded one in Scroll.
  store: Map<number, { hs: Highlight[]; ns: BookNote[] }>;
  layout: ReturnType<typeof currentLayout>;
  growing: boolean;
  syncFrame: number;
  // PDF Scroll mode: a slot for every page, drawn only while near the view.
  slots: Map<number, HTMLElement>;
  drawn: Map<number, RenderedPage>;
  pending: Set<number>;
  near: Set<number>;
  observer: IntersectionObserver | null;
  planTimer: number;
  pdfSeq: number;
  back: { section: number; para: number }[];
  saveTimer: number;
  loadSeq: number;
  alive: boolean;
  tocAt: number; // the contents entry the reader is in
  typing: boolean; // the reader is typing in the place field: a place change must not overwrite it
  passCopy: boolean;
  announcing: boolean; // this pane ignores its own marks announcement
  lastWidth: number;
  resizeTimer: number;
  unwirePaging: () => void;
  unwireMarks: () => void;
  ink: InkCtl; // ink on PDF pages (bookink.ts)
  unwireInk: () => void;
  byline: HTMLElement;
  prev: HTMLButtonElement;
  next: HTMLButtonElement;
  unitText: HTMLElement;
  whereInput: HTMLInputElement;
  ofText: HTMLElement;
  placeBox: HTMLElement;
  text: HTMLElement;
  pdfBox: HTMLElement;
  body: HTMLElement;
  attribution: HTMLElement;
  editor: HTMLElement;
  picker: HTMLElement;
  reader: HTMLElement;
  el: HTMLElement;
}

// newCtx makes one book pane's state and the elements it draws in.
export function newCtx(host: DocHost): BookCtx {
  const text = h("article", { class: "book-text doc-text", lang: "" });
  const pdfBox = h("div", { class: "pdf-box doc-text" });
  const body = h("div", { class: "book-body", tabindex: "0", role: "document", "aria-label": "Book text" }, text, pdfBox);
  const attribution = h("footer", { class: "attribution" });
  const editor = h("div", {});
  const picker = h("div", { class: "card book-picker", hidden: true });
  const byline = h("p", { class: "hint book-byline" });
  const msg = new Message();
  const copyMsg = new Message();
  const reader = h("div", { class: "book-reader", hidden: true }, byline, editor, copyMsg.el, body, attribution);
  const el = h("div", { class: "doc doc-book" }, picker, msg.el, reader);
  const c = {
    host, msg, copyMsg, prefs: loadPrefs(), book: null, section: 1, toc: [], pdf: null, page: null,
    store: new Map(), layout: currentLayout(), growing: false, syncFrame: 0,
    slots: new Map(), drawn: new Map(), pending: new Set(), near: new Set(), observer: null, planTimer: 0, pdfSeq: 0,
    back: [], saveTimer: 0, loadSeq: 0, alive: true, tocAt: -1, typing: false, passCopy: false, announcing: false,
    lastWidth: 0, resizeTimer: 0, unwirePaging: () => {}, unwireMarks: () => {}, ink: newInk(), unwireInk: () => {},
    byline, text, pdfBox, body, attribution, editor, picker, reader, el,
  } as unknown as BookCtx;
  buildPlace(c);
  return c;
}
