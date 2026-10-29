# T34: Read mode and study mode

Brief for one agent (Opus), later in M2, after T27 (word look-up needs lexicons) and T15. Read
`AGENTS.md` first. Requested by the user on 28 October 2026.

## Idea

Two ways to use the same Bibles and books.

- **Read mode** is for reading and nothing else:
  - one text, full width, with as little on screen as possible;
  - only reading settings: font, size, line spacing, margins, background and text colour (including
    sepia and dark), and page or scroll.
- **Study mode** is the workspace:
  - several tabs and panes (Bible beside books and notes);
  - visual filters, such as highlighting words of Christ, Strong's numbers or one lemma;
  - lemmas and morphology, tags, and passage tools.

Both modes are one switch away, and the switch keeps your place.

In both modes, **right-clicking selected text** opens a context menu:
- Look up (the word in a lexicon or dictionary, or the reference in the Bible pane);
- Highlight (with colours);
- Add a note;
- Copy (within the licence).

## Structure: one shared document renderer

Both modes use the same renderer; the modes only add features around it. Today Bibles
(`views/reader.ts`) and books (`views/book.ts`, `pdfview.ts`) are drawn by separate code. T34 merges
them into one renderer, in `frontend/src/document/`, that both modes use.

The shared renderer handles:
- Bible passages (one or parallel), EPUB and other reflowable books, and PDF pages;
- positions and locators: verse IDs, EPUB places and PDF pages, so the switch between modes keeps
  your place;
- text selection that works the same everywhere, including the triple-click case fixed earlier;
- highlights and notes drawn in the text, from `highlight.*` and `note.*` (T14);
- links: verse references to the Bible, and in-book jumps;
- reading settings: font, size, spacing, margins and theme.

Shared by both modes:
- highlighting, with the core colours;
- notes: add a note on a selection, and see note markers in the text;
- the right-click menu (Look up, Highlight, Add a note, Copy);
- Copy within the licence (`book.copy`).

Study mode adds:
- more highlight options: styles such as underline and box, your own colour labels, and highlighting
  by tag;
- notes in a side pane, with tags and filters;
- tabs and panes, visual filters, lemmas and morphology, passage tools and the word panel (T27);
- saved lists (T14).

Read mode adds nothing on screen beyond the text and a small, hideable reading-settings control.
Note markers stay subtle and open in a popover.

Modes are layers around the renderer, not separate copies. A feature added to the renderer works in
both modes.

## Design sources

The user likes how BookFusion and Zotero present their readers. Take ideas and workflows only:
- Zotero is AGPL, so copy no code;
- describe the result in Xegesis's own terms;
- do not reuse their screen names, layouts or wording (AGENTS.md).

Calibre's viewer and KOReader are further open references.

## Done when

- One renderer in `frontend/src/document/` draws Bibles, reflowable books and PDFs in both modes, and
  the old separate reader code is removed.
- Highlights and notes work in both modes, with the extra highlight options only in study mode.
- The mode switch keeps your place in a Bible or book.
- Read mode shows only reading settings, and they persist per device.
- Study mode has tabs and panes, at least two visual filters, lemma display for tagged Bibles, and tags.
- The right-click menu works on Bibles, EPUB, PDF and other books, and every item in it is a command.
- It works with the keyboard alone and at narrow window sizes.
