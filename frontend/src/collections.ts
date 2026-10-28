// Collections: the sidebar tree in the Library, the "New collection" form with its rule builder, the
// bar for managing the collection you are looking at, and the "Search in" picker for Search. Every
// action is a collection.* command; the words here are the ones a reader would use.

import { CollectionNode, CollectionRule, Folder, Message, RulePreview, Vocabulary, run } from "./api";
import { button, clear, field, h, select } from "./dom";
import {
  LANGUAGE_CHOICES, LICENCE_CHOICES, countText, find, formFromRule, indent, matchText, mine, ruleFromForm, ruleIsEmpty,
  ruleSummary, searchChoices, without,
} from "./collectionwords";
import { NOT_SET } from "./facetwords";

export interface RuleBuilder {
  el: HTMLElement;
  // rule is what the fields say now.
  rule(): CollectionRule;
  stop(): void;
}

// ruleBuilder is the form for "books where type is ... and tradition is ...", with a live count.
export function ruleBuilder(vocab: Vocabulary, folders: Folder[], start: CollectionRule, changed: () => void): RuleBuilder {
  const f = formFromRule(start);
  const msg = new Message();
  const type = select([["", "Any type"], ["none", NOT_SET], ...vocab.types.map((t): [string, string] => [t, t])], f.type);
  const tradition = select([["", "Any tradition"], ["none", NOT_SET], ...vocab.traditions.map((t): [string, string] => [t, t])], f.tradition);
  const author = h("input", { type: "text", autocomplete: "off", value: f.author, placeholder: "e.g. Calvin" });
  const text = h("input", { type: "text", autocomplete: "off", value: f.text, placeholder: "e.g. Romans" });
  const language = select(LANGUAGE_CHOICES, f.language);
  const from = h("input", { type: "number", inputmode: "numeric", value: f.yearFrom, placeholder: "e.g. 1500" });
  const to = h("input", { type: "number", inputmode: "numeric", value: f.yearTo, placeholder: "e.g. 1700" });
  const folder = select([["", "Any folder"], ...folders.map((x): [string, string] => [x.id, x.label || x.path])], f.folder);
  const licence = select(LICENCE_CHOICES, f.licence);
  const live = h("p", { class: "hint", role: "status", "aria-live": "polite" });
  const examples = h("ul", { class: "hint examples" });
  let timer: ReturnType<typeof setTimeout> | undefined;
  let latest = 0;

  const read = () => ruleFromForm({
    type: type.value, tradition: tradition.value, author: author.value, text: text.value, language: language.value,
    yearFrom: from.value, yearTo: to.value, folder: folder.value, licence: licence.value,
  });

  async function count() {
    const rule = read();
    const seq = ++latest;
    if (ruleIsEmpty(rule)) {
      live.textContent = "Choose at least one thing to match.";
      clear(examples);
      return;
    }
    const r = await msg.attempt(() => run<RulePreview>("collection.preview", { rule }));
    if (!r || seq !== latest) return;
    live.textContent = matchText(r.count);
    clear(examples, r.examples.map((e) => h("li", {}, e.title || "Untitled", e.authors.length ? ` · ${e.authors.join(", ")}` : "")),
      r.count > r.examples.length ? h("li", {}, "and more") : null);
  }

  function update() {
    changed();
    clearTimeout(timer);
    live.textContent = "Counting…";
    timer = setTimeout(count, 200);
  }
  const controls = [type, tradition, author, text, language, from, to, folder, licence];
  for (const c of controls) c.addEventListener(c instanceof HTMLSelectElement ? "change" : "input", update);

  const el = h("div", { class: "rule-builder" },
    h("div", { class: "grid2" },
      field("Type", type), field("Tradition", tradition), field("Author", author), field("Title contains", text),
      field("Published from (year)", from), field("Published to (year)", to),
      field("Language", language), field("Licence", licence), field("Folder", folder)),
    live, msg.el, examples);
  update();
  return { el, rule: read, stop: () => { clearTimeout(timer); latest++; } };
}

export interface SidebarDeps {
  folders(): Folder[];
  vocab(): Vocabulary;
  // picked is called when an entry is chosen (null for all books).
  picked(node: CollectionNode | null): void;
  // changed is called when a collection's books changed, so the book list reloads.
  changed(): void;
  // say shows a message in the Library.
  say(text: string): void;
}

