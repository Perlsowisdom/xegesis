// PDF pages drawn with pdf.js (Apache-2.0). It is loaded only when a PDF is opened. The file's
// bytes come from the core (book.file); pdf.js is never given a URL, and every option that could
// fetch something (fonts, wasm decoders, workers' own fetches) is off, so a PDF loads nothing.

import { GlobalWorkerOptions, TextLayer, getDocument } from "pdfjs-dist";
import type { PDFDocumentProxy, RenderTask } from "pdfjs-dist";
import workerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import type { OutlineItem } from "../bookhelpers";

GlobalWorkerOptions.workerSrc = workerUrl;

export type PdfDoc = PDFDocumentProxy;

export function openPdf(data: Uint8Array): Promise<PdfDoc> {
  return getDocument({
    data,
    isEvalSupported: false,
    useWorkerFetch: false,
    useWasm: false,
    enableXfa: false,
    useSystemFonts: true,
    disableAutoFetch: true,
    disableStream: true,
    stopAtErrors: false,
  }).promise;
}

export interface RenderedPage {
  page: HTMLElement; // holds the canvas, the highlight layer and the text layer
  marks: HTMLElement;
  cancel(): void;
}

// renderPage draws page n to fit width pixels times zoom, with a selectable text layer.
export async function renderPage(doc: PdfDoc, n: number, width: number, zoom: number): Promise<RenderedPage> {
  const p = await doc.getPage(n);
  const base = p.getViewport({ scale: 1 });
  const scale = Math.max(0.2, (width / base.width) * zoom);
  const vp = p.getViewport({ scale });
  const ratio = window.devicePixelRatio || 1;
  const canvas = document.createElement("canvas");
  canvas.width = Math.floor(vp.width * ratio);
  canvas.height = Math.floor(vp.height * ratio);
  canvas.style.width = `${Math.floor(vp.width)}px`;
  canvas.style.height = `${Math.floor(vp.height)}px`;
  canvas.setAttribute("aria-hidden", "true");
  const marks = document.createElement("div");
  marks.className = "pdf-marks";
  const text = document.createElement("div");
  text.className = "textLayer";
  const page = document.createElement("div");
  page.className = "pdf-page";
  page.style.width = `${Math.floor(vp.width)}px`;
  page.style.height = `${Math.floor(vp.height)}px`;
  page.style.setProperty("--scale-factor", String(scale));
  page.style.setProperty("--total-scale-factor", String(scale));
  page.append(canvas, marks, text);
  const task: RenderTask = p.render({
    canvas,
    viewport: vp,
    transform: ratio === 1 ? undefined : [ratio, 0, 0, ratio, 0, 0],
  });
  const layer = new TextLayer({ textContentSource: p.streamTextContent(), container: text, viewport: vp });
  let done = false;
  const cancel = () => {
    if (!done) task.cancel();
    layer.cancel();
  };
  try {
    await Promise.all([task.promise, layer.render()]);
  } finally {
    done = true;
  }
  return { page, marks, cancel };
}

// outline reads the PDF's bookmarks as contents entries with page numbers.
export async function outline(doc: PdfDoc): Promise<OutlineItem[]> {
  const items = await doc.getOutline().catch(() => null);
  const out: OutlineItem[] = [];
  async function walk(list: typeof items, level: number) {
    for (const it of list ?? []) {
      let page = 0;
      try {
        const dest = typeof it.dest === "string" ? await doc.getDestination(it.dest) : it.dest;
        if (Array.isArray(dest) && dest[0] && typeof dest[0] === "object") page = (await doc.getPageIndex(dest[0])) + 1;
        else if (Array.isArray(dest) && typeof dest[0] === "number") page = dest[0] + 1;
      } catch {
        page = 0;
      }
      if (page > 0 && it.title) out.push({ title: it.title, page, level });
      await walk(it.items as typeof items, level + 1);
      if (out.length > 2000) return;
    }
  }
  await walk(items, 1);
  return out;
}
