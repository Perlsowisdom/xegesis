import { describe, expect, it } from "vitest";
import { parseMarkdown, toMarkdown } from "./editor/markdown";
import { bookPlace, highlightQuoteLines, highlightText } from "./insertwords";
import { routeOf } from "./reportwords";

const round = (md: string) => toMarkdown(parseMarkdown(md));

describe("what Insert puts in round-trips through the stored Markdown", () => {
  it("keeps verse, note and sermon chips as links", () => {
    for (const md of [
      "See [Romans 8:28](#passage/Rom.8.28) today.",
      "Linked: [Grace study](#notes/note/01JABC) and [Easter](#sermons/01JXYZ)",
      "[Romans 8:28-30](#passage/Rom.8.28-Rom.8.30)",
    ]) expect(round(md)).toBe(md);
    const [p] = parseMarkdown("See [Romans 8:28](#passage/Rom.8.28) today.");
    expect(p).toEqual({ type: "paragraph", spans: [{ text: "See " }, { text: "Romans 8:28", link: "#passage/Rom.8.28" }, { text: " today." }] });
  });
  it("reads a book highlight's quotation as a quotation with its credit, and keeps it", () => {
    const lines = highlightQuoteLines({ item_id: "01BOOK", item_title: "Pilgrim's Progress", locator: { page: 12 } }, "As I walked through the wilderness");
    const md = lines.map((l) => `> ${l}`).join("\n");
    expect(md).toBe("> As I walked through the wilderness\n> — [Pilgrim's Progress, page 12](#book/01BOOK?page=12)");
    expect(parseMarkdown(md)[0]).toMatchObject({ type: "versequote" });
    expect(round(md)).toBe(md);
  });
});

describe("highlight words", () => {
  it("reads all the words, or a range of them", () => {
    const row = { text: "  And we   know that all things  " };
    expect(highlightText(row)).toBe("And we know that all things");
    expect(highlightText(row, { start: 7, end: 11 })).toBe("know");
    expect(highlightText(row, { start: -5, end: 999 })).toBe("And we know that all things");
    expect(highlightText({ locator: { text: "from the locator" } })).toBe("from the locator");
  });
  it("links back to the place in the book", () => {
    expect(bookPlace({ item_id: "01B", locator: { section: 3, para: 7 } })).toBe("#book/01B?spine=3&para=7");
    expect(routeOf(bookPlace({ item_id: "01B", locator: { section: 3, para: 7 } }))).toMatchObject({ to: "book", itemId: "01B", locator: { spine: 3, para: 7 } });
  });
});
