// docs/editor-behaviour.md, "Inserting Bible text, references and highlights", in the real app.

async function insertMenu(t, label) {
  await t.page.click(`${t.root} [data-tool=insert]`);
  const menu = t.page.locator(".popover .insert-menu");
  await menu.waitFor();
  await menu.locator("button", { hasText: label }).click();
}

// typeInPopover types into the popover that has just opened and waits for it to settle.
async function inPopover(t, text) {
  const input = t.page.locator(".popover input").first();
  await input.waitFor();
  await t.page.keyboard.type(text, { delay: 15 });
}

const chips = (t) => t.editor().locator(".ref-chip");

// drag drags with the mouse, as a person does: press on the source, move over the target, let go.
// readMode shows the Reader in Read mode (an earlier check may have left the Study grid on a note).
async function readMode(page, hash) {
  await page.evaluate((h) => { location.hash = h; }, hash);
  await page.locator(".shell button, header button", { hasText: /^Read$/ }).first().click();
  await page.waitForSelector(".verse .vnum", { timeout: 10000 });
}

async function drag(page, from, to) {
  await from.scrollIntoViewIfNeeded().catch(() => {});
  // Lists redraw as they load; wait until the source stays put.
  let a = null;
  for (let i = 0; i < 20 && !a; i++) {
    await page.waitForTimeout(250);
    a = await from.boundingBox().catch(() => null);
  }
  const b = await to.boundingBox();
  await page.mouse.move(a.x + a.width / 2, a.y + Math.min(a.height / 2, 12));
  await page.mouse.down();
  await page.mouse.move(a.x + a.width / 2 + 8, a.y + 8, { steps: 3 });
  await page.mouse.move(b.x + 30, b.y + 15, { steps: 8 });
  await page.mouse.up();
}

