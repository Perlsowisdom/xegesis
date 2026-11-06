import { describe, expect, it } from "vitest";
import {
  Grid, GridNode, MAX_PANES, MAX_SIBLINGS, MIN_SIZE, Pane, Split, TabSpec, activeTab, applyPreset, canDrop, closePane, closeTab, dropTab, emptyGrid, equalise,
  findPane, findTab, insertTab, moveTab, openBookTab, openTab, paneRef, panes, resizeBetween, resizeSplit, restore, selectTab, serialise,
  isSplit, setLinked, setRef, shape, updateTab,
} from "./grid";

const bible: TabSpec = { kind: "bible", modules: ["KJV"] };
const book = (n: number): TabSpec => ({ kind: "book", itemId: `b${n}`, title: `Book ${n}` });

// grid builds a grid from a list of columns, each a list of panes, each a list of tab titles (the
// layout an older save held, so it also tests opening those).
function grid(layout: string[][][]): Grid {
  const columns = layout.map((col) => ({ panes: col.map((pane) => ({ tabs: pane.map((title) => ({ kind: "book", title })) })) }));
  return restore({ columns, passage: "John 1" });
}

// columnsOf reads a grid made of columns back as the same list of columns of panes.
function columnsOf(g: Grid): { size: number; panes: { size: number; tabs: { title?: string; kind: string }[] }[] }[] {
  const r = g.root;
  if (!r) return [];
  const col = (n: GridNode) => (isSplit(n) ? { size: n.size, panes: n.kids as Pane[] } : { size: n.size, panes: [n] });
  return (isSplit(r) && r.dir === "h" ? r.kids.map(col) : [col(r)]).map((c) => ({ size: c.size, panes: c.panes }));
}

// titles shows a grid as the same nesting of tab titles.
function titles(g: Grid): string[][][] {
  return columnsOf(g).map((c) => c.panes.map((p) => p.tabs.map((t) => t.title ?? t.kind)));
}

function tabId(g: Grid, title: string): string {
  for (const p of panes(g)) for (const t of p.tabs) if (t.title === title) return t.id;
  throw new Error(`no tab ${title}`);
}

function paneOf(g: Grid, title: string): string {
  return findTab(g, tabId(g, title))!.pane.id;
}

describe("opening and closing", () => {
  it("makes the first pane when the grid is empty", () => {
    const g = openTab(emptyGrid("Rom 8"), bible);
    expect(g.root).toBe(panes(g)[0]);
    const p = panes(g)[0];
    expect(g.selected).toBe(p.id);
    expect(activeTab(p)?.kind).toBe("bible");
    expect(paneRef(g, p)).toBe("Rom 8");
  });

  it("opens a new tab in the selected pane and makes it current", () => {
    let g = grid([[["A"]], [["B"]]]);
    g = selectTab(g, tabId(g, "A"));
    g = openTab(g, book(9));
    expect(titles(g)).toEqual([[["A", "Book 9"]], [["B"]]]);
    expect(activeTab(findPane(g, g.selected)!.pane)?.title).toBe("Book 9");
  });

  it("shows a book already open in the pane instead of opening it again", () => {
    let g = grid([[["A"]]]);
    g = openBookTab(g, book(9));
    g = selectTab(g, tabId(g, "A"));
    g = openBookTab(g, book(9));
    expect(titles(g)).toEqual([[["A", "Book 9"]]]);
    expect(activeTab(findPane(g, g.selected)!.pane)?.title).toBe("Book 9");
    // A place in the book opens it there, in a tab of its own.
    g = openBookTab(g, { ...book(9), section: 2 });
    expect(titles(g)).toEqual([[["A", "Book 9", "Book 9"]]]);
  });
  it("makes the next tab current when the current one closes", () => {
    let g = grid([[["A", "B", "C"]]]);
    g = selectTab(g, tabId(g, "B"));
    g = closeTab(g, tabId(g, "B"));
    expect(activeTab(panes(g)[0])?.title).toBe("C");
    g = closeTab(g, tabId(g, "C"));
    expect(activeTab(panes(g)[0])?.title).toBe("A");
  });

  it("closes the pane with its last tab, and the column with its last pane", () => {
    let g = grid([[["A"], ["B"]], [["C"]]]);
    g = closeTab(g, tabId(g, "B"));
    expect(titles(g)).toEqual([[["A"]], [["C"]]]);
    g = closeTab(g, tabId(g, "C"));
    expect(titles(g)).toEqual([[["A"]]]);
    expect(columnsOf(g)[0].size).toBeCloseTo(1);
    g = closeTab(g, tabId(g, "A"));
    expect(g.root).toBeNull();
    expect(g.selected).toBe("");
  });

  it("moves the selection to a neighbour when the selected pane closes", () => {
    let g = grid([[["A"]], [["B"]]]);
    g = selectTab(g, tabId(g, "B"));
    g = closePane(g, paneOf(g, "B"));
    expect(g.selected).toBe(paneOf(g, "A"));
  });

  it("gives a closed pane's space to its neighbour", () => {
    let g = grid([[["A"], ["B"], ["C"]]]);
    g = equalise(g);
    g = closeTab(g, tabId(g, "B"));
    expect(columnsOf(g)[0].panes.map((p) => p.size)).toEqual([2 / 3, 1 / 3]);
  });

  it("does not change the grid it was given", () => {
    const g = grid([[["A", "B"]]]);
    const before = serialise(g);
    closeTab(g, tabId(g, "A"));
    openTab(g, bible);
    dropTab(g, tabId(g, "A"), paneOf(g, "A"), "right");
    expect(serialise(g)).toBe(before);
  });
});

