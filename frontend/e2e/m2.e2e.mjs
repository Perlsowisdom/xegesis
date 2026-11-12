// T19, the milestone 2 integration check: one study session in the real app, as a reader would do it
// in one sitting. Reports, notes and highlights, saved lists, collections, a reading plan, a sermon,
// printing and exporting, then a restart on the same library. The checks run in order and share
// what they made, but each one builds what it needs (the `need` helpers, by command) when an
// earlier one did not leave it, so a failure does not fail the rest. Each saves a screenshot.

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { inflateRawSync, inflateSync } from "node:zlib";
import { epub } from "./fixtures.mjs";
import { highlightWords, readRom12 } from "./highlights.e2e.mjs";

const ROM12 = 45012000;
const v = (n) => ROM12 + n;
// The two Bibles of the scratch library: the NET Bible (free to share, with a credit line) and the
// free NET (a personal copy, quoted up to about 2,000 characters at a time).
const NET_CREDIT = "Biblical Studies Press, L. L. C.";
const NETFREE_CREDIT = "1996-2005 Biblical Studies Press";
const AUTHOR = "M2 Integration Author";

// What the steps made, for the later steps and the restart.
const s = {};

const squash = (x) => x.replace(/\s+/g, "");

// quoteBy gives a verse quotation's Markdown from document.quote, for setting a step up by command.
async function quoteBy(app, ref, abbr) {
  const mods = await app.run("bible.modules", {});
  const m = (Array.isArray(mods) ? mods : mods.modules).find((x) => x.abbreviation === abbr || x.id === abbr);
  const q = await app.run("document.quote", { ref, module: m.id, preview: true });
  if (!q.allowed) throw new Error(`could not quote ${ref} from ${abbr}: ${q.reason}`);
  return { quote: q.quote.trim(), module: m.id };
}

// addBooks puts the three M2 books in the library (once) and waits until they are online.
async function addBooks(t) {
  if (s.books) return;
  const { app, page } = t;
  const dir = join(dirname(app.library), "m2-books");
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, "m2-sermon-on-the-mount.epub"), epub("M2 Sermon on the Mount", { author: AUTHOR }));
  writeFileSync(join(dir, "m2-letters-of-paul.epub"), epub("M2 Letters of Paul", { author: AUTHOR }));
  writeFileSync(join(dir, "m2-other-book.epub"), epub("M2 Other Book", { author: "Someone Else" }));
  const res = await app.run("library.add", { paths: [dir] });
  t.assert.ok(res.added >= 3, JSON.stringify(res));
  for (let i = 0; i < 40; i++) {
    const r = await app.run("library.items", { text: "M2 ", limit: 10 });
    if (r.items.filter((x) => x.status === "online").length >= 3) break;
    await page.waitForTimeout(250);
  }
  s.books = true;
}

