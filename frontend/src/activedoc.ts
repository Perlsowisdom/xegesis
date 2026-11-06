// The note or sermon open now, so the assistant can send it as context and insert a draft into it.
// Note and sermon pages register themselves; the one last typed in (or else any one on screen) is
// the open one.

export interface OpenDoc {
  kind: "note" | "sermon";
  el: HTMLElement;
  id(): string | null;
  title(): string;
  insert(md: string): void;
  text(): string; // its text as the editor holds it
}

const docs = new Set<OpenDoc>();
let last: OpenDoc | null = null;

export function registerDoc(d: OpenDoc): () => void {
  docs.add(d);
  const mark = () => (last = d);
  d.el.addEventListener("focusin", mark);
  return () => {
    docs.delete(d);
    d.el.removeEventListener("focusin", mark);
    if (last === d) last = null;
  };
}

const shown = (d: OpenDoc) => d.el.isConnected && d.el.offsetParent !== null && !d.el.closest("#rightbar") && !!d.id();

export function activeDoc(): OpenDoc | null {
  if (last && shown(last)) return last;
  for (const d of docs) if (shown(d)) return d;
  return null;
}
