import { describe, expect, it } from "vitest";
import { AssistantConfig, draftParts, linkRefs, providerLine, runningLabel, segments } from "./assistantwords";

describe("segments", () => {
  it("splits drafts from text", () => {
    const s = segments("Here is an outline.\n```draft\n# Hope\n- one\n```\nUse it well.");
    expect(s).toEqual([
      { kind: "text", text: "Here is an outline.\n" },
      { kind: "draft", text: "# Hope\n- one" },
      { kind: "text", text: "\nUse it well." },
    ]);
  });
  it("marks a draft still being written", () => {
    expect(segments("Text\n```draft\n# Half")).toEqual([{ kind: "text", text: "Text\n" }, { kind: "draft", text: "# Half", open: true }]);
  });
  it("leaves other code blocks as text", () => {
    expect(segments("```js\nx\n```")).toEqual([{ kind: "text", text: "```js\nx\n```" }]);
  });
});

describe("draftParts", () => {
  it("takes the first heading as the title", () => {
    expect(draftParts("\n# Hope in Romans 8\n\nBody text")).toEqual({ title: "Hope in Romans 8", body: "Body text" });
  });
  it("uses the first words without a heading", () => {
    expect(draftParts("God works all things for good for those who love him.").title).toBe("God works all things for good for those");
  });
});

describe("linkRefs", () => {
  it("links found references and skips existing links", () => {
    const text = "See Romans 8:28 and [John 3:16](#passage/John.3.16).";
    const out = linkRefs(text, [{ start: 4, end: 15, ref: "Rom.8.28" }, { start: 21, end: 31, ref: "John.3.16" }]);
    expect(out).toBe("See [Romans 8:28](#passage/Rom.8.28) and [John 3:16](#passage/John.3.16).");
  });
});

describe("words", () => {
  const cfg = (p: Partial<AssistantConfig>): AssistantConfig => ({ provider: "openai", label: "OpenAI", local: false, key_saved: false, needs_key: true, confirmed: false, presets: [], ...p });
  it("names the provider in plain words", () => {
    expect(providerLine(cfg({}))).toBe("OpenAI, with your own key");
    expect(providerLine(cfg({ provider: "off", label: undefined }))).toBe("Off");
    expect(providerLine(cfg({ provider: "local", label: "Local model", local: true }))).toBe("Local model, on this computer");
    expect(providerLine(cfg({ provider: "custom", label: "Custom", local: false }))).toBe("Custom address");
  });
  it("says what a running tool does", () => {
    expect(runningLabel("read_passage")).toBe("Reading the passage…");
    expect(runningLabel("other")).toBe("Working…");
  });
});
