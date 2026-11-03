// One commentary in a pane, following the pane's passage: what it says on each verse there.

import { Message, run } from "../api";
import { clear, h } from "../dom";
import { paragraphs } from "../markup";
import { DocHost, DocView } from "./view";

interface Commentary { module: string; name: string; attribution?: string; entries: { ref: string; markup?: string; text: string }[] }

export function commentaryDoc(host: DocHost): DocView {
  const msg = new Message();
  const input = h("input", { type: "search", value: host.ref(), autocomplete: "off", class: "place-input", "aria-label": "Passage" });
  const placeBox = h("form", { class: "passage-box" }, input);
  placeBox.addEventListener("submit", (e) => {
    e.preventDefault();
    if (input.value.trim()) host.go(input.value.trim());
  });
  const text = h("article", { class: "doc-text commentary-text", tabindex: "-1" });
  const el = h("div", { class: "doc doc-commentary" }, msg.el, text);
  let shown = "";
  let seq = 0;

  async function read(ref: string) {
    const n = ++seq;
    shown = ref;
    input.value = ref;
    const module = host.tab().modules?.[0];
    if (!ref) return clear(text, h("p", { class: "empty" }, "Type a passage in the box above."));
    const r = await msg.attempt(() => run<{ commentaries: Commentary[] }>("reference.for_passage", { ref }));
    if (!r || n !== seq) return;
    const c = r.commentaries.find((x) => x.module === module) ?? (module ? undefined : r.commentaries[0]);
    if (c && host.tab().title !== c.name) host.update({ title: c.name, modules: [c.module] });
    clear(text, !c || c.entries.length === 0
      ? h("p", { class: "empty" }, `This commentary has nothing on ${ref}.`)
      : [h("h2", { class: "module-name" }, `${c.name} on ${ref}`),
        c.entries.map((e) => h("section", { class: "entry" }, h("h3", {}, e.ref), paragraphs(e.markup || e.text, { onRef: (osis: string) => host.go(osis) }))),
        c.attribution ? h("footer", { class: "attribution" }, c.attribution) : null]);
  }

  void read(host.ref());
  return {
    el,
    placeBox,
    refChanged: (ref) => { if (ref !== shown) void read(ref); },
    focus: () => input.select(),
    destroy: () => { seq++; },
  };
}
