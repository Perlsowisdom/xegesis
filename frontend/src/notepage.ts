// A note as a page you write on: where it is filed, a large title, its details as chips (folder,
// verses, tags), a slim toolbar and then the text. The same page is in My work, in a Study tab and,
// smaller, in the right sidebar. A new note is made by its first change, so opening a page and
// leaving it empty makes nothing. Changes save on their own after a pause.

import { Message, Note, Ref, run } from "./api";
import { clear, h } from "./dom";
import {
  actionsMenu, docChip, pageHead, quotePopover, refPopover, saveIndicator, tagAdder, titleBox, verseRefPopover,
} from "./docpage";
import type { RichEditor } from "./editor";
import { icon } from "./icons";
import { deleteNote, historyDrawer, noteName } from "./notes";
import { openExport } from "./printexport";
import { folderName, loadFolders, moveMenu, workChanged } from "./work";
import { editedText } from "./workwords";

const SAVE_AFTER = 900;

export interface NotePageOptions {
  id?: string; // an existing note; none for a new one
  ref?: string; // where a new note is attached
  folderId?: string; // the folder a new note goes in
  tags?: string[]; // a new note's first tags
  place?: { itemId: string; title: string; locator: Record<string, unknown> }; // a new note on a place in a book
  variant?: "page" | "compact";
  // Called when the note is first made and whenever its title changes, for tab titles and lists.
  onSaved?(n: Note): void;
  // Called when the note goes to the bin.
  onDeleted?(): void;
  // More menu entries this place offers, such as "Open in a tab".
  more?: [string, () => void][];
  // The first crumb, e.g. a link back to the list.
  home?: Node;
}

export interface NotePage {
  el: HTMLElement;
  focus(): void;
  flush(): Promise<void>;
  destroy(): void;
  note(): Note | null;
}

