# Follow-ups found while checking screens

Small issues to fold into the next task that touches each area, or a tidy-up pass.

## 29 October 2026 (T14, T16 on screen)

- Reader: the "Notes on this passage" form sits below long commentary in the side column; the Save
  button can fall below the window (Ctrl+Enter still saves). T34's shared layout should give notes
  their own pane.
- Notes: after Restore in Recently deleted, the view stays on the empty deleted list; go back to the
  notes list instead.
- Notes: four filter menus are always shown; fold them under More filters, as in the Library.
- Plans: the green "Started … finishing …" message keeps the original finish date after days are
  marked; refresh or clear it.
- Confirm dialogs say "wails.localhost says"; replace them with the app's own dialogs.
- Abbott-Smith's Strong's index should install its companion lexicon automatically.
- Tagged words cannot be reached with the keyboard.
- Migrations are tracked by one version number, so an out-of-order merge could skip one; record each
  applied migration instead.

## From the T34a/T34b redesign (3 November 2026)

- Check Study mode in the real app: the grid, drag to split, presets, resizing, Move to…, and the
  right-click menu. It was only tested in a browser with stub data, and the screenshot option can't
  reach it.
- Check the rail popovers, auto-hide and the right sidebar in the real app.
- Sermons have no "date preached" or "theme" fields yet. They need a Go schema change (T17 or T34c).
  The user named passage, date preached and theme as the three most important things.
- The Reader shows only the verse in the passage box (Romans 8:28), not the chapter around it.
  Decide whether a passage should show the whole chapter scrolled to the verse.
- Narrow panes (under 440px) are meant to wrap their tabs; this hasn't been confirmed on screen.
- Next stages: T34c (Library and My work: Filter popover, folders holding notes and sermons,
  collapsible sections, bin) and T36 (the AI assistant).

## User testing of T34a/T34b (3 November 2026): fixed 3 November (branch m2/t34-fixes)

All five fixed. Left: some secondary forms (export options, plan weekdays, licence choices, Move to…)
still use native selects or tick-box lists rather than the new controls.

1. **The study grid doesn't fill the window.** Three panes are squeezed into about 300px on the left,
   with the header controls wrapping, text one word per line, and horizontal scrollbars in each pane.
   The grid container (or its columns) isn't growing to the main area's width and height.
2. **Installed Bibles don't open in the Reader or Study panes.** Panes say "No Bibles are installed
   yet. Get one in Library → Get Bibles", but the installed Bible does show in the sidebar's Compare
   tool. The pane Bible list must use the same source Compare uses.
3. **Controls look unfinished.** The user wants the toggles, switches, segmented controls and chips
   from the approved mock-up instead of plain checkboxes. Their words: it makes the app "feel like a
   linux opensource weekend project". Apply them across Settings, Rail settings, the sidebar tool
   list and the filters.
4. **Light mode.** Only the dark theme shows. Add Light and Dark plus "Match the system" in Settings →
   Appearance, using the colour tokens.
5. The pane header is too tall in narrow panes: the tabs, place box, Bibles picker, link and ⋯ are
   stacked on four rows. Keep it to one row, with overflow in ⋯.

The user liked the Library ("looks good so far").

## User testing after T34c (3 November 2026)

Design first: mock-up for approval, then build.

- **Right sidebar:** remove its × button. The top-bar sidebar toggle is enough.
- **Pane ⋯ menu:** its items feel out of place, because everything is in one long list. "Unlink"
  duplicates the link button.
- **Sermon editor and note editor:** both look "terrible": a form of labelled boxes and a toolbar of
  big text buttons. They need a document-style design.
- **Highlights:** unintuitive, with no way to see or delete them. Needed: a clear selection toolbar,
  clicking a highlight to change or remove it, and a list of highlights.
- **Word look-up:** "over-aggressive", because words are marked as clickable throughout the text.
  Words should look like plain text until you ask to look one up.
- **Bug: the auto-hide "show rail" button sits on top of page headers.** It covers "My work". Content
  must never be overlapped: the user reported "the sidebar cuts off part of the text UI".

## After T34d (4 November 2026)

- HTML and Word exports still show `---` dividers as text instead of a rule.
- T34d was checked in a browser preview with stand-in data. The real app was only seen on its start screen.

## User testing of T34d (4 November 2026): editors still miss the mark

- Markdown shortcuts do not work in the note editor (typing ## , **, - , > should format, as in Any or Obsidian).
- Highlights cover whole verses only. They must cover any part of a verse, down to words and characters.
- There is no way to insert a verse or a highlight into a note.
- The sermon editor has no way to choose the heading size (H1, H2, H3).
- The sermon toolbar duplicates verse insertion (the Verse reference button and the Quote verses button), and the sparkle icon reads as "AI" rather than "insert reference".
- The user feels the note and sermon editors keep missing the mark despite examples and detailed instructions.
  Next: study Any's editor and the user's examples hands-on first, write behaviour-level acceptance tests, and test typing in the real app before calling it done.

## From T34e (4 November 2026)

- **Real-app interface tests exist** (`frontend/e2e/`, `npm run e2e`): they build the desktop app,
  start it on a copy of the scratch library with a WebView2 debugging port, and type into it with
  real key events through `playwright-core`. This is the start of T30 (interface tests): new screens
  should add a `*.e2e.mjs` file there. Wails clears the `WEBVIEW2_*` variables, so the test build
  (only) uses a patched copy of its WebView2 loader through a temporary `go.work`; see `e2e/app.mjs`.
