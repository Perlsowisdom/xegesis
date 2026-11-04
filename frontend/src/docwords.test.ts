import { describe, expect, it } from "vitest";
import { addMonths, monthDays, paneMenu, preachedChip, saveText } from "./docwords";

describe("calendar", () => {
  it("lays out a month from Sunday", () => {
    const nov = monthDays(2026, 10); // 1 November 2026 is a Sunday
    expect(nov[0]).toEqual({ day: 1, iso: "2026-11-01", sunday: true });
    expect(nov).toHaveLength(30);
    const oct = monthDays(2026, 9); // 1 October 2026 is a Thursday
    expect(oct.slice(0, 4).every((d) => d.day === 0)).toBe(true);
    expect(oct[4]).toMatchObject({ day: 1, sunday: false });
    expect(oct.find((d) => d.day === 4)?.sunday).toBe(true);
  });
  it("steps months across years", () => {
    expect(addMonths(2026, 11, 1)).toEqual([2027, 0]);
    expect(addMonths(2026, 0, -1)).toEqual([2025, 11]);
  });
  it("words the date preached", () => {
    expect(preachedChip("2026-10-11")).toBe("Sun 11 Oct 2026");
    expect(preachedChip("")).toBe("Not preached yet");
  });
});

describe("words", () => {
  it("says how saving is going", () => {
    expect(saveText("saved")).toBe("Saved");
    expect(saveText("saving")).toBe("Saving…");
  });
});

describe("pane menu", () => {
  const all = { search: true, print: true, link: true, splitRight: true, splitDown: true };
  it("groups this pane, then the layout", () => {
    const g = paneMenu(all);
    expect(g.map((x) => x.head)).toEqual(["This pane", "Layout"]);
    expect(g[0].items.map((i) => i.label)).toEqual(["Search in this book", "Print or export…", "Copy link to this place"]);
    expect(g[1].items.map((i) => i.label)).toEqual(["Split right", "Split down", "Move to…", "Close pane"]);
    expect(g[1].items[3]).toMatchObject({ keys: "Ctrl+W", danger: true });
  });
  it("has no link, contents or history items", () => {
    const labels = paneMenu(all).flatMap((x) => x.items.map((i) => i.label.toLowerCase()));
    expect(labels.some((l) => l.includes("unlink") || l === "contents" || l === "history")).toBe(false);
  });
  it("greys out what the pane cannot do", () => {
    const g = paneMenu({ ...all, search: false, splitDown: false, bibles: true });
    expect(g[0].items.find((i) => i.id === "search")?.disabled).toBe(true);
    expect(g[1].items.find((i) => i.id === "split-down")?.disabled).toBe(true);
    expect(g[0].items.map((i) => i.id)).toContain("bibles");
  });
});
