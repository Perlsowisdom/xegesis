import { describe, expect, it } from "vitest";
import { growTo, hits, inkSVG, parseInk, serializeInk } from "./ink";
import { parseMarkdown, toMarkdown } from "./markdown";

describe("ink blocks", () => {
  const doc = { h: 300, s: [{ t: "pen" as const, c: "blue" as const, w: 2 as const, p: [[1.5, 2, 0], [3.25, 4.75, 61]] }] };
  const md = serializeInk(doc);
  it("writes one fenced line of JSON", () => {
    expect(md.split("\n")).toHaveLength(3);
    expect(md.startsWith('```ink\n{"v":1,"h":300')).toBe(true);
  });
  it("reads back what it wrote", () => {
    const back = parseInk(md.split("\n"))!;
    expect(back.h).toBe(300);
    expect(back.s[0].p[1]).toEqual([3.3, 4.8, 61]);
    expect(serializeInk(back)).toBe(md.replace("3.25,4.75", "3.3,4.8"));
  });
  it("round trips through the Markdown reader and writer", () => {
    const blocks = parseMarkdown(`Before\n\n${md}\n\nAfter`);
    expect(blocks.map((b) => b.type)).toEqual(["paragraph", "ink", "paragraph"]);
    const once = toMarkdown(blocks);
    expect(once).toBe(`Before\n\n${md.replace("3.25,4.75", "3.3,4.8")}\n\nAfter`);
    expect(toMarkdown(parseMarkdown(once))).toBe(once);
  });
  it("leaves damaged ink as plain text", () => {
    expect(parseMarkdown("```ink\n{nope\n```")[0].type).toBe("paragraph");
    expect(parseMarkdown('```ink\n{"v":2,"h":300,"s":[]}\n```')[0].type).toBe("paragraph");
    expect(parseMarkdown('```ink\n{"v":1,"h":300,"s":[{"t":"x"}]}\n```')[0].type).toBe("paragraph");
  });
  it("hit-tests whole strokes", () => {
    expect(hits(doc.s[0], 2, 3, 2)).toBe(true);
    expect(hits(doc.s[0], 200, 200, 2)).toBe(false);
  });
  it("grows the page near its bottom edge", () => {
    expect(growTo(300, 100)).toBe(300);
    expect(growTo(300, 260)).toBe(500);
    expect(growTo(7900, 7950)).toBe(8000);
  });
  it("draws a stroke as an SVG shape", () => {
    expect(inkSVG(doc)).toMatch(/^<svg [^>]*viewBox="0 0 1000 300"[^>]*><path d="M/);
  });
});
