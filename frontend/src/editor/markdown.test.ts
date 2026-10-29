import { describe, expect, it } from "vitest";
import { parseMarkdown, splitBlocks, joinBlocks, toMarkdown } from "./markdown";

const round = (md: string) => toMarkdown(parseMarkdown(md));

describe("blocks follow the canonical blank-line rules", () => {
  const cases: [string, string[]][] = [
    ["", []],
    ["a", ["a"]],
    ["a\n\nb", ["a", "b"]],
    ["a\nb", ["a\nb"]],
    ["a\n\n\nb", ["a", "", "b"]],
    ["a\n\n\n\nb", ["a", "", "", "b"]],
    ["\na", ["", "a"]],
    ["\n\na", ["", "", "a"]],
    ["a\n", ["a", ""]],
    ["\n", ["", ""]],
    ["\n\n", ["", "", ""]],
  ];
  for (const [md, blocks] of cases) {
    it(JSON.stringify(md), () => {
      expect(splitBlocks(md)).toEqual(blocks);
      expect(joinBlocks(blocks)).toBe(md);
    });
  }
});

describe("round trip", () => {
  const same = [
    "",
    "Plain text.",
    "Two paragraphs.\n\nSecond one.",
    "Line one\nline two in the same paragraph",
    "One\n\n\nafter an empty paragraph",
    "One\n\n\n\n\ntwo empty paragraphs",
    "\nstarts with an empty paragraph",
    "ends with one\n",
    "# Title\n\n## Sub\n\nText",
    "- a\n- b\n- c",
    "1. a\n2. b\n3. c",
    "> quoted\n> more",
    "Some **bold**, *italic*, `code` and [a link](https://example.com/x).",
    "***both*** and **bold with *italic* inside**",
    "[**bold link**](u)",
    "Verse: John 3:16 (ESV) – “love”",
    // Hand-written text the editor does not recognise stays exactly as it is.
    "* star list\n* second",
    "1. one\n3. three",
    "#nospace",
    "2 * 3 * 4",
    "snake_case_name and __dunder__",
    "- item\ncontinued",
    "```\ncode\n```",
    "<b>html</b>",
    "Trailing space ",
  ];
  for (const md of same) {
    it(JSON.stringify(md), () => expect(round(md)).toBe(md));
  }

  it("keeps empty paragraphs between blocks and at the edges", () => {
    const blocks = parseMarkdown("\n\nA\n\n\n\nB\n\n\n");
    expect(blocks.filter((b) => b.type === "paragraph" && b.spans.length === 0)).toHaveLength(7);
    expect(round("\n\nA\n\n\n\nB\n\n\n")).toBe("\n\nA\n\n\n\nB\n\n\n");
  });
});

describe("reading", () => {
  it("finds formatting", () => {
    const [b] = parseMarkdown("a **b** *c*");
    expect(b).toEqual({ type: "paragraph", spans: [
      { text: "a " }, { text: "b", bold: true }, { text: " " }, { text: "c", italic: true }] });
  });
  it("finds headings, lists and quotes", () => {
    expect(parseMarkdown("## H")[0]).toMatchObject({ type: "heading", level: 2 });
    expect(parseMarkdown("- a\n- b")[0]).toMatchObject({ type: "list", ordered: false });
    expect(parseMarkdown("1. a\n2. b")[0]).toMatchObject({ type: "list", ordered: true });
    expect(parseMarkdown("> a")[0]).toMatchObject({ type: "quote" });
  });
});
