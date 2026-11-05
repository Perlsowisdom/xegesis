// T36b, Claude Code and Codex on this computer, in the real app. The `claude` and `codex` commands
// are fakes in e2e/fake-cli (put first on the app's PATH by app.mjs) that print canned JSON; the
// real tools are never run and nothing reaches the internet. The checks run in order: Ready and
// Not found in Settings, a streamed answer with a tool line and a verse chip (Claude Code, then
// Codex), and Stop ending the tool and everything it started.

import { existsSync, readFileSync, renameSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fakeCli } from "./app.mjs";

const PIDS = join(tmpdir(), "xegesis-fake-cli.pids");
const CODEX = join(fakeCli, "codex.cmd");

const alive = (pid) => {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
};

async function openChat(page) {
  if (!(await page.locator("#rightbar").isVisible().catch(() => false))) await page.keyboard.press("Control+Period");
  await page.locator("#rightbar [role=tab]", { hasText: "AI chat" }).click();
  await page.locator("#rightbar .chat-input").waitFor({ timeout: 10000 });
}

// ask types a question; the first one to a tool shows the privacy panel, which is confirmed.
async function ask(page, question) {
  await page.locator("#rightbar .chat-input").click();
  await page.keyboard.type(question, { delay: 10 });
  await page.keyboard.press("Enter");
}

const lastAnswer = (page) => page.locator("#rightbar .chat-a").last();
const answered = (page) => page.waitForFunction(() => {
  const a = [...document.querySelectorAll("#rightbar .chat-a")].at(-1);
  return a && a.dataset.state === "done";
}, null, { timeout: 30000 });

async function settings(page) {
  await page.evaluate(() => { location.hash = "#settings/model"; });
  const sec = page.locator(".settings-section:visible");
  const change = sec.locator(".provider-row button", { hasText: "Change" });
  if ((await change.getAttribute("aria-expanded")) !== "true") await change.click();
  return sec;
}

// answerWith asks a question of the chosen tool, confirms the privacy panel and waits for the answer.
async function answerWith(t, label) {
  const { page } = t;
  await openChat(page);
  await page.locator("#rightbar button", { hasText: "New chat" }).click();
  await ask(page, "Summarise Romans 8 for me");
  const panel = page.locator("#rightbar .privacy-panel");
  await panel.waitFor({ timeout: 15000 });
  for (const text of [`Before your first message to ${label}`, "signs in and connects to its own online service", "Summarise Romans 8 for me", "Personal copies of books are never sent"]) {
    await panel.getByText(text, { exact: false }).first().waitFor();
  }
  await panel.scrollIntoViewIfNeeded();
  await t.shot(`36b-02-privacy-${label === "Codex" ? "codex" : "claude"}.png`);
  await panel.locator("button", { hasText: "Send" }).click();
  await answered(page);
}