// Each step makes what it needs: need.* builds a thing by command when an earlier step did not leave
// it, so one failing step does not fail the ones after it. A step that tests the making itself
// (through the screen) still does that, and sets what it made.
const need = {
  async highlight(t) {
    if (s.highlight && s.phrase) return;
    const { app } = t;
    const { module } = await quoteBy(app, "Rom 12:9", "ENGNET");
    // The first three words of the verse, as a reader selects them.
    const text = (await app.run("bible.read", { module, ref: "Rom 12:9" })).verses[0].text;
    const end = /^\s*(\S+\s+){2}\S+/.exec(text)[0].length;
    const words = text.slice(0, end).trim();
    const range = { start: { verse: v(9), offset: 0 }, end: { verse: v(9), offset: end } };
    const made = await app.run("highlight.create", { range, module, quote: words, style: "green" });
    s.highlight = (await app.run("highlight.list", { ref: "Rom 12:9" })).find((h) => h.id === made.id);
    s.phrase = words.replace(/\s+/g, " ");
  },
  async note(t) {
    if (s.note) return;
    await need.highlight(t);
    const { quote } = await quoteBy(t.app, "Rom 12:10", "ENGNET");
    const body = `Love must be without hypocrisy.\n\n> ${s.phrase}\n\n${quote}`;
    const n = await t.app.run("note.create", { title: "Sincere love", ref: "Rom 12:9", body });
    s.note = n.id;
    s.noteBody = (await t.app.run("note.get", { id: n.id })).body;
  },
  async sermon(t) {
    if (s.sermon) return;
    const { app } = t;
    await need.highlight(t);
    if (!s.folder) s.folder = (await app.run("folder.create", { name: "M2 Sunday" })).id;
    const { quote } = await quoteBy(app, "Rom 12:9-10", "NETfree");
    const body = `## Love without pretence\n\nPaul's first command.\n\n${quote}\n\n### Hold on to what is good\n\n> ${s.phrase}\n\n## Love in the church\n\n| Duty | Verse |\n| --- | --- |\n| Bless | 12:14 |\n`;
    const d = await app.run("document.create", { title: "Love in action", folder_id: s.folder, passage: "Rom 12:9-21", body,
      points: [{ h: "Love without pretence", level: 1 }, { h: "Hold on to what is good", level: 2 }, { h: "Love in the church", level: 1 }] });
    s.sermon = d.id;
    s.sermonBody = (await app.run("document.get", { id: d.id })).body;
  },
  async list(t) {
    if (s.list) return;
    const l = await t.app.run("list.create", { kind: "verses", name: "M2 verses on love" });
    await t.app.run("list.add", { list: l.id, ref: "Rom 12:11-12" });
    s.list = l.id;
  },
  async collection(t) {
    if (s.collection) return;
    await addBooks(t);
    s.collection = (await t.app.run("collection.create", { name: "M2 by the author", rule: { author: "M2 Integration" } })).id;
  },
  async plan(t) {
    if (s.plan) return;
    const { app } = t;
    const p = await app.run("plan.create", { template: "nt-90" });
    await app.run("plan.mark", { plan_id: p.id });
    s.plan = p.id;
  },
};

async function mode(page, name) {
  const b = page.locator(".shell button, header button", { hasText: new RegExp(`^${name}$`) }).first();
  if (await b.getAttribute("aria-pressed") !== "true") await b.click();
}

const verseShown = (page, id) => page.waitForFunction((id) => [...document.querySelectorAll(`.doc-bible .vt[data-tb="${id}"]`)].some((e) => e.offsetParent), id, { timeout: 15000 });

async function moreMenu(page, item) {
  await page.locator(".reader .topbar button[aria-label=More]:visible").first().click();
  await page.locator(".popover .menu-list button", { hasText: item }).click();
}

// slashInsert types a / command and picks the entry `down` places below the first.
async function slashInsert(t, cmd, down = 0) {
  await t.type(cmd);
  await t.page.waitForSelector(".slash-menu:not([hidden])");
  for (let i = 0; i < down; i++) await t.press("ArrowDown");
  await t.press("Enter");
}

// insertHighlight inserts the highlight with these words through the / menu.
async function insertHighlight(t, phrase) {
  await slashInsert(t, "/highlight");
  await t.page.locator(".popover input").first().waitFor();
  await t.type(phrase);
  await t.page.waitForFunction(() => [...document.querySelectorAll(".popover .pick-row")].some((r) => r.textContent.includes("12:9")), null, { timeout: 8000 });
  await t.press("Enter");
}

// quoteVerses opens Insert verse text, picks the Bible and types the verses; it gives back the
// popover's refusal, if the licence refused.
async function quoteVerses(t, ref, bible) {
  const { page } = t;
  await slashInsert(t, "/quote", 1);
  const input = page.locator(".popover .quote-pop input[aria-label='Verses to quote']");
  await input.waitFor();
  if (bible) {
    const chip = page.locator(".popover .bible-chip");
    await page.waitForFunction(() => !/No Bible/.test(document.querySelector(".popover .bible-chip")?.textContent ?? "No Bible"), null, { timeout: 8000 });
    if ((await chip.textContent()).trim() !== bible) {
      await chip.click();
      await page.locator(".popover .bible-list button", { hasText: new RegExp(`^${bible}$`) }).click();
    }
  }
  await input.click();
  await t.press("Control+a");
  await t.type(ref);
  await page.waitForFunction(() => !document.querySelector(".popover .quote-pop button[type=submit]")?.disabled, null, { timeout: 8000 });
  await t.press("Enter");
  const why = page.locator(".popover .ref-note.bad");
  const out = await Promise.race([
    why.filter({ hasText: /\S/ }).waitFor({ timeout: 8000 }).then(async () => (await why.textContent()).trim()),
    page.locator(".popover .quote-pop").waitFor({ state: "detached", timeout: 8000 }).then(() => ""),
  ]);
  return out;
}

