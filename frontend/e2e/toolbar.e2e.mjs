// T34g section 2: nothing hides behind the sticky toolbar, tested at a short window height.

const filler = (n, what) => Array.from({ length: n }, (_, i) => `Line ${i + 1} of the ${what} section.`).join("\n\n");
const LONG = `# Alpha\n\n${filler(25, "opening")}\n\n## Beta\n\n${filler(25, "middle")}\n\n# Gamma\n\n${filler(25, "closing")}\n\n## Delta\n\nLast words.`;

// short makes the window short for one check and gives back a function that undoes it.
async function short(t, height = 520) {
  const size = await t.page.evaluate(() => ({ width: innerWidth, height: innerHeight }));
  await t.page.setViewportSize({ width: size.width, height });
  await t.page.waitForTimeout(300);
  return () => t.page.setViewportSize(size).catch(() => {});
}

// covered tells whether the toolbar, stuck at the top, covers any of el.
async function covered(t, el) {
  return t.page.evaluate(([root, el]) => {
    const bar = document.querySelector(`${root} .rich-toolbar`).getBoundingClientRect();
    const r = el.getBoundingClientRect();
    return r.top < bar.bottom && r.bottom > bar.top;
  }, [t.root, await el.elementHandle()]);
}

async function hoverStrip(t) {
  const strip = t.page.locator(`${t.root} .rt-strip`);
  const names = strip.locator(".rt-strip-names");
  for (let i = 0; i < 3 && !(await names.isVisible()); i++) {
    await t.page.mouse.move(10, 10);
    await strip.locator(".rt-strip-mark").first().hover({ force: true });
    await t.page.waitForTimeout(150);
  }
  await names.waitFor({ state: "visible", timeout: 3000 });
  return names;
}

const checks = [];
for (const kind of ["note", "sermon"]) {
  checks.push({
    row: `Jumping to a heading puts it just below the sticky toolbar (${kind}, short window)`,
    shot: `tb-01-jump-${kind}.png`,
    async run(t) {
      const undo = await short(t);
      try {
        if (kind === "note") await t.openNote(LONG, "Jumps");
        else await t.openSermon(LONG, "Jumps");
        const names = await hoverStrip(t);
        await names.locator(".rt-strip-name", { hasText: "Gamma" }).click();
        await t.page.waitForTimeout(900);
        const gamma = t.editor().locator("h1", { hasText: "Gamma" });
        t.assert.equal(await covered(t, gamma), false, "Gamma is not under the toolbar");
        const gap = await t.page.evaluate(([root, el]) => el.getBoundingClientRect().top - document.querySelector(`${root} .rich-toolbar`).getBoundingClientRect().bottom,
          [t.root, await gamma.elementHandle()]);
        t.assert.ok(gap >= 0 && gap < 60, `Gamma sits just below the toolbar (${Math.round(gap)} px)`);
        await t.shot();
      } finally {
        await undo();
      }
    },
  });
}
checks.push({
  row: "The outline strip's names open above the sticky toolbar (short window, scrolled)",
  shot: "tb-02-strip-over-toolbar.png",
  async run(t) {
    const undo = await short(t, 420);
    try {
      await t.openNote(LONG, "Strip over toolbar");
      const onTop = async (when) => {
        const names = await hoverStrip(t);
        const hidden = await names.locator(".rt-strip-name").evaluateAll((els) => els.filter((e) => {
          const r = e.getBoundingClientRect();
          const x = r.left + Math.min(20, r.width / 2);
          const y = r.top + r.height / 2;
          const hit = document.elementFromPoint(x, y);
          return !(hit === e || e.contains(hit));
        }).map((e) => e.textContent));
        t.assert.deepEqual(hidden, [], `every name is on top and in view (${when})`);
      };
      await onTop("at the top of the page");
      await t.shot("tb-02-strip-top.png");
      // Scroll down so the toolbar is stuck at the top.
      await t.page.mouse.move(10, 10);
      await t.editor().locator("p", { hasText: "Line 20 of the opening" }).scrollIntoViewIfNeeded();
      await t.page.mouse.wheel(0, 300);
      await t.page.waitForTimeout(300);
      await onTop("scrolled");
      await t.shot();
    } finally {
      await undo();
    }
  },
});
checks.push({
  row: "Toolbar popovers (heading sizes, Insert) open above the toolbar's layer, in view (short window)",
  shot: "tb-03-popover.png",
  async run(t) {
    const undo = await short(t, 420);
    try {
      await t.openNote(LONG, "Popovers");
      await t.editor().locator("p", { hasText: "Line 20 of the opening" }).click();
      await t.page.mouse.wheel(0, 200);
      await t.page.waitForTimeout(200);
      await t.page.click(`${t.root} [data-tool=heading]`);
      const pop = t.page.locator(".popover").last();
      await pop.waitFor();
      const ok = await pop.evaluate((p) => {
        const r = p.getBoundingClientRect();
        const hit = document.elementFromPoint(r.left + r.width / 2, r.top + 12);
        return p.contains(hit) && r.bottom <= innerHeight + 1 && r.top >= 0;
      });
      t.assert.ok(ok, "the heading menu is on top and inside the window");
      await t.shot();
      await t.press("Escape");
    } finally {
      await undo();
    }
  },
});

export default checks;
