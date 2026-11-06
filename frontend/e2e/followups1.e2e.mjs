// Follow-up fixes (docs/tasks/followups.md, "Fixed in followups-1"), in the real app.

import { addBooks, closeBooks, openFromLibrary } from "./book.e2e.mjs";

const QUOTE = "> For God so loved the world.\n> — [John 3:16](#passage/John.3.16)\n\nAfter the quote";

async function moreMenu(page, item) {
  await page.locator(".reader .topbar button[aria-label=More]:visible").first().click();
  await page.locator(".popover .menu-list button", { hasText: item }).click();
}

export default [
  {
    row: "Verse quote: a click on its words puts the caret on the line after it, never inside",
    shot: "fu-01-quote-caret.png",
    async run(t) {
      await t.openNote(QUOTE, "Quote caret");
      const q = t.editor().locator(".verse-quote");
      await q.waitFor();
      await q.locator(".vq-text").click();
      const where = await t.page.evaluate(() => {
        const n = getSelection().anchorNode;
        const el = n && (n.nodeType === 1 ? n : n.parentElement);
        return { inQuote: !!el?.closest(".verse-quote"), inEditor: !!el?.closest(".rich-text") };
      });
      t.assert.equal(where.inQuote, false, "no caret inside the quote");
      t.assert.equal(where.inEditor, true, "the caret is in the text");
      await t.type("X");
      await t.shot();
      t.assert.match(await t.saved(), /\[John 3:16\]\(#passage\/John\.3\.16\)\n\nXAfter the quote/);
    },
  },
  {
    row: "Notes on this passage: lists the notes on the passage being read, not a note left open",
    shot: "fu-02-notes-here.png",
    async run(t) {
      const { page, app } = t;
      const a = await app.run("note.create", { title: "Romans eight note", body: "Eight", ref: "Rom 8:1" });
      await app.run("note.create", { title: "Romans twelve note", body: "Twelve", ref: "Rom 12:1" });
      await page.evaluate(() => { location.hash = "#passage/Rom.8"; });
      await page.locator(".reader .topbar button[aria-label=More]:visible").first().waitFor();
      const wasOpen = await page.locator("#rightbar").isVisible().catch(() => false);
      await moreMenu(page, "Notes on this passage");
      const side = "#rightbar .notes-panel";
      await page.locator(`${side} .side-note`, { hasText: "Romans eight note" }).click();
      await page.locator(`${side} .side-page:not([hidden]) .note-page`).waitFor();
      await page.evaluate(() => { location.hash = "#passage/Rom.12"; });
      await page.waitForTimeout(500);
      await moreMenu(page, "Notes on this passage");
      await page.locator(`${side} .side-list:not([hidden]) .side-note`, { hasText: "Romans twelve note" }).waitFor({ timeout: 8000 });
      t.assert.equal(await page.locator(`${side} .side-page:not([hidden])`).count(), 0, "no note left open");
      t.assert.equal(await page.locator(`${side} .side-note`, { hasText: "Romans eight note" }).count(), 0);
      await t.shot();
      // Leave the sidebar as it was: open, it folds the note toolbar's buttons away for later checks.
      await page.evaluate(() => { location.hash = "#passage/Rom.12"; });
      if (!wasOpen && await page.locator("#rightbar").isVisible()) {
        await page.keyboard.press("Control+.");
        await page.locator("#rightbar").waitFor({ state: "hidden", timeout: 5000 });
      }
      await app.run("note.delete", { id: a.id }).catch(() => {});
    },
  },
  {
    row: "Book highlight: a new one over an older one replaces the overlapped part (EPUB), never stacks colours",
    shot: "fu-03-book-overlap.png",
    async run(t) {
      const { page, app } = t;
      try {
        const { epub: it } = await addBooks(t);
        await app.run("book.set_position", { item_id: it.id, section: 1, para: 0 });
        await openFromLibrary(t, "E2E Test Book", "Read");
        const p = page.locator(".doc-book:visible .book-text p", { hasText: "Paragraph 3 of this chapter" }).first();
        await p.waitFor({ timeout: 15000 });
        const text = await p.evaluate((e) => e.textContent);
        // The paragraph's number is the one the book marks it with (a marker inside it).
        const para = Number(await p.evaluate((e) => (e.querySelector(".bp") ?? e.closest("[data-p]") ?? e).dataset.p));
        t.assert.ok(para > 0, "the paragraph has a number");
        await app.run("highlight.create", { item_id: it.id, style: "green", locator: { section: 1, para, offset: 0, end_para: para, end_offset: 24, text: text.slice(0, 24) } });
        await page.evaluate(() => document.dispatchEvent(new Event("xegesis:marks-changed")));
        await p.locator("mark.hl").first().waitFor({ timeout: 8000 });
        // Drag over characters 12 to 40 of the paragraph: the end of the green words and beyond.
        await p.scrollIntoViewIfNeeded();
        const at = (n) => p.evaluate((el, n) => {
          const w = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
          let seen = 0;
          for (let t = w.nextNode(); t; t = w.nextNode()) {
            if (n < seen + t.length) {
              const r = document.createRange();
              r.setStart(t, n - seen);
              r.setEnd(t, n - seen + 1);
              const b = r.getBoundingClientRect();
              return { x: b.left + 1, y: b.top + b.height / 2 };
            }
            seen += t.length;
          }
        }, n);
        const from = await at(12);
        const to = await at(40);
        for (let i = 0; i < 3; i++) {
          await page.mouse.move(from.x, from.y);
          await page.mouse.down();
          await page.mouse.move((from.x + to.x) / 2, to.y, { steps: 4 });
          await page.mouse.move(to.x, to.y, { steps: 6 });
          await page.mouse.up();
          if (await page.locator(".mark-bar").waitFor({ timeout: 3000 }).then(() => true, () => false)) break;
          await page.mouse.click(5, 300);
        }
        await page.locator('.mark-bar button[aria-label="Highlight pink"]').click();
        await page.waitForFunction(() => document.querySelector(".doc-book mark.hl-pink"), null, { timeout: 8000 });
        const list = (await app.run("highlight.list", { item_id: it.id })).filter((h) => h.locator.section === 1 && h.locator.para === para);
        list.sort((a, b) => (a.locator.offset ?? 0) - (b.locator.offset ?? 0));
        t.assert.deepEqual(list.map((h) => h.style), ["green", "pink"], JSON.stringify(list.map((h) => [h.style, h.locator.offset, h.locator.end_offset])));
        t.assert.equal(list[0].locator.offset ?? 0, 0);
        t.assert.ok(list[0].locator.end_offset <= list[1].locator.offset, "the green words stop where the pink ones start");
        t.assert.ok(list[0].locator.end_offset >= 11 && list[1].locator.end_offset >= 38, "both keep their words");
        t.assert.equal(await page.locator(".doc-book mark.hl mark.hl").count(), 0, "no mark inside another");
        const colours = await p.evaluate((e) => [...e.querySelectorAll("mark.hl")].map((m) => m.className.replace(/hl /, "").trim()));
        t.assert.ok(colours.every((c) => /hl-(green|pink)/.test(c)) && colours.some((c) => c.includes("pink")) && colours.some((c) => c.includes("green")), colours.join());
        await page.mouse.move(5, 5);
        await t.shot();
      } finally {
        await closeBooks(t);
      }
    },
  },
];
