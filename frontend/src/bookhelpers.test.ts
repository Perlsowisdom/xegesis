import { describe, expect, it } from "vitest";
import {
  base64ToBytes, concatBytes, copyMessage, currentEntry, pageRects, tidyPrefs, tocOptions,
} from "./bookhelpers";

describe("book bytes", () => {
  it("decodes and joins chunks", () => {
    const a = base64ToBytes(btoa("%PDF-"));
    const b = base64ToBytes(btoa("1.4"));
    expect(new TextDecoder().decode(concatBytes([a, b]))).toBe("%PDF-1.4");
    expect(concatBytes([]).length).toBe(0);
  });
});

describe("pageRects", () => {
  const page = { left: 100, top: 50, width: 200, height: 400 };
  it("normalises to fractions of the page and merges a line's pieces", () => {
    const r = pageRects([
      { left: 120, top: 90, width: 40, height: 10 },
      { left: 160, top: 90, width: 20, height: 10 },
      { left: 110, top: 110, width: 0, height: 10 },
      { left: 110, top: 130, width: 50, height: 10 },
    ], page);
    expect(r).toEqual([[0.1, 0.1, 0.3, 0.025], [0.05, 0.2, 0.25, 0.025]]);
  });
  it("clamps to the page and ignores a page with no size", () => {
    expect(pageRects([{ left: 0, top: 0, width: 400, height: 10 }], page)[0][0]).toBe(0);
    expect(pageRects([{ left: 0, top: 0, width: 10, height: 10 }], { ...page, width: 0 })).toEqual([]);
  });
});

describe("tidyPrefs", () => {
  it("fills defaults and bounds values", () => {
    expect(tidyPrefs(null)).toEqual({ font: "serif", size: 100, spacing: 1.7, theme: "auto", zoom: 1 });
    const p = tidyPrefs({ size: 900, zoom: -1, font: "comic" as never, theme: "sepia" });
    expect(p).toMatchObject({ size: 250, zoom: 0.5, font: "serif", theme: "sepia" });
  });
});

describe("contents", () => {
  const toc = [
    { title: "One", section: 1, level: 1 },
    { title: "One A", section: 1, anchor: "bk-a", level: 2 },
    { title: "Two", section: 3, level: 1 },
  ];
  it("indents by level", () => {
    expect(tocOptions(toc)[1]).toEqual(["1", " One A"]);
  });
  it("finds the entry for a section", () => {
    expect(currentEntry(toc, 1)).toBe(0);
    expect(currentEntry(toc, 2)).toBe(0);
    expect(currentEntry(toc, 5)).toBe(2);
    expect(currentEntry([{ title: "Late", section: 4, level: 1 }], 2)).toBe(-1);
  });
});

describe("copyMessage", () => {
  it("explains refusals and what is left", () => {
    expect(copyMessage({ allowed: false, reason: "too long", chars: 10, copied_today: 0 })).toBe("Not copied: too long.");
    const ok = copyMessage({ allowed: true, text: "abc\n\n© X", chars: 3, copied_today: 1003, daily_limit: 2000 });
    expect(ok).toContain("with the attribution");
    expect(ok).toContain("997");
  });
});
