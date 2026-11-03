// Reports: everything the library holds about a passage, a word or a topic in one place
// (report.passage, report.word, report.topic). Sections fold away, empty ones are not sent, and every
// reference, word and book is a link. "Save as note" keeps the report's headings and links.

import { Message, Ref, run } from "../api";
import { badge, button, clear, field, h, select } from "../dom";
import { markSegments } from "../helpers";
import { WordInfo, paragraphs, renderMarkup } from "../markup";
import {
  KINDS, Report, ReportEntry, ReportKind, ReportSection, Route, barWidths, countText, openByDefault, paramFor, routeOf,
  runs, sourceText, wordFacts,
} from "../reportwords";
import { navigate, setPassage, state } from "../state";
import { openBook } from "./book";

// Other views start reports through openReport; the Reports view makes it when it is shown.
let pending: { kind: ReportKind; query: string } | null = null;

export function openReport(kind: ReportKind, query: string) {
  if (!query.trim()) return;
  pending = { kind, query };
  navigate("reports");
}

// follow opens an app address (#passage/…, #word/…, #book/…); it returns false for anything else.
export function follow(hash: string): boolean {
  const r = routeOf(hash);
  if (!r) return false;
  go(r);
  return true;
}

function go(r: Route) {
  switch (r.to) {
    case "passage":
      void readPassage(r.ref);
      break;
    case "report":
      openReport(r.kind, r.query);
      break;
    case "book":
      navigate("reader");
      openBook({ itemId: r.itemId, fileId: r.fileId, locator: r.locator });
      break;
    case "section":
      navigate(r.section);
  }
}

async function readPassage(osis: string) {
  const p = await run<Ref>("verse.parse", { ref: osis }).catch(() => null);
  if (!p) return;
  setPassage(p.display, p);
  navigate("reader");
}

// link is an ordinary link to an app address, so it can be followed with the keyboard.
function link(label: string, address: string, attrs: Record<string, unknown> = {}): HTMLAnchorElement {
  const a = h("a", { href: address, ...attrs }, label);
  a.addEventListener("click", (e) => {
    e.preventDefault();
    follow(address);
  });
  return a;
}

const refLink = (osis: string, label?: string) => link(label || osis, `#passage/${osis}`, { class: "ref" });
const wordLink = (strong: string, label?: string) => link(label || strong, `#word/${encodeURIComponent(strong)}`, { class: "word-link", title: "Word report" });

const handlers = {
  onRef: (osis: string) => void readPassage(osis),
  onWord: (w: WordInfo) => {
    const n = w.strongs.filter((s) => !/^H9\d{3}/.test(s));
    openReport("word", n[n.length - 1] ?? w.strongs[0]);
  },
};

function joined(nodes: Node[], sep = "; "): Node[] {
  return nodes.flatMap((n, i) => (i ? [document.createTextNode(sep), n] : [n]));
}

function snippet(s: string): Node[] {
  return markSegments(s).map((x) => (x.mark ? h("mark", {}, x.text) : document.createTextNode(x.text)));
}

