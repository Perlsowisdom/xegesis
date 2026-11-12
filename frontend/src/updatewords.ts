// The words for updates: what the settings page says about the last check and the newest version.
// Kept apart from the page so they can be tested.

export interface UpdateInfo {
  enabled: boolean;
  asked: boolean;
  last_checked?: string;
  latest?: string;
  current: string;
  key_set: boolean;
}

// lastCheckedText says when the last check was, in plain words.
export function lastCheckedText(iso: string | undefined, now: Date = new Date()): string {
  if (!iso) return "Not checked yet.";
  const t = new Date(iso);
  if (Number.isNaN(t.getTime())) return "Not checked yet.";
  const mins = Math.max(0, Math.round((now.getTime() - t.getTime()) / 60000));
  if (mins < 1) return "Last checked just now.";
  if (mins < 60) return `Last checked ${mins} minute${mins === 1 ? "" : "s"} ago.`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `Last checked ${hours} hour${hours === 1 ? "" : "s"} ago.`;
  return `Last checked on ${t.toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })}.`;
}

// versionLine says which version is running and whether a newer one is known.
export function versionLine(info: UpdateInfo, newer: boolean): string {
  if (newer && info.latest) return `Version ${info.latest} is available. You have ${info.current}.`;
  return `You have version ${info.current}.`;
}
