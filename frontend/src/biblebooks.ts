// Library books whose type is Bible are shown with the installed Bibles, marked as books. They are
// read in a book tab, and going to a reference opens the book at that Bible book and chapter, found
// from the book's own contents (book.at_reference). They are not joined into verse-by-verse columns.

import { Message, run } from "./api";
import { h } from "./dom";
import { openInGrid, openInStudy } from "./document/open";

export interface BibleBook {
  id: string;
  title: string;
  authors: string[];
}

interface Summary extends BibleBook {
  status: string;
}

// bibleBooks lists the books typed Bible that can be opened now. It asks every time, so changing a
// book's type shows at once.
export async function bibleBooks(): Promise<BibleBook[]> {
  const r = await run<{ items: Summary[] }>("library.items", { type: "Bible", limit: 200 }).catch(() => ({ items: [] as Summary[] }));
  return r.items.filter((i) => i.status === "online");
}

interface Place {
  file_id: string;
  section: number;
  anchor?: string;
  found: "verse" | "chapter" | "book" | "none";
  offset?: number;
  title?: string;
}

// openAt opens the book at the reference, in the study grid or in Study mode. It says in msg when
// the book does not hold the reference or only its book could be found.
export async function openAt(book: BibleBook, ref: string, msg: Message, study = false): Promise<void> {
  const open = study ? openInStudy : openInGrid;
  const title = ref ? `${book.title} · ${ref}` : book.title;
  const place = ref ? await msg.attempt(() => run<Place>("book.at_reference", { item_id: book.id, ref })) : null;
  if (ref && !place) return;
  if (!place || place.found === "none") {
    if (ref) msg.info(`${book.title} has no ${ref} in its contents; opened at the start.`);
    return open({ kind: "book", itemId: book.id, title: book.title });
  }
  if (place.found === "book") msg.info(`${book.title}: opened at the start of the book; its chapters are not listed in its contents.`);
  open({ kind: "book", itemId: book.id, fileId: place.file_id, section: place.section, anchor: place.anchor, offset: place.found === "verse" ? place.offset : undefined, title });
}

// bookRows draws one button per Bible-type book, each marked as a book.
export function bookRows(books: BibleBook[], onOpen: (b: BibleBook) => void): HTMLElement {
  return h("div", { class: "bible-books", role: "group", "aria-label": "Books typed as Bible" },
    h("p", { class: "hint" }, "Books in your library typed as Bible. They open at the passage."),
    books.map((b) => h("button", { type: "button", class: "bible-book", title: `Open ${b.title} at the passage`, onclick: () => onOpen(b) },
      h("span", {}, b.title), h("small", { class: "book-tag" }, "book"))));
}
