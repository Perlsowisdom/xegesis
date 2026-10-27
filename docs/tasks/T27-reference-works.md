# T27: Commentaries, dictionaries and original-language tools

Brief for one agent (Opus), after T25. Read `AGENTS.md` first.

## What to build

1. **Module kinds beyond Bibles**: verse-keyed works (commentaries, cross-reference sets) and
   headword-keyed works (dictionaries, lexicons, Strong's dictionaries), stored in the module format
   in `docs/modules.md` with a `kind` field. Import from SWORD `zCom`/`RawCom`/`zLD`/`RawLD`
   (using T25's reader) and from STEPBible's openly licensed data (TAHOT, TAGNT and the brief
   lexicons; record the CC BY licence and attribution).
2. **Commands**: `reference.for_passage` (every installed commentary and cross-reference entry for a
   range), `lexicon.lookup` (by Strong's number or lemma), `word.study` (occurrences of a Strong's
   number across tagged Bibles, grouped by translation rendering).
3. **Interface**: a passage panel listing commentaries and cross-references for the passage being
   read, and a word panel when you click a tagged word.

## Done when

Clicking a tagged word in a tagged Bible shows its lexicon entry and occurrences. A passage shows its
installed commentary entries and cross-references. Tests use tiny fixtures.
