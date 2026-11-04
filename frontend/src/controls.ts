// The interface's own controls: a switch for on/off, segmented pills for two to four choices,
// chips for filters and Bible pickers, and tick boxes for lists. Each is a native input underneath,
// so the role (switch, radio in a radiogroup, checkbox), the keyboard (Space, arrow keys) and forms
// work as people expect; style.css draws them (see "Controls" there). Views that read their state
// from the page can keep using input:checked.

import { h, id } from "./dom";

type Child = Node | string | null;

// settingRow lays out one setting: its name and a line of explanation on the left, the control on
// the right. labelFor ties the name to an input so clicking it works the control.
export function settingRow(label: string, hint: string | undefined, control: HTMLElement, labelFor?: string): HTMLElement {
  const lid = id("setting");
  const hid = hint ? lid + "-hint" : undefined;
  const name = labelFor
    ? h("label", { class: "setting-label", id: lid, for: labelFor }, label)
    : h("span", { class: "setting-label", id: lid }, label);
  const target = control.matches("input, [role=radiogroup]") ? control : control.querySelector("input, [role=radiogroup]");
  if (target && !labelFor) target.setAttribute("aria-labelledby", lid);
  if (target && hid) target.setAttribute("aria-describedby", hid);
  return h("div", { class: "setting" },
    h("div", { class: "setting-text" }, name, hint ? h("small", { class: "hint", id: hid }, hint) : null),
    h("div", { class: "setting-control" }, control));
}

export interface SwitchOptions {
  label: string;
  checked: boolean;
  onChange(on: boolean): void;
  hint?: string;
}

// switchInput is the bare switch, for places that give it their own label.
export function switchInput(checked: boolean, onChange: (on: boolean) => void, attrs: Record<string, unknown> = {}): HTMLInputElement {
  const input = h("input", { type: "checkbox", role: "switch", class: "switch", checked, ...attrs });
  input.addEventListener("change", () => {
    input.setAttribute("aria-checked", String(input.checked));
    onChange(input.checked);
  });
  input.setAttribute("aria-checked", String(checked));
  return input;
}

// toggleSwitch is a setting row with a switch: on the right, with its name and hint on the left.
export function toggleSwitch(o: SwitchOptions): HTMLElement {
  const sid = id("switch");
  return settingRow(o.label, o.hint, switchInput(o.checked, o.onChange, { id: sid }), sid);
}

export interface SegmentedOptions<V extends string> {
  label: string; // the group's accessible name
  options: [V, string][];
  value: V;
  onChange(v: V): void;
  small?: boolean;
}

// segmented is a row of pills, one of which is chosen: a radiogroup of native radios.
export function segmented<V extends string>(o: SegmentedOptions<V>): HTMLElement {
  const name = id("seg");
  return h("div", { class: `seg${o.small ? " small" : ""}`, role: "radiogroup", "aria-label": o.label },
    o.options.map(([v, label]) => {
      const input = h("input", { type: "radio", class: "seg-input", name, value: v, checked: v === o.value });
      input.addEventListener("change", () => { if (input.checked) o.onChange(v); });
      return h("label", { class: "seg-opt" }, input, h("span", {}, label));
    }));
}

// segmentedValue reads a segmented control's current choice.
export function segmentedValue(el: HTMLElement): string {
  return el.querySelector<HTMLInputElement>("input:checked")?.value ?? "";
}

// setSegmented chooses v without telling the listener.
export function setSegmented(el: HTMLElement, v: string): void {
  for (const r of el.querySelectorAll<HTMLInputElement>("input")) r.checked = r.value === v;
}

export interface ChipOptions {
  label: string;
  checked: boolean;
  onChange?(on: boolean): void;
  value?: string;
  title?: string;
}

// chip is a rounded toggle for filters and pickers, ticked when on.
export function chip(o: ChipOptions): HTMLLabelElement {
  const input = h("input", { type: "checkbox", class: "chip-input", value: o.value ?? "", checked: o.checked });
  if (o.onChange) input.addEventListener("change", () => o.onChange!(input.checked));
  return h("label", { class: "chip-toggle", title: o.title }, input, h("span", { class: "chip-mark", "aria-hidden": "true" }), h("span", {}, o.label));
}

export interface TickOptions {
  label: string | Child[];
  checked: boolean;
  onChange?(on: boolean): void;
  value?: string;
  hint?: string;
  attrs?: Record<string, unknown>;
}

// tick is a tick box with its label, for choosing several items from a list.
export function tick(o: TickOptions): HTMLLabelElement {
  const input = h("input", { type: "checkbox", class: "tick-input", value: o.value ?? "", checked: o.checked, ...(o.attrs ?? {}) });
  if (o.onChange) input.addEventListener("change", () => o.onChange!(input.checked));
  return h("label", { class: "tick" }, input,
    h("span", { class: "tick-text" }, h("span", {}, o.label), o.hint ? h("small", { class: "hint" }, o.hint) : null));
}
