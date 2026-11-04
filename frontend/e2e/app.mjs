// Starts the real desktop app (Wails on WebView2) for the interface tests and connects to it over
// the Chrome DevTools Protocol. The app runs on a temporary copy of the scratch library, with its
// own WebView2 profile, so the user's library and settings are never touched. stop() closes the
// app and every process it started.
//
//   const app = await startApp({ build: true });
//   await app.page.keyboard.type("## Hello");
//   await app.stop();
//
// Nothing here waits without a limit: the app has XEGESIS_E2E_START_MS (60 s) to start, or the run
// stops with the app's output; each request to it is limited too. A run that was killed leaves its
// app, WebView2 processes and run folder behind; the next run closes and removes them first.

import { execFileSync, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { chmodSync, cpSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright-core";

const here = dirname(fileURLToPath(import.meta.url));
export const repo = resolve(here, "..", "..");
export const shots = join(here, "shots");

// The library the tests copy. Override with XEGESIS_E2E_SOURCE.
const SOURCE = process.env.XEGESIS_E2E_SOURCE ?? "C:\\Users\\perlj\\Documents\\Xegesis-scratch3";
// One folder per checkout: two worktrees running the tests at once must not build into, or run,
// each other's executable (a running app also locks it, so the other's build fails).
const WORK = join(tmpdir(), "xegesis-e2e", createHash("sha1").update(repo.toLowerCase()).digest("hex").slice(0, 10));
export const EXE = join(WORK, "xegesis-e2e.exe");
// How long the app may take to start before the run stops with an error.
const START_MS = Number(process.env.XEGESIS_E2E_START_MS ?? 60000);

function sh(cmd, args, cwd) {
  execFileSync(`${cmd} ${args.map((a) => (/[\s"]/.test(a) ? `"${a}"` : a)).join(" ")}`, { cwd, stdio: "inherit", shell: true });
}

// Wails's WebView2 loader clears the WEBVIEW2_* environment variables as it starts, so the
// debugging port could not be asked for. The test build (only) uses a copy of the loader package
// that reads them first, through a go.work file in the temporary folder. go.mod and the shipped
// app are unchanged.
function workspace() {
  const mod = "github.com/wailsapp/go-webview2";
  const info = JSON.parse(execFileSync("go", ["list", "-m", "-json", mod], { cwd: repo, encoding: "utf8" }));
  const copy = join(WORK, "go-webview2");
  rmSync(copy, { recursive: true, force: true });
  cpSync(info.Dir, copy, { recursive: true });
  const file = join(copy, "webviewloader", "env_create_options.go");
  const src = readFileSync(file, "utf8");
  const patched = src
    .replace("return o.additionalBrowserArguments", 'if e2eArgs == "" {\n\t\treturn o.additionalBrowserArguments\n\t}\n\treturn o.additionalBrowserArguments + " " + e2eArgs')
    .replace("wvep.userDataFolder = folder", 'wvep.userDataFolder = folder\n\t\tif e2eData != "" {\n\t\t\twvep.userDataFolder = e2eData\n\t\t}');
  if (!patched.includes("e2eArgs") || !patched.includes("e2eData")) throw new Error("The WebView2 loader has changed; update workspace() in e2e/app.mjs.");
  chmodSync(file, 0o644);
  writeFileSync(file, patched);
  writeFileSync(join(copy, "webviewloader", "e2e_env.go"), [
    "//go:build windows", "", "package webviewloader", "", 'import "os"', "",
    "// Read before the package's init clears them.",
    'var e2eArgs = os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS")',
    'var e2eData = os.Getenv("WEBVIEW2_USER_DATA_FOLDER")', "",
  ].join("\n"));
  const goVersion = /^go (\S+)/m.exec(readFileSync(join(repo, "go.mod"), "utf8"))[1];
  const work = join(WORK, "go.work");
  const slash = (p) => JSON.stringify(p);
  writeFileSync(work, `go ${goVersion}\n\nuse ${slash(repo)}\n\nreplace ${mod} ${info.Version} => ${slash(copy)}\n`);
  return work;
}

// build makes frontend/dist and the test executable that embeds it.
export function build() {
  mkdirSync(WORK, { recursive: true });
  sh("npm", ["run", "build"], join(repo, "frontend"));
  execFileSync("go", ["build", "-tags", "desktop,production", "-o", EXE, "./app"], {
    cwd: repo, stdio: "inherit", env: { ...process.env, GOWORK: workspace() },
  });
}

// within gives up on a promise after ms, so nothing the tests wait for can stall the run.
export function within(p, ms, what) {
  let timer;
  return Promise.race([
    Promise.resolve(p).finally(() => clearTimeout(timer)),
    new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(`Timed out after ${ms} ms: ${what}`)), ms); }),
  ]);
}

