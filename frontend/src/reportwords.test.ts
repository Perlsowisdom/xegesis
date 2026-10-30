import { describe, expect, it } from "vitest";
import { ReportSection, barWidths, countText, paramFor, routeOf, runs, sourceText, wordFacts } from "./reportwords";

const sec = (id: string, over: Partial<ReportSection> = {}): ReportSection => ({
  id, group: id.split(":")[0], title: id, sources: [], entries: [], total: 0, ...over });

describe("routes", () => {
  it("reads passage, word, topic and report addresses", () => {
    expect(routeOf("#passage/Rom.8.28-Rom.8.30")).toEqual({ to: "passage", ref: "Rom.8.28-Rom.8.30" });
    expect(routeOf("word/G25")).toEqual({ to: "report", kind: "word", query: "G25" });
    expect(routeOf("#topic/grace%20of%20god")).toEqual({ to: "report", kind: "topic", query: "grace of god" });
    expect(routeOf("#report/passage/Rom.8.28")).toEqual({ to: "report", kind: "passage", query: "Rom.8.28" });
  });
  it("reads book places and other sections", () => {
    expect(routeOf("#book/item%201?file=f1&para=1&spine=2")).toEqual({ to: "book", itemId: "item 1", fileId: "f1", locator: { spine: 2, para: 1 } });
    expect(routeOf("#book/abc?file=f&page=7&href=OEBPS%2Fc.xhtml")).toEqual({ to: "book", itemId: "abc", fileId: "f", locator: { page: 7, href: "OEBPS/c.xhtml" } });
    expect(routeOf("#note/01ABC")).toEqual({ to: "section", section: "notes" });
    expect(routeOf("#plan/x")).toEqual({ to: "section", section: "plans" });
  });
  it("leaves section names and nonsense alone", () => {
    for (const h of ["#reader", "", "#word/", "#report/other/x", "#report/word", "#elsewhere/x"]) expect(routeOf(h)).toBeNull();
  });
});

describe("sections", () => {
  it("groups neighbouring sections of one group", () => {
    const r = runs([sec("text:kjv"), sec("text:web"), sec("commentary:mhcc"), sec("crossrefs"), sec("commentary:x")]);
    expect(r.map((x) => `${x.group}:${x.sections.length}`)).toEqual(["text:2", "commentary:1", "crossrefs:1", "commentary:1"]);
  });
  it("credits sources", () => {
    expect(sourceText([])).toBe("");
    expect(sourceText([{ name: "Easton's Bible Dictionary" }])).toBe("From Easton's Bible Dictionary");
    expect(sourceText([{ name: "TBESG", attribution: "STEPBible.org, CC BY 4.0" }, { name: "KJV" }])).toBe("From TBESG. STEPBible.org, CC BY 4.0; KJV");
  });
  it("counts entries", () => {
    expect(countText(sec("x", { total: 1, entries: [{}] }))).toBe("");
    expect(countText(sec("x", { total: 12, entries: Array(12).fill({}) }))).toBe("12");
    expect(countText(sec("x", { total: 20, entries: Array(20).fill({}), more: "x@20" }))).toBe("20 of 20+");
  });
  it("scales bars", () => {
    expect(barWidths([{ label: "John", book: "John", count: 27 }, { label: "Jude", book: "Jude", count: 1 }, { label: "Rom", book: "Rom", count: 0 }])).toEqual([100, 4, 2]);
    expect(barWidths([])).toEqual([]);
  });
  it("describes words", () => {
    expect(wordFacts({ heading: "ἀγαπῶσιν", lemma: "ἀγαπάω", transliteration: "agapaō", gloss: "loving", strong: "G25", morphology: "V-PAP-DPM" }))
      .toBe("ἀγαπάω · agapaō · loving · G25 · V-PAP-DPM");
    expect(wordFacts({ heading: "ὅτι", lemma: "ὅτι", strong: "G3754" })).toBe("G3754");
    expect(paramFor("topic")).toBe("topic");
    expect(paramFor("passage")).toBe("ref");
  });
});
