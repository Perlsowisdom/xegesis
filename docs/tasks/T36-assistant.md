# T36: The AI assistant

Brief for agents (Opus), in two parts: **T36a**, then **T36b**. Read these first:
- `AGENTS.md`;
- ADR 0007 (clean room) and ADR 0009 (privacy);
- `docs/ui-requirements.md`, the sidebar, AI model settings and the right panel;
- `docs/tasks/T34a-shell.md`, which built the placeholders this task replaces;
- `docs/mcp.md` and `internal/mcp`;
- `frontend/e2e/`, the real-app harness.

## The user's decisions (5 November 2026)

1. **Read and suggest.**
   - The assistant can search the library and read passages, notes, highlights and reports. It
     answers with clickable references.
   - Anything it writes (a note, a sermon section, an outline) comes as a **draft**. The user inserts
     a draft with one click, into the open note or sermon or as a new note.
   - It **never changes the user's data itself**: it has no write tools at all.
2. **Providers, first:**
   - **OpenAI-compatible:** one adapter for OpenAI, OpenRouter, Gemini's OpenAI-compatible endpoint,
     and local models (Ollama, LM Studio) by base URL.
   - **Claude Code and Codex on this computer:** they use the command-line tool the user has
     already signed in to, with Xegesis's MCP server as their tools. No key is needed.
3. **Licences:** text reaches a model only through the same rules as quoting into a note
   (`document.quote`):
   - open-licence text goes freely;
   - copyrighted text only within its quoting limit;
   - **personal copies of books only to a local model** (a provider marked "on this computer").

   A tool result over the limit is shortened, and says so to the model.

## Rules for both parts

- **Privacy (ADR 0009):**
  - The assistant is **off** until the user picks a provider.
  - Before the first message to a provider, a panel shows exactly what is sent and where: the question,
    tool results with their text, and the model name. The user confirms once per provider.
  - Add each endpoint to `docs/privacy.md`. No telemetry.
- **Keys** are kept in the operating system's credential store, never in the library or a log. Use a
  permissively licensed library, such as `github.com/zalando/go-keyring` (MIT). Tests use a fake
  store.
- **Clean room:** design from the user's requirements and the Any app's model settings only. Nothing
  from Logos's AI features.
- Write an **ADR 0011**, "AI assistant", with status "Proposed". It covers the provider interface,
  read-only tools, the licence gate, and what is sent. The coordinator asks the user to accept it.
- **No real network calls in tests.** Use an `httptest` fake OpenAI-compatible server, with streaming
  and tool calls, and a fake command-line script. The e2e tests point the app at a local fake.
- Run long commands with timeouts. Close every process you start. Don't junction `node_modules`; run
  `npm ci`. Don't commit `frontend/dist`.

## T36a: the core, the OpenAI-compatible adapter, Settings and the chat panel

**Go (`internal/assistant`):**
- A provider interface: stream a reply, call tools, cancel.
- The **OpenAI-compatible adapter**, using Chat Completions with streaming and tool calls:
  - base URL, key and model;
  - list models from `/models` when the server offers it;
  - errors shown in plain words: a bad key, no connection, rate limited, a model with no tool support.
- **Read-only tools**, built on existing commands:
  - search the library;
  - get passage text, through the licence gate;
  - list and read notes and sermons;
  - list highlights;
  - run a passage or word report;
  - look up a word.

  Keep the list in one place so T36b's MCP mode can reuse it.
- A **licence gate** that every text-returning tool goes through. It applies quote limits, and sends
  personal copies only to local providers. It has tests.
- Commands such as `assistant.config.get` and `assistant.config.set` (with the key in the credential
  store), `assistant.chat`, which streams to the frontend through Wails events, and `assistant.cancel`.
- Conversations live in memory for the session. "Save as note" keeps one as a note.

**Settings → Assistant → Model** (replacing the placeholder, keeping its approved look):
- Provider chips: Claude, OpenAI, Gemini, OpenRouter and Local model all use the OpenAI-compatible
  adapter, with preset base URLs. A "Custom" choice takes any URL.
  - **Claude:** Anthropic's API offers an OpenAI-compatible endpoint. Use it, and say in the ADR if a
    feature is missing.
- A key field, which saves to the credential store and shows only "Saved".
- Model chips filled from the provider.
- A **Test connection** button.
- Off.
- Claude Code and Codex show as "coming in T36b".

**The AI chat panel** (the right sidebar's AI chat tab):
- Messages stream in. There is a Stop button, and Enter sends.
- References in answers become verse chips that open the passage.
- Each tool call shows as a small, foldable line, for example "Read Romans 8 (ESV, shortened to the
  licence limit)".
- A draft shows as a card with **Insert into note**, **New note** and **Copy**.
- The question is sent with context: the passage and the note open now, if any.
- The first-send privacy panel, as described above.

**e2e** (real app, local fake server):
- choose a provider and save a key;
- ask a question and see the streamed answer with a verse chip;
- a tool call is shown;
- a copyrighted passage is shortened;
- a personal-copy book is refused for an online provider;
- insert a draft into a note;
- Stop;
- the privacy panel appears on first use.

## T36b: Claude Code and Codex on this computer

- **Detect** the `claude` and `codex` command-line tools on PATH, with their versions. The Settings
  chips show **Ready** or **Not found**, with a link to install.
- Run them non-interactively, for example `claude -p` with JSON output, or `codex exec`. Each gets
  Xegesis's MCP server in a new **assistant mode**: it offers only the read-only tools, through the
  licence gate, with no write commands.
- Stream their output into the same chat panel, with the same tool lines, drafts and Stop. Stop
  kills the whole process tree.
- These tools run their own sign-in and network calls. Say so in the privacy panel and in
  `docs/privacy.md`.
- Tests: a fake `claude` or `codex` script on PATH that prints canned JSON. e2e with that script.

## Done when

- Every e2e item above has a passing real-app check with a screenshot.
- `go test ./...`, `npm test`, `npm run build` and `npm run e2e` pass.
- ADR 0011 is written ("Proposed") and `docs/privacy.md` is updated.
- The report lists each item as pass or fail, and anything left.
