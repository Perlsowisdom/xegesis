# What Xegesis sends over the network

ADR 0009: Xegesis sends no telemetry. Only the features below use the network, each off until you turn
it on. This page says exactly what each one sends. Other features add their sections when they are built.

## Bible catalogues (T25)

Off until you turn them on: the "Turn on Bible catalogues" button in Bibles → Get Bibles, or
`bible.sources {"enable": true}`. `bible.sources {"enable": false}` turns them off again; the cached
lists stay on disk and can still be browsed offline.

Every request is an HTTPS `GET` (or `HEAD` for a size) with the header `User-Agent: Xegesis` and no
cookies, account, identifier, search terms or information about your library. The servers see your IP
address and which files you request, as with any download.

| When | Requests |
| --- | --- |
| You refresh the catalogues (`bible.sources` with `refresh`) | `https://crosswire.org/ftpmirror/pub/sword/masterRepoList.conf`; `mods.d.tar.gz` from each HTTPS repository it lists (for example `https://crosswire.org/ftpmirror/pub/sword/raw/mods.d.tar.gz`); `https://ebible.org/Scriptures/translations.csv` |
| You click Install, before anything is downloaded (`bible.get` with `preview`) | eBible.org only: `HEAD https://ebible.org/Scriptures/<id>_usfm.zip` for the size, and, when the catalogue does not name the licence, `https://ebible.org/<id>/copr.htm` |
| You confirm the install (`bible.get`) | SWORD: the module's data files (`ot.bzs`, `ot.bzv`, `ot.bzz`, `nt.…`, or `ot`, `ot.vss`, `nt`, `nt.vss`) from its repository folder; eBible.org: `https://ebible.org/Scriptures/<id>_usfm.zip` |

Listing, searching the catalogues and checking for updates (`bible.available`, `bible.updates`) read the
cache only and never use the network.
