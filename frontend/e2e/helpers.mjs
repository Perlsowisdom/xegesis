// What the checks share: opening a fresh note or sermon in the real app, typing into it, and
// reading what the editor shows.

import assert from "node:assert/strict";

export function helpers(app, shotName) {
  const { page } = app;
  const t = {
    app,
    page,
    assert,
    shotTaken: false,
    // shot saves this check's screenshot (once the check has set the screen up).
    async shot(name = shotName) {
      t.shotTaken = true;
      await app.shot(name);
    },
    // openNote makes a note through note.create and opens its page in My work.
    async openNote(body = "", title = "E2E note") {
      const n = await app.run("note.create", { title, body });
      await page.evaluate((id) => { location.hash = `#notes/note/${id}`; }, n.id);
      await t.current(".note-page");
      t.id = n.id;
      return n;
    },
    async openSermon(body = "", title = "E2E sermon") {
      const d = await app.run("document.create", { kind: "sermon" });
      if (body || title) await app.run("document.update", { id: d.id, title, kind: "sermon", body });
      await page.evaluate((id) => { location.hash = `#sermons/${id}`; }, d.id);
      await t.current(".sermon-layout");
      t.id = d.id;
      return d;
    },
    // current marks the page now on screen (a Study tab may hold another, hidden, page) as the
    // one the check works in, and waits for its editor.
    async current(sel) {
      await page.waitForFunction((sel) => {
        const el = [...document.querySelectorAll(sel)].find((e) => e.offsetParent && !e.closest("#rightbar") && e.querySelector(".rich-text[contenteditable=true]"));
        if (!el) return false;
        document.querySelectorAll("[data-e2e]").forEach((e) => e.removeAttribute("data-e2e"));
        el.setAttribute("data-e2e", "current");
        return true;
      }, sel, { timeout: 15000 });
      await page.waitForTimeout(150);
      t.root = "[data-e2e=current]";
    },
    // still waits until a locator's element has stayed where it is for several frames. Pointing at
    // something that is still moving (the page settling, a redraw) loses the hover when it moves on.
    async still(loc) {
      const h = await loc.elementHandle();
      await h.evaluate((el) => new Promise((done) => {
        let last = "", same = 0;
        const tick = () => {
          const r = el.getBoundingClientRect();
          const now = [r.x, r.y, r.width, r.height].join();
          same = now === last ? same + 1 : 0;
          last = now;
          if (same >= 10) done(); else requestAnimationFrame(tick);
        };
        tick();
      }));
    },
    editor: () => page.locator(`${t.root} .rich-text`).first(),
    // focusEnd puts the caret at the end of the text, by clicking it and pressing Ctrl+End.
    async focusEnd() {
      await t.editor().click();
      await page.keyboard.press("Control+End");
    },
    async type(text) {
      await page.keyboard.type(text, { delay: 15 });
    },
    press: (k) => page.keyboard.press(k),
    html: () => t.editor().evaluate((e) => e.innerHTML),
    // blocks lists the editor's top-level blocks as tag names with their text.
    blocks: () => t.editor().evaluate((e) => [...e.children].map((c) => ({ tag: c.tagName.toLowerCase(), cls: c.className, text: c.textContent }))),
    // saved waits for the page to save and gives back the stored Markdown.
    async saved() {
      await page.keyboard.press("Control+s");
      await page.waitForFunction((root) => document.querySelector(`${root} .save-state`)?.getAttribute("data-state") === "saved", t.root, { timeout: 10000 });
      if (await page.locator(`${t.root}.note-page`).count()) return (await app.run("note.get", { id: t.id })).body;
      return (await app.run("document.get", { id: t.id })).body;
    },
  };
  return t;
}
