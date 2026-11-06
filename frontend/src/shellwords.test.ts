import { describe, expect, it } from "vitest";
import {
  RAIL_DEFAULTS, SIDEBAR_DEFAULTS, activeTool, addRecent, gridButtonPlace, isView, moveTool, normalizeRail,
  normalizeReading, normalizeSidebar, normalizeTheme, normalizeTools, placeForKey, placeOf, railPlaces, showTool, stepSize,
  toggleHidden, visibleTools,
} from "./shellwords";

describe("places", () => {
  it("puts every former section in a place", () => {
    const where = Object.fromEntries(["bibles", "library", "notes", "lists", "plans", "sermons", "reports", "bin", "search", "settings", "commands", "reader"]
      .map((v) => [v, placeOf(v).id]));
    expect(where).toEqual({
      bibles: "library", library: "library", notes: "work", lists: "work", plans: "work", sermons: "work", bin: "work",
      reports: "work", search: "search", settings: "settings", commands: "settings", reader: "reader",
    });
  });
  it("falls back to the Reader", () => {
    expect(placeOf("nowhere").id).toBe("reader");
    expect(isView("nowhere")).toBe(false);
    expect(isView("sermons")).toBe(true);
  });
  it("maps Alt+1 to Alt+4 to the rail places", () => {
    expect(["1", "2", "3", "4", "5", "0", "x"].map(placeForKey)).toEqual(["search", "library", "reader", "work", null, null, null]);
  });
});

describe("rail preferences", () => {
  it("defaults to condensed, not hiding, with the grid on the rail", () => {
    expect(normalizeRail(undefined)).toEqual(RAIL_DEFAULTS);
    expect(normalizeRail("junk")).toEqual(RAIL_DEFAULTS);
  });
  it("keeps valid values and drops the rest", () => {
    expect(normalizeRail({ width: "expanded", autoHide: true, hidden: ["work", "reader", "x", "search"], gridOnRail: false }))
      .toEqual({ width: "expanded", autoHide: true, hidden: ["search", "work"], gridOnRail: false });
  });
  it("hides places but never the Reader", () => {
    let p = toggleHidden(RAIL_DEFAULTS, "library", true);
    expect(railPlaces(p)).toEqual(["search", "reader", "work"]);
    p = toggleHidden(p, "reader", true);
    expect(railPlaces(p)).toEqual(["search", "reader", "work"]);
    p = toggleHidden(p, "library", false);
    expect(railPlaces(p)).toEqual(["search", "library", "reader", "work"]);
  });
  it("puts the grid button on the rail or the toolbar in Study mode only", () => {
    expect(gridButtonPlace("read", RAIL_DEFAULTS)).toBeNull();
    expect(gridButtonPlace("study", RAIL_DEFAULTS)).toBe("rail");
    expect(gridButtonPlace("study", { ...RAIL_DEFAULTS, gridOnRail: false })).toBe("toolbar");
  });
});

describe("right sidebar tools", () => {
  it("shows every tool by default, starting closed", () => {
    expect(normalizeSidebar(null)).toEqual(SIDEBAR_DEFAULTS);
    expect(visibleTools(SIDEBAR_DEFAULTS.tools)).toEqual(["chat", "related", "notes", "highlights", "word", "compare"]);
  });
  it("keeps the saved order, drops unknown and repeated tools, adds new ones", () => {
    const t = normalizeTools([{ id: "word", shown: false }, { id: "nope" }, { id: "chat" }, { id: "word" }]);
    expect(t).toEqual([
      { id: "word", shown: false }, { id: "compare", shown: true }, { id: "chat", shown: true }, { id: "related", shown: true },
      { id: "notes", shown: true }, { id: "highlights", shown: true },
    ]);
  });
  it("puts a new tool after the one it follows", () => {
    const saved = [{ id: "notes", shown: true }, { id: "chat", shown: false }, { id: "related" }, { id: "word" }, { id: "compare" }];
    expect(normalizeTools(saved).map((x) => x.id)).toEqual(["notes", "highlights", "chat", "related", "word", "compare"]);
  });
  it("moves tools up and down within bounds", () => {
    const t = SIDEBAR_DEFAULTS.tools;
    expect(visibleTools(moveTool(t, "notes", -1))).toEqual(["chat", "notes", "related", "highlights", "word", "compare"]);
    expect(moveTool(t, "chat", -1)).toBe(t);
    expect(moveTool(t, "compare", 1)).toBe(t);
  });
  it("chooses a shown tool", () => {
    const t = showTool(SIDEBAR_DEFAULTS.tools, "related", false);
    expect(activeTool(t, "related")).toBe("chat");
    expect(activeTool(t, "word")).toBe("word");
    expect(normalizeSidebar({ open: true, tool: "related", tools: t }).tool).toBe("chat");
  });
});

describe("reading preferences", () => {
  it("defaults to Read mode and normal text", () => {
    expect(normalizeReading({ mode: "x", size: 3, spacing: "huge", theme: "neon" }))
      .toEqual({ mode: "read", size: 100, font: "serif", width: "narrow", spacing: "normal", theme: "auto", taggedWords: false });
    expect(normalizeReading({ mode: "study", size: 130, font: "sans", width: "wide", spacing: "loose", theme: "sepia", taggedWords: true }))
      .toEqual({ mode: "study", size: 130, font: "sans", width: "wide", spacing: "loose", theme: "sepia", taggedWords: true });
  });
  it("steps text sizes", () => {
    expect(stepSize(100, 1)).toBe(115);
    expect(stepSize(85, -1)).toBe(85);
    expect(stepSize(150, 1)).toBe(150);
    expect(stepSize(99, 1)).toBe(115);
  });
});

describe("recent searches", () => {
  it("keeps recent searches unique and short", () => {
    let r: string[] = [];
    for (const q of ["love", "grace", "  Love  ", "", "a", "b", "c", "d", "e", "f"]) r = addRecent(r, q);
    expect(r).toEqual(["f", "e", "d", "c", "b", "a", "Love", "grace"]);
  });
});

describe("app theme", () => {
  it("matches the system unless Light or Dark was chosen", () => {
    expect(normalizeTheme(undefined)).toBe("system");
    expect(normalizeTheme("sepia")).toBe("system");
    expect(normalizeTheme("light")).toBe("light");
    expect(normalizeTheme("dark")).toBe("dark");
  });
});
