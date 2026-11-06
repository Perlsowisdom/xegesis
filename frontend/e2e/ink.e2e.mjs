// T37 stage 1: ink blocks in notes and sermons, drawn with the real mouse (and synthetic pen events).

const fresh = async (t, body = "") => {
  await t.openNote(body);
  await t.focusEnd();
};

// stroke draws a line through points (fractions of the page) with the mouse.
export async function stroke(t, pts, nth = 0) {
  const box = await t.page.locator(`${t.root} .ink-page`).nth(nth).boundingBox();
  const at = ([fx, fy]) => [box.x + fx * box.width, box.y + fy * 150];
  const [x0, y0] = at(pts[0]);
  await t.page.mouse.move(x0, y0);
  await t.page.mouse.down();
  for (const p of pts.slice(1)) {
    const [x, y] = at(p);
    await t.page.mouse.move(x, y, { steps: 6 });
  }
  await t.page.mouse.up();
}

export const wave = (y0, n = 8) => Array.from({ length: n }, (_, i) => [0.1 + (0.8 * i) / (n - 1), (y0 + (i % 2 ? 0.35 : 0)) / 1]);

// the strokes in the saved Markdown
export async function savedInk(t) {
  const md = await t.saved();
  return [...md.matchAll(/```ink\n(.*)\n```/g)].map((m) => JSON.parse(m[1]));
}

export default [
  {
    row: "/handwriting puts an ink block in a note, and a mouse draws strokes in it",
    shot: "ink-01-draw.png",
    async run(t) {
      await fresh(t);
      await t.type("Notes on Romans 8");
      await t.press("Enter");
      await t.type("/hand");
      await t.press("Enter");
      await t.page.waitForSelector(`${t.root} .ink-block`);
      t.assert.equal(await t.page.locator(`${t.root} .ink-bar button`).count() > 8, true, "the tool strip");
      await stroke(t, wave(0.2));
      await stroke(t, wave(0.9));
      t.assert.equal(await t.page.locator(`${t.root} .ink-page path`).count(), 2);
      const docs = await savedInk(t);
      t.assert.equal(docs.length, 1);
      t.assert.equal(docs[0].s.length, 2);
      t.assert.equal(docs[0].s[0].t, "pen");
      t.assert.ok(docs[0].s[0].p.length > 5);
      t.assert.ok((await t.blocks()).at(-1).tag === "p", "a line to write on after the ink");
    },
  },
];
