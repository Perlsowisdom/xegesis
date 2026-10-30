// Print or save: one dialog for every kind of content. It shows the page first, with what each
// item's licence left out and why; the common path is one click (Print, or a format to save as).
// Options are folded under More options. Printing uses the system print dialog, which also offers
// "Save as PDF"; saving writes a file through export.make.

import { Message, RunError, run, saveFile } from "./api";
import { button, clear, field, h, select } from "./dom";
import { flash } from "./listpicker";
import {
  ExportOptions, ExportPreview, ExportSource, FORMATS, Waiting, fileName, formatLabel, loadWaiting, rests, saveWaiting, summary,
} from "./printwords";

export interface ExportRequest {
  title: string; // what is being printed, in words, e.g. "John 3:16-18"
  sources: ExportSource[];
  options?: ExportOptions;
}

const OPTIONS_KEY = "xegesis.printOptions";

function savedOptions(): ExportOptions {
  try {
    return JSON.parse(localStorage.getItem(OPTIONS_KEY) ?? "{}") as ExportOptions;
  } catch {
    return {};
  }
}

function today(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

// printExportButton is the "Print or save" button views add; req is read when it is clicked.
export function printExportButton(req: () => ExportRequest | null, label = "Print or save…"): HTMLButtonElement {
  return button(label, () => {
    const r = req();
    if (r) openExport(r);
  }, { title: "Print, or save as PDF, Word, a web page or Markdown" });
}

export function openExport(req: ExportRequest) {
  const msg = new Message();
  const opts: ExportOptions = { ...savedOptions(), ...req.options };
  const passages = req.sources.filter((s) => s.kind === "passage");
  const parallel = passages.some((s) => Array.isArray(s.modules) && (s.modules as unknown[]).length > 1);

  const dlg = h("dialog", { class: "export-dialog", "aria-labelledby": "ex-title" });
  const info = h("p", { class: "hint", "aria-live": "polite" });
  const cuts = h("div", { class: "export-cuts" });
  const style = h("style");
  const paper = h("div", { class: "paper", "aria-label": "Preview", role: "document" });
  const result = h("p", { class: "msg", "aria-live": "polite" });

  // More options -------------------------------------------------------------------------------
  const size = select([["a4", "A4"], ["letter", "Letter"]], opts.page_size ?? "a4");
  const font = select(["10", "11", "12", "14", "16"], String(opts.font_size ?? 11));
  const check = (on: boolean | undefined, def: boolean) => h("input", { type: "checkbox", checked: on ?? def });
  const numbers = check(opts.verse_numbers, true);
  const notes = check(opts.notes, false);
  const header = check(opts.header, true);
  const footer = check(opts.footer, true);
  const layout = select([["side_by_side", "Side by side"], ["one_after_another", "One after another"]], opts.layout ?? "side_by_side");
  const credits = select([["each", "Under each piece"], ["end", "Together at the end"]], opts.credits ?? "each");
  const more = h("details", { class: "more" }, h("summary", {}, "More options"),
    h("div", { class: "row wrap" },
      field("Paper", size), field("Text size", font),
      parallel ? field("Bibles", layout) : null,
      field("Credits", credits)),
    h("div", { class: "row wrap" },
      passages.length ? h("label", { class: "check" }, numbers, " Verse numbers") : null,
      passages.length ? h("label", { class: "check" }, notes, " Include my notes") : null,
      h("label", { class: "check" }, header, " Title at the top of each page"),
      h("label", { class: "check" }, footer, " Page numbers")));

  function options(): ExportOptions {
    const o: ExportOptions = {
      page_size: size.value as ExportOptions["page_size"], font_size: Number(font.value),
      verse_numbers: numbers.checked, notes: notes.checked, header: header.checked, footer: footer.checked,
      credits: credits.value as ExportOptions["credits"],
    };
    if (parallel) o.layout = layout.value as ExportOptions["layout"];
    try {
      localStorage.setItem(OPTIONS_KEY, JSON.stringify({ ...o, notes: undefined }));
    } catch {
      // only a convenience
    }
    return o;
  }
  more.addEventListener("change", () => refresh());

  // Actions ------------------------------------------------------------------------------------
  const printBtn = button("Print", () => doPrint(), { class: "primary" });
  const saves = FORMATS.map((f) => button(f.label, () => doSave(f.id), { title: `Save as ${f.label}` }));
  const head = h("div", { class: "row between" }, h("h2", { id: "ex-title" }, `Print or save: ${req.title}`),
    button("Close", () => dlg.close(), { "aria-label": "Close" }));
  dlg.append(head, msg.el,
    h("div", { class: "row wrap export-actions" }, printBtn, h("span", { class: "hint" }, "or save as"), saves),
    info, cuts, result, more, waitingList(dlg) ?? "", style, paper);
  dlg.addEventListener("close", () => dlg.remove());
  document.body.append(dlg);
  dlg.showModal();
  printBtn.focus();

  let current: ExportPreview | null = null;

  async function refresh() {
    const p = await msg.attempt(() => run<ExportPreview>("export.preview", { sources: req.sources, title: req.title, format: "print", options: options() }));
    if (!p) return;
    current = p;
    show(p);
  }

  function show(p: ExportPreview) {
    info.textContent = summary(p);
    style.textContent = p.css.replace(/@page[^{]*\{[^{}]*(\{[^{}]*\}[^{}]*)*\}/g, ""); // page rules only when printing
    paper.innerHTML = p.body; // HTML built by the core; all text in it is escaped
    paper.classList.toggle("letter", size.value === "letter");
    clear(cuts, cutNotes(p, req));
  }

  async function doPrint() {
    const p = await msg.attempt(() => run<ExportPreview>("export.print", { sources: req.sources, title: req.title, options: options() }));
    if (!p) return;
    show(p);
    printPage(p);
  }

  async function doSave(format: string) {
    // Without the desktop save dialog the file goes to the exports folder in the data folder.
    let path = "";
    try {
      path = await saveFile(`Save as ${formatLabel(format)}`, fileName(current?.title || req.title, format), format);
      if (path === "") return; // cancelled
    } catch (e) {
      if (!(e instanceof RunError && e.info.code === "unavailable")) return msg.error(e);
    }
    const r = await msg.attempt(() => run<ExportPreview & { path: string }>("export.make", {
      sources: req.sources, title: req.title, format, options: options(), ...(path ? { path, replace: true } : {}),
    }));
    if (!r) return;
    show(r);
    result.className = "msg ok";
    result.textContent = `Saved ${formatLabel(format)}: ${r.path}${r.warnings.length ? ` — ${r.warnings.join(" ")}` : ""}`;
    flash(`Saved ${formatLabel(format)}`);
  }

  refresh();
}

// cutNotes says what each licence left out and why, and offers to keep the rest for another day.
function cutNotes(p: ExportPreview, req: ExportRequest): HTMLElement[] {
  const out: HTMLElement[] = [];
  const rest = rests(p);
  if (p.cuts.length) {
    out.push(h("div", { class: "notice", role: "status" },
      h("strong", {}, "Some of this is left out by its licence."),
      h("ul", {}, p.cuts.map((c) => h("li", {}, c))),
      rest.length ? button("Keep the rest to print later", (e) => {
        const list = loadWaiting();
        list.push({ title: `${req.title} (the rest)`, sources: rest.map((r) => r.source), options: req.options ?? {}, saved: today() });
        saveWaiting(list);
        (e.target as HTMLButtonElement).disabled = true;
        flash("Kept. It is listed under Waiting to print.");
      }) : null));
  }
  for (const w of p.warnings) out.push(h("p", { class: "hint" }, w));
  return out;
}

// waitingList shows parts kept to print another day, if any.
function waitingList(dlg: HTMLDialogElement): HTMLElement | null {
  const list = loadWaiting();
  if (!list.length) return null;
  const box = h("details", { class: "more" }, h("summary", {}, `Waiting to print (${list.length})`));
  const render = () => {
    const items = loadWaiting();
    clear(box, h("summary", {}, `Waiting to print (${items.length})`), h("ul", { class: "plain" }, items.map((w: Waiting, i) =>
      h("li", { class: "row between" }, h("span", {}, w.title, " ", h("span", { class: "hint" }, `kept ${w.saved}`)),
        h("span", { class: "row" },
          button("Open", () => {
            dlg.close();
            saveWaiting(items.filter((_, j) => j !== i));
            openExport({ title: w.title, sources: w.sources, options: w.options });
          }),
          button("Remove", () => {
            saveWaiting(items.filter((_, j) => j !== i));
            render();
          }, { "aria-label": `Remove ${w.title}` }))))));
  };
  render();
  return box;
}

// printPage puts the page in the print area and opens the system print dialog. Everything else on
// screen is hidden while printing (style.css, body.printing).
function printPage(p: ExportPreview) {
  let root = document.getElementById("print-root");
  if (!root) {
    root = h("div", { id: "print-root" });
    document.body.append(root);
  }
  const css = h("style", { media: "print" }, p.css);
  root.innerHTML = p.body; // escaped by the core
  root.prepend(css);
  document.body.classList.add("printing");
  const done = () => {
    document.body.classList.remove("printing");
    root?.replaceChildren();
    window.removeEventListener("afterprint", done);
  };
  window.addEventListener("afterprint", done);
  window.print();
}
