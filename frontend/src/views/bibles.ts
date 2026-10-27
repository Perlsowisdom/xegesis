// Bibles: the installed modules and installing a new one from a file or folder.

import { Message, Module, paramEnum, pick, run } from "../api";
import { button, clear, field, h, select } from "../dom";
import { licenceForm, typeLabel } from "../licence";

export function biblesView() {
  const msg = new Message();
  const table = h("div", { class: "table-wrap" });
  const install = installForm(() => show());

  const el = h("div", { class: "view" },
    h("h1", {}, "Bibles"),
    h("section", { "aria-labelledby": "installed-h" },
      h("h2", { id: "installed-h" }, "Installed"), msg.el, table),
    install.el);

  async function show() {
    const ms = await msg.attempt(() => run<Module[]>("bible.modules"));
    if (!ms) return;
    clear(table, ms.length === 0 ? h("p", { class: "empty" }, "No Bibles installed yet. Install one from an OSIS, USFM or USX file below.") :
      h("table", { class: "list" },
        h("thead", {}, h("tr", {}, ["Name", "Abbrev.", "Language", "Numbering", "Verses", "Licence", ""].map((t) => h("th", { scope: "col" }, t)))),
        h("tbody", {}, ms.map((m) => h("tr", {},
          h("td", {}, h("strong", {}, m.name || m.id), h("br"), h("small", { class: "hint" }, m.id)),
          h("td", {}, m.abbreviation),
          h("td", {}, m.language),
          h("td", {}, m.versification),
          h("td", { class: "num" }, (m.verse_count ?? 0).toLocaleString()),
          h("td", {}, typeLabel(m.licence?.type), m.licence?.spdx ? h("small", { class: "hint" }, ` ${m.licence.spdx}`) : null),
          h("td", {}, button("Remove", () => remove(m), { class: "danger", "aria-label": `Remove ${m.name || m.id}` })))))));
  }

  async function remove(m: Module) {
    if (!confirm(`Remove ${m.name || m.id}? Its module file is deleted; your notes are kept.`)) return;
    if ((await msg.attempt(() => run("bible.remove", { module: m.id }))) !== undefined) {
      msg.ok(`Removed ${m.name || m.id}.`);
      show();
    }
  }

  return { el, show };
}

function installForm(installed: () => void) {
  const msg = new Message();
  const path = h("input", { type: "text", required: true, spellcheck: "false", placeholder: "Choose a file or folder" });
  const format = select([["", "Detect from file"]], "");
  paramEnum("bible.install", "format").then((fs) => fs.forEach((f) => format.append(h("option", { value: f }, f.toUpperCase()))));
  const text = (placeholder = "") => h("input", { type: "text", placeholder, autocomplete: "off" });
  const fields = {
    id: text("e.g. web"), name: text("e.g. World English Bible"), abbreviation: text("e.g. WEB"),
    language: text("e.g. eng"), version: text(), versification: text("e.g. KJV"), source: text("Where it came from"),
  };
  const replace = h("input", { type: "checkbox" });
  const licence = licenceForm("Licence of this text");

  const browse = async (kind: "file" | "folder") => {
    const p = await msg.attempt(() => pick(kind, kind === "file" ? "Choose a Bible file" : "Choose a folder of Bible files"));
    if (p) path.value = p;
  };

  const form = h("form", { class: "card", "aria-labelledby": "install-h" },
    h("h2", { id: "install-h" }, "Install a Bible"),
    h("p", { class: "hint" }, "Import an OSIS, USFM or USX file, or a folder of USFM or USX books. State the licence so Xegesis knows what you may print, export and share."),
    h("div", { class: "field" },
      h("label", { for: "install-path" }, "File or folder"),
      h("div", { class: "row" }, Object.assign(path, { id: "install-path" }),
        button("Choose file…", () => browse("file")), button("Choose folder…", () => browse("folder")))),
    h("div", { class: "grid3" },
      field("Format", format),
      field("Module ID", fields.id, "Optional"),
      field("Name", fields.name, "Optional; read from the file if blank"),
      field("Abbreviation", fields.abbreviation),
      field("Language", fields.language, "ISO 639 code"),
      field("Version", fields.version),
      field("Verse numbering", fields.versification),
      field("Source", fields.source)),
    licence.el,
    h("label", { class: "check" }, replace, " Replace a module with the same ID"),
    h("div", { class: "row" }, h("button", { type: "submit", class: "primary" }, "Install")),
    msg.el);

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const l = licence.value();
    const params: Record<string, unknown> = { path: path.value.trim(), licence_type: l.type };
    if (format.value) params.format = format.value;
    for (const [k, input] of Object.entries(fields)) if (input.value.trim()) params[k] = input.value.trim();
    if (l.spdx) params.spdx = l.spdx;
    if (l.rights_holder) params.rights_holder = l.rights_holder;
    if (l.attribution) params.attribution = l.attribution;
    if (l.url) params.licence_url = l.url;
    if (replace.checked) params.replace = true;
    msg.info("Installing… large files can take a minute.");
    const r = await msg.attempt(() => run<{ module: Module; warnings: string[] }>("bible.install", params));
    if (!r) return;
    clear(msg.el, `Installed ${r.module.name || r.module.id} (${r.module.verse_count.toLocaleString()} verses).`,
      r.warnings?.length ? h("details", {}, h("summary", {}, `${r.warnings.length} warnings`), h("ul", {}, r.warnings.map((w) => h("li", {}, w)))) : null);
    msg.el.className = "msg ok";
    installed();
  });

  return { el: form };
}
