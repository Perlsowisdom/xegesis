// Line icons for the shell, drawn for Xegesis on a 24-unit grid. They take the text colour and are
// hidden from screen readers: every icon sits beside a label or inside a named button.

const PATHS: Record<string, string> = {
  search: '<circle cx="10.5" cy="10.5" r="6"/><path d="M15 15l5 5"/>',
  library: '<path d="M4 4h4v16H4zM10 4h4v16h-4z"/><path d="M15.2 5.4l3.6-1 3 15.2-3.6 1z"/>',
  reader: '<path d="M3 5.5C5.5 4.5 9 4.5 12 6.5c3-2 6.5-2 9-1V19c-2.5-1-6-1-9 1-3-2-6.5-2-9-1z"/><path d="M12 6.5V20"/>',
  work: '<path d="M4 4h10l4 4v12H4z"/><path d="M14 4v4h4"/><path d="M7.5 12.5h7M7.5 16h5"/>',
  grid: '<rect x="4" y="4" width="16" height="16" rx="2"/><path d="M4 12h16M12 4v16"/>',
  sliders: '<path d="M4 7h9M17 7h3M4 17h3M11 17h9"/><circle cx="15" cy="7" r="2"/><circle cx="9" cy="17" r="2"/>',
  help: '<circle cx="12" cy="12" r="8.5"/><path d="M9.6 9.5a2.5 2.5 0 1 1 3.4 2.3c-.6.3-1 .8-1 1.5v.5"/><path d="M12 16.8v.2"/>',
  panel: '<rect x="3.5" y="4.5" width="17" height="15" rx="2"/><path d="M14.5 4.5v15"/>',
  more: '<path d="M6 12h.01M12 12h.01M18 12h.01" stroke-width="3"/>',
  prev: '<path d="M14.5 6l-6 6 6 6"/>',
  next: '<path d="M9.5 6l6 6-6 6"/>',
  close: '<path d="M6 6l12 12M18 6L6 18"/>',
  rail: '<rect x="3.5" y="4.5" width="17" height="15" rx="2"/><path d="M9.5 4.5v15"/>',
  up: '<path d="M6 14.5l6-6 6 6"/>',
  down: '<path d="M6 9.5l6 6 6-6"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  link: '<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1"/><path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>',
  folder: '<path d="M3.5 7.5V18a1.5 1.5 0 0 0 1.5 1.5h14a1.5 1.5 0 0 0 1.5-1.5V9.5A1.5 1.5 0 0 0 19 8h-7l-2-2.5H5A1.5 1.5 0 0 0 3.5 7z"/>',
  notes: '<rect x="4.5" y="3.5" width="15" height="17" rx="2"/><path d="M8 8h8M8 12h8M8 16h5"/>',
  tag: '<path d="M3.5 12.2V4.5a1 1 0 0 1 1-1h7.7l8.3 8.3a1.4 1.4 0 0 1 0 2l-6.7 6.7a1.4 1.4 0 0 1-2 0z"/><circle cx="8" cy="8" r="1.4"/>',
  bin: '<path d="M4.5 7h15M9.5 7V4.5h5V7"/><path d="M6.5 7l1 12.5h9l1-12.5"/><path d="M10 11v5M14 11v5"/>',
  sermon: '<rect x="9" y="3.5" width="6" height="9" rx="3"/><path d="M6.5 11a5.5 5.5 0 0 0 11 0M12 16.5v4M8.5 20.5h7"/>',
  list: '<path d="M9 6.5h11M9 12h11M9 17.5h11"/><path d="M4.5 6.5h.01M4.5 12h.01M4.5 17.5h.01" stroke-width="2.6"/>',
  plan: '<rect x="3.5" y="5" width="17" height="15" rx="2"/><path d="M3.5 9.5h17M8 3v4M16 3v4"/><path d="M8.5 14.5l2 2 4-4"/>',
  report: '<path d="M5 20.5V10M10 20.5V4M15 20.5v-7M20 20.5H3"/>',
  filter: '<path d="M4 5.5h16l-6 7.5v5.5l-4 1.5v-7z"/>',
  covers: '<rect x="4" y="4" width="6.5" height="7" rx="1"/><rect x="13.5" y="4" width="6.5" height="7" rx="1"/><rect x="4" y="13.5" width="6.5" height="7" rx="1"/><rect x="13.5" y="13.5" width="6.5" height="7" rx="1"/>',
  rows: '<path d="M4 6h16M4 12h16M4 18h16"/>',
  bullets: '<path d="M10 6.5h10M10 12h10M10 17.5h10"/><circle cx="5" cy="6.5" r="1.3" fill="currentColor" stroke="none"/><circle cx="5" cy="12" r="1.3" fill="currentColor" stroke="none"/><circle cx="5" cy="17.5" r="1.3" fill="currentColor" stroke="none"/>',
  numbers: '<path d="M10 6.5h10M10 12h10M10 17.5h10"/><path d="M4 4.8l1.4-.8v5M3.6 10.4c.3-.6 1-1 1.7-.8.9.3 1 1.2.4 1.9L3.6 14h2.6M3.7 16.2c.4-.4 1-.6 1.6-.4.8.3.8 1.3 0 1.6.9.2 1 1.4.1 1.7-.6.2-1.3 0-1.7-.4" stroke-width="1.3"/>',
  quote: '<path d="M5 17.5c0-4 1-7.5 4.5-9.5M5 17.5h4v-4H5zM14 17.5c0-4 1-7.5 4.5-9.5M14 17.5h4v-4h-4z"/>',
  chevron: '<path d="M7 10l5 5 5-5"/>',
  calendar: '<rect x="3.5" y="5" width="17" height="15" rx="2"/><path d="M3.5 9.5h17M8 3v4M16 3v4"/>',
  copy: '<rect x="8.5" y="8.5" width="11" height="11" rx="2"/><path d="M15.5 8.5V6a1.5 1.5 0 0 0-1.5-1.5H6A1.5 1.5 0 0 0 4.5 6v8A1.5 1.5 0 0 0 6 15.5h2.5"/>',
  pen: '<path d="M4 20l1-4.5L15.5 5a2 2 0 0 1 3 3L8 18.5z"/><path d="M13.5 7l3 3"/>',
  // A closed book with a plus on its cover: the editor's Insert control.
  "book-plus": '<path d="M5 4.5A1.5 1.5 0 0 1 6.5 3H19v15H6.5A1.5 1.5 0 0 0 5 19.5z"/><path d="M5 19.5A1.5 1.5 0 0 0 6.5 21H19v-3"/><path d="M12 7.5v6M9 10.5h6"/>',
  highlight: '<path d="M14.5 4.5l5 5-8.5 8.5H6v-5z"/><path d="M4 20.5h16"/>',
  contents: '<path d="M4 6h16M4 12h16M4 18h10"/>',
  drag: '<path d="M9 6h.01M15 6h.01M9 12h.01M15 12h.01M9 18h.01M15 18h.01" stroke-width="2.6"/>',
  unlink:'<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1"/><path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/><path d="M4 4l16 16"/>',
};

export function icon(name: keyof typeof PATHS | string): SVGSVGElement {
  const NS = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(NS, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("class", "icon");
  svg.setAttribute("aria-hidden", "true");
  svg.setAttribute("focusable", "false");
  svg.innerHTML = PATHS[name] ?? "";
  return svg;
}
