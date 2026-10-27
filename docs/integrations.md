# Online Bible sources: API.Bible and the Digital Bible Library

Both integrations are optional, need your own account, and are written from the services' public
documentation only (clean-room, see ADR 0007). Code: `internal/integrations/`.

Secrets (your API key, DBL token and secret) are stored in the operating system keychain (Windows
Credential Manager, macOS Keychain, Secret Service on Linux) under the service name `xegesis`. They
are never written to `library.db`, a config file or a log, and no command echoes them.

## API.Bible

Docs: <https://docs.api.bible>. Base URL `https://rest.api.bible/v1`, key sent in the `api-key` header.

### Setup

1. Create a free account at <https://api.bible> and copy the key from your dashboard.
2. Run `apibible.set_key` with `{"key": "..."}`. Each user must use their own key; Xegesis ships none
   and you must not share yours (the terms forbid it).
3. `apibible.status` with `{"verify": true}` checks the key (uses one request of your monthly quota).
4. `apibible.bibles` (filter by `language`, `abbreviation`, `name`) lists what your plan can read.
   The free Starter plan includes open-access Bibles and up to three copyrighted ones you choose in
   your dashboard; a Bible outside your plan returns "not included in your plan" (HTTP 403).

### Commands

| Command | Purpose |
| --- | --- |
| `apibible.set_key` / `apibible.clear_key` | Store / remove the key in the keychain |
| `apibible.status` | Is a key set; optionally does it work |
| `apibible.bibles`, `apibible.books` | Browse Bibles and their books |
| `apibible.passage` | Text for a reference such as `Rom 8:28-30` or `Ps 23` |
| `apibible.search` | Search a Bible |

Endpoints used: `GET /bibles` (`language`, `abbreviation`, `name`, `include-full-details`),
`/bibles/{id}`, `/bibles/{id}/books`, `/bibles/{id}/passages/{passageId}`,
`/bibles/{id}/chapters/{chapterId}`, `/bibles/{id}/search` (`query`, `limit`, `offset`, `sort`).

References are converted to API.Bible IDs (`ROM.8.28-ROM.8.30`, `1JN`, `SNG`, `PSA`, `PHM`, ...).
Whole chapters use the chapters endpoint, up to 5 chapters per request. A range that ends "at the end
of the chapter" part-way through (for example `Rom 8:28-end`) is refused: give the last verse.
API.Bible limits one passage request to 200 verses.

### Terms that apply (summary)

Read the full terms: <https://api.bible/terms-and-conditions>, the licensing summary
<https://care.api.bible/article/396-licensing-policies-quick-reference> and
<https://docs.api.bible/quick-start/licensing-and-access>.

- **Non-commercial only on the free plan.** Anything that earns revenue (ads, paid access) needs a
  paid plan. Results from Xegesis carry `commercial_allowed: false`, `share_allowed: false` and
  `store_allowed: false` in their `licence`, so the licence rules in `internal/licence` refuse
  commercial use.
- **Copyright notice.** Every passage and search result includes `copyright` and a `notice` string to
  show wherever the text is displayed. Starter-plan users must also show a visible citation linking
  to <https://api.bible>; the notice includes it. Public-domain and Creative Commons Bibles have their
  own citation templates in the terms (Appendix A).
- **Usage reporting (FUMS).** API.Bible requires its Fair Use Management System for web apps and
  returns a token with each Scripture response (`meta.fumsToken`). Xegesis reports every token it
  displays by sending `GET https://fums.api.bible/f3?t=<token>&dId=<device>&sId=<session>`, as the
  documentation describes for non-browser reporting. `dId` is a random anonymous ID made once per
  installation (kept in the keychain), `sId` is a random ID per app session, and no user ID is sent.
  The result's `usage` field lists the tokens and `reported` says whether sending succeeded. Cached
  passages are reported again each time they are shown. If you build a web front end that shows the
  text itself, use the documented JavaScript tracker or report the same tokens.
- **Caching.** Cached content must be refreshed at least every 30 days (recommended 14 or fewer), may
  not be modified, and must be removed within 72 hours after your account is terminated. Xegesis
  caches responses (metadata and passages, not searches) **in memory only** with a default lifetime
  of 14 days, capped at 30 days, and clears the cache when you set or clear your key. Nothing is
  written to disk, so nothing outlives the app. Text is never altered other than joining chapters.
- **Rate limits.** Starter: 5,000 calls per month. A rate-limit response is reported as an error.
- Also from the terms: keep the key confidential; sub-licensing is not permitted.

### Not confirmed in the documentation

- The exact error body shape (parsed leniently) and the 429 `Retry-After` header.
- Whether search and chapter responses always include `meta.fumsToken` (the chapter page lists the
  older `fumsId`/`fums` fields; the fair-use page names `fumsToken`). Xegesis reads both.
- A single-verse passage is requested as `ROM.8.28-ROM.8.28`.
- Verse limits (a 500-verse display limit is mentioned by secondary sources, not by the pages read).

## Digital Bible Library (DBL)

Docs: <https://app.thedigitalbiblelibrary.org/static/docs/> (API 4.0 reference). Base URL
`https://thedigitalbiblelibrary.org/api`.

### What an individual can and cannot do

| Capability | Without an account | Individual user | Organisation credentials |
| --- | --- | --- | --- |
| List public and open-access entries (`/public_entries_list`, `/open_access_entries_list`) | Yes | Yes | Yes |
| List owned / publishable entries | No | No | Yes |
| Entry details with licences (`/entries/{id}`) | No | No | Yes |
| Download a text bundle (`.../revision/latest/license/{id}.zip`) | No | No | Yes, for entries the organisation owns or is licensed to publish |

The API's current authentication is a token and secret key created by an **organisation
administrator** (Admin, Organizations, Roles, Add New Access Token). The older email-and-key login is
deprecated and its key is equivalent to the account password, so Xegesis does not support it.
Downloading needs a licence ID held by an organisation with publisher rights (or `owner`); the
documentation describes no way for an individual to download a licensed text through the API.
Individuals who have a licensed text from a rights holder should import the bundle file through the
library import (not part of this task) instead.

### Commands

`dbl.status`, `dbl.entries` (`list`: public, open_access, owned, publishable; `type`; `latest`),
`dbl.entry`, `dbl.download` (writes a zip to a new file you name; refuses to overwrite),
`dbl.set_credentials`, `dbl.clear_credentials`. Entry results carry a cautious `licence` (not
confirmed, non-commercial); the entry's own metadata holds its real terms and you must confirm them
before reuse.

### Not confirmed

- The envelope of list responses is not shown in the reference; the client accepts a bare array or an
  object holding the array.
- The request signing (HMAC-SHA1 over method, path and selected headers, keyed with the token, then
  the secret) follows the reference's description and is tested only against that description, not
  a live organisation account. Treat `dbl.entries owned`, `dbl.entry` and `dbl.download` as
  unverified until tried with real credentials.
