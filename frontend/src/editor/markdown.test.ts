import { describe, expect, it } from "vitest";
import { parseMarkdown, parsePasted, splitBlocks, joinBlocks, toMarkdown } from "./markdown";

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

describe("underline, dividers and verse quotations", () => {
  it("round-trips them", () => {
    for (const md of [
      "Some <u>underlined</u> and **<u>bold underlined</u>** text",
      "Above\n\n---\n\nBelow",
      "> **28** And we know that all things work together for good.\n> — [Romans 8:28](#passage/Rom.8.28) (KJV). Public domain.",
      "> **1** In the beginning.\n>\n> — [Genesis 1:1](#passage/Gen.1.1) (WEB)",
    ]) expect(round(md)).toBe(md);
  });
  it("reads a credit line as a verse quotation", () => {
    const [b] = parseMarkdown("> **28** text\n> — [Rom 8:28](#passage/Rom.8.28) (KJV)");
    expect(b).toEqual({ type: "versequote", lines: ["**28** text", "— [Rom 8:28](#passage/Rom.8.28) (KJV)"] });
  });
  it("keeps a plain quote a quote", () => {
    expect(parseMarkdown("> just quoted\n> — said someone")[0]).toMatchObject({ type: "versequote" });
    expect(parseMarkdown("> just quoted\n> more")[0]).toMatchObject({ type: "quote" });
  });
  it("reads underline", () => {
    expect(parseMarkdown("<u>a</u>")[0]).toEqual({ type: "paragraph", spans: [{ text: "a", underline: true }] });
    expect(round("<u>unclosed")).toBe("<u>unclosed");
  });
});

describe("check lists", () => {
  it("round-trip with their ticks", () => {
    expect(round("- [ ] read\n- [x] pray")).toBe("- [ ] read\n- [x] pray");
    expect(parseMarkdown("- [x] done")[0]).toEqual({ type: "list", ordered: false, items: [[{ text: "done" }]], checked: [true] });
  });
  it("leave a mixed list a plain list", () => {
    expect(parseMarkdown("- [ ] a\n- b")[0]).not.toHaveProperty("checked");
  });
});

describe("pasted Markdown", () => {
  it("reads each line on its own", () => {
    const bs = parsePasted("# Title\nSome **bold** and _italic_.\n* one\n+ two\n1) first\n2) second\n> quoted\n> more\n***\n- [x] done\n- [ ] not yet");
    expect(bs.map((b) => b.type)).toEqual(["heading", "paragraph", "list", "list", "quote", "divider", "list"]);
    expect(bs[1]).toEqual({ type: "paragraph", spans: [{ text: "Some " }, { text: "bold", bold: true }, { text: " and " }, { text: "italic", italic: true }, { text: "." }] });
    expect(bs[2]).toMatchObject({ ordered: false, items: [[{ text: "one" }], [{ text: "two" }]] });
    expect(bs[3]).toMatchObject({ ordered: true });
    expect(bs[4]).toEqual({ type: "quote", spans: [{ text: "quoted" }, { text: "", br: true }, { text: "more" }] });
    expect(bs[6]).toMatchObject({ checked: [true, false] });
  });
  it("leaves underscores inside words alone", () => {
    expect(parsePasted("snake_case_name")).toEqual([{ type: "paragraph", spans: [{ text: "snake_case_name" }] }]);
  });
  it("skips blank lines and keeps plain lines as paragraphs", () => {
    expect(parsePasted("a\r\n\r\nb").map((b) => b.type)).toEqual(["paragraph", "paragraph"]);
  });
});
