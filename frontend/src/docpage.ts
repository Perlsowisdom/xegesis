// The pieces of the writing page that notes and sermons share: a top line with where the item is
// filed, whether it is saved and its ⋯ menu; a large title typed straight into; rounded chips for
// its details, each edited in a small popover; and the popovers for verses: a reference to link to,
// verses to quote (through document.quote, so each Bible's licence applies) and a month calendar.

import { activeDoc } from "./activedoc";
import { Module, Ref, run } from "./api";
import { installedBibles } from "./bibles";
import { button, clear, h } from "./dom";
import { SaveStatus, addMonths, isoDate, monthDays, saveText } from "./docwords";
import { formatTime } from "./helpers";
import { icon } from "./icons";
import { closePopover, openPopover } from "./popover";
import { state } from "./state";

type Child = Node | string | null | undefined | false;

// The top line ---------------------------------------------------------------------------------

export function saveIndicator() {
  const dot = h("span", { class: "save-dot", "aria-hidden": "true" });
  const text = h("span", {});
  const el = h("span", { class: "save-state", role: "status", "aria-live": "polite" }, dot, text);
  const set = (s: SaveStatus, at?: number) => {
    el.dataset.state = s;
    text.textContent = saveText(s);
    el.title = s === "saved" && at ? `Saved ${formatTime(at)}` : s === "error" ? "Not saved: see the message above" : "";
  };
  set("saved");
  return { el, set };
}

export interface HeadOptions {
  crumbs: Child[]; // with their own separators
  status: HTMLElement;
  extra?: Child[];
  menu(anchor: HTMLElement): void;
  menuLabel: string;
}

export function pageHead(o: HeadOptions): HTMLElement {
  const more = h("button", { type: "button", class: "ghost-btn page-more", "aria-label": o.menuLabel, title: "More", "aria-haspopup": "menu", "aria-expanded": "false" }, icon("more"));
  more.addEventListener("click", () => o.menu(more));
  const crumbs = o.crumbs.map((c) => (typeof c === "string" ? h("span", { class: "crumb" }, c) : c));
  return h("div", { class: "page-head" },
    h("nav", { class: "crumbs", "aria-label": "Where it is filed" }, crumbs),
    h("span", { class: "spacer" }), o.extra ?? null, o.status, more);
}

// menu draws a plain menu of actions in a popover; "-" is a line between groups.
export function actionsMenu(anchor: HTMLElement, label: string, items: ([string, () => void, string?] | "-")[]): void {
  openPopover(anchor, h("div", { class: "menu-list" }, items.map((it) => it === "-"
    ? h("div", { class: "menu-sep", role: "separator" })
    : button(it[0], it[1], { class: it[2] ?? "" }))), { label, menu: true });
}

// The title -------------------------------------------------------------------------------------

export interface TitleOptions {
  value: string;
  placeholder: string;
  label: string;
  onInput(v: string): void;
  onEnter(): void;
  compact?: boolean;
}

// titleBox is a large title typed straight into: one line, no box. Enter moves on to the text.
export function titleBox(o: TitleOptions) {
  const el = h("div", { class: `page-title${o.compact ? " compact" : ""}`, contenteditable: "plaintext-only", role: "textbox",
    "aria-label": o.label, "data-placeholder": o.placeholder, spellcheck: "true" });
  el.textContent = o.value;
  const sync = () => el.toggleAttribute("data-empty", !el.textContent);
  sync();
  el.addEventListener("input", () => {
    // Pasted line breaks become spaces: a title is one line.
    if (/\n/.test(el.textContent ?? "")) el.textContent = (el.textContent ?? "").replace(/\s*\n\s*/g, " ");
    sync();
    o.onInput(value());
  });
  el.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      o.onEnter();
    }
  });
  const value = () => (el.textContent ?? "").replace(/\s+/g, " ").trim();
  return { el, value, focus: () => {
    el.focus();
    const r = document.createRange();
    r.selectNodeContents(el);
    r.collapse(false);
    const s = getSelection();
    s?.removeAllRanges();
    s?.addRange(r);
  } };
}

// Chips -----------------------------------------------------------------------------------------

export interface ChipOptions {
  label: Child | Child[];
  kind?: "verse" | "tag" | "add" | "folder" | "plain";
  title?: string;
  onClick?(b: HTMLElement): void;
  onRemove?(): void;
  removeLabel?: string;
}

export function docChip(o: ChipOptions): HTMLElement {
  const b = h("button", { type: "button", class: `doc-chip chip-${o.kind ?? "plain"}`, title: o.title }, o.label as Child);
  if (o.onClick) b.addEventListener("click", () => o.onClick!(b));
  if (!o.onRemove) return b;
  const x = h("button", { type: "button", class: "doc-chip-x", "aria-label": o.removeLabel ?? "Remove", title: o.removeLabel ?? "Remove" }, "×");
  x.addEventListener("click", (e) => {
    e.stopPropagation();
    o.onRemove!();
  });
  return h("span", { class: `doc-chip-wrap chip-${o.kind ?? "plain"}` }, b, x);
}

