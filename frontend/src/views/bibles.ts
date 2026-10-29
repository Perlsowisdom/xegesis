// Bibles: get Bibles from the online catalogues (the default), the installed modules, and
// installing one from a file or folder. Every action is a bible.* command.

import { CatalogueItem, Message, Module, paramEnum, pick, run } from "../api";
import { badge, button, clear, field, h, select } from "../dom";
import { licenceChooser, licenceLine, plainName, termsText } from "../licence";
import { kindLabel } from "../markupwords";

type Tab = "get" | "installed" | "file";

export function biblesView() {
  const get = getBibles(() => installed.show());
  const installed = installedList(() => get.show());
  const file = installForm(() => installed.show());
  const panels: Record<Tab, { el: HTMLElement; show(): void }> = { get, installed, file };
  const tabs: Record<Tab, HTMLButtonElement> = {
    get: button("Get Bibles", () => open("get")),
    installed: button("Installed", () => open("installed")),
    file: button("Install from file", () => open("file")),
  };
  let current: Tab = "get";

  function open(t: Tab) {
    current = t;
    for (const [k, b] of Object.entries(tabs)) b.setAttribute("aria-pressed", String(k === t));
    for (const [k, p] of Object.entries(panels)) p.el.hidden = k !== t;
    panels[t].show();
  }

  const el = h("div", { class: "view" },
    h("div", { class: "row between wrap" }, h("h1", {}, "Bibles"),
      h("div", { class: "tabs", role: "group", "aria-label": "Bibles sections" }, tabs.get, tabs.installed, tabs.file)),
    get.el, installed.el, file.el);
  for (const [k, b] of Object.entries(tabs)) b.setAttribute("aria-pressed", String(k === current));
  for (const [k, p] of Object.entries(panels)) p.el.hidden = k !== current;
  return { el, show: () => panels[current].show() };
}

interface Available {
  items: CatalogueItem[];
  total: number;
  languages: { code: string; name: string; count: number }[];
  enabled: boolean;
  cached: boolean;
}

interface Progress { source: string; item: string; phase: string; done: number; total: number }

function size(n?: number): string {
  if (!n) return "—";
  return n >= 1 << 20 ? `${(n / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`;
}

const nouns: Record<string, string> = { bible: "Bibles", commentary: "commentaries and cross-reference sets", dictionary: "dictionaries and lexicons" };

function licenceText(l: CatalogueItem["licence"]): string {
  return plainName(l) + (l?.spdx ? ` (${l.spdx})` : "");
}

