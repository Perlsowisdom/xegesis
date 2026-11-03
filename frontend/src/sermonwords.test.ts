import { describe, expect, it } from "vitest";
import { cleanPoints, docSummary, outlineMarkdown, untitled } from "./sermonwords";

describe("sermon words", () => {
  it("summarises a document", () => {
    expect(docSummary({ kind: "sermon", passage: { display: "Romans 8:28-39" }, points: [{ h: "a" }, { h: "b" }], words: 1240 }))
      .toBe("Sermon · Romans 8:28-39 · 2 points · 1,240 words");
    expect(docSummary({ kind: "study", points: [{ h: "a" }], words: 1 })).toBe("Bible study · 1 point · 1 word");
  });
  it("names untitled documents by kind", () => {
    expect(untitled("lesson")).toBe("Untitled lesson");
  });
  it("cleans points", () => {
    expect(cleanPoints([{ h: "  God   works " }, { h: "", ref: " " }, { h: "", ref: "Rom 8:31", level: 2 }]))
      .toEqual([{ h: "God works", ref: "", level: 1 }, { h: "", ref: "Rom 8:31", level: 2 }]);
  });
  it("turns an outline into headings", () => {
    expect(outlineMarkdown([{ h: "God works", ref: "Rom 8:28", level: 1 }, { h: "For good", level: 2 }, { h: "", ref: "Rom 8:38", level: 1 }]))
      .toBe("## 1. God works\n\n*Rom 8:28*\n\n### For good\n\n## 2. Rom 8:38");
  });
});
