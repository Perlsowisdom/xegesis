# 0001: Go core with a Wails desktop interface

## Decision

The core is written in Go. The desktop interface uses Wails, which renders a web interface in the
operating system's own web view.

## Reasons

- Go builds standalone binaries for Windows, macOS and Linux from one code base.
- The MIT-licensed sync libraries considered for later (any-sync) are written in Go.
- EPUB is HTML, so a web view displays books natively; pdf.js handles PDF.
- Wails is far lighter than Electron, which bundles a whole browser.
- The same Go core serves the CLI and MCP server.

## Alternatives considered

- **Rust with Tauri** (as ArtCraft's WordCraft uses): faster and lighter, but no reuse of any-sync and
  slower to develop.
- **Electron**: rejected as too heavy for the speed goals.
