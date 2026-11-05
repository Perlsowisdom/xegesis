# T38: Fixes from the user's testing on 5 November 2026

Brief for agents (Opus), in two parts that run one after the other: **T38a** (Bibles), then **T38b**
(layout and writing). Read these first:
- `AGENTS.md`;
- `docs/ui-requirements.md`;
- `docs/editor-behaviour.md`;
- `frontend/e2e/`, the real-app harness. Every item needs a typed real-app check with a screenshot.

These are the user's words, numbered as they gave them. Notes on our side follow each one.

## T38a: Bibles

### 1. "I cannot install any bibles with a sword module."

- **Cause.** `bible.install` (`internal/bible/commands.go`) unzips a file, but only reads OSIS, USFM and
  USX. The SWORD reader (`internal/bible/sword`) is only wired into `bible.get`'s catalogue download
  (`internal/bible/repos/get.go`, `installSWORD` and `installSWORDDict`).
- **Fix.** When an installed folder or `.zip` holds `mods.d/*.conf`, install it with the SWORD reader.
  - Share one function with `bible.get`; don't copy it.
  - Handle Bibles, commentaries, dictionaries and lexicons, as `bible.get` does.
  - Choose the module from the `.conf` file. If a zip holds several modules, install each one, or let
    the user pick.
- **Licence.** Fill the licence from the `.conf` file, as the catalogue path does.
  - For example: `DistributionLicense=Copyrighted; Permission to distribute granted to CrossWire`, plus
    the quoting terms in `About`.
  - A copyrighted module defaults to a personal copy, which the user can change.
- **Locked modules.** A module with `CipherKey` is locked. Refuse it with a clear message. Do **not**
  add any way to unlock it.
- **Interface.** The install dialog's file picker accepts SWORD `.zip` files and says so.
- **Test fixture.** Use a small generated SWORD module (see `internal/bible/sword/swordtest`), never
  the user's ESV file.

### 3. "Books from my library labelled as bible should be treated as bibles in the rest of the software."

For example, an EPUB of Robert Alter's Hebrew Bible whose type is Bible
(`internal/library/classify.go`). Our reading of "treated as a Bible":
- It shows with the installed Bibles in the Bible chooser, Compare, and the "open in" lists, marked
  as a book.
- Going to a reference (for example "Psalm 84") opens the book at that Bible book and chapter.
  - Find chapters from the book's contents and headings, building a map from reference to place.
  - Store the map so it is built once per file version.
  - Where verse numbers can be found in the text, go to the verse. Where they can't, go to the
    chapter.
- Highlights and notes made in it are linked to the reference, so they show up in passage reports and
  "Notes on this passage".
- Quoting from it follows its licence. It is usually a personal copy.
- Changing the book's type away from Bible removes it from these lists.

Write down what could not be mapped in `docs/tasks/followups.md`. Use a generated EPUB fixture with
Bible book and chapter headings.

## T38b: Layout and writing

### 2. "I'm unable to have two tabs next to each other above a very wide tab."

The Study grid can stack panes vertically, but can't split one row into side-by-side panes while
another row stays one wide pane.
- Make the layout a tree of splits:
  - dropping a tab on a pane's left or right edge splits that pane side by side;
  - dropping on the top or bottom edge splits it one above the other.
- Keep the presets, dragging the dividers, and the 3×3 limit (at most 9 panes).
- Saved layouts from before still open.

### 6. "Long titles do weird things to the tab bar."

In the screenshot, a long title runs under the **+** button.
- Tabs have a maximum width, and the title ends in "…", with the full title on hover.
- The close button and **+** never overlap a title.
- When the tabs don't fit, they scroll or shrink, never overlap.

### 7. "I should be able to manually type the book section I want to go to."

"Section 3 of 1070" becomes a field:
- click it and type a number, then press Enter;
- for a PDF, it takes a page number;
- typing words searches the book's contents and offers matching headings.

### 8. "The notes properties take up too much screen space and are not collapsible."

This covers the Passage, Preached, Theme and Kind rows in notes and sermons.
- They fold into one compact line of chips, such as "Psalms 84 · Sun 15 Nov 2026 · Sermon", with an
  arrow to unfold them.
- Remember the choice per device. New documents open unfolded.

### 4. "If I create a note in study mode, it'd be helpful if I could convert it to sermon or bible study or lesson from that note."

- The note's ⋯ menu gets **Turn into → Sermon, Bible study, Lesson**. Use the Kind values that
  sermons already have; add Bible study if it's missing.
- The text, verses, highlights, tags, folder and links are kept.
- It is one command, so it shows in history and can be undone.
- The note's earlier versions stay reachable.
- It opens as the new document in the same pane.

### 5. "The sermon editor should let me select more than one passage as my sermon verse."

The screenshot shows a user typing "Passages: Matthew 14:22-33, Psalm 84" into the body because the
Passage field holds only one.
- The Passage field holds several references as chips, with **+** to add one and × to remove one, in
  order.
- Search, "Sermons on this passage", reports and exports find a sermon by any of its passages.
- Existing single passages keep working.
- If storage needs a migration, use the next free number (check `internal/store/migrations`; T36a
  may have taken one).

## Rules for both parts

- Keep the approved look: colour tokens and both themes.
- Every data change goes through a command.
- Use the harness's temporary copy of the scratch library only, never the user's library or files.
- Run long commands with timeouts, and close every process you start.
- Run `npm ci`; don't junction `node_modules`.
- Don't commit `frontend/dist`.

## Done when

- Each numbered item has a passing real-app check with a screenshot.
- `go test ./...`, `npm test`, `npm run build` and `npm run e2e` pass.
- The report lists each item as pass or fail, and anything left over.
