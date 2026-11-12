# Xegesis

A free, open-source Bible study app for Windows, macOS and Linux: fast, offline-first, and drivable by both people and AI agents.

Xegesis keeps your library, notes and sermons on your own computer in open formats. It has no accounts and no telemetry; the few features that use the internet are off until you turn them on (see [what Xegesis sends](docs/privacy.md)).

Status: under active development. Windows comes first; macOS and Linux builds follow.

## Installing (Windows)

- **Microsoft Store**: search for Xegesis once it is listed there. The Store installs updates.
- **Direct download**: take `Xegesis-<version>-windows-x64-setup.exe` from the [Releases page](https://github.com/Perlsowisdom/xegesis/releases). It installs for your user only (no administrator prompt). The setup is not code-signed, so Windows SmartScreen may ask you to confirm: choose More info, then Run anyway. Compare the file with `SHA256SUMS` on the release page if you like. This build can check for updates once a day, but only if you turn that on; each update is verified with the project's own signature before it runs.

Uninstall it from Windows Settings, Apps. Your library (`%APPDATA%\Xegesis`) is kept unless you tick "Also remove my library".

## Building

Needs Go (see `go.mod`). On Linux, also `libgtk-3-dev` and `libwebkit2gtk-4.1-dev`. On macOS, build the
desktop app with `CGO_LDFLAGS="-framework UniformTypeIdentifiers"`.

```sh
go test ./...
go build ./cmd/xegesis                                       # CLI
go build -tags desktop,production -o xegesis-desktop ./app   # desktop app (add ,webkit2_41 on Linux)
```

Release builds (setup and Store package) are made by `scripts/package-nsis.ps1` and `scripts/package-msix.ps1`; `docs/releasing.md` has the whole process. `go run ./cmd/xegesis version` prints the version a build was stamped with.

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

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md) first. Xegesis is designed clean-room from public sources: contributions must not use or copy any commercial Bible software.

## Licence

Licensed under either of

- Apache License, Version 2.0 ([LICENSE-APACHE](LICENSE-APACHE))
- MIT License ([LICENSE-MIT](LICENSE-MIT))

at your option.

Unless you explicitly state otherwise, any contribution intentionally submitted for inclusion in this work, as defined in the Apache-2.0 licence, shall be dual-licensed as above, without any additional terms or conditions.
