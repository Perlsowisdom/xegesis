import { beforeEach, describe, expect, it, vi } from "vitest";

const answers: unknown[] = [];
const calls: string[] = [];
vi.mock("./api", () => ({
  run: async (id: string) => {
    calls.push(id);
    const a = answers.shift();
    if (a instanceof Error) throw a;
    return a;
  },
}));

const kjv = { id: "kjv", abbreviation: "KJV" };
const web = { id: "web", abbreviation: "WEB" };

describe("installed Bibles", () => {
  beforeEach(() => {
    vi.resetModules();
    answers.length = 0;
    calls.length = 0;
  });

  it("asks again after an empty answer, and keeps a non-empty one", async () => {
    const b = await import("./bibles");
    answers.push([], [kjv]);
    expect(await b.installedBibles()).toEqual([]);
    expect(await b.installedBibles()).toEqual([kjv]);
    expect(await b.installedBibles()).toEqual([kjv]);
    expect(calls.length).toBe(2);
  });

  it("asks again after a failure", async () => {
    const b = await import("./bibles");
    answers.push(new Error("busy"), [kjv]);
    expect(await b.installedBibles()).toEqual([]);
    expect(await b.installedBibles()).toEqual([kjv]);
  });

  it("tells listeners when the list changes, and only then", async () => {
    const b = await import("./bibles");
    const seen: unknown[] = [];
    b.onBibles((l) => seen.push(l.map((m) => m.id)));
    answers.push([kjv], [kjv], [kjv, web]);
    await b.installedBibles();
    await b.biblesChanged();
    await b.biblesChanged();
    expect(seen).toEqual([["kjv"], ["kjv", "web"]]);
    expect(await b.installedBibles()).toEqual([kjv, web]);
  });
});
