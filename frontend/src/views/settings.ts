// Settings: grouped pages down the left side. General holds Appearance (the rail and reading),
// Right sidebar (which tools, in what order), Bibles online (the API.Bible key, kept in the system
// keychain and never shown back), Privacy and Your data. Assistant holds the Model page, which only
// saves a choice for now. Advanced holds every command and the app's details.
//
// Pages are addressed as #settings/<page>, so Help and the sidebar can link straight to one.

import { Message, run } from "../api";
import { tick } from "../controls";
import { badge, button, clear, field, h } from "../dom";
import { icon } from "../icons";
import { railControls } from "../shell/rail";
import { LOCAL_PROVIDERS, ONLINE_PROVIDERS, Provider, ToolId, moveTool, providerText, showTool, toolLabel } from "../shellwords";
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
  const el = pageEl("Appearance", null,
    h("h3", {}, "Rail"),
    h("div", { class: "settings-card" }, rail),
    h("h3", {}, "Reading"),
    h("p", { class: "hint" }, "How Bible text looks in Read mode. The Aa button in the Reader changes the same settings."),
    h("div", { class: "settings-card" }, reading));
  return {
    el,
    show: () => {
      clear(rail, railControls());
      clear(reading, readingControls());
    },
  };
}

const TOOL_HINTS: Record<ToolId, string> = {
  chat: "Ask questions about what you are reading.",
  related: "Cross-references, commentaries and your notes for the passage.",
  notes: "Your notes on the passage.",
  word: "The original word under a tapped word.",
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
    const [key, cat] = await Promise.all([
      run<KeyStatus>("apibible.status").catch(() => null),
      run<{ enabled: boolean }>("bible.sources").catch(() => null),
    ]);
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
      h("dd", {}, badge(prefs.provider === "off" ? "Off" : "Chosen, not connected"), " Not built yet. Each online service will say what it sends before you use it."),
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

// The Model page only saves the choice on this device. It runs no command and makes no network call.
function modelPage(): PageView {
  const current = h("strong", {});
  const chips = h("div", { class: "provider-choices", hidden: true });
  const change = button("Change", () => {
    chips.hidden = !chips.hidden;
    change.setAttribute("aria-expanded", String(!chips.hidden));
    if (!chips.hidden) chips.querySelector<HTMLElement>("[aria-pressed=true]")?.focus();
  }, { "aria-expanded": "false" });
  const saved = h("p", { class: "msg ok", role: "status", "aria-live": "polite" });

  const chip = (p: { id: Provider; label: string }) =>
    h("button", { type: "button", class: "provider-chip", "aria-pressed": "false", "data-id": p.id, onclick: () => pick(p.id) }, p.label);
  clear(chips,
    h("h3", {}, "Online, with your own key"),
    h("div", { class: "row wrap" }, ONLINE_PROVIDERS.map(chip)),
    h("h3", {}, "On this computer"),
    h("div", { class: "row wrap" }, LOCAL_PROVIDERS.map(chip), chip({ id: "off", label: "Off" })));

  function pick(id: Provider) {
    setPrefs({ provider: id });
    render();
    saved.textContent = id === "off" ? "The assistant is off." : `Saved on this device: ${providerText(id)}. Nothing has connected.`;
  }

  function render() {
    current.textContent = providerText(prefs.provider);
    for (const b of chips.querySelectorAll<HTMLElement>(".provider-chip")) b.setAttribute("aria-pressed", String(b.dataset.id === prefs.provider));
  }
  render();

  const el = pageEl("Model", null,
    h("p", { class: "notice" },
      h("strong", {}, "Coming in a later version. "),
      "Choosing here only saves your preference on this device. Nothing connects and nothing is sent."),
    h("div", { class: "provider-row" }, h("span", {}, "Provider"), current, change),
    chips, saved,
    h("p", { class: "hint" }, "Online services use a key you get from them, kept in your system keychain. Claude Code and Codex use the account already set up for them on this computer. A local model runs entirely on this computer."));
  return { el, show: render };
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
