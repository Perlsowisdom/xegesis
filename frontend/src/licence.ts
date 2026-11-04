// Choosing a licence in plain words: four choices, with the exact terms under More options. Used for
// books in the Library and for installed Bibles. Nothing asks for a licence up front; the core gives
// each book or Bible the licence it states, or Personal copy, and this is how the user changes it.

import { Licence } from "./api";
import { segmented, segmentedValue, setSegmented } from "./controls";
import { button, field, h } from "./dom";
import { CHOICES, Choice, choiceOf, licenceParams, originText, plainName } from "./licencewords";

export { originText, plainName } from "./licencewords";

export interface LicenceChooser {
  el: HTMLElement;
  // value is the parameters for library.set_licence.
  value(): Record<string, unknown>;
  set(l?: Partial<Licence> | null): void;
}

// licenceChooser builds the four choices. limits adds print and export limits to More options.
export function licenceChooser(legend: string, limits = true): LicenceChooser {
  const hints = h("small", { class: "hint", "aria-live": "polite" });
  const showHint = (c: string) => { hints.textContent = CHOICES.find((x) => x[0] === c)?.[2] ?? ""; };
  const pills = segmented<Choice>({ label: legend, options: CHOICES.map(([value, label]) => [value as Choice, label]), value: "personal", onChange: showHint });
  showHint("personal");
  const text = (placeholder = "", type = "text") => h("input", { type, placeholder, autocomplete: "off", spellcheck: "false" });
  const spdx = text("e.g. CC-BY-SA-4.0");
  const holder = text();
  const url = text("https://", "url");
  const credit = h("textarea", { rows: 2 });
  const num = () => h("input", { type: "number", min: "0", step: "1", inputmode: "numeric" });
  const printPages = num(), printPercent = num(), exportPages = num(), exportPercent = num();

  const more = h("details", { class: "more" }, h("summary", {}, "More options"),
    h("div", { class: "grid2" },
      field("Licence ID", spdx, "Optional, for Free to share"),
      field("Rights holder", holder),
      field("Licence web address", url)),
    field("Credit line to show with quotations", credit),
    limits ? h("div", { class: "grid2" },
      field("Print at most (pages)", printPages), field("Print at most (% of the book)", printPercent),
      field("Copy or export at most (pages)", exportPages), field("Copy or export at most (% of the book)", exportPercent)) : null,
    limits ? h("p", { class: "hint" }, "Leave the limits empty to use the usual ones for the licence.") : null);

  const el = h("fieldset", { class: "licence" }, h("legend", {}, legend),
    pills, hints, more);

  const chosen = (): Choice => (segmentedValue(pills) as Choice) || "personal";
  const n = (i: HTMLInputElement) => Number(i.value) || undefined;

  return {
    el,
    value: () => licenceParams(chosen(), {
      spdx: spdx.value, rights_holder: holder.value, url: url.value, attribution: credit.value,
      print_pages: n(printPages), print_percent: n(printPercent), export_pages: n(exportPages), export_percent: n(exportPercent),
    }),
    set(l) {
      const c = choiceOf(l);
      setSegmented(pills, c); showHint(c);
      spdx.value = l?.spdx ?? "";
      holder.value = l?.rights_holder ?? "";
      url.value = l?.url ?? "";
      credit.value = l?.attribution ?? "";
    },
  };
}

// licenceLine shows "Licence: Personal copy · no licence stated in the book · change". change opens
// the caller's editor.
export function licenceLine(l: Partial<Licence> | null | undefined, what: string, change: () => void): HTMLElement {
  const from = originText(l?.origin, what);
  return h("span", { class: "licence-line" }, "Licence: ", h("strong", {}, plainName(l)),
    l?.spdx ? ` (${l.spdx})` : "", from ? h("small", { class: "hint" }, ` · ${from}`) : null, " · ",
    button("change", change, { class: "link", "aria-label": `Change the licence of this ${what}` }));
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
