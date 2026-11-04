# Editor behaviour checklist (approved by the user, 4 November 2026)

The notes and sermon editors must pass each check below **by typing in the real app**. Each check is
"do this → see that". Behaviours marked *(Any)* were seen in the Any desktop app on 4 November 2026.
The others come from the user's requests.

## Markdown as you type

| Do this | See this |
| --- | --- |
| Type `# ` at the start of a line | It becomes a large heading (H1) *(Any)* |
| Type `## ` | It becomes a medium heading (H2) *(Any)* |
| Type `### ` | It becomes a small heading (H3) *(Any)* |
| Type `**word**` | **word** turns bold and the stars disappear *(Any)* |
| Type `*word*` or `_word_` | *word* turns italic *(Any)* |
| Type `` `code` `` | It shows as inline code *(Any)* |
| Type `- ` or `* ` | A bulleted list starts *(Any)* |
| Type `1. ` | A numbered list starts *(Any)* |
| Type `> ` | A quote block starts *(Any)* |
| Type `[] ` | A checkbox item starts *(Any)* |
| Type `---` and press Enter | A divider line appears *(Any)* |
| Press Backspace at the start of a heading or list item | It turns back into plain text |
| Paste Markdown text | It arrives formatted, not as raw symbols |

## Headings and structure

- The toolbar's heading button opens a choice of **Large, Medium and Small**, with Ctrl+Alt+1/2/3.
  The user asked for a way to choose the heading size.
- Headings can be **folded**: an arrow beside each heading hides its section *(Any)*.
- A slim **outline strip** at the right edge shows a mark for each heading. Hovering shows the names,
  and clicking jumps to one *(Any)*.
- In sermons, the left outline column lists the same headings with their levels.

## Inserting Bible text, references and highlights

The user said there was no way to insert a verse or a highlight into a note.

**A single, clear Insert control, with a book-and-quote icon (not a sparkle, which reads as "AI").**
It holds:
- **Verse reference:** a linked chip such as "Romans 8:28" that opens the passage when clicked.
- **Verse text:** the quoted verses in a quote block, with the Bible's credit line, following the
  licence rules.
- **Highlight:** pick from your highlights (searchable, with colours shown). It inserts the
  highlighted words as a quote with their place and a link back.
- **Note link:** a link to another note or sermon.

These also work:
- from the `/` menu (`/verse`, `/quote`, `/highlight`, `/link`);
- by typing a reference such as `Rom 8:28` and pressing Tab, which turns it into a chip;
- by dragging a highlight, a verse or a Related card from a study pane or the sidebar into the note.

The sermon editor has **one** way to insert verses, not two: the duplicate "Quote verses" button
goes.

## Highlights inside verses

- Selecting **part of a verse** highlights exactly the selected words, never the whole verse.
- A highlight can **span several verses** and start or end mid-verse.
- Highlights are stored as exact text ranges (verse ID plus character offsets, with the Bible's
  module), so they survive font and width changes.
- **Existing verse-level highlights are kept** as whole-verse highlights.

## Definition of done

- Every check above passes by **typing in the real Wails app**, not only in a browser preview.
- Each check is recorded with a screenshot or a short note.
