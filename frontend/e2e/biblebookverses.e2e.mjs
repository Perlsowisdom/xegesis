// T38c: a book typed Bible opens at the verse when its text has verse numbers, and a highlight made
// in it is linked to the verses, so the Bible's Highlights tool lists it.

import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { bibleEpub } from "./fixtures.mjs";

const TITLE = "E2E Verse Bible";

async function addBook(t) {
  const dir = join(dirname(t.app.library), "biblebookverses");
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, "e2e-verse-bible.epub"), bibleEpub(TITLE, true));
  await t.app.run("library.add", { paths: [dir] });
  for (let i = 0; i < 40; i++) {
    const r = await t.app.run("library.items", { text: TITLE, limit: 5 });
    const it = r.items.find((x) => x.status === "online");
    if (it) return it;
    await t.page.waitForTimeout(250);
  }
  throw new Error("the Bible book is not in the library");
}

export default [
  {
    row: "A Bible-type book with verse numbers opens at the verse (Psalm 84:3)",
    shot: "bk3v-01-verse.png",
    async run(t) {
      const { page } = t;
      const it = await addBook(t);
      await t.app.run("facet.set", { item_ids: [it.id], type: "Bible" });
      const at = await t.app.run("book.at_reference", { item_id: it.id, ref: "Psalm 84:3" });
      t.assert.equal(at.found, "verse", JSON.stringify(at));
      await page.evaluate(() => { location.hash = "#passage/Ps.84.3"; });
      await page.locator(".bibles-btn:visible").first().click();
      await page.locator(".popover .bible-book", { hasText: TITLE }).click();
      const book = page.locator(".doc-book:visible").first();
      const verse3 = book.locator('[id$="ps84"] ~ p', { hasText: "Verse line 2:" }).first();
      await verse3.waitFor({ timeout: 10000 });
      await page.waitForTimeout(1000);
      const top = await verse3.evaluate((e) => e.getBoundingClientRect().top - e.closest(".book-body").getBoundingClientRect().top);
      t.assert.ok(top >= 0 && top < 60, `verse 3 is at the top of the text: ${top}`);
      t.assert.equal((await verse3.locator("sup").textContent()).trim(), "3");
      await t.shot();
    },
  },
  {
    row: "A highlight made in a Bible-type book is linked to its verses and listed in the Highlights tool for them",
    shot: "bk3v-02-highlight-listed.png",
    async run(t) {
      const { page } = t;
      const book = page.locator(".doc-book:visible").first();
      const verse3 = book.locator('[id$="ps84"] ~ p', { hasText: "Verse line 2:" }).first();
      await verse3.scrollIntoViewIfNeeded();
      await page.waitForTimeout(500);
      const box = await verse3.boundingBox();
      const y = box.y + box.height / 2;
      await page.mouse.move(box.x + 60, y);
      await page.mouse.down();
      for (let i = 1; i <= 15; i++) await page.mouse.move(box.x + 60 + i * 14, y);
      await page.mouse.up();
      await page.locator(".mark-bar").waitFor({ timeout: 4000 });
      await page.locator('.mark-bar button[title="Highlight yellow"]').click();
      let list = [];
      for (let i = 0; i < 20 && !list.length; i++) {
        list = await t.app.run("highlight.list", { ref: "Psalm 84:3" });
        if (!list.length) await page.waitForTimeout(250);
      }
      t.assert.equal(list.length, 1, JSON.stringify(list));
      t.assert.equal(list[0].ref, "Psalms 84:3", JSON.stringify(list[0].locator));
      t.assert.ok(list[0].item_id && list[0].locator.para > 0, "keeps the book's own place");
      t.assert.equal((await t.app.run("highlight.list", { ref: "Psalm 84:3", on: "verses" })).length, 0);
      const note = await t.app.run("note.create", { body: "Linked", item_id: list[0].item_id, locator: list[0].locator });
      t.assert.equal(note.ref?.display, "Psalms 84:3");

      await page.evaluate(() => { location.hash = "#passage/Ps.84"; });
      await page.waitForSelector(".doc-bible .vt[data-tb]", { timeout: 10000 });
      const opened = !(await page.locator("#rightbar").isVisible().catch(() => false));
      if (opened) await page.keyboard.press("Control+.");
      await page.locator("#rightbar [role=tab]", { hasText: "Highlights" }).click();
      const row = page.locator("#rightbar .hl-row", { hasText: TITLE }).first();
      await row.waitFor({ timeout: 8000 });
      t.assert.ok((await row.textContent()).includes("Psalms 84:3"), await row.textContent());
      await t.shot();
      if (opened && await page.locator("#rightbar").isVisible()) {
        await page.keyboard.press("Control+.");
        await page.locator("#rightbar").waitFor({ state: "hidden", timeout: 5000 });
      }
    },
  },
  {
    row: "Insert → Verse text in a sermon offers the Bible-type book, marked book, and quotes from it",
    shot: "bk3v-03-insert-verse-text.png",
    async run(t) {
      const { page } = t;
      await t.openSermon("", "Verse text from a book");
      await t.focusEnd();
      await page.click(`${t.root} [data-tool=insert]`);
      await page.locator(".popover .insert-menu button", { hasText: "Verse text" }).click();
      await page.locator(".popover .quote-pop").waitFor();
      await page.locator(".popover .quote-pop input").first().fill("Psalm 84:3");
      await page.locator(".popover .bible-chip").click();
      const row = page.locator(".popover .bible-list button", { hasText: TITLE });
      await row.waitFor();
      t.assert.equal((await row.locator(".book-tag").textContent()).trim(), "book");
      await t.shot();
      await row.click();
      await page.waitForFunction(() => !document.querySelector(".popover .quote-pop button[type=submit]")?.disabled, null, { timeout: 8000 });
      t.assert.ok((await page.locator(".popover .quote-preview").textContent()).includes("Verse line 2"), await page.locator(".popover .quote-preview").textContent());
      await t.press("Enter");
      const q = t.editor().locator(".verse-quote");
      await q.waitFor();
      t.assert.ok((await q.locator(".vq-text").textContent()).includes("Verse line 2"), "the verse's words");
      t.assert.ok((await q.locator(".vq-credit").textContent()).includes(TITLE), "the credit line names the book");
      await t.shot("bk3v-03b-quoted.png");
    },
  },
  {
    row: "Search's Bible filter lists the Bible-type book, marked book, and finds words in it",
    shot: "bk3v-04-search.png",
    async run(t) {
      const { page } = t;
      await page.evaluate(() => { location.hash = "#search"; });
      const chip = page.locator(".module-picker .chip-toggle", { hasText: TITLE });
      await chip.waitFor({ timeout: 8000 });
      t.assert.ok((await chip.textContent()).includes("book"), await chip.textContent());
      await page.locator("#search-q").fill("translated");
      await page.locator('.view button[type=submit]', { hasText: "Search" }).first().click();
      const hit = page.locator(".results li", { hasText: TITLE }).first();
      await hit.waitFor({ timeout: 10000 });
      t.assert.equal((await hit.locator(".badge").first().textContent()).trim(), "book");
      await t.shot();
    },
  },
];
