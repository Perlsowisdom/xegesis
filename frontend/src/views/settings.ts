// Settings: grouped pages down the left side. General holds Appearance (the rail and reading),
// Right sidebar (which tools, in what order), Bibles online (the API.Bible key, kept in the system
// keychain and never shown back), Privacy and Your data. Assistant holds the Model page: provider,
// key, model and a connection test. Advanced holds every command and the app's details.
//
// Pages are addressed as #settings/<page>, so Help and the sidebar can link straight to one.

import { Message, RunError, run } from "../api";
import { AssistantConfig, CLIInfo, LOCAL_CHOICES, ONLINE_CHOICES, providerLine } from "../assistantwords";
import { segmented, settingRow, tick, toggleSwitch } from "../controls";
import { badge, button, clear, field, h } from "../dom";
import { icon } from "../icons";
import { railControls } from "../shell/rail";
import { APP_THEMES, ToolId, moveTool, showTool, toolLabel } from "../shellwords";
import { prefs, setPrefs } from "../state";
import { commandsView } from "./commands";
import { dataSection } from "./data";
import { readingControls } from "./reader";

interface KeyStatus {
  key_set: boolean;
  valid?: boolean;
  message?: string;
  cached_items?: number;
  commercial_allowed?: boolean;
}

interface PageView { el: HTMLElement; show?: () => void; focus?: () => void }
interface Page { id: string; label: string; make: () => PageView }

const GROUPS: { label: string; pages: Page[] }[] = [
  {
    label: "General", pages: [
      { id: "appearance", label: "Appearance", make: appearancePage },
      { id: "sidebar", label: "Right sidebar", make: sidebarPage },
      { id: "online", label: "Bibles online", make: apiBiblePage },
      { id: "privacy", label: "Privacy", make: privacyPage },
      { id: "data", label: "Your data", make: dataPage },
    ],
  },
  { label: "Assistant", pages: [{ id: "model", label: "Model", make: modelPage }] },
  { label: "Advanced", pages: [{ id: "commands", label: "Commands", make: commandsPage }, { id: "about", label: "About", make: aboutPage }] },
];

export function settingsView() {
  const all = GROUPS.flatMap((g) => g.pages);
  const made = new Map<string, PageView>();
  const links = new Map<string, HTMLAnchorElement>();
  const body = h("div", { class: "settings-page" });
  let current = "appearance";

  const nav = h("nav", { class: "settings-nav", "aria-label": "Settings pages" }, GROUPS.map((g) =>
    h("div", { class: "settings-group" },
      h("h2", {}, g.label),
      h("ul", { class: "plain" }, g.pages.map((p) => {
        const a = h("a", { href: `#settings/${p.id}` }, p.label);
        a.addEventListener("click", (e) => {
          e.preventDefault();
          open(p.id, true);
        });
        links.set(p.id, a);
        return h("li", {}, a);
      })))));

  const el = h("div", { class: "view settings" }, h("h1", {}, "Settings"), h("div", { class: "settings-body" }, nav, body));

  function open(id: string, focus = false) {
    const p = all.find((x) => x.id === id) ?? all[0];
    current = p.id;
    let page = made.get(p.id);
    if (!page) {
      page = p.make();
      made.set(p.id, page);
      body.append(page.el);
    }
    for (const [pid, pg] of made) pg.el.hidden = pid !== p.id;
    for (const [lid, a] of links) {
      if (lid === p.id) a.setAttribute("aria-current", "page");
      else a.removeAttribute("aria-current");
    }
    history.replaceState(null, "", `#settings/${p.id}`);
    page.show?.();
    if (focus) (page.focus ? page.focus() : page.el.querySelector<HTMLElement>("h2")?.focus());
  }

  return {
    el,
    show: () => {
      const asked = /^#settings\/([\w-]+)/.exec(location.hash)?.[1];
      open(asked && all.some((p) => p.id === asked) ? asked : current);
    },
    focus: () => links.get(current)?.focus(),
  };
}

