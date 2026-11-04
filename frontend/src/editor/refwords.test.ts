import { describe, expect, it } from "vitest";
import { refCandidates } from "./refwords";

const texts = (s: string) => refCandidates(s).map((c) => c.text);

describe("references before the caret", () => {
  it("offers the words before the numbers, longest first", () => {
    expect(texts("see Rom 8:28")).toEqual(["see Rom 8:28", "Rom 8:28"]);
    expect(texts("Rom 8:28")).toEqual(["Rom 8:28"]);
  });
  it("keeps numbered books whole", () => {
    expect(texts("read 1 John 3:16")).toContain("1 John 3:16");
    expect(texts("read 1 John 3:16").indexOf("1 John 3:16")).toBeLessThan(texts("read 1 John 3:16").indexOf("John 3:16"));
  });
  it("takes ranges and chapters", () => {
    expect(texts("Rom 8:28-30")).toEqual(["Rom 8:28-30"]);
    expect(texts("Psalm 23")).toEqual(["Psalm 23"]);
    expect(texts("Song of Songs 2:1")[0]).toBe("Song of Songs 2:1");
  });
  it("gives where each starts", () => {
    expect(refCandidates("see Rom 8:28")[1]).toEqual({ text: "Rom 8:28", start: 4 });
  });
  it("finds nothing without a book name or numbers", () => {
    expect(texts("8:28")).toEqual([]);
    expect(texts("Romans")).toEqual([]);
    expect(texts("version 2")).toEqual(["version 2"]);
  });
});
