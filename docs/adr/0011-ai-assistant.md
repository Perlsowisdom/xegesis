# 0011: AI assistant

Status: Accepted (approved by the user, 5 November 2026; T36a). Builds on 0007 (clean room) and 0009 (privacy). The user confirmed that personal-copy Bibles go to online models shortened to their quoting limit, and personal-copy library books go only to local models.

## Context

The user decided on 5 November 2026 that Xegesis has an AI assistant that **reads and suggests**: it
searches the library, reads passages, notes, sermons, highlights and reports, and answers with
references; whatever it writes is a draft the user inserts. It must never change the user's data. It
works with online services through the user's own key, with local models, and (T36b) with Claude Code
and Codex already signed in on the computer. Text from works with licences may reach a model only as it
may be quoted into a note. It is designed from the user's requirements and the Any app's model settings
only, never from another Bible program's AI features.

## Decision

**Provider interface** (`internal/assistant`). A provider streams one reply to a request (model,
messages, tool definitions), passing text as it arrives and returning the finished text and tool calls;
cancelling the context stops it. It also lists its models. Errors come back in plain words with a kind:
`bad_key`, `no_connection`, `rate_limited`, `no_tools`, `no_model`, `failed`.

- One adapter serves every OpenAI-compatible Chat Completions API, with server-sent events and
  streamed tool calls: OpenAI, OpenRouter, Gemini's compatible endpoint, Anthropic's compatible endpoint
  ("Claude"), and local servers such as Ollama and LM Studio ("Local model"), or any address ("Custom").
  It lists models from `/models` when the server has it, and accepts a reply that was not streamed.
- **Claude** goes through Anthropic's OpenAI-compatible endpoint (`https://api.anthropic.com/v1`). That
  endpoint covers streaming text and tool calls, which is all the assistant uses. Missing compared with
  Anthropic's own API: prompt caching, extended-thinking output, citations and document blocks, so long
  chats cost more than they would natively. A native adapter can be added behind the same interface.
- Claude Code and Codex (T36b) will be providers of their own that run the command-line tool with
  Xegesis's MCP server in an assistant mode offering the same tools.

**Read-only tools.** The tools are defined once (`assistant.Tools`) so the MCP assistant mode can offer
the same list: search the library, list Bibles, read a passage, read a section of a book, list and read
notes, list and read sermons, list highlights, passage report, word report and look up a word. Each one
runs existing commands that do not write (a test checks every command they run). There are no write
tools at all. A draft is a fenced ```` ```draft ```` block in the reply; the interface shows it as a card
whose **Insert into note**, **New note** and **Copy** buttons are the user's own actions. "Save as note"
keeps a chat as a note, also only when the user asks. Chats live in memory for the session.

**Licence gate.** Every tool that returns text from a work passes it through `assistant.Gate`, with the
rules of `document.quote`:

- open text (public domain, Creative Commons, other open licences, the user's own work) goes freely;
- text with a quoting limit goes only up to that limit (its export pages × 2,000 characters, or 2,000
  for a percentage-only limit) and is **shortened** to it, with a notice telling the model so; in a
  report, the limit holds for all of one work's entries in a section together;
- text whose licence does not allow keeping a copy is not sent;
- the text of a **personal copy of a library book** (including books whose licence is unknown) goes only
  to a provider on this computer. Search snippets, highlight words and report entries from such books
  are withheld from online providers the same way. Bibles and reference works follow the quoting rule
  above, as `document.quote` does; a personal-copy Bible is shortened, not withheld.

A provider is **on this computer** only when it is the Local model or a Custom address *and* its address
is a loopback host (`localhost`, `127.0.0.0/8`, `::1`). Online presets stay online even if their address
is changed, so pointing one at a proxy never unlocks personal copies.

**What is sent.** Nothing is sent until the user picks a provider (the assistant is off by default).
Before the first message to a provider and address, the chat shows a panel with the address, the model,
the question exactly as sent (with the passage and the note open now), what the tools may send, and the
full first request; the user confirms once per provider and address (`assistant.preview`,
`assistant.config.set {confirm}`, and `assistant.chat` refuses until then). Each request holds a system
prompt, the conversation so far, the tool definitions and the tool results. Every tool call shows in the
chat as a line that opens to exactly what the model received. Listing models and **Test connection**
send only the key (and, when a server has no model list, a one-line "Reply with the word OK." request).
No telemetry; endpoints are listed in `docs/privacy.md`.

**Keys** are kept in the operating system's credential store through `github.com/zalando/go-keyring`
(MIT), under `assistant.<provider>.key`, never in `library.db`, the change history or a log; the
interface shows only "Saved". The other settings (provider, addresses, models, confirmations) are the
`assistant` row of the `settings` table. Tests use an in-memory store; the interface tests file keys
under a separate service name (`XEGESIS_KEYRING_SERVICE`) and remove them.

**Commands**: `assistant.config.get`, `assistant.config.set`, `assistant.models`, `assistant.test`,
`assistant.preview`, `assistant.chat` (streams `assistant:event` events to the desktop app and returns
the whole reply), `assistant.cancel`, `assistant.conversation`, `assistant.save`.

## Consequences

- The assistant can never alter the library; anything it suggests reaches the library only through a
  button the user presses.
- Licence limits apply to models exactly as to notes, and personal copies of books stay on the computer
  unless the model runs there too.
- One adapter covers most services; services without OpenAI-compatible tool calling need their own
  adapter.
- Tests never use the network: Go tests use an `httptest` server and the interface tests a local fake
  server started by the test.