function pageEl(title: string, intro: string | null, ...children: (Node | null)[]): HTMLElement {
  return h("section", { class: "settings-section" },
    h("h2", { tabindex: "-1" }, title),
    intro ? h("p", { class: "hint" }, intro) : null,
    children);
}

// General -----------------------------------------------------------------------------------------

function appearancePage(): PageView {
  const rail = h("div", {});
  const reading = h("div", {});
  const theme = segmented({ label: "Theme", value: prefs.theme, options: APP_THEMES, onChange: (v) => setPrefs({ theme: v }) });
  const el = pageEl("Appearance", null,
    h("h3", {}, "Theme"),
    h("div", { class: "settings-card" },
      settingRow("Colours", "Light, dark, or the same as your computer. Kept on this device.", theme)),
    h("h3", {}, "Rail"),
    h("div", { class: "settings-card" }, rail),
    h("h3", {}, "Reading"),
    h("p", { class: "hint" }, "How Bible text looks in Read mode. The Aa button in the Reader changes the same settings."),
    h("div", { class: "settings-card" }, reading));
  return {
    el,
    show: () => {
      clear(rail, railControls());
      clear(reading, readingControls(), toggleSwitch({
        label: "Show tagged words", checked: prefs.reading.taggedWords,
        hint: "Mark words that carry Strong's numbers, and look one up with a click. Off: words look like plain text; Alt+click looks one up.",
        onChange: (taggedWords) => setPrefs({ reading: { ...prefs.reading, taggedWords } }),
      }));
    },
  };
}

const TOOL_HINTS: Record<ToolId, string> = {
  chat: "Ask questions about what you are reading.",
  related: "Cross-references, commentaries and your notes for the passage.",
  notes: "Your notes on the passage.",
  highlights: "What you have highlighted in the book you are reading.",
  word: "The original word behind a word you look up.",
  compare: "The passage in several Bibles, verse by verse.",
};

function sidebarPage(): PageView {
  const list = h("ol", { class: "tool-order" });
  const toggle = button("", () => setPrefs({ sidebar: { ...prefs.sidebar, open: !prefs.sidebar.open } }));
  const el = pageEl("Right sidebar", "Choose which tools appear in the right sidebar, and their order.", list,
    h("div", { class: "row wrap" }, toggle));

  function render(focus?: { id: ToolId; which: "up" | "down" | "box" }) {
    toggle.textContent = prefs.sidebar.open ? "Hide the sidebar now" : "Show the sidebar now";
    const tools = prefs.sidebar.tools;
    clear(list, tools.map((t, i) => {
      const box = tick({ label: toolLabel(t.id), hint: TOOL_HINTS[t.id], checked: t.shown, attrs: { "data-tool": t.id },
        onChange: (on) => {
          setPrefs({ sidebar: { ...prefs.sidebar, tools: showTool(prefs.sidebar.tools, t.id, on) } });
          render({ id: t.id, which: "box" });
        } });
      box.classList.add("grow");
      const move = (by: -1 | 1) => {
        setPrefs({ sidebar: { ...prefs.sidebar, tools: moveTool(prefs.sidebar.tools, t.id, by) } });
        render({ id: t.id, which: by < 0 ? "up" : "down" });
      };
      const arrow = (which: "up" | "down", by: -1 | 1, disabled: boolean) =>
        h("button", {
          type: "button", class: "icon-btn", "aria-label": `Move ${toolLabel(t.id)} ${which}`, title: `Move ${which}`, disabled,
          "data-tool": t.id, "data-which": which, onclick: () => move(by),
        }, icon(which));
      return h("li", { class: `tool-row${t.shown ? "" : " off"}` }, box,
        arrow("up", -1, i === 0), arrow("down", 1, i === tools.length - 1));
    }));
    if (focus) {
      const sel = focus.which === "box" ? `input[data-tool="${focus.id}"]` : `button[data-tool="${focus.id}"][data-which="${focus.which}"]`;
      let target = list.querySelector<HTMLElement>(sel);
      // At the top or bottom the button is disabled; keep the keyboard on the other one.
      if (target && (target as HTMLButtonElement).disabled) target = list.querySelector<HTMLElement>(`button[data-tool="${focus.id}"]:not([disabled])`);
      target?.focus();
    }
  }
  return { el, show: () => render() };
}

