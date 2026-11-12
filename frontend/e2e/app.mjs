// Starts the real desktop app (Wails on WebView2) for the interface tests and connects to it over
// the Chrome DevTools Protocol. The app runs on a temporary copy of the scratch library, with its
// own WebView2 profile, so the user's library and settings are never touched. restart() quits the
// app and opens it again on the same copy; stop() closes the app and every process it started.
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
import { DatabaseSync } from "node:sqlite";
import { homedir, tmpdir } from "node:os";
import { delimiter, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright-core";

const here = dirname(fileURLToPath(import.meta.url));
export const repo = resolve(here, "..", "..");
export const shots = join(here, "shots");
// Holds fake claude and codex commands (canned output), put first on the app's PATH.
export const fakeCli = join(here, "fake-cli");
const pathKey = Object.keys(process.env).find((k) => k.toLowerCase() === "path") ?? "PATH";

// The library the tests copy: a scratch library with the test Bibles, never your own. Set XEGESIS_E2E_SOURCE
// to its folder; the default is Documents/Xegesis-scratch3 in your user profile.
const SOURCE = process.env.XEGESIS_E2E_SOURCE ?? join(homedir(), "Documents", "Xegesis-scratch3");
// One folder per checkout: two worktrees running the tests at once must not build into, or run,
// each other's executable (a running app also locks it, so the other's build fails).
const WORK = join(tmpdir(), "xegesis-e2e", createHash("sha1").update(repo.toLowerCase()).digest("hex").slice(0, 10));
export const EXE = join(WORK, "xegesis-e2e.exe");
// The keychain service name the app files keys under while tested (secrets.ServiceEnv).
export const KEYRING_SERVICE = "xegesis-e2e";
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

// The first-run question about updates (T23) would sit over every test, so the copy starts with it
// answered "no". updates.e2e.mjs asks for it again to test it. This writes the copy, never the
// user's library.
function markUpdateQuestionAsked(db) {
  const d = new DatabaseSync(db);
  try {
    d.prepare("INSERT OR REPLACE INTO settings (key, value_json) VALUES ('update.settings', ?)").run('{"enabled":false,"asked":true}');
  } finally {
    d.close();
  }
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
  markUpdateQuestionAsked(join(lib, "library.db"));
  const profile = join(run, "webview");
  mkdirSync(profile, { recursive: true });

  // launch starts the app on the run folder's library and connects to it. Each launch uses the same
  // library and WebView2 profile, so a restart shows what a person who quits and reopens sees.
  let log = "";
  const launch = async (port) => {
    const child = spawn(EXE, [], {
      cwd: run,
      env: {
        ...process.env,
        // Fake claude and codex commands go first on PATH, so the real ones are never run.
        [pathKey]: `${fakeCli}${delimiter}${process.env[pathKey] ?? ""}`,
        XEGESIS_LIBRARY: join(lib, "library.db"),
        WEBVIEW2_USER_DATA_FOLDER: profile,
        // Keys the tests save go to their own keychain entries, never the person's.
        XEGESIS_KEYRING_SERVICE: KEYRING_SERVICE,
        WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS: `--remote-debugging-port=${port} --remote-allow-origins=*`,
      },
      stdio: ["ignore", "pipe", "pipe"],
      windowsHide: false,
    });
    const s = { child, exited: false, browser: null, page: null };
    child.stdout.on("data", (d) => (log += d));
    child.stderr.on("data", (d) => (log += d));
    child.on("exit", () => (s.exited = true));
    cur = s;
    const started = Date.now();
    const left = () => Math.max(1000, START_MS - (Date.now() - started));
    await waitFor(async () => {
      if (s.exited) throw Object.assign(new Error("the app quit"), { fatal: true });
      const r = await fetch(`http://127.0.0.1:${port}/json/version`, { signal: AbortSignal.timeout(2000) });
      return r.ok;
    }, left(), `the debugging port ${port}`);
    s.browser = await chromium.connectOverCDP(`http://127.0.0.1:${port}`, { timeout: left() });
    s.page = await waitFor(() => s.browser.contexts().flatMap((c) => c.pages()).find((p) => p.url().startsWith("http")), left(), "the app page");
    s.page.setDefaultTimeout(30000);
    await s.page.waitForFunction(() => !!(window.go && window.go.main && window.go.main.App), null, { timeout: left() });
    await s.page.waitForSelector("#app, body", { timeout: left() });
    return s;
  };
  // quit asks the app to quit, then closes whatever of it is left, its WebView2 processes too.
  const quit = async (s) => {
    if (!s) return;
    try {
      if (s.page) {
        // Quit in a moment, so the call returns before the page goes.
        await within(s.page.evaluate(() => { setTimeout(() => window.runtime?.Quit?.(), 0); }), 3000, "asking the app to quit").catch(() => {});
        await waitFor(() => s.exited, 5000, "the app to quit").catch(() => {});
      }
    } finally {
      await within(s.browser?.close(), 5000, "closing the connection").catch(() => {});
      killTree(s.child.pid);
      for (const [pid] of ps(run)) killTree(pid);
      await pause(500);
    }
  };
  let cur = null;
  const start = async (port) => {
    try {
      return await launch(port);
    } catch (e) {
      await quit(cur);
      await removeRun(run);
      throw new Error(`The app did not start within ${Math.round(START_MS / 1000)} s: ${e.message}\nApp output:\n${log.slice(-2000) || "(none)"}`);
    }
  };

  await start(port);
  const app = {
    page: cur.page,
    browser: cur.browser,
    library: lib,
    log: () => log,
    // run calls a command the way the interface does.
    run: (id, params = {}) => within(app.page.evaluate(async ([id, p]) => JSON.parse((await window.go.main.App.Run(id, JSON.stringify(p))) || "null"), [id, params]), 30000, `the command ${id}`),
    shot: (name) => app.page.screenshot({ path: join(shots, name), timeout: 15000 }),
    // restart quits the app and opens it again on the same library copy; app.page is then the
    // new window's page.
    async restart() {
      await quit(cur);
      await start(await freePort());
      app.page = cur.page;
      app.browser = cur.browser;
    },
    async stop() {
      try {
        await quit(cur);
      } finally {
        await removeRun(run);
      }
    },
  };
  return app;
}
