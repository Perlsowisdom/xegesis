// "Add to list": a small dialog that lets people pick one of their lists of a kind, or start a new
// one, and adds something to it. Used by the Reader (verses), the word panel (words) and the book
// pane (excerpts).

import { ListEntry, Message, SavedList, run } from "./api";
import { button, clear, field, h } from "./dom";

const NOUN = { verses: "verses", words: "words", excerpts: "excerpts" } as const;

// addToList opens the dialog. add receives the chosen list's ID and returns the params for list.add.
export async function addToList(kind: SavedList["kind"], what: string, params: Record<string, unknown> | Record<string, unknown>[], done?: (list: SavedList, entry: ListEntry) => void) {
  const msg = new Message();
  const dlg = h("dialog", { class: "drawer", "aria-labelledby": "atl-title" });
  const body = h("div", {});
  dlg.append(h("div", { class: "row between" }, h("h3", { id: "atl-title" }, `Add ${what} to a list`),
    button("Close", () => dlg.close(), { "aria-label": "Close" })), msg.el, body);
  dlg.addEventListener("close", () => dlg.remove());
  document.body.append(dlg);
  dlg.showModal();

  const lists = (await msg.attempt(() => run<SavedList[]>("list.list", { kind }))) ?? [];

  async function add(list: SavedList) {
    let entry: ListEntry | undefined;
    let fresh = 0;
    for (const p of Array.isArray(params) ? params : [params]) {
      entry = await msg.attempt(() => run<ListEntry>("list.add", { list: list.id, ...p }));
      if (!entry) return;
      if (!entry.already) fresh++;
    }
    dlg.close();
    if (entry) done?.(list, entry);
    flash(fresh === 0 ? `Already in "${list.name}".` : `Added to "${list.name}".`);
  }

  const name = h("input", { type: "text", placeholder: `e.g. ${kind === "verses" ? "Verses on grace" : kind === "words" ? "Greek words of love" : "Quotes for the sermon"}` });
  const create = h("form", { class: "row" }, field("New list", name),
    h("button", { type: "submit", class: "primary" }, "Create and add"));
  create.addEventListener("submit", async (e) => {
    e.preventDefault();
    const list = await msg.attempt(() => run<SavedList>("list.create", { kind, name: name.value || `My ${NOUN[kind]}` }));
    if (list) add(list);
  });
  clear(body,
    lists.length ? h("ul", { class: "plain" }, lists.map((l) => h("li", { class: "row between" },
      h("span", {}, l.name, " ", h("span", { class: "hint" }, `(${l.count})`)),
      button("Add here", () => add(l), { "aria-label": `Add to ${l.name}` })))) : h("p", { class: "hint" }, `You have no lists of ${NOUN[kind]} yet.`),
    create);
  (lists.length ? (body.querySelector("button") as HTMLElement) : name)?.focus();
}

// flash shows a short confirmation that fades away; it is also announced to screen readers.
export function flash(text: string) {
  let box = document.getElementById("flash");
  if (!box) {
    box = h("div", { id: "flash", class: "flash", role: "status" });
    document.body.append(box);
  }
  box.textContent = text;
  box.classList.add("show");
  setTimeout(() => box?.classList.remove("show"), 3500);
}
