// npm run e2e: builds the desktop app, starts it on a copy of the scratch library, runs every
// *.e2e.mjs file in this folder against it by typing with real key events, and closes the app.
//
//   npm run e2e                     build, then run everything
//   npm run e2e -- --no-build       use the last build
//   npm run e2e -- markdown         only files or checks whose name contains "markdown"
//
// Each file's default export is a list of checks { row, shot, run(t) }. A check passes when run
// returns; it saves its screenshot to e2e/shots/<shot>. Results go to e2e/shots/results.md.

import { readdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { shots, startApp } from "./app.mjs";
import { helpers } from "./helpers.mjs";

const args = process.argv.slice(2);
const filters = args.filter((a) => !a.startsWith("--")).map((a) => a.toLowerCase());
const here = import.meta.dirname;
const files = readdirSync(here).filter((f) => f.endsWith(".e2e.mjs")).sort();

const app = await startApp({ build: !args.includes("--no-build") });
const stop = async () => {
  await app.stop();
};
process.on("SIGINT", () => void stop().finally(() => process.exit(130)));

const results = [];
try {
  for (const f of files) {
    const checks = (await import(pathToFileURL(join(here, f)).href)).default;
    for (const c of checks) {
      if (filters.length && !filters.some((x) => f.toLowerCase().includes(x) || c.row.toLowerCase().includes(x))) continue;
      const t = helpers(app, c.shot);
      const started = Date.now();
      try {
        await c.run(t);
        if (!t.shotTaken) await t.shot();
        results.push({ file: f, row: c.row, ok: true, shot: c.shot });
        console.log(`PASS  ${c.row}  (${Date.now() - started} ms)`);
      } catch (e) {
        await app.shot(`FAIL-${c.shot}`).catch(() => {});
        results.push({ file: f, row: c.row, ok: false, shot: `FAIL-${c.shot}`, error: String(e?.message ?? e).split("\n")[0] });
        console.log(`FAIL  ${c.row}\n      ${String(e?.stack ?? e).split("\n").slice(0, 4).join("\n      ")}`);
      }
    }
  }
} finally {
  await stop();
}

const failed = results.filter((r) => !r.ok).length;
writeFileSync(join(shots, "results.md"), [
  "| Check | Result | Screenshot |", "| --- | --- | --- |",
  ...results.map((r) => `| ${r.row.replace(/\|/g, "\|")} | ${r.ok ? "PASS" : `FAIL: ${r.error.replace(/\|/g, "\|")}`} | ${r.shot} |`),
].join("\n") + "\n");
console.log(`\n${results.length - failed} passed, ${failed} failed (${results.length} checks). Screenshots in e2e/shots.`);
process.exit(failed ? 1 : 0);
