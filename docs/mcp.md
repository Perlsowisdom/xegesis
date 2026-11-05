# Using Xegesis from an MCP client

`xegesis mcp` runs a [Model Context Protocol](https://modelcontextprotocol.io) server on standard
input and output. It exposes every registered Xegesis command as a tool, so anything added to the
command registry appears without changes to the server.

Supported protocol versions: 2025-06-18, 2025-03-26 and 2024-11-05 (the server answers with the
client's version when it knows it, and with 2025-06-18 otherwise). It offers the `tools` capability.

## Add it to a client

Most clients take a JSON entry like this (Claude Desktop, Claude Code `.mcp.json`, and others):

```json
{
  "mcpServers": {
    "xegesis": {
      "command": "xegesis",
      "args": ["-library", "C:\\Users\\you\\Documents\\Xegesis\\library.db", "mcp"]
    }
  }
}
```

Global flags such as `-library` go before `mcp`. Without `-library`, the server uses the
`XEGESIS_LIBRARY` environment variable, then the default library in your user config folder.
If `xegesis` is not on your `PATH`, give the full path to the executable as `command`.

With Claude Code: `claude mcp add xegesis -- xegesis -library /path/to/library.db mcp`.

## Tools

- The tool name is the command ID with dots replaced by underscores: `note.create` is `note_create`.
- The description is the command's summary, plus "Changes the library; recorded in history." for
  commands that write.
- `inputSchema` is the command's parameter schema. Required parameters are listed in `required`.
- Annotations: `readOnlyHint` is true for commands that do not write; `destructiveHint` is true for
  IDs ending in `.delete` or `.remove`.
- A successful result is a text block holding the JSON result, and `structuredContent` when the
  result is a JSON object.
- A failed command returns `isError: true` with text `code: message` (for example
  `bad_params: ...`), so the model can read it and retry. Protocol problems use JSON-RPC errors:
  -32700 parse error, -32600 invalid request, -32601 method not found, -32602 invalid params
  (including an unknown tool).

## Assistant mode

`xegesis mcp-assistant` (also the desktop app's executable: `Xegesis.exe -library <path> mcp-assistant`)
serves a different, fixed list: only the AI assistant's read-only tools (`search_library`,
`read_passage`, `list_bibles`, `read_book`, `list_notes`, `read_note`, `list_sermons`, `read_sermon`,
`list_highlights`, `passage_report`, `word_report`, `look_up_word`). None writes, no other command is
reachable, and every text it returns passes the licence gate as for an online model: quoting limits
apply and personal copies of books are withheld. Xegesis starts it for Claude Code and Codex when you
choose them as the assistant; set `XEGESIS_ASSISTANT_BIBLE` to name the Bible used when a call names none.

## Try it by hand

```
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"verse_parse","arguments":{"ref":"John 3:16"}}}' | xegesis mcp
```

`xegesis help` lists the commands; `xegesis describe <id>` shows one command's parameters.