export function notePage(o: NotePageOptions): NotePage {
  const compact = o.variant === "compact";
  const msg = new Message();
  const status = saveIndicator();
  const drawer = historyDrawer(msg, () => void reload());
  const crumbs = h("span", { class: "crumb-path" });
  const chips = h("div", { class: "page-chips", role: "group", "aria-label": "Details" });
  const edHost = h("div", { class: "page-text", "aria-busy": "true" });
  const titleHost = h("div", {});
  const el = h("article", { class: `doc-page note-page${compact ? " compact" : ""}`, "aria-label": "Note" },
    pageHead({ crumbs: [crumbs], status: status.el, menu: (b) => menu(b), menuLabel: "Note actions" }),
    msg.el, titleHost, chips, edHost, drawer.el);

  let note: Note | null = null;
  let title = "";
  let tags = [...(o.tags ?? [])];
  let ref: Ref | null = null;
  let folder = o.folderId ?? "";
  let editor: RichEditor | null = null;
  let timer = 0;
  let dirty = false;
  let saving: Promise<void> | null = null;
  let alive = true;
  let adding = false;

  const tbox = titleBox({ value: "", placeholder: "Untitled", label: "Title", compact,
    onInput: (v) => { title = v; changed(); }, onEnter: () => editor?.focus() });
  titleHost.append(tbox.el);

  // With a way back (My work, the sidebar), the crumbs are that link and the folder; elsewhere
  // they say where the note is filed.
  function renderCrumbs() {
    const sep = () => h("span", { class: "crumb-sep", "aria-hidden": "true" }, "›");
    const where = folder ? h("span", { class: "crumb" }, folderName(folder) || "Folder") : null;
    if (o.home) clear(crumbs, o.home, where ? [sep(), where] : null);
    else clear(crumbs, h("span", { class: "crumb" }, "My work"), sep(), where ?? h("span", { class: "crumb" }, "Notes"));
  }

  function renderChips() {
    const items: Node[] = [];
    if (!compact || folder) {
      items.push(docChip({ kind: "folder", label: [icon("folder"), folder ? folderName(folder) || "Folder" : "No folder"],
        title: "Move to a folder", onClick: (b) => moveTo(b) }));
    }
    const book = note ? (note.item_id ? note.item_title || "a book" : "") : !ref && o.place ? o.place.title || "a book" : "";
    if (book) {
      items.push(docChip({ kind: "plain", label: [icon("library"), `In ${book}`],
        title: "Attached to a place in a book", onClick: (b) => editRef(b) }));
    } else if (ref) {
      items.push(docChip({ kind: "verse", label: [h("span", { class: "star", "aria-hidden": "true" }, "✦"), ref.display],
        title: "Change the verses", onClick: (b) => editRef(b) }));
    } else {
      items.push(docChip({ kind: "add", label: "+ verses", title: "Attach the note to verses", onClick: (b) => editRef(b) }));
    }
    for (const t of tags) {
      items.push(docChip({ kind: "tag", label: [h("span", { class: "hash", "aria-hidden": "true" }, "#"), t],
        title: `Tag ${t}`, removeLabel: `Remove tag ${t}`, onRemove: () => { tags = tags.filter((x) => x !== t); renderChips(); changed(); } }));
    }
    const plus = docChip({ kind: "add", label: "+ tag", title: "Add a tag", onClick: () => startTag(plus) });
    items.push(plus);
    if (note && !compact) items.push(h("span", { class: "page-edited" }, editedText(note.updated_at)));
    clear(chips, items);
    if (adding) startTag(plus);
  }

  function startTag(plus: HTMLElement) {
    adding = true;
    tagAdder(plus, () => tags, (t) => {
      tags.push(t);
      changed();
      renderChips();
    });
    const box = chips.querySelector<HTMLInputElement>(".tag-box");
    box?.addEventListener("blur", () => setTimeout(() => { if (!chips.contains(document.activeElement)) adding = false; }, 0));
  }

  function editRef(b: HTMLElement) {
    const inBook = !!note?.item_id;
    refPopover(b, {
      title: inBook ? "Attach to verses instead" : ref ? "Verses" : "Attach to verses",
      value: ref?.display ?? "", action: ref ? "Change" : "Attach",
      onSave: async (r) => {
        ref = r;
        if (note) {
          const got = await msg.attempt(() => run<Note>("note.move", { id: note!.id, ref: r.display }));
          if (got) note = got;
        } else changed();
        renderChips();
      },
      onRemove: ref || inBook ? async () => {
        ref = null;
        if (note) {
          const got = await msg.attempt(() => run<Note>("note.move", { id: note!.id, free: true }));
          if (got) note = got;
        }
        renderChips();
      } : undefined,
      removeText: inBook ? "Detach it from the book" : "Detach it from these verses",
    });
  }

  async function moveTo(b: HTMLElement) {
    await loadFolders();
    if (!note) {
      // A note not made yet is made first, so it can be filed.
      dirty = true;
      await save();
      if (!note) return;
    }
    moveMenu(b, msg, { kind: "note", id: note.id, title: noteName(note), folder_id: note.folder_id });
  }

  function menu(b: HTMLElement) {
    const n = note;
    actionsMenu(b, "Note actions", [
      ...(o.more ?? []).map(([l, fn]): [string, () => void] => [l, fn]),
      ["Move to folder…", () => void moveTo(b)],
      ["Print or export…", () => { if (n) openExport({ title: noteName(n), sources: [{ kind: "note", id: n.id }] }); else msg.info("Write something first."); }],
      ["History", () => { if (n) void drawer.open(n); else msg.info("A new note has no history yet."); }],
      "-",
      ["Delete", () => void remove(), "danger"],
    ]);
  }

  async function remove() {
    if (!note) {
      o.onDeleted?.();
      return;
    }
    clearTimeout(timer);
    dirty = false;
    await deleteNote(msg, note, () => workChanged());
    o.onDeleted?.();
  }

  function changed() {
    dirty = true;
    status.set("dirty");
    clearTimeout(timer);
    timer = window.setTimeout(() => void save(), SAVE_AFTER);
  }

  async function save(): Promise<void> {
    clearTimeout(timer);
    if (saving) await saving;
    if (!dirty) return;
    const body = editor?.getMarkdown() ?? note?.body ?? "";
    // An untouched new note is not made.
    if (!note && !title && !body.trim() && tags.length === (o.tags?.length ?? 0) && (!ref || !!o.ref)) {
      dirty = false;
      return;
    }
    dirty = false;
    status.set("saving");
    saving = (async () => {
      const before = note ? noteName(note) : "";
      const got = await msg.attempt(() => note
        ? run<Note>("note.update", { id: note.id, title, body, tags })
        : run<Note>("note.create", { title, body, tags, ...(ref ? { ref: ref.display } : {}), ...(folder ? { folder_id: folder } : {}),
          ...(o.place && !ref ? { item_id: o.place.itemId, locator: o.place.locator } : {}) }));
      if (!got) {
        dirty = true;
        status.set("error");
        return;
      }
      const made = !note;
      note = got;
      status.set("saved", got.updated_at);
      if (made || noteName(got) !== before) {
        o.onSaved?.(got);
        workChanged();
      }
      if (made) {
        renderChips();
        editor?.setFoldKey(got.id);
      }
    })();
    await saving;
    saving = null;
  }

  async function reload() {
    if (!note) return;
    const n = await msg.attempt(() => run<Note>("note.get", { id: note!.id }));
    if (!n || !alive) return;
    fill(n);
    editor?.setMarkdown(n.body);
  }

  function fill(n: Note) {
    note = n;
    title = n.title;
    tags = [...n.tags];
    ref = n.ref ?? null;
    folder = n.folder_id ?? "";
    tbox.el.textContent = n.title;
    tbox.el.toggleAttribute("data-empty", !n.title);
    status.set("saved", n.updated_at);
    renderCrumbs();
    renderChips();
  }

  async function start() {
    await loadFolders().catch(() => []);
    if (o.id) {
      const n = await msg.attempt(() => run<Note>("note.get", { id: o.id! }));
      if (!n || !alive) return;
      fill(n);
    } else {
      status.set("new");
      if (o.ref) ref = await run<Ref>("verse.parse", { ref: o.ref }).catch(() => null);
      renderCrumbs();
      renderChips();
    }
    const { createEditor } = await import("./editor");
    if (!alive) return;
    edHost.removeAttribute("aria-busy");
    editor = createEditor(edHost, {
      markdown: note?.body ?? "", placeholder: compact ? "Write your note…" : "Write your note… Type / for headings, lists and verses.",
      label: "Text of the note", variant: compact ? "compact" : "page", onChange: changed, foldKey: note?.id ?? "",
      onVerseRef: verseRefPopover,
      onVerseQuote: (anchor) => quotePopover(anchor, ref?.display ?? "", (md) => editor?.insertMarkdown(md)),
    });
    if (!o.id) tbox.focus();
  }

  // Leaving the page or the window saves first.
  const onUnload = () => void save();
  window.addEventListener("beforeunload", onUnload);
  const onKey = (e: KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s" && el.contains(document.activeElement)) {
      e.preventDefault();
      dirty = true;
      void save();
    }
  };
  el.addEventListener("keydown", onKey);
  renderCrumbs();
  void start();

  return {
    el,
    note: () => note,
    focus: () => (note?.title || !tbox.value() ? (note ? editor?.focus() : tbox.focus()) : editor?.focus()),
    flush: save,
    destroy() {
      void save().finally(() => {
        editor?.destroy();
        editor = null;
      });
      alive = false;
      window.removeEventListener("beforeunload", onUnload);
    },
  };
}
