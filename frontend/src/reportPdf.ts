// What the result view offers for a PDF a run left (plan
// scratch-and-pdf-reports.md D15, D16; agentry-desktop#730). Go decides
// whether the run's PDF export made it (services_pdf.go holds the bytes to
// the envelope's pdfs record); this says what the card draws from that
// answer: an exported PDF opens in the app's viewer and the system's, and
// can be saved; any other is Save only, and says why.

import type { main } from "../wailsjs/go/models";

/** The Go view of a PDF, as RunOutputs sends it. */
export type PdfFacts = Pick<main.OutputPDFView, "exported" | "pages" | "theme">;

export interface PdfActions {
  view: boolean;
  openInSystemViewer: boolean;
  save: boolean;
  /** The line the card shows in place of a preview. */
  line: string;
}

export function pdfActions(pdf: PdfFacts): PdfActions {
  if (!pdf.exported) {
    return {
      view: false,
      openInSystemViewer: false,
      save: true,
      line: "Not a PDF this run's PDF export made: its record does not list these bytes, so it is not opened here. Save it to open it.",
    };
  }
  const facts = [
    pdf.pages === 1 ? "1 page" : pdf.pages && pdf.pages > 1 ? `${pdf.pages} pages` : "",
    // The receipt's pdfs.theme names the report design by its name.
    pdf.theme ? `${pdf.theme} design` : "",
  ].filter(Boolean).join(", ");
  return {
    view: true,
    openInSystemViewer: true,
    save: true,
    line: `A PDF the run exported${facts ? ` (${facts})` : ""}.`,
  };
}