function apiBiblePage(): PageView {
  const msg = new Message();
  const status = h("div", { "aria-live": "polite" });
  const key = h("input", { type: "password", autocomplete: "off", spellcheck: "false", placeholder: "Paste your key" });

  const form = h("form", { "aria-label": "API.Bible key" },
    h("p", { class: "hint" }, "Use your own free API.Bible key to read Bibles online. The key is stored in your system keychain and is never shown again."),
    status,
    field("New key", key),
    h("div", { class: "row wrap" },
      h("button", { type: "submit", class: "primary" }, "Save key"),
      button("Check key works", () => check(true)),
      button("Remove key", async () => {
        if (!confirm("Remove the API.Bible key and its cached text?")) return;
        if (await msg.attempt(() => run("apibible.clear_key"))) {
          msg.ok("Key removed.");
          check(false);
        }
      }, { class: "danger" })),
    msg.el);
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const k = key.value.trim();
    key.value = "";
    if (!k) return msg.info("Paste a key first.");
    if (await msg.attempt(() => run("apibible.set_key", { key: k }))) {
      msg.ok("Key saved to the keychain.");
      check(true);
    }
  });

  async function check(verify: boolean) {
    const s = await msg.attempt(() => run<KeyStatus>("apibible.status", verify ? { verify: true } : {}));
    if (!s) return;
    clear(status, h("p", {},
      s.key_set ? badge("Key set", "ok") : badge("No key", "warn"), " ",
      verify && s.key_set ? (s.valid ? badge("Works", "ok") : badge("Not accepted", "bad")) : null, " ",
      s.message && s.key_set ? h("span", { class: "hint" }, s.message) : null,
      s.cached_items ? h("span", { class: "hint" }, ` ${s.cached_items} passages cached.`) : null));
  }

  return { el: pageEl("Bibles online", null, h("h3", {}, "API.Bible"), form), show: () => check(false), focus: () => key.focus() };
}

function privacyPage(): PageView {
  const msg = new Message();
  const rows = h("dl", { class: "info privacy-list" });
  const el = pageEl("Privacy",
    "Xegesis sends nothing about you: no tracking and no usage statistics. Only the features below can use the internet, and each stays off until you turn it on.",
    rows, msg.el);

  // Both status commands read local settings only; neither uses the network.
  async function show() {
    const [key, cat, ai] = await Promise.all([
      run<KeyStatus>("apibible.status").catch(() => null),
      run<{ enabled: boolean }>("bible.sources").catch(() => null),
      run<AssistantConfig>("assistant.config.get").catch(() => null),
    ]);
    const aiOn = !!ai && ai.provider !== "off";
    const on = (yes: boolean | undefined) => (yes ? badge("On", "ok") : badge("Off"));
    clear(rows,
      h("dt", {}, "Bible catalogues"),
      h("dd", {}, on(cat?.enabled), " Lists of Bibles to download, and the downloads you choose. Turn them on in Library → Get Bibles.",
        cat?.enabled ? [" ", button("Turn off", async () => {
          if ((await msg.attempt(() => run("bible.sources", { enable: false }))) !== undefined) void show();
        })] : null),
      h("dt", {}, "Bibles online"),
      h("dd", {}, on(key?.key_set), " Reads Bibles from API.Bible with your own key. Set it in Bibles online."),
      h("dt", {}, "Assistant"),
      h("dd", {}, badge(!aiOn ? "Off" : ai!.local ? "On this computer" : "On", aiOn ? "ok" : ""),
        !aiOn ? " Nothing is sent. Choose a provider in Assistant → Model."
          : ` ${providerLine(ai!)}. Before the first message, the chat shows exactly what is sent and where.`),
      h("dt", {}, "Problem reports"),
      h("dd", {}, badge("Off"), " Nothing is sent. When this arrives, you will see each report and choose whether to send it."));
  }
  return { el, show };
}

