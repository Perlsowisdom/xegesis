// Types and traditions: the choosers used for one book or a selection, and the small panel for the
// traditions list and its rules. Everything here calls facet.* commands. Values are set
// automatically and shown with their reason; these are how the user changes them.

import { Message, Rule, Vocabulary, run } from "./api";
import { button, clear, field, h, select } from "./dom";
import { AUTO, KEEP, setParams } from "./facetwords";

const NEW = "\u0000new";

export interface FacetChooser {
  el: HTMLElement;
  // value is the parameters for facet.set, or null when nothing was picked.
  value(ids: string[]): Record<string, unknown> | null;
}

// facetChooser builds the type and tradition pickers. For a selection ("bulk") each starts at
// "Leave as it is"; for one book it starts at the book's current value.
export function facetChooser(vocab: Vocabulary, bulk: boolean, now: { type?: string; typeBy?: string; tradition?: string; traditionBy?: string } = {}): FacetChooser {
  const first: [string, string][] = bulk ? [[KEEP, "Leave as it is"]] : [];
  const auto: [string, string] = [AUTO, "Let Xegesis decide"];
  const type = select([...first, auto, ...vocab.types.map((t): [string, string] => [t, t])], bulk ? KEEP : now.typeBy === "user" ? now.type! : AUTO);
  const startType = type.value;
  const mine = now.traditionBy === "user";
  const known = mine && vocab.traditions.includes(now.tradition ?? "");
  const tradition = select([...first, auto, ...vocab.traditions.map((t): [string, string] => [t, t]), [NEW, "Another tradition…"]],
    bulk ? KEEP : known ? now.tradition! : mine ? NEW : AUTO);
  const startTradition = tradition.value;
  const other = h("input", { type: "text", autocomplete: "off", placeholder: "Name of the tradition", value: mine && !known ? now.tradition || "" : "" });
  const otherField = field("Tradition name", other);
  const sync = () => { otherField.hidden = tradition.value !== NEW; };
  tradition.addEventListener("change", sync);
  sync();
  const el = h("div", { class: "grid2" }, field("Type", type), field("Tradition", tradition), otherField);
  return {
    el,
    value(ids) {
      const t = tradition.value === NEW ? other.value.trim() || KEEP : tradition.value;
      const ty = !bulk && type.value === startType ? KEEP : type.value;
      const tr = !bulk && t === startTradition ? KEEP : t;
      if (ty === KEEP && tr === KEEP) return null;
      return setParams(ids, ty, tr);
    },
  };
}

// traditionsPanel is the "Traditions and rules" section: the list of traditions and the rules that
// tag books automatically. changed runs after anything that may change which books carry a tradition.
export function traditionsPanel(changed: () => void) {
  const msg = new Message();
  const body = h("div", {});
  const el = h("details", { class: "more" }, h("summary", {}, "Traditions and rules"), msg.el, body);
  let loaded = false;
  el.addEventListener("toggle", () => { if (el.open && !loaded) { loaded = true; render(); } });

  async function render() {
    const [vocab, rules] = await Promise.all([
      msg.attempt(() => run<Vocabulary>("facet.vocabulary")),
      msg.attempt(() => run<{ rules: Rule[] }>("facet.rules", { action: "list" })),
    ]);
    if (!vocab || !rules) return;
    const name = h("input", { type: "text", autocomplete: "off" });
    const text = h("input", { type: "text", autocomplete: "off", placeholder: "e.g. Calvin" });
    const fieldSel = select([["author", "the author"], ["publisher", "the publisher"], ["series", "the series"], ["folder", "the folder"]], "author");
    const value = select(vocab.traditions);
    clear(body,
      h("h4", {}, "Your traditions"),
      h("ul", { class: "facets" }, vocab.traditions.map((t) => h("li", {}, t,
        button("×", () => editTraditions({ action: "remove", name: t }), { class: "icon", "aria-label": `Remove ${t} from the list` })))),
      h("div", { class: "row wrap end" }, field("Add a tradition", name),
        button("Add", () => name.value.trim() && editTraditions({ action: "add", name: name.value.trim() }))),
      h("h4", {}, "Rules"),
      h("p", { class: "hint" }, "Books that match a rule get that tradition automatically, now and whenever books are added. " +
        "A tradition you set yourself is never changed by a rule."),
      rules.rules.length === 0 ? h("p", { class: "empty" }, "No rules yet.") :
        h("ul", { class: "files" }, rules.rules.map((r) => h("li", {},
          `If ${ruleWords(r)}, the tradition is ${r.value} `,
          button("Remove", () => editRules({ action: "remove", id: r.id }), { class: "danger", "aria-label": `Remove the rule for ${r.text}` })))),
      h("div", { class: "row wrap end" }, field("If", fieldSel), field("contains", text), field("the tradition is", value),
        button("Add rule", () => text.value.trim() && editRules({ action: "add", field: fieldSel.value, text: text.value.trim(), value: value.value }),
          { class: "primary" })));
  }

  async function editTraditions(p: Record<string, unknown>) {
    if (await msg.attempt(() => run("facet.traditions", p))) {
      msg.clear();
      render();
      changed();
    }
  }

  async function editRules(p: Record<string, unknown>) {
    const r = await msg.attempt(() => run<{ summary: string }>("facet.rules", p));
    if (!r) return;
    msg.ok(r.summary);
    render();
    changed();
  }

  return { el, reload: () => { if (loaded) render(); } };
}

function ruleWords(r: Rule): string {
  return r.field === "folder" ? `the folder name contains "${r.text}"` : `the ${r.field} contains "${r.text}"`;
}
