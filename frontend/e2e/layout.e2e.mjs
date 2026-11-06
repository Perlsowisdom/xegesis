import { addBooks, closeBooks, openFromLibrary } from "./book.e2e.mjs";
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
  {
    row: "T38b 8: a sermon's properties fold into one line of chips and unfold, remembered on the device",
    shot: "t38b-08-props.png",
    async run(t) {
      const { page } = t;
      await page.evaluate(() => localStorage.removeItem("xegesis.sermon.props.folded"));
      const d = await t.app.run("document.create", { kind: "sermon" });
      await t.app.run("document.update", { id: d.id, title: "Props sermon", kind: "sermon", passage: "Psalm 84", preached_on: "2026-11-15", theme: "", body: "Body" });
      await page.evaluate((id) => { location.hash = `#sermons/${id}`; }, d.id);
      await t.current(".sermon-layout");
      const root = t.root;
      t.assert.ok(await page.locator(`${root} .page-props`).isVisible(), "new documents open unfolded");
      await t.shot("t38b-08-props-open.png");
      await page.locator(`${root} .props-fold`).click();
      t.assert.ok(!(await page.locator(`${root} .page-props`).isVisible()), "the rows fold away");
      const line = (await page.locator(`${root} .props-chip`).allTextContents()).join(" | ");
      t.assert.match(line, /Psalms? 84.*Sun 15 Nov 2026.*Sermon/);
      await page.waitForTimeout(150);
      await t.shot();
      // remembered: reopen
      await page.evaluate((id) => { location.hash = "#notes"; setTimeout(() => { location.hash = `#sermons/${id}`; }, 50); }, d.id);
      await t.current(".sermon-layout");
      t.assert.ok(!(await page.locator(`${t.root} .page-props`).isVisible()), "folded choice is remembered");
      await page.locator(`${t.root} .props-fold`).click();
      t.assert.ok(await page.locator(`${t.root} .page-props`).isVisible(), "unfolds again");
      t.assert.equal(await page.evaluate(() => localStorage.getItem("xegesis.sermon.props.folded")), "0");
      await t.app.run("document.delete", { id: d.id }).catch(() => {});
    },
  },
  {
    row: "T38b 7: the place in a book is a field: a number goes there, words list matching headings; a PDF takes a page",
    shot: "t38b-07-section-field.png",
    async run(t) {
      const { page } = t;
      await addBooks(t);
      await openFromLibrary(t, "E2E Test Book", "Open in Study");
      const pane = page.locator(".grid-pane:visible", { has: page.locator(".doc-book") }).first();
      await pane.locator(".book-text p").first().waitFor({ timeout: 15000 });
      const field = pane.locator(".book-where-input");
      t.assert.equal(await field.inputValue(), "1");
      t.assert.equal(await pane.locator(".book-of").textContent(), "of 2");
      await field.click();
      await t.type("2");
      await t.press("Enter");
      await page.waitForFunction(() => document.querySelector(".grid-pane .book-where-input:not([hidden])")?.value === "2", null, { timeout: 8000 });
      await pane.locator(".book-text p").first().waitFor();
      t.assert.equal(await field.inputValue(), "2");
      await field.click();
      await t.type("chapter one");
      await t.press("Enter");
      const hit = page.locator(".popover .toc-list button", { hasText: "Chapter One" });
      await hit.waitFor({ timeout: 5000 });
      await t.shot();
      await hit.click();
      await page.waitForFunction(() => [...document.querySelectorAll(".book-where-input")].some((i) => i.offsetParent && i.value === "1"), null, { timeout: 8000 });
      // a number out of range is refused
      await field.click();
      await t.type("99");
      await t.press("Enter");
      t.assert.equal(await field.inputValue(), "1");
      await closeBooks(t);
      await openFromLibrary(t, "e2e-test-pages", "Open in Study");
      const pp = page.locator(".grid-pane:visible", { has: page.locator(".doc-book") }).first();
      const pf = pp.locator(".book-where-input");
      await pf.waitFor({ timeout: 15000 });
      await page.waitForFunction(() => /Page/.test(document.querySelector(".grid-pane .book-unit")?.textContent ?? ""), null, { timeout: 15000 });
      await pf.click();
      await t.type("2");
      await t.press("Enter");
      await page.waitForFunction(() => [...document.querySelectorAll(".book-where-input")].some((i) => i.offsetParent && i.value === "2"), null, { timeout: 8000 });
      await closeBooks(t);
    },
  },
  {
    row: "T38b 5: a sermon holds several passages as chips (+ to add, x to remove), found by any of them",
    shot: "t38b-05-passages.png",
    async run(t) {
      const { page, app } = t;
      const d = await app.run("document.create", { kind: "sermon", title: "Walking on water" });
      await page.evaluate((id) => { location.hash = `#sermons/${id}`; }, d.id);
      await t.current(".sermon-layout");
      const root = t.root;
      const chips = page.locator(`${root} .passage-chips .doc-chip-wrap`);
      const addPassage = async (text) => {
        await page.locator(`${root} .passage-chips > .chip-add, ${root} .passage-chips > button.doc-chip`).last().click();
        const input = page.locator(".popover .ref-pop input");
        await input.waitFor();
        await t.type(text);
        await page.locator(".popover .ref-pop .ref-note.ok").waitFor({ timeout: 5000 });
        await t.press("Enter");
        await page.locator(".popover .ref-pop").waitFor({ state: "detached", timeout: 5000 });
      };
      await addPassage("Matt 14:22-33");
      await addPassage("Psalm 84");
      t.assert.equal(await chips.count(), 2);
      t.assert.deepEqual((await chips.allTextContents()).map((x) => x.replace(/[✦×]/g, "").trim()), ["Matthew 14:22-33", "Psalms 84"]);
      await t.press("Control+s");
      await page.waitForFunction((r) => document.querySelector(`${r} .save-state`)?.getAttribute("data-state") === "saved", root, { timeout: 10000 });
      await t.shot();
      const got = await app.run("document.get", { id: d.id });
      t.assert.deepEqual(got.passages.map((x) => x.display), ["Matthew 14:22-33", "Psalms 84"]);
      for (const ref of ["Matt 14:25", "Ps 84:2"]) {
        const found = await app.run("document.list", { ref });
        t.assert.ok((found.documents ?? found).some((x) => x.id === d.id), `found by ${ref}`);
      }
      // remove the first with its x
      await chips.first().locator(".doc-chip-x").click();
      await page.waitForFunction((r) => document.querySelectorAll(`${r} .passage-chips .doc-chip-wrap`).length === 1, root, { timeout: 5000 });
      await t.press("Control+s");
      await page.waitForTimeout(600);
      t.assert.deepEqual((await app.run("document.get", { id: d.id })).passages.map((x) => x.display), ["Psalms 84"]);
      await app.run("document.delete", { id: d.id }).catch(() => {});
    },
  },
  {
    row: "T38b 4: a note's menu has Turn into (Sermon, Bible study, Lesson); one command keeps text, verse, tags, folder; opens in the same pane; undoable",
    shot: "t38b-04-turn-into.png",
    async run(t) {
      const { page, app } = t;
      const title = "Turn me into a study";
      const n = await app.run("note.create", { title, body: "Grace is **free** and [John 3:16](#passage/John.3.16).", ref: "Rom 5:1-5", tags: ["grace", "romans"] });
      await openNoteInStudy(t, n.id, title);
      const pane = page.locator(".grid-pane:visible", { has: page.locator(".note-page") }).first();
      const panes = await page.locator(".grid-pane:visible").count();
      await pane.locator(".note-page .rich-text").waitFor({ timeout: 10000 });
      await pane.locator('.note-page [aria-label="Note actions"]').click();
      await page.locator(".popover .menu-list button", { hasText: "Turn into" }).click();
      await page.locator(".popover .turn-menu").waitFor({ timeout: 5000 });
      await t.shot("t38b-04-turn-menu.png");
      const before = (await app.run("history.list", { object_type: "notes", object_id: n.id }).catch(() => null));
      await page.locator(".popover .turn-menu button", { hasText: "Bible study" }).click();
      const sermon = page.locator(".grid-pane:visible .sermon-layout").first();
      await sermon.waitFor({ timeout: 10000 });
      t.assert.equal(await page.locator(".grid-pane:visible").count(), panes, "same pane");
      t.assert.equal(await page.locator(".grid-pane:visible .note-page").count(), 0, "the note page is gone from the pane");
      t.assert.match(await sermon.locator(".passage-chips").textContent(), /Romans 5:1-5/);
      t.assert.match(await sermon.locator(".kind-menu, .doc-chip", { hasText: "Bible study" }).first().textContent(), /Bible study/);
      t.assert.match(await sermon.locator(".rich-text").first().textContent(), /Grace is free and .*John 3:16/);
      await page.waitForTimeout(300);
      await t.shot();
      const docs = await app.run("document.list", { kind: "study" });
      const d = (docs.documents ?? docs).find((x) => x.title === title);
      t.assert.ok(d, "the study exists");
      t.assert.deepEqual((await app.run("document.get", { id: d.id })).tags, ["grace", "romans"]);
      t.assert.equal(d.from_note ?? (await app.run("document.get", { id: d.id })).from_note, n.id);
      const left = await app.run("note.list", {});
      t.assert.ok(!(left.notes ?? left).some((x) => x.id === n.id), "the note left My work");
      // undo: the note comes back from the bin
      await app.run("note.restore", { id: n.id });
      await app.run("document.delete", { id: d.id });
      const back = await app.run("note.list", {});
      t.assert.ok((back.notes ?? back).some((x) => x.id === n.id), "the note is back");
      await app.run("note.delete", { id: n.id });
    },
  },
  {
    row: "T38b 2: dropping a tab on a pane's edge splits that pane (side by side, or above/below), so two panes sit side by side above one wide pane",
    shot: "t38b-02-splits.png",
    async run(t) {
      const { page, app } = t;
      const names = ["Split alpha", "Split beta", "Split gamma"];
      const ids = [];
      for (const title of names) {
        const n = await app.run("note.create", { title, body: "x" });
        ids.push(n.id);
        await openNoteInStudy(t, n.id, title);
      }
      const tab = (name) => page.locator(".grid-pane:visible .grid-tab-btn", { hasText: name }).first();
      const paneOf = (name) => page.locator(".grid-pane:visible", { has: page.locator(".grid-tab-btn", { hasText: name }) }).first();
      const box = (name) => paneOf(name).evaluate((e) => { const r = e.getBoundingClientRect(); return { x: r.x, y: r.y, w: r.width, h: r.height }; });
      // gamma onto the bottom edge of alpha's pane: a pane below
      let b = await box("Split alpha");
      await tab("Split gamma").dragTo(paneOf("Split alpha"), { targetPosition: { x: b.w / 2, y: b.h - 12 } });
      await page.waitForTimeout(400);
      const g1 = await box("Split gamma");
      const a1 = await box("Split alpha");
      t.assert.ok(g1.y > a1.y + a1.h - 4 && Math.abs(g1.w - a1.w) < 4, "gamma is a pane below alpha, as wide");
      // beta onto the right edge of alpha's (top) pane: beside alpha, gamma stays wide below
      b = await box("Split alpha");
      await tab("Split beta").dragTo(paneOf("Split alpha"), { targetPosition: { x: b.w - 12, y: b.h / 2 } });
      await page.waitForTimeout(400);
      const a = await box("Split alpha"), be = await box("Split beta"), g = await box("Split gamma");
      t.assert.ok(Math.abs(a.y - be.y) < 3 && be.x > a.x + a.w - 4, "alpha and beta are side by side");
      t.assert.ok(g.y > a.y + a.h - 4 && Math.abs(g.x - a.x) < 3 && Math.abs(g.w - (be.x + be.w - a.x)) < 4, "gamma is one wide pane below both");
      await t.shot();
      for (const id of ids) await app.run("note.delete", { id });
    },
  },
];
