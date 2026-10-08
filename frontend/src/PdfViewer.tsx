// The in-app PDF viewer for a run's exported PDFs (plan
// scratch-and-pdf-reports.md D16; agentry-desktop#730, D-072). WebKitGTK has
// no PDF viewer of its own, so the app bundles pdf.js and draws each page
// on a canvas, the same on Linux, Windows and macOS.
//
// What it is held to:
//   - The bytes come from RunAPI.RunPDF, which hands over only a PDF the
//     run's envelope lists with the SHA-256 of those bytes; the page never
//     names a file path of its own.
//   - pdf.js's display layer runs nothing a PDF carries: the scripting
//     sandbox (pdf.sandbox) is never loaded, XFA forms are off, and only
//     canvases are drawn, so no link or form field in the PDF is live here.
//     The worker is the bundle's own file, which the window's CSP admits
//     under script-src 'self'.
import { useEffect, useRef, useState } from "react";
import * as pdfjs from "pdfjs-dist/legacy/build/pdf.mjs";
import workerUrl from "pdfjs-dist/legacy/build/pdf.worker.min.mjs?url";
import * as RunAPI from "../wailsjs/go/main/RunAPI";
import { Button, ErrorBox } from "./ui";

pdfjs.GlobalWorkerOptions.workerSrc = workerUrl;

// A page is drawn at most this wide, and a document past this many pages
// shows its first ones and says so: the system viewer has the rest.
const MAX_WIDTH = 900;
const MAX_PAGES = 200;

function bytesOf(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export function PdfViewer({ runId, path, name }: { runId: string; path: string; name: string }) {
  const pagesRef = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<{ phase: "loading" | "ready" | "failed"; pages?: number; error?: string }>({ phase: "loading" });
  const [zoom, setZoom] = useState(1);

  useEffect(() => {
    let gone = false;
    let task: pdfjs.PDFDocumentLoadingTask | null = null;
    const host = pagesRef.current;
    (async () => {
      try {
        const data = bytesOf(await RunAPI.RunPDF(runId, path));
        task = pdfjs.getDocument({ data, enableXfa: false, useSystemFonts: false, stopAtErrors: false });
        const doc = await task.promise;
        if (gone || !host) return;
        host.replaceChildren();
        const count = Math.min(doc.numPages, MAX_PAGES);
        const width = Math.min(host.clientWidth || MAX_WIDTH, MAX_WIDTH) * zoom;
        for (let n = 1; n <= count && !gone; n++) {
          const page = await doc.getPage(n);
          const base = page.getViewport({ scale: 1 });
          const scale = width / base.width;
          const viewport = page.getViewport({ scale: scale * (window.devicePixelRatio || 1) });
          const canvas = document.createElement("canvas");
          canvas.width = Math.floor(viewport.width);
          canvas.height = Math.floor(viewport.height);
          canvas.style.width = `${Math.floor(base.width * scale)}px`;
          canvas.setAttribute("role", "img");
          canvas.setAttribute("aria-label", `${name}, page ${n} of ${doc.numPages}`);
          canvas.className = "pdf-page";
          host.appendChild(canvas);
          await page.render({ canvas, viewport }).promise;
        }
        if (!gone) setState({ phase: "ready", pages: doc.numPages });
      } catch (e) {
        if (!gone) setState({ phase: "failed", error: String(e) });
      }
    })();
    return () => {
      gone = true;
      void task?.destroy();
    };
  }, [runId, path, name, zoom]);

  return (
    <div className="pdf-viewer">
      <div className="pdf-viewer-bar">
        <span className="muted small">
          {state.phase === "loading" ? "Opening the PDF…" : state.phase === "ready" ? `${state.pages} page${state.pages === 1 ? "" : "s"}` : ""}
          {state.phase === "ready" && (state.pages ?? 0) > MAX_PAGES ? `, the first ${MAX_PAGES} shown here: open it in your PDF viewer for the rest` : ""}
        </span>
        <span className="pdf-viewer-zoom">
          <Button onClick={() => setZoom((z) => Math.max(0.5, Math.round((z - 0.25) * 100) / 100))} aria-label="Zoom out">−</Button>
          <span className="muted small">{Math.round(zoom * 100)}%</span>
          <Button onClick={() => setZoom((z) => Math.min(3, Math.round((z + 0.25) * 100) / 100))} aria-label="Zoom in">+</Button>
        </span>
      </div>
      {state.phase === "failed" && <ErrorBox error={`The PDF could not be shown here: ${state.error}`} />}
      <div ref={pagesRef} className="pdf-viewer-pages" aria-busy={state.phase === "loading"} />
    </div>
  );
}
