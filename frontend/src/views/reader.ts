// Reader: a slim top bar (Read | Study, search in what is open, reading settings, grid presets, a ⋯
// menu and the sidebar toggle), today's reading as one line, then the study grid
// (document/studygrid.ts). Study mode shows the whole grid; Read mode shows the selected pane's
// document alone, full width, and switching keeps your place because the same view stays mounted.

import { Message } from "../api";
import { segmented, settingRow } from "../controls";
import { button, h } from "../dom";
import { studyGrid } from "../document/studygrid";
import { setOpener } from "../document/open";
import { openGridPresets, setGridPresets } from "../gridhook";
import { icon } from "../icons";
import { todayCard } from "../plancard";
import { togglePopover } from "../popover";
import { ReaderMode, ReadingPrefs, SPACINGS, TEXT_SIZES, gridButtonPlace, stepSize } from "../shellwords";
import { navigate, onPrefs, prefs, setPrefs, state } from "../state";
import { openReport } from "./report";
import { sidebar } from "./sidebar";

function iconButton(name: string, label: string, onclick: (e: Event) => void, attrs: Record<string, unknown> = {}): HTMLButtonElement {
  return h("button", { type: "button", class: "icon-btn", "aria-label": label, title: label, ...attrs, onclick }, icon(name));
}

export function readerView() {
  const msg = new Message();
  const side = sidebar();
  const today = todayCard();
  const grid = studyGrid({
    mode: () => prefs.reading.mode,
    defaultTab: () => ({ kind: "bible" }),
  });
  setGridPresets((anchor) => grid.presets(anchor));
  setOpener((spec) => grid.open(spec));

  // Read | Study ---------------------------------------------------------------------------------
  const modes: ReaderMode[] = ["read", "study"];
  const modeBtns = modes.map((m) => {
    const b = h("button", { type: "button", "aria-pressed": "false" }, m === "read" ? "Read" : "Study");
    b.addEventListener("click", () => setMode(m));
    return b;
  });
  const modeSwitch = h("div", { class: "segmented", role: "group", "aria-label": "Mode" }, modeBtns);
  function setMode(m: ReaderMode) {
    if (prefs.reading.mode !== m) setPrefs({ reading: { ...prefs.reading, mode: m } });
  }

  // Search in what is open: the selected pane's Bibles or book ------------------------------------
  const findInput = h("input", { type: "search", "aria-label": "Search in what is open", placeholder: "Search in what is open", autocomplete: "off" });
  const findForm = h("form", { class: "find", role: "search", hidden: true }, findInput);
  const findBtn = iconButton("search", "Search in what is open", () => {
    findForm.hidden = !findForm.hidden;
    findBtn.setAttribute("aria-expanded", String(!findForm.hidden));
    if (!findForm.hidden) findInput.focus();
  }, { "aria-expanded": "false" });
  findInput.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !findInput.value) {
      findForm.hidden = true;
      findBtn.setAttribute("aria-expanded", "false");
      findBtn.focus();
    }
  });
  findForm.addEventListener("submit", (e) => {
    e.preventDefault();
    const q = findInput.value.trim();
    const v = grid.selectedView();
    if (!q) return;
    if (v?.menu?.find) void v.menu.find(q, findInput);
    else msg.info("The selected pane has nothing to search. Select a pane with a Bible or a book.");
  });

  // Reading settings, grid and the ⋯ menu ---------------------------------------------------------
  const aaBtn = h("button", { type: "button", class: "aa-btn", "aria-haspopup": "dialog", "aria-expanded": "false", "aria-label": "Reading settings", title: "Reading settings" }, "Aa");
  aaBtn.addEventListener("click", () => togglePopover(aaBtn, () => readingControls(), { label: "Reading settings", side: "below" }));

  const gridBtn: HTMLButtonElement = iconButton("grid", "Grid presets", () => openGridPresets(gridBtn), { "aria-haspopup": "dialog", "aria-expanded": "false" });

  const moreBtn: HTMLButtonElement = iconButton("more", "More", () => togglePopover(moreBtn, () => h("div", { class: "menu-list" },
    button("Passage report", () => state.parsed && openReport("passage", state.parsed.osis),
      { title: "Everything your library holds about this passage, in one place" }),
    button("Notes on this passage", () => side.open("notes")),
    button("Print or save…", () => grid.selectedView()?.menu?.print?.(), { title: "Print, or save as PDF, Word, a web page or Markdown" }),
    button("Reading plans", () => navigate("plans"))), { label: "More", side: "below", menu: true }),
  { "aria-haspopup": "menu", "aria-expanded": "false" });

  const sideBtn = iconButton("panel", "Right sidebar", () => side.toggle(), { "aria-controls": "rightbar", title: "Right sidebar (Ctrl+.)", "aria-keyshortcuts": "Control+." });

  const bar = h("div", { class: "topbar", role: "group", "aria-label": "Reader" },
    modeSwitch, findBtn, findForm, aaBtn, gridBtn, h("span", { class: "spacer" }), moreBtn, sideBtn);

  const el = h("div", { class: "view reader" },
    h("h1", { class: "sr-only" }, "Reader"),
    bar, today.el, msg.el, grid.el);

  // applyPrefs shows the mode, the reading settings and where the grid button goes.
  let lastMode = prefs.reading.mode;
  function applyPrefs() {
    const r = prefs.reading;
    el.dataset.mode = r.mode;
    modeBtns.forEach((b, i) => b.setAttribute("aria-pressed", String(modes[i] === r.mode)));
    aaBtn.hidden = r.mode !== "read";
    gridBtn.hidden = gridButtonPlace(r.mode, prefs.rail) !== "toolbar";
    el.style.setProperty("--text-size", `${r.size / 100}`);
    el.dataset.font = r.font;
    el.dataset.width = r.width;
    el.dataset.theme = r.theme;
    el.dataset.tagged = String(r.taggedWords);
    el.style.setProperty("--line-height", String(SPACINGS[r.spacing]));
    sideBtn.setAttribute("aria-pressed", String(prefs.sidebar.open));
    if (r.mode !== lastMode) {
      lastMode = r.mode;
      grid.render();
    }
  }
  onPrefs(applyPrefs);
  applyPrefs();

  async function show() {
    void today.refresh();
  }

  return { el, show, focus: () => grid.focus() };
}

