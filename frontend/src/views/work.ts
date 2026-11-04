// My work's left column, beside every My work view: Folders ("All notes" and the user's folders,
// made, renamed, nested and deleted here), Tags with their counts, and Writing (sermons, lists,
// reading plans and reports). Each section folds away (▾ and ▸, remembered on this device), and
// the Bin stays pinned at the bottom. Notes and sermons dropped on a folder move into it.

import { Message, TagCount, run } from "../api";
import { button, clear, h } from "../dom";
import { icon } from "../icons";
import { openPopover } from "../popover";
import { loadPref, savePref } from "../state";
import { carriesItem, droppedItem, folders, loadFolders, moveItem, onWorkChanged, workChanged } from "../work";
import { BinEntry, FolderNode, Scope, canNestUnder, folderTree, sameScope, scopeHash, scopeOf } from "../workwords";

type Section = "folders" | "tags" | "writing";
const OPEN_KEY = "xegesis.work.sections";
const FOLDERS_KEY = "xegesis.work.openFolders";
const FOLDER_TYPE = "application/x-xegesis-folder";

const WRITING: { view: string; label: string; icon: string }[] = [
  { view: "sermons", label: "Sermons", icon: "sermon" },
  { view: "lists", label: "Lists", icon: "list" },
  { view: "plans", label: "Reading plans", icon: "plan" },
  { view: "reports", label: "Reports", icon: "report" },
];

export interface WorkSide {
  el: HTMLElement;
  setCurrent(view: string): void;
  reload(): Promise<void>;
}

