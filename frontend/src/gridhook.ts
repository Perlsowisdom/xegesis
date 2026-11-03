// The hook for the study grid's presets (T34b).
//
// The grid presets button appears in Study mode: on the rail when "Grid presets on the rail" is on
// (the default), otherwise in the Reader's top bar. Both call openGridPresets with the button that
// was pressed. Until the grid exists, the default handler shows a short popover saying so.
//
// T34b replaces it once, at start-up:
//
//   import { setGridPresets } from "./gridhook";
//   setGridPresets((anchor) => openPopover(anchor, presetPicker(), { label: "Grid presets", side: "right" }));
//
// The same request is also sent as a "xegesis:grid-presets" event on window, with the anchor as its
// detail, for code that would rather listen than replace the handler.

import { h } from "./dom";
import { togglePopover } from "./popover";

export type GridPresetsHandler = (anchor: HTMLElement) => void;

const placeholder: GridPresetsHandler = (anchor) => {
  togglePopover(anchor, () => h("div", { class: "pop-body" },
    h("h2", { class: "pop-title" }, "Grid presets"),
    h("p", { class: "hint" }, "Study mode will soon show up to nine resources side by side. Preset grids will be chosen here.")),
  { label: "Grid presets", side: anchor.closest(".rail") ? "right" : "below" });
};

let handler: GridPresetsHandler = placeholder;

export function setGridPresets(fn: GridPresetsHandler): void {
  handler = fn;
}

export function openGridPresets(anchor: HTMLElement): void {
  window.dispatchEvent(new CustomEvent("xegesis:grid-presets", { detail: anchor }));
  handler(anchor);
}