function dataPage(): PageView {
  const data = dataSection();
  return { el: pageEl("Your data", null, data.el), show: data.show };
}

// Assistant ---------------------------------------------------------------------------------------

// The Model page chooses the provider, keeps its key in the system keychain (shown back only as
// "Saved"), picks a model from the provider's list and tests the connection. Choosing sends
// nothing; the model list and the test reach the provider only when asked for.
function modelPage(): PageView {
  const msg = new Message();
  const current = h("strong", { class: "provider-current" });
  const chips = h("div", { class: "provider-choices", hidden: true });
  const change = button("Change", () => toggle(chips.hidden), { "aria-expanded": "false" });
  const saved = h("p", { class: "msg ok", role: "status", "aria-live": "polite" });
  const details = h("div", { class: "assistant-details" });
  let cfg: AssistantConfig | null = null;
  let tools: CLIInfo[] = []; // Claude Code and Codex, found or not

  function toggle(open: boolean) {
    chips.hidden = !open;
    change.setAttribute("aria-expanded", String(open));
    if (open) chips.querySelector<HTMLElement>("[aria-pressed=true]")?.focus();
  }

  const openLink = (url: string) => {
    const rt = (window as unknown as { runtime?: { BrowserOpenURL?: (u: string) => void } }).runtime;
    if (rt?.BrowserOpenURL) rt.BrowserOpenURL(url);
    else window.open(url, "_blank", "noopener");
  };
  const isTool = (id: string) => !!cfg?.presets.find((p) => p.id === id)?.cli;
  const chip = (id: string, label: string) => {
    if (!isTool(id)) {
      return h("button", { type: "button", class: "provider-chip", "aria-pressed": "false", "data-id": id, onclick: () => void pick(id) }, label);
    }
    // A command-line tool is Ready when found on PATH; otherwise the chip is off, with a link to install it.
    const t = tools.find((x) => x.id === id);
    const ready = !!t?.found;
    return h("span", { class: "tool-choice" },
      h("button", {
        type: "button", class: "provider-chip", "aria-pressed": "false", "data-id": id, "data-state": ready ? "ready" : "missing", disabled: !ready,
        title: ready ? t!.path : `${label} was not found on this computer`, onclick: () => void pick(id),
      }, label, h("span", { class: "chip-later" }, ready ? ` · Ready${t!.version ? ` · ${t!.version}` : ""}` : tools.length ? " · Not found" : " · checking…")),
      !ready && t ? h("a", { class: "install-link", href: t.install_url, onclick: (e: Event) => { e.preventDefault(); openLink(t.install_url); } }, `Install ${label}`) : null);
  };
  const labelOf = (id: string) => cfg?.presets.find((p) => p.id === id)?.label ?? id;

  function renderChips() {
    clear(chips,
      h("h3", {}, "Online, with your own key"),
      h("div", { class: "row wrap" }, ONLINE_CHOICES.map((id) => chip(id, labelOf(id)))),
      h("h3", {}, "On this computer"),
      h("div", { class: "row wrap" }, LOCAL_CHOICES.map((id) => chip(id, labelOf(id)))),
      h("h3", {}, "Other"),
      h("div", { class: "row wrap" }, chip("custom", "Custom address"), chip("off", "Off")));
  }

  async function set(params: object): Promise<boolean> {
    const c = await msg.attempt(() => run<AssistantConfig>("assistant.config.set", params));
    if (c) cfg = c;
    return !!c;
  }

  async function pick(id: string) {
    if (!(await set({ provider: id }))) return;
    saved.textContent = id === "off" ? "The assistant is off." : `Saved: ${providerLine(cfg!)}. Nothing has been sent.`;
    render();
    void loadModels();
  }

  function keyForm(c: AssistantConfig): HTMLElement {
    const key = h("input", { type: "password", autocomplete: "off", spellcheck: "false", placeholder: c.key_saved ? "Saved" : "Paste your key", "aria-label": `${c.label} key` });
    const form = h("form", { class: "key-form", "aria-label": `${c.label} key` },
      h("div", { class: "row wrap" }, key, h("button", { type: "submit", class: "primary" }, "Save key"),
        c.key_saved ? button("Remove key", async () => {
          if (await set({ clear_key: true })) { msg.ok("Key removed from the keychain."); render(); }
        }, { class: "danger" }) : null),
      h("p", { class: "key-state" }, c.key_saved ? badge("Saved", "ok") : badge(c.needs_key ? "No key" : "No key needed", c.needs_key ? "warn" : "")));
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      const k = key.value.trim();
      key.value = "";
      if (!k) return msg.info("Paste a key first.");
      if (await set({ key: k })) {
        msg.ok("Key saved to the keychain.");
        render();
        void loadModels();
      }
    });
    return form;
  }

  const models = h("div", { class: "model-chips row wrap", role: "group", "aria-label": "Models" });
  async function loadModels() {
    if (!cfg || cfg.provider === "off" || isTool(cfg.provider) || (cfg.needs_key && !cfg.key_saved)) {
      clear(models, h("span", { class: "hint" }, cfg?.needs_key ? "Save the key to see the models." : ""));
      return;
    }
    clear(models, h("span", { class: "hint" }, "Asking for the models…"));
    let list: string[] = [];
    try {
      list = await run<string[]>("assistant.models");
    } catch (e) {
      clear(models, h("span", { class: "hint" }, e instanceof RunError ? e.info.message : String(e)));
      return;
    }
    const chosen = cfg.model ?? "";
    if (chosen && !list.includes(chosen)) list = [chosen, ...list];
    clear(models, list.length ? list.map((m) => h("button", {
      type: "button", class: "provider-chip model-chip", "aria-pressed": String(m === chosen), "data-model": m,
      onclick: async () => {
        if (await set({ model: m })) {
          for (const b of models.querySelectorAll<HTMLElement>(".model-chip")) b.setAttribute("aria-pressed", String(b.dataset.model === m));
          saved.textContent = `Model: ${m}.`;
        }
      },
    }, m)) : h("span", { class: "hint" }, "This service does not list its models. Type the model's name below."));
  }

  function render() {
    if (!cfg) return;
    current.textContent = providerLine(cfg);
    for (const b of chips.querySelectorAll<HTMLElement>(".provider-chip")) b.setAttribute("aria-pressed", String(b.dataset.id === cfg.provider));
    if (cfg.provider === "off") return void clear(details);
    const c = cfg;
    if (isTool(c.provider)) return renderTool(c);
    const addr = h("input", { type: "url", value: c.base_url ?? "", placeholder: "https://…/v1", "aria-label": "Address", spellcheck: "false" });
    const saveAddr = button("Save address", async () => {
      if (await set({ base_url: addr.value.trim() })) { msg.ok("Address saved."); render(); void loadModels(); }
    });
    const typed = h("input", { type: "text", value: c.model ?? "", placeholder: "Model name", "aria-label": "Model name", spellcheck: "false" });
    const test = new Message();
    const where = c.local ? "Runs on this computer: nothing leaves it, and personal copies of books may be read."
      : `Sends to ${hostOf(c.base_url)} over the internet. Personal copies of books are never sent.`;
    const address = h("div", { class: "row wrap" }, addr, saveAddr);
    clear(details,
      h("p", { class: "hint" }, where),
      c.needs_key || c.provider === "local" || c.provider === "custom" ? [h("h3", {}, "Key"), keyForm(c)] : null,
      h("h3", {}, "Model"), models,
      h("div", { class: "row wrap" }, typed, button("Use this model", async () => {
        const m = typed.value.trim();
        if (m && (await set({ model: m }))) { saved.textContent = `Model: ${m}.`; void loadModels(); }
      })),
      c.provider === "custom" ? [h("h3", {}, "Address"), address]
        : h("details", { class: "more-options" }, h("summary", {}, "More options"), field("Address", address, "Change it only for a proxy or another server with the same interface.")),
      h("div", { class: "row wrap test-row" }, button("Test connection", async () => {
        test.info("Testing…");
        const r = await test.attempt(() => run<{ ok: boolean; message: string }>("assistant.test"));
        if (r) (r.ok ? test.ok(r.message) : test.error(new RunError({ code: "unavailable", message: r.message })));
      })),
      test.el);
  }

  // A command-line tool needs no key and offers no model list: it uses the account and model set in
  // the tool itself. The page says what that means for what is sent.
  function renderTool(c: AssistantConfig) {
    const t = tools.find((x) => x.id === c.provider);
    const test = new Message();
    clear(details,
      h("p", { class: "hint" }, t?.found
        ? `Runs ${c.label}${t.version ? ` ${t.version}` : ""} on this computer, signed in as you already are. It makes its own network calls to its own service, so what it reads from your library goes there. Personal copies of books are never given to it.`
        : `${c.label} was not found on this computer.`),
      h("div", { class: "row wrap test-row" }, button("Test connection", async () => {
        test.info("Testing…");
        const r = await test.attempt(() => run<{ ok: boolean; message: string }>("assistant.test"));
        if (r) (r.ok ? test.ok(r.message) : test.error(new RunError({ code: "unavailable", message: r.message })));
      })),
      test.el);
  }

  async function show() {
    cfg = (await msg.attempt(() => run<AssistantConfig>("assistant.config.get"))) ?? null;
    if (!cfg) return;
    tools = [];
    renderChips();
    render();
    void loadModels();
    // Looking for the tools runs each with --version; the chips fill in when that is done.
    tools = await run<CLIInfo[]>("assistant.cli.detect").catch(() => []);
    renderChips();
    render();
  }

  const el = pageEl("Model", null,
    h("div", { class: "provider-row" }, h("span", {}, "Provider"), current, change),
    chips, saved, details, msg.el,
    h("p", { class: "hint" }, "Online services use a key you get from them, kept in your system keychain. Claude Code and Codex use the account already set up for them on this computer. A local model runs entirely on this computer. The assistant only reads your library; anything it writes comes as a draft for you to insert."));
  return { el, show };
}

function hostOf(url?: string): string {
  try {
    return new URL(url ?? "").host;
  } catch {
    return url ?? "the service";
  }
}

// Advanced ----------------------------------------------------------------------------------------

function commandsPage(): PageView {
  const v = commandsView();
  return { el: v.el, show: v.show, focus: v.focus };
}

function aboutPage(): PageView {
  const msg = new Message();
  const info = h("dl", { class: "info" });
  async function show() {
    const a = await msg.attempt(() => run<{ version: string; schema_version: number; device: string }>("app.info"));
    if (a) clear(info, h("dt", {}, "Version"), h("dd", {}, a.version), h("dt", {}, "Library schema"), h("dd", {}, String(a.schema_version)),
      h("dt", {}, "This device"), h("dd", {}, h("code", {}, a.device)),
      h("dt", {}, "Licence"), h("dd", {}, "MIT OR Apache-2.0"));
  }
  return { el: pageEl("About Xegesis", "A free, open-source Bible study app that works offline.", info, msg.el), show };
}
