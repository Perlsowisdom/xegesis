// The installed Bibles, one list for the whole interface: the Reader's panes, Compare, Search,
// the Library browser and sermons all read it here. Installing or removing a Bible (views/bibles.ts),
// a library change, or coming back to the window refreshes it, and every view that listens redraws.
// A failed or empty answer is never kept, so a pane that opened before a Bible was installed, or
// while the core was busy, asks again instead of saying "no Bibles" for the rest of the session.

import { Module, run } from "./api";

type Listener = (list: Module[]) => void;
const listeners = new Set<Listener>();
let current: Module[] | null = null; // kept only when not empty
let known: Module[] | null = null; // the last answer, to tell whether it changed
let pending: Promise<Module[]> | null = null;

function fetchList(): Promise<Module[]> {
  const p = run<Module[]>("bible.modules")
    .then((list) => {
      const fresh = Array.isArray(list) ? list : [];
      const changed = !known || !sameIds(known, fresh);
      known = fresh;
      current = fresh.length ? fresh : null;
      if (changed) listeners.forEach((l) => l(fresh));
      return fresh;
    })
    .catch(() => [] as Module[])
    .finally(() => {
      if (pending === p) pending = null;
    });
  pending = p;
  return p;
}

function sameIds(a: Module[], b: Module[]): boolean {
  return a.length === b.length && a.every((m, i) => m.id === b[i].id && m.abbreviation === b[i].abbreviation);
}

// installedBibles returns the installed Bibles. A non-empty list is reused; an empty one is asked
// for again, because "none" is the answer most likely to go stale.
export function installedBibles(): Promise<Module[]> {
  if (current) return Promise.resolve(current);
  return pending ?? fetchList();
}

// biblesChanged asks the core again and tells the listeners if the list is different.
export function biblesChanged(): Promise<Module[]> {
  return fetchList();
}

// onBibles calls l whenever the list changes; the returned function stops it.
export function onBibles(l: Listener): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}
