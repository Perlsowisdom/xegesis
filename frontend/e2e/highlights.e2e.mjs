// docs/editor-behaviour.md, "Highlights inside verses" (T34f), in the real app: words are chosen
// by dragging the mouse over them in a Bible pane and coloured from the marking toolbar.

const ROM12 = 45012000;
const v = (n) => ROM12 + n;

// readRom12 shows Romans 12 in Read mode, its highlights cleared from the verses given.
async function readRom12(t, ...verses) {
  const { page, app } = t;
  for (const n of verses) {
    for (const h of await app.run("highlight.list", { ref: `Rom 12:${n}` })) await app.run("highlight.delete", { id: h.id });
  }
  await page.evaluate(() => { location.hash = "#passage/Rom.12"; });
  const read = page.locator(".shell button, header button", { hasText: /^Read$/ }).first();
  if (await read.getAttribute("aria-pressed") !== "true") await read.click();
  await page.waitForFunction(() => [...document.querySelectorAll(".doc-bible .vt[data-tb]")].some((e) => e.offsetParent && e.dataset.tb === "45012012"), null, { timeout: 15000 });
  // Repaint after the clean-up, as the app does after any change to highlights.
  await page.evaluate(() => document.dispatchEvent(new Event("xegesis:marks-changed")));
  await page.waitForTimeout(300);
}

// words finds words a..b (0-based, b excluded) of a shown verse: their offsets, text and where the
// first and last characters are on screen.
function words(page, verse, a, b) {
  return page.evaluate(([verse, a, b]) => {
    const vt = [...document.querySelectorAll(`.doc-bible .vt[data-tb="${verse}"]`)].find((e) => e.offsetParent);
    if (!vt) throw new Error(`verse ${verse} is not shown`);
    const box0 = vt.getBoundingClientRect();
    if (box0.top < 120 || box0.bottom > innerHeight - 60) vt.scrollIntoView({ block: "center" });
    const text = vt.textContent;
    const ws = [...text.matchAll(/\S+/g)];
    const wa = a < 0 ? ws.length + a : a;
    const wb = b < 0 ? ws.length + b : b;
    const start = ws[wa].index;
    const end = ws[wb - 1].index + ws[wb - 1][0].length;
    const at = (off) => {
      const w = document.createTreeWalker(vt, NodeFilter.SHOW_TEXT);
      let n = 0;
      for (let t = w.nextNode(); t; t = w.nextNode()) {
        if (off < n + t.length) return [t, off - n];
        n += t.length;
      }
      throw new Error("offset past the verse");
    };
    const box = (off) => {
      const [t, o] = at(off);
      const r = document.createRange();
      r.setStart(t, o);
      r.setEnd(t, o + 1);
      return r.getBoundingClientRect();
    };
    const f = box(start);
    const l = box(end - 1);
    return { start, end, text: text.slice(start, end), all: text, from: { x: f.left + 1, y: f.top + f.height / 2 }, to: { x: l.right - 1, y: l.top + l.height / 2 } };
  }, [verse, a, b]);
}

// drag selects from one point to another with the mouse, as a reader does.
async function drag(page, from, to) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move((from.x + to.x) / 2, (from.y + to.y) / 2, { steps: 4 });
  await page.mouse.move(to.x, to.y, { steps: 6 });
  await page.mouse.up();
}

// colour picks a colour (or Underline) from the toolbar that is open.
async function toolbar(page, label) {
  const bar = page.locator(".mark-bar");
  await bar.waitFor({ timeout: 5000 });
  await bar.locator(`button[aria-label="${label}"]`).click();
}

// highlightWords drags over words of one verse (or from one verse to another) and colours them.
async function highlightWords(t, from, to, colour = "green") {
  if (to) await words(t.page, ...to);
  const a = await words(t.page, ...from);
  const b = to ? await words(t.page, ...to) : a;
  // The first drag after the app starts can be lost while the window takes focus; drag again then.
  for (let i = 0; ; i++) {
    await drag(t.page, a.from, b.to);
    if (await t.page.locator(".mark-bar").waitFor({ timeout: 3000 }).then(() => true, () => false)) break;
    if (i === 2) throw new Error("no toolbar after selecting words");
    await t.page.mouse.click(5, 300);
  }
  await toolbar(t.page, `Highlight ${colour}`);
  await t.page.waitForFunction(([verse, s]) => [...document.querySelectorAll(`.doc-bible .vt[data-tb="${verse}"] mark[data-hl]`)].some((m) => m.classList.contains(`hl-${s}`)),
    [from[0], colour], { timeout: 8000 });
  return { a, b };
}

