// Interface state shared between views: the passage being read and which Bibles are shown.
// Only view preferences live here; everything else comes from commands.

import type { Ref } from "./api";

type Listener = () => void;
const listeners = new Set<Listener>();

export const state = {
  ref: "" as string,
  parsed: null as Ref | null,
  modules: load<string[]>("xegesis.modules", []),
};

export function setPassage(ref: string, parsed: Ref | null) {
  state.ref = ref;
  state.parsed = parsed;
  save("xegesis.ref", ref);
  listeners.forEach((l) => l());
}

export function setModules(ids: string[]) {
  state.modules = ids;
  save("xegesis.modules", ids);
}

export function onPassage(l: Listener): void {
  listeners.add(l);
}

export function lastRef(): string {
  return load<string>("xegesis.ref", "John 1");
}

function load<T>(key: string, fallback: T): T {
  try {
    const v = localStorage.getItem(key);
    return v ? (JSON.parse(v) as T) : fallback;
  } catch {
    return fallback;
  }
}

function save(key: string, v: unknown) {
  try {
    localStorage.setItem(key, JSON.stringify(v));
  } catch {
    /* storage may be unavailable; preferences are a convenience */
  }
}

// Navigation is set up by main.ts; views call navigate to switch sections.
let nav: (view: string) => void = () => {};
export function setNavigator(fn: (view: string) => void) {
  nav = fn;
}
export function navigate(view: string) {
  nav(view);
}
