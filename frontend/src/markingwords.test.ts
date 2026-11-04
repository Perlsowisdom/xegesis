import { describe, expect, it } from "vitest";
import { hlClasses, parseStyle, replaced, styleOf, toolbarPlace, wordAround } from "./markingwords";

describe("highlight styles", () => {
  it("reads and writes colour and underline", () => {
    expect(parseStyle("green")).toEqual({ colour: "green", underline: false });
    expect(parseStyle("pink-underline")).toEqual({ colour: "pink", underline: true });
    expect(parseStyle("underline")).toEqual({ colour: "yellow", underline: true });
    expect(parseStyle("plaid")).toEqual({ colour: "yellow", underline: false });
    expect(styleOf({ colour: "blue", underline: true })).toBe("blue-underline");
    expect(styleOf(parseStyle("purple"))).toBe("purple");
  });
  it("draws a style with classes", () => {
    expect(hlClasses("blue")).toEqual(["hl", "hl-blue"]);
    expect(hlClasses("underline")).toEqual(["hl", "hl-yellow", "hl-u"]);
  });
});

describe("highlight ranges", () => {
  const list = [
    { id: "a", verse_start: 43003016, verse_end: 43003016 },
    { id: "b", verse_start: 43003015, verse_end: 43003017 },
    { id: "c", verse_start: 43003017, verse_end: 43003018 },
    { id: "d" },
  ];
  it("replaces only highlights wholly inside the new one", () => {
    expect(replaced(list, 43003016, 43003017).map((x) => x.id)).toEqual(["a"]);
    expect(replaced(list, 43003015, 43003018).map((x) => x.id)).toEqual(["a", "b", "c"]);
    expect(replaced(list, 43003019, 43003020)).toEqual([]);
  });
});

describe("toolbar position", () => {
  const view = { width: 1000, height: 700 };
  const bar = { width: 300, height: 40 };
  it("sits just above the selection, centred", () => {
    expect(toolbarPlace({ left: 400, top: 300, width: 200, height: 24 }, bar, view)).toEqual({ left: 350, top: 252, below: false });
  });
  it("stays inside the window at the edges", () => {
    expect(toolbarPlace({ left: 0, top: 300, width: 40, height: 24 }, bar, view).left).toBe(8);
    expect(toolbarPlace({ left: 960, top: 300, width: 40, height: 24 }, bar, view).left).toBe(692);
  });
  it("goes below when there is no room above", () => {
    expect(toolbarPlace({ left: 400, top: 20, width: 200, height: 24 }, bar, view)).toEqual({ left: 350, top: 52, below: true });
    expect(toolbarPlace({ left: 400, top: 20, width: 200, height: 700 }, bar, view).top).toBe(652);
  });
});

describe("the word under the pointer", () => {
  const t = "In the beginning was the Word, and God’s word-play.";
  it("finds whole words", () => {
    expect(wordAround(t, 9)).toEqual([7, 16]);
    expect(t.slice(...wordAround(t, 26)!)).toBe("Word");
    expect(t.slice(...wordAround(t, 39)!)).toBe("God’s");
    expect(t.slice(...wordAround(t, 44)!)).toBe("word-play");
  });
  it("takes the word just before the pointer, and nothing between words", () => {
    expect(t.slice(...wordAround(t, 2)!)).toBe("In");
    expect(wordAround("a  b", 2)).toBeNull();
  });
});
