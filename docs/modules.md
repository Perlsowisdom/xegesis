# Bible module format

Each Bible is one read-only SQLite file, `modules/<id>.db` (ADR 0002). The `bible.install` command
creates it by importing OSIS 2.1.1, USFM 3 or USX 3 source files, and `bible.get` from a SWORD module or an
eBible.org USFM archive; nothing else writes it. A module can
always be rebuilt from its sources, so the format may change between versions: `PRAGMA user_version`
holds the format version (currently 1) and Xegesis refuses files of another version.

The module ID is lowercase letters, digits, `-` and `_` (at most 64 characters) and is also the file
name.

## Tables

### `module`

One row describing the module.

| Column | Contents |
| --- | --- |
| `id` | Module ID, same as the file name |
| `kind` | `bible`, `commentary` or `crossref` (verse-keyed, in `verses`), `dictionary` or `lexicon` (headword-keyed, in `entries`; a lexicon is keyed by Strong's numbers) |
| `name`, `abbreviation` | Full and short names |
| `language` | BCP 47 language tag; `und` when unknown |
| `versification` | Verse numbering the verse IDs use: `english`, `hebrew`, or another name with a mapping table |
| `licence` | JSON of `internal/licence.Licence`, as the user stated it at import, or as mapped from a catalogue (see Licences below) |
| `source`, `version` | Where the text came from and which edition |
| `books` | Space-separated OSIS book IDs present, in canonical order |
| `verse_count` | Rows in `verses`, or in `entries` for a dictionary or lexicon |
| `created_at` | Unix time in milliseconds |

### `verses`

| Column | Contents |
| --- | --- |
| `id` | Verse ID `BBCCCVVV` (`internal/verse`) in the module's own numbering; verse 0 is a superscription |
| `end_id` | Last verse covered. Equal to `id` except for combined verses such as "1-2", which are one row |
| `text` | Plain text: no headings, notes or markup, white space collapsed |
| `markup` | The same text with light markup (below) |

A passage `start..end` is the rows with `id <= end AND end_id >= start`, so asking for verse 2 finds a
combined verse 1-2.

### `strongs`

`(number, verse_id)`: every Strong's number tagged in a verse, normalised to `H7225` / `G26` (no leading
zeros). Used by `bible.search` with `strong`.

### `versification_map`

`(system, module_id, system_id)`: an optional full mapping from this module's verse IDs to the verse IDs
of a named numbering system. When a module has rows for a system, conversions to and from that system use
them; otherwise Xegesis falls back to `verse.Convert`, and then to the same number. Load a table at import
with `mapping_file`, a UTF-8 text file of tab-separated lines:

```
# system <TAB> verse in this module <TAB> same verse in the system (OSIS references)
english	Mal.3.19	Mal.4.1
```

### `entries`

Dictionary and lexicon entries (empty in other modules): `key` (the headword as the source gives it),
`fold` (lower case without accents or vowel points, for lookup), `strong` (`G26`, `H1254A`; empty in
dictionaries), `lemma` and `lemma_fold` (the original-language word), `translit`, `morph`, `gloss`,
`text` and `markup` (the same light markup as verses, where `<p/>` separates paragraphs).

A commentary stores one row per note in `verses`, with `end_id` covering the verses it is about (a
note the source repeats for consecutive verses is stored once). A cross-reference set stores one row
per verse whose markup is a list of `<r o="…">` targets.

### `verses_fts`

An FTS5 table over `verses.text` (external content, `rowid` = verse ID), tokenised with
`unicode61 remove_diacritics 2`. Search ranks by `bm25`.

## Markup

The `markup` column is XML-like text with a small fixed set of elements. Text is escaped (`&amp;`,
`&lt;`, `&gt;`, `&quot;`). Elements never span verses: an element that continues into the next verse is
closed at the end of one row and opened again at the start of the next.

| Element | Meaning | In `text`? |
| --- | --- | --- |
| `<h>…</h>` | Section heading before or in the verse. `k="psalm"` marks a psalm title | no |
| `<p/>` | A paragraph or poetry line starts here | — |
| `<j>…</j>` | Words of Christ | yes |
| `<w s="H7225 H853" m="…">…</w>` | Tagged word: Strong's numbers (`s`) and morphology code as given in the source (`m`) and, when the source gives one, an English gloss (`g`) | yes |
| `<add>…</add>` | Words added by the translators | yes |
| `<f>…</f>` | Footnote | no |
| `<x>…</x>` | Cross-reference note | no |
| `<r o="Rom.5.8">…</r>` | A reference, with its OSIS target | yes, outside notes |

