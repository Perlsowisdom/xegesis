import { describe, expect, it } from "vitest";
import { SLASH_ITEMS, filterSlash, slashQuery } from "./slashwords";

const ids = (q: string) => filterSlash(SLASH_ITEMS, q).map((i) => i.id);

describe("slash menu", () => {
  it("shows everything for an empty query, in order", () => {
    expect(ids("")).toEqual(["heading", "bullets", "numbers", "quote", "versequote", "divider"]);
  });
  it("narrows by the start of a word", () => {
    expect(ids("he")).toEqual(["heading"]);
    expect(ids("li")).toEqual(["bullets", "numbers", "divider"]);
    expect(ids("list")).toEqual(["bullets", "numbers"]);
    expect(ids("q")).toEqual(["quote", "versequote"]);
    expect(ids("verse")).toEqual(["versequote"]);
    expect(ids("bible")).toEqual(["versequote"]);
    expect(ids("RULE")).toEqual(["divider"]);
    expect(ids("zz")).toEqual([]);
  });
  it("puts names that start with the query first", () => {
    expect(ids("l")).toEqual(["bullets", "numbers", "divider"]);
    expect(ids("d")).toEqual(["divider"]);
    expect(ids("n")).toEqual(["numbers"]);
  });
  it("finds a slash being typed before the caret", () => {
    expect(slashQuery("/")).toBe("");
    expect(slashQuery("text /hea")).toBe("hea");
    expect(slashQuery("and/or")).toBeNull();
    expect(slashQuery("/two words")).toBeNull();
    expect(slashQuery("1/2")).toBeNull();
  });
});
