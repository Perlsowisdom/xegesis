// Settings: the API.Bible key (kept in the system keychain and never shown back), your data and backups,
// and app details.

import { Message, run } from "../api";
import { badge, button, clear, field, h } from "../dom";
import { dataSection } from "./data";

interface KeyStatus {
  key_set: boolean;
  valid?: boolean;
  message?: string;
  cached_items?: number;
  commercial_allowed?: boolean;
}

export function settingsView() {
  const msg = new Message();
  const status = h("div", { "aria-live": "polite" });
  const key = h("input", { type: "password", autocomplete: "off", spellcheck: "false", placeholder: "Paste your key" });
  const info = h("dl", { class: "info" });
  const data = dataSection();

  const form = h("form", { class: "card", "aria-labelledby": "apibible-h" },
    h("h2", { id: "apibible-h" }, "API.Bible"),
    h("p", { class: "hint" }, "Use your own free API.Bible key to read Bibles online. The key is stored in your system keychain and is never shown again."),
    status,
    field("New key", key),
    h("div", { class: "row wrap" },
      h("button", { type: "submit", class: "primary" }, "Save key"),
      button("Check key works", () => check(true)),
      button("Remove key", async () => {
        if (!confirm("Remove the API.Bible key and its cached text?")) return;
        if (await msg.attempt(() => run("apibible.clear_key"))) {
          msg.ok("Key removed.");
          check(false);
        }
      }, { class: "danger" })),
    msg.el);
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const k = key.value.trim();
    key.value = "";
    if (!k) return msg.info("Paste a key first.");
    if (await msg.attempt(() => run("apibible.set_key", { key: k }))) {
      msg.ok("Key saved to the keychain.");
      check(true);
    }
  });

  async function check(verify: boolean) {
    const s = await msg.attempt(() => run<KeyStatus>("apibible.status", verify ? { verify: true } : {}));
    if (!s) return;
    clear(status, h("p", {},
      s.key_set ? badge("Key set", "ok") : badge("No key", "warn"), " ",
      verify && s.key_set ? (s.valid ? badge("Works", "ok") : badge("Not accepted", "bad")) : null, " ",
      s.message && s.key_set ? h("span", { class: "hint" }, s.message) : null,
      s.cached_items ? h("span", { class: "hint" }, ` ${s.cached_items} passages cached.`) : null));
  }

  const el = h("div", { class: "view" },
    h("h1", {}, "Settings"),
    form,
    data.el,
    h("section", { class: "card", "aria-labelledby": "about-h" }, h("h2", { id: "about-h" }, "About"), info));

  async function show() {
    check(false);
    data.show();
    const a = await msg.attempt(() => run<{ version: string; schema_version: number; device: string }>("app.info"));
    if (a) clear(info, h("dt", {}, "Version"), h("dd", {}, a.version), h("dt", {}, "Library schema"), h("dd", {}, String(a.schema_version)),
      h("dt", {}, "This device"), h("dd", {}, h("code", {}, a.device)),
      h("dt", {}, "Licence"), h("dd", {}, "MIT OR Apache-2.0"));
  }
  return { el, show };
}