Other source markup (poetry groups, divine names, emphasis, tables) is kept as plain text. Markup that
cannot be placed is reported as import warnings, never as an error.

## Import mapping

| Source | Book and verse | Headings | Notes | Words of Christ | Strong's |
| --- | --- | --- | --- | --- | --- |
| OSIS | `div type="book"`, `verse osisID` (container or `sID`/`eID` milestones; several IDs = combined) | `title` (`runningHead` dropped) | `note`; `type="crossReference"` gives `<x>` | `q who="Jesus"` (container or milestones) | `w lemma="strong:…"`, `morph` |
| USFM | `\id`, `\c`, `\v 1` / `\v 1-2` | `\s#`, `\ms#`, `\mr`, `\r`, `\d`, `\sp`, `\qa`, `\cd` | `\f…\f*`, `\x…\x*` (origin `\fr`/`\xo` dropped) | `\wj` | `\w word\|strong="H7225" x-morph="…"\w*` |
| USX | `book code`, `chapter number`, `verse number` | `para` with heading styles | `note` | `char style="wj"` | `char style="w" strong=…` |

USFM headings are placed before the next verse. Introduction and title material (`\imt`, `\ip`, `\mt`,
`\toc`, …), alternate numbering (`\va`, `\vp`, `\ca`) and figures are skipped.

## Commands

`bible.modules`, `bible.install` (also from a `.zip` archive), `bible.remove`, `bible.read`, `bible.parallel`, `bible.search`, and for the catalogues `bible.sources`, `bible.available`, `bible.get`, `bible.progress` and `bible.updates`. None
of them writes `library.db`. `bible.parallel` and `bible.search` take references in a stated numbering
(`versification`, default `english`) and convert them for each module.

`bible.modules` lists Bibles by default; `kind` lists another kind, or `all`. `bible.search` searches
Bibles only unless given modules.

For reference works: `reference.for_passage` lists every installed commentary entry and cross-reference
for a passage (cross-references come from cross-reference sets and from `<x>` notes in installed
Bibles, merged, with the modules that give each); `lexicon.lookup` finds entries by Strong's number
(with or without the extended letter) or by original-language word or headword; `word.study` lists
where a Strong's number occurs in the tagged Bibles, grouped by the words each translation tags with it.

## SWORD modules

