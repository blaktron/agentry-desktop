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
    pdf.theme ? `${pdf.theme} theme` : "",
  ].filter(Boolean).join(", ");
  return {
    view: true,
    openInSystemViewer: true,
    save: true,
    line: `A PDF the run exported${facts ? ` (${facts})` : ""}.`,
  };
}

/** What each of the core's built-in report designs looks like, by id
 *  (plan report-designs.md §4, D14; agentry-desktop#766). Go names the
 *  built-ins the card offers (ReportThemeView.choices); an id this list
 *  lacks is offered by its name alone. */
export const REPORT_THEME_LOOKS: Record<string, string> = {
  plain: "Clean sans-serif pages, near-monochrome, with page numbers.",
  classic: "Serif throughout, small-capital headings, a navy accent.",
  modern: "Sans-serif with a coloured title band and barred headings.",
  editorial: "A display serif for headings over a sans-serif body, a warm red accent.",
  technical: "Dense, numbered headings, banded tables and stronger code blocks.",
};

/** The line under a built-in the agentry command this app carries cannot
 *  draw yet: its runs use Plain until the app is updated (D20). */
export const NOT_HERE_YET = "Runs from this app use Plain for it until the app is updated.";

/** The sentence a save answers with. */
export function themeSavedSentence(theme: string, name?: string, here = true): string {
  if (!theme) return "Saved. You have no report theme: runs from this app use Plain.";
  const label = name || theme;
  return here
    ? `Saved. Your runs' PDF reports use the ${label} theme.`
    : `Saved. Your report theme is ${label}; runs from this app use Plain for it until the app is updated.`;
}