export default [
  {
    row: "One Insert control (book-and-plus icon) in notes and sermons; no sparkle; no duplicate Quote verses",
    shot: "in-01-insert-control.png",
    async run(t) {
      for (const open of [() => t.openNote("", "Insert control"), () => t.openSermon("", "Insert control sermon")]) {
        await open();
        const bar = t.page.locator(`${t.root} .rich-toolbar`);
        t.assert.equal(await bar.locator("[data-tool=insert]").count(), 1, "one Insert control");
        t.assert.equal(await bar.locator("[data-tool=insert] svg.icon path").count(), 3, "the book-and-plus icon");
        t.assert.equal(await bar.locator("[data-tool=verse], .verse-tool").count(), 0, "no separate verse or Quote verses buttons");
        t.assert.ok(!(await bar.textContent()).includes("✦"), "no sparkle in the toolbar");
        t.assert.ok(!(await bar.textContent()).includes("Quote verses"));
      }
      await t.page.click(`${t.root} [data-tool=insert]`);
      const names = await t.page.locator(".popover .insert-menu .im-name").allTextContents();
      t.assert.deepEqual(names, ["Verse reference", "Verse text", "Highlight", "Link to a note or sermon"]);
      await t.shot();
      await t.press("Escape");
    },
  },
  {
    row: "Verse reference: a linked chip that opens the passage in the Study grid",
    shot: "in-02-verse-ref.png",
    async run(t) {
      const n = await t.openNote("", "Verse chip");
      await t.focusEnd();
      await t.type("Read ");
      await insertMenu(t, "Verse reference");
      await inPopover(t, "Rom 8:28");
      await t.page.waitForSelector(".popover .ref-note.ok");
      await t.press("Enter");
      await chips(t).first().waitFor();
      t.assert.equal(await chips(t).first().getAttribute("data-href"), "#passage/Rom.8.28");
      t.assert.ok((await chips(t).first().textContent()).includes("Romans 8:28"));
      t.assert.ok((await t.editor().getAttribute("class")).includes("rich-text"));
      t.assert.equal(await t.saved(), "Read [Romans 8:28](#passage/Rom.8.28) ");
      await t.shot();
      // Reopened, it is still a chip.
      await t.page.evaluate(() => { location.hash = "#reader"; });
      await t.page.waitForTimeout(300);
      await t.page.evaluate((id) => { location.hash = `#notes/note/${id}`; }, n.id);
      await t.current(".note-page");
      await t.page.locator("[data-e2e=current] .rich-text .ref-chip").click();
      await t.page.waitForFunction(() => location.hash.startsWith("#reader") && !!document.querySelector(".study-grid, .grid-body"), null, { timeout: 8000 });
      await t.page.waitForTimeout(800);
      const places = await t.page.locator("input").evaluateAll((els) => els.filter((e) => e.offsetParent).map((e) => e.value));
      t.assert.ok(places.some((v) => /Romans 8/.test(v)), `the Study grid shows Romans 8 (place boxes: ${places})`);
      await t.shot("in-02b-verse-ref-opened.png");
    },
  },
  {
    row: "Verse text: quoted verses in a quote block with the Bible's credit line",
    shot: "in-03-verse-text.png",
    async run(t) {
      await t.openNote("", "Verse text");
      await t.focusEnd();
      await insertMenu(t, "Verse text");
      await t.page.locator(".popover input").first().fill("");
      await inPopover(t, "John 3:16");
      await t.page.waitForFunction(() => !document.querySelector(".popover .quote-pop button[type=submit]")?.disabled, null, { timeout: 8000 });
      await t.press("Enter");
      const q = t.editor().locator(".verse-quote");
      await q.waitFor();
      t.assert.ok((await q.locator(".vq-text").textContent()).includes("God"), "the verse's words");
      t.assert.ok((await q.locator(".vq-credit").textContent()).includes("John 3:16"), "the credit line");
      const body = await t.saved();
      t.assert.match(body, /^> .*\n> — \[John 3:16\]\(#passage\/John\.3\.16\)/s);
    },
  },
  {
    row: "Highlight: searchable picker with colours; inserts the words as a quote with place and link back",
    shot: "in-04-highlight.png",
    async run(t) {
      await t.app.run("highlight.create", { ref: "Romans 8:28", style: "green" });
      await t.app.run("highlight.create", { ref: "Psalm 23:1", style: "pink" });
      await t.openNote("", "Highlight insert");
      await t.focusEnd();
      await insertMenu(t, "Highlight");
      const rows = t.page.locator(".popover .pick-row");
      await rows.first().waitFor();
      t.assert.ok(await rows.count() >= 2);
      t.assert.ok(await t.page.locator(".popover .pick-row .mark-dot.dot-green").count() >= 1, "colours shown");
      await inPopover(t, "Romans");
      await t.page.waitForFunction(() => [...document.querySelectorAll(".popover .pick-row")].every((r) => r.textContent.includes("Romans")), null, { timeout: 5000 });
      await t.shot();
      await t.press("Enter");
      const q = t.editor().locator(".verse-quote");
      await q.waitFor();
      t.assert.ok((await q.locator(".vq-credit").textContent()).includes("Romans 8:28"));
      t.assert.match(await t.saved(), /— \[Romans 8:28\]\(#passage\/Rom\.8\.28\)/);
    },
  },
  {
    row: "Note link: a link to another note or sermon",
    shot: "in-05-note-link.png",
    async run(t) {
      const target = await t.app.run("note.create", { title: "Target note for linking", body: "Linked to." });
      const sermon = await t.app.run("document.create", { kind: "sermon" });
      await t.app.run("document.update", { id: sermon.id, title: "Linked sermon", kind: "sermon", body: "" });
      await t.openNote("", "Note link");
      await t.focusEnd();
      await insertMenu(t, "Link to a note or sermon");
      await inPopover(t, "Target note");
      await t.page.waitForFunction(() => document.querySelectorAll(".popover .pick-row").length === 1, null, { timeout: 5000 });
      await t.press("Enter");
      await chips(t).first().waitFor();
      await t.type("and ");
      await insertMenu(t, "Link to a note or sermon");
      await inPopover(t, "Linked sermon");
      await t.page.waitForFunction(() => document.querySelectorAll(".popover .pick-row").length === 1, null, { timeout: 5000 });
      await t.press("Enter");
      await t.page.waitForFunction(() => document.querySelectorAll("[data-e2e=current] .ref-chip").length === 2);
      t.assert.equal(await t.saved(), `[Target note for linking](#notes/note/${target.id}) and [Linked sermon](#sermons/${sermon.id}) `);
      await t.shot();
      await chips(t).first().click();
      await t.page.waitForFunction((id) => !!document.querySelector(".grid-body .note-page") && document.body.textContent.includes("Target note for linking"), target.id, { timeout: 8000 });
      await t.shot("in-05b-note-link-opened.png");
    },
  },
  {
    row: "The / menu: /verse, /quote, /highlight, /link",
    shot: "in-06-slash.png",
    async run(t) {
      await t.openNote("", "Slash inserts");
      await t.focusEnd();
      // The menu is alphabetical, so /quote and /link list the block quote and the web link first.
      for (const [cmd, title, down] of [["/verse", "Insert a verse reference", 0], ["/quote", "Insert verse text", 1], ["/highlight", "Insert a highlight", 0], ["/link", "Link to a note or sermon", 1]]) {
        await t.type(cmd);
        await t.page.waitForSelector(".slash-menu:not([hidden])");
        for (let i = 0; i < down; i++) await t.press("ArrowDown");
        await t.press("Enter");
        const pop = t.page.locator(".popover .pop-title");
        await pop.waitFor({ timeout: 5000 });
        t.assert.equal(await pop.textContent(), title, cmd);
        if (cmd === "/verse") {
          await inPopover(t, "Ps 23:1");
          await t.page.waitForSelector(".popover .ref-note.ok");
          await t.press("Enter");
          await chips(t).first().waitFor();
          continue;
        }
        await t.press("Escape");
        await t.editor().click();
        await t.press("Control+End");
      }
      t.assert.ok((await chips(t).first().textContent()).includes("Psalm"));
      t.assert.ok(!(await t.blocks()).some((b) => b.text.includes("/")), "the slash commands are not left in the text");
    },
  },
  {
    row: "Typing a reference such as `Rom 8:28` and pressing Tab turns it into a chip",
    shot: "in-07-tab.png",
    async run(t) {
      await t.openNote("", "Tab chip");
      await t.focusEnd();
      await t.type("Compare 1 John 3:16");
      await t.press("Tab");
      await chips(t).first().waitFor({ timeout: 5000 });
      t.assert.equal(await chips(t).first().getAttribute("data-href"), "#passage/1John.3.16");
      await t.type("and Rom 8:28");
      await t.press("Tab");
      await t.page.waitForFunction(() => document.querySelectorAll("[data-e2e=current] .ref-chip").length === 2);
      t.assert.equal(await t.saved(), "Compare [1 John 3:16](#passage/1John.3.16) and [Romans 8:28](#passage/Rom.8.28) ");
    },
  },
  {
    row: "A saved note that ends with a verse quote, a divider or a table opens with a line under it to write on (T19)",
    shot: "in-07b-line-after.png",
    async run(t) {
      const quote = "> Love must be\n> — [Romans 12:9](#passage/Rom.12.9) (ENGNET)";
      for (const [end, label] of [[quote, "verse quote"], ["---", "divider"], ["| A | B |\n| --- | --- |\n| 1 | 2 |", "table"]]) {
        // Opening it adds nothing to the saved text.
        await t.openNote(`Para.\n\n${end}`, `Ends with a ${label}, untouched`);
        t.assert.equal(await t.saved(), `Para.\n\n${end}`, label);
        await t.openNote(`Para.\n\n${end}`, `Ends with a ${label}`);
        await t.focusEnd();
        await t.type("Typed after.");
        const body = await t.saved();
        t.assert.ok(body.endsWith("\n\nTyped after."), `${label}: ${body}`);
      }
    },
  },
  {
    row: "Drag a verse from a Bible pane into a note: the verse is quoted",
    shot: "in-08-drag-verse.png",
    async run(t) {
      const { page } = t;
      // Read Romans 8, with the Notes tool open beside it and a new note there.
      await readMode(page, "#passage/Rom.8");
      if (!(await page.locator("#rightbar").isVisible().catch(() => false))) await page.keyboard.press("Control+.");
      await page.locator("#rightbar [role=tab]", { hasText: "Notes" }).click();
      await page.locator("#rightbar button", { hasText: /new note/i }).first().click();
      const ed = page.locator("#rightbar .rich-text");
      await ed.waitFor();
      t.root = "#rightbar";
      await drag(page, page.locator(".verse .vnum", { hasText: /^28$/ }).first(), ed);
      const q = ed.locator(".verse-quote");
      await q.waitFor({ timeout: 8000 });
      t.assert.ok((await q.locator(".vq-credit").textContent()).includes("Romans 8:28"));
    },
  },
  {
    row: "Drag a highlight from the Highlights tool into a note: it is quoted",
    shot: "in-09-drag-highlight.png",
    async run(t) {
      const { page } = t;
      await t.app.run("highlight.create", { ref: "Romans 8:31", style: "blue" });
      await readMode(page, "#passage/Rom.8");
      await t.openNote("", "Drag a highlight");
      if (!(await page.locator("#rightbar").isVisible().catch(() => false))) await page.keyboard.press("Control+.");
      await page.locator("#rightbar [role=tab]", { hasText: "Highlights" }).click();
      const row = page.locator("#rightbar .hl-row", { hasText: "Romans 8:31" }).first();
      await row.waitFor({ timeout: 8000 });
      await drag(page, row, t.editor());
      const q = t.editor().locator(".verse-quote");
      await q.waitFor({ timeout: 8000 });
      t.assert.ok((await q.locator(".vq-credit").textContent()).includes("Romans 8:31"));
    },
  },
  {
    row: "Drag a Related card into a note: its words are quoted with their place (or a chip if the licence forbids)",
    shot: "in-10-drag-related.png",
    async run(t) {
      const { page } = t;
      const r = await t.app.run("reference.for_passage", { ref: "Rom.8.28" });
      await page.evaluate(() => { location.hash = "#passage/Rom.8.28"; });
      await page.waitForSelector(".verse .vnum");
      await t.openNote("", "Drag a Related card");
      if (!(await page.locator("#rightbar").isVisible().catch(() => false))) await page.keyboard.press("Control+.");
      await page.locator("#rightbar [role=tab]", { hasText: "Related" }).click();
      if (!r || !r.commentaries?.length) {
        // The scratch library has no commentary, so the Related tool has no card to drag. The drop
        // itself is checked with the data a card carries (study.ts related()), for a commentary
        // whose licence is unknown: its words are not copied and a chip to the passage goes in.
        t.note = "no commentary in the scratch library: drop checked with a card's data, not a dragged card";
        await t.editor().evaluate((el) => {
          const dt = new DataTransfer();
          dt.setData("application/x-xegesis-related", JSON.stringify({ module: "NoSuchCommentary", name: "A commentary", ref: "Romans 8:28", osis: "Rom.8.28", text: "Comment." }));
          const r = el.getBoundingClientRect();
          const at = { clientX: r.left + 20, clientY: r.top + 10, bubbles: true, cancelable: true, dataTransfer: dt };
          el.dispatchEvent(new DragEvent("dragover", at));
          el.dispatchEvent(new DragEvent("drop", at));
        });
        const chip = t.editor().locator(".ref-chip");
        await chip.waitFor({ timeout: 8000 });
        t.assert.equal(await chip.getAttribute("data-href"), "#passage/Rom.8.28");
        return;
      }
      const card = page.locator("#rightbar details.commentary").first();
      await card.waitFor();
      await drag(page, card, t.editor());
      await t.page.waitForFunction(() => !!document.querySelector("[data-e2e=current] .rich-text .verse-quote, [data-e2e=current] .rich-text .ref-chip"), null, { timeout: 8000 });
    },
  },
  {
    row: "A plain click on a link opens it; clicking while text is selected places the caret instead",
    shot: "in-06-link-click.png",
    async run(t) {
      await t.openNote("Go to the [reader page](#reader) now", "Link click");
      const link = t.editor().locator("a.rt-link").first();
      await link.waitFor();
      // With text selected, a click on the link only places the caret.
      await t.editor().click({ position: { x: 4, y: 8 } });
      await t.press("Control+a");
      const here = await t.page.evaluate(() => location.hash);
      await t.editor().locator("a.rt-link").first().click();
      await t.page.waitForTimeout(400);
      t.assert.equal(await t.page.evaluate(() => location.hash), here, "stayed on the note");
      t.assert.ok(await t.page.evaluate(() => getSelection().isCollapsed), "the caret was placed");
      await t.shot();
      // A plain click follows it.
      await t.editor().locator("a.rt-link").first().click();
      await t.page.waitForFunction(() => location.hash.startsWith("#reader"), null, { timeout: 5000 });
    },
  },
  {
    row: "A plain click on an http(s) or mailto link in a note asks the system to open it, and the app stays put",
    shot: "in-07-external-link.png",
    async run(t) {
      await t.openNote("Visit [the site](https://example.com/xegesis-e2e) or [write](mailto:e2e@example.com) today", "External link");
      const links = t.editor().locator("a.rt-link");
      await links.first().waitFor();
      // Record the calls instead of opening a browser.
      await t.page.evaluate(() => {
        window.__opened = [];
        window.__windows = [];
        window.runtime.BrowserOpenURL = (u) => { window.__opened.push(u); };
        window.open = (u) => { window.__windows.push(String(u)); return null; };
      });
      const here = await t.page.evaluate(() => location.href);
      await links.nth(0).click();
      await links.nth(1).click();
      await t.page.waitForTimeout(300);
      t.assert.equal(JSON.stringify(await t.page.evaluate(() => window.__opened)), JSON.stringify(["https://example.com/xegesis-e2e", "mailto:e2e@example.com"]), "BrowserOpenURL got both links");
      t.assert.equal(await t.page.evaluate(() => window.__windows.length), 0, "no pop-up window was opened");
      t.assert.equal(await t.page.evaluate(() => location.href), here, "the app did not navigate");
      await t.shot();
    },
  },
];
