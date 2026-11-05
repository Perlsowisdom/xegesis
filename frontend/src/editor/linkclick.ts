// Whether a click on a link in the editor follows the link or places the caret in its text.
// A plain click follows it; Shift or Alt+click, or any click while text is selected, places the caret
// (arrow keys also move into link text). Tested in linkclick.test.ts.
export interface LinkClick {
  button: number;
  shift: boolean;
  alt: boolean;
  hadSelection: boolean; // text was selected when the button went down
  hasSelection: boolean; // text is selected now (a drag across the link)
}

export function followsLink(c: LinkClick): boolean {
  return c.button === 0 && !c.shift && !c.alt && !c.hadSelection && !c.hasSelection;
}
