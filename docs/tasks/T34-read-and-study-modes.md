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

## Design sources

The user likes how BookFusion and Zotero present their readers. Take ideas and workflows only:
- Zotero is AGPL, so copy no code;
- describe the result in Xegesis's own terms;
- do not reuse their screen names, layouts or wording (AGENTS.md).

Calibre's viewer and KOReader are further open references.

## Done when

- The mode switch keeps your place in a Bible or book.
- Read mode shows only reading settings, and they persist per device.
- Study mode has tabs and panes, at least two visual filters, lemma display for tagged Bibles, and tags.
- The right-click menu works on Bibles, EPUB, PDF and other books, and every item in it is a command.
- It works with the keyboard alone and at narrow window sizes.
