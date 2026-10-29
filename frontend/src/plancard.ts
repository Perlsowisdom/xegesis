// The parts of reading plans shown in more than one place: opening a reading, and "where this plan
// stands today" with its buttons for marking days read, catching up and rescheduling. The Reader
// shows a small card of them (todayCard); the Plans section shows one per plan.

import { Message, Ref, run } from "./api";
import { button, clear, h } from "./dom";
import { DayInfo, PlanInfo, ReadingInfo, TodayInfo, behindText, friendlyDate, statusText } from "./planwords";
import { navigate, setPassage } from "./state";
import { openBook } from "./views/book";

// openReading shows a reading in the Reader: Bible passages in the text, library books in the book
// pane. stay is true when the Reader itself is asking, so it need not be switched to.
export async function openReading(r: ReadingInfo, stay = false): Promise<void> {
  if (r.item_id) {
    const s = r.section ?? 1;
    openBook({ itemId: r.item_id, locator: { spine: s, page: s } });
  } else if (r.osis) {
    const p = await run<Ref>("verse.parse", { ref: r.osis });
    setPassage(p.display, p);
  }
  if (!stay) navigate("reader");
}

function readingButtons(d: DayInfo, plan: string, stay: boolean, msg: Message): HTMLElement {
  return h("span", { class: "plan-readings" }, d.readings.map((r) =>
    button(r.ref, () => void msg.attempt(() => openReading(r, stay)),
      { class: "link", title: `Open ${r.ref}`, "aria-label": `Open ${r.ref} from ${plan}` })));
}

export interface BlockOptions {
  msg: Message;
  changed: () => void;
  stay?: boolean; // true in the Reader
  headline?: boolean; // show the plan's name
}

// todayBlock shows one plan's reading for today and anything to catch up on.
export function todayBlock(t: TodayInfo, o: BlockOptions): HTMLElement {
  const name = t.plan.name;
  const stay = o.stay ?? false;
  const act = async (fn: () => Promise<unknown>) => {
    if ((await o.msg.attempt(fn)) !== undefined) o.changed();
  };
  const mark = (days?: number[]) => act(() => run<PlanInfo>("plan.mark", { plan_id: t.plan.id, ...(days ? { days } : {}), }));
  const row = (d: DayInfo, label: string, markable: boolean) =>
    h("div", { class: "plan-day" },
      h("span", { class: "plan-when" }, label), " ", readingButtons(d, name, stay, o.msg),
      markable ? button("Mark done", () => mark([d.day]), { "aria-label": `Mark day ${d.day} of ${name} done` }) : null);

  const parts: (HTMLElement | null)[] = [];
  if (o.headline !== false) {
    parts.push(h("div", { class: "row between" }, h("strong", {}, name), h("span", { class: "hint" }, statusText(t))));
  }
  if (t.missed.length > 0) {
    parts.push(h("div", { class: "plan-catchup", role: "group", "aria-label": `Catch up on ${name}` },
      h("p", {}, h("strong", {}, "Catch up. "), behindText(t.missed.length)),
      t.missed.map((d) => row(d, friendlyDate(d.date), true)),
      h("div", { class: "row wrap" },
        button("Mark all as read", () => mark(t.missed.map((d) => d.day))),
        button("Reschedule from today", () => act(() => run("plan.reschedule", { plan_id: t.plan.id })),
          { title: "Spread the days not yet read over the coming days, starting today" }))));
  }
  if (t.today) {
    const d = t.today;
    parts.push(h("div", { class: "plan-day plan-today" },
      h("span", { class: "plan-when" }, `Day ${d.day} of ${t.plan.days}`), " ", readingButtons(d, name, stay, o.msg),
      d.done
        ? h("span", { class: "row" }, h("span", { class: "badge ok" }, "Done"),
            button("Undo", () => act(() => run("plan.unmark", { plan_id: t.plan.id, day: d.day })), { "aria-label": `Mark day ${d.day} of ${name} not read` }))
        : button("Mark done", () => mark([d.day]), { class: "primary", "aria-label": `Mark today's reading of ${name} done` })));
  } else if (t.next) {
    parts.push(row(t.next, `Next, ${friendlyDate(t.next.date)}`, false));
  }
  return h("div", { class: "plan-block" }, parts);
}

// todayCard is the small "Today's reading" card at the top of the Reader. It stays hidden when
// there is no plan with something to read.
export function todayCard() {
  const msg = new Message();
  const body = h("div", {});
  const el = h("section", { class: "card plan-card", "aria-label": "Today's reading", hidden: true },
    h("div", { class: "row between" }, h("h2", {}, "Today's reading"),
      button("All plans", () => navigate("plans"), { title: "Open the Plans section" })),
    msg.el, body);

  async function refresh() {
    const list = await msg.attempt(() => run<TodayInfo[]>("plan.today"));
    if (!list) return;
    const shown = list.filter((t) => t.status === "today" || t.status === "done_for_today" || t.status === "behind");
    el.hidden = shown.length === 0;
    clear(body, shown.map((t) => todayBlock(t, { msg, changed: refresh, stay: true })));
  }
  return { el, refresh };
}
