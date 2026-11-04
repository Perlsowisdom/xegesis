// Finding a Bible reference typed just before the caret, so Tab can turn it into a chip. The
// editor checks each candidate with verse.parse, longest first, so "1 John 3:16" is not taken for
// "John 3:16" and words before the book name are skipped.

export interface RefCandidate {
  text: string;
  start: number; // where it starts in the text before the caret
}

// refCandidates gives the possible references ending at the caret: a chapter (with verses, as
// 8:28, 8:28-30 or 8:28-9:2) and the one to four words before it, longest first.
export function refCandidates(before: string): RefCandidate[] {
  const m = /(\d{1,3}(?::\d{1,3}(?:\s?[-–]\s?\d{1,3}(?::\d{1,3})?)?)?)$/.exec(before);
  if (!m) return [];
  const numStart = before.length - m[1].length;
  const head = before.slice(0, numStart);
  if (!/[A-Za-z.]\s+$/.test(head) && !/[A-Za-z.]$/.test(head)) return [];
  // The words before the numbers, nearest last.
  const words = [...head.matchAll(/\S+/g)].map((w) => ({ text: w[0], at: w.index! }));
  const out: RefCandidate[] = [];
  for (let n = Math.min(4, words.length); n >= 1; n--) {
    const first = words[words.length - n];
    const lead = before.slice(first.at, numStart);
    // A book name has letters; it may start with a number (1 John) but has no other punctuation.
    if (!/^[1-3]?\s?[A-Za-z][A-Za-z. ]*\s*$/.test(lead.replace(/^([1-3])\s+/, "$1 "))) continue;
    if (first.at > 0 && !/[\s(]/.test(before[first.at - 1])) continue;
    out.push({ text: before.slice(first.at), start: first.at });
  }
  return out;
}
