// References and links in a book: Bible references open in the linked Bible, other links move in the book.

import { Ref, run } from "../api";
import { h } from "../dom";
import type { BookCtx } from "./bookctx";
import { go, jump } from "./bookpaging";

// linkPdfRefs turns Bible references in a PDF page's text layer into the same links EPUB chapters
// get, so clicking one opens it in the Bible pane. A reference split across lines stays plain.
export async function linkPdfRefs(pageEl: HTMLElement) {
  const spans = [...pageEl.querySelectorAll<HTMLElement>(".textLayer span")]
    .filter((sp) => sp.childElementCount === 0 && /\d/.test(sp.textContent ?? ""));
  if (spans.length === 0) return;
  const found = await run<{ start: number; end: number; ref: string }[][]>("book.refs",
    { texts: spans.map((sp) => sp.textContent ?? "") }).catch(() => null);
  if (!found || !pageEl.isConnected) return;
  spans.forEach((sp, i) => {
    const refs = found[i];
    if (!refs?.length) return;
    const t = sp.textContent ?? "";
    const parts: (string | Node)[] = [];
    let pos = 0;
    for (const f of refs) {
      parts.push(t.slice(pos, f.start), h("a", { href: "#", class: "vref", "data-ref": f.ref }, t.slice(f.start, f.end)));
      pos = f.end;
    }
    parts.push(t.slice(pos));
    sp.replaceChildren(...parts);
  });
}

// wireLinks makes links in the book work: Bible references, jumps inside it, and Alt+Page keys.
export function wireLinks(c: BookCtx) {
  c.body.addEventListener("click", async (e) => {
    const a = (e.target as HTMLElement).closest("a");
    if (!a || !c.body.contains(a)) return;
    e.preventDefault();
    if (a.classList.contains("vref") && a.dataset.ref) {
      const p = await c.msg.attempt(() => run<Ref>("verse.parse", { ref: a.dataset.ref }));
      if (p) c.host.go(p.display);
    } else if (a.dataset.section) {
      jump(c, Number(a.dataset.section), { anchor: a.dataset.anchor, flash: !!a.dataset.anchor });
    } else if (a.dataset.url) {
      c.msg.info(`This link goes to ${a.dataset.url}. Xegesis does not open web pages from books.`);
    }
  });
  c.el.addEventListener("keydown", (e) => {
    if (!e.altKey || (e.key !== "PageDown" && e.key !== "PageUp")) return;
    e.preventDefault();
    go(c, c.section + (e.key === "PageDown" ? 1 : -1));
  });
}