// A reference in a popover ------------------------------------------------------------------------

export interface RefOptions {
  title: string;
  value: string;
  action: string; // the button's words
  onSave(r: Ref): void | Promise<void>;
  onRemove?(): void | Promise<void>;
  removeText?: string;
  extra?: Child[];
}

// refPopover asks for a Bible reference and checks it as it is typed.
export function refPopover(anchor: HTMLElement, o: RefOptions): void {
  const input = h("input", { type: "text", value: o.value, placeholder: "e.g. Rom 8:28-30", autocomplete: "off", spellcheck: "false", "aria-label": "Reference" });
  const note = h("p", { class: "ref-note", "aria-live": "polite" });
  const ok = h("button", { type: "submit", class: "primary" }, o.action);
  let parsed: Ref | null = null;
  let seq = 0;
  const check = async () => {
    const n = ++seq;
    const v = input.value.trim();
    parsed = null;
    if (!v) {
      note.textContent = "";
      note.className = "ref-note";
      return;
    }
    const p = await run<Ref>("verse.parse", { ref: v }).catch(() => null);
    if (n !== seq) return;
    parsed = p;
    note.textContent = p ? p.display : "Not a reference yet";
    note.className = `ref-note ${p ? "ok" : "bad"}`;
  };
  let timer = 0;
  input.addEventListener("input", () => {
    clearTimeout(timer);
    timer = window.setTimeout(() => void check(), 160);
  });
  const form = h("form", { class: "pop-body ref-pop" },
    h("p", { class: "pop-title" }, o.title),
    h("div", { class: "row" }, input, ok), note,
    o.extra ?? null,
    o.onRemove ? button(o.removeText ?? "Remove", async () => { closePopover(true); await o.onRemove!(); }, { class: "link small danger-link" }) : null);
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    clearTimeout(timer);
    await check();
    if (!parsed) {
      note.textContent = input.value.trim() ? "Type a reference such as Rom 8:28." : "Type a reference first.";
      note.className = "ref-note bad";
      input.focus();
      return;
    }
    const p = parsed;
    closePopover(true);
    await o.onSave(p);
  });
  openPopover(anchor, form, { label: o.title, className: "doc-pop" });
  input.select();
  if (o.value) void check();
}

// verseRefPopover is Insert → Verse reference: the reference becomes a chip that opens the passage.
export function verseRefPopover(anchor: HTMLElement, selected: string, insert: (label: string, ref: string) => void): void {
  refPopover(anchor, {
    title: "Insert a verse reference", value: /\d/.test(selected) ? selected : "", action: "Insert",
    onSave: (r) => insert(r.display, r.osis),
  });
}

// Quoting verses ---------------------------------------------------------------------------------

let lastBible = "";

// verseQuoteFor quotes verses through document.quote, so the Bible's licence decides; with words, only those words of them.
export async function verseQuoteFor(ref: string, module: string, words = ""): Promise<{ md: string } | { reason: string }> {
  // The text of the open note or sermon goes too, so quotations from one limited Bible add up in it.
  const text = activeDoc()?.text();
  const r = await run<{ allowed: boolean; reason?: string; quote?: string }>("document.quote", { ref, module, preview: true, ...(words ? { words } : {}), ...(text ? { text } : {}) })
    .catch((err: Error) => ({ allowed: false, reason: err.message, quote: "" }));
  if (!r.allowed || !r.quote) return { reason: r.reason ?? "the licence does not allow it" };
  return { md: r.quote.trimEnd() };
}

