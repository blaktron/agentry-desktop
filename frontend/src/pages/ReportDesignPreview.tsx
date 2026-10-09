// The Report design page's live preview (plan report-designs.md §4 "The
// editor", D10; agentry-desktop#759): the CLI's own sample report
// (agentry-cli:runnerimage/toolsrv/testdata/pdf/thumbnail.md) laid out in
// HTML from the design's values, with the faces as web fonts. It is a
// likeness, not the render: the thumbnails above it are the render. It is
// the site's preview (agentry-web components/report-design-preview.tsx),
// and each rule follows the runner's template (agentry-cli:runnerimage/
// toolsrv/pdf/report.typ): its ink, hairline and code fill, the accent where
// the core's AccentOnText puts it on text and on rules, bars, tints and
// callout edges otherwise, the title block, the heading treatment on the
// top two levels, tables, quotes, and the footer's rule, line and page
// number.
//
// The page is white in both colour schemes, as a PDF is. Sizes are in
// points of the paper (A4 595 pt wide, US Letter 612 pt), scaled to the
// preview's width with container units, so the paper changes the
// proportions as it does in the PDF. Only the core's families and a hex
// accent reach a style (reportDesign.ts cssFamily, cssColour); the tint is
// the core's, from CheckReportDesign.
import type { CSSProperties, ReactNode } from "react";
import "../reportFonts/fonts.css";
import { cssColour, cssFamily, type ReportDesign, type ReportFamily } from "../reportDesign";

const INK = "#1a1a1a";
const MUTED = "#5b6068";
const HAIRLINE = "#c9cdd3";
const CODE_FILL = "#f3f4f6";

const PAPER: Record<string, { width: number; height: number; margin: number }> = {
  a4: { width: 595, height: 842, margin: 62 }, // 22 mm
  letter: { width: 612, height: 792, margin: 62 },
};

// pt is a length in points of the paper, in the preview's own scale.
const pt = (n: number) => `calc(var(--rd-pt) * ${Number(n.toFixed(3))})`;

