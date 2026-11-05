// T34g section 6: tables in notes and sermons, typed in the real app.

const table = (t) => t.editor().locator("table").first();
const shape = (t) => table(t).evaluate((tb) => [...tb.rows].map((r) => r.cells.length));
const cellText = (t) => table(t).evaluate((tb) => [...tb.rows].map((r) => [...r.cells].map((c) => c.textContent)));

// tableButton presses the toolbar's Table button, through ⋯ More when it is in there.
async function tableButton(t) {
  const b = t.page.locator(`${t.root} .rich-toolbar [data-tool=table]`);
  if (await b.isVisible()) return b.click();
  await t.page.click(`${t.root} .rich-toolbar [data-tool=more]`);
  await t.page.locator(".popover .more-menu button[data-tool=table]").click();
}

async function slash(t, cmd) {
  await t.type(cmd);
  await t.page.waitForSelector(".slash-menu:not([hidden])");
  await t.press("Enter");
  await table(t).waitFor();
}

export default [
  {
    row: "Table button: a size grid to point at (R × C label), up to 10 × 10",
    shot: "tb6-01-picker.png",
    async run(t) {
      await t.openNote("", "Table picker");
      await t.focusEnd();
      await tableButton(t);
      const picker = t.page.locator(".popover .table-picker");
      await picker.waitFor();
      t.assert.equal(await picker.locator(".tp-cell").count(), 100);
      await picker.locator('.tp-cell[data-r="3"][data-c="2"]').hover();
      t.assert.equal((await picker.locator(".tp-label").textContent()).trim(), "3 × 2");
      t.assert.equal(await picker.locator(".tp-cell.on").count(), 6);
      await t.shot();
      await picker.locator('.tp-cell[data-r="3"][data-c="2"]').click();
      t.assert.deepEqual(await shape(t), [2, 2, 2]);
      t.assert.equal(await table(t).locator("th").count(), 2, "a header row");
    },
  },
  {
    row: "/table makes 3 × 3; /table3x2 makes 3 rows by 2 columns; × works too; over 10 is capped",
    shot: "tb6-02-slash.png",
    async run(t) {
      await t.openNote("", "Table slash");
      await t.focusEnd();
      await slash(t, "/table");
      t.assert.deepEqual(await shape(t), [3, 3, 3]);
      for (const [cmd, want] of [["/table3x2", [2, 2, 2]], ["/table2×4", [4, 4]], ["/table12x30", Array(10).fill(10)]]) {
        await t.openNote("", `Table ${cmd}`);
        await t.focusEnd();
        await t.type(cmd);
        await t.page.waitForSelector(".slash-menu:not([hidden])");
        if (cmd === "/table3x2") {
          t.assert.equal((await t.page.locator(".slash-menu .slash-name").first().textContent()).trim(), "Table 3 × 2");
          await t.shot();
        }
        await t.press("Enter");
        await table(t).waitFor();
        t.assert.deepEqual(await shape(t), want, cmd);
      }
    },
  },
  {
    row: "In a cell: formats, Markdown shortcuts and chips work; Tab, Tab in the last cell adds a row, Shift+Tab goes back",
    shot: "tb6-03-cells.png",
    async run(t) {
      await t.openNote("", "Table cells");
      await t.focusEnd();
      await slash(t, "/table2x2");
      await t.type("Point");
      await t.press("Tab");
      await t.type("Verse");
      await t.press("Tab");
      await t.type("**Grace** and __hope__ ");
      await t.press("Control+i");
      await t.type("now");
      await t.press("Control+i");
      await t.press("Tab");
      await t.type("Rom 8:28");
      await t.press("Tab");
      await table(t).locator(".ref-chip").waitFor({ timeout: 5000 });
      t.assert.deepEqual(await shape(t), [2, 2], "Tab after a reference makes a chip, not a move");
      await t.press("Tab");
      t.assert.deepEqual(await shape(t), [2, 2, 2], "Tab in the last cell adds a row");
      await t.type("Third");
      await t.press("Shift+Tab");
      await t.type("x");
      const cells = await cellText(t);
      t.assert.equal(cells[1][0], "Grace and hope now");
      t.assert.ok(cells[1][1].includes("Romans 8:28"));
      t.assert.equal(cells[2][0], "Third", "the new row's first cell");
      t.assert.ok(cells[1][1].endsWith("x"), `Shift+Tab went back: ${JSON.stringify(cells)}`);
      t.assert.equal(await table(t).locator(".rt-b, strong").first().textContent(), "Grace");
      t.assert.equal(await table(t).locator(".rt-u").first().textContent(), "hope");
      t.assert.equal(await table(t).locator(".rt-i, em").first().textContent(), "now");
      const md = await t.saved();
      t.assert.ok(md.includes("| Point | Verse |\n| --- | --- |\n| **Grace** and <u>hope</u> *now* | [Romans 8:28](#passage/Rom.8.28)"), md);
      await t.shot();
    },
  },
  {
    row: "Right-click a cell: add rows and columns, delete them, toggle the header row, delete the table",
    shot: "tb6-04-menu.png",
    async run(t) {
      await t.openNote("| A | B |\n| --- | --- |\n| c | d |", "Table menu");
      const act = async (cell, id) => {
        await table(t).locator("td, th").filter({ hasText: cell }).first().click({ button: "right" });
        const menu = t.page.locator(".popover .table-menu");
        await menu.waitFor();
        if (id === "row-below") await t.shot();
        await menu.locator(`button[data-action=${id}]`).click();
        await t.page.waitForTimeout(150);
      };
      t.assert.deepEqual(await shape(t), [2, 2]);
      await act("c", "row-below");
      t.assert.deepEqual(await shape(t), [2, 2, 2]);
      await act("c", "row-above");
      t.assert.deepEqual(await shape(t), [2, 2, 2, 2]);
      await act("d", "col-right");
      t.assert.deepEqual(await shape(t), [3, 3, 3, 3]);
      await act("c", "col-left");
      t.assert.deepEqual(await shape(t), [4, 4, 4, 4]);
      await act("c", "del-col");
      t.assert.deepEqual(await shape(t), [3, 3, 3, 3]);
      t.assert.equal(await table(t).locator("td, th").filter({ hasText: /^c$/ }).count(), 0, "c's column went");
      await act("d", "del-row");
      t.assert.deepEqual(await shape(t), [3, 3, 3]);
      t.assert.equal(await table(t).locator("th").count(), 3);
      await act("B", "header");
      t.assert.equal(await table(t).locator("th").count(), 0, "header row off");
      const md = await t.saved();
      t.assert.ok(md.startsWith("<!-- table header=no -->\n|  | B |  |\n| --- | --- | --- |"), JSON.stringify(md));
      await act("B", "header");
      t.assert.equal(await table(t).locator("th").count(), 3, "header row on");
      // The toolbar's Table button offers the same menu inside a table.
      await table(t).locator("td").first().click();
      await tableButton(t);
      await t.page.locator(".popover .table-menu").waitFor();
      await t.press("Escape");
      await act("B", "del-table");
      t.assert.equal(await t.editor().locator("table").count(), 0);
    },
  },
  {
    row: "Columns resize by dragging their borders; widths are saved and read back (sermon)",
    shot: "tb6-05-resize.png",
    async run(t) {
      const s = await t.openSermon("Before\n\n| Name | Note |\n| --- | --- |\n| a | b |", "Table widths");
      const th = table(t).locator("th").first();
      const box = await th.boundingBox();
      const { mouse } = t.page;
      await mouse.move(box.x + box.width - 2, box.y + box.height / 2);
      await t.page.waitForTimeout(100);
      t.assert.ok(await t.editor().evaluate((e) => e.classList.contains("rt-col-resize")), "the border shows a resize cursor");
      await mouse.down();
      for (let i = 1; i <= 8; i++) await mouse.move(box.x + box.width - 2 + i * 15, box.y + box.height / 2);
      await mouse.up();
      await t.page.waitForTimeout(200);
      const after = (await th.boundingBox()).width;
      t.assert.ok(after > box.width + 60, `wider: ${box.width} -> ${after}`);
      const md = await t.saved();
      const m = /<!-- table widths=(\d+),(\d+) -->/.exec(md);
      t.assert.ok(m && Math.abs(Number(m[1]) - after) < 6, md);
      await t.shot();
      // Read back: leave and open the sermon again.
      await t.page.evaluate(() => { location.hash = "#reader"; });
      await t.page.waitForTimeout(300);
      await t.page.evaluate((id) => { location.hash = `#sermons/${id}`; }, s.id);
      await t.current(".sermon-layout");
      const again = (await table(t).locator("th").first().boundingBox()).width;
      t.assert.ok(Math.abs(again - after) < 6, `kept: ${after} -> ${again}`);
    },
  },
];
