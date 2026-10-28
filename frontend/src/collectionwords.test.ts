import { describe, expect, it } from "vitest";
import { CollectionNode } from "./api";
import { emptyForm, find, formFromRule, matchText, mine, ruleFromForm, ruleIsEmpty, ruleSummary, searchChoices, without } from "./collectionwords";

const node = (id: string, name: string, kind: CollectionNode["kind"], count = 0, children: CollectionNode[] = []): CollectionNode => ({ id, name, kind, count, children });

const tree: CollectionNode[] = [
  node("b:all", "All books", "builtin", 9),
  node("b:type", "Type", "builtin", 7, [node("b:type:Bible", "Bible", "builtin", 2), node("b:type:none", "Not set yet", "builtin", 2)]),
  node("b:year", "Year", "builtin", 5, [node("b:year:c16", "16th century", "builtin", 3)]),
  node("a", "Sunday", "manual", 3, [node("a1", "Romans", "manual", 1), node("a2", "Reformed", "rule", 2)]),
  node("b", "Study", "rule", 4),
];

describe("ruleFromForm", () => {
  it("leaves out anything left empty or blank", () => {
    expect(ruleFromForm(emptyForm())).toEqual({});
    expect(ruleFromForm({ ...emptyForm(), type: "Bible", author: "  calvin ", yearFrom: "1500", yearTo: "x" }))
      .toEqual({ type: "Bible", author: "calvin", year_from: 1500 });
  });
  it("round trips through formFromRule", () => {
    const r = { type: "none", tradition: "Reformed", text: "romans", language: "en", year_from: 1500, year_to: 1700, folder_id: "f1", licence: "public_domain" };
    expect(ruleFromForm(formFromRule(r))).toEqual(r);
  });
  it("knows when nothing is set", () => {
    expect(ruleIsEmpty({})).toBe(true);
    expect(ruleIsEmpty({ author: "  " })).toBe(true);
    expect(ruleIsEmpty({ year_to: 1700 })).toBe(false);
  });
});

describe("ruleSummary", () => {
  it("says it in plain words", () => {
    expect(ruleSummary({ type: "Commentary (New Testament)", tradition: "Reformed", author: "Calvin" }))
      .toBe("Commentary (New Testament), Reformed, by Calvin");
    expect(ruleSummary({ type: "none", year_from: 1500, year_to: 1600, licence: "creative_commons", language: "grc" }))
      .toBe("no type yet, Ancient Greek, 1500 to 1600, Free to share");
    expect(ruleSummary({ year_to: 1700, folder_id: "f" }, () => "Books")).toBe("1700 or earlier, in Books");
    expect(ruleSummary({})).toBe("all books");
  });
});

describe("matchText", () => {
  it("counts books", () => {
    expect(matchText(0)).toBe("No books match yet.");
    expect(matchText(1)).toBe("1 book matches.");
    expect(matchText(12)).toBe("12 books match.");
  });
});

describe("lists", () => {
  it("mine lists only the user's own, in order, with depth", () => {
    expect(mine(tree).map((f) => [f.node.id, f.depth])).toEqual([["a", 0], ["a1", 1], ["a2", 1], ["b", 0]]);
  });
  it("without drops a collection and what is inside it", () => {
    expect(without(mine(tree), "a").map((f) => f.node.id)).toEqual(["b"]);
    expect(without(mine(tree), "a1").map((f) => f.node.id)).toEqual(["a", "a2", "b"]);
  });
  it("find looks at every level", () => {
    expect(find(tree, "a2")?.name).toBe("Reformed");
    expect(find(tree, "zz")).toBeUndefined();
  });
  it("searchChoices has my collections first, then the built-in ones", () => {
    const g = searchChoices(tree);
    expect(g.map((x) => x.label)).toEqual(["My collections", "Type", "Century"]);
    expect(g[0].options[1]).toEqual(["a1", "  Romans (1)"]);
    expect(g[1].options).toEqual([["b:type:Bible", "Bible (2)"], ["b:type:none", "Type: Not set yet (2)"]]);
    expect(searchChoices([node("b:all", "All books", "builtin", 1)])).toEqual([]);
  });
});
