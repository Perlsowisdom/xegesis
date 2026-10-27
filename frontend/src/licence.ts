// A licence form used wherever the user states the terms of a text: installing a Bible and
// answering licence questions after a library scan. Licence types and their default terms come
// from the core (licence.defaults).

import { Licence, Message, paramEnum, run } from "./api";
import { field, h, id, select } from "./dom";

export const TYPE_LABELS: Record<string, string> = {
  public_domain: "Public domain",
  creative_commons: "Creative Commons",
  open_other: "Other open licence",
  personal_purchase: "Bought for personal use",
  own_work: "My own work",
  unknown: "Not sure",
};

export function typeLabel(t: string | undefined): string {
  return t ? TYPE_LABELS[t] ?? t : "Undecided";
}

export interface LicenceValue {
  type: string;
  spdx?: string;
  rights_holder?: string;
  attribution?: string;
  url?: string;
}

export interface LicenceForm {
  el: HTMLElement;
  value(): LicenceValue;
  set(l: Partial<Licence>): void;
}

// licenceForm builds the fields. withUrl adds a web address field.
export function licenceForm(legend: string, withUrl = true): LicenceForm {
  const type = select([], "", { required: true });
  const spdx = h("input", { type: "text", placeholder: "e.g. CC-BY-SA-4.0", autocomplete: "off", spellcheck: "false" });
  const holder = h("input", { type: "text", autocomplete: "off" });
  const attribution = h("textarea", { rows: 2 });
  const url = h("input", { type: "url", placeholder: "https://", autocomplete: "off" });
  const terms = h("p", { class: "hint", id: id("terms"), "aria-live": "polite" });
  const msg = new Message();
  type.setAttribute("aria-describedby", terms.id);

  paramEnum("licence.defaults", "type").then((types) => {
    const current = type.value;
    type.replaceChildren(...types.map((t) => h("option", { value: t }, typeLabel(t))));
    type.value = types.includes(current) ? current : types.includes("unknown") ? "unknown" : types[0] ?? "";
    describe();
  });

  async function describe() {
    if (!type.value) return;
    const d = await msg.attempt(() => run<Licence>("licence.defaults", { type: type.value, spdx: spdx.value.trim() }));
    if (d) terms.textContent = termsText(d);
  }
  type.addEventListener("change", describe);
  spdx.addEventListener("change", describe);

  const el = h("fieldset", { class: "licence" },
    h("legend", {}, legend),
    h("div", { class: "grid2" },
      field("Licence type", type),
      field("SPDX or Creative Commons ID", spdx, "Optional")),
    terms,
    h("div", { class: "grid2" },
      field("Rights holder", holder),
      withUrl ? field("Licence web address", url) : null),
    field("Attribution to show with quotations", attribution),
    msg.el);

  return {
    el,
    value() {
      const v: LicenceValue = { type: type.value };
      const put = (k: keyof LicenceValue, s: string) => { if (s.trim()) v[k] = s.trim(); };
      put("spdx", spdx.value);
      put("rights_holder", holder.value);
      put("attribution", attribution.value);
      if (withUrl) put("url", url.value);
      return v;
    },
    set(l) {
      if (l.type) type.value = l.type;
      spdx.value = l.spdx ?? "";
      holder.value = l.rights_holder ?? "";
      attribution.value = l.attribution ?? "";
      url.value = l.url ?? "";
      describe();
    },
  };
}

function limit(l: Licence["print_limit"]): string {
  if (!l) return "no limit";
  const parts = [];
  if (l.pages) parts.push(`${l.pages} pages`);
  if (l.percent) parts.push(`${l.percent}%`);
  return parts.length ? parts.join(" or ") : "no limit";
}

// termsText says in one line what a licence allows.
export function termsText(l: Licence): string {
  const yes = (b?: boolean) => (b ? "yes" : "no");
  return `Sharing: ${yes(l.share_allowed)} · Commercial use: ${yes(l.commercial_allowed)} · ` +
    `Print: ${limit(l.print_limit)} · Export: ${limit(l.export_limit)}`;
}
