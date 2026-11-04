// docs/editor-behaviour.md, "Markdown as you type": each row typed into a note in the real app.

const fresh = async (t) => {
  await t.openNote("");
  await t.focusEnd();
};

export default [
  {
    row: "Type `# ` at the start of a line: a large heading (H1)",
    shot: "md-01-h1.png",
    async run(t) {
      await fresh(t);
      await t.type("# Large heading");
      const [b] = await t.blocks();
      t.assert.deepEqual([b.tag, b.text], ["h1", "Large heading"]);
      t.assert.equal(await t.saved(), "# Large heading");
    },
  },
  {
    row: "Type `## `: a medium heading (H2)",
    shot: "md-02-h2.png",
    async run(t) {
      await fresh(t);
      await t.type("## Medium heading");
      const [b] = await t.blocks();
      t.assert.deepEqual([b.tag, b.text], ["h2", "Medium heading"]);
    },
  },
  {
    row: "Type `### `: a small heading (H3)",
    shot: "md-03-h3.png",
    async run(t) {
      await fresh(t);
      await t.type("### Small heading");
      const [b] = await t.blocks();
      t.assert.deepEqual([b.tag, b.text], ["h3", "Small heading"]);
    },
  },
  {
    row: "Type `**word**`: bold, and the stars disappear",
    shot: "md-04-bold.png",
    async run(t) {
      await fresh(t);
      await t.type("Grace is **free** today");
      t.assert.equal(await t.editor().locator("strong").first().textContent(), "free");
      t.assert.equal((await t.blocks())[0].text, "Grace is free today");
      t.assert.equal(await t.saved(), "Grace is **free** today");
    },
  },
  {
    row: "Type `*word*` or `_word_`: italic",
    shot: "md-05-italic.png",
    async run(t) {
      await fresh(t);
      await t.type("One *star* and _line_ done");
      const em = await t.editor().locator("em").allTextContents();
      t.assert.deepEqual(em, ["star", "line"]);
      t.assert.equal((await t.blocks())[0].text, "One star and line done");
    },
  },
  {
    row: "Type `` `code` ``: inline code",
    shot: "md-06-code.png",
    async run(t) {
      await fresh(t);
      await t.type("Run `verse.parse` now");
      t.assert.equal(await t.editor().locator("code").first().textContent(), "verse.parse");
      t.assert.equal((await t.blocks())[0].text, "Run verse.parse now");
    },
  },
  {
    row: "Type `- ` or `* `: a bulleted list",
    shot: "md-07-bullets.png",
    async run(t) {
      await fresh(t);
      await t.type("- dash item");
      await t.press("Enter");
      await t.press("Enter");
      await t.type("between");
      await t.press("Enter");
      await t.type("* star item");
      const bs = await t.blocks();
      t.assert.deepEqual(bs.map((b) => [b.tag, b.text]), [["ul", "dash item"], ["p", "between"], ["ul", "star item"]]);
      t.assert.equal(await t.saved(), "- dash item\n\nbetween\n\n- star item");
    },
  },
  {
    row: "Type `1. `: a numbered list",
    shot: "md-08-numbers.png",
    async run(t) {
      await fresh(t);
      await t.type("1. first");
      await t.press("Enter");
      await t.type("second");
      const [b] = await t.blocks();
      t.assert.equal(b.tag, "ol");
      t.assert.deepEqual(await t.editor().locator("ol li").allTextContents(), ["first", "second"]);
      t.assert.equal(await t.saved(), "1. first\n2. second");
    },
  },
  {
    row: "Type `> `: a quote block",
    shot: "md-09-quote.png",
    async run(t) {
      await fresh(t);
      await t.type("> Quoted words");
      const [b] = await t.blocks();
      t.assert.deepEqual([b.tag, b.text], ["blockquote", "Quoted words"]);
    },
  },
  {
    row: "Type `[] `: a checkbox item",
    shot: "md-10-checkbox.png",
    async run(t) {
      await fresh(t);
      await t.type("[] Read Romans 8");
      await t.press("Enter");
      await t.type("Pray");
      const items = t.editor().locator("ul li[role=checkbox]");
      t.assert.equal(await items.count(), 2);
      t.assert.deepEqual(await items.allTextContents(), ["Read Romans 8", "Pray"]);
      // Clicking the box ticks it, and the tick is saved.
      const box = await items.first().boundingBox();
      await t.page.mouse.click(box.x + 6, box.y + box.height / 2);
      t.assert.equal(await items.first().getAttribute("aria-checked"), "true");
      t.assert.equal(await t.saved(), "- [x] Read Romans 8\n- [ ] Pray");
    },
  },
  {
    row: "Type `---` and press Enter: a divider line",
    shot: "md-11-divider.png",
    async run(t) {
      await fresh(t);
      await t.type("Above");
      await t.press("Enter");
      await t.type("---");
      await t.press("Enter");
      await t.type("Below");
      const bs = await t.blocks();
      t.assert.deepEqual(bs.map((b) => b.cls.includes("rich-divider") ? "hr" : b.text), ["Above", "hr", "Below"]);
      t.assert.equal(await t.saved(), "Above\n\n---\n\nBelow");
    },
  },
  {
    row: "Backspace at the start of a heading or list item: back to plain text",
    shot: "md-12-backspace.png",
    async run(t) {
      await fresh(t);
      await t.type("Intro");
      await t.press("Enter");
      await t.type("## Point");
      await t.press("Home");
      await t.press("Backspace");
      let bs = await t.blocks();
      t.assert.deepEqual(bs.map((b) => [b.tag, b.text]), [["p", "Intro"], ["p", "Point"]]);
      await t.press("End");
      await t.press("Enter");
      await t.type("- item");
      await t.press("Home");
      await t.press("Backspace");
      bs = await t.blocks();
      t.assert.deepEqual(bs.map((b) => [b.tag, b.text]), [["p", "Intro"], ["p", "Point"], ["p", "item"]]);
    },
  },
  {
    row: "Paste Markdown text: it arrives formatted",
    shot: "md-13-paste.png",
    async run(t) {
      await fresh(t);
      // A paste event carrying plain text, as copying from a text editor gives.
      await t.editor().evaluate((el) => {
        const dt = new DataTransfer();
        dt.setData("text/plain", "# Title\nSome **bold** and *italic* words.\n\n- one\n- two\n\n> A quote\n\n1. first\n2. second\n\n---\n\n- [ ] to do");
        el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true }));
      });
      await t.page.waitForTimeout(200);
      const bs = await t.blocks();
      const tags = bs.filter((b) => b.text || b.cls.includes("rich-divider")).map((b) => (b.cls.includes("rich-divider") ? "hr" : b.tag));
      t.assert.deepEqual(tags, ["h1", "p", "ul", "blockquote", "ol", "hr", "ul"]);
      t.assert.equal(await t.editor().locator("strong").first().textContent(), "bold");
      t.assert.ok(!(await t.html()).includes("**"));
    },
  },
];
