// T23, updates in the real app: the first-run question asks once, plainly, and is off by default;
// Settings has an Updates section. Nothing here turns on a check that reaches the internet: the
// "Check for updates" button is never pressed, and the Go tests cover the checks against a local
// server.

export default [
  {
    row: "Updates: the first-run question is asked once and updates are off by default",
    shot: "t23-first-run.png",
    async run(t) {
      const { app, page } = t;
      // The test library starts with the question answered; ask it again, as on a first start.
      await app.run("update.settings.set", { asked: false });
      const before = await app.run("update.settings.get");
      t.assert.equal(before.enabled, false, "off by default");
      t.assert.equal(before.asked, false);
      await page.reload();
      const banner = page.locator("#update-banner");
      await banner.waitFor({ timeout: 20000 });
      const text = await banner.innerText();
      t.assert.ok(text.includes("Check for updates automatically?"), text);
      t.assert.ok(text.includes("nothing happens unless you say yes"), text);
      await t.shot();
      // "No thanks" answers it and leaves updates off.
      await banner.getByRole("button", { name: "No thanks" }).click();
      await banner.waitFor({ state: "detached" });
      const after = await app.run("update.settings.get");
      t.assert.equal(after.asked, true);
      t.assert.equal(after.enabled, false, "still off after No thanks");
      // It does not come back after a reload.
      await page.reload();
      await page.waitForSelector(".shell", { timeout: 20000 });
      await page.waitForTimeout(1500);
      t.assert.equal(await page.locator("#update-banner").count(), 0, "asked once");
    },
  },
  {
    row: "Updates: while off, the check and the install are refused",
    shot: "t23-refused.png",
    async run(t) {
      const { app } = t;
      for (const id of ["update.check", "update.install"]) {
        await t.assert.rejects(() => app.run(id), /updates are off/, `${id} while off`);
      }
      const auto = await app.run("update.auto");
      t.assert.equal(auto.ran, false);
    },
  },
  {
    row: "Updates: the Settings section shows the switch, the version and the last check",
    shot: "t23-settings.png",
    async run(t) {
      const { app, page } = t;
      await page.evaluate(() => { location.hash = "#settings/updates"; });
      const sec = page.locator(".settings-section:visible");
      await sec.getByRole("heading", { name: "Updates" }).waitFor();
      const sw = sec.getByRole("switch", { name: "Check for updates automatically" });
      t.assert.equal(await sw.isChecked(), false, "the switch is off by default");
      const check = sec.getByRole("button", { name: "Check for updates" });
      t.assert.equal(await check.isDisabled(), true, "no check while off");
      const status = await sec.locator(".update-status").innerText();
      t.assert.ok(status.includes("Not checked yet."), status);
      const info = await app.run("update.settings.get");
      t.assert.ok(status.includes(info.current), `the version ${info.current} is shown: ${status}`);
      await t.shot();
      // Turning it on enables the button; turning it off again leaves it as it was.
      await sw.click();
      await page.waitForFunction(() => !document.querySelector(".settings-section:not([hidden]) .check-updates")?.disabled);
      t.assert.equal((await app.run("update.settings.get")).enabled, true);
      await sec.getByRole("switch", { name: "Check for updates automatically" }).click();
      await page.waitForFunction(() => document.querySelector(".settings-section:not([hidden]) .check-updates")?.disabled);
      t.assert.equal((await app.run("update.settings.get")).enabled, false);
    },
  },
];
