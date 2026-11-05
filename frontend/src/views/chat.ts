// The AI chat tool of the right sidebar. Questions go to the provider chosen in Settings → Assistant
// → Model with what is open now; the reply streams in, each tool call shows as a line that opens to
// exactly what was sent, references become verse chips and drafts become cards the person inserts.
// Before the first message to a provider, a panel shows what is sent and where (ADR 0009, 0011).

import { Message, RunError, onEvent, run } from "../api";
import { activeDoc } from "../activedoc";
import { AssistantConfig, AssistantEvent, FoundRef, ToolLine, draftParts, linkRefs, runningLabel, segments } from "../assistantwords";
import { button, clear, h } from "../dom";
import { renderMarkdown } from "../editor/render";
import { navigate, state } from "../state";

interface Turn {
  el: HTMLElement; // the reply's container
  body: HTMLElement;
  text: string;
  frame: number;
}

let listeners: ((e: AssistantEvent) => void)[] = [];
let listening = false;
function listen(fn: (e: AssistantEvent) => void) {
  listeners.push(fn);
  if (listening) return;
  listening = true;
  onEvent("assistant:event", (e) => listeners.forEach((l) => l(e as AssistantEvent)));
}

export function chatPanel() {
  const msg = new Message();
  const el = h("div", { class: "chat" });
  let cfg: AssistantConfig | null = null;
  let conv = "";
  let busy = false;
  let confirming = false; // the privacy panel is waiting for an answer
  let turn: Turn | null = null;
  const tools = new Map<string, HTMLDetailsElement>();

  const log = h("div", { class: "chat-log", "aria-live": "polite", "aria-label": "Conversation" });
  const input = h("textarea", { class: "chat-input", rows: 2, placeholder: "Ask about the passage, your notes or your books…", "aria-label": "Your question" });
  const send = h("button", { type: "button", class: "primary chat-send" }, "Send");
  const where = h("p", { class: "hint chat-context" });
  const saveBtn = button("Save as note", () => void saveChat(), { class: "ghost-btn text", disabled: true });
  const head = h("div", { class: "chat-head" },
    h("span", { class: "hint chat-provider" }),
    h("span", { class: "chat-head-actions" }, saveBtn, button("New chat", () => reset(), { class: "ghost-btn text" })));
  const composer = h("div", { class: "chat-compose" }, where, h("div", { class: "row chat-row" }, input, send));
  const on = h("div", { class: "chat-on" }, head, log, msg.el, composer);

  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
      e.preventDefault();
      if (!busy) void ask();
    }
  });
  input.addEventListener("focus", () => showContext());
  send.addEventListener("click", () => (busy ? void stop() : void ask()));

  listen((e) => {
    if (!busy) return;
    if (!conv && e.type === "start") conv = e.conversation_id;
    if (e.conversation_id !== conv || !turn) return;
    switch (e.type) {
      case "text":
        turn.text += e.text ?? "";
        draw(turn, false);
        break;
      case "tool":
        if (e.tool) toolLine(e.tool, true);
        break;
      case "tool_done":
        if (e.tool) toolLine(e.tool, false);
        break;
    }
  });

  function off() {
    clear(el, h("div", { class: "chat-off" },
      h("h3", {}, "The assistant is off"),
      h("p", {}, "Xegesis can work with an AI assistant when you choose one. There are three ways to turn it on:"),
      h("ul", {},
        h("li", {}, h("strong", {}, "Your own key"), " for an online service such as Claude or OpenAI."),
        h("li", {}, h("strong", {}, "Claude Code or Codex"), " already set up on this computer."),
        h("li", {}, h("strong", {}, "A local model"), " that runs on this computer, so nothing leaves it.")),
      h("p", { class: "hint" }, "Nothing is sent anywhere until you choose, and each online option says what it sends first."),
      button("Choose in Settings…", () => navigate("settings/model"), { class: "primary" })));
  }

  async function refresh() {
    cfg = await run<AssistantConfig>("assistant.config.get").catch(() => null);
    if (!cfg || cfg.provider === "off") return off();
    if (!el.contains(on)) clear(el, on);
    const p = head.querySelector<HTMLElement>(".chat-provider")!;
    p.textContent = `${cfg.label}${cfg.model ? ` · ${cfg.model}` : ""}${cfg.local ? " · on this computer" : ""}`;
    showContext();
  }

  function context() {
    const doc = activeDoc();
    const c: Record<string, string> = {};
    if (state.parsed) c.ref = state.parsed.display;
    if (state.modules[0]) c.bible = state.modules[0];
    if (doc) {
      c.note_id = doc.id() ?? "";
      c.note_kind = doc.kind;
      c.note_title = doc.title();
    }
    return c;
  }

  function showContext() {
    const c = context();
    const parts = [c.ref, c.note_title && `${c.note_kind === "sermon" ? "sermon" : "note"} “${c.note_title}”`].filter(Boolean);
    where.textContent = parts.length ? `Sent with: ${parts.join(" · ")}` : "";
  }

  function bubble(text: string) {
    log.append(h("div", { class: "chat-q" }, text));
  }

  function newTurn(): Turn {
    const body = h("div", { class: "chat-a-body" });
    const t: Turn = { el: h("div", { class: "chat-a", "data-state": "writing" }, body), body, text: "", frame: 0 };
    log.append(t.el);
    return t;
  }

  // draw shows the reply so far; at the end, references become verse chips.
  function draw(t: Turn, final: boolean, refs?: FoundRef[][]) {
    if (!final) {
      if (t.frame) return;
      t.frame = requestAnimationFrame(() => {
        t.frame = 0;
        render(t, false);
      });
      return;
    }
    cancelAnimationFrame(t.frame);
    t.frame = 0;
    render(t, true, refs);
  }

  function render(t: Turn, final: boolean, refs?: FoundRef[][]) {
    let i = 0;
    clear(t.body, segments(t.text).map((s) => {
      if (s.kind === "draft") return draftCard(s.text, !!s.open && !final);
      const md = refs ? linkRefs(s.text, refs[i++] ?? []) : s.text;
      const out = renderMarkdown(md);
      for (const a of out.querySelectorAll<HTMLAnchorElement>("a.md-link[href^='#passage/']")) {
        a.className = "ref-chip chip-verse";
        a.title = `Open ${a.textContent}`;
        a.prepend(h("span", { class: "ref-chip-mark", "aria-hidden": "true" }, "✦"));
      }
      return out;
    }));
    scroll();
  }

  // scroll keeps the newest part in view; the sidebar's panel is what scrolls.
  function scroll() {
    const box = el.closest<HTMLElement>(".tool-panels") ?? log;
    box.scrollTop = box.scrollHeight;
  }

  function toolLine(t: ToolLine, running: boolean) {
    let d = tools.get(t.id);
    if (!d) {
      d = h("details", { class: "tool-line" }) as HTMLDetailsElement;
      tools.set(t.id, d);
      turn!.el.insertBefore(d, turn!.body);
    }
    d.dataset.state = running ? "running" : "done";
    clear(d,
      h("summary", {}, running ? runningLabel(t.name) : t.label),
      running ? null : h("div", { class: "tool-sent" }, h("p", { class: "hint" }, "Sent to the model:"), h("pre", {}, t.content ?? "")));
    scroll();
  }

  function draftCard(md: string, open: boolean): HTMLElement {
    const note = new Message();
    const into = button("Insert into note", () => {
      const doc = activeDoc();
      if (!doc) return note.info("Open a note or sermon first, then insert.");
      doc.insert(md);
      note.ok(`Inserted into “${doc.title()}”.`);
    }, { class: "primary" });
    const fresh = button("New note", async () => {
      const { title, body } = draftParts(md);
      const n = await note.attempt(() => run<{ id: string }>("note.create", { title, body }));
      if (n) {
        note.ok(`Saved as the note “${title}”.`);
        location.hash = `#notes/note/${n.id}`;
      }
    });
    const copy = button("Copy", async () => {
      try {
        await navigator.clipboard.writeText(md);
        note.ok("Copied.");
      } catch {
        note.info("Could not copy here; select the text instead.");
      }
    });
    return h("section", { class: "draft-card", "aria-label": "Draft" },
      h("p", { class: "draft-label" }, open ? "Draft (being written)…" : "Draft"),
      renderMarkdown(md),
      open ? null : h("div", { class: "row wrap draft-actions" }, into, fresh, copy),
      note.el);
  }

  // privacy shows, before the first message to a provider, what is sent and where.
  async function privacy(question: string): Promise<boolean> {
    const pv = await msg.attempt(() => run<{ provider: string; endpoint: string; model: string; local: boolean; question: string; tools: string[]; body: string }>(
      "assistant.preview", { message: question, context: context() }));
    if (!pv) return false;
    return new Promise((done) => {
      const finish = async (yes: boolean) => {
        panel.remove();
        if (yes && !(await msg.attempt(() => run("assistant.config.set", { confirm: true })))) return done(false);
        done(yes);
      };
      const panel = h("section", { class: "privacy-panel", role: "dialog", "aria-label": `What is sent to ${pv.provider}` },
        h("h3", {}, `Before your first message to ${pv.provider}`),
        h("p", {}, pv.local ? "This model runs on this computer. Nothing leaves it." : "This is sent over the internet. The service's own terms apply to it."),
        h("dl", { class: "info" },
          h("dt", {}, "Sent to"), h("dd", {}, h("code", {}, pv.endpoint)),
          h("dt", {}, "Model"), h("dd", {}, pv.model),
          h("dt", {}, "Your question"), h("dd", {}, h("pre", { class: "privacy-question" }, pv.question)),
          h("dt", {}, "What it reads"), h("dd", {},
            "The assistant may read your library with ", String(pv.tools.length), " read-only tools. Their results are sent too: passage text, your notes, sermons and highlights, and book passages, within each work's quoting limit.",
            pv.local ? null : " Personal copies of books are never sent to an online service.",
            " Each one shows below as a line you can open to see exactly what was sent.")),
        h("details", {}, h("summary", {}, "Show the full request"), h("pre", { class: "privacy-body" }, pv.body)),
        h("p", { class: "hint" }, "Your key is sent only to this address and is not shown here. You are asked once for each service."),
        h("div", { class: "row wrap" },
          button("Send", () => void finish(true), { class: "primary" }),
          button("Cancel", () => void finish(false))));
      log.append(panel);
      scroll();
      panel.querySelector<HTMLButtonElement>("button.primary")?.focus();
    });
  }

  async function ask() {
    const q = input.value.trim();
    if (!q || busy || confirming) return;
    msg.clear();
    await refresh();
    if (!cfg || cfg.provider === "off") return;
    if (!cfg.confirmed) {
      confirming = true;
      const yes = await privacy(q).finally(() => (confirming = false));
      if (!yes) return;
      cfg.confirmed = true;
    }
    input.value = "";
    bubble(q);
    busy = true;
    send.textContent = "Stop";
    send.classList.replace("primary", "danger");
    turn = newTurn();
    const t = turn;
    try {
      const res = await run<{ conversation_id: string; reply: string; stopped?: boolean }>("assistant.chat", { conversation_id: conv || undefined, message: q, context: context() });
      conv = res.conversation_id;
      t.text = res.reply;
      const texts = segments(t.text).filter((s) => s.kind === "text").map((s) => s.text);
      const refs = texts.length ? await run<FoundRef[][]>("book.refs", { texts }).catch(() => undefined) : undefined;
      draw(t, true, refs);
      if (res.stopped) t.el.append(h("p", { class: "hint chat-stopped" }, "Stopped."));
      saveBtn.disabled = false;
    } catch (e) {
      draw(t, true);
      msg.error(e instanceof RunError ? e : e);
    } finally {
      t.el.dataset.state = "done";
      busy = false;
      send.textContent = "Send";
      send.classList.replace("danger", "primary");
      input.focus();
    }
  }

  async function stop() {
    if (conv) await run("assistant.cancel", { conversation_id: conv }).catch(() => {});
  }

  async function saveChat() {
    if (!conv) return;
    const n = await msg.attempt(() => run<{ id: string; title: string }>("assistant.save", { conversation_id: conv }));
    if (n) msg.ok(`Saved as the note “${n.title}”.`);
  }

  function reset() {
    if (busy) return;
    conv = "";
    tools.clear();
    clear(log);
    msg.clear();
    saveBtn.disabled = true;
    input.focus();
  }

  off();
  return { el, refresh };
}
