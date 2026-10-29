// Notes: the pieces shared by the notes beside the Bible text (below) and the Notes section
// (views/notes.ts): the note form with its formatted-text editor and tags, a note's card, and the
// history drawer that restores earlier versions.

import { Change, Message, Note, TagCount, run } from "./api";
import { button, clear, field, h } from "./dom";
import { renderMarkdown } from "./editor/render";
import { formatTime, summarise } from "./helpers";

// Where a note is attached, in words.
export function placeOf(n: Note): string {
  if (n.ref?.display) return n.ref.display;
  if (n.item_id) return n.item_title ? `In ${n.item_title}` : "In a book";
  return "";
}

export const noteName = (n: Note) => n.title || "Untitled";

// Tags ----------------------------------------------------------------------------------------------

// tagInput is a row of tags with a box to add more; tags already in use are suggested as you type.
export function tagInput(initial: string[]) {
  let tags = [...initial];
  let known: TagCount[] = [];
  const chips = h("span", { class: "chips" });
  const input = h("input", { type: "text", placeholder: "Add a tag, then press Enter", autocomplete: "off", "aria-label": "Add a tag" });
  const sug = h("div", { class: "suggest", "aria-label": "Suggested tags" });
  const el = h("div", { class: "tag-input" }, chips, input, sug);

  function render() {
    clear(chips, tags.map((t) => h("span", { class: "chip" }, t,
      button("×", () => remove(t), { "aria-label": `Remove tag ${t}`, class: "chip-x" }))));
    const q = input.value.trim().toLowerCase();
    const options = known.filter((k) => !tags.includes(k.tag) && k.tag.includes(q)).slice(0, 8);
    clear(sug, options.map((k) => button(`${k.tag} (${k.count})`, () => add(k.tag), { class: "link", "aria-label": `Add tag ${k.tag}` })));
  }
  function add(raw: string) {
    for (const t of raw.split(",")) {
      const tag = t.trim().replace(/\s+/g, " ").toLowerCase();
      if (tag && !tags.includes(tag)) tags.push(tag);
    }
    input.value = "";
    render();
  }
  function remove(t: string) {
    tags = tags.filter((x) => x !== t);
    render();
    input.focus();
  }
  input.addEventListener("input", () => {
    if (input.value.includes(",")) add(input.value);
    else render();
  });
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && input.value.trim()) {
      e.preventDefault();
      e.stopPropagation();
      add(input.value);
    } else if (e.key === "Backspace" && !input.value && tags.length) {
      tags.pop();
      render();
    }
  });
  run<TagCount[]>("note.tags").then((t) => {
    known = t;
    render();
  }).catch(() => {});
  render();
  return {
    el,
    // get includes a tag still being typed.
    get: () => {
      if (input.value.trim()) add(input.value);
      return [...tags];
    },
  };
}

// The note form -----------------------------------------------------------------------------------

export interface NoteFormOptions {
  note: Note | null; // null for a new note
  ref?: string; // the passage a new note starts on
  onDone(saved: Note | null): void;
}

