// What the result view offers for a PDF a run left (plan
// scratch-and-pdf-reports.md D15, D16; agentry-desktop#730). Go decides
// whether the run's PDF export made it (services_pdf.go holds the bytes to
// the envelope's pdfs record); this says what the card draws from that
// answer: an exported PDF opens in the app's viewer and the system's, and
// can be saved; any other is Save only, and says why.

/** The Go view of a PDF, as RunOutputs sends it. */
export interface PdfFacts {
  exported: boolean;
  pages?: number;
  theme?: string;
}

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

/** The account's report theme as Settings → Profile offers it ("" is never chosen). */
export const REPORT_THEMES: { name: string; label: string; look: string }[] = [
  { name: "plain", label: "Plain", look: "Clean sans-serif pages with page numbers." },
  { name: "formal", label: "Formal", look: "Serif, with a cover page for a titled report and “page n of N”." },
  { name: "brief", label: "Brief", look: "A compact memo with a ruled heading and no cover." },
  { name: "technical", label: "Technical", look: "Dense, with banded tables and stronger code blocks." },
  { name: "letter", label: "Letter", look: "Serif, the first heading as a letterhead, and short lines such as an address kept as written." },
];

/** The sentence a save answers with. */
export function themeSavedSentence(theme: string): string {
  const t = REPORT_THEMES.find((x) => x.name === theme);
  return t
    ? `Saved. Your runs' PDF reports use the ${t.label} theme.`
    : "Saved. You have no report theme: runs from this app use Plain.";
}
