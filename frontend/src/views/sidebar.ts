// The right sidebar: study tools beside whatever is open. Its tabs are AI chat, Related, Notes, Word
// and Compare; Settings → Right sidebar chooses which show and in what order. Tools follow the
// passage in the Reader and load only while they are visible. At phone width it is a sheet.

import { installedBibles } from "../bibles";
import { chip } from "../controls";
import { Message, Module, Ref, run } from "../api";
import { button, clear, h } from "../dom";
import { Column, alignColumns, verseParts } from "../helpers";
import { icon } from "../icons";
import { WordInfo } from "../markup";
import { openInStudy } from "../document/open";
import { noteName, notesPanel } from "../notes";
import { TOOLS, ToolId, activeTool, visibleTools } from "../shellwords";
import { loadPref, navigate, onPassage, onPrefs, prefs, savePref, setPassage, setPrefs, state } from "../state";
import { relatedPanel, wordPanel } from "./study";

interface Tool {
  el: HTMLElement;
  refresh?(): void | Promise<void>;
  stale: boolean;
}

async function go(ref: string) {
  const p = await run<Ref>("verse.parse", { ref }).catch(() => null);
  if (!p) return;
  setPassage(p.display, p);
  navigate("reader");
}

function makeSidebar() {
  const related = relatedPanel(go);
  const word = wordPanel(go);
  const notes = notesPanel({
    scope: () => (state.parsed ? { ref: state.parsed.osis, display: state.parsed.display } : null),
    openTab: (n) => openInStudy({ kind: "note", target: n.id, title: noteName(n) }),
  });
  const compare = comparePanel();
  const tools: Record<ToolId, Tool> = {
    chat: { el: chatPanel(), stale: false },
    related: { el: related.el, stale: true, refresh: () => related.showPassage(state.parsed?.osis ?? null, state.parsed?.display) },
    notes: { el: notes.el, stale: true, refresh: () => notes.refresh() },
    word: { el: word.el, stale: false },
    compare: { el: compare.el, stale: true, refresh: () => compare.refresh() },
  };

  const tablist = h("div", { class: "tool-tabs", role: "tablist", "aria-label": "Study tools" });
  const closeBtn = h("button", { type: "button", class: "icon-btn", "aria-label": "Close the sidebar", title: "Close the sidebar (Ctrl+.)" }, icon("close"));
  closeBtn.addEventListener("click", () => setOpen(false));
  const panels = h("div", { class: "tool-panels" });
  const tabs = new Map<ToolId, HTMLButtonElement>();
  const panelEls = new Map<ToolId, HTMLElement>();
  for (const t of TOOLS) {
    const tool = tools[t.id];
    const b = h("button", { type: "button", role: "tab", id: `tool-tab-${t.id}`, "aria-controls": `tool-panel-${t.id}` }, t.label);
    b.addEventListener("click", () => select(t.id, true));
    b.addEventListener("keydown", (e) => {
      const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
      if (!d) return;
      e.preventDefault();
      const shown = visibleTools(prefs.sidebar.tools);
      const next = shown[(shown.indexOf(t.id) + d + shown.length) % shown.length];
      select(next, false);
      tabs.get(next)?.focus();
    });
    tabs.set(t.id, b);
    const panel = h("section", { id: `tool-panel-${t.id}`, class: "tool-panel", role: "tabpanel", "aria-labelledby": b.id, tabindex: "-1" }, tool.el);
    panelEls.set(t.id, panel);
    panels.append(panel);
  }

  const el = h("aside", { class: "rightbar", "aria-label": "Study tools", id: "rightbar" },
    h("div", { class: "rightbar-head" }, tablist, closeBtn), panels);

  function render() {
    const p = prefs.sidebar;
    el.hidden = !p.open;
    document.body.classList.toggle("sidebar-open", p.open);
    const shown = visibleTools(p.tools);
    const current = activeTool(p.tools, p.tool);
    tablist.replaceChildren(...shown.map((id) => tabs.get(id)!));
    for (const t of TOOLS) {
      const on = t.id === current;
      const b = tabs.get(t.id)!;
      b.setAttribute("aria-selected", String(on));
      b.tabIndex = on ? 0 : -1;
      panelEls.get(t.id)!.hidden = !on;
    }
    if (shown.length === 0) {
      tablist.replaceChildren(h("span", { class: "hint" }, "No tools chosen."));
    }
    if (p.open) load(current);
  }

  function load(id: ToolId) {
    const t = tools[id];
    if (t.stale && t.refresh) {
      t.stale = false;
      void t.refresh();
    }
  }

  function select(id: ToolId, focusPanel: boolean) {
    setPrefs({ sidebar: { ...prefs.sidebar, tool: id, open: true } });
    if (focusPanel) panelEls.get(id)?.focus();
  }

  function setOpen(open: boolean) {
    setPrefs({ sidebar: { ...prefs.sidebar, open } });
    if (!open) document.dispatchEvent(new CustomEvent("xegesis:sidebar-closed"));
  }

  // open shows the sidebar, on a tool when one is named (showing it if Settings had hidden it).
  function open(id?: ToolId) {
    let tools = prefs.sidebar.tools;
    if (id && !visibleTools(tools).includes(id)) tools = tools.map((t) => (t.id === id ? { ...t, shown: true } : t));
    setPrefs({ sidebar: { ...prefs.sidebar, tools, open: true, tool: id ?? prefs.sidebar.tool } });
  }

  onPassage(() => {
    for (const t of Object.values(tools)) if (t.refresh) t.stale = true;
    if (prefs.sidebar.open) load(activeTool(prefs.sidebar.tools, prefs.sidebar.tool));
  });
  onPrefs(render);
  render();

  return {
    el,
    open,
    toggle: () => setOpen(!prefs.sidebar.open),
    isOpen: () => prefs.sidebar.open,
    showWord(w: WordInfo) {
      open("word");
      void word.showWord(w);
    },
    newNote(text: string) {
      open("notes");
      notes.create(text);
    },
    // newNoteOn starts a note on a place in a book.
    newNoteOn(place: { itemId: string; title: string; locator: Record<string, unknown> }) {
      open("notes");
      notes.createOn(place);
    },
  };
}

