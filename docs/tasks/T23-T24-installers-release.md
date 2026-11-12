# T23 and T24: Windows installers, Store package, signed auto-update and the release process

Brief for one agent (Sonnet), with the user's decisions from 12 November 2026. Read `AGENTS.md`,
ADR 0009 (privacy) and `docs/privacy.md` first.

## The user's decisions

- **Windows first.** macOS and Linux come later.
- **Two channels.**
  1. **Microsoft Store**, as an MSIX package. Microsoft signs it at certification, and the Store
     installs updates. The Store build must **not** update itself, to follow Store policy.
  2. **Direct download**, as an **unsigned** NSIS setup `.exe` (Wails supports NSIS). It has its
     own auto-update.
- **Auto-update for the direct download.**
  - It is off until the user turns it on (ADR 0009). On the first run, ask once, plainly.
  - When on, it checks the public GitHub Releases of the repo, downloads a newer installer, and
    installs it after asking the user ("Restart to update").
  - **Every update must carry our own Ed25519 signature** over the installer's SHA-256. The app holds
    the public key, and refuses any download whose signature doesn't verify. The installer is not
    code-signed, so this signature is what keeps updates safe.
  - Use Go's standard `crypto/ed25519`. Don't add a third-party updater framework unless it is
    permissively licensed and clearly simpler.
- **The repository will be made public.** The coordinator flips the visibility after the user's
  final go. Releases are then public GitHub Releases on the same repo.

## Build

1. **Version stamping.**
   - The version comes from a git tag `vX.Y.Z` and is set at build time through `-ldflags`.
   - Help → About shows it.
   - Add a `version` command.
2. **Direct-download installer (NSIS).**
   - Per-user install by default, so no admin prompt.
   - Start menu shortcut, and an uninstaller that keeps the user's library (`%APPDATA%\Xegesis`)
     unless the user ticks "Also remove my library".
   - Install the WebView2 runtime if it is missing (Wails' bootstrapper).
   - The licence files go in the install folder.
3. **The updater** (`internal/update`).
   - A manifest is published with each release: `latest.json` with version, installer URL, SHA-256
     and Ed25519 signature.
   - The updater checks it, then downloads to a temporary folder and verifies the hash and the
     signature.
   - It then runs the installer silently after the app exits, and relaunches the app.
   - **Store builds** are built with a tag or flag that removes the updater and its setting
     completely.
   - Settings gets a section for this, with "Check for updates" and the last check time.
   - The endpoint goes in `docs/privacy.md`. Nothing is sent but the HTTP GET.
4. **Signing tool.**
   - A small Go command, `cmd/xegesis-sign` (or `xegesis sign` in the main CLI). `keygen` writes a
     key pair, and `sign <file>` prints the signature.
   - The **private key must never be committed.** CI reads it from the `XEGESIS_UPDATE_KEY` secret.
     The user creates the key and adds the secret themselves; write the exact steps in
     `docs/releasing.md`.
   - Embed the public key in the app at build time, from a committed file
     `internal/update/pubkey.txt`. Commit a **placeholder or test key only**, and document how to
     replace it.
5. **Microsoft Store package (MSIX).**
   - Build an MSIX from the Wails exe: the manifest, logos in every required size (made from the
     app's existing icon), the capabilities needed (`runFullTrust` for a desktop app), and a
     publisher placeholder to fill in from Partner Center.
   - Use the Windows SDK's `makeappx`, or `winapp`/MSIX tooling available on GitHub's Windows
     runners.
   - It is **not** signed for submission. The Store signs it. For a local test install, a script
     can sign it with a throwaway self-signed certificate. Document this.
   - The Store build uses the no-updater build tag.
6. **CI release workflow** (`.github/workflows/release.yml`) on `windows-latest`, triggered by a
   `v*` tag. It:
   - builds the frontend and the Go app;
   - runs `go test ./...` and `npm test`;
   - builds the NSIS installer and the MSIX;
   - computes the SHA-256 and signs it with `XEGESIS_UPDATE_KEY`, skipping signing with a warning
     when the secret is missing;
   - writes `latest.json`;
   - creates a **draft** GitHub Release with the installer, the MSIX, `latest.json` and
     `SHA256SUMS`.

   Also add a normal CI workflow (`ci.yml`) on pushes and pull requests: Go vet and tests, and
   vitest and the build. The real-app e2e stays local, because it needs a desktop session.
7. **Store listing materials** in `docs/store/`:
   - a description and short description;
   - a feature list;
   - a privacy policy, written from ADR 0009 and `docs/privacy.md`, published as a page in the
     repo for the Store's privacy-policy link;
   - screenshots taken by an e2e script from the harness's library copy, at the Store's required
     sizes;
   - a checklist of what the user must do in Partner Center: create the free individual account,
     reserve the name, age rating, submit.
8. **Pre-public cleanup.**
   - Replace the hard-coded `C:\Users\perlj\...` default in `frontend/e2e/app.mjs` with an
     environment variable, or a path under `%USERPROFILE%`.
   - Remove the user's name from paths in `docs/tasks/T34e-editor-behaviour.md`.
   - Add or check the root `README.md` (what it is, licence, how to build and install), and
     `CONTRIBUTING.md` (including the clean-room rule from ADR 0007).
   - Check that `LICENSE-MIT` and `LICENSE-APACHE` are present.
9. **`docs/releasing.md`.** The step-by-step release process:
   - bump and tag;
   - CI makes the draft;
   - check it, then publish;
   - upload the MSIX to Partner Center;
   - generate and rotate keys.

## Tests

- Go tests for the updater:
  - a version is newer or older;
  - the manifest is parsed;
  - the signature verifies, and is rejected when the hash is wrong, the signature is wrong, or it
    was made with another key;
  - a download happens only when the setting is on, against an `httptest` server. There must be no
    real network calls.
- A test that the Store build has no updater.
- Run the workflows' steps locally where possible: NSIS build, MSIX packing, test signing.
- A real-app e2e check for the Settings update section, and for the first-run question being off by
  default.
- Run only touched e2e files while working, and the full suite once at the end.

## Rules

- No real network calls in tests.
- Never commit a private key, and never use the user's real library.
- Run `npm ci`; no junctions.
- Run long commands with timeouts, and close every process you start. Never touch
  `%LOCALAPPDATA%\Temp\xegesis-check\Xegesis*.exe`.
- Don't commit `frontend/dist`.
- **Don't push, don't create tags or releases, and don't change repository settings.** The
  coordinator does those with the user.

## Done when

- `wails build` with NSIS produces a working setup `.exe` locally, and an MSIX is packed, which a
  test-signed install can open.
- The updater's Go tests pass, and the Store build has no updater.
- `release.yml` and `ci.yml` are written and their steps are checked locally as far as possible.
- The docs and Store materials exist, and the cleanup is done.
- The final full real-app suite passes.
