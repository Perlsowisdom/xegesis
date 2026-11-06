// docs/editor-behaviour.md, "Headings and structure", in the real app.

const SECTIONS = "# Alpha\n\nAlpha text\n\n## Beta\n\nBeta text\n\n# Gamma\n\nGamma text";

const visible = (t, text) => t.editor().locator(":scope > *", { hasText: text }).first().isVisible();

async function foldAlpha(t) {
  const alpha = t.editor().locator("h1", { hasText: "Alpha" });
  await t.still(alpha);
  await alpha.hover();
  const arrow = t.page.locator(`${t.root} .rt-fold.hover`);
  await arrow.waitFor({ timeout: 5000 });
  t.assert.equal(await arrow.count(), 1, "an arrow shows beside the heading under the pointer");
  await arrow.click();
  t.assert.equal(await visible(t, "Alpha text"), false);
  t.assert.equal(await visible(t, "Beta"), false);
  t.assert.equal(await visible(t, "Beta text"), false);
  t.assert.equal(await visible(t, "Gamma text"), true);
  t.assert.equal(await alpha.isVisible(), true);
}

export default [
  {
    row: "Heading button offers Large, Medium and Small, with Ctrl+Alt+1/2/3",
    shot: "st-01-heading-sizes.png",
    async run(t) {
      await t.openSermon("A point", "Heading sizes");
      await t.focusEnd();
      await t.page.click(`${t.root} [data-tool=heading]`);
      const menu = t.page.locator(".popover .heading-menu");
      t.assert.deepEqual(await menu.locator(".hm-name").allTextContents(), ["Large heading", "Medium heading", "Small heading", "Normal text"]);
      await t.shot();
      await menu.locator("button", { hasText: "Small heading" }).click();
      t.assert.equal((await t.blocks())[0].tag, "h3");
      for (const [key, tag] of [["1", "h1"], ["2", "h2"], ["3", "h3"]]) {
        await t.press(`Control+Alt+${key}`);
        t.assert.equal((await t.blocks())[0].tag, tag, `Ctrl+Alt+${key}`);
      }
      t.assert.equal(await t.saved(), "### A point");
    },
  },
  {
    row: "Headings fold: an arrow beside each heading hides its section (notes), remembered",
    shot: "st-02-fold-note.png",
    async run(t) {
      const n = await t.openNote(SECTIONS, "Folding");
      await foldAlpha(t);
      await t.shot();
      // Display only: the text is unchanged.
      t.assert.equal((await t.app.run("note.get", { id: n.id })).body, SECTIONS);
      // Leave and come back: still folded.
      await t.page.evaluate(() => { location.hash = "#reader"; });
      await t.page.waitForTimeout(400);
      await t.page.evaluate((id) => { location.hash = `#notes/note/${id}`; }, n.id);
      await t.current(".note-page");
      t.assert.equal(await visible(t, "Alpha text"), false, "the fold is remembered");
      await t.page.locator("[data-e2e=current] .rt-fold.closed").first().click();
      t.assert.equal(await visible(t, "Alpha text"), true);
    },
  },
  {
    row: "Headings fold in sermons too",
    shot: "st-03-fold-sermon.png",
    async run(t) {
      await t.openSermon(SECTIONS, "Folding sermon");
      await foldAlpha(t);
    },
  },
  {
    row: "Outline strip at the right edge: a mark per heading, names on hover, click jumps",
    shot: "st-04-strip.png",
    async run(t) {
      const filler = Array.from({ length: 30 }, (_, i) => `Line ${i + 1} of the opening section.`).join("\n\n");
      await t.openNote(`# Alpha\n\n${filler}\n\n## Beta\n\n${filler}\n\n# Gamma\n\nGamma text`, "Strip");
      const strip = t.page.locator("[data-e2e=current] .rt-strip");
      t.assert.equal(await strip.locator(".rt-strip-mark").count(), 3);
      const box = await strip.boundingBox();
      const text = await t.editor().boundingBox();
      t.assert.ok(box.x >= text.x + text.width, "the strip is at the right edge of the text");
      await t.still(strip);
      // Point at the first mark (the names then cover the marks, so move the mouse directly).
      await strip.locator(".rt-strip-mark").first().hover({ force: true });
      const names = strip.locator(".rt-strip-names");
      await names.waitFor({ state: "visible", timeout: 3000 }).catch(async (e) => {
        throw new Error(`${e.message}\nhovered: ${await t.page.evaluate(() => [...document.querySelectorAll(":hover")].map((x) => x.className).join(" > "))}`);
      });
      t.assert.deepEqual(await names.locator(".rt-strip-name").allTextContents(), ["Alpha", "Beta", "Gamma"]);
      await t.shot();
      await names.locator(".rt-strip-name", { hasText: "Gamma" }).click();
      await t.page.waitForTimeout(700);
      const gamma = t.editor().locator("h1", { hasText: "Gamma" });
      const inView = await gamma.evaluate((el) => { const r = el.getBoundingClientRect(); return r.top >= 0 && r.bottom <= innerHeight; });
      t.assert.ok(inView, "Gamma is scrolled into view");
      const caretIn = await gamma.evaluate((el) => el.contains(getSelection().anchorNode));
      t.assert.ok(caretIn, "the caret is in Gamma");
    },
  },
  {
    row: "Sermons have no left outline column; the right-edge strip is their outline",
    shot: "st-05-sermon-strip.png",
    async run(t) {
      await t.openSermon("# Big point\n\nText\n\n## Middle point\n\nText\n\n### Small point\n\nText\n\n# Second big", "Outline levels");
      t.assert.equal(await t.page.locator(".outline-col, .outline-point").count(), 0, "no left outline column");
      const strip = t.page.locator("[data-e2e=current] .rt-strip");
      t.assert.equal(await strip.locator(".rt-strip-mark").count(), 4);
      const box = await strip.boundingBox();
      const text = await t.editor().boundingBox();
      t.assert.ok(box.x >= text.x + text.width, "the strip is at the right edge");
      const page = await t.page.locator("[data-e2e=current] .sermon-page").boundingBox();
      const layout = await t.page.locator("[data-e2e=current]").boundingBox();
      t.assert.ok(page.x - layout.x < 40, "the page starts at the left, with no column beside it");
    },
  },
  {
    row: "A folded heading stays folded when its words are edited",
    shot: "st-06-fold-rename.png",
    async run(t) {
      await t.openNote(SECTIONS, "Fold rename");
      await foldAlpha(t);
      // Edit the folded heading: click at its end and type more words.
      await t.editor().locator("h1", { hasText: "Alpha" }).click();
      await t.press("End");
      await t.type(" renamed");
      await t.page.waitForTimeout(300);
      t.assert.equal(await t.editor().locator("h1", { hasText: "Alpha renamed" }).count(), 1);
      t.assert.equal(await visible(t, "Alpha text"), false, "still folded after the rename");
      t.assert.equal(await visible(t, "Gamma text"), true);
      t.assert.equal(await t.page.locator(`${t.root} .rt-fold.closed`).count(), 1);
      await t.shot();
    },
  },
];
