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

import { parseMarkdown } from "./editor/markdown";
import { migrateOutline, moveSection, outlineOf, pointsOf, sectionEnd } from "./sermonwords";

describe("the outline is the headings", () => {
  const md = [
    "Opening words.",
    "## 1. What we know",
    "We *know* it. See [Romans 8:28](#passage/Rom.8.28).",
    "> **28** And we know.\n> — [Romans 8:28](#passage/Rom.8.28) (KJV)",
    "### Not a feeling",
    "## For whom it is true",
    "> **29** For whom.\n> — [Romans 8:29](#passage/Rom.8.29) (KJV). Public domain.",
    "# The golden chain",
  ].join("\n\n");
  const blocks = parseMarkdown(md);

  it("reads points, levels and verses", () => {
    expect(outlineOf(blocks)).toEqual([
      { index: 1, h: "What we know", level: 1, refs: [{ label: "Romans 8:28", ref: "Rom.8.28" }] },
      { index: 4, h: "Not a feeling", level: 2, refs: [] },
      { index: 5, h: "For whom it is true", level: 1, refs: [{ label: "Romans 8:29", ref: "Rom.8.29" }] },
      { index: 7, h: "The golden chain", level: 1, refs: [] },
    ]);
  });
  it("saves each point's first verses", () => {
    expect(pointsOf(blocks).map((p) => p.ref)).toEqual(["Rom.8.28", "", "Rom.8.29", ""]);
  });
  it("finds where a section ends", () => {
    const levels = [0, 2, 0, 0, 3, 2, 0, 1];
    expect(sectionEnd(levels, 1)).toBe(5);
    expect(sectionEnd(levels, 4)).toBe(5);
    expect(sectionEnd(levels, 5)).toBe(7);
    expect(sectionEnd(levels, 7)).toBe(8);
  });
  it("moves a section with its sub-points", () => {
    const levels = [0, 2, 0, 0, 3, 2, 0, 1];
    expect(moveSection(levels, 5, 1)).toEqual([0, 5, 6, 1, 2, 3, 4, 7]);
    expect(moveSection(levels, 1, 8)).toEqual([0, 5, 6, 7, 1, 2, 3, 4]);
    expect(moveSection(levels, 1, 3)).toEqual([0, 1, 2, 3, 4, 5, 6, 7]);
    expect(moveSection(levels, 4, 1)).toEqual([0, 4, 1, 2, 3, 5, 6, 7]);
  });
  it("migrates an older outline into headings", () => {
    const points = [{ h: "What we know", ref: "Rom 8:28" }, { h: "A new point", ref: "Rom 8:31", level: 2 as const }, { h: "", ref: "Rom 8:38" }];
    expect(migrateOutline(points, blocks)).toBe("### A new point\n\n[Rom 8:31](#passage/Rom%208%3A31)\n\n## Rom 8:38");
    expect(migrateOutline([], blocks)).toBe("");
  });
});
