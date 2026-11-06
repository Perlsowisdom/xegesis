// Notes: the pieces shared by the notes beside the Bible text (below), the note page (notepage.ts)
// and the Notes section (views/notes.ts): where a note is attached in words, deleting with Undo,
// the sidebar's Notes tool and the history drawer that restores earlier versions.

import { Change, Message, Note, run } from "./api";
import { button, clear, h } from "./dom";
import { formatTime, summarise } from "./helpers";
import { icon } from "./icons";
import { NotePage, notePage } from "./notepage";
import { firstLine } from "./workwords";

// Where a note is attached, in words.
export function placeOf(n: Note): string {
  if (n.ref?.display) return n.ref.display;
  if (n.item_id) return n.item_title ? `In ${n.item_title}` : "In a book";
  return "";
}

export const noteName = (n: Note) => n.title || "Untitled";

// deleteNote deletes a note and offers Undo in the message; Recently deleted keeps it afterwards.
export async function deleteNote(msg: Message, n: Note, changed: () => void) {
  const ok = await msg.attempt(() => run("note.delete", { id: n.id }));
  if (ok === undefined) return;
  clear(msg.el, `"${noteName(n)}" went to the bin. `,
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
// The right sidebar's Notes tool: the notes on the passage as small cards, and the note page in its
// compact form. Choosing a card opens it there; "Open in a tab" opens the full page in Study.

export interface NotesOptions {
  // scope returns the reference to list and attach notes to, or null for every note.
  scope: () => { ref: string; display: string } | null;
  openTab(n: Note): void;
}

export function notesPanel(opts: NotesOptions) {
  const msg = new Message();
  const caption = h("p", { class: "side-caption" });
  const list = h("ul", { class: "side-notes", "aria-live": "polite" });
  const newBtn = h("button", { type: "button", class: "ghost-btn text side-new", "aria-keyshortcuts": "Alt+N" }, icon("plus"), h("span", {}, "New note"));
  newBtn.addEventListener("click", () => open());
  const listView = h("div", { class: "side-list" }, h("div", { class: "side-head" }, caption, newBtn), msg.el, list);
  const pageView = h("div", { class: "side-page", hidden: true });
  const el = h("section", { class: "notes-panel", "aria-label": "Notes" }, listView, pageView);
  let page: NotePage | null = null;

  async function refresh() {
    if (page) return;
    const scope = opts.scope();
    caption.textContent = scope ? `On ${scope.display}` : "Every note, newest first";
    const params: Record<string, unknown> = { limit: 200 };
    if (scope) params.ref = scope.ref;
    const got = await msg.attempt(() => run<Note[]>("note.list", params));
    if (!got) return;
    clear(list, got.length === 0
      ? h("li", { class: "side-empty" }, scope ? "No notes on these verses yet." : "No notes yet.")
      : got.map((n) => card(n)));
  }

  function card(n: Note): HTMLElement {
    const place = placeOf(n);
    const line = firstLine(n.body.replace(/<\/?u>/g, ""), 140);
    const b = h("button", { type: "button", class: "side-note" },
      h("span", { class: "side-note-title" }, n.title || h("span", { class: "untitled" }, "Untitled")),
      line ? h("span", { class: "side-note-line" }, line) : null,
      h("span", { class: "side-note-meta" }, [place, ...n.tags.map((t) => `#${t}`), `Changed ${formatTime(n.updated_at)}`].filter(Boolean).join(" · ")));
    b.addEventListener("click", () => open(n.id));
    return h("li", {}, b);
  }

  // open shows a note (or a new one, on ref or a place in a book) in the compact page.
  function open(id?: string, ref?: string, place?: { itemId: string; title: string; locator: Record<string, unknown> }) {
    void close(false);
    const scope = opts.scope();
    const back = h("button", { type: "button", class: "crumb crumb-back", title: "Back to the notes" }, icon("prev"), h("span", {}, "Notes"));
    back.addEventListener("click", () => void close(true));
    page = notePage({
      id, ref: id ? undefined : ref ?? (place ? undefined : scope?.display), place, variant: "compact", home: back,
      onDeleted: () => void close(true),
      more: [["Open in a tab", () => {
        const n = page?.note();
        if (!n) return msg.info("Write something first.");
        void close(true);
        opts.openTab(n);
      }]],
    });
    clear(pageView, page.el);
    listView.hidden = true;
    pageView.hidden = false;
    if (id) page.focus();
  }

  async function close(showList: boolean) {
    const p = page;
    page = null;
    if (p) {
      await p.flush();
      p.destroy();
    }
    if (!showList) return;
    clear(pageView);
    pageView.hidden = true;
    listView.hidden = false;
    await refresh();
  }

  // Alt+N works anywhere while this panel is on screen.
  document.addEventListener("keydown", (e) => {
    if (e.altKey && !e.ctrlKey && e.key.toLowerCase() === "n" && el.offsetParent !== null && !document.querySelector(".work-view:not([hidden])")) {
      e.preventDefault();
      open();
    }
  });

  return {
    el,
    refresh,
    // showList leaves any open note (saving it) and lists the notes on the passage being read.
    showList: () => close(true),
    create: (ref?: string) => open(undefined, ref),
    createOn: (place: { itemId: string; title: string; locator: Record<string, unknown> }) => open(undefined, undefined, place),
  };
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
