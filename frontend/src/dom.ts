// A tiny element builder, so views stay plain TypeScript without a framework.

type Child = Node | string | number | null | undefined | false | Child[];
type Attrs = Record<string, unknown>;

export function h<K extends keyof HTMLElementTagNameMap>(tag: K, attrs: Attrs = {}, ...children: Child[]): HTMLElementTagNameMap[K] {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === null || v === false) continue;
    if (k.startsWith("on") && typeof v === "function") {
      el.addEventListener(k.slice(2).toLowerCase(), v as EventListener);
    } else if (k === "class") {
      el.className = String(v);
    } else if (k === "value" && "value" in el) {
      (el as HTMLInputElement).value = String(v);
    } else if (k === "checked" || k === "disabled" || k === "selected") {
      (el as unknown as Record<string, boolean>)[k] = Boolean(v);
    } else {
      el.setAttribute(k, v === true ? "" : String(v));
    }
  }
  append(el, children);
  return el;
}

function append(el: Node, children: Child[]) {
  for (const c of children) {
    if (c === null || c === undefined || c === false) continue;
    if (Array.isArray(c)) append(el, c);
    else el.appendChild(typeof c === "object" ? c : document.createTextNode(String(c)));
  }
}

export function clear(el: Element, ...children: Child[]) {
  el.replaceChildren();
  append(el, children);
}

let uid = 0;
export function id(prefix: string): string {
  return `${prefix}-${++uid}`;
}

// field wraps a control with a visible label.
export function field(label: string, control: HTMLElement, hint?: string): HTMLElement {
  const cid = control.id || id("f");
  control.id = cid;
  const hid = hint ? cid + "-hint" : undefined;
  if (hid) control.setAttribute("aria-describedby", hid);
  return h("div", { class: "field" },
    h("label", { for: cid }, label),
    control,
    hint ? h("small", { id: hid, class: "hint" }, hint) : null);
}

export function select(options: (string | [string, string])[], value = "", attrs: Attrs = {}): HTMLSelectElement {
  return h("select", attrs, options.map((o) => {
    const [v, label] = Array.isArray(o) ? o : [o, o];
    return h("option", { value: v, selected: v === value }, label);
  }));
}

export function button(label: string, onclick: (e: Event) => void, attrs: Attrs = {}): HTMLButtonElement {
  return h("button", { type: "button", ...attrs, onclick }, label);
}

export function badge(text: string, kind = ""): HTMLElement {
  return h("span", { class: `badge ${kind}` }, text);
}
