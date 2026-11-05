// T34g section 7: library books open again. A generated EPUB (and a PDF) is added to the copied
// library, opened in Read mode and in a Study pane, scrolled, and its text selected.

import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { epub, pdf } from "./fixtures.mjs";

const EPUB_TITLE = "E2E Test Book";

// addBooks puts the fixtures in a folder beside the copied library and adds that folder.
async function addBooks(t) {
  if (t.app.books) return t.app.books;
  const dir = join(dirname(t.app.library), "books");
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, "e2e-test-book.epub"), epub(EPUB_TITLE));
  writeFileSync(join(dir, "e2e-test-pages.pdf"), pdf());
  const res = await t.app.run("library.add", { paths: [dir] });
  t.assert.ok(res.added >= 2, `books added: ${JSON.stringify(res)}`);
  const find = async (text) => {
    for (let i = 0; i < 40; i++) {
      const r = await t.app.run("library.items", { text, limit: 5 });
      const it = r.items.find((x) => x.status === "online");
      if (it) return it;
      await t.page.waitForTimeout(250);
    }
    throw new Error(`"${text}" is not in the library`);
  };
  t.app.books = { epub: await find(EPUB_TITLE), pdf: await find("e2e-test-pages") };
  return t.app.books;
}

// openFromLibrary finds a book in the Library and presses its Read or Open in Study button.
async function openFromLibrary(t, title, button) {
  const { page } = t;
  await page.evaluate(() => { location.hash = "#library"; });
  const q = page.locator("input.work-search:visible").first();
  await q.waitFor();
  await q.fill(title);
  const row = page.locator(`.book-tile:visible, .lib-table tbody tr:visible`).filter({ hasText: title }).first();
  await row.waitFor({ timeout: 15000 });
  await row.click();
  await page.locator(`button:visible`, { hasText: button }).first().click();
}

// closeBooks closes the test books' tabs and goes back to Study mode, so later checks start clean.
async function closeBooks(t) {
  const { page } = t;
  await page.evaluate(() => { location.hash = "#reader"; });
  await page.waitForTimeout(200);
  for (let i = 0; i < 12; i++) {
    const tab = page.locator(".grid-tab-btn").filter({ hasText: /E2E Test Book|e2e-test-pages/ }).first();
    if (!(await tab.count())) break;
    await tab.dispatchEvent("auxclick", { button: 1 });
    await page.waitForTimeout(100);
  }
  const study = page.locator("button", { hasText: /^Study$/ }).first();
  if (await study.count()) await study.click().catch(() => {});
  await page.waitForTimeout(200);
}

const visibleBook = (t) => t.page.locator(".doc-book:visible").first();

async function selectWords(t, el) {
  // A drag sent over the DevTools connection is sometimes lost by WebView2 right after a scroll;
  // a reader would simply drag again, and so does the test (once).
  const first = await dragOver(t, el);
  if (first.trim().length > 10) return first;
  const again = await dragOver(t, el);
  if (again.trim().length > 10) return again;
  // Last, a triple-click, which selects the paragraph with the mouse too.
  await el.click({ clickCount: 3 });
  await t.page.waitForTimeout(150);
  return t.page.evaluate(() => getSelection().toString());
}

async function dragOver(t, el) {
  // Drag along the paragraph's first line with the mouse, at a reader's pace.
  await el.scrollIntoViewIfNeeded();
  await t.page.waitForTimeout(200);
  const box = await el.boundingBox();
  const { mouse } = t.page;
  await mouse.move(box.x + 30, box.y + 10);
  await t.page.waitForTimeout(100);
  await mouse.down();
  await t.page.waitForTimeout(150);
  for (let i = 1; i <= 20; i++) {
    await mouse.move(box.x + 30 + i * 14, box.y + 10);
    await t.page.waitForTimeout(40);
  }
  await mouse.up();
  await t.page.waitForTimeout(100);
  return t.page.evaluate(() => getSelection().toString());
}

