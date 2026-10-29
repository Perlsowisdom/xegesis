import { describe, expect, it } from "vitest";
import { hasWords, kindLabel, parseMarkup, plainText, strongNumbers } from "./markupwords";

describe("parseMarkup", () => {
  it("reads tagged words, references and notes", () => {
    const nodes = parseMarkup(`<h>Love</h><w s="G25" m="V-AAI-3S" g="loved">ἠγάπησεν</w> &amp; <r o="Rom.5.8">Rom 5:8</r><f>note</f><p/>`);
    expect(nodes.map((n) => n.tag)).toEqual(["h", "w", "", "r", "f", "p"]);
    expect(nodes[1].attrs).toEqual({ s: "G25", m: "V-AAI-3S", g: "loved" });
    expect(nodes[2].text).toBe(" & ");
    expect(plainText(nodes)).toBe("ἠγάπησεν & Rom 5:8");
  });

  it("survives unbalanced tags", () => {
    const nodes = parseMarkup(`a</j><j>b<w s="H1">c</j>d`);
    expect(plainText(nodes)).toBe("abcd");
  });
});

describe("helpers", () => {
  it("finds tagged words and numbers", () => {
    expect(hasWords(`<w s="H7225">x</w>`)).toBe(true);
    expect(hasWords(`plain`)).toBe(false);
    expect(strongNumbers("H9003 H7225G x")).toEqual(["H9003", "H7225G"]);
    expect(kindLabel("crossref")).toBe("Cross-references");
  });
});
