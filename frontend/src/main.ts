// The interface shell: sidebar navigation, sections and keyboard shortcuts. Every action in the
// sections calls a registered command (api.ts); the shell itself only switches views.

import { bound, onEvent } from "./api";
import { button, h } from "./dom";
import { notesPanel } from "./notes";
import { setNavigator } from "./state";
import { biblesView } from "./views/bibles";
import { commandsView } from "./views/commands";
import { libraryView } from "./views/library";
import { plansView } from "./views/plans";
import { readerView } from "./views/reader";
import { searchView } from "./views/search";
import { settingsView } from "./views/settings";

interface View {
  el: HTMLElement;
  show(): void | Promise<void>;
  focus?(): void;
  drop?(paths: string[]): void;
  changed?(): void;
}

function notesView(): View {
  const panel = notesPanel({ heading: "All notes", scope: () => null, headingLevel: "h2" });
  return { el: h("div", { class: "view" }, h("h1", {}, "Notes"), panel.el), show: panel.refresh };
}

const sections: { id: string; label: string; make: () => View }[] = [
  { id: "reader", label: "Reader", make: readerView },
  { id: "bibles", label: "Bibles", make: biblesView },
  { id: "library", label: "Library", make: libraryView },
  { id: "notes", label: "Notes", make: notesView },
  { id: "plans", label: "Plans", make: plansView },
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
      const a = h("a", { href: `#${s.id}`, "aria-keyshortcuts": `Alt+${i + 1}`, title: `${s.label} (Alt+${i + 1})` }, s.label);
      a.addEventListener("click", (e) => {
        e.preventDefault();
        show(s.id, true);
      });
      links.set(s.id, a);
      return h("li", {}, a);
    })),
    h("p", { class: "hint shortcuts" }, "Alt+1–8 sections · Ctrl+L passage · Alt+N new note"));
  app.replaceChildren(h("div", { class: "shell" }, nav, main));
  setNavigator((id) => show(id));

  document.addEventListener("keydown", (e) => {
    if (e.altKey && !e.ctrlKey && /^[1-9]$/.test(e.key)) {
      const s = sections[Number(e.key) - 1];
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
  window.addEventListener("hashchange", () => show(location.hash.slice(1)));
  // Files and folders dropped on the window are added to the library.
  onEvent("files:dropped", (paths) => {
    if (!Array.isArray(paths) || paths.length === 0) return;
    show("library");
    view("library").drop?.(paths as string[]);
  });
  onEvent("library:changed", () => views.get("library")?.changed?.());
  show(location.hash.slice(1) || "reader");
}

window.addEventListener("load", () => boot());
