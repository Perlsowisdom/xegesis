// The interface shell: the left rail of places (shell/rail.ts), the main area, and the right sidebar
// of study tools (views/sidebar.ts). Places hold the views: Library (your library, Get Bibles),
// Reader (reading, and its reading plans page), My work (notes, sermons, lists, reports), Search and
// Settings (with Commands under Advanced). Every action in the views calls a registered command
// (api.ts); the shell itself only switches views.

import { bound, onEvent } from "./api";
import { biblesChanged } from "./bibles";
import { button, h } from "./dom";
import { closePopover } from "./popover";
import { closeMarking } from "./marking";
import { rail } from "./shell/rail";
import { PLACES, PlaceId, isView, placeForKey, placeOf, viewLabel } from "./shellwords";
import { applyTheme, loadPref, onPrefs, prefs, savePref, setNavigator } from "./state";
import { binView } from "./views/bin";
import { biblesView } from "./views/bibles";
import { libraryView } from "./views/library";
import { listsView } from "./views/lists";
import { notesSectionView } from "./views/notes";
import { plansView } from "./views/plans";
import { readerView } from "./views/reader";
import { follow, reportView } from "./views/report";
import { highlightsView } from "./highlightspanel";
import { searchFor, searchView } from "./views/search";
import { sermonsView } from "./views/sermons";
import { settingsView } from "./views/settings";
import { sidebar } from "./views/sidebar";
import { WorkSide, workSide } from "./views/work";

interface View {
  el: HTMLElement;
  show(): void | Promise<void>;
  focus?(): void;
  drop?(paths: string[]): void;
  changed?(): void;
}

// "commands" is not a view of its own: it is the Commands page of Settings (#settings/commands).
const makers: Record<string, () => View> = {
  reader: readerView, plans: plansView,
  library: libraryView, bibles: biblesView,
  notes: notesSectionView, sermons: sermonsView, highlights: highlightsView, lists: listsView, reports: reportView, bin: binView,
  search: searchView, settings: settingsView,
};

const LAST_KEY = "xegesis.shell.views";
const views = new Map<string, View>();
const main = h("main", { id: "main", tabindex: "-1" });
const placeNavs = new Map<PlaceId, { el: HTMLElement; links: Map<string, HTMLElement> }>();
// The view each place last showed, so going back to My work returns to the same tab.
const last: Partial<Record<PlaceId, string>> = loadPref(LAST_KEY, {});
let current = "reader";
let theRail: ReturnType<typeof rail>;
let workCol: WorkSide | null = null;

function view(id: string): View {
  let v = views.get(id);
  if (!v) {
    v = makers[id]();
    views.set(id, v);
    main.append(v.el);
  }
  return v;
}

// placeNav is the row of tabs at the top of a place whose views are tabs (the Library).
function placeNav(p: PlaceId): { el: HTMLElement; links: Map<string, HTMLElement> } | null {
  const place = PLACES.find((x) => x.id === p)!;
  if (!place.tabs) return null;
  let nav = placeNavs.get(p);
  if (!nav) {
    const links = new Map<string, HTMLElement>();
    const el = h("nav", { class: "place-tabs", "aria-label": place.label }, h("ul", {}, place.views.map((v) => {
      const a = h("a", { href: `#${v.id}` }, v.label);
      a.addEventListener("click", (e) => {
        e.preventDefault();
        show(v.id, true);
      });
      links.set(v.id, a);
      return h("li", {}, a);
    })));
    nav = { el, links };
    placeNavs.set(p, nav);
    main.prepend(el);
  }
  return nav;
}

function show(target: string, focus = false) {
  let [id, ...rest] = target.split("/");
  let sub = rest.join("/");
  if (id === "commands") [id, sub] = ["settings", "commands"];
  if (!makers[id]) [id, sub] = ["reader", ""];
  // An address inside a view (#sermons/<id>, #settings/model) is kept for the view to open.
  if (sub && !location.hash.startsWith(`#${id}/${sub}`)) history.replaceState(null, "", `#${id}/${sub}`);
  else if (!sub && location.hash !== `#${id}` && !location.hash.startsWith(`#${id}/`)) history.replaceState(null, "", `#${id}`);
  closePopover();
  closeMarking();

  const place = placeOf(id);
  const v = view(id);
  current = id;
  // My work comes back to the folder or tag it showed.
  last[place.id] = id === "notes" && sub ? `${id}/${sub}` : id;
  savePref(LAST_KEY, last);
  for (const [vid, x] of views) x.el.hidden = vid !== id;
  const nav = placeNav(place.id);
  for (const [p, n] of placeNavs) n.el.hidden = p !== place.id;
  // My work keeps its column of folders, tags and writing beside every one of its views.
  const inWork = place.id === "work";
  if (inWork && !workCol) {
    workCol = workSide((t) => show(t, true));
    main.prepend(workCol.el);
    void workCol.reload();
  }
  main.classList.toggle("with-work", inWork);
  if (workCol) {
    workCol.el.hidden = !inWork;
    if (inWork) workCol.setCurrent(id);
  }
  if (nav) for (const [lid, a] of nav.links) {
    if (lid === id) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  }
  theRail.setContext(place.id, prefs.reading.mode);
  document.title = `${viewLabel(id)} · Xegesis`;
  main.scrollTop = 0;
  void v.show();
  if (focus) (v.focus ? v.focus() : main.focus());
}

