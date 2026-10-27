// Notes: a list with an editor and a change-history drawer. Used beside the Bible text (notes on
// the current passage) and as the Notes section (every note).

import { Change, Message, Note, run } from "./api";
import { button, clear, field, h } from "./dom";
import { formatTime, summarise } from "./helpers";

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
  let editing: Note | "new" | null = null;

  const el = h("section", { class: "notes-panel", "aria-label": opts.heading },
    h("div", { class: "row between" },
      h(opts.headingLevel ?? "h3", {}, opts.heading),
      button("New note", () => edit("new"), { class: "primary", "aria-keyshortcuts": "Alt+N" })),
    caption, msg.el, editor, list, drawer.el);

  async function refresh() {
    const scope = opts.scope();
    caption.textContent = scope ? `Notes on verses in ${scope.display}.` : "Every note, most recent first.";
    const params: Record<string, unknown> = { limit: 200 };
    if (scope) params.ref = scope.ref;
    const got = await msg.attempt(() => run<Note[]>("note.list", params));
    if (!got) return;
    notes = got;
    render();
  }

  function render() {
    clear(list, notes.length === 0 ? h("li", { class: "empty" }, "No notes yet.") : notes.map((n) =>
      h("li", { class: "note" },
        h("div", { class: "row between" },
          h("strong", {}, n.title || "Untitled"),
          n.ref?.display ? h("span", { class: "badge" }, n.ref.display) : null),
        h("p", { class: "note-body" }, n.body),
        h("div", { class: "row small" },
          h("span", { class: "hint" }, `Changed ${formatTime(n.updated_at)}`),
          button("Edit", () => edit(n), { "aria-label": `Edit ${n.title || "untitled note"}` }),
          button("History", () => drawer.open(n), { "aria-label": `History of ${n.title || "untitled note"}` }),
          button("Delete", () => remove(n), { class: "danger", "aria-label": `Delete ${n.title || "untitled note"}` })))));
  }

  function edit(n: Note | "new") {
    editing = n;
    const scope = opts.scope();
    const title = h("input", { type: "text", value: n === "new" ? "" : n.title });
    const body = h("textarea", { rows: 6 }, n === "new" ? "" : n.body);
    const ref = h("input", { type: "text", value: n === "new" ? scope?.display ?? "" : n.ref?.display ?? "",
      disabled: n !== "new", placeholder: "e.g. Rom 8:28" });
    const form = h("form", { class: "card", "aria-label": n === "new" ? "New note" : "Edit note" },
      field("Title", title),
      field("Bible reference", ref, n === "new" ? "Optional. The note appears with these verses." : "The reference cannot be changed yet."),
      field("Text", body),
      h("div", { class: "row" },
        h("button", { type: "submit", class: "primary" }, n === "new" ? "Create note" : "Save"),
        button("Cancel", close)));
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      const ok = n === "new"
        ? await msg.attempt(() => run<Note>("note.create", { title: title.value, body: body.value, ...(ref.value.trim() ? { ref: ref.value.trim() } : {}) }))
        : await msg.attempt(() => run<Note>("note.update", { id: n.id, title: title.value, body: body.value }));
      if (ok) {
        close();
        msg.ok(n === "new" ? "Note created." : "Note saved.");
        refresh();
      }
    });
    form.addEventListener("keydown", (e) => {
      if (e.key === "Escape") close();
      if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) form.requestSubmit();
    });
    clear(editor, form);
    title.focus();
  }

  function close() {
    editing = null;
    clear(editor);
  }

  async function remove(n: Note) {
    if (!confirm(`Delete "${n.title || "Untitled"}"? You can undo this from its history.`)) return;
    const ok = await msg.attempt(() => run("note.delete", { id: n.id }));
    if (ok === undefined) return;
    if (editing !== "new" && editing?.id === n.id) close();
    clear(msg.el, `Deleted "${n.title || "Untitled"}". `, button("Undo", () => undelete(n)));
    msg.el.className = "msg ok";
    msg.el.setAttribute("role", "status");
    refresh();
  }

  async function undelete(n: Note) {
    const history = await msg.attempt(() => run<Change[]>("history.list", { object_type: "notes", object_id: n.id }));
    if (!history) return;
    const del = history.map((c) => c.op).lastIndexOf("delete");
    const before = del > 0 ? history[del - 1] : null;
    if (!before) return msg.info("Nothing to restore.");
    const ok = await msg.attempt(() => run("history.restore", { object_type: "notes", object_id: n.id, change_id: before.id }));
    if (ok !== undefined) {
      msg.ok("Note restored.");
      refresh();
    }
  }

  // Alt+N works anywhere while this panel is on screen.
  document.addEventListener("keydown", (e) => {
    if (e.altKey && !e.ctrlKey && e.key.toLowerCase() === "n" && el.offsetParent !== null) {
      e.preventDefault();
      edit("new");
    }
  });

  return { el, refresh, create: () => edit("new") };
}

// historyDrawer lists a note's recorded changes; each can be restored.
function historyDrawer(msg: Message, changed: () => void) {
  const body = h("div", {});
  const title = h("h3", {}, "History");
  const el = h("dialog", { class: "drawer" },
    h("div", { class: "row between" }, title, button("Close", () => el.close(), { "aria-label": "Close history" })),
    body);
  title.id = "history-" + Math.random().toString(36).slice(2);
  el.setAttribute("aria-labelledby", title.id);

  async function open(n: Note) {
    title.textContent = `History: ${n.title || "Untitled"}`;
    clear(body, h("p", { class: "hint" }, "Loading…"));
    el.showModal();
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
    clear(body, inner.el, h("p", { class: "hint" }, "Oldest first. Restoring returns the note to how it was just after that change."),
      h("ol", { class: "history" }, groups.map((g, i) => {
        const c = g[0];
        const last = g[g.length - 1];
        return h("li", {},
          h("div", { class: "row between" },
            h("strong", {}, describeChange(c)),
            h("span", { class: "hint" }, formatTime(c.created_at))),
          c.op === "update" ? h("p", { class: "small" }, h("del", {}, summarise(c.before)), " → ", h("ins", {}, summarise(c.after))) : null,
          i < groups.length - 1 && c.op !== "delete"
            ? button("Restore to here", async () => {
              const ok = await inner.attempt(() => run("history.restore", { object_type: "notes", object_id: n.id, change_id: last.id }));
              if (ok !== undefined) {
                el.close();
                msg.ok("Note restored.");
                changed();
              }
            })
            : null);
      })));
  }
  return { el, open };
}

const BOOKKEEPING = new Set(["updated_at", "created_at"]);

function describeChange(c: Change): string {
  if (c.op === "create") return "Created";
  if (c.op === "delete") return "Deleted";
  if (c.op === "update") return `Changed ${c.field || "note"}`;
  return c.op;
}