`internal/bible/sword` reads CrossWire SWORD Bibles, commentaries and dictionaries. It was written from CrossWire's published
documentation (the wiki's `.conf` reference and repository pages), a third-party prose description of
the index records, and inspection of public-domain modules; no SWORD, JSword or front-end code was used
(ADR 0007). Supported: the `RawText`, `RawText4`, `zText` and `zText4` Bible drivers, the `RawCom`, `RawCom4`, `zCom` and `zCom4` commentary drivers (the same verse index), the `RawLD`, `RawLD4` and `zLD` dictionary drivers, `ZIP` (zlib) and
`BZIP2` compression, OSIS, ThML, GBF, TEI (dictionaries) and plain-text entries in UTF-8 or Latin-1 (Windows-1252), in
the numberings below. Not yet: LZSS and XZ compression, general books (`RawGenBook`), and the KJVA,
NRSVA, LXX and other numberings listed below.
Locked modules (any `CipherKey` entry) are listed as locked and never installed.

Index files are little-endian records, one per entry: `ot.vss`/`nt.vss` hold start (4 bytes) and size
(2 bytes; 4 in RawText4) into `ot`/`nt`; `ot.bzv`/`nt.bzv` hold block (4), start (4) and size (2; 4 in
zText4) within a decompressed block, and `ot.bzs`/`nt.bzs` give each block's offset, size and
uncompressed size (4 bytes each) in `ot.bzz`/`nt.bzz`. Entries are positional: entry 0 is the module
introduction, 1 the testament introduction, then each book has an introduction entry followed, per
chapter, by a chapter introduction and the verses. Introductions keep only their headings, placed
before the next verse. Each testament's index must hold exactly the entries its numbering needs, which
checks both the download and the numbering.

Dictionaries (`RawLD`, `RawLD4`, `zLD`), seen in public-domain modules: `<name>.idx` holds start (4)
and size (2; 4 in RawLD4 and zLD) into `<name>.dat`, where an entry is the key, CR LF, then the text
(RawLD) or a block number (4) and entry number (4) (zLD). `<name>.zdx` gives each block's offset and
size (4 each) in `<name>.zdt`; a decompressed block holds an entry count (4), then start (4) and size
(4) per entry, then the texts. The `DataPath` of a dictionary ends with `<name>`. A dictionary with
`Feature=GreekDef` or `HebrewDef` and numeric keys (`00026`) is installed as a lexicon keyed by
Strong's number; TEI `orth` gives the lemma (`type="trans"` the transliteration). Commentaries keep
the verse index of Bibles and are installed as `commentary` modules.

### Versification counts

`internal/bible/sword/kjv.txt` lists the verses per chapter of the KJV numbering (the documented
default). Provenance: counted from the public-domain King James Version USFM published by eBible.org
(`https://ebible.org/Scriptures/eng-kjv2006_usfm.zip`, 2026-10-27), taking the highest `\v` in each
chapter: 23 145 Old Testament and 7 957 New Testament verses, matching eBible.org's catalogue.
CrossWire's KJV and ASV modules have exactly 2 + books + chapters + verses index entries per testament
with these counts, and their verses land where expected (Ps 23:1, 3 John 1:14, Rev 22:21).

The other numberings are in `internal/bible/sword/v11n/<name>.txt`, one `[ot]` and one `[nt]` section
of books in index order. Their counts were derived on 2026-10-28 from the index files of public-domain
or openly licensed CrossWire modules: modules built with CrossWire's tools keep the OSIS book and
chapter markers (`<div type="book" osisID="…">`, `<chapter osisID="…">`) in the book and chapter
introduction entries, so the verse entries between two markers are a chapter's verse count. Several
modules of each numbering were combined (each fills markers the others lack) and the result was
accepted only when every book and chapter had a marker, chapters ran 1, 2, 3 … in every book, the
modules agreed wherever both had a marker, and each module's index held exactly 2 + books + chapters +
verses entries. Spot checks: Synodal Ps 22:1 is "Господь - Пастырь мой" (English Ps 23:1), Vulg Dan 13:1
begins Susanna, German Mal 3:19 is English Mal 4:1, MT Ps 51:3 is English Ps 51:1.

| Numbering | Modules counted | Xegesis numbering |
| --- | --- | --- |
| NRSV | NHEB, KorRV | `nrsv` |
| MT | JPS, OSHB (Old Testament only) | `hebrew` |
| Leningrad | WLC (Old Testament only, Hebrew book order) | `hebrew` |
| Synodal | RusSynodal, CSlElizabeth | `synodal` |
| Vulg | VulgClementine, VulgSistine, DRC, FreVulgGlaire | `vulg` |
| Catholic | ChiSB, CroSaric | `catholic` |
| Catholic2 | FreCrampon | `catholic2` |
| German | GerBoLut, GerTextbibel, Mg1865 | `german` |
| Luther | GerMenge | `luther` |
| Calvin | HunKar, FreBA | `calvin` |

Books Xegesis does not know (Luther's `AddDan`, Vulg's `EpLao`) keep their index entries but are not
imported. Xegesis's chapter limits were raised to fit these numberings: Esther 16, Psalms 151, Daniel 14.

Not supported, with the reason, so their modules are listed as "cannot be installed": **KJVA** (the
only open module, DutSVV, has none of the Apocrypha but Baruch, so the book order and counts cannot be
confirmed; the eBible.org KJV Apocrypha counts do not add up to its index), **NRSVA** (no open module
has markers for Greek Esther 8–9 or for the books after 2 Esdras), **LXX** (no open module marks the
books after 1 Enoch), and Orthodox, SynodalProt, Segond, DarbyFr (not attempted).

#### Mapping to the english numbering

A SWORD module in one of these numberings is installed with a `versification_map` table for the
`english` system, worked out from the counts (`Versification.EnglishMap`) using differences that are
facts of the texts: Psalms in the Greek and Latin numbering (Ps 9 = English 9-10, Ps 113 = 114-115,
Ps 114-115 = 116, Ps 146-147 = 147, and the chapters between shifted by one; Ps 151 is the Psalm 151
book); psalm titles counted as verses (a psalm with one or two more verses than the English one has
those as title, english verse 0); books whose chapters match the Hebrew layout of `verse.Convert`; and
the Greek additions to Daniel (3:24-90 to the Prayer of Azariah, 3:91-100 renumbered, chapters 13 and
14 to Susanna and Bel) and to Esther in the Latin layout (10:4-16:24 to Esther (Greek)). Chapters that
differ in any other way keep their numbers; `TestEnglishMapCoverage` logs how many there are per
numbering (for example 2 for NRSV, Rev 12 and 3 John, and 44 for MT).

## Bible catalogues

`internal/bible/repos` lists and installs Bibles from online catalogues. They are off until the user
turns them on (ADR 0009; `docs/privacy.md` lists every request).

- **CrossWire SWORD repositories.** The repository list is CrossWire's `masterRepoList.conf` (lines
  `id=Type=Name|host|path`; only `HTTPSPackagePreference` entries are used). Each repository folder has
  `mods.d.tar.gz`, a tar of module `.conf` files; a module's files are fetched one by one from the
  repository folder plus its `DataPath` (an absent testament returns 404 and is skipped).
