# T13: Word, passage and topic reports

Brief for one agent (Opus). Read `AGENTS.md` first: clean room (use generic names, not another
product's), design philosophy (simple by default, plain words), every feature a command.

## Idea

A report gathers in one place everything your library holds about a passage, a word or a topic. It is
built only from what you have installed and from open data.

- **Passage report** (for example "Romans 8:28-30"), built from:
  - the text in your Bibles, side by side;
  - commentary entries and cross-references (T27);
  - library books that cite the passage (`search.refs`);
  - your notes, highlights and lists on it (T14);
  - its reading-plan days (T16);
  - the original-language words in it, with glosses from tagged Bibles (T27).
- **Word report** (a Strong's number, a lemma or an English word), built from:
  - lexicon entries;
  - where the word occurs, with a count by book of the Bible and a small chart;
  - how each Bible translates it;
  - related words that share a root, where the lexicon data says so;
  - library books that use it.
- **Topic report** (a word or phrase, such as "grace" or "covenant"), built from:
  - topical index entries, if a public-domain topical index is installed (for example Nave's or
    Torrey's from the CrossWire catalogue; install them through Get Bibles like other dictionaries and
    check their licences);
  - dictionary articles;
  - Bible verses that match;
  - the best library passages from search.

## What to build

1. **Commands:** `report.passage`, `report.word` and `report.topic`. Each returns sections with a stable,
   documented shape, so the interface, the CLI, the MCP server and printing (T18) all use the same data.
   - Each section names its source and carries its attribution and licence.
   - Long sections are paged or limited, with a "more" cursor.
   - Reports run quickly by gathering sections at the same time; the first sections show within about
     a second at 10,000 books.
2. **Interface:** a Reports view, in a new file `views/report.ts` with minimal wiring.
   - Start a report from the passage box, the word panel ("Word report"), the right-click menu where
     one exists, or the Reports view.
   - Sections are collapsible, and empty sections are hidden.
   - Every reference, word and book in a report is a link.
3. **Keeping reports:** "Save as note" stores a snapshot of the report's links and headings, not
   copyrighted text. Excerpts follow licences as T14 does.

## Rules

- Work only in your worktree.
- Another agent (T18, printing and exporting) works at the same time:
  - do not build print or export, only the report data and its view;
  - T18 will add printing of reports through your commands;
  - use migration **0008** if you need one; T18 uses 0009.
- Do not commit `frontend/dist`.
- Use a scratch library (`XEGESIS_LIBRARY`); install only public-domain or openly licensed data.

## Done when

- A passage report for Romans 8:28 shows Bible text, commentary, cross-references, citing books, your
  notes and the Greek words, each from the right source with attribution.
- A word report for G25 shows the lexicon entry, a count by book with a chart, translation renderings
  and occurrences.
- A topic report for "grace" shows topical entries (with a topical index installed) plus Bible and
  library results.
- Each report's first sections show within about a second, measured at 10,000 books with the T12
  generator.
- `gofmt`, `go vet`, `go test`, the frontend tests and build, and the desktop build pass.