// getBibles lists what the catalogues offer, with a language picker, search and Install buttons.
function getBibles(changed: () => void) {
  const msg = new Message();
  const off = h("div", { class: "card", hidden: true });
  const what = select([["bible", "Bibles"], ["commentary", "Commentaries and cross-references"], ["dictionary", "Dictionaries and lexicons"]],
    "bible", { "aria-label": "What to get" });
  const language = select([["", "All languages"]], "", { "aria-label": "Language" });
  const search = h("input", { type: "search", placeholder: "Search names", "aria-label": "Search Bibles" });
  const licence = select([["", "Any licence"], ["public_domain", "Public domain"], ["creative_commons", "Free to share"],
    ["open_other", "Other licences"]], "", { "aria-label": "Licence" });
  const installable = h("input", { type: "checkbox", checked: true });
  const status = h("p", { class: "hint", role: "status" });
  const list = h("div", { class: "table-wrap" });
  const dialog = h("div");
  const preferred = (navigator.language || "en").split("-")[0].toLowerCase();
  let pickedLanguage = false;
  let timer = 0;

  const disableButton = button("Turn off catalogues", () => disable(), { hidden: true });
  const toolbar = h("div", { class: "row wrap" },
    what, language, h("div", { class: "grow" }, search), licence,
    h("label", { class: "check" }, installable, " Only ones Xegesis can install"),
    button("Refresh catalogues", () => refresh()),
    disableButton);

  const el = h("section", { "aria-label": "Get Bibles" },
    h("p", { class: "hint" }, "Bibles, commentaries, cross-references and lexicons from CrossWire, eBible.org, STEPBible and OpenBible.info. Pick a language, then Install."),
    off, toolbar, status, msg.el, dialog, list);

  language.addEventListener("change", () => { pickedLanguage = true; load(); });
  what.addEventListener("change", () => load());
  for (const c of [licence, installable]) c.addEventListener("change", () => load());
  search.addEventListener("input", () => { clearTimeout(timer); timer = window.setTimeout(load, 250); });

  async function disable() {
    if (!confirm("Turn off the Bible catalogues? Installed Bibles stay; Xegesis stops downloading the lists.")) return;
    if ((await msg.attempt(() => run("bible.sources", { enable: false }))) !== undefined) {
      msg.ok("Bible catalogues are off.");
      load();
    }
  }

  async function enable() {
    if ((await msg.attempt(() => run("bible.sources", { enable: true }))) !== undefined) refresh();
  }

  async function refresh() {
    msg.info("Downloading the catalogues…");
    const r = await msg.attempt(() => run<{ sources: { name: string; error?: string }[] }>("bible.sources", { refresh: true }));
    if (!r) return;
    const failed = r.sources.filter((s) => s.error);
    if (failed.length) msg.info(`Could not refresh ${failed.map((s) => s.name).join(", ")}; their cached lists are used.`);
    else msg.clear();
    load();
  }

  async function load() {
    const params: Record<string, unknown> = { limit: 500, supported: installable.checked, kind: what.value };
    if (language.value) params.language = language.value;
    if (search.value.trim()) params.text = search.value.trim();
    if (licence.value) params.licence = licence.value;
    const r = await msg.attempt(() => run<Available>("bible.available", params));
    if (!r) return;
    off.hidden = r.enabled;
    disableButton.hidden = !r.enabled;
    clear(off, h("h2", {}, "Bible catalogues are off"),
      h("p", {}, "When you turn them on, Xegesis downloads the lists of Bibles from CrossWire (crosswire.org) and " +
        "eBible.org, and the Bibles you choose to install. Only these files are requested; nothing about you or your " +
        "library is sent. You can turn them off again here at any time."),
      button("Turn on Bible catalogues", enable, { class: "primary" }));
    if (fillLanguages(r)) return load();
    status.textContent = r.cached ? `${r.total.toLocaleString()} ${nouns[what.value] ?? "items"}${r.total > r.items.length ? `, showing ${r.items.length}` : ""}.` :
      r.enabled ? "No catalogue downloaded yet. Choose Refresh catalogues." : "";
    clear(list, r.items.length === 0 ? (r.cached ? h("p", { class: "empty" }, "Nothing matches.") : null) :
      h("table", { class: "list" },
        h("thead", {}, h("tr", {}, ["Name", "Language", "Licence", "Size", ""].map((t) => h("th", { scope: "col" }, t)))),
        h("tbody", {}, r.items.map(row))));
  }

  // fillLanguages refreshes the picker; it returns true when it chose the user's own language
  // for the first time, so the list must be loaded again.
  function fillLanguages(r: Available): boolean {
    const want = language.value;
    language.replaceChildren(h("option", { value: "" }, "All languages"),
      ...r.languages.map((l) => h("option", { value: l.code }, `${l.name} (${l.count})`)));
    language.value = want;
    if (!pickedLanguage && !want && r.languages.some((l) => l.code === preferred)) {
      pickedLanguage = true;
      language.value = preferred;
      return true;
    }
    return false;
  }

  function row(it: CatalogueItem) {
    const action = h("td", { class: "actions" });
    if (it.installed && !it.update_available) action.append(badge("Installed", "ok"));
    else if (it.locked) action.append(badge("Locked", "bad"), " ", h("small", { class: "hint" }, "Needs the publisher's unlock key"));
    else if (!it.supported) action.append(h("small", { class: "hint" }, `Not available: ${it.reason ?? ""}`));
    else {
      const verb = it.update_available ? "Update" : "Install";
      action.append(button(verb, () => start(it, action), { class: "primary", "aria-label": `${verb} ${it.name}` }));
    }
    return h("tr", {},
      h("td", {}, h("strong", {}, it.name), h("br"),
        h("small", { class: "hint" }, `${it.abbreviation || it.id} · ${it.source_name}${it.version ? ` · version ${it.version}` : ""}`)),
      h("td", {}, it.language_name),
      h("td", {}, licenceText(it.licence), it.licence_clear ? null : h("small", { class: "hint" }, " (to confirm)")),
      h("td", { class: "num" }, size(it.size)),
      action);
  }

  // start shows the size and licence first; the download runs only after the user confirms.
  async function start(it: CatalogueItem, cell: HTMLElement) {
    const pre = await msg.attempt(() => run<{ item: CatalogueItem; size: number }>("bible.get", { source: it.source, item: it.id, preview: true }));
    if (!pre) return;
    const item = pre.item;
    const verb = item.update_available ? "Update" : "Install";
    const form = item.licence_clear ? null : licenceChooser("What may you do with this text?", false);
    if (form) form.set(item.licence);
    const box = h("div", { class: "card", role: "dialog", "aria-label": `${verb} ${item.name}` },
      h("h2", {}, `${verb} ${item.name}`),
      h("p", {}, `Download: ${size(pre.size)} from ${item.source_name}.`),
      item.licence_clear ? h("p", {}, `Licence: ${licenceText(item.licence)}. ${termsText(item.licence)}`) :
        h("p", {}, "The catalogue does not say clearly what this licence allows. Choose the nearest below; you can change it later."),
      h("details", {}, h("summary", {}, "What the source says"), h("pre", { class: "rights" }, item.rights || "Nothing")),
      form?.el,
      h("div", { class: "row" },
        button(verb, () => go(), { class: "primary" }),
        button("Cancel", () => clear(dialog))));
    clear(dialog, box);
    box.querySelector<HTMLButtonElement>(".row button")?.focus();

    async function go() {
      const params: Record<string, unknown> = { source: item.source, item: item.id, accept_licence: true };
      if (item.update_available) params.replace = true;
      if (form) Object.assign(params, bibleLicenceParams(form.value(), "licence_type"));
      clear(dialog);
      const bar = h("progress", { "aria-label": `Installing ${item.name}` });
      const phase = h("small", { class: "hint", "aria-live": "polite" }, "Starting…");
      clear(cell, bar, " ", phase);
      const poll = window.setInterval(async () => {
        const ps = await run<Progress[]>("bible.progress").catch(() => [] as Progress[]);
        const p = ps.find((x) => x.source === item.source && x.item === item.id);
        if (!p) return;
        if (p.total > 0) { bar.max = p.total; bar.value = Math.min(p.done, p.total); }
        phase.textContent = p.phase === "downloading" ? `Downloading ${size(p.done)}` : p.phase === "checking" ? "Checking" : "Converting";
      }, 400);
      const r = await msg.attempt(() => run<{ module: Module; warnings: string[] }>("bible.get", params));
      clearInterval(poll);
      if (r) {
        msg.ok(`Installed ${r.module.name}.`);
        changed();
      }
      load();
    }
  }

  return { el, show: load };
}

