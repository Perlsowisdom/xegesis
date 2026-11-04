// docs/editor-behaviour.md, "Headings and structure", in the real app.

const SECTIONS = "# Alpha\n\nAlpha text\n\n## Beta\n\nBeta text\n\n# Gamma\n\nGamma text";

const visible = (t, text) => t.editor().locator(":scope > *", { hasText: text }).first().isVisible();

async function foldAlpha(t) {
  const alpha = t.editor().locator("h1", { hasText: "Alpha" });
  await alpha.hover();
  const arrow = t.page.locator(`${t.root} .rt-fold.hover`);
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
      await t.page.waitForSelector(".note-page .rich-text h1");
      await t.page.waitForTimeout(200);
      t.assert.equal(await visible(t, "Alpha text"), false, "the fold is remembered");
      await t.page.locator(".note-page .rt-fold.closed").first().click();
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
      const strip = t.page.locator(".note-page .rt-strip");
      t.assert.equal(await strip.locator(".rt-strip-mark").count(), 3);
      const box = await strip.boundingBox();
      const text = await t.editor().boundingBox();
      t.assert.ok(box.x >= text.x + text.width, "the strip is at the right edge of the text");
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
    row: "Sermon outline column lists the headings with their levels (indent)",
    shot: "st-05-sermon-outline.png",
    async run(t) {
      await t.openSermon("# Big point\n\nText\n\n## Middle point\n\nText\n\n### Small point\n\nText\n\n# Second big", "Outline levels");
      const pts = t.page.locator(".outline-point");
      t.assert.equal(await pts.count(), 4);
      const pads = await pts.evaluateAll((els) => els.map((e) => parseFloat(getComputedStyle(e).paddingLeft)));
      t.assert.ok(pads[0] < pads[1] && pads[1] < pads[2] && pads[3] === pads[0], `indents ${pads}`);
      t.assert.deepEqual(await pts.locator(".outline-num").allTextContents(), ["1", "a", "i", "2"]);
    },
  },
];