// marked gives each highlight's drawn words in a verse, in order: [[id, classes, text]].
function marked(page, verse) {
  return page.evaluate((verse) => {
    const vt = [...document.querySelectorAll(`.doc-bible .vt[data-tb="${verse}"]`)].find((e) => e.offsetParent);
    const out = [];
    for (const m of vt.querySelectorAll("mark[data-hl]")) {
      const last = out[out.length - 1];
      if (last && last[0] === m.dataset.hl) last[2] += m.textContent;
      else out.push([m.dataset.hl, m.className, m.textContent]);
    }
    return out;
  }, verse);
}

// lined checks that a highlight's marks sit exactly on its words: the rectangles of the marked
// characters and of the marks are the same.
function lined(page, verse, start, end) {
  return page.evaluate(([verse, start, end]) => {
    const vt = [...document.querySelectorAll(`.doc-bible .vt[data-tb="${verse}"]`)].find((e) => e.offsetParent);
    const marks = [...vt.querySelectorAll("mark[data-hl]")];
    if (!marks.length) return "no marks";
    const r = document.createRange();
    r.setStartBefore(marks[0]);
    r.setEndAfter(marks[marks.length - 1]);
    const want = r.toString();
    const all = vt.textContent;
    if (all.slice(start, end) !== want) return `marks cover "${want}", not "${all.slice(start, end)}"`;
    const a = marks[0].getBoundingClientRect();
    const first = document.createRange();
    const w = document.createTreeWalker(marks[0], NodeFilter.SHOW_TEXT);
    const t0 = w.nextNode();
    first.setStart(t0, 0);
    first.setEnd(t0, 1);
    const c = first.getBoundingClientRect();
    return Math.abs(a.left - c.left) < 1.5 && Math.abs(a.top - c.top) < 4 ? "ok" : `mark at ${a.left},${a.top}, word at ${c.left},${c.top}`;
  }, [verse, start, end]);
}

const ranged = async (t, ref) => (await t.app.run("highlight.list", { ref })).filter((h) => h.range);

