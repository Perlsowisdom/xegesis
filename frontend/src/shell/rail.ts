// The left rail: the places (Search, Library, Reader, My work), the grid presets button in Study
// mode, and Rail settings and Help at the bottom. Condensed shows icons with a tooltip on hover and
// on keyboard focus; Expanded adds labels. With "Hide automatically" the rail slides away and comes
// back over the page when the pointer reaches the left edge, from the "show rail" button, or from
// Alt+0 or F6. At phone width the same element becomes a dock at the bottom (style.css).

import { button, h, id } from "../dom";
import { openGridPresets } from "../gridhook";
import { icon } from "../icons";
import { closePopover, popoverOpen, togglePopover } from "../popover";
import {
  HIDEABLE, PLACES, PlaceId, RAIL_PLACES, ReaderMode, addRecent, gridButtonPlace, railPlaces, toggleHidden,
} from "../shellwords";
import { loadPref, onPrefs, prefs, savePref, setPrefs } from "../state";

export interface RailHooks {
  go(place: PlaceId): void;
  search(query: string): void;
  settings(page?: string): void;
}

const ICONS: Record<string, string> = { search: "search", library: "library", reader: "reader", work: "work" };
const RECENT_KEY = "xegesis.search.recent";

function placeLabel(p: PlaceId): string {
  return PLACES.find((x) => x.id === p)?.label ?? p;
}

// item is one rail entry: an icon and a label, which Condensed shows as a tooltip.
function item(tag: "a" | "button", name: string, label: string, attrs: Record<string, unknown>): HTMLElement {
  return h(tag as "a", { class: "rail-item", ...attrs }, icon(name), h("span", { class: "rail-label" }, label));
}

export function rail(shell: HTMLElement, hooks: RailHooks) {
  let place: PlaceId = "reader";
  let mode: ReaderMode = prefs.reading.mode;

  const links = new Map<PlaceId, HTMLElement>();
  for (const p of RAIL_PLACES) {
    const key = RAIL_PLACES.indexOf(p) + 1;
    const el = p === "search"
      ? item("button", "search", placeLabel(p), { type: "button", "aria-haspopup": "dialog", "aria-expanded": "false", "aria-keyshortcuts": `Alt+${key}` })
      : item("a", ICONS[p], placeLabel(p), { href: `#${PLACES.find((x) => x.id === p)!.views[0].id}`, "aria-keyshortcuts": `Alt+${key}` });
    el.addEventListener("click", (e) => {
      e.preventDefault();
      if (p === "search") openSearch(el);
      else hooks.go(p);
    });
    links.set(p, el);
  }
  const grid = item("button", "grid", "Grid presets", { type: "button", "aria-haspopup": "dialog", "aria-expanded": "false", class: "rail-item rail-grid" });
  grid.addEventListener("click", () => openGridPresets(grid));
  const settingsBtn = item("button", "sliders", "Rail settings", { type: "button", "aria-haspopup": "dialog", "aria-expanded": "false" });
  settingsBtn.addEventListener("click", () => togglePopover(settingsBtn, () => h("div", { class: "pop-body" },
    h("h2", { class: "pop-title" }, "Rail settings"),
    railControls(),
    button("All settings…", () => { closePopover(); hooks.settings(); }, { class: "link", "aria-keyshortcuts": "Control+," })),
  { label: "Rail settings", side: side(), onClose: afterPopover }));
  const helpBtn = item("button", "help", "Help", { type: "button", "aria-haspopup": "dialog", "aria-expanded": "false" });
  helpBtn.addEventListener("click", () => togglePopover(helpBtn, () => helpContent(hooks), { label: "Help", side: side(), onClose: afterPopover }));

  const nav = h("nav", { class: "rail", "aria-label": "Places" },
    h("div", { class: "rail-top" }, [...links.values()], grid),
    h("div", { class: "rail-bottom" }, settingsBtn, helpBtn));
  const edge = h("div", { class: "rail-edge", "aria-hidden": "true" });
  const reveal = h("button", { type: "button", class: "rail-reveal", "aria-label": "Show the rail", title: "Show the rail (Alt+0)" }, icon("rail"));

  // Arrow keys move between rail entries, as in a toolbar.
  nav.addEventListener("keydown", (e) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp" && e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    const items = [...nav.querySelectorAll<HTMLElement>(".rail-item")].filter((x) => x.offsetParent !== null);
    const i = items.indexOf(document.activeElement as HTMLElement);
    if (i < 0) return;
    e.preventDefault();
    const d = e.key === "ArrowDown" || e.key === "ArrowRight" ? 1 : -1;
    items[(i + d + items.length) % items.length].focus();
  });

  function side(): "right" | "below" {
    return matchMedia("(max-width: 700px)").matches ? "below" : "right";
  }

  // Auto-hide -------------------------------------------------------------------------------------
  function peek(on: boolean) {
    shell.classList.toggle("peek", on && prefs.rail.autoHide);
  }
  function keyboardInside(): boolean {
    return nav.querySelector(":focus-visible") !== null;
  }
  function maybeHide(x?: number) {
    if (!prefs.rail.autoHide || !shell.classList.contains("peek")) return;
    if (popoverOpen() || keyboardInside()) return;
    if (x !== undefined && x <= nav.getBoundingClientRect().right + 16) return;
    peek(false);
  }
  function afterPopover() {
    setTimeout(() => maybeHide(lastX), 0);
  }
  let lastX = 0;
  edge.addEventListener("pointerenter", () => peek(true));
  document.addEventListener("pointermove", (e) => {
    lastX = e.clientX;
    maybeHide(e.clientX);
  });
  nav.addEventListener("focusout", () => setTimeout(() => {
    if (!nav.contains(document.activeElement)) maybeHide(lastX);
  }, 0));
  reveal.addEventListener("click", () => focus());

  // Search flyout ---------------------------------------------------------------------------------
  function openSearch(anchor: HTMLElement) {
    togglePopover(anchor, () => {
      const input = h("input", { type: "search", "aria-label": "Search the Bibles", placeholder: "Words to find", autocomplete: "off" });
      const run = (q: string) => {
        q = q.trim();
        if (!q) return;
        savePref(RECENT_KEY, addRecent(loadPref<string[]>(RECENT_KEY, []), q));
        closePopover();
        hooks.search(q);
      };
      const form = h("form", { role: "search" }, input);
      form.addEventListener("submit", (e) => {
        e.preventDefault();
        run(input.value);
      });
      const recent = loadPref<string[]>(RECENT_KEY, []);
      return h("div", { class: "pop-body search-flyout" },
        form,
        recent.length ? [h("h2", { class: "pop-title" }, "Recent searches"),
          h("ul", { class: "plain recent" }, recent.map((q) => h("li", {}, button(q, () => run(q), { class: "link" }))))]
          : h("p", { class: "hint" }, "Search the text of your Bibles. Press Enter to see the results."),
        button("More ways to search…", () => {
          closePopover();
          hooks.go("search");
        }, { class: "link" }));
    }, { label: "Search", side: side(), className: "flyout", onClose: afterPopover });
  }

  // Applying preferences and context --------------------------------------------------------------
  function apply() {
    const r = prefs.rail;
    shell.dataset.rail = r.width;
    shell.classList.toggle("autohide", r.autoHide);
    if (!r.autoHide) shell.classList.remove("peek");
    const shown = railPlaces(r);
    for (const [p, el] of links) el.hidden = !shown.includes(p);
    grid.hidden = !(place === "reader" && gridButtonPlace(mode, r) === "rail");
  }
  onPrefs(apply);
  apply();

  function setContext(p: PlaceId, m: ReaderMode) {
    place = p;
    mode = m;
    for (const [lp, el] of links) {
      if (lp === p) el.setAttribute("aria-current", "page");
      else el.removeAttribute("aria-current");
    }
    apply();
  }

  // focus brings the rail back (when hidden) and puts the keyboard on the current place.
  function focus() {
    peek(true);
    const target = (links.get(place) && !links.get(place)!.hidden ? links.get(place) : links.get("reader"))!;
    target.focus();
  }

  return { el: nav, edge, reveal, setContext, focus, openSearch: () => openSearch(links.get("search")!) };
}

