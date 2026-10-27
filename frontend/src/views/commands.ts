// Commands: every registered command with its schema, run with JSON. This is the same surface
// the CLI and MCP server offer, so anything the interface does can be repeated here.

import { CommandInfo, Message, commands, runRaw } from "../api";
import { clear, h } from "../dom";
import { exampleParams } from "../helpers";

export function commandsView() {
  const msg = new Message();
  const filter = h("input", { type: "search", id: "cmd-filter", placeholder: "Find a command", autocomplete: "off" });
  const list = h("ul", { class: "cmd-list", role: "listbox", "aria-label": "Commands" });
  const title = h("h2", {}, "Choose a command");
  const summary = h("p", { class: "hint" }, "Everything Xegesis does is a command, here, in the CLI and for AI agents.");
  const params = h("textarea", { id: "cmd-params", rows: 8, spellcheck: "false", class: "mono" }, "{}");
  const runBtn = h("button", { type: "submit", class: "primary", disabled: true }, "Run");
  const out = h("pre", { class: "mono out", "aria-live": "polite", tabindex: "0", "aria-label": "Result" });
  const schema = h("details", {}, h("summary", {}, "Schema"), h("pre", { class: "mono" }));
  let all: CommandInfo[] = [];
  let current: CommandInfo | null = null;

  const form = h("form", { class: "console" },
    title, summary,
    h("label", { for: "cmd-params" }, "Parameters (JSON)"), params,
    h("div", { class: "row" }, runBtn, h("span", { class: "hint" }, "Ctrl+Enter runs")),
    msg.el, out, schema);
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!current) return;
    out.textContent = "";
    const res = await msg.attempt(() => runRaw(current!.id, params.value));
    if (res !== undefined) {
      out.textContent = res ? JSON.stringify(JSON.parse(res), null, 2) : "(no result)";
      msg.ok("Done.");
    }
  });
  params.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) form.requestSubmit();
  });
  filter.addEventListener("input", render);
  filter.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      list.querySelector<HTMLElement>("button")?.focus();
    }
  });
  list.addEventListener("keydown", (e) => {
    const items = [...list.querySelectorAll<HTMLElement>("button")];
    const i = items.indexOf(document.activeElement as HTMLElement);
    if (e.key === "ArrowDown" && i < items.length - 1) { e.preventDefault(); items[i + 1].focus(); }
    if (e.key === "ArrowUp") { e.preventDefault(); (i > 0 ? items[i - 1] : filter).focus(); }
  });

  const el = h("div", { class: "view" },
    h("h1", {}, "Commands"),
    h("div", { class: "panes commands" },
      h("div", { class: "pane" }, h("label", { for: "cmd-filter", class: "sr-only" }, "Find a command"), filter, list),
      h("div", { class: "pane" }, form)));

  function render() {
    const q = filter.value.toLowerCase();
    clear(list, all.filter((c) => !q || (c.id + " " + c.summary).toLowerCase().includes(q)).map((c) =>
      h("li", { role: "option", "aria-selected": String(c === current) },
        h("button", { type: "button", class: c === current ? "on" : "", onclick: () => choose(c) },
          h("code", {}, c.id), c.writes ? h("span", { class: "badge", title: "Recorded in history" }, "writes") : null,
          h("small", {}, c.summary)))));
  }

  function choose(c: CommandInfo) {
    current = c;
    title.textContent = c.id;
    summary.textContent = c.summary + (c.writes ? " Recorded in the change history." : "");
    params.value = exampleParams(c.params);
    schema.querySelector("pre")!.textContent = JSON.stringify({ params: c.params, result: c.result }, null, 2);
    runBtn.disabled = false;
    out.textContent = "";
    msg.clear();
    render();
    params.focus();
  }

  async function show() {
    if (!all.length) all = await commands();
    render();
  }

  return { el, show, focus: () => filter.focus() };
}
