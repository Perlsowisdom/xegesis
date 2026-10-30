// The interface shell: sidebar navigation, sections and keyboard shortcuts. Every action in the
// sections calls a registered command (api.ts); the shell itself only switches views.

import { bound, onEvent } from "./api";
import { button, h } from "./dom";
import { setNavigator } from "./state";
import { biblesView } from "./views/bibles";
import { commandsView } from "./views/commands";
import { libraryView } from "./views/library";
import { listsView } from "./views/lists";
import { notesSectionView } from "./views/notes";
import { plansView } from "./views/plans";
import { readerView } from "./views/reader";
import { follow, reportView } from "./views/report";
import { searchView } from "./views/search";
import { settingsView } from "./views/settings";

interface View {
  el: HTMLElement;
  show(): void | Promise<void>;
  focus?(): void;
  drop?(paths: string[]): void;
  changed?(): void;
}

const sections: { id: string; label: string; make: () => View }[] = [
  { id: "reader", label: "Reader", make: readerView },
  { id: "bibles", label: "Bibles", make: biblesView },
  { id: "library", label: "Library", make: libraryView },
  { id: "notes", label: "Notes", make: notesSectionView },
  { id: "lists", label: "Lists", make: listsView },
  { id: "plans", label: "Plans", make: plansView },
  { id: "reports", label: "Reports", make: reportView },
  { id: "search", label: "Search", make: searchView },
  { id: "commands", label: "Commands", make: commandsView },
  { id: "settings", label: "Settings", make: settingsView },
];

const views = new Map<string, View>();
const main = h("main", { id: "main", tabindex: "-1" });
const links = new Map<string, HTMLAnchorElement>();

function view(id: string): View {
  const s = sections.find((x) => x.id === id) ?? sections[0];
  let v = views.get(s.id);
  if (!v) {
    v = s.make();
    views.set(s.id, v);
    main.append(v.el);
  }
  return v;
}

function show(id: string, focus = false) {
  const s = sections.find((x) => x.id === id) ?? sections[0];
  const v = view(s.id);
  for (const [vid, view] of views) view.el.hidden = vid !== s.id;
  for (const [lid, a] of links) {
    if (lid === s.id) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  }
  document.title = `${s.label} · Xegesis`;
  if (location.hash !== `#${s.id}`) history.replaceState(null, "", `#${s.id}`);
  v.show();
  if (focus) (v.focus ? v.focus() : main.focus());
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
  const nav = h("nav", { class: "sidebar", "aria-label": "Sections" },
    h("div", { class: "brand" }, "Xegesis"),
    h("ul", {}, sections.map((s, i) => {
      const k = i === 9 ? 0 : i + 1;
      const a = h("a", { href: `#${s.id}`, "aria-keyshortcuts": i < 10 ? `Alt+${k}` : undefined, title: i < 10 ? `${s.label} (Alt+${k})` : s.label }, s.label);
      a.addEventListener("click", (e) => {
        e.preventDefault();
        show(s.id, true);
      });
      links.set(s.id, a);
      return h("li", {}, a);
    })),
    h("p", { class: "hint shortcuts" }, `Alt+1–${Math.min(sections.length, 9)}${sections.length > 9 ? ", 0" : ""} sections · Ctrl+L passage · Alt+N new note`));
  app.replaceChildren(h("div", { class: "shell" }, nav, main));
  setNavigator((id) => show(id));

  document.addEventListener("keydown", (e) => {
    if (e.altKey && !e.ctrlKey && /^[0-9]$/.test(e.key)) {
      const s = sections[e.key === "0" ? 9 : Number(e.key) - 1];
      if (s) {
        e.preventDefault();
        show(s.id, true);
      }
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "l") {
      e.preventDefault();
      show("reader", true);
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
  onEvent("library:changed", () => views.get("library")?.changed?.());
  if (!follow(location.hash)) show(location.hash.slice(1) || "reader");
}

window.addEventListener("load", () => boot());