export function noteForm(msg: Message, o: NoteFormOptions): HTMLFormElement {
  const n = o.note;
  const title = h("input", { type: "text", value: n?.title ?? "" });
  const original = n ? (n.ref?.display ?? "") : o.ref ?? "";
  const ref = h("input", { type: "text", value: original, placeholder: "e.g. Rom 8:28" });
  const attachedToBook = !!n?.item_id;
  const detach = h("input", { type: "checkbox", id: "detach-" + Math.random().toString(36).slice(2) });
  const tags = tagInput(n?.tags ?? []);
  const host = h("div", { class: "editor-host", "aria-busy": "true" }, h("p", { class: "hint" }, "Loading the editor…"));
  const save = h("button", { type: "submit", class: "primary", disabled: true }, n ? "Save" : "Create note");
  let editor: { getMarkdown(): string; focus(): void; destroy(): void } | null = null;
  let closed = false;

  import("./editor").then(({ createEditor }) => {
    if (closed) return;
    clear(host);
    host.removeAttribute("aria-busy");
    editor = createEditor(host, { markdown: n?.body ?? "", placeholder: "Write your note…", label: "Text of the note" });
    save.disabled = false;
  });

  const where = attachedToBook
    ? [field("Attached to", ref, `Now attached to a place in ${n!.item_title || "a book"}. Type a Bible reference to move the note to verses, or tick the box below.`),
      h("label", { class: "check" }, detach, " Not attached to anything")]
    : [field("Bible reference", ref, n ? "Clear this to leave the note free of any passage." : "Optional. The note shows with these verses.")];

  const form = h("form", { class: "card note-form", "aria-label": n ? "Edit note" : "New note" },
    field("Title", title),
    ...where,
    h("div", { class: "field" }, h("label", {}, "Text"), host),
    h("div", { class: "field" }, h("label", {}, "Tags"), tags.el),
    h("div", { class: "row" }, save, button("Cancel", () => finish(null))));

  function finish(saved: Note | null) {
    closed = true;
    editor?.destroy();
    editor = null;
    o.onDone(saved);
  }

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!editor) return;
    const body = editor.getMarkdown();
    const want = ref.value.trim();
    const saved = await msg.attempt(async () => {
      if (!n) {
        return run<Note>("note.create", { title: title.value, body, tags: tags.get(), ...(want ? { ref: want } : {}) });
      }
      let note = await run<Note>("note.update", { id: n.id, title: title.value, body, tags: tags.get() });
      const moves = attachedToBook ? detach.checked || !!want : want !== original;
      if (moves) {
        const free = !want || (attachedToBook && detach.checked);
        note = await run<Note>("note.move", free ? { id: n.id, free: true } : { id: n.id, ref: want });
      }
      return note;
    });
    if (saved) finish(saved);
  });
  form.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !(e.target as HTMLElement).closest(".rich-link")) finish(null);
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) form.requestSubmit();
  });
  queueMicrotask(() => title.focus());
  return form;
}

// A note's card -------------------------------------------------------------------------------------

export interface CardActions {
  edit(n: Note): void;
  history(n: Note): void;
  remove(n: Note): void;
}

export function noteCard(n: Note, a: CardActions): HTMLElement {
  const place = placeOf(n);
  return h("li", { class: "note" },
    h("div", { class: "row between" },
      h("strong", {}, noteName(n)),
      place ? h("span", { class: "badge" }, place) : null),
    renderMarkdown(n.body),
    n.tags.length ? h("p", { class: "chips" }, n.tags.map((t) => h("span", { class: "chip" }, t))) : null,
    h("div", { class: "row small" },
      h("span", { class: "hint" }, `Changed ${formatTime(n.updated_at)}`),
      button("Edit", () => a.edit(n), { "aria-label": `Edit ${noteName(n)}` }),
      button("History", () => a.history(n), { "aria-label": `History of ${noteName(n)}` }),
      button("Delete", () => a.remove(n), { class: "danger", "aria-label": `Delete ${noteName(n)}` })));
}

// deleteNote deletes a note and offers Undo in the message; Recently deleted keeps it afterwards.
export async function deleteNote(msg: Message, n: Note, changed: () => void) {
  const ok = await msg.attempt(() => run("note.delete", { id: n.id }));
  if (ok === undefined) return;
  clear(msg.el, `Deleted "${noteName(n)}". It stays under Recently deleted. `,
    button("Undo", async () => {
      if ((await msg.attempt(() => run("note.restore", { id: n.id }))) !== undefined) {
        msg.ok("Note restored.");
        changed();
      }
    }));
  msg.el.className = "msg ok";
  msg.el.setAttribute("role", "status");
  changed();
}

// The notes beside the Bible text -------------------------------------------------------------------

export interface NotesOptions {
  heading: string;
  // scope returns the reference to list and attach notes to, or null for every note.
  scope: () => { ref: string; display: string } | null;
  headingLevel?: "h2" | "h3";
}

