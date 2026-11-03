// The Lists section: saved lists of verses, words and book excerpts. A list opens, can be renamed,
// reordered and emptied, and its verses and excerpts open where they came from.

import { printExportButton } from "../printexport";
import { ListEntry, Message, Ref, SavedList, run } from "../api";
import { button, clear, field, h, select } from "../dom";
import { Locator, openBook } from "../document/open";
import { setPassage, state } from "../state";
import { navigate } from "../state";
import { flash } from "../listpicker";

const KINDS: [SavedList["kind"], string][] = [["verses", "Verses"], ["words", "Words"], ["excerpts", "Excerpts from books"]];
export const KIND_NAME: Record<string, string> = { verses: "Verse list", words: "Word list", excerpts: "Excerpt list" };

export function listsView() {
  const msg = new Message();
  const sidebar = h("div", { class: "lists-index" });
  const detail = h("section", { class: "list-detail", "aria-label": "List" });
  const el = h("div", { class: "view" }, h("h1", {}, "Lists"),
    h("p", { class: "hint" }, "Keep verses, words and quotes from your books together. Add to a list from the Reader, the word panel, or a selection in a book."),
    msg.el, h("div", { class: "lists-layout" }, sidebar, detail));

  let lists: SavedList[] = [];
  let open: string | null = null;

  async function refresh() {
    const got = await msg.attempt(() => run<SavedList[]>("list.list"));
    if (!got) return;
    lists = got;
    renderIndex();
    if (open && !lists.some((l) => l.id === open)) open = null;
    if (open) await showList(open);
    else clear(detail, h("p", { class: "empty" }, lists.length ? "Choose a list to see what is in it." : "You have no lists yet. Make one on the left."));
  }

  function renderIndex() {
    const name = h("input", { type: "text", placeholder: "Name of the new list" });
    const kind = select(KINDS.map(([k, l]) => [k, l] as [string, string]), "verses", { "aria-label": "What the list holds" });
    const form = h("form", { class: "card" }, field("New list", name), field("Holds", kind),
      h("button", { type: "submit", class: "primary" }, "Make list"));
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      const l = await msg.attempt(() => run<SavedList>("list.create", { kind: kind.value, name: name.value }));
      if (l) {
        open = l.id;
        await refresh();
      }
    });
    clear(sidebar, h("h2", {}, "Your lists"),
      lists.length === 0 ? null : h("ul", { class: "plain" }, lists.map((l) => h("li", {},
        h("button", { type: "button", class: "list-pick", "aria-current": l.id === open ? "true" : null, onclick: () => { open = l.id; renderIndex(); showList(l.id); } },
          h("strong", {}, l.name), h("span", { class: "hint" }, ` ${KIND_NAME[l.kind]} · ${l.count}`))))),
      form);
  }

  async function showList(id: string) {
    const data = await msg.attempt(() => run<{ list: SavedList; entries: ListEntry[] }>("list.items", { id }));
    if (!data) return;
    const { list, entries } = data;
    const titleRow = h("div", { class: "row between" });
    const rename = button("Rename", () => {
      const input = h("input", { type: "text", value: list.name, "aria-label": "Name of the list" });
      const f = h("form", { class: "row" }, input, h("button", { type: "submit", class: "primary" }, "Save"), button("Cancel", () => showList(id)));
      f.addEventListener("submit", async (ev) => {
        ev.preventDefault();
        if ((await msg.attempt(() => run("list.rename", { id, name: input.value }))) !== undefined) refresh();
      });
      clear(titleRow, f);
      input.select();
    });
    const del = button("Delete list", async () => {
      if (!confirm(`Delete "${list.name}"? You can get it back from the change history.`)) return;
      if ((await msg.attempt(() => run("list.delete", { id }))) !== undefined) {
        open = null;
        flash(`Deleted "${list.name}".`);
        refresh();
      }
    }, { class: "danger" });

    function move(i: number, d: number) {
      const ids = entries.map((e) => e.id);
      [ids[i], ids[i + d]] = [ids[i + d], ids[i]];
      msg.attempt(() => run("list.reorder", { list: id, ids })).then((r) => r && showList(id));
    }
    async function remove(e: ListEntry) {
      if ((await msg.attempt(() => run("list.remove", { id: e.id }))) !== undefined) {
        refresh();
        flash("Removed from the list.");
      }
    }

    clear(detail,
      (clear(titleRow, h("h2", {}, list.name), h("div", { class: "row" }, printExportButton(() => ({ title: list.name,
        sources: [{ kind: "list", id: list.id, ...(list.kind === "verses" && state.modules[0] ? { module: state.modules[0] } : {}) }] })), rename, del)), titleRow),
      h("p", { class: "hint" }, `${KIND_NAME[list.kind]} · ${entries.length} ${entries.length === 1 ? "entry" : "entries"}`),
      entries.length === 0 ? h("p", { class: "empty" }, emptyHint(list.kind)) :
        h("ol", { class: "entries" }, entries.map((e, i) => h("li", { class: "entry-row" },
          h("div", { class: "entry-main" }, entryBody(list.kind, e)),
          h("div", { class: "row small entry-actions" },
            button("↑", () => move(i, -1), { disabled: i === 0, "aria-label": `Move ${entryName(e)} up`, title: "Move up" }),
            button("↓", () => move(i, 1), { disabled: i === entries.length - 1, "aria-label": `Move ${entryName(e)} down`, title: "Move down" }),
            button("Remove", () => remove(e), { class: "danger", "aria-label": `Remove ${entryName(e)}` }))))));
  }

  function show() {
    return refresh();
  }
  return { el, show };
}

