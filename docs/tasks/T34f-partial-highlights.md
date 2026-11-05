# T34f: Highlights on any part of the text

Brief for one agent (Opus), after T34g. Read these first:
- `AGENTS.md`;
- `docs/editor-behaviour.md`, the section "Highlights inside verses", which the user approved;
- `frontend/e2e/`, the real-app harness. Every item needs a typed real-app test with a screenshot.

**The problem.** Selecting part of a verse highlights the whole verse. The user wants exactly the
selected words.

## Storage

- Today a highlight points at an `anchors` row (`verse_start` and `verse_end`, plus `locator_json`).
- Use `anchors.locator_json` to store the exact range, so **no new migration** should be needed. For
  example:

  ```json
  {"range":{"start":{"verse":45008028,"offset":12},"end":{"verse":45008029,"offset":40}},"module":"KJV","quote":"…"}
  ```

- Offsets count characters in the verse's **plain text**, as the app draws it, without verse numbers
  or notes.
- Keep the module the range was made in. In another Bible, show the highlight on the whole verse or
  verses, faintly, and say so when you hover.
- Keep `verse_start` and `verse_end` filled, so lookups by verse still work.
- **Existing highlights** have no range, and stay whole-verse highlights.
- **Books (EPUB):** use the same range idea with the existing locator (CFI or text position). **PDFs**
  keep page and rectangle locators. The Bible is required; books are a plus if time allows.
- If a migration turns out to be truly needed, it is **0011**.

## Behaviour

- Selecting part of a verse and choosing a colour marks exactly those characters.
- Ranges can span verses, and start or end mid-verse.
- Overlapping a highlight replaces the overlapped part, so two colours never stack.
- Clicking a highlight selects that highlight's whole range for the toolbar: recolour, underline,
  remove.
- The Highlights sidebar tool and My work → Highlights show the **exact quoted words**.
- Inserting a highlight into a note quotes the exact words (T34e's
  `highlightText(row, range?)`).
- Highlights still line up after changing the font size, the window width, and Read or Study mode.

## Commands and tests

- Extend `highlight.create`, `highlight.update` and `highlight.list` with the range. Write Go tests
  for:
  - range validation;
  - overlap splitting;
  - leaving old whole-verse highlights alone.
- Write vitest tests for the range maths: mapping a DOM selection to verse and offset, and back.
- Write e2e tests in the real app for:
  - a partial highlight inside one verse;
  - one spanning two verses;
  - recolouring one;
  - removing one;
  - the sidebar list showing exact words;
  - inserting it into a note;
  - it surviving a font-size change.

## Done when

- Each item has a passing real-app test with a screenshot.
- `go test ./...`, `npm test`, `npm run build` and `npm run e2e` pass.