// waitFor retries fn until it gives a value. Each try is limited too, so a request that never
// answers (a port another program holds, a half-started WebView2) cannot hang it.
async function waitFor(fn, ms, what) {
  const until = Date.now() + ms;
  let last;
  while (Date.now() < until) {
    try {
      const v = await within(fn(), Math.max(500, Math.min(5000, until - Date.now())), what);
      if (v) return v;
    } catch (e) {
      if (e.fatal) throw e;
      last = e;
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`Timed out waiting for ${what}${last ? `: ${last.message}` : ""}`);
}

// freePort asks the system for a port nothing listens on.
function freePort() {
  return new Promise((ok, fail) => {
    const srv = createServer();
    srv.unref();
    srv.on("error", fail);
    srv.listen(0, "127.0.0.1", () => {
      const { port } = srv.address();
      srv.close(() => ok(port));
    });
  });
}

function killTree(pid) {
  if (!pid) return;
  try {
    if (process.platform === "win32") execFileSync("taskkill", ["/PID", String(pid), "/T", "/F"], { stdio: "ignore", timeout: 15000 });
    else process.kill(pid, "SIGKILL");
  } catch {
    // already gone
  }
}

// ps lists the processes whose path or command line mentions text (WebView2's processes name
// the profile folder they use), as [pid, name].
function ps(text) {
  const q = text.replaceAll("'", "''");
  try {
    const out = execFileSync("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command",
      `Get-CimInstance Win32_Process | Where-Object { ($_.CommandLine -and $_.CommandLine.Contains('${q}')) -or ($_.ExecutablePath -and $_.ExecutablePath.Contains('${q}')) } | ForEach-Object { "$($_.ProcessId) $($_.Name)" }`],
    { encoding: "utf8", timeout: 30000, stdio: ["ignore", "pipe", "ignore"] });
    return out.split(/\r?\n/).filter(Boolean).map((l) => {
      const [pid, ...name] = l.trim().split(" ");
      return [Number(pid), name.join(" ")];
    }).filter(([pid, name]) => pid !== process.pid && !/^(node|powershell)/i.test(name));
  } catch {
    return [];
  }
}

const pause = (ms) => new Promise((r) => setTimeout(r, ms));

// removeRun deletes a run folder, retrying while WebView2 lets go of its files. A folder still
// locked after that is left for the next run's sweep, with a warning, instead of failing this one.
async function removeRun(dir, ms = 10000) {
  const until = Date.now() + ms;
  for (;;) {
    try {
      rmSync(dir, { recursive: true, force: true, maxRetries: 2, retryDelay: 200 });
      return true;
    } catch (e) {
      if (Date.now() > until) {
        console.warn(`e2e: could not remove ${dir} yet (${e.code ?? e.message}); the next run removes it.`);
        return false;
      }
      await pause(400);
    }
  }
}

// sweep closes what an earlier, interrupted run left behind (its app and WebView2 processes)
// and removes its run folders.
async function sweep() {
  if (!existsSync(WORK)) return;
  for (const [pid] of [...ps(EXE), ...ps(join(WORK, "run-"))]) killTree(pid);
  for (const f of readdirSync(WORK)) if (f.startsWith("run-")) await removeRun(join(WORK, f), 3000);
}

export async function startApp({ build: doBuild = true, port } = {}) {
  if (process.platform !== "win32") throw new Error("The interface tests drive WebView2 and run on Windows.");
  if (!existsSync(join(SOURCE, "library.db"))) throw new Error(`No library to copy at ${SOURCE}`);
  await sweep();
  if (doBuild || !existsSync(EXE)) build();
  mkdirSync(shots, { recursive: true });
  port ??= await freePort();

  const run = join(WORK, `run-${Date.now()}`);
  const lib = join(run, "library");
  cpSync(SOURCE, lib, { recursive: true });
  const profile = join(run, "webview");
  mkdirSync(profile, { recursive: true });

  const child = spawn(EXE, [], {
    cwd: run,
    env: {
      ...process.env,
      XEGESIS_LIBRARY: join(lib, "library.db"),
      WEBVIEW2_USER_DATA_FOLDER: profile,
      WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS: `--remote-debugging-port=${port} --remote-allow-origins=*`,
    },
    stdio: ["ignore", "pipe", "pipe"],
    windowsHide: false,
  });
  let log = "";
  child.stdout.on("data", (d) => (log += d));
  child.stderr.on("data", (d) => (log += d));
  let exited = false;
  child.on("exit", () => (exited = true));

  let browser;
  // Close everything this run started, then remove its folder.
  const cleanUp = async () => {
    await within(browser?.close(), 5000, "closing the connection").catch(() => {});
    killTree(child.pid);
    for (const [pid] of ps(run)) killTree(pid);
    await pause(500);
    await removeRun(run);
  };
  const started = Date.now();
  const left = () => Math.max(1000, START_MS - (Date.now() - started));
  try {
    await waitFor(async () => {
      if (exited) throw Object.assign(new Error("the app quit"), { fatal: true });
      const r = await fetch(`http://127.0.0.1:${port}/json/version`, { signal: AbortSignal.timeout(2000) });
      return r.ok;
    }, left(), `the debugging port ${port}`);
    browser = await chromium.connectOverCDP(`http://127.0.0.1:${port}`, { timeout: left() });
    const page = await waitFor(() => browser.contexts().flatMap((c) => c.pages()).find((p) => p.url().startsWith("http")), left(), "the app page");
    page.setDefaultTimeout(30000);
    await page.waitForFunction(() => !!(window.go && window.go.main && window.go.main.App), null, { timeout: left() });
    await page.waitForSelector("#app, body", { timeout: left() });
    return {
      page,
      browser,
      library: lib,
      log: () => log,
      // run calls a command the way the interface does.
      run: (id, params = {}) => within(page.evaluate(async ([id, p]) => JSON.parse((await window.go.main.App.Run(id, JSON.stringify(p))) || "null"), [id, params]), 30000, `the command ${id}`),
      shot: (name) => page.screenshot({ path: join(shots, name), timeout: 15000 }),
      async stop() {
        try {
          // Quit in a moment, so the call returns before the page goes.
          await within(page.evaluate(() => { setTimeout(() => window.runtime?.Quit?.(), 0); }), 3000, "asking the app to quit").catch(() => {});
          await waitFor(() => exited, 5000, "the app to quit").catch(() => {});
        } finally {
          await cleanUp();
        }
      },
    };
  } catch (e) {
    await cleanUp();
    throw new Error(`The app did not start within ${Math.round(START_MS / 1000)} s: ${e.message}\nApp output:\n${log.slice(-2000) || "(none)"}`);
  }
}
