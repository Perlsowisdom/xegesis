// The only way the interface does anything: the commands bound by the desktop app
// (app/main.go). Every feature is a registered command; the interface has none of its own.

import { CommandError, Schema, errorText, parseError } from "./helpers";
import { h } from "./dom";

export interface CommandInfo {
  id: string;
  summary: string;
  writes: boolean;
  params: Schema | null;
  result?: Schema | null;
}

interface Bound {
  Commands(): Promise<CommandInfo[]>;
  Run(id: string, params: string): Promise<string>;
  PickFile?(title: string): Promise<string>;
  PickFiles?(title: string): Promise<string[] | null>;
  PickFolder?(title: string): Promise<string>;
}

export function bound(): Bound | null {
  const w = window as unknown as { go?: { main?: { App?: Bound } } };
  return w.go?.main?.App ?? null;
}

export class RunError extends Error {
  constructor(public info: CommandError) {
    super(errorText(info));
  }
}

let pending = 0;
function busy(delta: number) {
  pending += delta;
  document.body.classList.toggle("busy", pending > 0);
  document.body.setAttribute("aria-busy", String(pending > 0));
}

// run calls a command and returns its parsed result. Failures throw RunError.
export async function run<T = unknown>(id: string, params: object = {}): Promise<T> {
  const app = bound();
  if (!app) throw new RunError({ code: "unavailable", message: "the Xegesis app is not connected" });
  busy(1);
  try {
    const out = await app.Run(id, JSON.stringify(params));
    return (out ? JSON.parse(out) : null) as T;
  } catch (e) {
    throw e instanceof RunError ? e : new RunError(parseError(e));
  } finally {
    busy(-1);
  }
}

// runRaw passes JSON text through unchanged, for the command console.
export async function runRaw(id: string, params: string): Promise<string> {
  const app = bound();
  if (!app) throw new RunError({ code: "unavailable", message: "the Xegesis app is not connected" });
  busy(1);
  try {
    return await app.Run(id, params);
  } catch (e) {
    throw new RunError(parseError(e));
  } finally {
    busy(-1);
  }
}

let cached: CommandInfo[] | null = null;
export async function commands(): Promise<CommandInfo[]> {
  const app = bound();
  if (!app) return [];
  cached ??= await app.Commands();
  return cached;
}

// paramEnum reads an allowed-values list from a command's schema, so forms follow the core.
export async function paramEnum(id: string, param: string): Promise<string[]> {
  const c = (await commands()).find((c) => c.id === id);
  return c?.params?.properties?.[param]?.enum ?? [];
}

// pick opens a native file or folder dialog; it returns "" when cancelled.
export async function pick(kind: "file" | "folder", title: string): Promise<string> {
  const app = bound();
  const fn = kind === "file" ? app?.PickFile : app?.PickFolder;
  if (!app || !fn) throw new RunError({ code: "unavailable", message: "file dialogs need the desktop app" });
  return (await fn.call(app, title)) ?? "";
}

// pickFiles opens a native dialog for several files; it returns [] when cancelled.
export async function pickFiles(title: string): Promise<string[]> {
  const app = bound();
  if (!app?.PickFiles) throw new RunError({ code: "unavailable", message: "file dialogs need the desktop app" });
  return (await app.PickFiles(title)) ?? [];
}

// onEvent listens to an event the desktop app emits, such as files dropped on the window.
export function onEvent(name: string, fn: (...data: unknown[]) => void): void {
  const rt = (window as unknown as { runtime?: { EventsOn?(n: string, f: (...d: unknown[]) => void): void } }).runtime;
  rt?.EventsOn?.(name, fn);
}

// Message is a status line under a form: errors are announced assertively, other news politely.
export class Message {
  el = h("p", { class: "msg", "aria-live": "polite" });