describe("drop rules", () => {
  it("moves a tab into another pane's middle", () => {
    const g = grid([[["A", "B"]], [["C"]]]);
    const n = dropTab(g, tabId(g, "B"), paneOf(g, "C"), "center")!;
    expect(titles(n)).toEqual([[["A"]], [["C", "B"]]]);
    expect(activeTab(findPane(n, n.selected)!.pane)?.title).toBe("B");
  });

  it("closes the pane a moved tab leaves empty", () => {
    const g = grid([[["A"]], [["C"]]]);
    expect(titles(dropTab(g, tabId(g, "A"), paneOf(g, "C"), "center")!)).toEqual([[["C", "A"]]]);
  });

  it("splits into a new column on the left or right edge", () => {
    const g = grid([[["A", "B"]]]);
    expect(titles(dropTab(g, tabId(g, "B"), paneOf(g, "A"), "right")!)).toEqual([[["A"]], [["B"]]]);
    expect(titles(dropTab(g, tabId(g, "B"), paneOf(g, "A"), "left")!)).toEqual([[["B"]], [["A"]]]);
  });

  it("splits into a new pane on the top or bottom edge", () => {
    const g = grid([[["A", "B"]]]);
    expect(titles(dropTab(g, tabId(g, "B"), paneOf(g, "A"), "bottom")!)).toEqual([[["A"], ["B"]]]);
    expect(titles(dropTab(g, tabId(g, "B"), paneOf(g, "A"), "top")!)).toEqual([[["B"], ["A"]]]);
  });

  it("halves the space of the pane or column it splits", () => {
    const g = equalise(grid([[["A", "B"]], [["C"]]]));
    const n = dropTab(g, tabId(g, "B"), paneOf(g, "C"), "right")!;
    expect(columnsOf(n).map((c) => c.size)).toEqual([0.5, 0.25, 0.25]);
    const m = dropTab(g, tabId(g, "B"), paneOf(g, "C"), "bottom")!;
    expect(columnsOf(m)[1].panes.map((p) => p.size)).toEqual([0.5, 0.5]);
  });

  it("refuses a fourth pane in a row or a column", () => {
    const wide = grid([[["A", "D"]], [["B"]], [["C"]]]);
    expect(columnsOf(wide)).toHaveLength(MAX_SIBLINGS);
    expect(dropTab(wide, tabId(wide, "D"), paneOf(wide, "B"), "right")).toBeNull();
    expect(canDrop(wide, tabId(wide, "D"), paneOf(wide, "B"), "left")).toBe(false);
    expect(canDrop(wide, tabId(wide, "D"), paneOf(wide, "B"), "bottom")).toBe(true);
    const tall = grid([[["A", "D"], ["B"], ["C"]]]);
    expect(dropTab(tall, tabId(tall, "D"), paneOf(tall, "B"), "top")).toBeNull();
  });

  it("allows at most 9 panes", () => {
    const full = applyPreset(emptyGrid(), 3, 3, () => bible);
    expect(panes(full)).toHaveLength(MAX_PANES);
    const first = panes(full)[0];
    expect(insertTab(full, { kind: "library" }, first.id, "right")).toBeNull();
    expect(insertTab(full, { kind: "library" }, first.id, "center")).not.toBeNull();
  });

  it("splits one pane of a column side by side, leaving the rest of the column alone", () => {
    // Two tabs next to each other above one wide tab.
    const g = grid([[["A", "B"], ["W"]]]);
    const n = dropTab(g, tabId(g, "B"), paneOf(g, "A"), "right")!;
    expect(n.root && isSplit(n.root) && n.root.dir).toBe("v");
    const top = (n.root as Split).kids[0] as Split;
    expect(top.dir).toBe("h");
    expect(top.kids.map((k) => (k as Pane).tabs[0].title)).toEqual(["A", "B"]);
    expect(((n.root as Split).kids[1] as Pane).tabs[0].title).toBe("W");
    expect(top.kids.map((k) => k.size)).toEqual([0.5, 0.5]);
    expect((n.root as Split).kids.map((k) => k.size)).toEqual([0.5, 0.5]);
  });

  it("splits a pane below inside a row, and nests", () => {
    let g = grid([[["A", "B", "C"]]]);
    g = dropTab(g, tabId(g, "B"), paneOf(g, "A"), "right")!;
    g = dropTab(g, tabId(g, "C"), paneOf(g, "B"), "bottom")!;
    expect(panes(g)).toHaveLength(3);
    const root = g.root as Split;
    expect(root.dir).toBe("h");
    expect((root.kids[1] as Split).dir).toBe("v");
  });

  it("closing a pane collapses a split left with one child", () => {
    let g = grid([[["A", "B"], ["W"]]]);
    g = dropTab(g, tabId(g, "B"), paneOf(g, "A"), "right")!;
    g = closeTab(g, tabId(g, "B"));
    expect(titles(g)).toEqual([[["A"], ["W"]]]);
    g = closeTab(g, tabId(g, "W"));
    expect(g.root).toBe(panes(g)[0]);
    expect(g.root!.size).toBe(1);
  });

  it("allows a split that frees the space it needs", () => {
    // Moving C's only tab out of the third column leaves room for a new column.
    const g = grid([[["A"]], [["B"]], [["C"]]]);
    expect(titles(dropTab(g, tabId(g, "C"), paneOf(g, "A"), "left")!)).toEqual([[["C"]], [["A"]], [["B"]]]);
  });

  it("ignores drops that change nothing", () => {
    const g = grid([[["A", "B"]], [["C"]]]);
    expect(dropTab(g, tabId(g, "A"), paneOf(g, "A"), "center")).toBeNull();
    expect(dropTab(g, tabId(g, "C"), paneOf(g, "C"), "left")).toBeNull();
    expect(dropTab(g, "nope", paneOf(g, "C"), "left")).toBeNull();
  });

  it("splits a pane with its own tab when it has others", () => {
    const g = grid([[["A", "B"]]]);
    expect(titles(dropTab(g, tabId(g, "A"), paneOf(g, "A"), "right")!)).toEqual([[["B"]], [["A"]]]);
  });

  it("inserts new tabs dragged in from outside the grid", () => {
    const g = grid([[["A"]]]);
    const n = insertTab(g, book(7), paneOf(g, "A"), "bottom")!;
    expect(titles(n)).toEqual([[["A"], ["Book 7"]]]);
    expect(activeTab(findPane(n, n.selected)!.pane)?.title).toBe("Book 7");
    expect(titles(insertTab(g, book(7), paneOf(g, "A"), "center")!)).toEqual([[["A", "Book 7"]]]);
  });

  it("offers Move to… as the keyboard way to drop", () => {
    const g = grid([[["A", "B"]], [["C"]]]);
    expect(titles(moveTab(g, tabId(g, "B"), paneOf(g, "C"), "below")!)).toEqual([[["A"]], [["C"], ["B"]]]);
    expect(titles(moveTab(g, tabId(g, "B"), paneOf(g, "C"), "above")!)).toEqual([[["A"]], [["B"], ["C"]]]);
    expect(titles(moveTab(g, tabId(g, "B"), paneOf(g, "C"), "into")!)).toEqual([[["A"]], [["C", "B"]]]);
    expect(titles(moveTab(g, tabId(g, "B"), paneOf(g, "A"), "left")!)).toEqual([[["B"]], [["A"]], [["C"]]]);
  });
});

