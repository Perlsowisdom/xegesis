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
];
