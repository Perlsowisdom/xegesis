# 0003: MIT OR Apache-2.0, permissive dependencies only

## Decision

Xegesis is dual-licensed MIT OR Apache-2.0. Every dependency must be permissively licensed.

## Consequences

- No GPL, LGPL, AGPL or source-available code. Excluded examples: MuPDF (AGPL), libsword (GPL),
  anytype-heart (Any Source Available License).
- PDF rendering uses pdf.js (Apache-2.0).
- SWORD modules are read by Xegesis's own reader, written from the published module format.
- Bundled texts and datasets must be public domain or openly licensed, with the licence recorded.