// installedList shows the installed modules and available updates.
function installedList(changed: () => void) {
  const msg = new Message();
  const updates = h("div");
  const editor = h("div");
  const table = h("div", { class: "table-wrap" });
  const el = h("section", { "aria-label": "Installed Bibles" }, msg.el, updates, editor, table);

  async function show() {
    const [ms, ups] = await Promise.all([
      msg.attempt(() => run<Module[]>("bible.modules", { kind: "all" })),
      run<{ module: string; name: string; version: string; installed_version: string }[]>("bible.updates").catch(() => []),
    ]);
    if (!ms) return;
    clear(updates, ups.length ? h("p", { class: "msg", role: "status" },
      `Updates are available for ${ups.map((u) => `${u.name} (${u.installed_version || "?"} → ${u.version})`).join(", ")}. ` +
      "Update them from Get Bibles.") : null);
    clear(table, ms.length === 0 ? h("p", { class: "empty" }, "No Bibles installed yet. Get one from the catalogues, or install one from a file.") :
      h("table", { class: "list" },
        h("thead", {}, h("tr", {}, ["Name", "Kind", "Language", "Numbering", "Contents", "Licence", ""].map((t) => h("th", { scope: "col" }, t)))),
        h("tbody", {}, ms.map((m) => h("tr", {},
          h("td", {}, h("strong", {}, m.name || m.id), h("br"), h("small", { class: "hint" }, `${m.abbreviation} · ${m.id}`)),
          h("td", {}, kindLabel(m.kind)),
          h("td", {}, m.language),
          h("td", {}, m.kind === "dictionary" || m.kind === "lexicon" ? "—" : m.versification),
          h("td", { class: "num" }, `${(m.verse_count ?? 0).toLocaleString()} ${m.kind === "dictionary" || m.kind === "lexicon" ? "entries" : "verses"}`),
          h("td", {}, licenceLine(m.licence, "Bible", () => clear(editor, bibleLicenceEditor(m, () => { clear(editor); show(); })))),
          h("td", {}, button("Remove", () => remove(m), { class: "danger", "aria-label": `Remove ${m.name || m.id}` })))))));
  }

  async function remove(m: Module) {
    if (!confirm(`Remove ${m.name || m.id}? Its module file is deleted; your notes are kept.`)) return;
    if ((await msg.attempt(() => run("bible.remove", { module: m.id }))) !== undefined) {
      msg.ok(`Removed ${m.name || m.id}.`);
      show();
      changed();
    }
  }

  return { el, show };
}

