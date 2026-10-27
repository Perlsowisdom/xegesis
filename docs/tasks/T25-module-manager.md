# T25: Get Bibles (module manager)

Brief for one agent (Opus). Read `AGENTS.md` first; its clean-room and licence rules apply.

## Problem

Installing a Bible today means finding a file online, knowing what OSIS or USFM is, picking the file and
filling in a licence form by hand. Users should instead browse Bibles by language and click Install.

## Sources (verify every URL and format detail against the live documentation before relying on it)

- **CrossWire SWORD repositories.** Repository list: `masterRepoList.conf` on crosswire.org. Each
  repository has a `mods.d.tar.gz` of module `.conf` files and per-module zip packages. Format
  documentation: the CrossWire wiki (module `.conf` keys, `ModDrv` drivers, index and data files).
- **eBible.org.** A catalogue of translations (`translations.csv`) and USFM zips per translation, with
  copyright and licence details per translation.

## Rules specific to this task

- Write the SWORD reader from CrossWire's published documentation. Never copy, port or translate code
  from libsword, JSword, Xiphos, BibleTime or AndBible (GPL/LGPL).
- SWORD's raw text index is positional, so mapping entries to verses needs each versification's book,
  chapter and verse counts. These counts are facts; take them from a public-domain or permissively
  licensed dataset, or derive them from documented sources, and record the provenance in
  `docs/modules.md`. If the only source you can find is GPL code, stop and report to the coordinator.
- Locked modules (a `CipherKey` entry in the `.conf`) are listed as locked and never installed
  without the user's own unlock key. Never attempt to bypass encryption.
- Licences come from the module's own fields (`DistributionLicense`, `Copyright`, `About` in SWORD;
  the catalogue fields on eBible.org) and map onto `internal/licence` types. Ask the user only when
  the mapping is missing or unclear. Record what the source said, word for word, in the licence's
  attribution or rights fields.
- Downloads run only when the user clicks Install. Show the size first. Keep downloaded files in a
  temporary folder, verify them, convert them, then delete them.

## What to build

1. `internal/bible/sword`: read `zText`/`zText4`/`RawText`/`RawText4` Bible modules (and `zCom`/`RawCom`
   commentaries if time allows) into the existing module format (`docs/modules.md`), converting OSIS
   or ThML markup with the existing importers' markup tags.
2. `internal/bible/repos`: fetch and cache repository lists and module catalogues (CrossWire and
   eBible.org), refresh on demand, work offline from the cache.
3. Commands: `bible.sources` (list and refresh catalogues), `bible.available` (filter by language,
   type, text, licence; marks installed and locked items), `bible.get` (download and install one,
   with progress reporting suited to the interface), `bible.updates` (installed modules with newer
   versions). Keep `bible.install` for local files, and let it accept `.zip` archives.
4. Interface (`frontend/src/views/bibles.ts`): a "Get Bibles" screen as the default Bibles view, with
   a language picker, search, a list with name, language, licence and size, Install buttons with
   progress, and "Install from file" kept as a secondary action. Rebuild `frontend/dist`.

## Done when

- Tests with small recorded fixtures (no network): parsing `.conf` files and catalogues, reading a
  tiny zText module you build in the test, licence mapping, locked modules refused, offline cache.
- A manual check against the live sources installs one public-domain Bible from each source.
- `gofmt -l .`, `go vet ./...` and `go test ./...` pass; the desktop app builds.