  ok(text: string) {
    this.el.className = "msg ok";
    this.el.setAttribute("role", "status");
    this.el.textContent = text;
  }
  info(text: string) {
    this.el.className = "msg";
    this.el.setAttribute("role", "status");
    this.el.textContent = text;
  }
  error(e: unknown) {
    const err = e instanceof RunError ? e.info : parseError(e);
    this.el.className = "msg error";
    this.el.setAttribute("role", "alert");
    this.el.replaceChildren(h("span", {}, errorText(err)), " ", h("code", {}, err.code));
  }
  clear() {
    this.el.className = "msg";
    this.el.textContent = "";
  }

  // attempt runs fn and shows its failure here; it returns undefined on failure.
  async attempt<T>(fn: () => Promise<T>): Promise<T | undefined> {
    try {
      const r = await fn();
      if (this.el.classList.contains("error")) this.clear();
      return r;
    } catch (e) {
      this.error(e);
      return undefined;
    }
  }
}

// Types of the results the interface reads.

export interface Licence {
  type: string;
  spdx?: string;
  rights_holder?: string;
  attribution?: string;
  url?: string;
  commercial_allowed?: boolean;
  share_allowed?: boolean;
  store_allowed?: boolean;
  confirmed_by_user?: boolean;
  origin?: string;
  print_limit?: { pages?: number; percent?: number } | null;
  export_limit?: { pages?: number; percent?: number } | null;
}

export interface Module {
  id: string;
  name: string;
  abbreviation: string;
  language: string;
  versification: string;
  version: string;
  verse_count: number;
  books: string[];
  licence: Licence;
  source: string;
}

export interface Ref {
  display: string;
  osis: string;
  start: number;
  end: number;
}

export interface Note {
  id: string;
  title: string;
  body: string;
  ref?: Ref | null;
  created_at: number;
  updated_at: number;
}

export interface Change {
  id: string;
  op: string;
  field: string;
  before: unknown;
  after: unknown;
  command: string;
  created_at: number;
}

export interface Folder {
  id: string;
  label: string;
  path: string;
  online: boolean;
  last_seen_at: number;
}

export interface Facet {
  facet: string;
  value: string;
  origin?: string;
  reason?: string;
  confidence?: number;
}

export interface ItemSummary {
  id: string;
  kind: string;
  title: string;
  subtitle?: string;
  year?: number;
  language?: string;
  authors: string[];
  status: string;
  licence: string;
  licence_name?: string;
  licence_origin?: string;
  suggested_licence?: Licence | null;
  type?: string;
  type_by?: "auto" | "user";
  type_reason?: string;
  tradition?: string;
  tradition_by?: "auto" | "user";
  tradition_reason?: string;
}

export interface Vocabulary {
  types: string[];
  traditions: string[];
}

export interface Rule {
  id: string;
  field: "author" | "publisher" | "series" | "folder";
  text: string;
  value: string;
}

// The result of library.add and library.add_files.
export interface AddResult {
  added: number;
  copied: number;
  folders: Folder[];
  since: number;
  errors: { rel_path: string; error: string }[];
  summary: string;
}

export interface Item {
  id: string;
  kind: string;
  title: string;
  subtitle: string;
  year: number;
  language: string;
  publisher: string;
  series: string;
  series_no: string;
  isbn: string;
  status: string;
  people: { id: string; name: string; role: string }[];
  facets: Facet[];
  files: { id: string; folder_id: string; path: string; rel_path: string; format: string; size: number; online: boolean }[];
  licence?: Licence | null;
  licence_name?: string;
  suggested_licence?: Licence | null;
}

// An item from the Bible catalogues (bible.available).
export interface CatalogueItem {
  source: string;
  source_name: string;
  id: string;
  name: string;
  abbreviation?: string;
  language: string;
  language_name: string;
  kind: string;
  version?: string;
  size?: number;
  licence: Licence;
  licence_clear: boolean;
  rights: string;
  about?: string;
  locked: boolean;
  supported: boolean;
  reason?: string;
  origin: string;
  installed: boolean;
  module?: string;
  update_available?: boolean;
}
