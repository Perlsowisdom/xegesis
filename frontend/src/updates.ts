// Updates for the direct download. Off until the user says yes (ADR 0009): the first time, a small
// notice asks once, plainly. When a newer version is known, another notice offers "Restart to
// update". The Microsoft Store build has no updater, so the commands are missing and nothing shows.

import { Message, onEvent, run } from "./api";
import { button, h } from "./dom";
import { UpdateInfo } from "./updatewords";

let banner: HTMLElement | null = null;

function show(label: string, ...children: (Node | string)[]) {
  hide();
  banner = h("div", { class: "update-banner", role: "region", "aria-label": label, id: "update-banner" }, ...children);
  document.body.append(banner);
}

function hide() {
  banner?.remove();
  banner = null;
}

// restartToUpdate downloads, verifies and starts the installer; the app quits when it is running.
export async function restartToUpdate(msg: Message): Promise<void> {
  try {
    await run("update.install");
    msg.ok("Updating. Xegesis will close and open again in a moment.");
  } catch (e) {
    msg.error(e);
  }
}

function offer(version: string) {
  const msg = new Message();
  show("Update ready",
    h("p", {}, h("strong", {}, `Xegesis ${version} is ready.`), " It installs in a moment and opens again; your library is not touched."),
    h("div", { class: "row" },
      button("Restart to update", () => void restartToUpdate(msg), { class: "primary" }),
      button("Later", hide)),
    msg.el);
}

function ask() {
  show("Check for updates",
    h("p", {}, h("strong", {}, "Check for updates automatically?")),
    h("p", { class: "hint" }, "Xegesis can look at its public releases page on GitHub once a day and tell you when a new version is ready. It sends nothing but that web request, and nothing happens unless you say yes. You can change this in Settings, under Updates."),
    h("div", { class: "row" },
      button("Yes, check for updates", () => void answer(true), { class: "primary" }),
      button("No thanks", () => void answer(false))));
}

async function answer(on: boolean) {
  hide();
  try {
    await run("update.settings.set", { enabled: on });
  } catch {
    // The question will come back at the next start.
  }
}

// startUpdates asks the first-run question if it was not answered, and listens for a newer version.
export async function startUpdates(): Promise<void> {
  onEvent("update:available", (r) => {
    const v = (r as { latest?: string } | undefined)?.latest;
    if (v) offer(v);
  });
  try {
    const info = await run<UpdateInfo>("update.settings.get");
    if (!info.asked) ask();
    else if (info.enabled && info.latest && info.latest !== info.current) {
      // A version found earlier and not yet installed is offered again at start.
      const check = await run<{ newer: boolean; latest?: string }>("update.auto");
      if (check.newer && check.latest) offer(check.latest);
    }
  } catch {
    // No update commands: the Store build, or no library open.
  }
}
