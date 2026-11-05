import { describe, expect, it } from "vitest";
import { followsLink } from "./linkclick";

const base = { button: 0, shift: false, alt: false, hadSelection: false, hasSelection: false };
describe("followsLink", () => {
  it("follows on a plain click", () => expect(followsLink(base)).toBe(true));
  it("places the caret when text is selected, or with Shift or Alt", () => {
    expect(followsLink({ ...base, hadSelection: true })).toBe(false);
    expect(followsLink({ ...base, hasSelection: true })).toBe(false);
    expect(followsLink({ ...base, shift: true })).toBe(false);
    expect(followsLink({ ...base, alt: true })).toBe(false);
    expect(followsLink({ ...base, button: 1 })).toBe(false);
  });
});