describe("linking", () => {
  it("moves every linked pane to the passage, and leaves unlinked ones", () => {
    let g = grid([[["A"]], [["B"]], [["C"]]]);
    g = setLinked(g, paneOf(g, "C"), false);
    g = setRef(g, paneOf(g, "A"), "Rom 8");
    const ref = (t: string) => paneRef(g, findPane(g, paneOf(g, t))!.pane);
    expect([ref("A"), ref("B"), ref("C")]).toEqual(["Rom 8", "Rom 8", "John 1"]);
    g = setRef(g, paneOf(g, "C"), "Ps 23");
    expect([ref("A"), ref("C")]).toEqual(["Rom 8", "Ps 23"]);
  });

  it("takes the group's passage when a pane links again", () => {
    let g = grid([[["A"]], [["B"]]]);
    g = setLinked(g, paneOf(g, "B"), false);
    g = setRef(g, paneOf(g, "B"), "Ps 23");
    g = setLinked(g, paneOf(g, "B"), true);
    expect(paneRef(g, findPane(g, paneOf(g, "B"))!.pane)).toBe("John 1");
  });
});

describe("presets", () => {
  const def = (): TabSpec => ({ kind: "bible", title: "default" });

  it("splits panes with several tabs before opening defaults", () => {
    const g = grid([[["A", "B", "C"]]]);
    const n = applyPreset(g, 2, 2, def);
    expect(titles(n)).toEqual([[["A"], ["B"]], [["C"], ["default"]]]);
    expect(shape(n)).toEqual([2, 2]);
  });

  it("merges extra panes into the last one", () => {
    const g = grid([[["A"], ["B"]], [["C"], ["D"]], [["E"]]]);
    expect(titles(applyPreset(g, 1, 2, def))).toEqual([[["A"], ["B", "C", "D", "E"]]]);
    expect(titles(applyPreset(g, 1, 1, def))).toEqual([[["A", "B", "C", "D", "E"]]]);
  });

  it("makes equal sizes and keeps within 3 × 3", () => {
    const n = applyPreset(emptyGrid(), 5, 4, def);
    expect(shape(n)).toEqual([3, 3]);
    expect(columnsOf(n).map((c) => c.size)).toEqual([1 / 3, 1 / 3, 1 / 3]);
    expect(panes(n)).toHaveLength(9);
    expect(n.selected).toBe(panes(n)[0].id);
  });

  it("keeps the selected pane when it survives", () => {
    let g = grid([[["A"]], [["B"]]]);
    g = selectTab(g, tabId(g, "B"));
    expect(applyPreset(g, 2, 2, def).selected).toBe(paneOf(g, "B"));
  });
});