const ALL = "b:all";

export function collectionsSidebar(d: SidebarDeps) {
  const msg = new Message();
  let tree: CollectionNode[] = [];
  let current: CollectionNode | null = null;
  const open = new Set<string>(["b:type"]);
  const treeEl = h("div", { class: "coll-tree" });
  const bar = h("div", { class: "coll-bar", hidden: true });
  const form = h("div", { class: "card", hidden: true });
  const newButton = button("New collection…", () => startNew([]), { class: "primary" });
  const el = h("nav", { class: "collections", "aria-labelledby": "collections-h" },
    h("h2", { id: "collections-h" }, "Collections"), msg.el, treeEl, newButton, form);
  let builder: RuleBuilder | undefined;

  async function reload() {
    const r = await msg.attempt(() => run<{ tree: CollectionNode[] }>("collection.list", { authors: open.has("b:author") }));
    if (!r) return;
    tree = r.tree;
    if (current) {
      const fresh = find(tree, current.id);
      if (!fresh) {
        current = null;
        d.picked(null);
      } else {
        current = fresh;
      }
    }
    render();
  }

  function render() {
    const builtin = tree.filter((n) => n.kind === "builtin" && (n.id === ALL || !!n.children?.length || n.has_children)), own = tree.filter((n) => n.kind !== "builtin");
    clear(treeEl,
      h("ul", { class: "coll-list" }, builtin.map(row)),
      h("h3", {}, "My collections"),
      own.length ? h("ul", { class: "coll-list" }, own.map(row))
        : h("p", { class: "hint" }, "None yet. Tick some books and choose Add to collection, or make one with New collection."));
    renderBar();
  }

  function row(n: CollectionNode): HTMLElement {
    const kids = n.children ?? [];
    const expandable = kids.length > 0 || !!n.has_children;
    const isOpen = open.has(n.id);
    const toggle = expandable
      ? h("button", { type: "button", class: "icon coll-toggle", "aria-expanded": String(isOpen), "aria-label": `${isOpen ? "Hide" : "Show"} what is inside ${n.name}`,
        onclick: async () => {
          if (isOpen) open.delete(n.id); else open.add(n.id);
          if (n.id === "b:author") await reload(); else render();
        } }, isOpen ? "▾" : "▸")
      : h("span", { class: "coll-gap" });
    const pick = h("button", { type: "button", class: "coll-pick", "aria-current": current?.id === n.id ? "true" : undefined,
      title: n.rule ? ruleSummary(n.rule, folderName) : undefined, onclick: () => choose(n) },
    h("span", {}, n.name), h("span", { class: "count" }, String(n.count)));
    return h("li", {}, h("div", { class: "coll-row" }, toggle, pick),
      isOpen && kids.length ? h("ul", { class: "coll-list" }, kids.map(row)) : null);
  }

  const folderName = (id: string) => d.folders().find((x) => x.id === id)?.label ?? "";

  function choose(n: CollectionNode | null) {
    current = n && n.id !== ALL ? n : null;
    render();
    d.picked(current);
  }

  // The bar over the book list ----------------------------------------------------------------

  function renderBar() {
    if (!current) {
      bar.hidden = true;
      return;
    }
    bar.hidden = false;
    const n = current;
    const own = n.kind !== "builtin";
    clear(bar,
      h("div", { class: "row between wrap" },
        h("p", {}, "Showing ", h("strong", {}, n.name), ` · ${countText(n.count)}`,
          n.rule ? h("small", { class: "hint" }, h("br"), `Books where: ${ruleSummary(n.rule, folderName)}`) : null),
        h("span", { class: "row" },
          button("Show all books", () => choose(null), { class: "link" }))),
      own ? manage(n) : null);
  }

  function manage(n: CollectionNode): HTMLElement {
    const bmsg = new Message();
    const name = h("input", { type: "text", value: n.name, autocomplete: "off" });
    const siblings = (n.parent_id ? find(tree, n.parent_id)?.children : tree.filter((x) => x.kind !== "builtin")) ?? [];
    const at = siblings.findIndex((x) => x.id === n.id);
    const inside = select([["", "Nothing (top level)"], ...without(mine(tree), n.id).map((f): [string, string] => [f.node.id, indent(f.depth, f.node.name)])], n.parent_id ?? "");
    const ruleBox = h("div", { hidden: true });

    const act = async (id: string, p: Record<string, unknown>, done: string) => {
      if (!(await bmsg.attempt(() => run(id, { id: n.id, ...p })))) return;
      d.say(done);
      await reload();
      d.changed();
    };
    inside.addEventListener("change", () => act("collection.update", { parent_id: inside.value }, `Moved ${n.name}.`));

    const changeRule = button("Change the rule", () => {
      if (!ruleBox.hidden) { builder?.stop(); ruleBox.hidden = true; return; }
      builder = ruleBuilder(d.vocab(), d.folders(), n.rule ?? {}, () => {});
      const b = builder;
      clear(ruleBox, b.el, h("div", { class: "row" },
        button("Save rule", () => {
          if (ruleIsEmpty(b.rule())) return bmsg.info("Choose at least one thing to match.");
          act("collection.update", { rule: b.rule() }, `Saved the rule for ${n.name}.`);
        }, { class: "primary" }), button("Cancel", () => { b.stop(); ruleBox.hidden = true; })));
      ruleBox.hidden = false;
    });

    return h("details", { class: "more" }, h("summary", {}, "Change this collection"),
      h("div", { class: "row wrap end" }, field("Name", name),
        button("Rename", () => {
          if (name.value.trim() && name.value.trim() !== n.name) act("collection.update", { name: name.value.trim() }, "Renamed.");
        })),
      h("div", { class: "row wrap" },
        button("Move up", () => act("collection.update", { move: "up" }, "Moved up."), { disabled: at <= 0 }),
        button("Move down", () => act("collection.update", { move: "down" }, "Moved down."), { disabled: at < 0 || at >= siblings.length - 1 }),
        n.kind === "rule" ? changeRule : null),
      h("div", { class: "row wrap end" }, field("Put it inside", inside)),
      ruleBox, bmsg.el,
      h("div", { class: "row" }, button("Delete this collection", () => remove(n), { class: "danger" })));
  }

  async function remove(n: CollectionNode) {
    const inner = collect(n);
    const extra = inner.length > 1 ? ` The ${inner.length - 1} ${inner.length === 2 ? "collection" : "collections"} inside it go too.` : "";
    if (!confirm(`Delete ${n.name}?${extra} Your books are not deleted.`)) return;
    if (!(await msg.attempt(() => run("collection.delete", { id: n.id })))) return;
    current = null;
    await reload();
    d.picked(null);
    undoLink(n.name, inner);
  }

  // undoLink offers to bring back what was just deleted, from the change history.
  function undoLink(name: string, ids: string[]) {
    const undo = button("Undo", async () => {
      for (const id of ids) {
        const h = await msg.attempt(() => run<{ id: string; op: string }[]>("history.list", { object_type: "collections", object_id: id }));
        const before = h && h.length >= 2 ? h[h.length - 2] : undefined;
        if (before) await msg.attempt(() => run("history.restore", { object_type: "collections", object_id: id, change_id: before.id }));
      }
      msg.clear();
      d.say(`Brought back ${name}.`);
      await reload();
      d.changed();
    }, { class: "link" });
    msg.info(`Deleted ${name}. `);
    msg.el.append(undo);
  }

  const collect = (n: CollectionNode): string[] => [n.id, ...(n.children ?? []).flatMap(collect)];

  // New collection ------------------------------------------------------------------------------

  // startNew opens the form; ids are the books ticked in the list ("From selected books").
  function startNew(ids: string[]) {
    builder?.stop();
    const radioName = `coll-kind-${Math.random().toString(36).slice(2)}`;
    const fromSel = h("input", { type: "radio", name: radioName, value: "selected", checked: ids.length > 0, disabled: ids.length === 0 });
    const byRule = h("input", { type: "radio", name: radioName, value: "rule", checked: ids.length === 0 });
    const name = h("input", { type: "text", autocomplete: "off", placeholder: "e.g. For Sunday" });
    const inside = select([["", "Nothing (top level)"], ...mine(tree).map((f): [string, string] => [f.node.id, indent(f.depth, f.node.name)])], "");
    const ruleBox = h("div", { hidden: ids.length > 0 });
    const fmsg = new Message();
    const b = ruleBuilder(d.vocab(), d.folders(), {}, () => {});
    builder = b;
    ruleBox.append(b.el);
    const sync = () => { ruleBox.hidden = !byRule.checked; };
    fromSel.addEventListener("change", sync);
    byRule.addEventListener("change", sync);

    const create = async () => {
      if (!name.value.trim()) return fmsg.info("Give the collection a name.");
      const p: Record<string, unknown> = { name: name.value.trim() };
      if (inside.value) p.parent_id = inside.value;
      if (byRule.checked) {
        const rule = b.rule();
        if (ruleIsEmpty(rule)) return fmsg.info("Choose at least one thing to match.");
        p.rule = rule;
      } else {
        p.item_ids = ids;
      }
      const r = await fmsg.attempt(() => run<{ id: string; count: number }>("collection.create", p));
      if (!r) return;
      b.stop();
      form.hidden = true;
      d.say(`Made ${name.value.trim()} with ${countText(r.count)}.`);
      await reload();
      const made = find(tree, r.id);
      if (made) choose(made);
    };

    clear(form,
      h("fieldset", {}, h("legend", {}, "New collection"),
        h("label", { class: "choice" }, fromSel, h("span", {}, ids.length ? `From the ${countText(ids.length)} I ticked` : "From the books I tick (tick some first)")),
        h("label", { class: "choice" }, byRule, h("span", {}, "By rule: books that match what I choose")),
        field("Name", name), ruleBox,
        h("details", { class: "more" }, h("summary", {}, "More options"), field("Put it inside", inside))),
      fmsg.el,
      h("div", { class: "row" }, button("Make collection", create, { class: "primary" }),
        button("Cancel", () => { b.stop(); form.hidden = true; })));
    form.hidden = false;
    name.focus();
  }

  // addTo is the picker for "Add to collection" on the ticked books.
  function addTo(ids: string[], done: () => void): HTMLElement {
    const hand = mine(tree).filter((f) => f.node.kind === "manual");
    const target = select(hand.map((f): [string, string] => [f.node.id, indent(f.depth, f.node.name)]));
    const amsg = new Message();
    return h("fieldset", {}, h("legend", {}, `Add ${countText(ids.length)} to a collection`),
      hand.length ? field("Collection", target) : h("p", { class: "hint" }, "You have no hand-picked collections yet."),
      amsg.el,
      h("div", { class: "row" },
        hand.length ? button("Add", async () => {
          const r = await amsg.attempt(() => run<{ changed: number; count: number }>("collection.add", { collection_id: target.value, item_ids: ids }));
          if (!r) return;
          const name = hand.find((f) => f.node.id === target.value)?.node.name ?? "the collection";
          d.say(r.changed === ids.length ? `Added ${countText(r.changed)} to ${name}.` : `Added ${countText(r.changed)} to ${name}; the others were already there.`);
          done();
          await reload();
          d.changed();
        }, { class: "primary" }) : null,
        button("New collection from these books…", () => { done(); startNew(ids); form.scrollIntoView({ block: "nearest" }); })));
  }

  async function removeFrom(ids: string[]) {
    if (!current || current.kind !== "manual") return;
    const name = current.name;
    const r = await msg.attempt(() => run<{ changed: number }>("collection.remove", { collection_id: current!.id, item_ids: ids }));
    if (!r) return;
    d.say(`Took ${countText(r.changed)} out of ${name}. They are still in your library.`);
    await reload();
    d.changed();
  }

  return {
    el, bar, reload, addTo, removeFrom, startNew,
    current: () => current,
    tree: () => tree,
    // pick selects an entry by ID (null for all books) without telling the library.
    reset() { current = null; render(); },
  };
}

// collectionPicker is the "Search in" list: everything, one of your collections, or a built-in grouping.
export function collectionPicker() {
  const sel = h("select", {}, h("option", { value: "" }, "Everything in my library"));
  async function reload() {
    const r = await run<{ tree: CollectionNode[] }>("collection.list").catch(() => null);
    if (!r) return;
    const keep = sel.value;
    clear(sel, h("option", { value: "" }, "Everything in my library"),
      searchChoices(r.tree).map((g) => h("optgroup", { label: g.label }, g.options.map(([v, l]) => h("option", { value: v }, l)))));
    sel.value = [...sel.options].some((o) => o.value === keep) ? keep : "";
  }
  return { el: field("Search in", sel), reload, value: () => sel.value };
}