export function emptyHint(kind: string): string {
  return kind === "verses" ? "Empty. In the Reader, select verses and choose Add to list."
    : kind === "words" ? "Empty. In the Reader, select a word and choose Add to a word list."
      : "Empty. In a book, select some text and choose Add to list.";
}

function entryName(e: ListEntry): string {
  return e.ref?.display ?? e.lemma ?? e.word ?? (e.text ? e.text.slice(0, 30) : "entry");
}

// entryBody draws one entry of a list. go, when given, shows a passage there (a list tab in the
// study grid moves its linked panes); otherwise verses open in the Reader.
export function entryBody(kind: string, e: ListEntry, go?: (display: string) => void): Node[] {
  const note = e.note ? [h("p", { class: "hint" }, e.note)] : [];
  if (kind === "verses" && e.ref) {
    const ref = e.ref;
    return [button(ref.display, async () => {
      if (go) return go(ref.display);
      const p = await run<Ref>("verse.parse", { ref: ref.osis }).catch(() => null);
      if (!p) return;
      navigate("reader");
      setPassage(p.display, p);
    }, { class: "link", title: "Open in the Reader" }), ...note];
  }
  if (kind === "words") {
    return [h("strong", {}, e.lemma || e.word || ""), document.createTextNode(" "),
      h("span", { class: "hint" }, [e.word && e.word !== e.lemma ? `“${e.word}”` : "", e.strong, e.language].filter(Boolean).join(" · ")), ...note];
  }
  const loc = e.locator ?? {};
  const where: Locator = { spine: Number(loc.section) || undefined, page: Number(loc.page) || undefined, para: Number(loc.para) || undefined, offset: Number(loc.offset) || undefined };
  return [
    h("blockquote", { class: "excerpt" }, e.text ?? ""),
    e.attribution ? h("p", { class: "attribution" }, e.attribution) : null,
    e.item_id ? button(e.title || "Open the book", () => {
      navigate("reader");
      openBook({ itemId: e.item_id!, locator: where });
    }, { class: "link", title: "Open the book at this place" }) : null,
    ...note,
  ].filter(Boolean) as Node[];
}
