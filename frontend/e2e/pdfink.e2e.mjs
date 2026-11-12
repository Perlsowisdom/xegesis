// T37 stage 2: ink on PDF pages. Generated PDFs are added to the copied library; strokes are drawn with
// the real mouse over the page, kept as annotations through ink.page.set, and must stay pinned to the
// page box at every zoom, in Pages and Scroll mode, in Read and Study panes and after a restart.

import { addBooks, closeBooks, openFromLibrary } from "./book.e2e.mjs";

const visibleBook = (t) => t.app.page.locator(".doc-book:visible").first();
const near = (t, a, b, tol, what) => t.assert.ok(Math.abs(a - b) <= tol, `${what}: ${a} is not within ${tol} of ${b}`);

async function chooseLayout(t, which) {
  const page = t.app.page;
  await page.evaluate(() => { location.hash = "#reader"; });
  if (!(await page.locator(".aa-btn:visible").count())) await page.locator(".topbar button", { hasText: /^Read$/ }).first().click();
  await page.locator(".aa-btn:visible").first().click();
  await page.getByRole("radiogroup", { name: "Books" }).locator("label", { hasText: which }).click();
  await page.keyboard.press("Escape");
  await page.waitForTimeout(200);
}

async function zoom(t, label, times = 1) {
  const page = t.app.page;
  await page.locator(".aa-btn:visible").first().click();
  for (let i = 0; i < times; i++) await page.getByRole("button", { name: label }).click();
  await page.keyboard.press("Escape");
  await page.waitForTimeout(900);
}

async function paneMenu(t, item) {
  const page = t.app.page;
  await page.locator("[aria-label='Pane menu']:visible").first().click();
  await page.locator(".menu-item:visible", { hasText: item }).first().click();
  await page.waitForTimeout(150);
}

const onPage = (t, n = 0) => visibleBook(t).locator(".pdf-page").nth(n);

// draw drags the mouse through points given as fractions of the page box.
async function draw(t, pts, n = 0) {
  const page = t.app.page;
  const box = await onPage(t, n).boundingBox();
  const at = ([fx, fy]) => [box.x + fx * box.width, box.y + fy * box.height];
  const [x0, y0] = at(pts[0]);
  await page.mouse.move(x0, y0);
  await page.mouse.down();
  for (const p of pts.slice(1)) {
    const [x, y] = at(p);
    await page.mouse.move(x, y, { steps: 8 });
  }
  await page.mouse.up();
  await page.waitForTimeout(400); // the layer has handed the stroke over and it is saved
}

// inkBox is the box of one page's drawn strokes as fractions of the page box.
async function inkBox(t, n = 0) {
  return onPage(t, n).evaluate((pg) => {
    const p = pg.getBoundingClientRect();
    const paths = [...pg.querySelectorAll(".pdf-ink-svg path")];
    if (!paths.length) return null;
    let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
    for (const e of paths) {
      const r = e.getBoundingClientRect();
      x0 = Math.min(x0, r.left); y0 = Math.min(y0, r.top); x1 = Math.max(x1, r.right); y1 = Math.max(y1, r.bottom);
    }
    return { x0: (x0 - p.left) / p.width, y0: (y0 - p.top) / p.height, x1: (x1 - p.left) / p.width, y1: (y1 - p.top) / p.height, w: p.width, strokes: paths.length };
  });
}

// Kept in the top third of the page, which is on screen at the default zoom.
const stroke = [[0.2, 0.1], [0.35, 0.2], [0.5, 0.15], [0.7, 0.28]];
const shared = {};