const checks = [
  {
    row: "EPUB opens in Read mode, scrolls, and its text can be selected",
    shot: "bk-01-epub-read.png",
    async run(t) {
      const { page } = t;
      await addBooks(t);
      await openFromLibrary(t, EPUB_TITLE, "Read");
      const book = visibleBook(t);
      const first = book.locator(".book-text p", { hasText: "The first words of chapter 1" });
      await first.waitFor({ timeout: 15000 }).catch(async (e) => {
        throw new Error(`${e.message}\nshown: ${(await page.locator("#reader").innerText().catch(() => "")).slice(0, 400)}`);
      });
      t.assert.ok((await book.locator(".book-text p").count()) > 30, "the whole chapter is shown");
      const body = book.locator(".book-body");
      await body.hover();
      await page.mouse.wheel(0, 1500);
      await page.waitForTimeout(400);
      t.assert.ok(await body.evaluate((e) => e.scrollTop > 200), "the text scrolls");
      const para = book.locator(".book-text p", { hasText: "Paragraph 20 of" });
      const sel = await selectWords(t, para);
      t.assert.ok(sel.trim().length > 10 && "Paragraph 20 of this chapter speaks of grace".includes(sel.trim().slice(0, 12)), `selected "${sel}"`);
      await page.locator(".mark-bar").waitFor({ timeout: 3000 });
      await t.shot();
      // The selection toolbar goes with the page: it must not float over the next one.
      await page.evaluate(() => { location.hash = "#library"; });
      await page.waitForTimeout(300);
      t.assert.equal(await page.locator(".mark-bar").count(), 0, "the selection toolbar closed");
    },
  },
  {
    row: "EPUB opens in a Study pane, scrolls, and its text can be selected",
    shot: "bk-02-epub-study.png",
    async run(t) {
      const { page } = t;
      await addBooks(t);
      await openFromLibrary(t, EPUB_TITLE, "Open in Study");
      const pane = page.locator(".grid-pane:visible", { has: page.locator(".doc-book") }).first();
      await pane.locator(".book-text p", { hasText: "The first words of chapter 1" }).waitFor({ timeout: 15000 });
      t.assert.ok((await page.locator(".grid-pane:visible").count()) >= 1);
      const body = pane.locator(".book-body");
      await body.hover();
      await page.mouse.wheel(0, 1200);
      await page.waitForTimeout(400);
      t.assert.ok(await body.evaluate((e) => e.scrollTop > 200), "the pane scrolls");
      const sel = await selectWords(t, pane.locator(".book-text p", { hasText: "Paragraph 18 of" }));
      t.assert.ok(sel.trim().length > 10, `selected "${sel}"`);
      await t.shot();
    },
  },
  {
    row: "A PDF still opens",
    shot: "bk-03-pdf.png",
    async run(t) {
      const { page } = t;
      await addBooks(t);
      await openFromLibrary(t, "e2e-test-pages", "Read");
      const book = visibleBook(t);
      await book.locator(".pdf-box .textLayer span", { hasText: "Page one" }).first().waitFor({ timeout: 20000 });
      t.assert.ok(await book.locator(".pdf-box canvas").count() >= 1, "the page is drawn");
      await t.shot();
    },
  },
  {
    row: "Reading the same EPUB again reuses its tab, and it opens again after a reload",
    shot: "bk-04-epub-reopen.png",
    async run(t) {
      const { page } = t;
      await addBooks(t);
      const tabs = () => page.evaluate(() => JSON.parse(localStorage.getItem("xegesis.grid") || "{}").columns
        .flatMap((c) => c.panes.flatMap((p) => p.tabs)).filter((x) => x.kind === "book" && x.title === "E2E Test Book").length);
      await openFromLibrary(t, EPUB_TITLE, "Read");
      await visibleBook(t).locator(".book-text p", { hasText: "The first words" }).waitFor();
      const before = await tabs();
      await openFromLibrary(t, EPUB_TITLE, "Read");
      await visibleBook(t).locator(".book-text p", { hasText: "The first words" }).waitFor();
      t.assert.equal(await tabs(), before, "no second tab for the same book");
      await page.reload();
      await page.waitForFunction(() => !!(window.go && window.go.main && window.go.main.App));
      await page.evaluate(() => { location.hash = "#reader"; });
      await visibleBook(t).locator(".book-text p", { hasText: "The first words" }).waitFor({ timeout: 15000 });
    },
  },
];

export default checks.map((c) => ({ ...c, async run(t) {
  try {
    await c.run(t);
  } finally {
    await closeBooks(t);
  }
} }));
