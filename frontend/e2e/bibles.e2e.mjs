// T38a item 1: a SWORD module .zip installs from the Bibles page's "Install from file" tab, a
// locked module in the same zip is refused, and the installed Bible opens at a passage.

import { execFileSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";

export default [
  {
    row: "Install a SWORD module .zip from the Bibles page, refuse its locked twin, then read a passage in it",
    shot: "bb-01-sword-install.png",
    async run(t) {
      const { page } = t;
      const zip = join(dirname(t.app.library), "sword-e2e.zip");
      execFileSync("go", ["run", "./internal/bible/sword/swordtest/gen", zip], { cwd: resolve(import.meta.dirname, "../.."), stdio: "pipe", timeout: 120000 });

      await page.evaluate(() => { location.hash = "#bibles"; });
      await page.getByRole("button", { name: "Install from file" }).click();
      const hint = await page.locator("form.card .hint").first().textContent();
      t.assert.match(hint, /SWORD/, "the dialog says SWORD zips are accepted");
      await page.locator("#install-path").fill(zip);
      await page.getByRole("button", { name: "Install", exact: true }).click();
      await page.locator(".msg.ok", { hasText: "Installed" }).waitFor({ timeout: 30000 });
      await page.getByText("Not installed", { exact: false }).first().waitFor({ timeout: 5000 }).catch(() => {});
      await page.locator("form.card summary", { hasText: "warning" }).click();
      t.assert.match(await page.locator("form.card details[open] li").first().textContent(), /locked/);
      await t.shot();

      const mods = await t.app.run("bible.modules", { kind: "all" });
      const m = mods.find((x) => x.id === "sworde2e");
      t.assert.ok(m, `installed modules: ${mods.map((x) => x.id)}`);
      t.assert.equal(m.licence.type, "personal_purchase", "a copyrighted module is a personal copy");
      t.assert.ok(!mods.some((x) => x.id === "lockede2e"), "the locked module is not installed");

      await page.evaluate(() => { location.hash = "#passage/John.3"; });
      await page.locator(".bibles-btn:visible").first().click();
      await page.locator(`.popover .chip-toggle:has(input[value="sworde2e"])`).click();
      await page.keyboard.press("Escape");
      const vt = page.locator(`.doc-bible .vt[data-tb="43003016"][data-module="sworde2e"]:visible`).first();
      await vt.waitFor({ timeout: 10000 });
      t.assert.match(await vt.textContent(), /gave his one and only Son/);
      await t.shot("bb-02-sword-passage.png");
    },
  },
];
