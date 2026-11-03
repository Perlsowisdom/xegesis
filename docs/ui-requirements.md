# Interface requirements from the user (3 November 2026)

Feedback on the layout mock-up (https://claude.ai/artifact/7aacZ3hDDcDgnSkhGN4Ak7), for the redesign
brief (T34, widened to the whole interface).

## Left: navigation rail

- Keep the rail.
- Hover tooltips name each icon.
- Settings for the rail:
  - **Width:** condensed (icons) or expanded (icons and labels).
  - **Auto-hide:** on or off. The chosen width persists through hiding.
- A hidden rail comes back with **both** a button and hovering at the left edge.
- Search is a **magnifier icon on the rail** that expands into a search field when clicked.
- Reference for the feel: the Any desktop app's icons and rail (Any is not a competitor; looking at it
  is fine).

## Right: sidebar

- A right sidebar holds the **AI chat** and the study tools.
- Which study tools appear in it, and in what order, is set in **Settings**, tucked away.
- AI can come from any of these, chosen in Settings like the Any app does:
  - **your own API key** for an online service;
  - **a local model** on the computer;
  - **Claude Code or OpenAI Codex**.

  Privacy (ADR 0009): every online option stays off until chosen, and says what it sends.

## Reader

- **Read and Study** switch in the reader toolbar.
- A magnifier next to the switch searches within what is open.
- Read mode reads any library book, not only Bibles.
- **Study mode** shows up to a **3×3 grid** of resources together:
  - each cell is resized freely by dragging its dividers;
  - each cell's tab can change its resource from within the tab, from a menu bar, or by dragging from
    the library.

### From the user's own screenshot (3 November 2026)

The user showed a screenshot of their own desktop setup to show what they want to work. Only these
functional ideas are taken from it, in our own words. The layout, look, icons and wording are not
copied.

- **Each pane has its own reference box:** a resource picker and a passage box, plus Back.
- **Linked panes:** panes in a group follow each other to the same passage. A pane can leave the group.
- **"Related at this verse" panel:** for the current verse, a short excerpt from each commentary or
  study Bible in the library, with a link to open it in full. In Xegesis this is the Passage study tool
  in the right sidebar, not a third column inside the pane.
- **Search inside one resource,** with results grouped by verse.
- **In-pane navigation:** contents, history, and next or previous article.
- **Avoid:** the screenshot puts two rows of toolbar in every pane. That is the clutter the user
  objected to. Xegesis puts one slim row per pane and moves the rest into a ⋯ menu.

### Round 3 feedback (3 November 2026)

- **Drag tabs** between panes. Dropping on a pane's edge splits it, and the grid changes to fit.
  Keep the preset grids as well.
- **Drop the Open… button.** Each pane gets its own + for a new tab.
- **Everything except Read mode can sit in the grid:** passages, books, notes, sermons, lists, and
  the library itself. For example, a note beside a sermon beside a commentary.
- **Settings at the bottom of the rail.**
- **Library and My work need real sample content** to judge.

### Final round 3 feedback (3 November 2026): approved for building

- **Rail bottom:** drop the general Settings icon. Rail settings → "All settings…" leads there.
- **Grid presets on the rail:** a rail button in Study mode. It's a setting, on by default; off moves
  the presets to the Reader toolbar.
- **My work:**
  - the Folders, Tags and Writing sections collapse;
  - you can create folders;
  - a folder holds notes and sermons together.
- **Phone:** Search is a fourth button in the bottom dock.
- The user said the layout "looks good and can probably be implemented in the app".

### What the Any app does (looked at 3 November 2026)

- **Rail:** auto-hide slides the rail over the content when you reach the left edge, so the layout
  doesn't jump.
- **Bottom of the rail:** three small icons, for Settings, Sidebar settings and Help.
  - The sidebar settings popover holds a Compact or Regular switch, an eye toggle to show or hide
    each rail section, and an Auto-hide switch.
- **Settings:** opens in the left panel as a grouped list (Personal, then Agent), with a back
  arrow. The page is in the middle, and the AI panel stays open on the right.
- **AI model settings:**
  - a Provider row with Change;
  - online providers as chips (Claude, OpenAI, Gemini, OpenRouter), using your key, kept under
    Credentials;
  - "On this computer" chips (Claude Code, Codex) with a Ready status;
  - the model chosen as chips.
- **Right panel:** an "Agent" chat panel, opened from an icon at the top right and collapsible.

## Library and My work (from the earlier round)

- **Library filters:** one Filter button that opens all the facets, with several choices allowed
  together (Bible and Commentary, plus a tradition). Only active filters show, as removable chips.
- **Notes and lists:** categorised with notebooks or folders and tags.
- **Sermons:** each row shows the passage, the date preached and the theme or topic.
- **The bin** is visible on every My work tab.
- **Reading plans** belong to reading: today's reading lives in the Reader.

## Not to be used as a design source

- The Logos web app, including its public, signed-out version. The clean-room rule (AGENTS.md,
  ADR 0007) keeps Logos's screens out of the design. The user's own descriptions of what they like are
  fine.
