import { describe, expect, it } from "vitest";
import { SLASH_ITEMS, filterSlash, slashQuery, tableSize } from "./slashwords";

const ids = (q: string) => filterSlash(SLASH_ITEMS, q).map((i) => i.id);
const labels = (q: string) => filterSlash(SLASH_ITEMS, q).map((i) => i.label);

describe("slash menu", () => {
  it("lists every format and insert, alphabetically", () => {
    expect(labels("")).toEqual([
      "Bulleted list", "Checkbox list", "Code", "Divider", "Heading large", "Heading medium", "Heading small", "Highlight",
      "Link", "Link to a note or sermon", "Numbered list", "Quote", "Table", "Verse reference", "Verse text",
    ]);
  });
  it("keeps /verse, /quote, /highlight and /link as filter words", () => {
    expect(ids("verse")).toEqual(["verseref", "versequote"]);
    expect(ids("quote")).toEqual(["quote", "versequote"]);
    expect(ids("highlight")).toEqual(["highlight"]);
    expect(ids("link")).toEqual(["link", "notelink"]);
  });
  it("narrows by the start of a word, staying alphabetical", () => {
    expect(ids("he")).toEqual(["h1", "h2", "h3"]);
    expect(ids("list")).toEqual(["bullets", "checks", "numbers"]);
    expect(ids("bible")).toEqual(["versequote"]);
    expect(ids("RULE")).toEqual(["divider"]);
    expect(ids("zz")).toEqual([]);
  });
  it("finds the table with a size", () => {
    expect(ids("table3x2")).toEqual(["table"]);
    expect(ids("table4×5")).toEqual(["table"]);
    expect(ids("tab")).toEqual(["table"]);
  });
  it("finds a slash being typed before the caret", () => {
    expect(slashQuery("/")).toBe("");
    expect(slashQuery("text /hea")).toBe("hea");
    expect(slashQuery("/table3x2")).toBe("table3x2");
    expect(slashQuery("/table3×")).toBe("table3×");
    expect(slashQuery("and/or")).toBeNull();
    expect(slashQuery("/two words")).toBeNull();
    expect(slashQuery("1/2")).toBeNull();
  });
});

describe("table sizes", () => {
  it("reads rows by columns, with x or ×", () => {
    expect(tableSize("table")).toEqual({ rows: 3, cols: 3 });
    expect(tableSize("table3x2")).toEqual({ rows: 3, cols: 2 });
    expect(tableSize("table2×4")).toEqual({ rows: 2, cols: 4 });
    expect(tableSize("TABLE 5 x 1")).toEqual({ rows: 5, cols: 1 });
  });
  it("caps anything over 10", () => {
    expect(tableSize("table12x30")).toEqual({ rows: 10, cols: 10 });
    expect(tableSize("table0x3")).toEqual({ rows: 1, cols: 3 });
  });
  it("is null for something else", () => {
    expect(tableSize("tab3")).toBeNull();
    expect(tableSize("quote")).toBeNull();
  });
});
