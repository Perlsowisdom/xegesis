import { existsSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { inflateSync } from "node:zlib";

// T37 stage 1: ink blocks in notes and sermons, drawn with the real mouse (and synthetic pen events).

const fresh = async (t, body = "") => {
  await t.openNote(body);
  await t.focusEnd();
};

// stroke draws a line through points (fractions of the page width, and of 150 px down) with the mouse.
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

export const wave = (y0, n = 8) => Array.from({ length: n }, (_, i) => [0.1 + (0.8 * i) / (n - 1), y0 + (i % 2 ? 0.35 : 0)]);

// the pages of strokes in the saved Markdown
export async function savedInk(t) {
  await t.page.waitForTimeout(500); // the editor has handed the strokes over
  const md = await t.saved();
  return [...md.matchAll(/```ink\n(.*)\n```/g)].map((m) => JSON.parse(m[1]));
}

export async function addInk(t) {
  await t.press("Enter");
  await t.type("/hand");
  await t.press("Enter");
  await t.page.waitForSelector(`${t.root} .ink-block`);
}

const shared = {};

const tool = (t, label) => t.page.click(`${t.root} .ink-bar [aria-label="${label}"]`);
const count = (t) => t.page.locator(`${t.root} .ink-page path`).count();

// pen draws with synthetic pen pointer events (pressure, tilt, the eraser end), since the harness has no pen.
export async function pen(t, pts, o = {}) {
  await t.page.evaluate(({ root, pts, o }) => {
    const svg = document.querySelector(`${root} .ink-page`);
    const r = svg.getBoundingClientRect();
    const ev = (type, [fx, fy, pr], buttons) => new PointerEvent(type, {
      bubbles: true, cancelable: true, pointerId: o.id ?? 7, pointerType: o.type ?? "pen", isPrimary: true, pressure: pr, tiltX: 20, tiltY: 10,
      button: type === "pointerup" ? 0 : (o.button ?? 0), buttons: type === "pointerup" ? 0 : buttons, clientX: r.left + fx * r.width, clientY: r.top + fy * 150,
    });
    svg.dispatchEvent(ev("pointerdown", pts[0], o.buttons ?? 1));
    for (const p of pts.slice(1)) svg.dispatchEvent(ev("pointermove", p, o.buttons ?? 1));
    svg.dispatchEvent(ev("pointerup", pts[pts.length - 1], 0));
  }, { root: t.root, pts, o });
}

export const line = (y, from = 0.1, to = 0.9, n = 10) => Array.from({ length: n }, (_, i) => [from + ((to - from) * i) / (n - 1), y]);

export default [
  {
    row: "/handwriting puts an ink block in a note, and a mouse draws strokes in it",
    shot: "ink-01-draw.png",
    async run(t) {
      await fresh(t);
      await t.type("Notes on Romans 8");
      await addInk(t);
      t.assert.equal((await t.page.locator(`${t.root} .ink-bar button`).count()) > 8, true, "the tool strip");
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
  {
    row: "Ink is saved with the note, survives a restart, and comes back drawn",
    shot: "ink-02-restart.png",
    timeout: 180000,
    async run(t) {
      await fresh(t, "Intro line");
      await t.focusEnd();
      await addInk(t);
      await stroke(t, wave(0.2));
      const saved = await t.saved();
      t.assert.ok(saved.startsWith('Intro line\n\n```ink\n{"v":1,"h":'), saved.slice(0, 60));
      const id = t.id;
      shared.note = { id, body: saved };
      await t.app.restart();
      const page = t.app.page;
      t.assert.equal((await t.app.run("note.get", { id })).body, saved, "the same Markdown after a restart");
      await page.evaluate((id) => { location.hash = `#notes/note/${id}`; }, id);
      await page.locator(".note-page .ink-page path").first().waitFor({ timeout: 15000 });
      t.assert.equal(await page.locator(".note-page .ink-page path").count(), 1);
      await t.app.shot("ink-02-restart.png");
      t.shotTaken = true;
    },
  },
  {
    row: "Ink exports as SVG in HTML and PDF, and the strokes stay in Markdown",
    shot: "ink-03-export.png",
    timeout: 120000,
    async run(t) {
      const { id, body } = shared.note;
      const dir = join(dirname(t.app.library), "exports");
      mkdirSync(dir, { recursive: true });
      const make = (format) => t.app.run("export.make", { sources: [{ kind: "note", id }], format, path: join(dir, `ink.${format}`), replace: true });
      const html = readFileSync((await make("html")).path, "utf8");
      t.assert.match(html, /<svg [^>]*viewBox="0 0 1000 \d+"/);
      t.assert.ok(html.includes("<polyline"), "strokes drawn in the HTML");
      const md = readFileSync((await make("md")).path, "utf8");
      t.assert.ok(md.includes(body.split("\n\n")[1]), "the stroke data is kept in Markdown");
      const pdfPath = (await make("pdf")).path;
      t.assert.ok(existsSync(pdfPath));
      const pdf = readFileSync(pdfPath);
      let drawn = false;
      const seen = [];
      for (let at = pdf.indexOf("stream\n"); at >= 0 && !drawn; at = pdf.indexOf("stream\n", at + 7)) {
        const end = pdf.indexOf("endstream", at);
        let c = "";
        try {
          c = inflateSync(pdf.subarray(at + 7, end - 1)).toString("latin1");
        } catch { /* not a compressed stream */ }
        if (c.includes("BT")) seen.push(c.slice(0, 300));
        drawn = /1 J 1 j\n[\d. ]+ RG [\d.]+ w [\d. ]+ m [\d. l]+ S/.test(c);
      }
      t.assert.ok(drawn, "strokes drawn in the PDF: " + JSON.stringify(seen));
      // The Print or save preview shows the drawing too.
      await t.page.evaluate((id) => { location.hash = `#notes/note/${id}`; }, id);
      await t.current(".note-page");
      await t.page.locator(`${t.root} button[aria-label="Note actions"]`).click();
      await t.page.locator("button, [role=menuitem]", { hasText: /^Print or export…$/ }).first().click();
      const dlg = t.page.locator("dialog.export-dialog[open]");
      await dlg.locator(".paper svg").first().waitFor({ timeout: 15000 });
      await t.shot();
      await dlg.locator("button[aria-label=Close]").click();
    },
  },
  {
    row: "Pen, highlighter, three widths and the highlight colours are chosen in the block's tool strip",
    shot: "ink-04-tools.png",
    async run(t) {
      await fresh(t);
      await addInk(t);
      await tool(t, "Pen");
      await tool(t, "Thin");
      await tool(t, "Blue");
      await stroke(t, line(0.15));
      await tool(t, "Highlighter");
      await tool(t, "Pink");
      await tool(t, "Thick");
      await stroke(t, line(0.5));
      await tool(t, "Pen");
      await tool(t, "Green");
      await tool(t, "Medium");
      await stroke(t, line(0.85));
      await tool(t, "Purple");
      await tool(t, "Thick");
      await stroke(t, line(1.2));
      const [d] = await savedInk(t);
      t.assert.deepEqual(d.s.map((k) => [k.t, k.c, k.w]), [["pen", "blue", 1], ["hl", "pink", 3], ["pen", "green", 2], ["pen", "purple", 3]]);
      const fills = await t.page.locator(`${t.root} .ink-page path`).evaluateAll((ps) => ps.map((p) => [p.style.fill, p.style.fillOpacity]));
      t.assert.deepEqual(fills.map((f) => f[0]), ["var(--hl-blue)", "var(--hl-pink)", "var(--hl-green)", "var(--hl-purple)"]);
      t.assert.equal(fills[1][1], "0.38", "the highlighter is see-through");
      t.assert.equal(d.h > 320, true, "the page grew as the writing went down");
      await t.shot();
      await t.page.emulateMedia({ colorScheme: "dark" });
      await t.page.waitForTimeout(300);
      await t.shot("ink-04b-tools-dark.png");
      await t.page.emulateMedia({ colorScheme: "light" });
    },
  },
  {
    row: "The eraser removes a whole stroke; undo and redo step through changes",
    shot: "ink-05-erase-undo.png",
    async run(t) {
      await fresh(t);
      await addInk(t);
      await tool(t, "Pen");
      await stroke(t, line(0.15));
      await stroke(t, line(0.5));
      await stroke(t, line(0.85));
      t.assert.equal(await count(t), 3);
      await tool(t, "Eraser (removes a whole stroke)");
      await stroke(t, [[0.5, 0.4], [0.5, 0.6]]); // across the middle stroke only
      t.assert.equal(await count(t), 2, "one whole stroke gone");
      t.assert.equal((await savedInk(t))[0].s.length, 2);
      await tool(t, "Undo last stroke");
      t.assert.equal(await count(t), 3, "undo brings it back");
      await tool(t, "Redo");
      t.assert.equal(await count(t), 2, "redo removes it again");
      await tool(t, "Undo last stroke");
      await tool(t, "Undo last stroke");
      t.assert.equal(await count(t), 2, "undo also takes back the last drawn stroke");
      await t.page.locator(`${t.root} .ink-page`).focus();
      await t.press("Control+z");
      t.assert.equal(await count(t), 1, "Ctrl+Z with the page focused");
      await t.press("Control+y");
      t.assert.equal(await count(t), 2, "Ctrl+Y");
      t.assert.equal((await savedInk(t))[0].s.length, 2, "saved as shown");
      await t.shot();
    },
  },
  {
    row: "A pen's pressure is kept, its eraser end erases, and palm touches are ignored while a pen is in use",
    shot: "ink-06-pen.png",
    async run(t) {
      await fresh(t);
      await addInk(t);
      await tool(t, "Pen"); // the pen is shared by every block, and an earlier check left the eraser in hand
      await tool(t, "Black");
      await tool(t, "Medium");
      const press = Array.from({ length: 12 }, (_, i) => [0.1 + i * 0.06, 0.2 + Math.sin(i / 2) * 0.1, 0.1 + i * 0.07]);
      await pen(t, press);
      await pen(t, line(0.7).map(([x, y]) => [x, y, 0.5]));
      const [d] = await savedInk(t);
      t.assert.equal(d.s.length, 2, "two pen strokes saved");
      const pr = d.s[0].p.map((p) => p[2]);
      t.assert.ok(pr[0] >= 1 && pr[pr.length - 1] > pr[0] + 30, "pressure rises along the stroke: " + pr);
      // A palm (touch) while a pen has just been used draws nothing.
      await pen(t, line(0.45), { type: "touch", id: 9 });
      t.assert.equal(await count(t), 2, "touch ignored");
      // The eraser end (button 5) takes away a whole stroke.
      await pen(t, [[0.5, 0.6, 0.5], [0.5, 0.8, 0.5]], { button: 5, buttons: 32 });
      t.assert.equal(await count(t), 1, "pen eraser end");
      await t.shot();
    },
  },
];
