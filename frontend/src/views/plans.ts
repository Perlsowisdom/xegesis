// Plans: the reading plans you follow (today's reading, catching up, progress), and a short form to
// start a new one. Everything beyond the plan and the start date is under More options.

import { printExportButton } from "../printexport";
import { ItemSummary, Message, run } from "../api";
import { button, clear, field, h, select } from "../dom";
import { todayBlock, openReading } from "../plancard";
import {
  DayInfo, PlanInfo, TemplateInfo, TodayInfo, WEEKDAYS, finishText, friendlyDate, handDays, isoDate, progressText,
  readingText, splitList,
} from "../planwords";

const OWN = "own";

interface BookPart {
  item_id: string;
  title: string;
  from: number;
  to: number;
}

export function plansView() {
  const msg = new Message();
  const list = h("div", { class: "plan-list" });
  const startBox = h("div", {});
  let templates: TemplateInfo[] = [];
  let books: ItemSummary[] = [];
  let open: string | null = null; // the plan whose days are listed

  const el = h("div", { class: "view" }, h("h1", {}, "Reading plans"), msg.el,
    h("section", { "aria-label": "Your plans" }, h("h2", {}, "Your plans"), list),
    h("section", { "aria-label": "Start a plan" }, h("h2", {}, "Start a plan"), startBox));

  async function show() {
    if (templates.length === 0) templates = (await msg.attempt(() => run<TemplateInfo[]>("plan.templates"))) ?? [];
    if (startBox.childElementCount === 0) renderStart();
    await refresh();
  }

  async function refresh() {
    const [plans, today] = await Promise.all([
      msg.attempt(() => run<PlanInfo[]>("plan.list")),
      msg.attempt(() => run<TodayInfo[]>("plan.today", {})),
    ]);
    if (!plans) return;
    const stands = new Map((today ?? []).map((t) => [t.plan.id, t]));
    clear(list, plans.length === 0
      ? h("p", { class: "empty" }, "No plans yet. Start one below.")
      : plans.map((p) => planCard(p, stands.get(p.id))));
  }

  function planCard(p: PlanInfo, t: TodayInfo | undefined): HTMLElement {
    const days = h("div", {});
    const card = h("article", { class: "card plan", "aria-label": p.name },
      h("div", { class: "row between" },
        h("h3", {}, p.name),
        p.behind > 0 ? h("span", { class: "badge warn" }, `${p.behind} behind`) : p.finished ? h("span", { class: "badge ok" }, "Finished") : null),
      h("progress", { max: p.days, value: p.days_done, "aria-label": `${p.name}: ${progressText(p)}` }),
      h("p", { class: "hint" }, progressText(p), p.finish || p.finished ? " · " : "", finishText(p),
        p.weekdays.length < 7 ? ` · reads ${p.weekdays.join(", ")}` : ""),
      t ? todayBlock(t, { msg, changed: refresh, headline: false }) : null,
      h("div", { class: "row wrap" },
        button(open === p.id ? "Hide days" : "Show all days", () => toggleDays(p), { "aria-expanded": String(open === p.id) }),
        p.behind === 0 && !p.finished ? button("Reschedule from today", () => reschedule(p), { title: "Spread the days not yet read over the coming days, starting today" }) : null,
        printExportButton(() => ({ title: p.name, sources: [{ kind: "plan", id: p.id }] })),
        h("span", { class: "spacer" }),
        button("Delete", () => remove(p), { class: "danger", "aria-label": `Delete ${p.name}` })),
      days);
    if (open === p.id) void fillDays(p, days);
    return card;
  }

  async function toggleDays(p: PlanInfo) {
    open = open === p.id ? null : p.id;
    await refresh();
  }

  async function fillDays(p: PlanInfo, into: HTMLElement) {
    const ds = await msg.attempt(() => run<DayInfo[]>("plan.days", { plan_id: p.id }));
    if (!ds) return;
    clear(into, h("table", { class: "plan-days" },
      h("caption", { class: "sr-only" }, `Days of ${p.name}`),
      h("thead", {}, h("tr", {}, ["Day", "Date", "Reading", "Done"].map((c) => h("th", { scope: "col" }, c)))),
      h("tbody", {}, ds.map((d) => h("tr", { class: d.done ? "done" : "" },
        h("td", {}, String(d.day)), h("td", {}, friendlyDate(d.date)),
        h("td", {}, d.readings.map((r) => button(r.ref, () => void msg.attempt(() => openReading(r)), { class: "link", title: `Open ${readingText(d)}` }))),
        h("td", {}, h("input", {
          type: "checkbox", checked: d.done, "aria-label": `Day ${d.day}, ${readingText(d)}: read`,
          onchange: async (e: Event) => {
            const on = (e.target as HTMLInputElement).checked;
            const ok = await msg.attempt(() => on ? run("plan.mark", { plan_id: p.id, days: [d.day] }) : run("plan.unmark", { plan_id: p.id, day: d.day }));
            if (ok !== undefined) refresh();
          },
        })))))));
  }

  async function reschedule(p: PlanInfo) {
    if ((await msg.attempt(() => run("plan.reschedule", { plan_id: p.id }))) !== undefined) {
      msg.ok(`${p.name}: the days left now start today.`);
      refresh();
    }
  }

  async function remove(p: PlanInfo) {
    if (!confirm(`Delete "${p.name}"? Your progress is kept in its history.`)) return;
    if ((await msg.attempt(() => run("plan.delete", { id: p.id }))) !== undefined) {
      if (open === p.id) open = null;
      msg.ok(`Deleted ${p.name}.`);
      refresh();
    }
  }

  // Starting a plan ---------------------------------------------------------------------------------

  function renderStart() {
    const choice = select([...templates.map((t): [string, string] => [t.id, `${t.name} (${t.days} days)`]), [OWN, "Your own plan…"]],
      templates[0]?.id ?? OWN, { "aria-describedby": "plan-about" });
    const about = h("p", { class: "hint", id: "plan-about", "aria-live": "polite" });
    const start = h("input", { type: "date", value: isoDate(new Date()), required: true });

    const own = h("textarea", { rows: 3, placeholder: "e.g. Mark, Romans 1-8, Psalm 23" });
    const ownField = field("What to read", own, "Books or passages, in order, separated by commas.");
    const name = h("input", { type: "text", placeholder: "Plan name" });
    const days = h("input", { type: "number", min: 1, max: 3660, placeholder: "e.g. 30" });
    const end = h("input", { type: "date" });
    const weekdays = WEEKDAYS.map(([v, label]) => ({ v, label, box: h("input", { type: "checkbox", value: v, checked: true }) }));
    const byHand = h("textarea", { rows: 4, placeholder: "John 1; Psalm 23\nJohn 2\nRomans 8" });
    const parts: BookPart[] = [];
    const partList = h("ul", { class: "book-parts" });
    const bookSel = select([["", "Choose a library book…"]], "", { "aria-label": "Library book" });
    const from = h("input", { type: "number", min: 1, placeholder: "first section", "aria-label": "First section" });
    const to = h("input", { type: "number", min: 1, placeholder: "last section", "aria-label": "Last section" });

    const form = h("form", { class: "card plan-start", "aria-label": "Start a plan" });
    const mine = () => choice.value === OWN;

    function layout() {
      const t = templates.find((x) => x.id === choice.value);
      about.textContent = t ? `${t.description} ${t.rule}` : "Choose books or passages, and how long you want to take.";
      ownField.hidden = !mine();
      byHandField.hidden = !mine();
      bookBox.hidden = !mine();
      nameField.hidden = false;
    }
    const byHandField = field("Or list the days by hand", byHand, "One day a line; separate readings with a semicolon. Used instead of the box above.");
    const nameField = field("Name", name, "Optional.");
    const bookBox = h("div", { class: "field" },
      h("strong", {}, "Library books"),
      h("div", { class: "row wrap" }, bookSel, from, to, button("Add", addPart)),
      h("small", { class: "hint" }, "Sections are read after the passages, not when you list the days by hand. Leave the numbers empty for the whole book."), partList);

    async function loadBooks() {
      const r = await msg.attempt(() => run<{ items: ItemSummary[] }>("library.items", { limit: 200 }));
      books = (r?.items ?? []).filter((it) => it.kind !== "bible" && it.status === "online");
      clear(bookSel, h("option", { value: "" }, books.length ? "Choose a library book…" : "No library books"),
        books.map((b) => h("option", { value: b.id }, b.title || "Untitled")));
    }
    function addPart() {
      const b = books.find((x) => x.id === bookSel.value);
      if (!b) return;
      parts.push({ item_id: b.id, title: b.title, from: Number(from.value) || 0, to: Number(to.value) || 0 });
      from.value = to.value = "";
      renderParts();
    }
    function renderParts() {
      clear(partList, parts.map((p, i) => h("li", {}, `${p.title || "Untitled"}`,
        p.from || p.to ? ` (sections ${p.from || 1}–${p.to || "end"})` : " (whole book)", " ",
        button("Remove", () => { parts.splice(i, 1); renderParts(); }, { "aria-label": `Remove ${p.title}` }))));
    }

    const more = h("details", { class: "more" }, h("summary", {}, "More options"),
      nameField,
      h("div", { class: "row wrap end" },
        field("Number of days", days, mine() ? "Default: one chapter a day." : "Changes the length of the plan."),
        field("Or finish on", end)),
      h("fieldset", { class: "weekdays" }, h("legend", {}, "Days to read"),
        h("div", { class: "row wrap" }, weekdays.map((w) => h("label", { class: "check" }, w.box, ` ${w.label}`)))),
      byHandField, bookBox);

    form.append(field("Plan", choice), about, field("Start date", start), more,
      h("div", { class: "row" }, h("button", { type: "submit", class: "primary" }, "Start plan")));
    clear(startBox, form);
    choice.addEventListener("change", layout);
    layout();
    void loadBooks();

    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      const picked = weekdays.filter((w) => w.box.checked).map((w) => w.v);
      if (picked.length === 0) {
        msg.error(new Error("bad_params: choose at least one day of the week to read on"));
        return;
      }
      const p: Record<string, unknown> = { start: start.value };
      if (name.value.trim()) p.name = name.value.trim();
      if (picked.length < 7) p.weekdays = picked;
      if (days.value) p.days = Number(days.value);
      if (end.value) p.end = end.value;
      if (mine()) {
        const hand = handDays(byHand.value);
        if (hand.length > 0) {
          p.manual = hand;
          delete p.days;
          delete p.end;
        } else {
          p.ranges = splitList(own.value);
          if (parts.length) p.books = parts.map(partParams);
        }
      } else {
        p.template = choice.value;
      }
      const made = await msg.attempt(() => run<PlanInfo>("plan.create", p));
      if (made) {
        msg.ok(`Started ${made.name}: ${made.days} days, finishing ${friendlyDate(made.finish ?? "")}.`);
        open = null;
        refresh();
      }
    });
  }

  return { el, show };
}

function partParams(p: BookPart) {
  return { item_id: p.item_id, ...(p.from ? { from: p.from } : {}), ...(p.to ? { to: p.to } : {}) };
}
