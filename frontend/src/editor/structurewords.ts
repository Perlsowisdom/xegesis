// The sums behind folding and outlines, kept apart from the DOM so they are tested on their own
// (structurewords.test.ts).

// foldId names a heading for remembering its fold: its level, words and how many headings with
// the same level and words come before it.
export function foldId(level: number, text: string, nth: number): string {
  return `${level}|${text}|${nth}`;
}

// foldedRange says which blocks a folded heading hides: everything after it up to the next heading
// of the same or a higher level (a smaller number). levels has 0 for a block that is not a heading.
export function foldedRange(levels: number[], folded: boolean[]): boolean[] {
  const hidden = levels.map(() => false);
  for (let i = 0; i < levels.length; i++) {
    if (!levels[i] || !folded[i]) continue;
    for (let j = i + 1; j < levels.length && !(levels[j] && levels[j] <= levels[i]); j++) hidden[j] = true;
  }
  return hidden;
}

export interface OutlineMark {
  depth: number; // 0 for the outline's top level
  label: string; // 1, 2… then a, b… then i, ii…
}

const ROMAN = ["i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix", "x"];

// outlineMarks numbers headings as an outline. Depth counts the distinct heading levels above
// each one, so a text with only medium and small headings still starts at the top level.
export function outlineMarks(levels: number[]): OutlineMark[] {
  const used = [...new Set(levels)].sort((a, b) => a - b);
  const counts = [0, 0, 0];
  return levels.map((lv) => {
    const depth = Math.min(used.indexOf(lv), 2);
    counts[depth]++;
    for (let d = depth + 1; d < counts.length; d++) counts[d] = 0;
    const n = counts[depth];
    const label = depth === 0 ? String(n) : depth === 1 ? String.fromCharCode(96 + Math.min(n, 26)) : ROMAN[n - 1] ?? String(n);
    return { depth, label };
  });
}

// carryFolds keeps a folded heading folded when its words change: prev and next map each heading's
// node key to its foldId before and after an edit. It changes folded in place and says whether it did.
export function carryFolds(prev: Map<string, string>, next: Map<string, string>, folded: Set<string>): boolean {
  const still = new Set(next.values());
  const moved: [string, string][] = [];
  for (const [k, id] of next) {
    const was = prev.get(k);
    if (was && was !== id && folded.has(was) && !still.has(was)) moved.push([was, id]);
  }
  for (const [was, id] of moved) {
    folded.delete(was);
    folded.add(id);
  }
  return moved.length > 0;
}
