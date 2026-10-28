// Book types and traditions in plain words. A value is set automatically (with a reason) or by the
// user; the interface says which, and never shows the origin names the core stores.

export type SetBy = "auto" | "user" | "" | undefined;

export const NOT_SET = "Not set yet";

// byText says who chose a value.
export function byText(by: SetBy): string {
  return by === "user" ? "set by you" : by === "auto" ? "set automatically" : "";
}

// reasonText is the small line under a value: why it was set, or that you set it.
export function reasonText(by: SetBy, reason?: string): string {
  if (by === "user") return "set by you";
  if (by === "auto") return reason ? `${reason}` : "set automatically";
  return "";
}

// facetValue is what a cell shows for a value that may be missing.
export function facetValue(value?: string): string {
  return value || NOT_SET;
}

// setParams turns the chooser's picks into facet.set parameters. "" means leave it alone; AUTO sends
// an empty text, which goes back to automatic.
export const KEEP = "";
export const AUTO = "\u0000auto";

export function setParams(ids: string[], type: string, tradition: string): Record<string, unknown> {
  const p: Record<string, unknown> = { item_ids: ids };
  if (type !== KEEP) p.type = type === AUTO ? "" : type;
  if (tradition !== KEEP) p.tradition = tradition === AUTO ? "" : tradition;
  return p;
}

// countText says how many books were changed.
export function countText(n: number): string {
  return `${n} ${n === 1 ? "book" : "books"}`;
}
