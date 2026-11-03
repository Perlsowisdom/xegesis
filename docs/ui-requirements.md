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
