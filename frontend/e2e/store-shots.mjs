// Takes the Microsoft Store screenshots (1366x768) from the real app on a copy of the test library:
//   npm run store-shots
// They go to docs/store/screenshots/. Nothing of the user's own library is used: startApp copies the
// scratch library (XEGESIS_E2E_SOURCE) and the app runs on the copy. The notes and sermon shown are
// made here, from plain sample text, and live only in that copy.

import { mkdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { repo, startApp } from "./app.mjs";

const out = join(repo, "docs", "store", "screenshots");
rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

const app = await startApp({ build: !process.argv.includes("--no-build") });
try {
  const { page } = app;
  await page.setViewportSize({ width: 1366, height: 768 });
  const go = async (hash, waitFor) => {
    await page.evaluate((h) => { location.hash = h; }, hash);
    if (waitFor) await page.waitForSelector(waitFor, { state: "visible", timeout: 20000 });
    await page.waitForTimeout(800);
  };
  const shot = (name) => page.screenshot({ path: join(out, name), timeout: 20000 });

  await go("#passage/Rom.8", ".doc-bible .vt");
  await shot("01-reading-romans-8.png");

  const note = await app.run("note.create", {
    title: "Romans 8: no condemnation",
    body: "Paul moves from the struggle of chapter 7 to life in the Spirit.\n\nKey thought: there is now no condemnation for those who are in Christ Jesus.",
    ref: "Rom 8:1",
  });
  await go(`#notes/note/${note.id}`, ".note-page .rich-text");
  await shot("02-writing-a-note.png");

  await go("#search", "main");
  await page.getByPlaceholder("e.g. love one another").fill("no condemnation");
  await page.locator("button.primary", { hasText: /^Search$/ }).click();
  await page.waitForTimeout(2500);
  await shot("03-search.png");

  await go("#bibles", "main");
  await shot("04-get-bibles.png");

  await go("#settings/privacy", ".settings-section");
  await shot("05-privacy-settings.png");
  console.log(`Screenshots saved in ${out}`);
} finally {
  await app.stop();
}