export function reportView() {
  const msg = new Message();
  const kind = select(KINDS.map(([k, label]) => [k, label] as [string, string]), "passage", { "aria-label": "Report on" });
  const input = h("input", { type: "search", autocomplete: "off", spellcheck: "false", placeholder: KINDS[0][2] });
  const form = h("form", { class: "card report-form", role: "search", "aria-label": "Make a report" },
    h("div", { class: "row wrap end" }, field("Report on", kind), h("div", { class: "grow" }, field("About", input)),
      h("button", { type: "submit", class: "primary" }, "Make report")));
  const head = h("div", { class: "report-head" });
  const body = h("div", { class: "report", "aria-live": "polite" });
  let current: Report | null = null;
  let seq = 0;

  kind.addEventListener("change", () => {
    input.placeholder = KINDS.find((k) => k[0] === kind.value)?.[2] ?? "";
  });
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    make(kind.value as ReportKind, input.value);
  });

  async function make(k: ReportKind, query: string) {
    query = query.trim();
    if (!query) return;
    kind.value = k;
    input.value = query;
    const mine = ++seq;
    clear(head, h("p", { class: "hint", role: "status" }, "Gathering the report…"));
    clear(body);
    const rep = await msg.attempt(() => run<Report>(`report.${k}`, { [paramFor(k)]: query, bibles: state.modules }));
    if (mine !== seq) return;
    if (!rep) return void clear(head);
    current = rep;
    if (rep.kind === "passage") input.value = rep.subject;
    render(rep);
  }

  function render(rep: Report) {
    clear(head,
      h("div", { class: "row between wrap" },
        h("h2", {}, rep.title),
        button("Save as note", () => save(), { title: "Keep this report's headings and links as a note; the text of the books is not copied" })),
      rep.candidates && rep.candidates.length > 1 ? h("p", { class: "hint" }, `"${input.value}" translates several words. Also: `,
        joined(rep.candidates.slice(1).map((c) => wordLink(c.strong!, c.heading)), ", ")) : null);
    if (rep.sections.length === 0) {
      clear(body, h("p", { class: "empty" }, emptyText(rep)));
      return;
    }
    let index = 0;
    clear(body, runs(rep.sections).map((r) => {
      if (r.group === "text" && r.sections.length > 1) {
        const open = openByDefault(index++, r.sections[0]);
        return h("details", { class: "report-section", open },
          h("summary", {}, h("h3", {}, "Bible text")),
          h("div", { class: "report-columns" }, r.sections.map((s) => h("div", { class: "report-column" },
            h("h4", {}, s.title), sectionBody(s), credit(s)))));
      }
      return r.sections.map((s) => sectionEl(s, openByDefault(index++, s)));
    }));
  }

  function emptyText(rep: Report): string {
    if (rep.kind === "word" && !rep.strong) return "No installed lexicon or tagged Bible has this word. Lexicons and tagged Bibles are in Library → Get Bibles.";
    return "Nothing in your library is about this yet. Install Bibles, commentaries and dictionaries in Library → Get Bibles, and index your books in Library.";
  }

  function sectionEl(s: ReportSection, open: boolean): HTMLElement {
    const count = countText(s);
    return h("details", { class: "report-section", open, "data-section": s.id },
      h("summary", {}, h("h3", {}, s.title), count ? h("span", { class: "badge" }, count) : null),
      s.hint ? h("p", { class: "hint" }, s.hint) : null,
      s.error ? h("p", { class: "msg error" }, `This part could not be gathered: ${s.error}`) : null,
      sectionBody(s),
      credit(s));
  }

  function credit(s: ReportSection) {
    const text = sourceText(s.sources.filter((x) => x.licence !== "own_work"));
    return text ? h("p", { class: "attribution" }, text) : null;
  }

  // sectionBody draws the entries, with a button for the next page.
  function sectionBody(s: ReportSection): HTMLElement {
    const list = h("div", { class: `report-entries g-${s.group}` });
    const more = h("div", {});
    const fill = (sec: ReportSection) => {
      list.append(...entries(sec));
      clear(more, sec.more ? button("Show more", async () => {
        if (!current) return;
        const next = await msg.attempt(() => run<Report>(`report.${current!.kind}`,
          { [paramFor(current!.kind)]: current!.query, cursor: sec.more, bibles: state.modules }));
        const got = next?.sections.find((x) => x.id === s.id);
        if (got) fill(got);
        else clear(more);
      }) : null);
    };
    const wrap = h("div", {}, s.chart?.length ? chart(s) : null, list, more);
    fill(s);
    return wrap;
  }

  function entries(s: ReportSection): Node[] {
    switch (s.group) {
      case "text":
      case "occurrences":
        return s.entries.map((e) => h("p", { class: "report-verse" }, e.ref ? refLink(e.ref, e.ref_label) : null, " ",
          e.markup ? renderMarkup(e.markup, handlers) : e.text ?? ""));
      case "crossrefs":
        return [h("dl", { class: "info" }, s.entries.map((e) => [
          h("dt", {}, e.ref ? refLink(e.ref, e.ref_label) : e.heading),
          h("dd", {}, joined((e.links ?? []).map((l) => refLink(l.ref, l.label))))]))];
      case "words":
        return [table(["Word", "Dictionary form", "Meaning", "Number", "Verse"], s.entries.map((e) => [
          e.strong ? wordLink(e.strong, e.heading) : e.heading ?? "",
          e.lemma ? `${e.lemma}${e.transliteration ? ` (${e.transliteration})` : ""}` : "",
          e.gloss ?? "",
          e.strong ? wordLink(e.strong) : "",
          e.ref ? refLink(e.ref, e.ref_label) : ""]))];
      case "renderings":
        return [h("ul", { class: "renderings" }, s.entries.map((e) => h("li", {},
          h("strong", {}, e.heading), ` ${e.count}× `, joined((e.links ?? []).map((l) => refLink(l.ref, l.label))))))];
      case "related":
        return [h("ul", {}, s.entries.map((e) => h("li", {}, wordLink(e.strong!, e.heading), e.gloss ? ` · ${e.gloss}` : "")))];
      case "citing":
      case "books":
        return [h("ol", { class: "results" }, s.entries.map(bookEntry))];
      case "verses":
        return [h("ul", { class: "results" }, s.entries.map((e) => h("li", {},
          e.ref ? refLink(e.ref, e.ref_label) : null, h("p", {}, e.snippet ? snippet(e.snippet) : e.text))))];
      case "commentary":
      case "lexicon":
      case "dictionary":
      case "topical":
        return s.entries.map((e) => h("div", { class: "lex-entry" },
          h("h4", {}, entryHeading(e)),
          e.strong && s.group === "lexicon" ? h("p", { class: "word-facts" }, wordFacts(e)) : null,
          paragraphs(e.markup || e.text || "", handlers)));
    }
    return [h("ul", { class: "report-list" }, s.entries.map((e) => h("li", {}, plainEntry(e))))];
  }

  function entryHeading(e: ReportEntry): Node | string {
    if (e.topic) return link(e.heading ?? e.topic, `#topic/${encodeURIComponent(e.topic)}`, { title: "Topic report" });
    if (e.ref) return refLink(e.ref, e.heading || e.ref_label);
    return e.heading ?? "";
  }

  // plainEntry is a note, highlight, list entry or plan day.
  function plainEntry(e: ReportEntry): (Node | string)[] {
    const where = e.note_id ? `#note/${e.note_id}` : e.list_id ? `#list/${e.list_id}` : e.plan_id ? `#plan/${e.plan_id}` : "";
    const out: (Node | string)[] = [];
    if (e.heading && e.heading !== e.ref_label) out.push(where ? link(e.heading, where) : h("strong", {}, e.heading));
    if (e.ref) out.push(out.length ? " · " : "", refLink(e.ref, e.ref_label));
    else if (e.ref_label) out.push(` · ${e.ref_label}`);
    for (const x of e.extra ?? []) if (x) out.push(` · ${x}`);
    if (e.done) out.push(" ", badge("read", "ok"));
    if (e.text) out.push(h("p", { class: "hint" }, e.text));
    return out;
  }

  function bookEntry(e: ReportEntry) {
    const b = e.book!;
    const open = h("button", { type: "button", class: "link", disabled: !b.online }, b.title || "Untitled");
    open.addEventListener("click", () => {
      navigate("reader");
      openBook({ itemId: b.item_id, fileId: b.file_id, locator: b.locator });
    });
    const where = b.locator.page ? `page ${b.locator.page}` : b.heading ?? "";
    return h("li", {},
      h("div", { class: "row wrap" }, open,
        b.authors.length ? h("span", { class: "hint" }, b.authors.join(", ")) : null,
        where ? h("span", { class: "badge" }, where) : null,
        e.ref ? refLink(e.ref, e.ref_label) : null,
        b.online ? null : h("span", { class: "badge warn", title: "Its library folder's drive is not connected" }, "offline")),
      e.snippet ? h("p", {}, snippet(e.snippet)) : null);
  }

  function chart(s: ReportSection) {
    const widths = barWidths(s.chart!);
    return h("figure", { class: "report-chart" },
      h("figcaption", { class: "hint" }, "Verses by book"),
      h("ol", { class: "bars" }, s.chart!.map((b, i) => h("li", {},
        h("span", { class: "bar-label" }, b.label),
        h("span", { class: "bar", style: `width: ${widths[i]}%`, "aria-hidden": "true" }),
        h("span", { class: "bar-count" }, String(b.count))))));
  }

  function table(headings: string[], rows: (Node | string)[][]) {
    return h("div", { class: "table-wrap" }, h("table", { class: "list" },
      h("thead", {}, h("tr", {}, headings.map((t) => h("th", { scope: "col" }, t)))),
      h("tbody", {}, rows.map((r) => h("tr", {}, r.map((c) => h("td", {}, c)))))));
  }

  async function save() {
    if (!current) return;
    const rep = current;
    const n = await msg.attempt(() => run<{ title: string }>("report.save", { kind: rep.kind, query: rep.query, bibles: state.modules }));
    if (!n) return;
    clear(msg.el, `Saved as the note "${n.title}", with its headings and links. `, button("Open notes", () => navigate("notes")));
    msg.el.className = "msg ok";
    msg.el.setAttribute("role", "status");
  }

  const el = h("div", { class: "view" },
    h("h1", {}, "Reports"),
    h("p", { class: "hint" }, "Everything your library holds about a passage, a word or a topic, in one place."),
    form, msg.el, head, body);

  return {
    el,
    show() {
      if (pending) {
        const p = pending;
        pending = null;
        void make(p.kind, p.query);
      } else if (!input.value && state.ref) {
        input.value = state.ref;
      }
    },
    focus: () => input.focus(),
  };
}