describe("resizing", () => {
  it("moves space between neighbours, keeping a minimum", () => {
    expect(resizeBetween([0.5, 0.5], 0, 0.1)).toEqual([0.6, 0.4]);
    const [a, b] = resizeBetween([0.5, 0.5], 0, 0.9);
    expect(b).toBeCloseTo(MIN_SIZE);
    expect(a + b).toBeCloseTo(1);
    expect(resizeBetween([0.5, 0.5], 1, 0.1)).toEqual([0.5, 0.5]);
  });

  it("resizes columns and evens them out again", () => {
    let g = equalise(grid([[["A"]], [["B"]]]));
    g = resizeSplit(g, (g.root as Split).id, 0, 0.2);
    expect(columnsOf(g)[0].size).toBeCloseTo(0.7);
    expect(columnsOf(equalise(g)).map((c) => c.size)).toEqual([0.5, 0.5]);
  });
});

describe("saving", () => {
  it("restores what it saved", () => {
    let g = grid([[["A", "B"], ["C"]], [["D"]]]);
    g = setLinked(g, paneOf(g, "D"), false);
    g = updateTab(g, tabId(g, "A"), { section: 4 });
    g = selectTab(g, tabId(g, "C"));
    const back = restore(serialise(g));
    expect(back).toEqual(g);
    // New ids after a restore do not clash with old ones.
    const more = openTab(back, bible);
    const ids = panes(more).flatMap((p) => [p.id, ...p.tabs.map((t) => t.id)]);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("survives damaged saves", () => {
    expect(restore("{not json").root).toBeNull();
    expect(restore(null).root).toBeNull();
    expect(restore({ columns: [{ panes: [{ tabs: [{ kind: "virus" }] }] }] }).root).toBeNull();
    expect(restore({ root: { kids: [{ kids: [] }] } }).root).toBeNull();
    const g = restore({ columns: [{ panes: [{ tabs: [{ kind: "bible", id: "x" }, { kind: "book", id: "x" }], active: "zzz", size: -3 }] }] });
    const p = panes(g)[0];
    expect(p.tabs.map((t) => t.kind)).toEqual(["bible", "book"]);
    expect(new Set(p.tabs.map((t) => t.id)).size).toBe(2);
    expect(p.active).toBe(p.tabs[0].id);
    expect(p.size).toBe(1);
    expect(g.selected).toBe(p.id);
  });

  it("applies the 3 × 3 limit to saved layouts", () => {
    const col = (n: number) => ({ panes: Array.from({ length: n }, (_, i) => ({ tabs: [{ kind: "book", title: `x${i}` }] })) });
    const g = restore({ columns: [col(5), col(1), col(1), col(1)] });
    expect(shape(g)).toEqual([3, 3]);
    expect(columnsOf(g)[0].panes[2].tabs).toHaveLength(3);
    expect(columnsOf(g)[2].panes[0].tabs).toHaveLength(2);
  });

  it("opens layouts saved before splits, and keeps the new ones", () => {
    const old = { v: 1, columns: [{ id: "c1", size: 0.6, panes: [{ id: "p1", size: 0.5, tabs: [{ id: "t1", kind: "bible" }] }, { id: "p2", size: 0.5, tabs: [{ id: "t2", kind: "book" }] }] },
      { id: "c2", size: 0.4, panes: [{ id: "p3", size: 1, tabs: [{ id: "t3", kind: "list" }] }] }], selected: "p2", passage: "John 3", seq: 9 };
    const g = restore(old);
    expect(titles(g)).toEqual([[["bible"], ["book"]], [["list"]]]);
    expect(columnsOf(g).map((c) => c.size)).toEqual([0.6, 0.4]);
    expect(g.selected).toBe("p2");
    expect(g.passage).toBe("John 3");
    const nested = dropTab(g, "t2", "p1", "right")!;
    expect(restore(serialise(nested))).toEqual(nested);
  });

  it("applies the limits to a saved tree, and is not fooled by deep nesting", () => {
    const leaf = (n: number) => ({ tabs: [{ kind: "book", title: `x${n}` }] });
    const g = restore({ root: { kids: [leaf(1), leaf(2), leaf(3), leaf(4)], dir: "h" } });
    expect(panes(g)).toHaveLength(3);
    expect(panes(g)[2].tabs).toHaveLength(2);
    let deep: unknown = leaf(0);
    for (let i = 0; i < 50; i++) deep = { dir: i % 2 ? "h" : "v", kids: [deep, leaf(i)] };
    expect(() => restore({ root: deep })).not.toThrow();
    expect(panes(restore({ root: deep })).length).toBeLessThanOrEqual(MAX_PANES);
  });
});
