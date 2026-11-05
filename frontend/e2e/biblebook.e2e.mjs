// T38a item 3: a library book typed Bible is listed with the Bibles, marked as a book, and going to
// a reference opens it at that chapter, found from its contents. Changing its type removes it.

import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { bibleEpub } from "./fixtures.mjs";

const TITLE = "E2E Hebrew Bible";

async function addBook(t) {
  const dir = join(dirname(t.app.library), "biblebooks");
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, "e2e-hebrew-bible.epub"), bibleEpub(TITLE));
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
    row: "A library book typed Bible is in the Bibles chooser as a book and opens at the passage (Psalm 84), by its contents",
    shot: "bk3-01-chooser.png",
    async run(t) {
      const { page } = t;
      const it = await addBook(t);
      await t.app.run("facet.set", { item_ids: [it.id], type: "Bible" });
      const map = await t.app.run("book.bible_map", { item_id: it.id });
      t.assert.deepEqual(map.entries.map((e) => `${e.book}:${e.chapter}`), ["1:0", "1:1", "19:0", "19:83", "19:84", "19:85"], JSON.stringify(map.entries));

      await page.evaluate(() => { location.hash = "#passage/Ps.84"; });
      await page.locator(".bibles-btn:visible").first().click();
      const row = page.locator(".popover .bible-book", { hasText: TITLE });
      await row.waitFor({ timeout: 8000 });
      t.assert.equal(await row.locator(".book-tag").textContent(), "book");
      await t.shot();
      await row.click();
      const book = page.locator(".doc-book:visible").first();
      await book.locator(`[id$="ps84"]`).first().waitFor({ timeout: 10000 });
      await page.waitForTimeout(800);
      const where = await book.locator('[id$="ps84"]').first().evaluate((e) => {
        const body = e.closest(".doc-book").querySelector(".book-body");
        return { top: e.getBoundingClientRect().top - body.getBoundingClientRect().top, height: body.clientHeight, text: e.textContent };
      });
      t.assert.equal(where.text, "Psalm 84");
      t.assert.ok(where.top >= 0 && where.top < 60, `Psalm 84 is at the top of the text: ${JSON.stringify(where)}`);
      await t.shot("bk3-02-opened-at-psalm-84.png");
    },
  },
  {
    row: "Changing the book's type away from Bible removes it from the chooser",
    shot: "bk3-03-removed.png",
    async run(t) {
      const { page } = t;
      const r = await t.app.run("library.items", { text: TITLE, limit: 5 });
      await t.app.run("facet.set", { item_ids: [r.items[0].id], type: "Commentary (whole Bible)" });
      await page.evaluate(() => { location.hash = "#passage/Ps.84"; });
      await page.locator(".bibles-btn:visible").first().click();
      await page.locator(".popover .module-picker").waitFor();
      await page.waitForTimeout(600);
      t.assert.equal(await page.locator(".popover .bible-book").count(), 0);
      await t.shot();
    },
  },
];