let instance: ReturnType<typeof makeSidebar> | null = null;

// sidebar is the one right sidebar; the shell places it and the Reader asks it to show things.
export function sidebar() {
  return (instance ??= makeSidebar());
}

// The AI chat tool is a placeholder until the assistant is built (T36). It makes no network calls.
function chatPanel(): HTMLElement {
  return h("div", { class: "chat-off" },
    h("h3", {}, "The assistant is off"),
    h("p", {}, "Xegesis can work with an AI assistant when you choose one. There are three ways to turn it on:"),
    h("ul", {},
      h("li", {}, h("strong", {}, "Your own key"), " for an online service such as Claude or OpenAI."),
      h("li", {}, h("strong", {}, "Claude Code or Codex"), " already set up on this computer."),
      h("li", {}, h("strong", {}, "A local model"), " that runs on this computer, so nothing leaves it.")),
    h("p", { class: "hint" }, "Nothing is sent anywhere until you choose, and each online option says what it sends first."),
    button("Choose in Settings…", () => navigate("settings/model"), { class: "primary" }));
}

// Compare shows the passage in several Bibles, verse by verse.
const COMPARE_KEY = "xegesis.compare";

function comparePanel() {
  const msg = new Message();
  const picker = h("div", { class: "module-picker", role: "group", "aria-label": "Bibles to compare" });
  const body = h("div", { class: "compare", "aria-live": "polite" });
  const el = h("div", {}, picker, msg.el, body);
  let modules: Module[] = [];
  let seq = 0;

  function chosen(): string[] {
    const saved = loadPref<string[] | null>(COMPARE_KEY, null);
    const ids = (saved ?? modules.map((m) => m.id)).filter((id) => modules.some((m) => m.id === id));
    return ids;
  }

  function renderPicker() {
    const on = new Set(chosen());
    clear(picker, modules.map((m) => {
      return chip({ label: m.abbreviation || m.id, title: m.name, value: m.id, checked: on.has(m.id), onChange: () => {
        savePref(COMPARE_KEY, [...picker.querySelectorAll<HTMLInputElement>("input:checked")].map((b) => b.value));
        void draw();
      } });
    }));
  }

  async function draw() {
    const n = ++seq;
    const p = state.parsed;
    const ids = chosen();
    if (!p) return void clear(body, h("p", { class: "empty" }, "Open a passage in the Reader to compare Bibles."));
    if (modules.length < 2) return void clear(body, h("p", { class: "empty" }, "Compare needs two or more Bibles. Add them from Library → Get Bibles."));
    if (ids.length === 0) return void clear(body, h("p", { class: "empty" }, "Choose the Bibles to compare."));
    clear(body, h("p", { class: "hint" }, `Comparing ${p.display}…`));
    const r = await msg.attempt(() => run<{ columns: Column[] }>("bible.parallel", { modules: ids, ref: p.osis }));
    if (n !== seq || !r) return;
    const names = r.columns.map((c) => modules.find((m) => m.id === c.module)?.abbreviation || c.module);
    clear(body, h("h3", {}, p.display), alignColumns(r.columns).map((row) => {
      const { chapter, verse } = verseParts(row.base);
      return h("div", { class: "compare-verse" },
        h("h4", {}, `${chapter}:${verse}`),
        h("dl", {}, row.cells.map((v, i) => v ? [h("dt", {}, names[i]), h("dd", {}, v.text)] : null)));
    }));
  }

  async function refresh() {
    modules = await installedBibles();
    renderPicker();
    await draw();
  }

  return { el, refresh };
}
