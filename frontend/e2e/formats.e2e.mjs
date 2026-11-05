// T34g sections 4 and 5: every Markdown format is a toolbar button (with ⋯ More when the row does not
// fit), and the / menu lists every format and insert alphabetically.

const TOOLS = ["bold", "italic", "underline", "code", "|", "heading", "|", "bullets", "numbers", "checks", "|", "quote", "divider", "link", "table", "insert"];

// selectWord selects the last word typed, with the keyboard.
async function selectBack(t, n) {
  for (let i = 0; i < n; i++) await t.press("Shift+ArrowLeft");
}

const tool = (t, id) => t.page.click(`${t.root} .rich-toolbar [data-tool=${id}]`);

export default [
  {
    row: "Toolbar: bold, italic, underline, code | heading | three lists | quote, divider, link, table, Insert, in groups",
    shot: "fm-01-toolbar.png",
    async run(t) {
      await t.openNote("", "Buttons");
      const order = await t.page.locator(`${t.root} .rich-toolbar > :is(button[data-tool], .rich-sep)`).evaluateAll((els) =>
        els.filter((e) => e.dataset.tool !== "more").map((e) => (e.classList.contains("rich-sep") ? "|" : e.dataset.tool)));
      t.assert.deepEqual(order.slice(0, TOOLS.length), TOOLS, JSON.stringify(order));
      await t.focusEnd();
      for (const [id, sel] of [["bold", "strong, .rt-b"], ["italic", "em, .rt-i"], ["underline", ".rt-u"], ["code", "code, .rt-code"]]) {
        await t.type(` ${id}`);
        await selectBack(t, id.length);
        await tool(t, id);
        await t.press("End");
        await tool(t, id); // off again for what is typed next
        t.assert.equal(await t.editor().locator(sel).last().textContent(), id, id);
      }
      const md = await t.saved();
      t.assert.equal(md, " **bold** *italic* <u>underline</u> `code`", md);
      await t.press("Enter");
      await t.type("Point");
      await tool(t, "heading");
      await t.page.locator(".popover .heading-menu button", { hasText: "Medium heading" }).click();
      t.assert.equal((await t.blocks()).at(-1).tag, "h2");
      for (const [id, tag, cls] of [["bullets", "ul", ""], ["numbers", "ol", ""], ["checks", "ul", "rt-checklist"]]) {
        await t.press("End");
        await t.press("Enter");
        await t.type(id);
        await tool(t, id);
        const b = (await t.blocks()).at(-1);
        t.assert.equal(b.tag, tag, id);
        if (cls) t.assert.ok(b.cls.includes(cls), id);
        await tool(t, id);
      }
      await t.type(" quoted");
      await tool(t, "quote");
      t.assert.equal((await t.blocks()).at(-1).tag, "blockquote");
      await tool(t, "quote");
      await tool(t, "divider");
      t.assert.ok((await t.blocks()).some((b) => b.cls.includes("rich-divider") || b.tag === "hr"), "a divider");
      await t.type("site");
      await selectBack(t, 4);
      await tool(t, "link");
      await t.page.locator(".popover input[type=url]").fill("https://example.com");
      await t.press("Enter");
      t.assert.equal(await t.editor().locator("a.rt-link").last().getAttribute("href"), "https://example.com");
      await tool(t, "table");
      await t.page.locator(".popover .table-picker").waitFor();
      await t.press("Escape");
      await tool(t, "insert");
      await t.page.locator(".popover .insert-menu").waitFor();
      await t.shot();
      await t.press("Escape");
    },
  },
  {
    row: "Toolbar in a narrow pane: what does not fit goes into ⋯ More, never off-screen",
    shot: "fm-02-more.png",
    async run(t) {
      {
        await t.openNote("Narrow", "Narrow");
        // A narrow pane: the page is made as narrow as a Study pane beside two others.
        await t.page.evaluate((root) => { document.querySelector(`${root} .page-text`).style.width = "360px"; }, t.root);
        await t.page.waitForTimeout(300);
        const bar = t.page.locator(`${t.root} .rich-toolbar`);
        const fits = await bar.evaluate((e) => e.scrollWidth <= e.clientWidth + 1 && e.getBoundingClientRect().right <= innerWidth);
        t.assert.ok(fits, "the toolbar fits its pane");
        const more = bar.locator("[data-tool=more]");
        t.assert.ok(await more.isVisible(), "⋯ More shows");
        const hidden = await bar.locator("button[data-tool]:not([data-tool=more])").evaluateAll((els) => els.filter((e) => e.hidden).map((e) => e.dataset.tool));
        t.assert.ok(hidden.length > 0 && hidden.includes("insert"), `in More: ${hidden}`);
        await more.click();
        const items = await t.page.locator(".popover .more-menu button").evaluateAll((els) => els.map((e) => e.dataset.tool));
        t.assert.deepEqual(items, hidden);
        await t.shot();
        await t.page.locator(".popover .more-menu button[data-tool=table]").click();
        await t.page.locator(".popover .table-picker").waitFor();
        await t.press("Escape");
      }
    },
  },
  {
    row: "The / menu lists every format and insert alphabetically, and typing filters it",
    shot: "fm-03-slash.png",
    async run(t) {
      await t.openNote("", "Slash list");
      await t.focusEnd();
      await t.type("/");
      const menu = t.page.locator(".slash-menu:not([hidden])");
      await menu.waitFor();
      const names = await menu.locator(".slash-name").allTextContents();
      t.assert.deepEqual(names, ["Bulleted list", "Checkbox list", "Code", "Divider", "Heading large", "Heading medium", "Heading small",
        "Highlight", "Link", "Link to a note or sermon", "Numbered list", "Quote", "Table", "Verse reference", "Verse text"]);
      await t.shot();
      await t.type("he");
      t.assert.deepEqual(await menu.locator(".slash-name").allTextContents(), ["Heading large", "Heading medium", "Heading small"]);
      await t.press("ArrowDown");
      await t.press("ArrowDown");
      await t.press("Enter");
      await t.type("Small one");
      t.assert.equal((await t.blocks())[0].tag, "h3");
      for (const [q, want] of [["verse", ["Verse reference", "Verse text"]], ["quote", ["Quote", "Verse text"]], ["highlight", ["Highlight"]], ["link", ["Link", "Link to a note or sermon"]]]) {
        await t.press("End");
        await t.press("Enter");
        await t.type(`/${q}`);
        await menu.waitFor();
        t.assert.deepEqual(await menu.locator(".slash-name").allTextContents(), want, q);
        await t.press("Escape");
      }
    },
  },
];