// readingControls are the Read mode settings, in the Aa popover and in Settings → Appearance.
export function readingControls(): HTMLElement {
  const size = h("output", { "aria-live": "polite", class: "size-out" }, `${prefs.reading.size}%`);
  const set = (patch: Partial<ReadingPrefs>) => {
    setPrefs({ reading: { ...prefs.reading, ...patch } });
    size.textContent = `${prefs.reading.size}%`;
  };
  const choice = <K extends "font" | "width" | "spacing" | "theme">(key: K, label: string, hint: string | undefined, options: [ReadingPrefs[K], string][]) =>
    settingRow(label, hint, segmented({ label, value: prefs.reading[key], small: true, options, onChange: (v) => set({ [key]: v } as Partial<ReadingPrefs>) }));
  const stepper = h("div", { class: "stepper", role: "group", "aria-label": "Text size" },
    button("A−", () => set({ size: stepSize(prefs.reading.size, -1) }), { "aria-label": "Smaller text", title: `Smaller (down to ${TEXT_SIZES[0]}%)` }),
    size,
    button("A+", () => set({ size: stepSize(prefs.reading.size, 1) }), { "aria-label": "Larger text", title: `Larger (up to ${TEXT_SIZES[TEXT_SIZES.length - 1]}%)` }));
  return h("div", { class: "pop-body reading-controls" },
    h("h2", { class: "pop-title" }, "Reading"),
    settingRow("Text size", undefined, stepper),
    choice("font", "Typeface", undefined, [["serif", "Book"], ["sans", "Plain"]]),
    choice("width", "Line length", undefined, [["narrow", "Comfortable"], ["wide", "Full width"]]),
    choice("spacing", "Line spacing", undefined, [["tight", "Tight"], ["normal", "Normal"], ["loose", "Loose"]]),
    choice("theme", "Page", "Read mode's own page colour.", [["auto", "App"], ["light", "Light"], ["sepia", "Sepia"], ["dark", "Dark"]]));
}