async function saveNow(page, root) {
  await page.keyboard.press("Control+s");
  await page.waitForFunction((root) => document.querySelector(`${root} .save-state`)?.getAttribute("data-state") === "saved", root, { timeout: 10000 });
}

// stubDialogs stands in for the two system dialogs a test cannot answer: the save dialog answers
// with a path in the exports folder, and the print dialog only counts that it was asked for.
async function stubDialogs(page, dir) {
  await page.evaluate((dir) => {
    window.__printed = 0;
    window.print = () => {
      window.__printed++;
      window.__printedText = document.getElementById("print-root")?.textContent ?? "";
      window.dispatchEvent(new Event("afterprint"));
    };
    window.go.main.App.SaveFile = async (_title, name) => `${dir}\\${name}`;
  }, dir);
}

// saveAs presses a format's button in the open Print or save dialog and gives back the file's path.
async function saveAs(page, label) {
  const dlg = page.locator("dialog.export-dialog[open]");
  const result = dlg.locator(".msg.ok", { hasText: `Saved ${label}:` });
  await dlg.locator(".export-actions button", { hasText: new RegExp(`^${label}$`) }).click();
  await result.waitFor({ timeout: 20000 });
  return (await result.textContent()).replace(`Saved ${label}: `, "").split(" — ")[0].trim();
}

// unzip gives a zip's files by name (enough for a .docx).
function unzip(buf) {
  const out = {};
  let end = buf.length - 22;
  while (end >= 0 && buf.readUInt32LE(end) !== 0x06054b50) end--;
  let p = buf.readUInt32LE(end + 16);
  for (let i = 0, n = buf.readUInt16LE(end + 10); i < n; i++) {
    const method = buf.readUInt16LE(p + 10);
    const size = buf.readUInt32LE(p + 20);
    const nameLen = buf.readUInt16LE(p + 28);
    const extra = buf.readUInt16LE(p + 30);
    const comment = buf.readUInt16LE(p + 32);
    const local = buf.readUInt32LE(p + 42);
    const name = buf.toString("utf8", p + 46, p + 46 + nameLen);
    const start = local + 30 + buf.readUInt16LE(local + 26) + buf.readUInt16LE(local + 28);
    const data = buf.subarray(start, start + size);
    out[name] = (method === 8 ? inflateRawSync(data) : data).toString("utf8");
    p += 46 + nameLen + extra + comment;
  }
  return out;
}

// pdfText reads the text of a PDF Xegesis wrote: each font's ToUnicode map turns the glyph numbers
// of its text runs back into letters.
function pdfText(buf) {
  const src = buf.toString("latin1");
  const objs = new Map();
  for (const m of src.matchAll(/(\d+) 0 obj\s*([\s\S]*?)endobj/g)) {
    let body = m[2];
    const st = /stream\r?\n([\s\S]*?)\r?\nendstream/.exec(body);
    let stream = null;
    if (st) {
      const raw = Buffer.from(st[1], "latin1");
      stream = /FlateDecode/.test(body.slice(0, st.index)) ? inflateSync(raw).toString("latin1") : st[1];
      body = body.slice(0, st.index);
    }
    objs.set(Number(m[1]), { dict: body, stream });
  }
  const cmaps = new Map();
  const cmap = (n) => {
    if (cmaps.has(n)) return cmaps.get(n);
    const map = new Map();
    for (const m of (objs.get(n)?.stream ?? "").matchAll(/<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>/g)) {
      const u = m[2].match(/.{4}/g).map((x) => String.fromCharCode(parseInt(x, 16))).join("");
      map.set(m[1].toUpperCase(), u);
    }
    cmaps.set(n, map);
    return map;
  };
  let text = "";
  for (const [, o] of objs) {
    if (!/\/Type\s*\/Page\b/.test(o.dict)) continue;
    const fonts = {};
    for (const m of o.dict.matchAll(/\/(F\w*)\s+(\d+) 0 R/g)) {
      const tu = /\/ToUnicode (\d+) 0 R/.exec(objs.get(Number(m[2]))?.dict ?? "");
      if (tu) fonts[m[1]] = cmap(Number(tu[1]));
    }
    const content = /\/Contents (\d+) 0 R/.exec(o.dict);
    let font = null;
    for (const m of (objs.get(Number(content?.[1]))?.stream ?? "").matchAll(/\/(F\w*) [\d.]+ Tf|<([0-9A-F]*)> Tj/g)) {
      if (m[1]) font = fonts[m[1]];
      else if (font) text += m[2].match(/.{4}/g)?.map((g) => font.get(g) ?? "").join("") + " ";
    }
  }
  return text;
}