export function notesPanel(opts: NotesOptions) {
  const msg = new Message();
  const list = h("ul", { class: "notes", "aria-live": "polite" });
  const editor = h("div", { class: "note-editor" });
  const caption = h("p", { class: "hint" });
  const drawer = historyDrawer(msg, () => refresh());
  let notes: Note[] = [];

  const el = h("section", { class: "notes-panel", "aria-label": opts.heading },
    h("div", { class: "row between" },
      h(opts.headingLevel ?? "h3", {}, opts.heading),
      button("New note", () => edit(null), { class: "primary", "aria-keyshortcuts": "Alt+N" })),
    caption, msg.el, editor, list, drawer.el);

  async function refresh() {
    const scope = opts.scope();
    caption.textContent = scope ? `Notes on verses in ${scope.display}.` : "Every note, most recent first.";
    const params: Record<string, unknown> = { limit: 200 };
    if (scope) params.ref = scope.ref;
    const got = await msg.attempt(() => run<Note[]>("note.list", params));
    if (!got) return;
    notes = got;
    clear(list, notes.length === 0 ? h("li", { class: "empty" }, "No notes yet.") : notes.map((n) => noteCard(n, actions)));
  }

  const actions: CardActions = {
    edit: (n) => edit(n),
    history: (n) => drawer.open(n),
    remove: (n) => deleteNote(msg, n, refresh),
  };

  function edit(n: Note | null, ref?: string) {
    const scope = opts.scope();
    clear(editor, noteForm(msg, {
      note: n,
      ref: ref ?? scope?.display,
      onDone: (saved) => {
        clear(editor);
        if (saved) {
          msg.ok(n ? "Note saved." : "Note created.");
          refresh();
        }
      },
    }));
  }

  // Alt+N works anywhere while this panel is on screen.
  document.addEventListener("keydown", (e) => {
    if (e.altKey && !e.ctrlKey && e.key.toLowerCase() === "n" && el.offsetParent !== null) {
      e.preventDefault();
      edit(null);
    }
  });

  return { el, refresh, create: (ref?: string) => edit(null, ref) };
}

// History -------------------------------------------------------------------------------------------

const BOOKKEEPING = new Set(["updated_at", "created_at"]);

// historyDrawer lists a note's earlier versions. Choosing one restores it; the restore is recorded
// like any other change, so "Undo" (or the history itself) brings the later text back.
export function historyDrawer(msg: Message, changed: () => void) {
  const body = h("div", {});
  const title = h("h3", {}, "History");
  const el = h("dialog", { class: "drawer" },
    h("div", { class: "row between" }, title, button("Close", () => el.close(), { "aria-label": "Close history" })),
    body);
  title.id = "history-" + Math.random().toString(36).slice(2);
  el.setAttribute("aria-labelledby", title.id);

  async function open(n: Note) {
    title.textContent = `History: ${noteName(n)}`;
    clear(body, h("p", { class: "hint" }, "Loading…"));
    if (!el.open) el.showModal();
    const inner = new Message();
    const changes = await inner.attempt(() => run<Change[]>("history.list", { object_type: "notes", object_id: n.id }));
    if (!changes) return clear(body, inner.el);
    // One command can record several changes (a field and its timestamp); show each group once,
    // and restore to its last change.
    const groups: Change[][] = [];
    for (const c of changes) {
      if (BOOKKEEPING.has(c.field) && groups.length) groups[groups.length - 1].push(c);
      else groups.push([c]);
    }
    const latest = changes[changes.length - 1];
    clear(body, inner.el, h("p", { class: "hint" }, "Oldest first. Restoring returns the note to how it was just after that change, and can itself be undone."),
      h("ol", { class: "history" }, groups.map((g, i) => {
        const c = g[0];
        const last = g[g.length - 1];
        return h("li", {},
          h("div", { class: "row between" },
            h("strong", {}, describeChange(g)),
            h("span", { class: "hint" }, formatTime(c.created_at))),
          c.op === "update" && c.field !== "anchor_id" ? h("p", { class: "small" }, h("del", {}, summarise(c.before)), " → ", h("ins", {}, summarise(c.after))) : null,
          i < groups.length - 1 && c.op !== "delete"
            ? button("Restore this version", async () => {
              const ok = await inner.attempt(() => run("history.restore", { object_type: "notes", object_id: n.id, change_id: last.id }));
              if (ok === undefined) return;
              el.close();
              clear(msg.el, "Note restored to an earlier version. ", button("Undo", async () => {
                const back = await msg.attempt(() => run("history.restore", { object_type: "notes", object_id: n.id, change_id: latest.id }));
                if (back !== undefined) {
                  msg.ok("Restore undone.");
                  changed();
                }
              }));
              msg.el.className = "msg ok";
              msg.el.setAttribute("role", "status");
              changed();
            }, { "aria-label": `Restore the version from ${formatTime(c.created_at)}` })
            : null);
      })));
  }
  return { el, open };
}

function describeChange(g: Change[]): string {
  const c = g[0];
  if (c.op === "create") return "Created";
  if (c.op === "delete") return "Deleted";
  if (c.op === "update") {
    const f = g.map((x) => x.field).filter((x) => !BOOKKEEPING.has(x));
    const names = f.map((x) => (x === "body_md" ? "text" : x === "anchor_id" ? "place" : x === "deleted_at" ? "restored" : x));
    return `Changed ${names.join(" and ") || "note"}`;
  }
  return c.op;
}