function goPlace(p: PlaceId, focus = true) {
  const place = PLACES.find((x) => x.id === p)!;
  const want = last[p];
  show(want && place.views.some((v) => v.id === want.split("/")[0]) ? want : place.views[0].id, focus);
}

// boot waits briefly for the desktop app's bindings, which are injected alongside the page.
function boot(tries = 10) {
  const app = document.getElementById("app")!;
  if (!bound() && tries > 0) {
    setTimeout(() => boot(tries - 1), 100);
    return;
  }
  if (!bound()) {
    app.replaceChildren(h("main", { class: "view standalone" },
      h("h1", {}, "Xegesis"),
      h("p", {}, "This page is the Xegesis desktop interface. Run it inside the Xegesis app."),
      button("Try again", () => boot())));
    return;
  }
  const shell = h("div", { class: "shell" });
  theRail = rail(shell, {
    go: (p) => goPlace(p),
    search: (q) => searchFor(q),
    settings: (page) => show(page ? `settings/${page}` : "settings", true),
  });
  const side = sidebar();
  shell.append(theRail.edge, theRail.reveal, theRail.el, main, side.el);
  app.replaceChildren(shell);
  setNavigator((id) => show(id));
  onPrefs(() => theRail.setContext(placeOf(current).id, prefs.reading.mode));
  // The sidebar's close button hands the keyboard back to the page.
  document.addEventListener("xegesis:sidebar-closed", () => main.focus());

  document.addEventListener("keydown", (e) => {
    const ctrl = e.ctrlKey || e.metaKey;
    if (e.altKey && !ctrl && /^[0-9]$/.test(e.key)) {
      const p = placeForKey(e.key);
      if (e.key === "0") {
        e.preventDefault();
        theRail.focus();
      } else if (p) {
        e.preventDefault();
        goPlace(p);
      }
      return;
    }
    if (e.key === "F6" && !ctrl && !e.altKey) {
      e.preventDefault();
      theRail.focus();
    } else if (ctrl && e.key.toLowerCase() === "l") {
      e.preventDefault();
      show("reader", true);
    } else if (ctrl && e.key === ",") {
      e.preventDefault();
      show("settings", true);
    } else if (ctrl && e.key === ".") {
      e.preventDefault();
      side.toggle();
      if (side.isOpen()) side.el.querySelector<HTMLElement>("[role=tab][aria-selected=true]")?.focus();
    } else if (e.key === "Escape" && side.isOpen() && side.el.contains(document.activeElement) && matchMedia("(max-width: 700px)").matches) {
      // At phone width the sidebar is a sheet over the page, so Escape closes it.
      side.toggle();
      main.focus();
    }
  });
  // Links in reports and saved notes are app addresses such as #passage/Rom.8.28 (views/report.ts).
  window.addEventListener("hashchange", () => {
    if (!follow(location.hash)) show(location.hash.slice(1));
  });
  // Files and folders dropped on the window are added to the library.
  onEvent("files:dropped", (paths) => {
    if (!Array.isArray(paths) || paths.length === 0) return;
    show("library");
    view("library").drop?.(paths as string[]);
  });
  onEvent("library:changed", () => {
    views.get("library")?.changed?.();
    void biblesChanged();
  });
  // A Bible installed from the command line or another window shows when you come back.
  window.addEventListener("focus", () => void biblesChanged());
  const start = location.hash.slice(1);
  if (!follow(location.hash)) show(start && isView(start.split("/")[0]) ? start : "reader");
}

// The theme is set before the first paint, so a Light choice never flashes dark.
applyTheme();
onPrefs(applyTheme);
window.addEventListener("load", () => boot());
