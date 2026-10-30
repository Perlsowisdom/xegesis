import { describe, expect, it } from "vitest";
import { ExportPreview, fileName, loadWaiting, rests, saveWaiting, summary } from "./printwords";

const base: ExportPreview = { title: "John 3", format: "print", pieces: [], cuts: [], warnings: [], body: "", css: "" };

describe("print words", () => {
  it("suggests safe file names", () => {
    expect(fileName('John 3:16 / "love"', "pdf")).toBe("John 3 16 love.pdf");
    expect(fileName("", "md")).toBe("Xegesis export.md");
  });

  it("summarises what is in the result and what was cut", () => {
    expect(summary({ ...base, pieces: [{ title: "a", licence: "Public domain", pages: 0.3 }] })).toBe("1 item, about 1 page of text");
    const cut = { reason: "Personal copy", kept_pages: 3, total_pages: 12, rest: { kind: "book" as const, item_id: "x", skip: 6 } };
    const p = { ...base, pieces: [{ title: "a", licence: "Personal copy", pages: 12, cut }, { title: "b", licence: "Public domain", pages: 2 }], cuts: ["a: Personal copy"] };
    expect(summary(p)).toBe("2 items, about 5 pages of text · 1 item shortened by its licence");
    expect(rests(p)).toEqual([{ title: "a", source: cut.rest }]);
  });

  it("remembers parts to print later, and copes with bad storage", () => {
    const mem: Record<string, string> = {};
    const store = { getItem: (k: string) => mem[k] ?? null, setItem: (k: string, v: string) => void (mem[k] = v) };
    expect(loadWaiting(store)).toEqual([]);
    saveWaiting([{ title: "a", sources: [{ kind: "note", id: "1" }], options: {}, saved: "2026-10-29" }], store);
    expect(loadWaiting(store)[0].title).toBe("a");
    mem["xegesis.printLater"] = "{bad";
    expect(loadWaiting(store)).toEqual([]);
  });
});
