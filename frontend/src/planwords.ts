// Reading plans: the shapes of the plan commands' results and the plain words the interface uses
// for them. Pure functions, so they can be unit tested.

export interface ReadingInfo {
  ref: string;
  osis?: string;
  start?: number;
  end?: number;
  item_id?: string;
  section?: number;
  title?: string;
  verses?: number;
}

export interface DayInfo {
  day: number;
  date: string;
  readings: ReadingInfo[];
  done: boolean;
  completed_at?: number;
}

export interface PlanInfo {
  id: string;
  name: string;
  template?: string;
  start_date: string;
  weekdays: string[];
  days: number;
  days_done: number;
  days_left: number;
  behind: number;
  finish?: string;
  finished: boolean;
}

export type TodayStatus = "today" | "done_for_today" | "rest" | "upcoming" | "behind" | "finished";

export interface TodayInfo {
  plan: PlanInfo;
  date: string;
  status: TodayStatus;
  today?: DayInfo | null;
  next?: DayInfo | null;
  missed: DayInfo[];
}

export interface TemplateInfo {
  id: string;
  name: string;
  description: string;
  days: number;
  rule: string;
}

export const WEEKDAYS: [string, string][] = [
  ["mon", "Mon"], ["tue", "Tue"], ["wed", "Wed"], ["thu", "Thu"], ["fri", "Fri"], ["sat", "Sat"], ["sun", "Sun"],
];

function plural(n: number, one: string, many = one + "s"): string {
  return `${n} ${n === 1 ? one : many}`;
}

// localDate reads an ISO date (YYYY-MM-DD) as a day in the user's calendar.
export function localDate(iso: string): Date {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(y, (m || 1) - 1, d || 1);
}

// isoDate writes a Date as YYYY-MM-DD in the user's calendar.
export function isoDate(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

// friendlyDate shows a plan date the way people say it: "Mon 2 Mar", with the year when it is
// not the current one.
export function friendlyDate(iso: string, now = new Date()): string {
  if (!iso) return "";
  const d = localDate(iso);
  const opts: Intl.DateTimeFormatOptions = { weekday: "short", day: "numeric", month: "short" };
  if (d.getFullYear() !== now.getFullYear()) opts.year = "numeric";
  return d.toLocaleDateString("en-GB", opts);
}

// readingText joins a day's readings: "Genesis 1-3; Psalms 1".
export function readingText(d: DayInfo): string {
  return d.readings.map((r) => r.ref).join("; ");
}

// progressText: "12 days done, 18 days left".
export function progressText(p: PlanInfo): string {
  if (p.finished) return `All ${plural(p.days, "day")} done`;
  return `${plural(p.days_done, "day")} done, ${plural(p.days_left, "day")} left`;
}

// finishText: when the plan is expected to end.
export function finishText(p: PlanInfo, now = new Date()): string {
  if (p.finished) return "Finished";
  if (!p.finish) return "";
  return p.behind > 0 ? `Finish by ${friendlyDate(p.finish, now)} if you read each day from today`
    : `Finish on ${friendlyDate(p.finish, now)}`;
}

// behindText: what to say when days were missed.
export function behindText(n: number): string {
  return n === 1 ? "You missed 1 day." : `You missed ${n} days.`;
}

// statusText: one line on where a plan stands today.
export function statusText(t: TodayInfo, now = new Date()): string {
  switch (t.status) {
    case "today": return "Today";
    case "done_for_today": return "Today's reading is done";
    case "behind": return behindText(t.missed.length);
    case "finished": return "Finished";
    case "upcoming": return `Starts ${friendlyDate(t.plan.start_date, now)}`;
    default: return t.next ? `No reading today. Next: ${friendlyDate(t.next.date, now)}` : "No reading today";
  }
}

// splitList reads a typed list of books or passages, separated by commas, semicolons or lines.
export function splitList(text: string): string[] {
  return text.split(/[\n;,]+/).map((s) => s.trim()).filter(Boolean);
}

// handDays reads days listed by hand: one day a line, readings separated by semicolons.
export function handDays(text: string): { refs: string[] }[] {
  return text.split("\n").map((line) => ({ refs: line.split(";").map((s) => s.trim()).filter(Boolean) }))
    .filter((d) => d.refs.length > 0);
}

// todayLine is the one-line summary of today's reading at the top of the Reader, such as
// "Gospels · Day 7: John 3, John 4 · 2 behind". More plans are counted, not listed.
export function todayLine(list: TodayInfo[]): string {
  if (list.length === 0) return "";
  const t = list[0];
  const parts = [t.plan.name];
  if (t.today) parts.push(`Day ${t.today.day}: ${t.today.readings.map((r) => r.ref).join(", ")}${t.today.done ? " (done)" : ""}`);
  else if (t.status === "done_for_today") parts.push("done for today");
  if (t.missed.length) parts.push(`${t.missed.length} behind`);
  const more = list.length - 1;
  return parts.join(" · ") + (more > 0 ? ` · and ${more} more ${more === 1 ? "plan" : "plans"}` : "");
}
