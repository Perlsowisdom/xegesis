import { describe, expect, it } from "vitest";
import { fitTabs } from "./sidebar";

describe("fitTabs", () => {
  it("shows everything when it fits", () => expect(fitTabs([50, 50, 50], 0, 150, 30)).toEqual([0, 1, 2]));
  it("drops the tail when it does not", () => expect(fitTabs([50, 50, 50, 50], 0, 150, 30)).toEqual([0, 1]));
  it("swaps the chosen tab into the strip", () => expect(fitTabs([50, 50, 50, 50], 3, 150, 30)).toEqual([0, 3]));
});
