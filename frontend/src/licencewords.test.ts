import { describe, expect, it } from "vitest";
import { choiceOf, licenceParams, originText, plainName } from "./licencewords";

describe("plainName", () => {
  it("uses four plain words", () => {
    expect(plainName("public_domain")).toBe("Public domain");
    expect(plainName("creative_commons")).toBe("Free to share");
    expect(plainName("personal_purchase")).toBe("Personal copy");
    expect(plainName("own_work")).toBe("My own work");
  });
  it("shows unknown and missing licences as personal copies", () => {
    expect(plainName("unknown")).toBe("Personal copy");
    expect(plainName(undefined)).toBe("Personal copy");
  });
  it("names other open licences by what they allow", () => {
    expect(plainName({ type: "open_other", share_allowed: true })).toBe("Free to share");
    expect(plainName({ type: "open_other", share_allowed: false })).toBe("Personal copy");
  });
});

it("originText says where a licence came from", () => {
  expect(originText("file")).toBe("from the book's own details");
  expect(originText("default", "Bible")).toBe("no licence stated in the Bible");
  expect(originText("user")).toBe("set by you");
  expect(originText(undefined)).toBe("");
});

describe("licenceParams", () => {
  it("maps choices to licence types", () => {
    expect(licenceParams("personal")).toEqual({ type: "personal_purchase" });
    expect(licenceParams("own")).toEqual({ type: "own_work" });
    expect(choiceOf({ type: "unknown" })).toBe("personal");
  });
  it("keeps Creative Commons IDs and records other IDs as open licences", () => {
    expect(licenceParams("free", { spdx: " CC-BY-4.0 " })).toEqual({ type: "creative_commons", spdx: "CC-BY-4.0" });
    expect(licenceParams("free", { spdx: "GFDL-1.3" })).toEqual({ type: "open_other", share_allowed: true, spdx: "GFDL-1.3" });
  });
  it("passes More options and limits", () => {
    expect(licenceParams("personal", { rights_holder: "Press", print_pages: 20, export_percent: 5, url: " " }))
      .toEqual({ type: "personal_purchase", rights_holder: "Press", print_limit: { pages: 20 }, export_limit: { percent: 5 } });
  });
});
