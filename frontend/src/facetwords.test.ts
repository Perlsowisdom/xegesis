import { describe, expect, it } from "vitest";
import { AUTO, KEEP, NOT_SET, byText, countText, facetValue, reasonText, setParams } from "./facetwords";

describe("reasonText", () => {
  it("gives the reason for an automatic value", () => {
    expect(reasonText("auto", "title mentions a commentary on Romans")).toBe("title mentions a commentary on Romans");
    expect(reasonText("auto")).toBe("set automatically");
  });
  it("says a chosen value was set by you", () => {
    expect(reasonText("user", "ignored")).toBe("set by you");
    expect(reasonText(undefined)).toBe("");
  });
});

describe("byText and facetValue", () => {
  it("uses plain words", () => {
    expect(byText("auto")).toBe("set automatically");
    expect(byText("user")).toBe("set by you");
    expect(facetValue("")).toBe(NOT_SET);
    expect(facetValue("Sermons")).toBe("Sermons");
  });
});

describe("setParams", () => {
  it("leaves out what is kept", () => {
    expect(setParams(["a"], KEEP, "Baptist")).toEqual({ item_ids: ["a"], tradition: "Baptist" });
  });
  it("sends an empty text to go back to automatic", () => {
    expect(setParams(["a", "b"], AUTO, KEEP)).toEqual({ item_ids: ["a", "b"], type: "" });
  });
});

describe("countText", () => {
  it("counts books", () => {
    expect(countText(1)).toBe("1 book");
    expect(countText(3)).toBe("3 books");
  });
});