// quotePopover finds verses to quote, shows them as they are typed, and gives back the quotation
// with its credit line, if the Bible's licence allows keeping a copy.
export function quotePopover(anchor: HTMLElement, initial: string, onInsert: (md: string) => void): void {
  const input = h("input", { type: "text", value: initial, placeholder: "e.g. Rom 8:28-30", autocomplete: "off", spellcheck: "false", "aria-label": "Verses to quote" });
  const preview = h("div", { class: "quote-preview", "aria-live": "polite" }, h("span", { class: "hint" }, "Type the verses to quote."));
  const bibleBtn = h("button", { type: "button", class: "doc-chip chip-plain bible-chip", "aria-haspopup": "menu", "aria-expanded": "false", title: "Bible to quote from" });
  const ok = h("button", { type: "submit", class: "primary", disabled: true }, "Insert");
  const why = h("p", { class: "ref-note bad", role: "alert" });
  let bibles: Module[] = [];
  let module = "";
  let seq = 0;
  let timer = 0;

  const label = () => bibles.find((m) => m.id === module)?.abbreviation || module || "No Bible";
  const setBible = (id: string) => {
    module = id;
    lastBible = id;
    clear(bibleBtn, label(), icon("chevron"));
    void show();
  };
  bibleBtn.addEventListener("click", () => {
    // A small menu inside the popover, so the popover stays open.
    const list = h("div", { class: "menu-list bible-list", role: "menu" }, bibles.map((m) => button(m.abbreviation || m.id, () => {
      list.remove();
      bibleBtn.setAttribute("aria-expanded", "false");
      setBible(m.id);
      input.focus();
    }, { role: "menuitem", title: m.name, class: m.id === module ? "on" : "" })));
    const open = form.querySelector(".bible-list");
    if (open) {
      open.remove();
      bibleBtn.setAttribute("aria-expanded", "false");
      return;
    }
    bibleBtn.after(list);
    bibleBtn.setAttribute("aria-expanded", "true");
    list.querySelector<HTMLElement>("button")?.focus();
  });

  async function show() {
    const n = ++seq;
    why.textContent = "";
    const ref = input.value.trim();
    if (!ref || !module) {
      ok.disabled = true;
      clear(preview, h("span", { class: "hint" }, module ? "Type the verses to quote." : "Install a Bible first, from Library → Get Bibles."));
      return;
    }
    const p = await run<{ verses: { id: number; text: string }[] }>("bible.read", { module, ref }).catch(() => null);
    if (n !== seq) return;
    if (!p || p.verses.length === 0) {
      ok.disabled = true;
      clear(preview, h("span", { class: "hint" }, p ? "No verses there in this Bible." : "Not a reference yet."));
      return;
    }
    ok.disabled = false;
    const text = p.verses.map((v) => v.text.replace(/\s+/g, " ").trim()).join(" ");
    clear(preview, text.length > 260 ? `${text.slice(0, 260)}…` : text);
  }
  input.addEventListener("input", () => {
    clearTimeout(timer);
    timer = window.setTimeout(() => void show(), 220);
  });

  const form = h("form", { class: "pop-body quote-pop" },
    h("p", { class: "pop-title" }, "Insert verse text"),
    input, preview, why,
    h("div", { class: "row quote-actions" }, bibleBtn, h("span", { class: "spacer" }), ok));
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    clearTimeout(timer);
    const ref = input.value.trim();
    if (!ref || !module) return;
    ok.disabled = true;
    const r = await verseQuoteFor(ref, module);
    ok.disabled = false;
    if ("reason" in r) {
      why.textContent = `Not quoted: ${r.reason}.`;
      return;
    }
    closePopover();
    onInsert(r.md);
  });
  form.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && form.querySelector(".bible-list")) {
      e.stopPropagation();
      form.querySelector(".bible-list")?.remove();
      bibleBtn.setAttribute("aria-expanded", "false");
      bibleBtn.focus();
    }
  });
  openPopover(anchor, form, { label: "Quote verses", className: "doc-pop" });
  input.select();

  void installedBibles().then((mods) => {
    bibles = mods;
    const want = [lastBible, ...state.modules].find((id) => mods.some((m) => m.id === id));
    setBible(want ?? mods[0]?.id ?? "");
  });
}

// A month calendar ------------------------------------------------------------------------------

export interface CalendarOptions {
  value: string; // YYYY-MM-DD or ""
  noneText: string;
  onPick(iso: string): void;
}

