// Collections in plain words: what a rule says, the choices the rule builder offers, and the
// collection lists the pickers show. Pure functions, so they can be tested.

import { CollectionNode, CollectionRule } from "./api";
import { NOT_SET, countText } from "./facetwords";

export { countText };

// The licence choices use the four plain words. "Free to share" is a Creative Commons licence.
export const LICENCE_CHOICES: [string, string][] = [
  ["", "Any licence"], ["public_domain", "Public domain"], ["creative_commons", "Free to share"],
  ["personal_purchase", "Personal copy"], ["own_work", "My own work"],
];

export const LANGUAGE_CHOICES: [string, string][] = [
  ["", "Any language"], ["en", "English"], ["grc", "Ancient Greek"], ["he", "Hebrew"], ["la", "Latin"],
  ["de", "German"], ["fr", "French"], ["es", "Spanish"],
];

// The rule builder's fields as typed; empty text means "any".
export interface RuleForm {
  type: string;
  tradition: string;
  author: string;
  text: string;
  language: string;
  yearFrom: string;
  yearTo: string;
  folder: string;
  licence: string;
}

export const emptyForm = (): RuleForm => ({ type: "", tradition: "", author: "", text: "", language: "", yearFrom: "", yearTo: "", folder: "", licence: "" });

// "none" in the type and tradition pickers means books with no type or tradition yet.
export function ruleFromForm(f: RuleForm): CollectionRule {
  const r: CollectionRule = {};
  const put = (k: "type" | "tradition" | "author" | "text" | "language" | "licence" | "folder_id", v: string) => {
    if (v.trim()) r[k] = v.trim();
  };
  put("type", f.type);
  put("tradition", f.tradition);
  put("author", f.author);
  put("text", f.text);
  put("language", f.language);
  put("licence", f.licence);
  put("folder_id", f.folder);
  const from = parseInt(f.yearFrom, 10), to = parseInt(f.yearTo, 10);
  if (from > 0) r.year_from = from;
  if (to > 0) r.year_to = to;
  return r;
}

export function formFromRule(r: CollectionRule = {}): RuleForm {
  return {
    type: r.type ?? "", tradition: r.tradition ?? "", author: r.author ?? "", text: r.text ?? "", language: r.language ?? "",
    yearFrom: r.year_from ? String(r.year_from) : "", yearTo: r.year_to ? String(r.year_to) : "", folder: r.folder_id ?? "", licence: r.licence ?? "",
  };
}

export const ruleIsEmpty = (r: CollectionRule): boolean => Object.keys(ruleFromForm(formFromRule(r))).length === 0;

const choice = (list: [string, string][], v: string) => list.find(([k]) => k === v)?.[1] ?? v;

// ruleSummary says in words what a rule matches, e.g. "Commentary (New Testament), Reformed, by Calvin".
export function ruleSummary(r: CollectionRule, folderName: (id: string) => string = () => ""): string {
  const parts: string[] = [];
  if (r.type) parts.push(r.type === "none" ? "no type yet" : r.type);
  if (r.tradition) parts.push(r.tradition === "none" ? "no tradition yet" : r.tradition);
  if (r.author) parts.push(`by ${r.author}`);
  if (r.text) parts.push(`"${r.text}" in the title`);
  if (r.language) parts.push(choice(LANGUAGE_CHOICES, r.language));
  if (r.year_from && r.year_to) parts.push(`${r.year_from} to ${r.year_to}`);
  else if (r.year_from) parts.push(`${r.year_from} or later`);
  else if (r.year_to) parts.push(`${r.year_to} or earlier`);
  if (r.folder_id) parts.push(`in ${folderName(r.folder_id) || "one folder"}`);
  if (r.licence) parts.push(choice(LICENCE_CHOICES, r.licence));
  return parts.join(", ") || "all books";
}

// matchText is the live line under the rule builder.
export function matchText(count: number): string {
  return count === 0 ? "No books match yet." : `${countText(count)} match${count === 1 ? "es" : ""}.`;
}

export interface Flat {
  node: CollectionNode;
  depth: number;
}

// mine lists the user's own collections in tree order, with how deep each is.
export function mine(tree: CollectionNode[]): Flat[] {
  const out: Flat[] = [];
  const walk = (nodes: CollectionNode[], depth: number) => {
    for (const n of nodes) {
      if (n.kind === "builtin") continue;
      out.push({ node: n, depth });
      walk(n.children ?? [], depth + 1);
    }
  };
  walk(tree, 0);
  return out;
}

export function find(tree: CollectionNode[], id: string): CollectionNode | undefined {
  for (const n of tree) {
    if (n.id === id) return n;
    const f = find(n.children ?? [], id);
    if (f) return f;
  }
  return undefined;
}

// without removes a collection and everything inside it, for the "put inside" list.
export function without(list: Flat[], id: string): Flat[] {
  const out: Flat[] = [];
  let skip = -1;
  for (const f of list) {
    if (skip >= 0 && f.depth > skip) continue;
    skip = f.node.id === id ? f.depth : -1;
    if (skip < 0) out.push(f);
  }
  return out;
}

export const indent = (depth: number, name: string): string => "  ".repeat(depth) + name;

export interface SearchGroup {
  label: string;
  options: [string, string][];
}

// searchChoices are the entries of the "Search in" list: the user's collections, then types,
// traditions and centuries. Authors are left out; the search has its own author box.
export function searchChoices(tree: CollectionNode[]): SearchGroup[] {
  const label = (n: CollectionNode, name = n.name) => `${name} (${n.count})`;
  const groups: SearchGroup[] = [{ label: "My collections", options: mine(tree).map(({ node, depth }): [string, string] => [node.id, label(node, indent(depth, node.name))]) }];
  for (const [id, title] of [["b:type", "Type"], ["b:tradition", "Tradition"], ["b:year", "Century"]] as const) {
    const g = tree.find((n) => n.id === id);
    const options = (g?.children ?? []).map((n): [string, string] => [n.id, label(n, n.id.endsWith(":none") ? `${title}: ${NOT_SET}` : n.name)]);
    groups.push({ label: title, options });
  }
  return groups.filter((g) => g.options.length > 0);
}
