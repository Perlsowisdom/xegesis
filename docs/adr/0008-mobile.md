# 0008: Mobile apps reuse the Go core and the web interface

## Decision

Xegesis will run on Android and iOS. The Go core is built into each app with `gomobile` (BSD-3), and
the existing web interface runs in the platform's web view. A thin native shell per platform (Kotlin
on Android, Swift on iOS) passes `Run(command, json)` calls between the web view and the core and
provides platform features: file picking, keychain and sharing.

## Reasons

- Every feature is already a command with JSON in and out, so a mobile shell needs one call.
- The interface reaches the core only through `frontend/src/api.ts`; mobile swaps that one file.
- Embedding a Go server in a mobile app is proven; any documents its iOS build this way.
- Rust with Tauri 2 would cover all five platforms with one framework, but would mean rewriting the
  core and interface, and would rule out embedding any directly (ADR 0001).

## Consequences

- Two shells to maintain: Wails on desktop, native shells on mobile.
- The interface must work on small screens and with touch.
- Memory matters more on phones: tests check for leaked goroutines (goleak, MIT) and the 10,000-book
  test (T12) checks a memory budget; the app sets a memory limit (`GOMEMLIMIT`).
- An Android trial (T31) proves the approach before mobile work grows.
