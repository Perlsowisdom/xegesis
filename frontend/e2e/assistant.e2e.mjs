// T36a, the AI assistant, in the real app against a fake OpenAI-compatible server this file starts
// on 127.0.0.1. Nothing reaches the internet. The checks run in order and share the fake server:
// choose a provider and save a key, the privacy panel, a streamed answer with a verse chip, a tool
// line, a copyrighted passage shortened, a personal copy refused, a draft inserted into a note, and
// Stop. The last check removes the test key and closes the server.

import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { dirname, join } from "node:path";
import { KEYRING_SERVICE } from "./app.mjs";
import { epub } from "./fixtures.mjs";

const BOOK = "Assistant Personal Commentary";
const fake = { url: "", server: null, reqs: [], book: "", aborted: false };

const pause = (ms) => new Promise((r) => setTimeout(r, ms));

// reply decides what the fake model says to a request: a tool call first, then text that uses
// the tool's result. Each question names its scenario.
function reply(body) {
  const msgs = body.messages ?? [];
  const last = msgs.at(-1) ?? {};
  // The question, without the line saying what is open in the app.
  const q = ([...msgs].reverse().find((m) => m.role === "user")?.content ?? "").replace(/^\[Open in Xegesis now:[^\]]*\]\n\n/, "");
  const call = (name, args) => ({ calls: [[name, JSON.stringify(args)]] });
  if (last.role === "tool") {
    if (q.includes("commentary")) return { text: ["I could not read that book: ", "it is a personal copy."] };
    return { text: ["Paul ", "says in ", "Romans 8:28 ", "that God works ", "all things ", "together for good ", "for those who love him."], delay: 250 };
  }
  if (q.includes("Romans 8")) return call("read_passage", { ref: "Romans 8", bible: "NETfree" });
  if (q.includes("commentary")) return call("read_book", { item_id: fake.book });
  if (q.includes("outline")) return { text: ["Here is an outline.\n\n", "```draft\n# Hope in Romans 8\n\n", "- Suffering now\n- Glory to come\n```\n"] };
  if (q.includes("long")) return { text: Array.from({ length: 80 }, (_, i) => `word${i} `), delay: 200 };
  return { text: ["OK."] };
}

async function handle(req, res) {
  if (req.method === "GET" && req.url === "/v1/models") {
    res.writeHead(200, { "Content-Type": "application/json" });
    return res.end(JSON.stringify({ data: [{ id: "fake-tools" }, { id: "fake-small" }] }));
  }
  if (req.method !== "POST" || req.url !== "/v1/chat/completions") {
    res.writeHead(404);
    return res.end();
  }
  let raw = "";
  for await (const c of req) raw += c;
  const body = JSON.parse(raw);
  fake.reqs.push(body);
  const r = reply(body);
  let closed = false;
  res.on("close", () => {
    if (!res.writableFinished) {
      closed = true;
      fake.aborted = true;
    }
  });
  res.writeHead(200, { "Content-Type": "text/event-stream" });
  const send = (v) => res.write(`data: ${JSON.stringify(v)}\n\n`);
  for (const t of r.text ?? []) {
    if (closed) return;
    send({ choices: [{ delta: { content: t } }] });
    await pause(r.delay ?? 60);
  }
  (r.calls ?? []).forEach(([name, args], i) => send({ choices: [{ delta: { tool_calls: [{ index: i, id: `call_${fake.reqs.length}_${i}`, type: "function", function: { name, arguments: args } }] } }] }));
  if (!closed) res.end("data: [DONE]\n\n");
}

async function startFake() {
  if (fake.server) return;
  fake.server = createServer((req, res) => void handle(req, res).catch(() => res.destroy()));
  await new Promise((ok) => fake.server.listen(0, "127.0.0.1", ok));
  fake.url = `http://127.0.0.1:${fake.server.address().port}/v1`;
}

async function stopFake() {
  if (!fake.server) return;
  fake.server.closeAllConnections?.();
  await new Promise((ok) => fake.server.close(() => ok()));
  fake.server = null;
}

// testKeys lists the keychain entries filed under the tests' service name.
function testKeys() {
  const out = execFileSync("cmdkey", ["/list"], { encoding: "utf8", timeout: 15000 });
  return out.split(/\r?\n/).filter((l) => l.includes(`${KEYRING_SERVICE}:`)).map((l) => l.trim());
}

