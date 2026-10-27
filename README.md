# Xegesis

A free, open-source Bible study app for Windows, macOS and Linux: fast, offline-first, and drivable by both people and AI agents.

Status: foundation (milestone M0). The Phase 1 functional survey and the Phase 2 implementation plan are kept in the project's planning document.

## Building

Needs Go (see `go.mod`). On Linux, also `libgtk-3-dev` and `libwebkit2gtk-4.1-dev`. On macOS, build the
desktop app with `CGO_LDFLAGS="-framework UniformTypeIdentifiers"`.

```sh
go test ./...
go build ./cmd/xegesis                                       # CLI
go build -tags desktop,production -o xegesis-desktop ./app   # desktop app (add ,webkit2_41 on Linux)
```

## Using the CLI

Everything Xegesis does is a command. The CLI, the desktop app and the JSON channel all run the same ones.

```sh
xegesis commands                                    # list commands with their parameters
xegesis run verse.parse '{"ref":"Rom 8:28-30"}'
xegesis run note.create '{"body":"All things work together","ref":"Rom 8:28"}'
xegesis run history.list '{"object_type":"notes","object_id":"<id>"}'
xegesis serve                                       # one JSON request per line on stdin
```

The library lives in your user config folder (`Xegesis/library.db`); set `XEGESIS_LIBRARY` or `-library` to use another.

## Licence

Licensed under either of

- Apache License, Version 2.0 ([LICENSE-APACHE](LICENSE-APACHE))
- MIT License ([LICENSE-MIT](LICENSE-MIT))

at your option.

Unless you explicitly state otherwise, any contribution intentionally submitted for inclusion in this work, as defined in the Apache-2.0 licence, shall be dual-licensed as above, without any additional terms or conditions.
