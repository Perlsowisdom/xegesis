# T34a: New interface shell (rail, right sidebar, settings)

Brief for one agent (Opus). This is the first stage of the interface redesign: T34, widened to the
whole interface. Read these first:
- `AGENTS.md`;
- `docs/ui-requirements.md`, the user's requirements, approved on 3 November 2026;
- `docs/ui-review-2026-11-02.md`, which lists what is cluttered today.

The approved mock-up is the "Desktop, round 3" board at
https://claude.ai/artifact/7aacZ3hDDcDgnSkhGN4Ak7. You may not be able to open it, so this brief and
`docs/ui-requirements.md` describe everything you need.

Design philosophy: "Simple things should be simple, complex things should be possible."

## Scope

Replace the shell in `frontend/src/main.ts`. Today it is a static list of 11 sections in a sidebar.
The new shell has a left rail, a main area and a right sidebar. Keep every existing feature
reachable. Move features; don't delete them.

### Left rail

**Places on the rail, top to bottom:**
- **Search:** a magnifier. It opens a search flyout beside the rail: one field and recent searches.
  Results use the existing search view and commands (`views/search.ts`).
- **Library:** the existing library. Get Bibles (`views/bibles.ts`) becomes a button or tab inside
  the Library, not its own place.
- **Reader:** read and study.
- **My work:** notes, sermons, lists and reports as tabs or sub-pages of one place. The existing views
  are reused for now; T34c redesigns them.
- **Grid presets:** a grid button, shown only in Study mode and only when the setting
  "Grid presets on the rail" is on, which is the default. Until T34b builds the grid, it can open an
  empty popover that says the grid is coming. Better still, leave a clear hook in the code.

**Bottom of the rail:**
- **Rail settings** (a sliders icon) opens a popover with:
  - Condensed or Expanded;
  - show or hide Search, Library and My work;
  - Hide automatically (on or off);
  - Grid presets on the rail (on or off);
  - "All settings…", which opens Settings.
- **Help**, linking to the commands and shortcuts list (today's Commands view) and app details.
- There is **no** separate Settings icon. Settings is reached from Rail settings → All settings…, or
  from Ctrl+, if you add it.

**Width:**
- Condensed shows icons only, with a tooltip naming each on hover and on keyboard focus.
- Expanded shows icons and labels.

**Auto-hide:**
- When on, the rail slides away. It comes back when the pointer reaches the left window edge, and as
  an overlay over the page, so the layout doesn't jump. It hides again when the pointer leaves.
- A small "show rail" button at the top left of the page also brings it back.
- It keeps the chosen width.
- From the keyboard, a shortcut such as Alt+0 or F6 must reach the rail.

**Persisting:** the width, auto-hide, the hidden places and grid-on-rail persist per device, in
`localStorage` through `state.ts`'s helpers.

### Reader top bar

The top bar is slim and holds:
- the **Read | Study** switch;
- a **magnifier beside it** that expands into "search in what is open";
- in Read mode, **Aa** for reading settings;
- at the right, the toggle for the **right sidebar**.

Today's toolbar items that don't fit go into a ⋯ menu, such as Print.

### Right sidebar

- A collapsible panel on the right with tool tabs: **AI chat, Related, Notes, Word, Compare**.
- Settings → Right sidebar lets you choose which tools show (tick boxes) and their order (up and
  down). It persists per device.
- **Related, Notes, Word and Compare:** move the existing study panel contents (`views/study.ts`)
  into these tabs. Related is commentary excerpts at the current verse plus cross references, where
  the data exists. Otherwise say plainly that there is nothing yet.
- **AI chat** is a placeholder only. It says the assistant is off and shows the three ways to turn it
  on: your own key, Claude Code or Codex on this computer, or a local model. It links to
  Settings → Assistant → Model. **No network calls.** The assistant itself is a later task (T36).

### Settings

Settings has grouped sections down its left side:
- **General:**
  - **Appearance:** the rail settings again, and reading defaults;
  - **Right sidebar:**
  - **Bibles online:** today's API.Bible key form;
  - **Privacy:**
  - **Your data:** today's data section;
- **Assistant:**
  - **Model:** a placeholder page laid out like the mock-up. It has a Provider row with Change,
    chips for online services (Claude, OpenAI, Gemini, OpenRouter) and for this computer
    (Claude Code, Codex, Local model), and Off. Choosing only saves the preference locally, and
    nothing connects. It says so.
- **Advanced:** today's Commands view.

### Phone width

At narrow widths (under about 700px):
- the rail becomes a **bottom dock** with Search, Library, Reader and My work;
- the right sidebar becomes a sheet.

Keep it simple; T31 does the real phone app.

## Rules

- Every action still calls a registered command (`api.ts`). The shell only switches views.
- Accessibility: landmarks, `aria-current`, focus moves into a place when it opens, Escape closes
  popovers, everything works by keyboard, and tooltips also show on focus. Keep the Alt+number
  shortcuts working for the new places.
- Colours come from tokens in `style.css`. Keep the existing theme and the dark mode.
- Don't commit `frontend/dist`; the coordinator rebuilds it.
- Plain words in the interface (AGENTS.md). Don't copy the Logos layout or wording. Any is a fine
  reference: it isn't a competitor.
- Unit-test pure logic, such as settings defaults and tool ordering, with vitest, as the existing
  `*words.ts` files do.

## Done when

1. The rail shows Search, Library, Reader and My work at the top, and Rail settings and Help at the
   bottom, with no separate Settings icon.
2. Condensed, Expanded and auto-hide all work as described and persist after a restart. A hidden rail
   returns both by edge hover and by the button.
3. Every former section is reachable: Bibles, Library, Notes, Lists, Plans, Sermons, Reports, Search,
   Settings and Commands. Plans or today's reading shows in the Reader.
4. The right sidebar shows the chosen tools in the chosen order, and Settings changes them.
5. Settings has the grouped sections above, and the Assistant → Model placeholder makes no network
   calls.
6. The Reader has the slim top bar with Read | Study, the search magnifier and the sidebar toggle.
7. The phone width shows the bottom dock.
8. `npm run build` and `npm test` pass in `frontend`, and `go test ./...` passes.
