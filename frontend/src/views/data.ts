// Your data: export and import in open formats, backups now, and the automatic backup schedule.

import { Message, pick, run } from "../api";
import { button, clear, field, h, select } from "../dom";

const TYPES: [string, string][] = [
  ["note", "Notes"], ["highlight", "Highlights"], ["list", "Lists"], ["collection", "Collections"],
  ["reading_plan", "Reading plans"], ["document", "Documents and sermons"],
];

interface BackupInfo { path: string; bytes: number; scheduled?: boolean; time?: { $date: string } }
interface Schedule { every: string; keep: number; dir?: string }

function counts(c: Record<string, number> | undefined): string {
  const parts = Object.entries(c ?? {}).filter(([, n]) => n > 0).map(([t, n]) => `${n} ${t.replace("_", " ")}`);
  return parts.length ? parts.join(", ") : "nothing";
}

function size(bytes: number): string {
  return bytes > 1 << 20 ? `${(bytes / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(bytes / 1024))} KB`;
}

export function dataSection(): { el: HTMLElement; show: () => void } {
  const msg = new Message();
  const boxes = TYPES.map(([value, label]) => {
    const box = h("input", { type: "checkbox", value, checked: "" }) as HTMLInputElement;
    return h("label", { class: "check" }, box, " ", label);
  });
  const history = h("input", { type: "checkbox" }) as HTMLInputElement;
  const every = select([["off", "Off"], ["daily", "Daily"], ["weekly", "Weekly"]]);
  const keep = h("input", { type: "number", min: "1", max: "100", value: "7" }) as HTMLInputElement;
  const list = h("ul", { class: "plain" });

  const chosen = () => boxes.map((b) => b.querySelector("input") as HTMLInputElement).filter((b) => b.checked).map((b) => b.value);

  async function exportTo(kind: "folder" | "zip") {
    const dir = await msg.attempt(() => pick("folder", kind === "zip" ? "Folder for the .zip file" : "Folder for the export"));
    if (!dir) return;
    const path = kind === "zip" ? `${dir}/xegesis-export-${new Date().toISOString().slice(0, 10)}.zip` : dir;
    msg.info("Exporting…");
    const r = await msg.attempt(() => run<{ path: string; counts: Record<string, number> }>("export.data",
      { path, types: chosen(), history: history.checked }));
    if (r) msg.ok(`Exported ${counts(r.counts)} to ${r.path}.`);
  }

  async function importFrom(kind: "file" | "folder") {
    const path = await msg.attempt(() => pick(kind, kind === "file" ? "Choose an export .zip" : "Choose an export folder"));
    if (!path) return;
    if (!confirm("Merge this export into your library? Items with the same ID are replaced; every change is recorded in history.")) return;
    msg.info("Importing…");
    const r = await msg.attempt(() => run<{ created: Record<string, number>; updated: Record<string, number> }>("import.data", { path }));
    if (r) msg.ok(`Imported: created ${counts(r.created)}; updated ${counts(r.updated)}.`);
  }

  async function backupNow() {
    msg.info("Backing up…");
    const r = await msg.attempt(() => run<{ path: string; bytes: number }>("backup.create"));
    if (r) {
      msg.ok(`Backed up ${size(r.bytes)} to ${r.path}.`);
      refresh();
    }
  }

  async function saveSchedule() {
    const n = Number(keep.value);
    if (await msg.attempt(() => run("backup.settings.set", { every: every.value, keep: n >= 1 && n <= 100 ? n : 7 }))) {
      msg.ok(every.value === "off" ? "Automatic backups are off." : `Backing up ${every.value}, keeping ${keep.value} copies.`);
      await msg.attempt(() => run("backup.auto"));
      refresh();
    }
  }

  async function refresh() {
    const s = await msg.attempt(() => run<Schedule>("backup.settings.get"));
    if (s) {
      every.value = s.every || "off";
      if (s.keep) keep.value = String(s.keep);
    }
    const copies = await msg.attempt(() => run<BackupInfo[]>("backup.list"));
    if (!copies) return;
    clear(list, ...(copies.length ? copies.slice(0, 10).map((c) => h("li", {},
      h("code", {}, c.path), " ", h("span", { class: "hint" },
        `${size(c.bytes)}${c.time ? ", " + new Date(c.time.$date).toLocaleString() : ""}${c.scheduled ? ", scheduled" : ""}`)))
      : [h("li", { class: "hint" }, "No backups yet.")]));
  }

  const el = h("section", { class: "card", "aria-labelledby": "data-h" },
    h("h2", { id: "data-h" }, "Your data"),
    h("p", { class: "hint" }, "Export your notes, highlights, lists, collections, reading plans and documents as Markdown and JSON. The format follows any's encodings, so other tools can read it."),
    h("fieldset", {}, h("legend", {}, "What to export"), ...boxes,
      h("label", { class: "check" }, history, " Include the change history")),
    h("div", { class: "row wrap" },
      button("Export to a folder", () => exportTo("folder"), { class: "primary" }),
      button("Export as .zip", () => exportTo("zip")),
      button("Import a .zip", () => importFrom("file")),
      button("Import a folder", () => importFrom("folder"))),
    h("h3", {}, "Backups"),
    h("p", { class: "hint" }, "A backup is a complete copy of your library database, made safely while Xegesis runs."),
    h("div", { class: "row wrap" },
      button("Back up now", backupNow),
      field("Automatic backups", every),
      field("Copies to keep", keep),
      button("Save schedule", saveSchedule)),
    list,
    msg.el);

  return { el, show: refresh };
}