// installForm asks only for the file. The licence is the one the file states, or Personal copy, and
// can be changed afterwards; the module's names and numbering are under More options.
function installForm(installed: () => void) {
  const msg = new Message();
  const result = h("div");
  const path = h("input", { type: "text", required: true, spellcheck: "false", placeholder: "Choose a file or folder" });
  const format = select([["", "Detect from file"]], "");
  paramEnum("bible.install", "format").then((fs) => fs.forEach((f) => format.append(h("option", { value: f }, f.toUpperCase()))));
  const text = (placeholder = "") => h("input", { type: "text", placeholder, autocomplete: "off" });
  const fields = {
    id: text("e.g. web"), name: text("e.g. World English Bible"), abbreviation: text("e.g. WEB"),
    language: text("e.g. eng"), version: text(), versification: text("english or hebrew"), source: text("Where it came from"),
  };
  const replace = h("input", { type: "checkbox" });

  const browse = async (kind: "file" | "folder") => {
    const p = await msg.attempt(() => pick(kind, kind === "file" ? "Choose a Bible file or .zip archive" : "Choose a folder of Bible files"));
    if (p) path.value = p;
  };

  const form = h("form", { class: "card", "aria-labelledby": "install-h" },
    h("h2", { id: "install-h" }, "Install a Bible from a file"),
    h("p", { class: "hint" }, "An OSIS, USFM or USX file, a folder of USFM or USX books, or a .zip archive of them. " +
      "Its licence is read from the file; if the file names none, it is a personal copy. You can change it afterwards."),
    h("div", { class: "field" },
      h("label", { for: "install-path" }, "File or folder"),
      h("div", { class: "row" }, Object.assign(path, { id: "install-path" }),
        button("Choose file…", () => browse("file")), button("Choose folder…", () => browse("folder")))),
    h("details", { class: "more" }, h("summary", {}, "More options"),
      h("div", { class: "grid3" },
        field("Format", format),
        field("Module ID", fields.id, "Made from the abbreviation if blank"),
        field("Name", fields.name, "Read from the file if blank"),
        field("Abbreviation", fields.abbreviation),
        field("Language", fields.language, "Read from the file if blank"),
        field("Version", fields.version),
        field("Verse numbering", fields.versification),
        field("Source", fields.source)),
      h("label", { class: "check" }, replace, " Replace a Bible with the same module ID")),
    h("div", { class: "row" }, h("button", { type: "submit", class: "primary" }, "Install")),
    msg.el, result);

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const params: Record<string, unknown> = { path: path.value.trim() };
    if (format.value) params.format = format.value;
    for (const [k, input] of Object.entries(fields)) if (input.value.trim()) params[k] = input.value.trim();
    if (replace.checked) params.replace = true;
    msg.info("Installing… large files can take a minute.");
    clear(result);
    const r = await msg.attempt(() => run<{ module: Module; warnings: string[] }>("bible.install", params));
    if (!r) return;
    msg.ok(`Installed ${r.module.name || r.module.id} (${r.module.verse_count.toLocaleString()} verses).`);
    const showResult = (m: Module) => clear(result,
      h("p", {}, licenceLine(m.licence, "Bible", () => clear(result, bibleLicenceEditor(m, (fresh) => showResult(fresh ?? m))))),
      r.warnings?.length ? h("details", {}, h("summary", {}, `${r.warnings.length} warnings`), h("ul", {}, r.warnings.map((w) => h("li", {}, w)))) : null);
    showResult(r.module);
    installed();
  });

  return { el: form, show() {} };
}

// bibleLicenceParams turns chooser values into bible.* licence parameters; typeKey is the name of
// the type parameter ("type" for bible.set_licence, "licence_type" for bible.get).
function bibleLicenceParams(v: Record<string, unknown>, typeKey: string): Record<string, unknown> {
  const p: Record<string, unknown> = { [typeKey]: v.type };
  for (const k of ["spdx", "rights_holder", "attribution", "share_allowed"]) if (v[k] !== undefined) p[k] = v[k];
  if (v.url) p.licence_url = v.url;
  if (typeKey === "licence_type") delete p.share_allowed;
  return p;
}

// bibleLicenceEditor changes an installed Bible's licence; done gets the updated module, or null
// when cancelled.
function bibleLicenceEditor(m: Module, done: (m: Module | null) => void): HTMLElement {
  const msg = new Message();
  const chooser = licenceChooser(`Licence of ${m.name || m.id}`, false);
  chooser.set(m.licence);
  const box = h("div", { class: "card" }, chooser.el,
    h("p", { class: "hint" }, termsText(m.licence)),
    h("div", { class: "row" },
      button("Save licence", async () => {
        const r = await msg.attempt(() => run<Module>("bible.set_licence", { module: m.id, ...bibleLicenceParams(chooser.value(), "type") }));
        if (r) done(r);
      }, { class: "primary" }),
      button("Cancel", () => done(null))),
    msg.el);
  setTimeout(() => box.querySelector<HTMLInputElement>("input:checked")?.focus());
  return box;
}