export function ReportDesignPreview({ design, tint, families }: { design: ReportDesign; tint: string; families: ReportFamily[] }) {
  const paper = PAPER[design.paper] ?? PAPER.a4;
  const accent = cssColour(design.accent);
  const tinted = cssColour(tint || "#ffffff");
  const body = 10.5;
  const headingFont = cssFamily(design.fonts.headings, families);

  const page: CSSProperties = {
    // 100cqw is the frame's width; one point of this paper is that over its width.
    ["--rd-pt" as string]: `calc(100cqw / ${paper.width})`,
    aspectRatio: `${paper.width} / ${paper.height}`,
    padding: pt(paper.margin),
    paddingBottom: pt(36),
    background: "#ffffff",
    color: INK,
    fontFamily: cssFamily(design.fonts.body, families),
    fontSize: pt(body),
    lineHeight: 1.45,
    display: "flex",
    flexDirection: "column",
    boxShadow: "0 1px 3px rgba(0,0,0,0.18)",
    textAlign: "left",
  };

  // A treated heading: levels 1 and 2 of the sections (the title is apart).
  const heading = (level: 1 | 2, text: string, num: string): ReactNode => {
    const base: CSSProperties = {
      fontFamily: headingFont,
      fontWeight: 700,
      fontSize: pt(body * (level === 1 ? 1.3 : 1.12)),
      color: INK,
      margin: `${pt(body * 1.3)} 0 ${pt(body * 0.6)}`,
      lineHeight: 1.2,
    };
    const style = design.headings;
    const Tag = level === 1 ? "h2" : "h3";
    if (style === "smallcaps") {
      return <Tag style={{ ...base, color: accent, fontVariant: "small-caps", letterSpacing: "0.04em" }}>{text}</Tag>;
    }
    if (style === "numbered") {
      return <Tag style={{ ...base, color: accent }}>{num} {text}</Tag>;
    }
    if (style === "rule") {
      return <Tag style={{ ...base, paddingBottom: pt(2), borderBottom: `${pt(level === 1 ? 1 : 0.5)} solid ${accent}` }}>{text}</Tag>;
    }
    if (style === "bar") {
      // A short accent bar above the heading.
      return (
        <div>
          <div aria-hidden style={{ width: "2.2em", height: pt(3), background: accent, marginTop: pt(body * 1.3), fontSize: pt(body) }} />
          <Tag style={{ ...base, marginTop: pt(4) }}>{text}</Tag>
        </div>
      );
    }
    return <Tag style={base}>{text}</Tag>;
  };

  const titleText = "Quarterly review";
  const titleStyle: CSSProperties = { fontFamily: headingFont, fontWeight: 700, fontSize: pt(body * 2), lineHeight: 1.15, margin: 0, color: INK };
  let title: ReactNode;
  switch (design.titleBlock) {
    case "band":
      title = (
        <div style={{ background: accent, padding: `${pt(16)} ${pt(14)}` }}>
          <h1 style={{ ...titleStyle, color: "#ffffff" }}>{titleText}</h1>
        </div>
      );
      break;
    case "bar":
      title = (
        <div style={{ borderLeft: `${pt(4)} solid ${accent}`, padding: `${pt(4)} 0 ${pt(4)} ${pt(12)}` }}>
          <h1 style={titleStyle}>{titleText}</h1>
        </div>
      );
      break;
    case "ruled":
      title = (
        <div style={{ borderTop: `${pt(1)} solid ${accent}`, borderBottom: `${pt(0.5)} solid ${accent}`, padding: `${pt(body * 0.5)} 0` }}>
          <h1 style={titleStyle}>{titleText}</h1>
        </div>
      );
      break;
    default:
      title = <h1 style={titleStyle}>{titleText}</h1>;
  }

  const cell = (y: number, i: number): CSSProperties => ({
    padding: `${pt(4)} ${pt(5)}`,
    textAlign: "left",
    fontWeight: y === 0 ? 700 : 400,
    background: (y === 0 && design.tables.tinted) || (y > 0 && design.tables.banded && y % 2 === 0) ? tinted : undefined,
    borderTop: y === 1 ? `${pt(0.8)} solid ${accent}` : y > 1 ? `${pt(0.4)} solid ${HAIRLINE}` : undefined,
    borderBottom: `${pt(0.4)} solid ${HAIRLINE}`,
    whiteSpace: i === 0 ? "nowrap" : undefined,
  });
  const rows = [
    ["Service", "Severity", "Status"],
    ["billing-gateway", "high", "fixed"],
    ["route-planner", "medium", "in progress"],
    ["warehouse-sync", "low", "accepted"],
  ];

  const quote: CSSProperties =
    design.quotes === "callout"
      ? { background: tinted, borderLeft: `${pt(3)} solid ${accent}`, padding: `${pt(8)} ${pt(10)}`, margin: `${pt(body)} 0` }
      : { borderLeft: `${pt(1)} solid ${HAIRLINE}`, padding: `${pt(2)} 0 ${pt(2)} ${pt(10)}`, margin: `${pt(body)} 0` };

  const numbers = design.footer.pageNumbers === "n" ? "1" : design.footer.pageNumbers === "nOfN" ? "1 of 1" : "";
  const footer = numbers !== "" || design.footer.text !== "";

  return (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <div
        role="img"
        aria-label={`A likeness of a report's first page in this design: ${design.fonts.headings} headings, ${design.fonts.body} text, the accent ${design.accent}.`}
        style={page}
      >
        <div aria-hidden style={{ flex: "1 1 auto" }}>
          {title}
          <p style={{ margin: `${pt(body * 1.4)} 0 ${pt(body * 0.7)}` }}>
            This sample shows how a report design lays out a page: the title block, the headings, a table, a quote and
            code. The words are the same in every theme.
          </p>
          {heading(1, "Summary", "1")}
          <ul style={{ margin: 0, paddingLeft: "1.2em", listStyle: "disc" }}>
            <li>Twelve services were reviewed; nine passed without findings.</li>
            <li>Three had findings, one of them <strong>high</strong> severity, now fixed.</li>
          </ul>
          <div style={quote}>A review says what was checked and what was found.</div>
          {heading(1, "Findings", "2")}
          <table style={{ borderCollapse: "collapse", fontSize: pt(body) }}>
            <tbody>
              {rows.map((r, y) => (
                <tr key={r[0]}>
                  {r.map((c, i) => <td key={c} style={cell(y, i)}>{c}</td>)}
                </tr>
              ))}
            </tbody>
          </table>
          {heading(2, "The change", "2.1")}
          <pre
            style={{
              fontFamily: cssFamily(design.fonts.code, families),
              fontSize: pt(9),
              background: CODE_FILL,
              color: INK,
              padding: pt(7),
              borderRadius: pt(2),
              margin: 0,
              whiteSpace: "pre-wrap",
            }}
          >
            <span style={{ color: "#d73a49" }}>ssl_protocols</span> TLSv1.2 TLSv1.3;
          </pre>
          {heading(1, "Next steps", "3")}
          <ol style={{ margin: 0, paddingLeft: "1.4em", listStyle: "decimal" }}>
            <li>The identity team closes the gap by 31 October.</li>
            <li>The review team checks the fix and updates this record.</li>
          </ol>
        </div>
        {footer && (
          <div aria-hidden style={{ borderTop: `${pt(0.5)} solid ${accent}`, paddingTop: pt(3), fontSize: pt(8), color: MUTED, display: "flex", gap: pt(8) }}>
            <span style={{ flex: "1 1 auto", overflowWrap: "anywhere" }}>{design.footer.text}</span>
            <span>{numbers}</span>
          </div>
        )}
      </div>
    </div>
  );
}
