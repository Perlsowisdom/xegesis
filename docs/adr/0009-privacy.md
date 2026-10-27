# 0009: Privacy

## Decision

- **No telemetry.** Xegesis never sends usage statistics, analytics or identifiers.
- **One exception: bug and crash reports to the developer,** and only with the user's consent:
  - Crashes and errors are written to a local log first.
  - On the next start, or from Help → Report a problem, the user sees exactly what the report contains,
    can edit or remove parts of it, and chooses whether to send it. Nothing is sent automatically.
  - Reports contain the app version, operating system, error, stack trace and recent log lines. They
    never contain note or document text, book titles, file paths, verse lists, search terms, keys or
    account details; the report builder strips these, and the preview shows what remains.
  - Where reports go (an endpoint run by the developer, or a self-hosted, permissively licensed
    collector) is decided when the feature is built (T32).
- **Every network call is listed.** Only named features may use the network: API.Bible, the Digital
  Bible Library, the Bible catalogues (T25), sync (M3) and crash reports. Each is off until the user
  turns it on, and `docs/privacy.md` says exactly what each one sends.
- **Sync must not let servers read user data.** Any sync option chosen in M3 encrypts content end to
  end (for example any, or Syncthing with untrusted devices).
- **On-device data** is protected by the operating system's disk encryption; the app recommends
  turning it on.

## Consequences

- No analytics libraries or SDKs, including inside dependencies; dependency reviews check for them.
- Integrations show what they send before first use.