export default [
  {
    row: "CLI assistant: Ready and Not found show on the Model page, and choosing one makes it the provider",
    shot: "36b-01-detect.png",
    async run(t) {
      const { page, app } = t;
      const sec = await settings(page);
      const claude = sec.locator(".provider-chip[data-id=claude-code][data-state=ready]");
      await claude.waitFor();
      t.assert.ok((await claude.textContent()).includes("Ready · 2.1.0 (Claude Code)"), await claude.textContent());
      await sec.locator(".provider-chip[data-id=codex][data-state=ready]", { hasText: "Ready · codex-cli 0.99.0" }).waitFor();
      // Hiding the fake codex shows Not found, with a way to install it.
      renameSync(CODEX, `${CODEX}.off`);
      try {
        const sec2 = await (async () => { await page.evaluate(() => { location.hash = "#settings/general"; }); return settings(page); })();
        const gone = sec2.locator(".provider-chip[data-id=codex][data-state=missing]", { hasText: "Not found" });
        await gone.waitFor({ timeout: 20000 });
        t.assert.ok(await gone.isDisabled());
        const link = sec2.locator(".install-link", { hasText: "Install Codex" });
        await link.waitFor();
        t.assert.ok((await link.getAttribute("href")).startsWith("https://"));
        await sec2.scrollIntoViewIfNeeded();
        await t.shot();
      } finally {
        renameSync(`${CODEX}.off`, CODEX);
      }
      await page.evaluate(() => { location.hash = "#settings/general"; });
      const sec3 = await settings(page);
      await sec3.locator(".provider-chip[data-id=codex][data-state=ready]").waitFor({ timeout: 20000 });
      await sec3.locator(".provider-chip[data-id=claude-code]").click();
      await sec3.locator(".provider-current", { hasText: "Claude Code, signed in on this computer" }).waitFor();
      const c = await app.run("assistant.config.get");
      t.assert.ok(c.provider === "claude-code" && !c.local && !c.needs_key && !c.confirmed, JSON.stringify(c));
      await sec3.getByText("signed in as you already are", { exact: false }).waitFor();
      await sec3.locator("button", { hasText: "Test connection" }).click();
      await sec3.getByText("Claude Code 2.1.0 (Claude Code) is on this computer", { exact: false }).waitFor();
    },
  },
  {
    row: "CLI assistant: Claude Code streams an answer with a tool line and a verse chip",
    shot: "36b-03-answer-claude.png",
    async run(t) {
      const { page } = t;
      await page.evaluate(() => { location.hash = "#passage/Rom.8"; });
      await answerWith(t, "Claude Code");
      const a = lastAnswer(page);
      const chip = a.locator("a.ref-chip.chip-verse", { hasText: "Romans 8:28" });
      await chip.waitFor();
      const line = a.locator("details.tool-line").first();
      await line.locator("summary", { hasText: "Read Romans 8 (KJV)" }).waitFor();
      t.assert.equal(await line.getAttribute("open"), null, "tool lines start folded");
      await line.locator("summary").click();
      t.assert.ok((await line.locator(".tool-sent pre").textContent()).includes("spiritually minded is life and peace"));
      const text = await a.locator(".chat-a-body").textContent();
      t.assert.ok(text.includes("Let me read it.") && text.includes("together for good."), text);
      await line.scrollIntoViewIfNeeded();
      await t.shot();
      await chip.click();
      await page.waitForFunction(() => [...document.querySelectorAll('.vt[data-tb="45008028"]')].some((e) => e.offsetParent), null, { timeout: 15000 });
    },
  },
  {
    row: "CLI assistant: Codex streams an answer with a tool line and a verse chip",
    shot: "36b-04-answer-codex.png",
    async run(t) {
      const { page, app } = t;
      await app.run("assistant.config.set", { provider: "codex" });
      await page.evaluate(() => { location.hash = "#passage/Rom.8"; });
      await answerWith(t, "Codex");
      const a = lastAnswer(page);
      await a.locator("a.ref-chip.chip-verse", { hasText: "Romans 8:28" }).waitFor();
      await a.locator("details.tool-line summary", { hasText: "Read Romans 8 (KJV)" }).waitFor();
      await a.locator("details.tool-line").first().scrollIntoViewIfNeeded();
    },
  },
  {
    row: "CLI assistant: Stop kills the tool and everything it started",
    shot: "36b-05-stop.png",
    timeout: 90000,
    async run(t) {
      const { page } = t;
      rmSync(PIDS, { force: true });
      await openChat(page);
      await page.locator("#rightbar button", { hasText: "New chat" }).click();
      await ask(page, "stop test please");
      await page.waitForFunction(() => ([...document.querySelectorAll("#rightbar .chat-a")].at(-1)?.textContent ?? "").includes("Thinking about it"), null, { timeout: 30000 });
      let pids = [];
      for (let i = 0; i < 40 && pids.length < 2; i++) {
        pids = existsSync(PIDS) ? readFileSync(PIDS, "utf8").split(/\s+/).filter(Boolean).map(Number) : [];
        await page.waitForTimeout(100);
      }
      t.assert.ok(pids.length === 2 && pids.every(alive), `the tool and its child are running: ${pids}`);
      const send = page.locator("#rightbar .chat-send");
      t.assert.equal(await send.textContent(), "Stop");
      await send.click();
      await lastAnswer(page).locator(".chat-stopped", { hasText: "Stopped." }).waitFor({ timeout: 15000 });
      for (let i = 0; i < 50 && pids.some(alive); i++) await page.waitForTimeout(100);
      t.assert.ok(!pids.some(alive), `still running after Stop: ${pids.filter(alive)}`);
      t.assert.equal(await send.textContent(), "Send");
    },
  },
  {
    row: "CLI assistant: the assistant is turned off again",
    shot: "36b-06-cleanup.png",
    async run(t) {
      const { page, app } = t;
      rmSync(PIDS, { force: true });
      await app.run("assistant.config.set", { provider: "off" });
      await page.evaluate(() => { location.hash = "#settings/model"; });
      await page.locator(".settings-section:visible .provider-current", { hasText: "Off" }).waitFor();
      // Later checks expect the right sidebar closed, as the app starts.
      if (await page.locator("#rightbar").isVisible().catch(() => false)) await page.keyboard.press("Control+Period");
      await page.locator("#rightbar").waitFor({ state: "hidden", timeout: 5000 }).catch(() => {});
      t.assert.ok(!(await page.locator("#rightbar").isVisible()), "the right sidebar is closed again");
    },
  },
];
