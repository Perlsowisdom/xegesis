# Contributing to Xegesis

Thank you for helping. Xegesis is a free, open-source Bible study app. These rules keep it open and safe
to share. [AGENTS.md](AGENTS.md) has the full working rules, for people and for AI agents alike; this
page is the short version.

## The clean-room rule (ADR 0007)

Xegesis is designed from public sources and open data only. This is not negotiable, and a change that
breaks it cannot be merged.

- **Do not use, run, study, screenshot or copy** the Logos Bible Software app, or any other commercial
  Bible software: not its files, file formats, databases, network traffic, content or interface.
  Do not decompile any commercial Bible software.
- **Design from** published specifications (such as CrossWire's SWORD module documentation), open
  datasets, and the ideas and workflows of open-source projects (AndBible, BibleTime, Xiphos, Ezra Bible
  App, STEPBible, the CrossWire SWORD Project, Calibre, KOReader). **Take ideas, never code**: most of
  these are GPL, and translating GPL code into Go is still copying it.
- **Describe features in generic terms.** Do not reuse another product's screen names, layouts or wording.
- **Name your source.** Each pull request that adds a feature says where the idea comes from: a
  specification, an open project, or a public description of the feature.
- If you are not sure whether something is allowed, ask in the pull request before writing it.

## Licences

Xegesis is licensed MIT OR Apache-2.0, and your contribution is too. Dependencies must be permissive
(MIT, Apache-2.0, BSD, ISC, zlib or similar). No GPL, LGPL, AGPL or "source available" code, even as a
linked library. Bundled Bibles and datasets must be public domain or openly licensed, with the licence
recorded.

## Privacy

No telemetry, ever. Only the features listed in `docs/privacy.md` use the network, each off until the
user turns it on, and each documented there. A change that adds a network call updates that page.

## Making a change

1. Build and test: see the README. `go vet ./...` and `go test ./...` must pass; the interface has
   `npm test` in `frontend/`.
2. Every feature is a registered command (`internal/command`); the app, the CLI and the MCP server all use
   the same commands. Do not make a feature reachable only from the interface.
3. Every change has tests. Format Go with `gofmt`.
4. Keep the words in the interface plain: no command names or jargon.
5. Do not commit private keys, API keys or anyone's library. Update signing keys are never in the
   repository (`docs/releasing.md`).
6. Open a pull request that says what changed, why, and its source (see the clean-room rule).
