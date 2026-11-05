// T38b: tab bar, collapsible properties, section field, several sermon passages, Turn into, split tree.

const LONG = "A very long title that goes on and on about the whole counsel of God";

// openNoteInStudy opens a note's card from My work in a Study tab.
export async function openNoteInStudy(t, id, title) {
  const { page } = t;
  await page.evaluate(() => { location.hash = "#notes"; });
  const card = page.locator(".work-card:visible", { hasText: title }).first();
  await card.waitFor({ timeout: 10000 });
  await card.dblclick();
  await page.locator(".grid-tab-btn", { hasText: title.slice(0, 20) }).first().waitFor({ timeout: 10000 });
}

export default [
  {
    row: "T38b 6: long tab titles end in an ellipsis with the full title on hover; + and close never overlap a title",
    shot: "t38b-06-tabs.png",
    async run(t) {
      const { page } = t;
      const ids = [];
      for (let i = 0; i < 4; i++) {
        const title = `${LONG} ${i + 1}`;
        const n = await t.app.run("note.create", { title, body: "x" });
        ids.push(n.id);
        await openNoteInStudy(t, n.id, title);
      }
      const r = await page.evaluate(() => {
        const bar = [...document.querySelectorAll(".grid-pane")].find((p) => p.offsetParent && p.querySelector(".grid-tab"))?.querySelector(".grid-bar");
        const plus = bar.querySelector(".grid-plus").getBoundingClientRect();
        const strip = bar.querySelector(".grid-tabstrip").getBoundingClientRect();
        const btns = [...bar.querySelectorAll(".grid-tab-btn")];
        const tabs = [...bar.querySelectorAll(".grid-tab")].map((e) => e.getBoundingClientRect());
        const b0 = btns.find((b) => b.title.startsWith("A very long"));
        return {
          plusLeft: plus.left, stripRight: strip.right,
          maxTab: Math.max(...tabs.map((x) => x.width)), rem: parseFloat(getComputedStyle(document.documentElement).fontSize),
          ellipsis: getComputedStyle(b0).textOverflow, clipped: b0.scrollWidth > b0.clientWidth,
          title: b0.title, n: btns.length, scrolls: bar.querySelector(".grid-tabstrip").scrollWidth > bar.querySelector(".grid-tabstrip").clientWidth,
          closeInside: [...bar.querySelectorAll(".grid-tab")].every((tb) => { const c = tb.querySelector(".grid-tab-close").getBoundingClientRect(), bb = tb.querySelector(".grid-tab-btn").getBoundingClientRect(); return c.left >= bb.right - 1 && c.right <= tb.getBoundingClientRect().right + 1; }),
        };
      });
      t.assert.ok(r.n >= 4, JSON.stringify(r));
      t.assert.ok(r.maxTab <= 12 * r.rem + 2, `tab width capped: ${JSON.stringify(r)}`);
      t.assert.equal(r.ellipsis, "ellipsis");
      t.assert.ok(r.clipped, "long title is clipped");
      t.assert.ok(r.title.startsWith(LONG), "full title on hover");
      t.assert.ok(r.stripRight <= r.plusLeft + 1, `tab strip ends before +: ${JSON.stringify(r)}`);
      t.assert.ok(r.closeInside, "close buttons sit after their title");
      t.note = `tabs scroll: ${r.scrolls}`;
      await t.shot();
      for (const id of ids) await t.app.run("note.delete", { id }).catch(() => {});
    },
  },
];
