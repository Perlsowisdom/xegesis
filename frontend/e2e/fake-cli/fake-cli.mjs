// A fake `claude` or `codex` for the interface tests: prints canned JSON lines and never touches
// the network or any real tool. claude.cmd and codex.cmd run it as `node fake-cli.mjs <tool> <args>`.
//
//   --version               prints a version
//   a question that has    prints a tool call and an answer that mentions Romans 8:28
//     "Romans 8"
//   a question that has    writes the process IDs of itself and a child to xegesis-fake-cli.pids
//     "stop test"            in the temp folder, streams a first word, then waits for a minute
//
// The prompt is read from standard input, as the app sends it.

import { spawn } from "node:child_process";
import { appendFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const [tool, ...args] = process.argv.slice(2);
if (args.includes("--version")) {
  console.log(tool === "codex" ? "codex-cli 0.99.0" : "2.1.0 (Claude Code)");
  process.exit(0);
}

let prompt = "";
for await (const c of process.stdin) prompt += c;
// The question is the part after the app's instructions.
const q = prompt.split("The person's new question:").at(-1);
const out = (v) => console.log(JSON.stringify(v));
const pause = (ms) => new Promise((r) => setTimeout(r, ms));
const claude = tool !== "codex";
const text = (t) => (claude
  ? out({ type: "stream_event", event: { type: "content_block_delta", delta: { type: "text_delta", text: t } } })
  : out({ type: "item.completed", item: { id: `m${Math.random()}`, type: "agent_message", text: t } }));

if (q.includes("stop test")) {
  const pids = join(tmpdir(), "xegesis-fake-cli.pids");
  // A child that would outlive its parent if only the parent were killed.
  const child = spawn(process.execPath, ["-e", "setTimeout(() => {}, 120000)"], { stdio: "ignore" });
  appendFileSync(pids, `${process.pid}\n${child.pid}\n`);
  text("Thinking about it. ");
  await pause(120000);
  process.exit(0);
}

if (q.includes("Romans 8")) {
  const input = { ref: "Romans 8", bible: "KJV" };
  const result = "For to be carnally minded is death; but to be spiritually minded is life and peace.";
  if (claude) {
    out({ type: "system", subtype: "init", session_id: "fake-session" });
    text("Let me read it. ");
    await pause(150);
    out({ type: "assistant", message: { content: [{ type: "text", text: "Let me read it. " }, { type: "tool_use", id: "toolu_1", name: "mcp__xegesis__read_passage", input }] } });
    await pause(150);
    out({ type: "user", message: { content: [{ type: "tool_result", tool_use_id: "toolu_1", content: [{ type: "text", text: result }] }] } });
  } else {
    out({ type: "thread.started", thread_id: "fake-thread" });
    out({ type: "item.started", item: { id: "item_1", type: "mcp_tool_call", server: "xegesis", tool: "read_passage", arguments: input, status: "in_progress" } });
    await pause(150);
    out({ type: "item.completed", item: { id: "item_1", type: "mcp_tool_call", server: "xegesis", tool: "read_passage", arguments: input, result: { content: [{ type: "text", text: result }] }, status: "completed" } });
  }
  for (const t of ["Paul says in ", "Romans 8:28 ", "that God works all things ", "together for good."]) {
    text(t);
    await pause(80);
  }
} else {
  text("OK.");
}
if (claude) out({ type: "result", subtype: "success", is_error: false, result: "done" });
else out({ type: "turn.completed", usage: {} });
