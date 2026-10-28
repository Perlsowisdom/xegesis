// Licences in plain words. The interface offers four choices; the core's six licence types map onto
// them, and unknown licences behave as, and are shown as, personal copies.

export type Choice = "public_domain" | "free" | "personal" | "own";

export const CHOICES: [Choice, string, string][] = [
  ["public_domain", "Public domain", "Anyone may copy and share it."],
  ["free", "Free to share", "Creative Commons and similar open licences."],
  ["personal", "Personal copy", "Bought or borrowed; quotations are limited."],
  ["own", "My own work", "You wrote it."],
];

export interface LicenceLike {
  type?: string;
  share_allowed?: boolean;
  origin?: string;
}

export function choiceOf(l: LicenceLike | null | undefined): Choice {
  switch (l?.type) {
    case "public_domain": return "public_domain";
    case "creative_commons": return "free";
    case "open_other": return l.share_allowed === false ? "personal" : "free";
    case "own_work": return "own";
  }
  return "personal";
}

export function choiceName(c: Choice): string {
  return CHOICES.find(([k]) => k === c)?.[1] ?? "Personal copy";
}

// plainName names a licence, or a bare licence type as the catalogue lists it.
export function plainName(l: LicenceLike | string | null | undefined): string {
  return choiceName(choiceOf(typeof l === "string" ? { type: l } : l));
}

// originText says where a licence came from, for a book or a Bible.
export function originText(origin: string | undefined, what = "book"): string {
  switch (origin) {
    case "file": return `from the ${what}'s own details`;
    case "default": return `no licence stated in the ${what}`;
    case "user": return "set by you";
  }
  return "";
}

export interface Extra {
  spdx?: string;
  rights_holder?: string;
  url?: string;
  attribution?: string;
  print_pages?: number;
  print_percent?: number;
  export_pages?: number;
  export_percent?: number;
}

// licenceParams turns a choice and the More options fields into library.set_licence parameters. A
// licence ID that is not Creative Commons, such as GFDL, is recorded as another open licence.
export function licenceParams(c: Choice, x: Extra = {}): Record<string, unknown> {
  const p: Record<string, unknown> = {};
  const spdx = x.spdx?.trim();
  switch (c) {
    case "public_domain": p.type = "public_domain"; break;
    case "own": p.type = "own_work"; break;
    case "personal": p.type = "personal_purchase"; break;
    case "free":
      if (spdx && !/^cc/i.test(spdx)) {
        p.type = "open_other";
        p.share_allowed = true;
      } else p.type = "creative_commons";
  }
  if (spdx) p.spdx = spdx;
  for (const k of ["rights_holder", "url", "attribution"] as const) {
    const v = x[k]?.trim();
    if (v) p[k] = v;
  }
  const limit = (pages?: number, percent?: number) => {
    const l: Record<string, number> = {};
    if (pages && pages > 0) l.pages = pages;
    if (percent && percent > 0) l.percent = percent;
    return Object.keys(l).length ? l : undefined;
  };
  const print = limit(x.print_pages, x.print_percent);
  const exp = limit(x.export_pages, x.export_percent);
  if (print) p.print_limit = print;
  if (exp) p.export_limit = exp;
  return p;
}
