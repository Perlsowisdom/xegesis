// Search: words in the installed Bibles. Searching the library's books will join this view once
// the core has a library search command.

import { Message, Module, Ref, paramEnum, run } from "../api";
import { clear, field, h, select } from "../dom";
import { markSegments } from "../helpers";
import { navigate, setModules, setPassage, state } from "../state";

interface Hit {
  id: number;
  end: number;
  module: string;
  ref: string;
  text: string;
  snippet?: string;
  score: number;
}

const MODE_LABELS: Record<string, string> = {
  all: "All the words", any: "Any of the words", phrase: "Exact phrase", fts: "Search syntax",
};

export function searchView() {
  const msg = new Message();
  const query = h("input", { type: "search", id: "search-q", autocomplete: "off", placeholder: "e.g. love one another" });
  const mode = select([], "all");
  paramEnum("bible.search", "mode").then((ms) => ms.forEach((m) => mode.append(h("option", { value: m, selected: m === "all" }, MODE_LABELS[m] ?? m))));
  const within = h("input", { type: "text", placeholder: "e.g. John, or Rom–Gal", autocomplete: "off" });
  const strong = h("input", { type: "text", placeholder: "e.g. G26", autocomplete: "off" });
  const modules = h("div", { class: "module-picker", role: "group", "aria-label": "Bibles to search" });
  const results = h("ol", { class: "results" });
  const count = h("p", { class: "hint", role: "status", "aria-live": "polite" });
  let installed: Module[] = [];

  const form = h("form", { class: "card", role: "search", "aria-label": "Search the Bibles" },
    h("div", { class: "grid3" },
      field("Words", query),
      field("Match", mode),
      field("Only in", within, "Optional reference range")),
    h("div", { class: "grid3" }, field("Strong's number", strong, "Optional; for Bibles tagged with Strong's numbers")),
    modules,
    h("div", { class: "row" }, h("button", { type: "submit", class: "primary" }, "Search")));
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    search();
  });

  const el = h("div", { class: "view" },
    h("h1", {}, "Search"),
    h("p", { class: "hint" }, "Searches the text of your installed Bibles. Searching your library's books is not available yet."),
    form, msg.el, count, results);

  async function search() {
    const p: Record<string, unknown> = { mode: mode.value || "all", limit: 200 };
    if (query.value.trim()) p.query = query.value.trim();
    if (strong.value.trim()) p.strong = strong.value.trim();
    if (within.value.trim()) p.ref = within.value.trim();
    const chosen = [...modules.querySelectorAll<HTMLInputElement>("input:checked")].map((b) => b.value);
    if (chosen.length) p.modules = chosen;
    count.textContent = "Searching…";
    const hits = await msg.attempt(() => run<Hit[]>("bible.search", p));
    if (!hits) return void (count.textContent = "");
    count.textContent = hits.length === 0 ? "No matches." : `${hits.length} matches${hits.length === 200 ? " (showing the best 200)" : ""}.`;
    clear(results, hits.map((hit) => {
      const name = installed.find((m) => m.id === hit.module)?.abbreviation || hit.module;
      const segs = markSegments(hit.snippet || hit.text);
      const open = h("button", { type: "button", class: "link" }, hit.ref);
      open.addEventListener("click", () => openRef(hit));
      return h("li", {},
        h("div", { class: "row" }, open, h("span", { class: "badge" }, name)),
        h("p", {}, segs.map((s) => (s.mark ? h("mark", {}, s.text) : s.text))));
    }));
  }

  async function openRef(hit: Hit) {
    const p = await msg.attempt(() => run<Ref>("verse.parse", { ref: hit.ref }));
    if (!p) return;
    if (!state.modules.includes(hit.module)) setModules([hit.module, ...state.modules]);
    navigate("reader");
    setPassage(p.display, p);
  }

  async function show() {
    installed = (await msg.attempt(() => run<Module[]>("bible.modules"))) ?? [];
    const checked = new Set([...modules.querySelectorAll<HTMLInputElement>("input:checked")].map((b) => b.value));
    clear(modules, h("span", { class: "hint" }, installed.length ? "Bibles:" : "No Bibles installed."),
      installed.map((m) => h("label", { class: "check chip" },
        h("input", { type: "checkbox", value: m.id, checked: checked.size ? checked.has(m.id) : true }), " ", m.abbreviation || m.id)));
  }

  return { el, show, focus: () => query.focus() };
}