export function workSide(go: (target: string) => void): WorkSide {
  const msg = new Message();
  const open: Record<Section, boolean> = { folders: true, tags: true, writing: true, ...loadPref<Partial<Record<Section, boolean>>>(OPEN_KEY, {}) };
  const openFolders = new Set<string>(loadPref<string[]>(FOLDERS_KEY, []));
  let view = "notes";
  let scope: Scope = { kind: "all" };
  let tags: TagCount[] = [];
  let total = 0;
  let binCount = 0;
  let editing: { kind: "new"; parent?: string } | { kind: "rename"; id: string } | null = null;

  const folderList = h("ul", { class: "work-tree", role: "list" });
  const tagList = h("ul", { class: "work-tree", role: "list" });
  const writingList = h("ul", { class: "work-tree", role: "list" });
  const addFolder = h("button", { type: "button", class: "icon-btn work-add", title: "New folder", "aria-label": "New folder" }, icon("plus"));
  addFolder.addEventListener("click", () => startNew());

  const sections: Record<Section, HTMLElement> = {
    folders: section("folders", "Folders", folderList, addFolder),
    tags: section("tags", "Tags", tagList),
    writing: section("writing", "Writing", writingList),
  };
  const bin = h("a", { href: "#bin", class: "work-link work-bin" }, icon("bin"), h("span", { class: "work-name" }, "Bin"), h("span", { class: "count" }));
  bin.addEventListener("click", (e) => { e.preventDefault(); go("bin"); });
  const el = h("aside", { class: "work-side", "aria-label": "My work" },
    h("div", { class: "work-scroll" }, h("p", { class: "work-title" }, "My work"), msg.el, sections.folders, sections.tags, sections.writing),
    h("div", { class: "work-foot" }, bin));

  // The Folders heading takes a folder dropped on it back to the top.
  dropTarget(sections.folders.querySelector(".work-sec-head")!, (e) => {
    const f = draggedFolder(e);
    return f && rows().find((r) => r.id === f)?.parent_id ? "" : null;
  }, false);

  function section(id: Section, label: string, list: HTMLElement, extra?: HTMLElement): HTMLElement {
    const listId = `work-${id}`;
    list.id = listId;
    const toggle = h("button", { type: "button", class: "work-sec-toggle", "aria-expanded": String(open[id]), "aria-controls": listId },
      h("span", { class: "caret", "aria-hidden": "true" }, open[id] ? "▾" : "▸"), h("span", {}, label));
    const sec = h("section", { class: "work-sec", "data-open": String(open[id]) }, h("div", { class: "work-sec-head" }, toggle, extra ?? null), list);
    list.hidden = !open[id];
    toggle.addEventListener("click", () => {
      open[id] = !open[id];
      savePref(OPEN_KEY, open);
      toggle.setAttribute("aria-expanded", String(open[id]));
      toggle.querySelector(".caret")!.textContent = open[id] ? "▾" : "▸";
      sec.dataset.open = String(open[id]);
      list.hidden = !open[id];
    });
    return sec;
  }

  const rows = () => folders();

  function link(target: string, label: string, iconName: string, count: number | null, current: boolean, extra: Partial<Record<string, string>> = {}): HTMLAnchorElement {
    const a = h("a", { href: `#${target}`, class: "work-link", "aria-current": current ? "page" : null, ...extra },
      icon(iconName), h("span", { class: "work-name" }, h("span", { class: "work-label" }, label)), count === null ? null : h("span", { class: "count" }, String(count)));
    a.addEventListener("click", (e) => { e.preventDefault(); go(target); });
    return a;
  }

  function render() {
    const tree = folderTree(rows());
    const here = (s: Scope) => view === "notes" && sameScope(scope, s);
    clear(folderList,
      h("li", {}, link("notes", "All notes", "notes", total, here({ kind: "all" }))),
      tree.map((t) => folderItem(t, here)),
      editing?.kind === "new" && !editing.parent ? h("li", {}, nameBox("", (name) => create(name))) : null,
      tree.length === 0 && editing?.kind !== "new" ? h("li", { class: "work-hint" }, button("New folder", () => startNew(), { class: "link" }), " for notes and sermons together") : null);
    clear(tagList, tags.length === 0 ? h("li", { class: "work-hint" }, "Tags you give notes show here.")
      : tags.map((t) => h("li", {}, link(scopeHash({ kind: "tag", tag: t.tag }), t.tag, "tag", t.count, here({ kind: "tag", tag: t.tag }), { title: `Notes tagged ${t.tag}` }))));
    clear(writingList, WRITING.map((w) => h("li", {}, link(w.view, w.label, w.icon, null, view === w.view))));
    if (view === "bin") bin.setAttribute("aria-current", "page");
    else bin.removeAttribute("aria-current");
    bin.querySelector(".count")!.textContent = binCount ? String(binCount) : "";
    bin.title = binCount ? `${binCount} ${binCount === 1 ? "thing" : "things"} in the bin` : "The bin is empty";
    el.querySelector<HTMLInputElement>(".work-rename")?.focus();
  }

  function folderItem(f: FolderNode, here: (s: Scope) => boolean): HTMLElement {
    const s: Scope = { kind: "folder", id: f.id };
    const hasKids = f.children.length > 0 || (editing?.kind === "new" && editing.parent === f.id);
    const expanded = hasKids && (openFolders.has(f.id) || editing?.kind === "new" && editing.parent === f.id || f.children.some((c) => here({ kind: "folder", id: c.id })));
    const caret = hasKids
      ? h("button", { type: "button", class: "folder-caret", "aria-expanded": String(expanded), "aria-label": `${expanded ? "Fold" : "Unfold"} ${f.name}` }, expanded ? "▾" : "▸")
      : h("span", { class: "folder-caret", "aria-hidden": "true" });
    caret.addEventListener("click", () => {
      if (openFolders.has(f.id)) openFolders.delete(f.id);
      else openFolders.add(f.id);
      savePref(FOLDERS_KEY, [...openFolders]);
      render();
    });
    const row = editing?.kind === "rename" && editing.id === f.id
      ? h("div", { class: "folder-row" }, caret, nameBox(f.name, (name) => rename(f.id, name)))
      : h("div", { class: "folder-row" }, caret, link(scopeHash(s), f.name, "folder", f.total, here(s), { title: f.name }), folderMenuButton(f));
    const li = h("li", { class: "folder-item" }, row);
    if (!editing) {
      const a = row.querySelector<HTMLElement>(".work-link")!;
      a.draggable = true;
      a.addEventListener("dragstart", (e) => {
        e.dataTransfer?.setData(FOLDER_TYPE, f.id);
        e.dataTransfer && (e.dataTransfer.effectAllowed = "move");
      });
      dropTarget(row, (e) => {
        const dragged = draggedFolder(e);
        if (dragged) return canNestUnder(rows(), dragged, f.id) ? f.id : null;
        return carriesItem(e) ? f.id : null;
      });
    }
    if (expanded) {
      li.append(h("ul", { class: "work-tree sub", role: "list" },
        f.children.map((c) => folderItem({ ...c, children: [] }, here)),
        editing?.kind === "new" && editing.parent === f.id ? h("li", {}, nameBox("", (name) => create(name, f.id))) : null));
    }
    return li;
  }

  // dropTarget makes el accept notes, sermons and folders; target says which folder they go to
  // (null when this drop is not allowed).
  function dropTarget(el: HTMLElement, target: (e: DragEvent) => string | null, items = true) {
    el.addEventListener("dragover", (e) => {
      if (!(items && carriesItem(e)) && !e.dataTransfer?.types.includes(FOLDER_TYPE)) return;
      if (target(e) === null) return;
      e.preventDefault();
      e.dataTransfer!.dropEffect = "move";
      el.classList.add("drop-on");
    });
    el.addEventListener("dragleave", (e) => {
      if (!el.contains(e.relatedTarget as Node)) el.classList.remove("drop-on");
    });
    el.addEventListener("drop", async (e) => {
      el.classList.remove("drop-on");
      const to = target(e);
      if (to === null) return;
      e.preventDefault();
      const folder = draggedFolder(e);
      if (folder) {
        await moveFolder(folder, to);
        return;
      }
      const item = droppedItem(e);
      if (item) await moveItem(msg, item, to);
    });
  }

  // HTML5 drag and drop hides the data until the drop, but its types are visible: the folder's id
  // travels in the type list too, so dragover can tell whether nesting is allowed.
  function draggedFolder(e: DragEvent): string | null {
    const dt = e.dataTransfer;
    if (!dt?.types.includes(FOLDER_TYPE)) return null;
    return dt.getData(FOLDER_TYPE) || dragFolder;
  }
  let dragFolder: string | null = null;
  el.addEventListener("dragstart", (e) => {
    dragFolder = (e.target as HTMLElement).closest?.(".folder-row") ? (e.dataTransfer?.getData(FOLDER_TYPE) ?? null) : null;
  });
  el.addEventListener("dragend", () => { dragFolder = null; });

  function folderMenuButton(f: FolderNode): HTMLButtonElement {
    const b = h("button", { type: "button", class: "icon-btn folder-more", title: "Folder actions", "aria-label": `Actions for ${f.name}`, "aria-haspopup": "menu", "aria-expanded": "false" }, icon("more"));
    b.addEventListener("click", (e) => {
      e.stopPropagation();
      const r = rows();
      const top = !f.parent_id;
      const into = r.filter((x) => !x.parent_id && x.id !== f.parent_id && canNestUnder(r, f.id, x.id));
      const body = h("div", { class: "menu-list" },
        button("Rename", () => { editing = { kind: "rename", id: f.id }; render(); }),
        top ? button("New folder inside", () => { editing = { kind: "new", parent: f.id }; openFolders.add(f.id); render(); }) : null,
        f.parent_id ? button("Move to the top", () => void moveFolder(f.id, "")) : null,
        into.length ? h("p", { class: "pop-title" }, "Put inside") : null,
        into.map((x) => button(x.name, () => void moveFolder(f.id, x.id), { class: "move-target depth-1" })),
        h("hr", { class: "menu-sep" }),
        button("Delete folder", () => void remove(f), { class: "danger" }));
      openPopover(b, body, { label: `Actions for ${f.name}`, menu: true });
    });
    return b;
  }

  function nameBox(value: string, save: (name: string) => Promise<void> | void): HTMLElement {
    const input = h("input", { type: "text", class: "work-rename", value, placeholder: "Folder name", "aria-label": value ? "New name" : "Name of the new folder", maxlength: "120" });
    let done = false;
    const finish = async (keep: boolean) => {
      if (done) return;
      done = true;
      const name = input.value.trim();
      if (keep && name && name !== value) await save(name);
      editing = null;
      render();
    };
    input.addEventListener("keydown", (e) => {
      if (e.key === "Enter") { e.preventDefault(); void finish(true); }
      if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); void finish(false); }
    });
    input.addEventListener("blur", () => void finish(true));
    return h("div", { class: "folder-edit" }, icon("folder"), input);
  }

  function startNew(parent?: string) {
    if (!open.folders) sections.folders.querySelector<HTMLButtonElement>(".work-sec-toggle")!.click();
    editing = { kind: "new", parent };
    render();
  }

  async function create(name: string, parent?: string) {
    const f = await msg.attempt(() => run<{ id: string }>("folder.create", { name, ...(parent ? { parent_id: parent } : {}) }));
    if (!f) return;
    await loadFolders();
    workChanged();
    go(scopeHash({ kind: "folder", id: f.id }));
  }

  async function rename(id: string, name: string) {
    if (await msg.attempt(() => run("folder.rename", { id, name })) === undefined) return;
    await loadFolders();
    workChanged();
  }

  async function moveFolder(id: string, parent: string) {
    if (await msg.attempt(() => run("folder.move", { id, ...(parent ? { parent_id: parent } : {}) })) === undefined) return;
    if (parent) openFolders.add(parent);
    savePref(FOLDERS_KEY, [...openFolders]);
    await loadFolders();
    workChanged();
  }

  async function remove(f: FolderNode) {
    const r = await msg.attempt(() => run<{ folders: number; notes: number; documents: number }>("folder.delete", { id: f.id }));
    if (!r) return;
    const what = [r.notes ? `${r.notes} ${r.notes === 1 ? "note" : "notes"}` : "", r.documents ? `${r.documents} ${r.documents === 1 ? "sermon" : "sermons"}` : ""].filter(Boolean).join(" and ");
    clear(msg.el, `"${f.name}"${what ? ` and its ${what}` : ""} went to the bin. `,
      button("Undo", async () => {
        if (await msg.attempt(() => run("bin.restore", { kind: "folder", id: f.id })) === undefined) return;
        msg.ok(`"${f.name}" is back.`);
        await loadFolders();
        workChanged();
      }, { class: "link" }));
    msg.el.className = "msg ok";
    msg.el.setAttribute("role", "status");
    const shown = scope.kind === "folder" ? scope.id : "";
    if (view === "notes" && shown && (shown === f.id || f.children.some((c) => c.id === shown))) go("notes");
    await loadFolders();
    workChanged();
  }

  async function reload() {
    const [t, n, b] = await Promise.all([
      msg.attempt(() => run<TagCount[]>("note.tags")),
      countNotes(),
      msg.attempt(() => run<BinEntry[]>("bin.list")),
      msg.attempt(() => loadFolders()),
    ]);
    if (t) tags = [...t].sort((a, b) => a.tag.localeCompare(b.tag));
    total = n;
    if (b) binCount = b.length;
    render();
  }

  // countNotes counts every note: note.sources splits them by what they are attached to.
  async function countNotes(): Promise<number> {
    const s = await msg.attempt(() => run<{ bible_books: { count: number }[]; library_books: { count: number }[]; free: number }>("note.sources"));
    if (!s) return 0;
    return s.free + s.bible_books.reduce((n, b) => n + b.count, 0) + s.library_books.reduce((n, b) => n + b.count, 0);
  }

  onWorkChanged(() => void reload());

  return {
    el,
    setCurrent(v: string) {
      view = v;
      scope = scopeOf(location.hash);
      render();
    },
    reload,
  };
}
