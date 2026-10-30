// The in-app PDF viewer for a run's exported PDFs. WebKitGTK has no PDF
// viewer of its own, so the app bundles pdf.js and draws each page on a
// canvas, the same on Linux, Windows and macOS.
//
// What it is held to:
//   - The bytes come from RunAPI.RunPDF, given the run's id and the path
//     RunOutputs listed: Go holds that path to the run folder (no link at
//     any part of it) and hands over only a PDF the run's envelope lists
//     with the SHA-256 of those bytes.
//   - pdf.js's display layer runs nothing a PDF carries: the scripting
//     sandbox (pdf.sandbox) is never loaded, XFA forms are off, and only
//     canvases are drawn, so no link or form field in the PDF is live here.
//     pdf.js 6 has no eval path to turn off, and the window's CSP has no
//     'unsafe-eval'. The worker is the bundle's own file, which the window's CSP admits
//     under script-src 'self'; no cMap, font or wasm URL is given, so
//     nothing is fetched.
//   - Memory: the document is loaded once and kept across zoom changes. A
//     page is drawn only when it scrolls near the view and its canvas is
//     dropped again when it is far away, and no canvas is larger than
//     MAX_CANVAS_PIXELS, so a long PDF at a high zoom cannot exhaust the
//     renderer.
import { useEffect, useRef, useState } from "react";
import * as pdfjs from "pdfjs-dist/legacy/build/pdf.mjs";
import workerUrl from "pdfjs-dist/legacy/build/pdf.worker.min.mjs?url";
import * as RunAPI from "../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import { Button, ErrorBox } from "./ui";

pdfjs.GlobalWorkerOptions.workerSrc = workerUrl;

// A page is drawn at most this wide at 100%, and a document past this many
// pages shows its first ones and says so: the system viewer has the rest.
const MAX_WIDTH = 900;
const MAX_PAGES = 200;
const MAX_CANVAS_PIXELS = 16_000_000;
// How far outside the view a page is drawn ahead, and let go beyond.
const DRAW_MARGIN = "600px 0px";

type Drawn = { canvas: HTMLCanvasElement; task?: { cancel: () => void } };

function bytesOf(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

/** The bytes of one PDF, base64, as Go hands them on: the PDF export's record
 *  by default (RunAPI.RunPDF), a deliverable's with RunAPI.RunDeliverablePDF. */
export type PdfSource = (runId: string, path: string) => Promise<string>;

export function PdfViewer({ runId, path, name, load = RunAPI.RunPDF }: { runId: string; path: string; name: string; load?: PdfSource }) {
  const pagesRef = useRef<HTMLDivElement>(null);
  const [doc, setDoc] = useState<pdfjs.PDFDocumentProxy | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [zoom, setZoom] = useState(1);

  // Load the document once per PDF.
  useEffect(() => {
    let gone = false;
    let task: pdfjs.PDFDocumentLoadingTask | null = null;
    setDoc(null);
    setError(null);
    (async () => {
      try {
        const data = bytesOf(await load(runId, path));
        task = pdfjs.getDocument({ data, enableXfa: false, useSystemFonts: false, stopAtErrors: false });
        const d = await task.promise;
        if (!gone) setDoc(d);
      } catch (e) {
        if (!gone) setError(String(e));
      }
    })();
    return () => {
      gone = true;
      void task?.destroy();
    };
  }, [runId, path, load]);

  // A slot per page at this zoom; a page's canvas is drawn only while its
  // slot is near the view, and let go when it is not.
  useEffect(() => {
    const host = pagesRef.current;
    if (!doc || !host) return;
    let gone = false;
    const count = Math.min(doc.numPages, MAX_PAGES);
    const width = Math.floor(Math.min(host.clientWidth || MAX_WIDTH, MAX_WIDTH) * zoom);
    const ratio = window.devicePixelRatio || 1;
    const drawn = new Map<number, Drawn>();
    host.replaceChildren();

    const draw = async (n: number, slot: HTMLDivElement) => {
      if (drawn.has(n)) return;
      const entry: Drawn = { canvas: document.createElement("canvas") };
      drawn.set(n, entry);
      try {
        const page = await doc.getPage(n);
        if (gone || drawn.get(n) !== entry) return;
        const base = page.getViewport({ scale: 1 });
        let scale = (width / base.width) * ratio;
        const pixels = base.width * base.height * scale * scale;
        if (pixels > MAX_CANVAS_PIXELS) scale *= Math.sqrt(MAX_CANVAS_PIXELS / pixels);
        const viewport = page.getViewport({ scale });
        entry.canvas.width = Math.floor(viewport.width);
        entry.canvas.height = Math.floor(viewport.height);
        entry.canvas.className = "pdf-page-canvas";
        slot.style.height = `${Math.floor((base.height / base.width) * width)}px`;
        slot.replaceChildren(entry.canvas);
        const t = page.render({ canvas: entry.canvas, viewport });
        entry.task = t;
        await t.promise;
      } catch {
        // A cancelled render (scrolled away, zoom changed) is not an error;
        // a page that cannot be drawn stays blank.
      }
    };
    const release = (n: number, slot: Element | null) => {
      const entry = drawn.get(n);
      if (!entry) return;
      entry.task?.cancel();
      entry.canvas.width = 0;
      entry.canvas.height = 0;
      slot?.replaceChildren();
      drawn.delete(n);
    };

    const observer = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          const slot = e.target as HTMLDivElement;
          const n = Number(slot.dataset.page);
          if (e.isIntersecting) void draw(n, slot);
          else release(n, slot);
        }
      },
      { root: host, rootMargin: DRAW_MARGIN },
    );

    (async () => {
      // The first page's proportions lay every slot out until a page is
      // drawn and sets its own height.
      const first = await doc.getPage(1);
      if (gone) return;
      const base = first.getViewport({ scale: 1 });
      const height = Math.floor((base.height / base.width) * width);
      for (let n = 1; n <= count; n++) {
        const slot = document.createElement("div");
        slot.className = "pdf-page";
        slot.dataset.page = String(n);
        slot.style.width = `${width}px`;
        slot.style.height = `${height}px`;
        slot.setAttribute("role", "img");
        slot.setAttribute("aria-label", `${name}, page ${n} of ${doc.numPages}`);
        host.appendChild(slot);
        observer.observe(slot);
      }
    })();

    return () => {
      gone = true;
      observer.disconnect();
      for (const n of [...drawn.keys()]) release(n, host.querySelector(`[data-page="${n}"]`));
    };
  }, [doc, zoom, name]);

  const pages = doc?.numPages ?? 0;
  return (
    <div className="pdf-viewer">
      <div className="pdf-viewer-bar">
        <span className="muted small">
          {error ? "" : !doc ? "Opening the PDF…" : `${pages} page${pages === 1 ? "" : "s"}`}
          {doc && pages > MAX_PAGES ? `, the first ${MAX_PAGES} shown here: open it in your PDF viewer for the rest` : ""}
        </span>
        <span className="pdf-viewer-zoom">
          <Button onClick={() => setZoom((z) => Math.max(0.5, Math.round((z - 0.25) * 100) / 100))} aria-label="Zoom out">−</Button>
          <span className="muted small">{Math.round(zoom * 100)}%</span>
          <Button onClick={() => setZoom((z) => Math.min(3, Math.round((z + 0.25) * 100) / 100))} aria-label="Zoom in">+</Button>
        </span>
      </div>
      {error && <ErrorBox error={`The PDF could not be shown here: ${error}`} />}
      <div ref={pagesRef} className="pdf-viewer-pages" aria-busy={!doc && !error} />
    </div>
  );
}
