// Going to one verse shows only that verse, with a button that shows the whole chapter and back.
export default [
  {
    row: "Bible: a single verse offers the whole chapter, and the way back",
    shot: "10-show-chapter.png",
    async run(t) {
      const { page } = t;
      await page.evaluate(() => { location.hash = "#reader"; });
      const input = page.locator(".place-input:visible").first();
      await input.waitFor({ timeout: 15000 });
      await input.fill("Romans 8:28");
      await input.press("Enter");
      const text = page.locator(".doc-bible:visible .bible-text").first();
      const verses = text.locator(".verse");
      await page.waitForFunction(() => document.querySelectorAll(".bible-text .verse").length === 1, null, { timeout: 15000 });
      t.assert.equal(await verses.count(), 1);
      const toggle = text.locator(".chapter-toggle");
      t.assert.equal((await toggle.textContent()).trim(), "Show all of Romans 8");
      await t.shot("10-show-chapter-verse.png");
      await toggle.focus();
      await page.keyboard.press("Enter");
      await page.waitForFunction(() => document.querySelectorAll(".doc-bible .bible-text .verse").length > 5);
      t.assert.equal(await text.locator(".verse.jumped").count(), 1);
      t.assert.ok((await text.locator(".verse.jumped").textContent()).includes("28"));
      t.assert.equal((await text.locator(".chapter-toggle").textContent()).trim(), "Show only Romans 8:28");
      await t.shot("10-show-chapter-all.png");
      await text.locator(".chapter-toggle").click();
      await page.waitForFunction(() => document.querySelectorAll(".doc-bible .bible-text .verse").length === 1);
      await input.fill("Romans 8");
      await input.press("Enter");
      await page.waitForFunction(() => document.querySelectorAll(".doc-bible .bible-text .verse").length > 5);
      t.assert.equal(await text.locator(".chapter-toggle").count(), 0);
    },
  },
];