export default [
  {
    row: "M2 step 1: a passage in Read mode, then in a Study pane",
    shot: "m2-01-study.png",
    async run(t) {
      const { page } = t;
      await page.evaluate(() => { location.hash = "#passage/Rom.12"; });
      await mode(page, "Read");
      await verseShown(page, v(9));
      t.assert.equal(await page.locator(".reader").getAttribute("data-mode"), "read");
      await t.shot("m2-01a-read.png");
      await mode(page, "Study");
      await page.waitForFunction(() => document.querySelector(".reader")?.dataset.mode === "study");
      await verseShown(page, v(9));
      t.assert.ok(await page.locator(".doc-bible .vt[data-tb='45012009']:visible").count(), "Romans 12:9 is in a Study pane");
      await t.shot();
    },
  },
  {
    row: "M2 step 2: a passage report and a word report on it",
    shot: "m2-02-reports.png",
    async run(t) {
      const { page } = t;
      await moreMenu(page, "Passage report");
      const title = page.locator(".report-head h2:visible");
      await title.waitFor({ timeout: 20000 });
      t.assert.match(await title.textContent(), /Romans 12/);
      await page.locator(".report .report-section:visible").first().waitFor();
      await t.shot("m2-02a-passage-report.png");
      // A word from the report, or (in a Bible without word tags) the report form.
      const word = page.locator(".report .word-link:visible").first();
      if (await word.count()) {
        await word.click();
        t.note = "word report from a word in the passage report";
      } else {
        await page.locator(".report-form select").first().selectOption("word");
        await page.locator(".report-form input[type=search]").fill("love");
        await page.locator(".report-form button[type=submit]").click();
        t.note = "word report from the report form (the Bible has no word tags)";
      }
      await page.waitForFunction(() => {
        const h = document.querySelector(".report-head h2");
        return h && h.offsetParent && !/Romans 12/.test(h.textContent);
      }, null, { timeout: 20000 });
      await page.locator(".report .report-section:visible").first().waitFor();
      s.wordReport = (await title.textContent()).trim();
      await t.shot();
    },
  },
  {
    row: "M2 step 3: highlight part of a verse and write a note on the passage with it and a verse quote",
    shot: "m2-03-note.png",
    async run(t) {
      const { page, app } = t;
      await readRom12(t, 9, 10);
      const { a } = await highlightWords(t, [v(9), 0, 3], null, "green");
      s.phrase = a.text.replace(/\s+/g, " ");
      s.highlight = (await app.run("highlight.list", { ref: "Rom 12:9" })).find((h) => h.range);
      t.assert.ok(s.highlight, "the highlight is kept with its words");

      await moreMenu(page, "Notes on this passage");
      const side = "#rightbar .notes-panel";
      // The sidebar may still show a note an earlier check left open there: back to the list first.
      const back = page.locator(`${side} .side-page:not([hidden]) .crumb-back`);
      if (await back.count()) await back.click();
      await page.locator(`${side} .side-new`).click();
      const root = "#rightbar .note-page";
      await page.locator(`${root} .rich-text[contenteditable=true]`).waitFor();
      await page.locator(`${root} .page-title`).click();
      await t.type("Sincere love");
      await t.press("Enter");
      t.root = root;
      await t.type("Love must be without hypocrisy.");
      await t.press("Enter");
      await insertHighlight(t, s.phrase);
      await t.editor().locator(".verse-quote").first().waitFor({ timeout: 8000 });
      t.assert.equal(await quoteVerses(t, "Rom 12:10", "ENGNET"), "");
      await page.waitForFunction((r) => document.querySelectorAll(`${r} .rich-text .verse-quote`).length === 2, root, { timeout: 8000 });
      await saveNow(page, root);
      const n = (await app.run("note.list", { ref: "Rom 12:9" })).find((x) => x.title === "Sincere love");
      t.assert.ok(n, "the note is on the passage");
      s.note = n.id;
      s.noteBody = n.body;
      s.noteInSidebar = true;
      t.assert.ok(n.body.includes(`> ${s.phrase}`), `the highlight is quoted:\n${n.body}`);
      t.assert.ok(n.body.includes("[Romans 12:10](#passage/Rom.12.10)"), "the verse quote links back");
      t.assert.ok(n.body.includes(NET_CREDIT), "the verse quote carries the NET Bible's credit line");
      await t.shot();
    },
  },
  {
    row: "M2 step 4: edit the note, then restore an earlier version",
    shot: "m2-04-history.png",
    async run(t) {
      const { page, app } = t;
      await need.note(t);
      if (s.noteInSidebar) {
        await page.locator("#rightbar .note-page button[aria-label='Note actions']").click();
        await page.locator(".popover .menu-list button", { hasText: "Open in a tab" }).click();
      } else {
        await page.evaluate((id) => { location.hash = `#notes/note/${id}`; }, s.note);
      }
      await t.current(".note-page");
      t.id = s.note;
      await t.focusEnd();
      await t.press("Enter");
      await t.type("A thought to take back.");
      const edited = await t.saved();
      t.assert.ok(edited.includes("A thought to take back."), "the edit is saved");
      await t.shot("m2-04a-edited.png");

      await page.locator(`${t.root} button[aria-label='Note actions']`).click();
      await page.locator(".popover .menu-list button", { hasText: "History" }).click();
      const drawer = page.locator("dialog.drawer[open]");
      await drawer.locator("button", { hasText: "Restore this version" }).first().waitFor();
      await t.shot("m2-04b-history.png");
      await drawer.locator("button", { hasText: "Restore this version" }).last().click();
      await page.locator(`${t.root} .msg.ok`, { hasText: "Note restored to an earlier version." }).waitFor();
      const back = (await app.run("note.get", { id: s.note })).body;
      t.assert.equal(back, s.noteBody, "the note is as it was before the edit");
      await page.waitForFunction((r) => !document.querySelector(`${r} .rich-text`)?.textContent.includes("A thought to take back."), t.root, { timeout: 8000 });
      await t.shot();
    },
  },
  {
    row: "M2 step 5: add the passage to a saved list",
    shot: "m2-05-list.png",
    async run(t) {
      const { page, app } = t;
      await readRom12(t);
      // The toolbar closes on any scroll, and a scroll event arrives a frame late: a click that first
      // scrolls its verse into view would lose the toolbar it opens. Bring both verses on screen, wait
      // for the page to stop moving, then pick them, as a reader does.
      const nums = [11, 12].map((n) => page.locator(`.doc-bible .vnum[aria-label="verse ${n}"]:visible`).first());
      for (const num of nums) await num.scrollIntoViewIfNeeded();
      await t.settled();
      for (const num of nums) await num.click();
      await page.locator(".mark-bar button[aria-label='Add to a list']").click();
      const dlg = page.locator("dialog.drawer[open]", { hasText: "to a list" });
      await dlg.locator("input[type=text]").fill("M2 verses on love");
      await dlg.locator("button", { hasText: "Create and add" }).click();
      await page.locator("#flash.show", { hasText: "Added to" }).waitFor({ timeout: 8000 });
      const list = (await app.run("list.list", {})).find((l) => l.name === "M2 verses on love");
      t.assert.ok(list, "the list is made");
      s.list = list.id;
      const { entries } = await app.run("list.items", { id: list.id });
      t.assert.equal(entries.length, 1, JSON.stringify(entries));
      t.assert.deepEqual([entries[0].ref.start, entries[0].ref.end], [v(11), v(12)]);
      await page.evaluate(() => { location.hash = "#lists"; });
      await page.locator(".view:not([hidden])", { hasText: "M2 verses on love" }).first().waitFor();
      await t.shot();
    },
  },
  {
    row: "M2 step 6: a collection with a rule holds the books it should",
    shot: "m2-06-collection.png",
    async run(t) {
      const { page, app } = t;
      await addBooks(t);
      await page.evaluate(() => { location.hash = "#library"; });
      await page.locator(".collections button", { hasText: "New collection…" }).click();
      const form = page.locator(".collections .card:not([hidden])");
      await form.getByLabel("Name").fill("M2 by the author");
      await form.getByLabel("Author").fill("M2 Integration");
      await form.locator("[role=status]", { hasText: /^2 books/ }).waitFor({ timeout: 10000 });
      await form.locator("button", { hasText: "Make collection" }).click();
      await page.getByText("Made M2 by the author with 2 books.").waitFor({ timeout: 10000 });
      // An earlier check may have left words in the Library's search box.
      const search = page.locator("input.work-search:visible").first();
      if (await search.count()) await search.fill("");
      const node = (await app.run("collection.list", {})).tree.find((c) => c.name === "M2 by the author");
      t.assert.ok(node, "the collection is made");
      s.collection = node.id;
      const items = await app.run("collection.items", { collection_id: node.id });
      const titles = (items.items ?? items).map((x) => x.title).sort();
      t.assert.deepEqual(titles, ["M2 Letters of Paul", "M2 Sermon on the Mount"]);
      // The Library shows its books.
      await page.locator(".coll-tree", { hasText: "M2 by the author" }).getByText("M2 by the author").first().click();
      await page.locator(".book-tile:visible, .lib-table tbody tr:visible").filter({ hasText: "M2 Letters of Paul" }).first().waitFor();
      t.assert.equal(await page.locator(".book-tile:visible, .lib-table tbody tr:visible").filter({ hasText: "M2 Other Book" }).count(), 0);
      await t.shot();
    },
  },
  {
    row: "M2 step 7: start a reading plan, mark today's reading done, and see the progress",
    shot: "m2-07-plan.png",
    async run(t) {
      const { page, app } = t;
      await page.evaluate(() => { location.hash = "#plans"; });
      const form = page.locator("form.plan-start");
      await form.waitFor();
      await form.getByLabel("Plan", { exact: true }).selectOption("nt-90");
      await form.locator("button[type=submit]", { hasText: "Start plan" }).click();
      const mark = page.locator("button[aria-label^=\"Mark today's reading of\"]:visible").first();
      await mark.waitFor({ timeout: 10000 });
      await mark.click();
      const plan = (await app.run("plan.list", {})).find((p) => /New Testament/.test(p.name));
      t.assert.ok(plan, "the plan is started");
      s.plan = plan.id;
      t.assert.equal(plan.days_done, 1);
      const bar = page.locator(`.card.plan progress`).first();
      await page.waitForFunction(() => Number(document.querySelector(".card.plan progress")?.getAttribute("value")) === 1, null, { timeout: 8000 });
      s.progress = await bar.getAttribute("aria-label");
      await t.shot();
    },
  },
  {
    row: "M2 step 8: a sermon in a folder with a title, an outline of headings, verses, the highlight and a table",
    shot: "m2-08-sermon.png",
    timeout: 180000,
    async run(t) {
      const { page, app } = t;
      await need.highlight(t);
      await page.evaluate(() => { location.hash = "#notes"; });
      await page.locator("button[aria-label='New folder']:visible").click();
      await page.locator("input[aria-label='Name of the new folder']").fill("M2 Sunday");
      await t.press("Enter");
      // Enter starts folder.create; wait for it to land instead of reading the list at once.
      let folder;
      for (let i = 0; i < 50 && !folder; i++) {
        const r = await app.run("folder.list", {});
        folder = (Array.isArray(r) ? r : r.folders ?? []).find((f) => f.name === "M2 Sunday");
        if (!folder) await page.waitForTimeout(100);
      }
      t.assert.ok(folder, "the folder is made");
      s.folder = folder.id;
      await page.waitForFunction(() => /folder/.test(location.hash), null, { timeout: 8000 });
      await page.locator(".new-btn:visible").click();
      await page.locator(".popover .menu-list button", { hasText: "Sermon in M2 Sunday" }).click();
      await t.current(".sermon-layout");
      const made = await app.run("document.list", { folder_id: s.folder });
      t.assert.equal(made.length, 1, "the new sermon is filed in the folder");
      s.sermon = made[0].id;
      t.id = s.sermon;

      // A new sermon's title starts empty and takes the keyboard.
      await page.waitForFunction((r) => document.activeElement === document.querySelector(`${r} .page-title`), t.root, { timeout: 5000 });
      t.assert.equal(await page.locator(`${t.root} .page-title`).textContent(), "");
      await t.type("Love in action");
      await page.locator(`${t.root} .page-props button`, { hasText: "+ passage" }).click();
      await page.locator(".popover input[aria-label=Reference]").fill("Rom 12:9-21");
      await t.press("Enter");
      await page.locator(`${t.root} .page-props`, { hasText: "Romans 12:9" }).waitFor();

      await t.focusEnd();
      await t.type("## Love without pretence");
      await t.press("Enter");
      await t.type("Paul's first command.");
      await t.press("Enter");
      // A whole chapter of the personal-copy NET is over its licence's quoting limit: refused.
      s.refusal = await quoteVerses(t, "Rom 12", "NETfree");
      t.assert.match(s.refusal, /^Not quoted: the licence of NETfree allows quoting about 2000 characters at a time/);
      await t.shot("m2-08a-refused.png");
      await page.locator(".popover .quote-pop input[aria-label='Verses to quote']").click();
      await t.press("Control+a");
      await t.type("Rom 12:9-10");
      await page.waitForFunction(() => !document.querySelector(".popover .quote-pop button[type=submit]")?.disabled, null, { timeout: 8000 });
      await t.press("Enter");
      await t.editor().locator(".verse-quote").first().waitFor({ timeout: 8000 });
      await t.focusEnd();
      await t.type("### Hold on to what is good");
      await t.press("Enter");
      await insertHighlight(t, s.phrase);
      await page.waitForFunction((r) => document.querySelectorAll(`${r} .rich-text .verse-quote`).length === 2, t.root, { timeout: 8000 });
      await t.focusEnd();
      await t.type("## Love in the church");
      await t.press("Enter");
      await slashInsert(t, "/table2x2");
      await t.editor().locator("table").first().waitFor();
      for (const [i, cell] of ["Duty", "Verse", "Bless", "12:14"].entries()) {
        if (i) await t.press("Tab");
        await t.type(cell);
      }
      const body = await t.saved();
      const d = await app.run("document.get", { id: s.sermon });
      t.assert.equal(d.title, "Love in action");
      t.assert.equal(d.folder_id, s.folder);
      t.assert.deepEqual(d.points.map((p) => [p.h, p.level ?? 1]), [["Love without pretence", 1], ["Hold on to what is good", 2], ["Love in the church", 1]], JSON.stringify(d.points));
      t.assert.equal(d.passage?.display, "Romans 12:9-21");
      t.assert.ok(body.includes("[Romans 12:9-10](#passage/Rom.12.9-Rom.12.10)") || body.includes("Romans 12:9-10"), body);
      t.assert.ok(body.includes(NETFREE_CREDIT), "the NETfree credit line is in the text");
      t.assert.ok(body.includes(`> ${s.phrase}`), "the highlight is quoted");
      t.assert.match(body, /\| Duty \| Verse \|/);
      s.sermonBody = body;
      await t.shot();
    },
  },
  {
    row: "M2 step 9: print and export the sermon (HTML, Word, PDF) and the note (Markdown), with credit lines",
    shot: "m2-09-export.png",
    timeout: 180000,
    async run(t) {
      const { page } = t;
      await need.sermon(t);
      await need.note(t);
      const dir = join(dirname(t.app.library), "exports");
      mkdirSync(dir, { recursive: true });
      await stubDialogs(page, dir);
      await page.evaluate((id) => { location.hash = `#sermons/${id}`; }, s.sermon);
      await t.current(".sermon-layout");
      await page.locator(`${t.root} button`, { hasText: "Print or save…" }).click();
      const dlg = page.locator("dialog.export-dialog[open]");
      await dlg.locator(".paper h1, .paper h2").first().waitFor({ timeout: 15000 });
      const paper = await dlg.locator(".paper").textContent();
      for (const want of ["Love without pretence", "Hold on to what is good", "Love in the church", NETFREE_CREDIT, "Duty"]) t.assert.ok(paper.includes(want), `the preview has "${want}"`);
      await t.shot("m2-09a-preview.png");
      await dlg.locator(".export-actions button", { hasText: /^Print$/ }).click();
      await page.waitForFunction(() => window.__printed === 1, null, { timeout: 10000 });
      t.assert.ok((await page.evaluate(() => window.__printedText)).includes(NETFREE_CREDIT), "the printed page has the credit line");

      const files = {};
      for (const [label, ext] of [["Web page", "html"], ["Word", "docx"], ["PDF", "pdf"]]) {
        files[ext] = await saveAs(page, label);
        t.assert.ok(files[ext].endsWith(`.${ext}`) && existsSync(files[ext]), `${label}: ${files[ext]}`);
      }
      await dlg.locator("button[aria-label=Close]").click();
      const html = readFileSync(files.html, "utf8");
      const docx = unzip(readFileSync(files.docx))["word/document.xml"] ?? "";
      const pdf = readFileSync(files.pdf);
      t.assert.equal(pdf.subarray(0, 5).toString(), "%PDF-");
      const pdfWords = squash(pdfText(pdf));
      for (const want of ["Love without pretence", "Love in the church", NETFREE_CREDIT, s.phrase.split(" ")[0]]) {
        t.assert.ok(html.includes(want), `HTML has "${want}"`);
        t.assert.ok(docx.includes(want), `Word has "${want}"`);
        t.assert.ok(pdfWords.includes(squash(want)), `PDF has "${want}"`);
      }
      t.assert.ok(/<table/.test(html) && /<w:tbl>/.test(docx), "the table is exported");
      // The outline: two main points numbered 1 and 2, the sub-point between them.
      t.assert.ok(html.includes(`<ol start="2"><li dir="auto">Love in the church</li></ol>`), "HTML numbers the second main point 2");
      t.assert.ok(pdfWords.includes("2.Loveinthechurch"), "PDF numbers the second main point 2");

      // The note, to Markdown, from its page.
      await page.evaluate((id) => { location.hash = `#notes/note/${id}`; }, s.note);
      await t.current(".note-page");
      await page.locator(`${t.root} button[aria-label='Note actions']`).click();
      await page.locator(".popover .menu-list button", { hasText: "Print or export…" }).click();
      await page.locator("dialog.export-dialog[open] .paper *").first().waitFor({ timeout: 15000 });
      files.md = await saveAs(page, "Markdown");
      const md = readFileSync(files.md, "utf8");
      t.assert.ok(md.includes(s.phrase) && md.includes(NET_CREDIT), md);
      await t.shot();
      await page.locator("dialog.export-dialog[open] button[aria-label=Close]").click();
      s.files = files;
    },
  },
  {
    row: "M2 step 10: after a restart on the same library, everything is still there",
    shot: "m2-10-restart.png",
    timeout: 180000,
    async run(t) {
      const { app } = t;
      // What the earlier steps made must survive; whatever one of them did not leave is made now.
      for (const k of ["note", "list", "collection", "sermon", "plan"]) await need[k](t);
      await need.highlight(t);
      await app.restart();
      const page = app.page;
      t.assert.equal((await app.run("note.get", { id: s.note })).body, s.noteBody, "the note, as restored");
      t.assert.ok((await app.run("highlight.list", { ref: "Rom 12:9" })).some((h) => h.id === s.highlight.id), "the highlight");
      t.assert.equal((await app.run("list.items", { id: s.list })).entries.length, 1, "the saved list");
      const items = await app.run("collection.items", { collection_id: s.collection });
      t.assert.equal((items.items ?? items).length, 2, "the collection");
      const d = await app.run("document.get", { id: s.sermon });
      t.assert.equal(d.body, s.sermonBody, "the sermon");
      t.assert.equal(d.folder_id, s.folder, "the sermon's folder");
      for (const f of Object.values(s.files ?? {})) t.assert.ok(existsSync(f), f);

      // And on screen.
      await page.evaluate(() => { location.hash = "#plans"; });
      await page.waitForFunction(() => Number(document.querySelector(".card.plan progress")?.getAttribute("value")) === 1, null, { timeout: 10000 });
      if (s.progress) t.assert.equal(await page.locator(".card.plan progress").first().getAttribute("aria-label"), s.progress);
      await page.evaluate(() => { location.hash = "#passage/Rom.12"; });
      await mode(page, "Read");
      await page.locator(`.doc-bible .vt[data-tb="${v(9)}"] mark[data-hl="${s.highlight.id}"]:visible`).first().waitFor({ timeout: 15000 });
      await page.evaluate((id) => { location.hash = `#sermons/${id}`; }, s.sermon);
      const sermon = page.locator(".sermon-layout:visible .rich-text");
      await sermon.first().waitFor({ timeout: 15000 });
      for (const want of ["Love without pretence", "Love in the church", "Duty"]) {
        await page.locator(".sermon-layout:visible .rich-text", { hasText: want }).first().waitFor({ timeout: 8000 });
      }
      t.assert.ok((await page.locator(".sermon-layout:visible .crumbs").first().textContent()).includes("M2 Sunday"), "filed in its folder");
      await t.shot();
    },
  },
];
