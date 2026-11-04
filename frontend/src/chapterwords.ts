// "Show the whole chapter" for a passage that is one verse or a few verses of a chapter.

import { Book, verseParts, verseRangeText } from "./helpers";

interface Span { start: number; end: number }

// chapterOsis names the chapter a passage lies in, or null when it spans chapters.
export function chapterOsis(p: Span, books: Book[]): string | null {
  const a = verseParts(p.start);
  const b = verseParts(p.end);
  const book = books.find((x) => x.number === a.book);
  return book && a.book === b.book && a.chapter === b.chapter ? `${book.osis} ${a.chapter}` : null;
}

// chapterToggle says whether the button shows and what it says. chapter is the whole chapter's
// span; a passage that already is the whole chapter, or spans chapters (chapter null), has none.
export function chapterToggle(p: Span, chapter: Span | null, expanded: boolean, books: Book[]): { label: string; expand: boolean } | null {
  if (!chapter || (p.start === chapter.start && p.end === chapter.end)) return null;
  if (expanded) return { label: `Show only ${verseRangeText(p.start, p.end, books)}`, expand: false };
  return { label: `Show all of ${verseRangeText(chapter.start, chapter.start, books).replace(/:\d+$/, "")}`, expand: true };
}