// railControls are the rail's settings, shown in the Rail settings popover and in Settings →
// Appearance. Each change is saved at once.
export function railControls(): HTMLElement {
  const r = prefs.rail;
  const name = id("width");
  const radio = (value: "condensed" | "expanded", label: string) => {
    const box = h("input", { type: "radio", name, value, checked: r.width === value });
    box.addEventListener("change", () => setPrefs({ rail: { ...prefs.rail, width: value } }));
    return h("label", { class: "check" }, box, " ", label);
  };
  const check = (label: string, on: boolean, set: (on: boolean) => void, hint?: string) => {
    const box = h("input", { type: "checkbox", checked: on });
    box.addEventListener("change", () => set(box.checked));
    return h("div", { class: "check-row" }, h("label", { class: "check" }, box, " ", label), hint ? h("small", { class: "hint" }, hint) : null);
  };
  return h("div", { class: "rail-controls" },
    h("fieldset", {}, h("legend", {}, "Width"),
      radio("condensed", "Condensed: icons only"),
      radio("expanded", "Expanded: icons and labels")),
    h("fieldset", {}, h("legend", {}, "Show on the rail"),
      HIDEABLE.map((p) => check(placeLabel(p), !r.hidden.includes(p), (on) => setPrefs({ rail: toggleHidden(prefs.rail, p, !on) })))),
    check("Hide automatically", r.autoHide, (on) => setPrefs({ rail: { ...prefs.rail, autoHide: on } }),
      "The rail comes back when you point at the left edge of the window, or press Alt+0."),
    check("Grid presets on the rail", r.gridOnRail, (on) => setPrefs({ rail: { ...prefs.rail, gridOnRail: on } }),
      "In Study mode. When off, they are in the Reader's top bar."));
}

function helpContent(hooks: RailHooks): HTMLElement {
  const keys: [string, string][] = [
    ["Alt+1 to Alt+4", "Search, Library, Reader, My work"],
    ["Alt+0 or F6", "Go to the rail"],
    ["Ctrl+L", "Type a passage"],
    ["Alt+← and Alt+→", "Previous and next chapter"],
    ["Ctrl+.", "Show or hide the right sidebar"],
    ["Alt+N", "New note"],
    ["Ctrl+,", "Settings"],
  ];
  return h("div", { class: "pop-body help" },
    h("h2", { class: "pop-title" }, "Keyboard shortcuts"),
    h("dl", { class: "keys" }, keys.map(([k, what]) => [h("dt", {}, h("kbd", {}, k)), h("dd", {}, what)])),
    h("div", { class: "pop-links" },
      button("Every command Xegesis has…", () => { closePopover(); hooks.settings("commands"); }, { class: "link" }),
      button("About Xegesis", () => { closePopover(); hooks.settings("about"); }, { class: "link" })));
}