export function calendarPopover(anchor: HTMLElement, o: CalendarOptions): void {
  const now = new Date();
  const today = isoDate(now.getFullYear(), now.getMonth(), now.getDate());
  const at = /^(\d{4})-(\d{2})/.exec(o.value);
  let y = at ? Number(at[1]) : now.getFullYear();
  let m = at ? Number(at[2]) - 1 : now.getMonth();
  const title = h("span", { class: "cal-title", "aria-live": "polite" });
  const grid = h("div", { class: "cal-grid", role: "grid" });
  const pick = (iso: string) => {
    closePopover(true);
    o.onPick(iso);
  };
  const draw = () => {
    title.textContent = new Date(y, m, 1).toLocaleDateString("en-GB", { month: "long", year: "numeric" });
    clear(grid, ["S", "M", "T", "W", "T", "F", "S"].map((d, i) => h("span", { class: `cal-dow${i === 0 ? " sun" : ""}`, "aria-hidden": "true" }, d)),
      monthDays(y, m).map((d) => d.day === 0 ? h("span", {}) : button(String(d.day), () => pick(d.iso), {
        class: `cal-day${d.sunday ? " sun" : ""}${d.iso === o.value ? " on" : ""}${d.iso === today ? " today" : ""}`,
        "aria-label": new Date(y, m, d.day).toLocaleDateString("en-GB", { weekday: "long", day: "numeric", month: "long", year: "numeric" }),
        "aria-pressed": String(d.iso === o.value), tabindex: d.iso === (o.value || today) || (d.day === 1 && !grid.querySelector("[tabindex='0']")) ? "0" : "-1",
      })));
  };
  const step = (by: number) => {
    [y, m] = addMonths(y, m, by);
    draw();
  };
  grid.addEventListener("keydown", (e) => {
    const days = [...grid.querySelectorAll<HTMLElement>(".cal-day")];
    const i = days.indexOf(document.activeElement as HTMLElement);
    if (i < 0) return;
    const d = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7 }[e.key];
    if (!d) return;
    e.preventDefault();
    days[Math.min(days.length - 1, Math.max(0, i + d))]?.focus();
  });
  draw();
  const body = h("div", { class: "pop-body cal-pop" },
    h("div", { class: "cal-head" }, title,
      h("span", { class: "cal-nav" },
        button("", () => step(-1), { class: "ghost-btn", "aria-label": "Previous month" }),
        button("", () => step(1), { class: "ghost-btn", "aria-label": "Next month" }))),
    grid,
    h("div", { class: "cal-foot" },
      button(o.noneText, () => pick(""), { class: "cal-link muted" }),
      button("Today", () => pick(today), { class: "cal-link" })));
  const [prevB, nextB] = body.querySelectorAll<HTMLButtonElement>(".cal-nav button");
  prevB.append(icon("prev"));
  nextB.append(icon("next"));
  openPopover(anchor, body, { label: "Choose a date", className: "doc-pop" });
  grid.querySelector<HTMLElement>(".cal-day.on, .cal-day.today")?.focus();
}

// Tags ------------------------------------------------------------------------------------------

// tagAdder turns the "+ tag" chip into a small box typed straight into, with the tags already in
// use suggested underneath. Enter or a comma adds the tag; Escape or leaving the box ends it.
export function tagAdder(chip: HTMLElement, have: () => string[], onAdd: (tag: string) => void): void {
  const input = h("input", { type: "text", class: "tag-box", placeholder: "tag", "aria-label": "New tag", autocomplete: "off", size: "8" });
  const sug = h("div", { class: "tag-suggest", role: "listbox", "aria-label": "Tags in use" });
  const wrap = h("span", { class: "tag-adder" }, input, sug);
  chip.replaceWith(wrap);
  input.focus();
  let known: { tag: string; count: number }[] = [];
  void run<{ tag: string; count: number }[]>("note.tags").then((t) => { known = t; draw(); }).catch(() => {});
  const draw = () => {
    const q = input.value.trim().toLowerCase();
    const opts = q ? known.filter((k) => k.tag.includes(q) && !have().includes(k.tag)).slice(0, 6) : [];
    clear(sug, opts.map((k) => {
      const b = button(k.tag, () => { add(k.tag); }, { role: "option", class: "tag-opt", tabindex: "-1" });
      b.addEventListener("mousedown", (e) => e.preventDefault());
      return b;
    }));
    sug.hidden = opts.length === 0;
  };
  let done = false;
  const end = () => {
    if (done) return;
    done = true;
    wrap.replaceWith(chip);
  };
  const add = (raw: string) => {
    const tag = raw.trim().replace(/\s+/g, " ").toLowerCase();
    if (tag && !have().includes(tag)) onAdd(tag);
    input.value = "";
    draw();
    input.focus();
  };
  input.addEventListener("input", () => {
    if (input.value.includes(",")) add(input.value.replace(/,/g, ""));
    else draw();
  });
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      if (input.value.trim()) add(input.value);
      else {
        end();
        chip.focus();
      }
    } else if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      end();
      chip.focus();
    } else if (e.key === "ArrowDown" && !sug.hidden) {
      e.preventDefault();
      sug.querySelector<HTMLElement>("button")?.focus();
    }
  });
  sug.addEventListener("keydown", (e) => {
    const items = [...sug.querySelectorAll<HTMLElement>("button")];
    const i = items.indexOf(document.activeElement as HTMLElement);
    if (e.key === "ArrowDown") { e.preventDefault(); items[Math.min(items.length - 1, i + 1)]?.focus(); }
    if (e.key === "ArrowUp") { e.preventDefault(); (i <= 0 ? input : items[i - 1]).focus(); }
    if (e.key === "Escape") { e.preventDefault(); input.focus(); }
  });
  wrap.addEventListener("focusout", () => setTimeout(() => {
    if (!wrap.contains(document.activeElement)) {
      if (input.value.trim()) add(input.value);
      end();
    }
  }, 0));
  draw();
}