- **eBible.org.** `https://ebible.org/Scriptures/translations.csv` is the catalogue; a translation's text
  is `https://ebible.org/Scriptures/<id>_usfm.zip` and its copyright page `https://ebible.org/<id>/copr.htm`.

Catalogues are cached in `catalogues/` next to `library.db` (the raw downloaded files plus
`state.json`) and listed offline. Installed modules record the item's `origin` as their `source`
(`<repository URL>#<module>` or `https://ebible.org/<id>/`) and its version, so updates can be found.

Downloads go to a temporary folder, are checked (SWORD index sizes against the numbering; eBible.org
zips must hold USFM files; every file must match its announced length), converted, and deleted.

### Licences

The licence comes from the source's own fields; the user is asked only when the mapping is unclear.
What the source said is kept word for word: the catalogue item's `rights`, and the licence's
`attribution` (the `Copyright` text) and `rights_holder` (`CopyrightHolder`).

| Source says | Licence |
| --- | --- |
| SWORD `DistributionLicense=Public Domain`; eBible.org `Copyright` "public domain" | `public_domain` |
| `Creative Commons: BY-SA 4.0` (any CC value with a version), `CC0` | `creative_commons` with the SPDX ID |
| `GPL`, `GFDL` | `open_other`, sharing and commercial use allowed |
| `Copyrighted; Freely distributable`, `Copyrighted; Free non-commercial distribution` | `open_other`, sharing allowed, no commercial use |
| `Copyrighted; Permission to distribute granted to …`, `… non-commercially in SWORD format` | `open_other`, no sharing or commercial use, 10% print and export limits |
| eBible.org copyright page linking a Creative Commons licence | `creative_commons` with that licence |
| Anything else (`Copyrighted` alone, `See README file`, missing) | unclear: `bible.get` needs `licence_type` |

A licence taken from a catalogue has `confirmed_by_user` true only when the user accepted it
(`accept_licence`, which the Install button sends after showing it); one the user states is always
confirmed.

## Open datasets

Besides the catalogues, Get Bibles lists a few openly licensed datasets by name (`internal/bible/repos/datasets.go`),
once the catalogues are turned on. They download from their publishers' own sites when installed, are
converted, and the downloads are deleted. Each records CC BY 4.0 with its attribution line, and the
item's `rights` quotes the source's own licence words.

| Item | Source files | Installed as |
| --- | --- | --- |
| STEPBible TAHOT | `TAHOT Gen-Deu`, `Jos-Est`, `Job-Sng`, `Isa-Mal` from `Translators Amalgamated OT+NT/` at `https://raw.githubusercontent.com/STEPBible/STEPBible-Data/master/` | Hebrew Bible in English numbering (psalm titles as verse 0); each word a `<w>` with its Strong's number (extended letter dropped), morphology and gloss |
| STEPBible TAGNT | `TAGNT Mat-Jhn`, `Act-Rev` from the same folder | Greek New Testament: the words whose edition letters include `N` (Nestle-Aland based text) |
| STEPBible TBESG | `Lexicons/TBESG …` | Lexicon: lemma, transliteration, part of speech, gloss and the Abbott-Smith definition |
| STEPBible TBESH | `Lexicons/TBESH …` | Lexicon of glosses only. The file's definitions are the Abridged BDB of Online Bible, whose notice asks for permission before use, so they are not installed |
| OpenBible.info cross-references | `https://a.openbible.info/data/cross-references.zip` | Cross-reference set: references with at least one vote, the 25 best for each verse |

The STEPBible repository is CC BY 4.0 ("Data created by www.STEPBible.org based on work at Tyndale
House Cambridge"), checked on 2026-10-28; it asks others to refer to it as the source rather than
redistribute the files, which is why Xegesis downloads them from GitHub. OpenBible.info states its
cross-references are "licensed under a Creative Commons Attribution License".