export default [
  {
    row: "Highlights in verses: selecting part of a verse marks exactly those words",
    shot: "hl-01-partial.png",
    async run(t) {
      await readRom12(t, 1);
      const { a } = await highlightWords(t, [v(1), 3, 8], null, "green");
      const m = await marked(t.page, v(1));
      t.assert.equal(m.length, 1);
      t.assert.equal(m[0][2], a.text, "only the selected words are marked");
      t.assert.ok(!(await t.page.locator(`.doc-bible .verse[data-base="${v(1)}"].hl`).count()), "the verse itself is not highlighted");
      const [h] = await ranged(t, "Rom 12:1");
      t.assert.deepEqual(h.range, { start: { verse: v(1), offset: a.start }, end: { verse: v(1), offset: a.end } });
      t.assert.equal(h.text, a.text.replace(/\s+/g, " "));
      t.assert.ok(h.module, "the Bible is kept");
      await t.page.mouse.move(5, 5);
    },
  },
  {
    row: "Highlights in verses: a highlight spans two verses, starting and ending mid-verse",
    shot: "hl-02-span.png",
    async run(t) {
      await readRom12(t, 1, 2);
      const { a, b } = await highlightWords(t, [v(1), -3, -1], [v(2), 0, 4], "blue");
      const [h] = await ranged(t, "Rom 12:1-2");
      t.assert.equal(h.verse_start, v(1));
      t.assert.equal(h.verse_end, v(2));
      t.assert.deepEqual(h.range, { start: { verse: v(1), offset: a.start }, end: { verse: v(2), offset: b.end } });
      t.assert.equal((await marked(t.page, v(1)))[0][2], a.all.slice(a.start));
      t.assert.equal((await marked(t.page, v(2)))[0][2], b.all.slice(0, b.end));
      t.assert.equal(h.ref, "Romans 12:1-2");
    },
  },
  {
    row: "Highlights in verses: overlapping a highlight replaces the overlapped part, so colours never stack",
    shot: "hl-03-overlap.png",
    async run(t) {
      await readRom12(t, 3);
      const { a } = await highlightWords(t, [v(3), 1, 12], null, "green");
      const { a: mid } = await highlightWords(t, [v(3), 4, 7], null, "pink");
      const m = await marked(t.page, v(3));
      t.assert.deepEqual(m.map((x) => x[1].match(/hl-(\w+)/)[1]), ["green", "pink", "green"]);
      t.assert.equal(m[1][2], mid.text);
      t.assert.equal(m.map((x) => x[2]).join(""), a.text, "together they still cover the first selection");
      t.assert.equal(await t.page.locator(".doc-bible mark[data-hl] mark[data-hl]").count(), 0, "no mark inside another");
      const list = await ranged(t, "Rom 12:3");
      t.assert.deepEqual(list.map((h) => h.style), ["green", "pink", "green"]);
    },
  },
  {
    row: "Highlights in verses: clicking a highlight selects all of it for the toolbar; recolour and underline",
    shot: "hl-04-recolour.png",
    async run(t) {
      const { page } = t;
      await readRom12(t, 4, 5);
      const { a, b } = await highlightWords(t, [v(4), 2, -1], [v(5), 0, 3], "yellow");
      // Click the part in the second verse.
      const box = await page.locator(`.doc-bible .vt[data-tb="${v(5)}"] mark[data-hl]`).first().boundingBox();
      await page.mouse.click(box.x + 4, box.y + box.height / 2);
      const bar = page.locator(".mark-bar[aria-label=Highlight]");
      await bar.waitFor({ timeout: 5000 });
      t.assert.equal(await bar.locator('button[aria-label="Highlight yellow"]').getAttribute("aria-pressed"), "true");
      const sel = (await page.evaluate(() => getSelection().toString())).replace(/\s+/g, " ").trim();
      t.assert.equal(sel, `${a.all.slice(a.start)} ${b.all.slice(0, b.end)}`.replace(/\s+/g, " ").trim(), "the whole highlight is selected");
      await t.shot("hl-04a-selected.png");
      await toolbar(page, "Highlight blue");
      await page.waitForFunction((n) => [...document.querySelectorAll(`.doc-bible .vt[data-tb="${n}"] mark[data-hl]`)].every((m) => m.classList.contains("hl-blue")), v(4));
      t.assert.ok((await marked(page, v(5)))[0][1].includes("hl-blue"), "both verses recoloured");
      const box2 = await page.locator(`.doc-bible .vt[data-tb="${v(4)}"] mark[data-hl]`).first().boundingBox();
      await page.mouse.click(box2.x + 4, box2.y + box2.height / 2);
      await toolbar(page, "Underline");
      await page.waitForFunction((n) => !!document.querySelector(`.doc-bible .vt[data-tb="${n}"] mark.hl-u`), v(5));
      const [h] = await ranged(t, "Rom 12:4-5");
      t.assert.equal(h.style, "blue-underline");
      t.assert.deepEqual(h.range, { start: { verse: v(4), offset: a.start }, end: { verse: v(5), offset: b.end } }, "the range is unchanged");
      await page.mouse.move(5, 5);
      await t.shot();
    },
  },
  {
    row: "Highlights in verses: removing a highlight from its toolbar",
    shot: "hl-05-remove.png",
    async run(t) {
      const { page } = t;
      await readRom12(t, 6);
      await highlightWords(t, [v(6), 2, 6], null, "purple");
      const box = await page.locator(`.doc-bible .vt[data-tb="${v(6)}"] mark[data-hl]`).first().boundingBox();
      await page.mouse.click(box.x + 4, box.y + box.height / 2);
      await t.shot("hl-05a-before-remove.png");
      await toolbar(page, "Remove the highlight");
      await page.waitForFunction((n) => !document.querySelector(`.doc-bible .vt[data-tb="${n}"] mark[data-hl]`), v(6), { timeout: 8000 });
      t.assert.equal((await t.app.run("highlight.list", { ref: "Rom 12:6" })).length, 0);
      await t.shot();
    },
  },
  {
    row: "Highlights in verses: the Highlights tool and My work → Highlights show the exact words",
    shot: "hl-06-sidebar.png",
    async run(t) {
      const { page } = t;
      await readRom12(t, 7);
      const { a } = await highlightWords(t, [v(7), 1, 5], null, "green");
      const phrase = a.text.replace(/\s+/g, " ");
      const opened = !(await page.locator("#rightbar").isVisible().catch(() => false));
      if (opened) await page.keyboard.press("Control+.");
      await page.locator("#rightbar [role=tab]", { hasText: "Highlights" }).click();
      const row = page.locator("#rightbar .hl-row", { hasText: "Romans 12:7" }).first();
      await row.waitFor({ timeout: 8000 });
      t.assert.equal((await row.locator(".hl-text").textContent()).trim(), `“${phrase}”`);
      await t.shot();
      await page.locator("#rightbar button", { hasText: "All highlights in My work" }).click();
      const work = page.locator(".work-view .hl-row", { hasText: "Romans 12:7" }).first();
      await work.waitFor({ timeout: 8000 });
      t.assert.equal((await work.locator(".hl-text").textContent()).trim(), `“${phrase}”`);
      await t.shot("hl-06b-my-work.png");
      // Leave the sidebar as it was: open, it folds the note toolbar's Insert button away.
      await page.evaluate(() => { location.hash = "#passage/Rom.12"; });
      await page.waitForSelector(".doc-bible .vt[data-tb]", { timeout: 10000 });
      if (opened && await page.locator("#rightbar").isVisible()) {
        await page.keyboard.press("Control+.");
        await page.locator("#rightbar").waitFor({ state: "hidden", timeout: 5000 });
      }
    },
  },
  {
    row: "Highlights in verses: inserting a highlight into a note quotes the exact words",
    shot: "hl-07-insert.png",
    async run(t) {
      const { page } = t;
      await readRom12(t, 9);
      const { a } = await highlightWords(t, [v(9), 0, 3], null, "pink");
      const phrase = a.text.replace(/\s+/g, " ");
      await t.openNote("", "Partial highlight insert");
      await t.focusEnd();
      // The / menu, as typed: the toolbar may fold its Insert button away when the sidebar is open.
      await t.type("/highlight");
      await page.waitForSelector(".slash-menu:not([hidden])");
      await t.press("Enter");
      await page.locator(".popover input").first().waitFor();
      await t.type(phrase);
      await page.waitForFunction(() => [...document.querySelectorAll(".popover .pick-row")].some((r) => r.textContent.includes("12:9")), null, { timeout: 8000 });
      await t.press("Enter");
      const q = t.editor().locator(".verse-quote");
      await q.waitFor({ timeout: 8000 });
      t.assert.ok((await q.locator(".vq-credit").textContent()).includes("Romans 12:9"));
      const body = await t.saved();
      t.assert.ok(body.startsWith(`> ${phrase}\n> — [Romans 12:9](#passage/Rom.12.9)`), body);
      t.assert.ok(!body.includes(a.all.slice(a.end).trim().split(/\s+/).slice(0, 3).join(" ")), "the rest of the verse is not quoted");
    },
  },
  {
    row: "Highlights in verses: highlights stay on their words after a font-size change, a narrower window and Study mode",
    shot: "hl-08-font-size.png",
    async run(t) {
      const { page } = t;
      await readRom12(t, 10);
      const { a } = await highlightWords(t, [v(10), 2, 7], null, "blue");
      t.assert.equal(await lined(page, v(10), a.start, a.end), "ok");
      const aa = page.locator("button.aa-btn:visible").first();
      await aa.click();
      const larger = page.locator(".popover button[aria-label='Larger text']");
      await larger.click();
      await larger.click();
      await page.keyboard.press("Escape");
      await page.waitForTimeout(300);
      t.assert.equal(await lined(page, v(10), a.start, a.end), "ok", "after a larger font");
      await page.locator(`.doc-bible .vt[data-tb="${v(10)}"]`).first().scrollIntoViewIfNeeded();
      await t.shot();
      const size = await page.evaluate(() => [window.outerWidth, window.outerHeight]);
      const resized = await page.evaluate(() => { if (!window.runtime?.WindowSetSize) return false; window.runtime.WindowSetSize(820, 760); return true; });
      if (resized) {
        await page.waitForFunction(() => window.innerWidth < 900, null, { timeout: 5000 });
        await page.waitForTimeout(300);
        t.assert.equal(await lined(page, v(10), a.start, a.end), "ok", "in a narrower window");
        await page.locator(`.doc-bible .vt[data-tb="${v(10)}"]`).first().scrollIntoViewIfNeeded();
        await t.shot("hl-08b-narrow.png");
        await page.evaluate(([w, h]) => window.runtime.WindowSetSize(w, h), size);
      } else t.note = "the window could not be resized here";
      await page.locator(".shell button, header button", { hasText: /^Study$/ }).first().click();
      await page.waitForFunction((n) => [...document.querySelectorAll(`.doc-bible .vt[data-tb="${n}"]`)].some((e) => e.offsetParent && e.querySelector("mark[data-hl]")), v(10), { timeout: 10000 });
      t.assert.equal(await lined(page, v(10), a.start, a.end), "ok", "in Study mode");
      await page.locator(`.doc-bible .vt[data-tb="${v(10)}"]:visible`).first().scrollIntoViewIfNeeded();
      await t.shot("hl-08c-study.png");
      await page.locator(".shell button, header button", { hasText: /^Read$/ }).first().click();
      await aa.click();
      const smaller = page.locator(".popover button[aria-label='Smaller text']");
      await smaller.click();
      await smaller.click();
      await page.keyboard.press("Escape");
    },
  },
  {
    row: "Highlights in verses: in another Bible the words show on the whole verse, faintly, and say so on hover",
    shot: "hl-09-other-bible.png",
    async run(t) {
      const { page } = t;
      await readRom12(t, 11);
      const { a } = await highlightWords(t, [v(11), 1, 4], null, "purple");
      const [h] = await ranged(t, "Rom 12:11");
      const bibles = await t.app.run("bible.modules");
      const other = bibles.find((m) => m.id !== h.module);
      if (!other) { t.note = "only one Bible is installed"; return; }
      // Show the other Bible instead, from the Bibles button.
      const pick = async (on, off) => {
        await page.locator(".bibles-btn:visible").first().click();
        const chip = (id) => page.locator(`.popover .chip-toggle:has(input[value="${id}"])`);
        await chip(on).click();
        await chip(off).click();
        await page.keyboard.press("Escape");
        await page.waitForFunction((on) => [...document.querySelectorAll(".doc-bible .vt[data-tb]")].some((e) => e.offsetParent && e.dataset.module === on), on, { timeout: 10000 });
        await page.waitForTimeout(300);
      };
      await pick(other.id, h.module);
      const vt = page.locator(`.doc-bible .vt[data-tb="${v(11)}"]:visible`).first();
      t.assert.ok((await vt.getAttribute("class")).includes("hl-faint"), "the whole verse, faintly");
      t.assert.equal(await vt.locator("mark").count(), 0, "no words marked in this Bible");
      t.assert.match(await vt.getAttribute("title"), /highlighted in .*whole verse/i);
      await vt.hover();
      await page.waitForTimeout(1200);
      await t.shot();
      await pick(h.module, other.id);
      t.assert.equal((await marked(page, v(11)))[0][2], a.text);
    },
  },
  {
    row: "Highlights in verses: an older whole-verse highlight stays whole, with new words drawn over it",
    shot: "hl-10-whole-verse.png",
    async run(t) {
      const { page } = t;
      await readRom12(t, 12);
      const whole = await t.app.run("highlight.create", { ref: "Rom 12:12", style: "green" });
      await page.evaluate(() => document.dispatchEvent(new Event("xegesis:marks-changed")));
      await page.waitForFunction((n) => !!document.querySelector(`.doc-bible .verse[data-base="${n}"].hl-green`), v(12), { timeout: 8000 });
      await t.shot("hl-10a-whole.png");
      const { a } = await highlightWords(t, [v(12), 2, 4], null, "pink");
      const m = await marked(page, v(12));
      t.assert.deepEqual(m.map((x) => x[0] === whole.id ? "whole" : x[1].match(/hl-(\w+)/)[1]), ["whole", "pink", "whole"]);
      t.assert.equal(m[1][2], a.text);
      const list = await t.app.run("highlight.list", { ref: "Rom 12:12" });
      const kept = list.find((h) => h.id === whole.id);
      t.assert.ok(kept && !kept.range && kept.verse_start === v(12) && kept.style === "green", "the old highlight is unchanged");
      await t.shot();
    },
  },
];
