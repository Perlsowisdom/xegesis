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
  writeFileSync(join(dir, "e2e-scroll-pages.pdf"), pdf(Array.from({ length: 40 }, (_, i) => `Scroll page ${i + 1}: grace.`)));
  const res = await t.app.run("library.add", { paths: [dir] });
  t.assert.ok(res.added >= 3, `books added: ${JSON.stringify(res)}`);
  const find = async (text) => {
    for (let i = 0; i < 40; i++) {
      const r = await t.app.run("library.items", { text, limit: 5 });
      const it = r.items.find((x) => x.status === "online");
      if (it) return it;
      await t.page.waitForTimeout(250);
    }
    throw new Error(`"${text}" is not in the library`);
  };
  t.app.books = { epub: await find(EPUB_TITLE), pdf: await find("e2e-test-pages"), scrollPdf: await find("e2e-scroll-pages") };
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
  await page.evaluate(() => { location.hash = "#reader"; localStorage.removeItem("xegesis.book.layout"); });
  await page.waitForTimeout(200);
  for (let i = 0; i < 12; i++) {
    const tab = page.locator(".grid-tab-btn").filter({ hasText: /E2E Test Book|e2e-test-pages|e2e-scroll-pages/ }).first();
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

// chooseLayout presses Pages or Scroll in the Aa popover (Read mode).
async function chooseLayout(t, which) {
  const { page } = t;
  await page.evaluate(() => { location.hash = "#reader"; });
  if (!(await page.locator(".aa-btn:visible").count())) await page.locator(".topbar button", { hasText: /^Read$/ }).first().click();
  await page.locator(".aa-btn:visible").first().click();
  await page.getByRole("radiogroup", { name: "Books" }).locator("label", { hasText: which }).click();
  await page.keyboard.press("Escape");
  await page.waitForTimeout(200);
}

const place = (t) => ({ innerText: () => t.page.evaluate(() => [...document.querySelectorAll(".book-where")].map((e) => e.textContent).join(" | ")) });

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
  {
    row: "Scroll mode: an EPUB flows into the next chapter, and back to Pages",
    shot: "bk-05-epub-scroll.png",
    async run(t) {
      const { page } = t;
      await addBooks(t);
      await chooseLayout(t, "Scroll");
      await openFromLibrary(t, EPUB_TITLE, "Read");
      const book = visibleBook(t);
      await book.locator(".book-sec[data-sec='1'] p", { hasText: "The first words of chapter 1" }).waitFor({ timeout: 15000 });
      t.assert.equal(await book.locator(".book-sec[data-sec='2']").count(), 0, "chapter 2 is not loaded yet");
      const body = book.locator(".book-body");
      await body.hover();
      for (let i = 0; i < 12 && !(await book.locator(".book-sec[data-sec='2']").count()); i++) {
        await page.mouse.wheel(0, 1200);
        await page.waitForTimeout(350);
      }
      t.assert.equal(await book.locator(".book-sec[data-sec='2']").count(), 1, "chapter 2 loaded below chapter 1");
      t.assert.equal(await book.locator(".book-sec[data-sec='1']").count(), 1, "chapter 1 is still there");
      // Space moves by a screen.
      await body.focus();
      const before = await body.evaluate((e) => e.scrollTop);
      await page.keyboard.press("Space");
      await page.waitForTimeout(500);
      t.assert.ok(await body.evaluate((e, b) => e.scrollTop > b + 100, before), "Space scrolls a screen");
      // Scroll into chapter 2: the place follows.
      await body.evaluate((e) => { const s = e.querySelector(".book-sec[data-sec='2']"); e.scrollTop += s.getBoundingClientRect().top - e.getBoundingClientRect().top + 60; });
      await page.waitForTimeout(500);
      t.assert.ok(/Section 2 of/.test(await place(t).innerText()), `the place says ${await place(t).innerText()}`);
      const sel = await selectWords(t, book.locator(".book-sec[data-sec='2'] .book-text p, .book-sec[data-sec='2'] p").first());
      t.assert.ok(sel.trim().length > 5, `selected "${sel}" in chapter 2`);
      await page.locator(".mark-bar").waitFor({ timeout: 3000 });
      await t.shot();
      await chooseLayout(t, "Pages");
      await page.waitForTimeout(1000);
      await book.locator(".book-sec[data-sec='2']").waitFor({ timeout: 10000 });
      t.assert.equal(await book.locator(".book-sec").count(), 1, "one chapter at a time again");
    },
  },
  {
    row: "Scroll mode: a PDF stacks its pages, draws only those near, and goes back to Pages",
    shot: "bk-06-pdf-scroll.png",
    async run(t) {
      const { page } = t;
      await addBooks(t);
      await chooseLayout(t, "Scroll");
      await openFromLibrary(t, "e2e-scroll-pages", "Read");
      const book = visibleBook(t);
      await book.locator(".pdf-slot .textLayer span", { hasText: "Scroll page 1" }).first().waitFor({ timeout: 20000 });
      t.assert.equal(await book.locator(".pdf-slot").count(), 40, "a slot for every page");
      t.assert.ok(/Page 1 of 40/.test(await place(t).innerText()), "starts on page 1");
      const body = book.locator(".book-body");
      // Let the pages near the start finish drawing, so a late one cannot move the view after the jump.
      await book.locator(".pdf-slot[data-page='1'] canvas").waitFor({ timeout: 10000 });
      await page.waitForTimeout(500);
      const drawnCount = () => book.locator(".pdf-slot canvas").count();
      t.assert.ok((await drawnCount()) < 10, `only near pages are drawn (${await drawnCount()})`);
      await body.evaluate((e) => { e.scrollTop = e.querySelector(".pdf-slot[data-page='20']").offsetTop; });
      await page.waitForTimeout(1500);
      const txt = await place(t).innerText();
      t.assert.ok(/Page (19|20|21) of 40/.test(txt), `the indicator follows the scroll: ${txt}`);
      await book.locator(".pdf-slot[data-page='20'] canvas").waitFor({ timeout: 10000 });
      t.assert.equal(await book.locator(".pdf-slot[data-page='1'] canvas").count(), 0, "page 1 was freed");
      t.assert.ok((await drawnCount()) < 12, `far pages stay freed (${await drawnCount()})`);
      await t.shot();
      await chooseLayout(t, "Pages");
      await book.locator(".pdf-box .pdf-page").first().waitFor({ timeout: 10000 });
      t.assert.equal(await book.locator(".pdf-slot").count(), 0, "no stack of pages in Pages mode");
      t.assert.equal(await book.locator(".pdf-page").count(), 1, "one page at a time");
    },
  },
  {
    row: "Scroll mode: Go to page in the Contents popover lands on that page of a PDF",
    shot: "bk-07-pdf-scroll-goto.png",
    async run(t) {
      const { page } = t;
      await addBooks(t);
      await chooseLayout(t, "Scroll");
      await openFromLibrary(t, "e2e-scroll-pages", "Read");
      const book = visibleBook(t);
      await book.locator(".pdf-slot .textLayer span", { hasText: "Scroll page 1" }).first().waitFor({ timeout: 20000 });
      await page.locator(".grid-contents:visible").first().click();
      const input = page.locator(".contents-pop input:visible").first();
      await input.fill("25");
      await input.press("Enter");
      await book.locator(".pdf-slot[data-page='25'] canvas").waitFor({ timeout: 10000 });
      await page.waitForTimeout(800);
      const txt = await place(t).innerText();
      t.assert.ok(/Page 25 of 40/.test(txt), `the place says ${txt}`);
      const top = await book.locator(".book-body").evaluate((e) => {
        const s = e.querySelector(".pdf-slot[data-page='25']");
        return s.getBoundingClientRect().top - e.getBoundingClientRect().top;
      });
      t.assert.ok(Math.abs(top) < 150, `page 25 is at the top of the view (${Math.round(top)}px)`);
      t.assert.equal(await book.locator(".pdf-slot[data-page='1'] canvas").count(), 0, "page 1 stays freed");
      await t.shot();
      // And back to a page above.
      await page.locator(".grid-contents:visible").first().click();
      const again = page.locator(".contents-pop input:visible").first();
      await again.fill("3");
      await again.press("Enter");
      await book.locator(".pdf-slot[data-page='3'] canvas").waitFor({ timeout: 10000 });
      await page.waitForTimeout(800);
      t.assert.ok(/Page 3 of 40/.test(await place(t).innerText()), "went back to page 3");
    },
  },
  {
    row: "Scroll mode: a link inside an EPUB to another chapter goes there",
    shot: "bk-08-epub-scroll-link.png",
    async run(t) {
      const { page } = t;
      await addBooks(t);
      await chooseLayout(t, "Scroll");
      await openFromLibrary(t, EPUB_TITLE, "Read");
      const book = visibleBook(t);
      const link = book.locator(".book-sec[data-sec='1'] a[data-section='2']").first();
      await link.waitFor({ timeout: 15000 });
      await link.click();
      await page.waitForTimeout(1200);
      t.assert.ok(/Section 2 of/.test(await place(t).innerText()), `the place says ${await place(t).innerText()}`);
      const head = book.locator(".book-sec[data-sec='2'] h2").first();
      await head.waitFor({ timeout: 10000 });
      const info = await book.locator(".book-body").evaluate((e) => {
        const s = e.querySelector(".book-sec[data-sec='2']");
        return { top: Math.round(s.getBoundingClientRect().top - e.getBoundingClientRect().top), scrollTop: Math.round(e.scrollTop), height: e.clientHeight,
          secs: [...e.querySelectorAll(".book-sec")].map((x) => x.dataset.sec) };
      });
      t.assert.ok(info.top > -20 && info.top < 300, `the Chapter Two heading is at the top of the view: ${JSON.stringify(info)}`);
      await t.shot();
    },
  },
  {
    row: "Scroll mode: a book in a Study pane flows into the next chapter and follows the place",
    shot: "bk-09-study-scroll.png",
    async run(t) {
      const { page } = t;
      await addBooks(t);
      await chooseLayout(t, "Scroll");
      await openFromLibrary(t, EPUB_TITLE, "Open in Study");
      const pane = page.locator(".grid-pane:visible", { has: page.locator(".doc-book") }).first();
      await pane.locator(".book-sec[data-sec='1'] p", { hasText: "The first words of chapter 1" }).waitFor({ timeout: 15000 });
      const body = pane.locator(".book-body");
      await body.hover();
      for (let i = 0; i < 12 && !(await pane.locator(".book-sec[data-sec='2']").count()); i++) {
        await page.mouse.wheel(0, 1200);
        await page.waitForTimeout(350);
      }
      t.assert.equal(await pane.locator(".book-sec[data-sec='2']").count(), 1, "chapter 2 loaded below chapter 1");
      await body.evaluate((e) => { const s = e.querySelector(".book-sec[data-sec='2']"); e.scrollTop += s.getBoundingClientRect().top - e.getBoundingClientRect().top + 60; });
      await page.waitForTimeout(600);
      t.assert.ok(/Section 2 of/.test(await pane.locator(".book-where").innerText().catch(() => "")) || /Section 2 of/.test(await place(t).innerText()), "the place follows");
      await t.shot();
    },
  },
  {
    row: "Book view is uncluttered: Aa holds Pages/Scroll and PDF zoom, Highlights lists this section, Contents lists chapters",
    shot: "bk-10-book-clutter.png",
    async run(t) {
      const { page } = t;
      const { epub: it } = await addBooks(t);
      await t.app.run("book.set_position", { item_id: it.id, section: 1, para: 0 });
      await chooseLayout(t, "Pages");
      await openFromLibrary(t, EPUB_TITLE, "Read");
      const book = visibleBook(t);
      await book.locator(".book-text p", { hasText: "The first words of chapter 1" }).waitFor({ timeout: 15000 });
      t.assert.equal(await page.locator(".book-settings, .book-marks-box, select.book-toc").count(), 0, "no View fold-out, marks bar or chapter dropdown");
      // The Aa popover has what the View fold-out had.
      await page.locator(".aa-btn:visible").first().click();
      await page.getByRole("radiogroup", { name: "Books" }).waitFor();
      await page.getByRole("group", { name: "PDF zoom" }).waitFor();
      await page.keyboard.press("Escape");
      // Contents popover lists the chapters.
      await page.locator(".grid-contents:visible").first().click();
      await page.locator(".contents-pop button", { hasText: "Chapter Two" }).first().waitFor();
      await page.keyboard.press("Escape");
      // Highlights and a note by section show in the sidebar tool.
      const loc = (section, para) => ({ section, para, offset: 0, end_para: para, end_offset: 12, text: `Paragraph ${para}` });
      await t.app.run("highlight.create", { item_id: it.id, locator: loc(1, 3), style: "yellow" });
      await t.app.run("highlight.create", { item_id: it.id, locator: loc(2, 4), style: "green" });
      await t.app.run("note.create", { title: "Section one note", body: "about grace", item_id: it.id, locator: loc(1, 5) });
      const openedSidebar = !(await page.locator("#rightbar").isVisible().catch(() => false));
      if (openedSidebar) await page.locator("button[aria-label='Right sidebar']").click();
      await page.locator("#rightbar [role=tab]", { hasText: "Highlights" }).click();
      const rows = page.locator("#rightbar .hl-row");
      await page.locator("#rightbar .hl-row", { hasText: "Paragraph 3" }).first().waitFor({ timeout: 8000 });
      t.assert.equal(await rows.filter({ hasText: "Paragraph 4" }).count(), 0, "section 2's highlight is not listed in section 1");
      t.assert.ok(await rows.filter({ hasText: "Section one note" }).count() >= 1, "the section's note is listed");
      await t.shot();
      await page.locator("#rightbar .side-scope").click();
      await page.locator("#rightbar .hl-row", { hasText: "Paragraph 4" }).first().waitFor({ timeout: 8000 });
      if (openedSidebar) await page.locator("button[aria-label='Right sidebar']").click();
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