const checks = [
  {
    row: "PDF ink: Write on this PDF, draw with the mouse, and the stroke is saved against the page",
    shot: "pdfink-01-draw.png",
    async run(t) {
      const page = t.app.page;
      const books = await addBooks(t);
      shared.pdf = books.pdf;
      await chooseLayout(t, "Pages");
      await openFromLibrary(t, "e2e-test-pages", "Read");
      await onPage(t).waitFor({ timeout: 20000 });
      await visibleBook(t).locator(".pdf-box .textLayer span", { hasText: "Page one" }).first().waitFor({ timeout: 20000 });
      t.assert.equal(await visibleBook(t).locator(".pdf-ink-bar:visible").count(), 0, "no tool strip before writing");
      await paneMenu(t, "Write on this PDF");
      await visibleBook(t).locator(".pdf-ink-bar").waitFor({ state: "visible" });
      await draw(t, stroke);
      const saved = await t.app.run("ink.page.get", { item_id: shared.pdf.id, page: 1 });
      t.assert.equal(saved.ink.strokes.length, 1, "one stroke saved");
      const s = saved.ink.strokes[0];
      t.assert.equal(s.t, "pen");
      near(t, s.p[0][0], 0.2, 0.01, "first x as a fraction of the page");
      near(t, s.p[0][1], 0.1, 0.01, "first y as a fraction of the page");
      t.assert.ok(s.p.every(([x, y]) => x >= 0 && x <= 1 && y >= 0 && y <= 1), "points are fractions");
      const b = await inkBox(t);
      t.assert.equal(b.strokes, 1);
      near(t, b.x0, 0.2, 0.02, "drawn left edge");
      near(t, b.x1, 0.7, 0.02, "drawn right edge");
      near(t, b.y0, 0.1, 0.02, "drawn top edge");
      near(t, b.y1, 0.28, 0.02, "drawn bottom edge");
      // a second stroke with the highlighter in another colour
      await visibleBook(t).locator(".pdf-ink-bar [aria-label='Highlighter']").click();
      await visibleBook(t).locator(".pdf-ink-bar [aria-label='Green']").click();
      await draw(t, [[0.15, 0.22], [0.8, 0.22]]);
      const two = await t.app.run("ink.page.get", { item_id: shared.pdf.id, page: 1 });
      const kinds = JSON.stringify(two.ink.strokes.map((k) => [k.t, k.c]));
      t.assert.equal(kinds, JSON.stringify([["pen", "ink"], ["hl", "green"]]));
      // the list entry
      const list = await t.app.run("highlight.list", { item_id: shared.pdf.id });
      t.assert.equal(list.filter((h) => h.style === "ink").length, 1);
      shared.box = await inkBox(t);
      await t.shot();
    },
  },
  {
    row: "PDF ink: strokes stay aligned with the page at every zoom",
    shot: "pdfink-02-zoom.png",
    async run(t) {
      const before = await inkBox(t);
      const widths = [before.w];
      for (const [label, times] of [["Zoom in", 3], ["Zoom out", 6]]) {
        await zoom(t, label, times);
        await onPage(t).waitFor();
        const b = await inkBox(t);
        t.assert.ok(b, "the ink is drawn again after the zoom");
        widths.push(b.w);
        for (const k of ["x0", "y0", "x1", "y1"]) near(t, b[k], shared.box[k], 0.004, `${k} after zoom to width ${Math.round(b.w)}`);
        if (label === "Zoom in") await t.shot();
      }
      t.assert.ok(Math.max(...widths) - Math.min(...widths) > 100, `the page changed size: ${widths.map(Math.round)}`);
      await zoom(t, "Zoom in", 3);
    },
  },
  {
    row: "PDF ink: Scroll mode draws each page's ink in place, and writing works on a later page",
    shot: "pdfink-03-scroll.png",
    async run(t) {
      const page = t.app.page;
      const books = await addBooks(t);
      await closeBooks(t);
      await chooseLayout(t, "Scroll");
      await openFromLibrary(t, "e2e-scroll-pages", "Read");
      await visibleBook(t).locator(".pdf-slot[data-page='1'] canvas").waitFor({ timeout: 20000 });
      await paneMenu(t, "Write on this PDF");
      await draw(t, stroke, 0);
      const first = await inkBox(t, 0);
      near(t, first.x0, 0.2, 0.02, "page 1 left edge in Scroll mode");
      near(t, first.y1, 0.28, 0.02, "page 1 bottom edge in Scroll mode");
      // scroll to page 3 and write there
      await visibleBook(t).locator(".book-body").evaluate((e) => { e.scrollTop = e.querySelector(".pdf-slot[data-page='3']").offsetTop - 8; });
      await visibleBook(t).locator(".pdf-slot[data-page='3'] canvas").waitFor({ timeout: 10000 });
      const idx = await visibleBook(t).locator(".pdf-slot").evaluateAll((s) => s.findIndex((e) => e.dataset.page === "3"));
      await page.waitForTimeout(400);
      const box = await visibleBook(t).locator(".pdf-slot[data-page='3'] .pdf-page").boundingBox();
      await page.mouse.move(box.x + box.width * 0.3, box.y + box.height * 0.1);
      await page.mouse.down();
      await page.mouse.move(box.x + box.width * 0.6, box.y + box.height * 0.15, { steps: 6 });
      await page.mouse.up();
      await page.waitForTimeout(400);
      t.assert.ok(idx === 2, "page 3 is the third slot");
      const p3 = await t.app.run("ink.page.get", { item_id: books.scrollPdf.id, page: 3 });
      t.assert.equal((p3.ink.strokes ?? []).length, 1, "page 3 has its stroke");
      near(t, p3.ink.strokes[0].p[0][0], 0.3, 0.01, "page 3 first x");
      near(t, p3.ink.strokes[0].p[0][1], 0.1, 0.01, "page 3 first y");
      const p1 = await t.app.run("ink.page.get", { item_id: books.scrollPdf.id, page: 1 });
      t.assert.equal((p1.ink.strokes ?? []).length, 1, "page 1 kept its own");
      // zoom in Scroll mode: page 3's ink stays on the page
      const was = await visibleBook(t).locator(".pdf-slot[data-page='3'] .pdf-page").evaluate((pg) => {
        const p = pg.getBoundingClientRect(), r = pg.querySelector(".pdf-ink-svg path").getBoundingClientRect();
        return [(r.left - p.left) / p.width, (r.top - p.top) / p.height];
      });
      await zoom(t, "Zoom out", 2);
      await visibleBook(t).locator(".pdf-slot[data-page='3'] .pdf-page").waitFor({ timeout: 10000 }).catch(() => {});
      await visibleBook(t).locator(".book-body").evaluate((e) => { e.scrollTop = e.querySelector(".pdf-slot[data-page='3']").offsetTop - 8; });
      await visibleBook(t).locator(".pdf-slot[data-page='3'] .pdf-ink-svg path").waitFor({ timeout: 10000 });
      const now = await visibleBook(t).locator(".pdf-slot[data-page='3'] .pdf-page").evaluate((pg) => {
        const p = pg.getBoundingClientRect(), r = pg.querySelector(".pdf-ink-svg path").getBoundingClientRect();
        return [(r.left - p.left) / p.width, (r.top - p.top) / p.height];
      });
      near(t, now[0], was[0], 0.004, "page 3 ink x after zoom");
      near(t, now[1], was[1], 0.004, "page 3 ink y after zoom");
      await zoom(t, "Zoom in", 2);
      await t.shot();
    },
  },
];

export default checks.map((c) => ({ ...c, async run(t) {
  try {
    await c.run(t);
  } finally {
    if (c === checks[checks.length - 1]) {
      await closeBooks(t);
      await t.app.page.evaluate(() => localStorage.removeItem("xegesis.book.layout"));
    }
  }
} }));
