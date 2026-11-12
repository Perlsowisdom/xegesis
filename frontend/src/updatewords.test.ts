import { describe, expect, it } from "vitest";
import { lastCheckedText, versionLine } from "./updatewords";

const now = new Date("2026-11-12T12:00:00Z");

describe("lastCheckedText", () => {
  it("says so when there was no check", () => {
    expect(lastCheckedText(undefined, now)).toBe("Not checked yet.");
    expect(lastCheckedText("nonsense", now)).toBe("Not checked yet.");
  });
  it("counts minutes and hours", () => {
    expect(lastCheckedText("2026-11-12T12:00:00Z", now)).toBe("Last checked just now.");
    expect(lastCheckedText("2026-11-12T11:59:00Z", now)).toBe("Last checked 1 minute ago.");
    expect(lastCheckedText("2026-11-12T11:15:00Z", now)).toBe("Last checked 45 minutes ago.");
    expect(lastCheckedText("2026-11-12T09:00:00Z", now)).toBe("Last checked 3 hours ago.");
  });
  it("gives a date after a day", () => {
    expect(lastCheckedText("2026-11-01T09:00:00Z", now)).toMatch(/^Last checked on .*2026\.$/);
  });
});

describe("versionLine", () => {
  const info = { enabled: true, asked: true, current: "1.0.0", latest: "1.1.0", key_set: true };
  it("names a newer version", () => {
    expect(versionLine(info, true)).toBe("Version 1.1.0 is available. You have 1.0.0.");
  });
  it("otherwise just the current one", () => {
    expect(versionLine(info, false)).toBe("You have version 1.0.0.");
  });
});
