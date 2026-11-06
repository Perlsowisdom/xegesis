import { describe, expect, it } from "vitest";
import {
  BinEntry, FolderRow, binContents, binKind, canNestUnder, editedText, firstLine, folderPath, folderTree, moveTargets, newChoices,
  nextSort, preachedText, purgeQuestion, scopeHash, scopeOf, sortSermons,
} from "./workwords";

const f = (id: string, name: string, sort: number, parent_id?: string, notes = 0, documents = 0): FolderRow =>
  ({ id, name, sort, parent_id, notes, documents });

const rows = [f("b", "Romans", 1, undefined, 2), f("a", "Advent", 0, undefined, 1, 1), f("c", "Week 2", 1, "a", 3), f("d", "Week 1", 0, "a", 0, 2),
  f("e", "Lost", 0, "zz", 1), f("g", "Deep", 0, "c")];

describe("the folder tree", () => {
  it("nests one level in order and adds up what is inside", () => {
    const t = folderTree(rows);
    expect(t.map((x) => x.name)).toEqual(["Advent", "Deep", "Lost", "Romans"]);
    expect(t[0].children.map((x) => x.name)).toEqual(["Week 1", "Week 2"]);
    expect(t[0].total).toBe(2 + 2 + 3);
  });
  it("shows a folder whose parent is missing or nested at the top", () => {
    const t = folderTree(rows);
    expect(t.find((x) => x.id === "e")?.children).toEqual([]);
    expect(t.find((x) => x.id === "g")).toBeTruthy();
  });
  it("only nests under a top folder, and not a folder that holds folders", () => {
    expect(canNestUnder(rows, "b", "a")).toBe(true);
    expect(canNestUnder(rows, "a", "b")).toBe(false);
    expect(canNestUnder(rows, "b", "c")).toBe(false);
    expect(canNestUnder(rows, "b", "b")).toBe(false);
  });
  it("names paths and move targets", () => {
    expect(folderPath(rows, "c")).toBe("Advent / Week 2");
    expect(folderPath(rows, "b")).toBe("Romans");
    expect(folderPath(rows, "nope")).toBe("");
    const m = moveTargets(rows.slice(0, 4));
    expect(m.map((x) => `${x.depth}${x.label}`)).toEqual(["0No folder", "0Advent", "1Week 1", "1Week 2", "0Romans"]);
  });
});

describe("addresses", () => {
  it("reads and writes the notes view's scope", () => {
    expect(scopeOf("#notes")).toEqual({ kind: "all" });
    expect(scopeOf("#notes/folder/01AB")).toEqual({ kind: "folder", id: "01AB" });
    expect(scopeOf("#notes/tag/grace%20alone")).toEqual({ kind: "tag", tag: "grace alone" });
    expect(scopeHash({ kind: "tag", tag: "grace alone" })).toBe("notes/tag/grace%20alone");
    expect(scopeOf("#" + scopeHash({ kind: "folder", id: "x/y" }))).toEqual({ kind: "folder", id: "x/y" });
  });
});

describe("sorting sermons", () => {
  const s = [
    { id: "1", title: "Grace", passage: { display: "Eph 2", start: 49002001 }, preached_on: "2026-03-01", theme: "grace", updated_at: 1 },
    { id: "2", title: "advent hope", passage: { display: "Isa 9", start: 23009001 }, preached_on: "2025-12-01", updated_at: 2 },
    { id: "3", title: "Draft", updated_at: 3 },
  ];
  it("sorts by each column with blanks last", () => {
    expect(sortSermons(s, "title", "asc").map((x) => x.id)).toEqual(["2", "3", "1"]);
    expect(sortSermons(s, "passage", "asc").map((x) => x.id)).toEqual(["2", "1", "3"]);
    expect(sortSermons(s, "passage", "desc").map((x) => x.id)).toEqual(["1", "2", "3"]);
    expect(sortSermons(s, "preached", "desc").map((x) => x.id)).toEqual(["1", "2", "3"]);
    expect(sortSermons(s, "theme", "asc").map((x) => x.id)).toEqual(["1", "3", "2"]);
  });
  it("flips the same column and starts a new one sensibly", () => {
    expect(nextSort({ key: "title", dir: "asc" }, "title")).toEqual({ key: "title", dir: "desc" });
    expect(nextSort({ key: "title", dir: "asc" }, "preached")).toEqual({ key: "preached", dir: "desc" });
    expect(nextSort({ key: "preached", dir: "desc" }, "theme")).toEqual({ key: "theme", dir: "asc" });
  });
  it("shows a date preached without moving it a day", () => {
    expect(preachedText("2026-11-01", "en-GB")).toBe("1 Nov 2026");
    expect(preachedText("")).toBe("");
    expect(preachedText("nonsense")).toBe("");
  });
});

describe("cards and the bin", () => {
  it("takes a note's first line without marks", () => {
    expect(firstLine("\n## A **bold** start\nmore")).toBe("A bold start");
    expect(firstLine("> See [Rom 8:28](#passage/Rom.8.28) here")).toBe("See Rom 8:28 here");
    expect(firstLine("")).toBe("");
    expect(firstLine("x".repeat(200), 10)).toBe("xxxxxxxxx…");
  });
  it("says when something was edited", () => {
    const now = new Date(2026, 10, 3, 12).getTime();
    expect(editedText(now - 30_000, now)).toBe("Edited just now");
    expect(editedText(now - 5 * 60_000, now)).toBe("Edited 5 min ago");
    expect(editedText(now - 3 * 3600_000, now)).toBe("Edited 3 hours ago");
    expect(editedText(now - 26 * 3600_000, now)).toBe("Edited yesterday");
  });
  it("describes bin entries and asks before deleting for good", () => {
    const folder: BinEntry = { kind: "folder", id: "f", title: "Series", deleted_at: 1, folders: 1, notes: 2, documents: 1 };
    expect(binKind(folder)).toBe("Folder");
    expect(binContents(folder)).toBe("With 1 folder, 2 notes and 1 sermon");
    expect(purgeQuestion(folder)).toBe(`Delete the folder "Series" and everything in it for good? That is 1 folder, 2 notes and 1 sermon. This cannot be undone.`);
    const note: BinEntry = { kind: "note", id: "n", title: "", deleted_at: 1 };
    expect(purgeQuestion(note)).toBe(`Delete "Untitled note" for good? This cannot be undone.`);
    expect(binKind({ kind: "document", id: "d", title: "x", sort: "lesson", deleted_at: 1 })).toBe("Lesson");
    expect(binKind({ kind: "list", id: "l", title: "x", sort: "words", deleted_at: 1 })).toBe("Word list");
    expect(binContents({ ...folder, folders: 0, notes: 0, documents: 0 })).toBe("Empty");
  });
  it("offers the matching thing first in + New", () => {
    expect(newChoices("notes", "Romans")).toEqual([{ kind: "note", label: "Note in Romans" }, { kind: "handwritten", label: "Handwritten note in Romans" }, { kind: "sermon", label: "Sermon in Romans" }]);
    expect(newChoices("sermons").map((c) => c.label)).toEqual(["Sermon", "Note", "Handwritten note"]);
    const ink = ["```ink", '{"v":1,"h":300,"s":[]}', "```"].join("\n");
    expect(firstLine(ink)).toBe("Handwriting");
    expect(firstLine(`${ink}\n\nGrace first`)).toBe("Grace first");
  });
});