// chat opens the right sidebar on the AI chat tool.
async function chat(page) {
  if (!(await page.locator("#rightbar").isVisible().catch(() => false))) await page.keyboard.press("Control+Period");
  await page.locator("#rightbar [role=tab]", { hasText: "AI chat" }).click();
  await page.locator("#rightbar .chat-input").waitFor({ timeout: 10000 });
}

async function ask(page, question) {
  const input = page.locator("#rightbar .chat-input");
  await input.click();
  await page.keyboard.type(question, { delay: 10 });
  await page.keyboard.press("Enter");
}

const lastAnswer = (page) => page.locator("#rightbar .chat-a").last();
const answered = (page) => page.waitForFunction(() => {
  const a = [...document.querySelectorAll("#rightbar .chat-a")].at(-1);
  return a && a.dataset.state === "done";
}, null, { timeout: 20000 });

// toolSent is the content the fake model received from the last tool call.
const toolSent = () => [...fake.reqs].reverse().map((b) => b.messages.at(-1)).find((m) => m.role === "tool")?.content ?? "";

async function readerAt(page, ref) {
  await page.evaluate((r) => { location.hash = `#passage/${r}`; }, ref);
}

export default [
  {
    row: "Assistant: choose a provider and save a key",
    shot: "36a-01-provider.png",
    async run(t) {
      const { page, app } = t;
      await startFake();
      t.assert.deepEqual(testKeys(), [], "a test key was left from before");
      await page.evaluate(() => { location.hash = "#settings/model"; });
      const sec = page.locator(".settings-section:visible");
      await sec.locator(".provider-row button", { hasText: "Change" }).click();
      await sec.locator(".provider-chip[data-id=claude-code]", { hasText: "coming in T36b" }).waitFor();
      t.assert.ok(await sec.locator(".provider-chip[data-id=codex]").isDisabled());
      await sec.locator(".provider-chip[data-id=openai]").click();
      await sec.locator(".provider-current", { hasText: "OpenAI, with your own key" }).waitFor();
      // The fake server stands in for OpenAI's address (More options).
      await sec.locator(".more-options summary").click();
      const addr = sec.getByLabel("Address");
      await addr.fill(fake.url);
      await sec.locator("button", { hasText: "Save address" }).click();
      await sec.getByText(`Sends to 127.0.0.1:${new URL(fake.url).port}`).waitFor();
      const key = sec.locator("input[type=password]");
      await key.click();
      await page.keyboard.type("sk-e2e-test-key", { delay: 5 });
      await sec.locator("button", { hasText: "Save key" }).click();
      await sec.locator(".key-state .badge", { hasText: "Saved" }).waitFor();
      t.assert.equal(await key.inputValue(), "", "the key is not shown back");
      await sec.locator(".model-chip[data-model=fake-tools]").click();
      await sec.locator(".model-chip[data-model=fake-tools][aria-pressed=true]").waitFor();
      await sec.locator("button", { hasText: "Test connection" }).click();
      await sec.getByText("Connected to OpenAI. 2 models are available.").waitFor();
      const c = await app.run("assistant.config.get");
      t.assert.ok(c.key_saved && c.model === "fake-tools" && !c.local && !c.confirmed, JSON.stringify(c));
      t.assert.equal(testKeys().length, 1, "the key is in the keychain under the test service");
      const settings = await app.run("history.list", { limit: 50 }).catch(() => []);
      t.assert.ok(!JSON.stringify(settings).includes("sk-e2e-test-key"), "the key is not in the library's history");
      t.assert.equal(fake.reqs.length, 0, "nothing was asked of the model");
    },
  },
  {
    row: "Assistant: the privacy panel appears on first use",
    shot: "36a-02-privacy.png",
    async run(t) {
      const { page } = t;
      await readerAt(page, "Rom.8");
      await chat(page);
      await ask(page, "Summarise Romans 8 for me");
      const panel = page.locator("#rightbar .privacy-panel");
      await panel.waitFor({ timeout: 10000 });
      for (const text of ["Before your first message to OpenAI", `${fake.url}/chat/completions`, "fake-tools", "Summarise Romans 8 for me", "Personal copies of books are never sent"]) {
        await panel.getByText(text, { exact: false }).first().waitFor();
      }
      t.assert.equal(fake.reqs.length, 0, "nothing is sent before the panel is confirmed");
      await panel.scrollIntoViewIfNeeded();
      await t.shot();
      await panel.locator("button", { hasText: "Send" }).click();
      await page.locator("#rightbar .chat-q", { hasText: "Summarise Romans 8" }).waitFor();
    },
  },
  {
    row: "Assistant: a question gets a streamed answer with a verse chip",
    shot: "36a-03-answer.png",
    async run(t) {
      const { page } = t;
      // While it streams, part of the answer shows before the reply is done.
      await page.waitForFunction(() => {
        const a = [...document.querySelectorAll("#rightbar .chat-a")].at(-1);
        const text = a?.querySelector(".chat-a-body")?.textContent ?? "";
        return a?.dataset.state === "writing" && text.includes("Paul") && !text.includes("love him");
      }, null, { timeout: 15000 });
      await answered(page);
      const chip = lastAnswer(page).locator("a.ref-chip.chip-verse", { hasText: "Romans 8:28" });
      await chip.waitFor();
      await t.shot();
      // The same in the dark theme.
      await page.emulateMedia({ colorScheme: "dark" });
      await page.waitForTimeout(200);
      await t.shot("36a-03-answer-dark.png");
      await page.emulateMedia({ colorScheme: "light" });
      t.assert.ok(fake.reqs[0].messages[1].content.includes("passage Romans 8"), "the open passage is sent as context");
      await chip.click();
      await page.waitForFunction(() => [...document.querySelectorAll('.vt[data-tb="45008028"]')].some((e) => e.offsetParent), null, { timeout: 15000 });
    },
  },
  {
    row: "Assistant: a tool call is shown",
    shot: "36a-04-tool.png",
    async run(t) {
      const { page } = t;
      await chat(page);
      const line = lastAnswer(page).locator("details.tool-line").first();
      await line.locator("summary", { hasText: "Read Romans 8 (NETfree" }).waitFor();
      t.assert.equal(await line.getAttribute("open"), null, "tool lines start folded");
      await line.locator("summary").click();
      await line.locator(".tool-sent pre").waitFor();
      const shown = await line.locator(".tool-sent pre").textContent();
      t.assert.equal(shown, toolSent(), "the line shows exactly what the model received");
      await line.scrollIntoViewIfNeeded();
    },
  },
  {
    row: "Assistant: a copyrighted passage is shortened",
    shot: "36a-05-shortened.png",
    async run(t) {
      const { page } = t;
      const line = lastAnswer(page).locator("details.tool-line").first();
      await line.locator("summary", { hasText: "Read Romans 8 (NETfree, shortened to the licence limit)" }).waitFor();
      const sent = toolSent();
      t.assert.ok(sent.includes("Shortened to about 2000 characters"), sent.slice(-300));
      t.assert.ok(sent.length < 2600, `sent ${sent.length} characters`);
      t.assert.ok(!sent.includes("39 "), "verse 39 is past the limit");
      await line.locator(".tool-sent pre").evaluate((e) => { e.scrollTop = e.scrollHeight; });
      await line.scrollIntoViewIfNeeded();
    },
  },
  {
    row: "Assistant: a personal-copy book is refused for an online provider",
    shot: "36a-06-personal.png",
    async run(t) {
      const { page, app } = t;
      const dir = join(dirname(app.library), "assistant-books");
      mkdirSync(dir, { recursive: true });
      writeFileSync(join(dir, "assistant-commentary.epub"), epub(BOOK));
      await app.run("library.add", { paths: [dir] });
      for (let i = 0; i < 40 && !fake.book; i++) {
        const r = await app.run("library.items", { text: BOOK, limit: 5 });
        fake.book = r.items.find((x) => x.status === "online")?.id ?? "";
        if (!fake.book) await page.waitForTimeout(250);
      }
      t.assert.ok(fake.book, "the book was added");
      // The fixture says it is public domain; this one is a book the person bought.
      await app.run("library.set_licence", { item_ids: [fake.book], type: "personal_purchase" });
      t.assert.equal((await app.run("library.item", { id: fake.book })).licence_name, "Personal copy");
      await chat(page);
      await ask(page, "What does my commentary say?");
      await answered(page);
      const line = lastAnswer(page).locator("details.tool-line").first();
      await line.locator("summary", { hasText: `Read "${BOOK}" (not sent: a personal copy goes only to a model on this computer)` }).waitFor();
      await line.locator("summary").click();
      const sent = toolSent();
      t.assert.ok(sent.includes("personal copy") && !sent.includes("Paragraph") && !sent.includes("grace and mercy"), sent);
      await lastAnswer(page).getByText("it is a personal copy").waitFor();
    },
  },
  {
    row: "Assistant: a draft is inserted into a note",
    shot: "36a-07-draft.png",
    async run(t) {
      const { page } = t;
      await t.openNote("My study notes.", "Assistant draft note");
      await chat(page);
      await page.locator("#rightbar .chat-context", { hasText: "Assistant draft note" }).waitFor();
      await ask(page, "Give me an outline");
      await answered(page);
      const card = lastAnswer(page).locator(".draft-card");
      await card.locator("h3, h4, .md-h", { hasText: "Hope in Romans 8" }).first().waitFor();
      await card.locator("button", { hasText: "Insert into note" }).click();
      await card.getByText("Inserted into “Assistant draft note”.").waitFor();
      await page.waitForFunction((root) => document.querySelector(`${root} .rich-text`)?.textContent.includes("Glory to come"), t.root, { timeout: 8000 });
      const body = await t.saved();
      t.assert.ok(body.includes("Hope in Romans 8") && body.includes("- Glory to come") && body.startsWith("My study notes."), body);
      t.assert.ok(fake.reqs.at(-1).messages.find((m) => m.role === "user" && m.content.includes("Give me an outline")).content.includes('note "Assistant draft note"'), "the open note is sent as context");
    },
  },
  {
    row: "Assistant: Stop ends a reply",
    shot: "36a-08-stop.png",
    async run(t) {
      const { page } = t;
      await chat(page);
      fake.aborted = false;
      await ask(page, "Write a long answer");
      await page.waitForFunction(() => ([...document.querySelectorAll("#rightbar .chat-a")].at(-1)?.textContent ?? "").includes("word3"), null, { timeout: 15000 });
      const send = page.locator("#rightbar .chat-send");
      t.assert.equal(await send.textContent(), "Stop");
      await send.click();
      await lastAnswer(page).locator(".chat-stopped", { hasText: "Stopped." }).waitFor({ timeout: 10000 });
      const len = (await lastAnswer(page).textContent()).length;
      await page.waitForTimeout(1200);
      t.assert.equal((await lastAnswer(page).textContent()).length, len, "nothing more arrives after Stop");
      t.assert.ok(!(await lastAnswer(page).textContent()).includes("word79"));
      t.assert.equal(await send.textContent(), "Send");
      t.assert.ok(fake.aborted, "the request to the model was closed");
      await page.waitForTimeout(200);
    },
  },
  {
    row: "Assistant: the test key is removed and the fake server closed",
    shot: "36a-09-cleanup.png",
    async run(t) {
      const { page, app } = t;
      if (/panic|fatal error/.test(app.log())) console.log(app.log().slice(-4000));
      try {
        if (fake.server || testKeys().length) {
          await app.run("assistant.config.set", { provider: "openai", clear_key: true });
          await app.run("assistant.config.set", { provider: "off" });
        }
        await page.evaluate(() => { location.hash = "#settings/model"; });
        await page.locator(".settings-section:visible .provider-current", { hasText: "Off" }).waitFor();
      } finally {
        await stopFake();
        // Later checks expect the right sidebar closed, as the app starts.
        if (await page.locator("#rightbar").isVisible().catch(() => false)) await page.keyboard.press("Control+Period");
        await page.locator("#rightbar").waitFor({ state: "hidden", timeout: 5000 }).catch(() => {});
      }
      t.assert.ok(!(await page.locator("#rightbar").isVisible()), "the right sidebar is closed again");
      t.assert.deepEqual(testKeys(), [], "test keys left in the keychain");
    },
  },
];
