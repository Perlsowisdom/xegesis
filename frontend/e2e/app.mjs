// Starts the real desktop app (Wails on WebView2) for the interface tests and connects to it over
// the Chrome DevTools Protocol. The app runs on a temporary copy of the scratch library, with its
// own WebView2 profile, so the user's library and settings are never touched. stop() closes the
// app and every process it started.
//
//   const app = await startApp({ build: true });
//   await app.page.keyboard.type("## Hello");
//   await app.stop();

import { execFileSync, spawn } from "node:child_process";
import { chmodSync, cpSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright-core";

const here = dirname(fileURLToPath(import.meta.url));
export const repo = resolve(here, "..", "..");
export const shots = join(here, "shots");

// The library the tests copy. Override with XEGESIS_E2E_SOURCE.
const SOURCE = process.env.XEGESIS_E2E_SOURCE ?? "C:\\Users\\perlj\\Documents\\Xegesis-scratch3";
const WORK = join(tmpdir(), "xegesis-e2e");
export const EXE = join(WORK, "xegesis-e2e.exe");

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

async function waitFor(fn, ms, what) {
  const until = Date.now() + ms;
  let last;
  while (Date.now() < until) {
    try {
      const v = await fn();
      if (v) return v;
    } catch (e) {
      last = e;
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`Timed out waiting for ${what}${last ? `: ${last.message}` : ""}`);
}

function killTree(pid) {
  if (!pid) return;
  try {
    if (process.platform === "win32") execFileSync("taskkill", ["/PID", String(pid), "/T", "/F"], { stdio: "ignore" });
    else process.kill(pid, "SIGKILL");
  } catch {
    // already gone
  }
}

export async function startApp({ build: doBuild = true, port = 9333 + Math.floor(Math.random() * 400) } = {}) {
  if (process.platform !== "win32") throw new Error("The interface tests drive WebView2 and run on Windows.");
  if (!existsSync(join(SOURCE, "library.db"))) throw new Error(`No library to copy at ${SOURCE}`);
  if (doBuild || !existsSync(EXE)) build();
  mkdirSync(shots, { recursive: true });

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
  try {
    await waitFor(async () => {
      if (exited) throw new Error(`the app quit:\n${log}`);
      const r = await fetch(`http://127.0.0.1:${port}/json/version`);
      return r.ok;
    }, 30000, `the debugging port ${port}`);
    browser = await chromium.connectOverCDP(`http://127.0.0.1:${port}`);
    const page = await waitFor(() => browser.contexts().flatMap((c) => c.pages()).find((p) => p.url().startsWith("http")), 20000, "the app page");
    await page.waitForFunction(() => !!(window.go && window.go.main && window.go.main.App), null, { timeout: 20000 });
    await page.waitForSelector("#app, body", { timeout: 20000 });
    return {
      page,
      browser,
      library: lib,
      log: () => log,
      // run calls a command the way the interface does.
      run: (id, params = {}) => page.evaluate(async ([id, p]) => JSON.parse((await window.go.main.App.Run(id, JSON.stringify(p))) || "null"), [id, params]),
      shot: (name) => page.screenshot({ path: join(shots, name) }),
      async stop() {
        try {
          await page.evaluate(() => window.runtime?.Quit?.()).catch(() => {});
          await waitFor(() => exited, 5000, "the app to quit").catch(() => {});
          await browser.close().catch(() => {});
        } finally {
          killTree(child.pid);
          await new Promise((r) => setTimeout(r, 500));
          rmSync(run, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
        }
      },
    };
  } catch (e) {
    await browser?.close().catch(() => {});
    killTree(child.pid);
    await new Promise((r) => setTimeout(r, 500));
    rmSync(run, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
    throw e;
  }
}
