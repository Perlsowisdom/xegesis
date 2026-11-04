// Interface state shared between views: the passage being read and which Bibles are shown.
// Only view preferences live here; everything else comes from commands.

import type { Ref } from "./api";
import {
  AppTheme, Provider, RailPrefs, ReadingPrefs, SidebarPrefs, normalizeProvider, normalizeRail, normalizeReading, normalizeSidebar,
  normalizeTheme,
} from "./shellwords";

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

// loadPref and savePref keep per-device preferences in localStorage. Both survive storage being
// unavailable: preferences are a convenience, never data.
export function loadPref<T>(key: string, fallback: T): T {
  return load(key, fallback);
}

export function savePref(key: string, v: unknown): void {
  save(key, v);
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

// Per-device preferences for the shell: the rail, the right sidebar, reading and the assistant.
// Changing one saves it and tells every listener, so the rail popover and Settings stay in step.
export interface Prefs {
  rail: RailPrefs;
  sidebar: SidebarPrefs;
  reading: ReadingPrefs;
  provider: Provider;
  theme: AppTheme;
}

const PREF_KEYS: Record<keyof Prefs, string> = {
  rail: "xegesis.rail", sidebar: "xegesis.sidebar", reading: "xegesis.reading", provider: "xegesis.assistant.provider",
  theme: "xegesis.theme",
};

export const prefs: Prefs = {
  rail: normalizeRail(load(PREF_KEYS.rail, null)),
  sidebar: normalizeSidebar(load(PREF_KEYS.sidebar, null)),
  reading: normalizeReading(load(PREF_KEYS.reading, null)),
  provider: normalizeProvider(load(PREF_KEYS.provider, "off")),
  theme: normalizeTheme(load(PREF_KEYS.theme, "system")),
};

// applyTheme sets the app's colours on the page: style.css follows data-theme, or the system's
// setting when there is none.
export function applyTheme(): void {
  const root = document.documentElement;
  if (prefs.theme === "system") delete root.dataset.theme;
  else root.dataset.theme = prefs.theme;
}

const prefListeners = new Set<Listener>();

export function setPrefs(patch: Partial<Prefs>): void {
  for (const k of Object.keys(patch) as (keyof Prefs)[]) {
    (prefs as unknown as Record<string, unknown>)[k] = patch[k];
    save(PREF_KEYS[k], patch[k]);
  }
  prefListeners.forEach((l) => l());
}

export function onPrefs(l: Listener): void {
  prefListeners.add(l);
}

// Navigation is set up by main.ts; views call navigate to switch sections.
let nav: (view: string) => void = () => {};
export function setNavigator(fn: (view: string) => void) {
  nav = fn;
}
export function navigate(view: string) {
  nav(view);
}
