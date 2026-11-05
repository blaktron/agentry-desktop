// The Report design page's live preview: the runner's own sample report laid
// out in HTML from the design's values, with the faces as web fonts. It is a
// likeness, not the render: the kits' pictures are the render. Each rule
// follows the runner's template: its ink, hairline and code fill, the accent
// where the core puts it on text and on rules, bars, tints and callout edges
// otherwise, the secondary on a split cover's panel and a geometric cover's
// shapes, the title block, the heading treatment on the top two levels, the
// heading scale and the margin presets, tables, quotes, the running header in
// its style with its slots, and the footer's rule, three slots and page
// number. Above the first page, when the design draws a cover, a likeness of
// the cover page in its style with the facts it shows; a custom cover shows
// its last render, since the page draws no Typst.
//
// The page is white in both colour schemes, as a PDF is. Sizes are in points
// of the paper (A4 595 pt wide, US Letter 612 pt), scaled to the preview's
// width with container units, so the paper changes the proportions as it does
// in the PDF. Only the core's families and a hex colour reach a style
// (reportDesign.ts cssFamily, cssColour); the tints and the slots' expanded
// text are the core's, from CheckReportDesign. The person's images are drawn
// only as the data: URLs Go's view built from the core's answer.
import type { CSSProperties, ReactNode } from "react";
import "../reportFonts/fonts.css";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { cssColour, cssFamily, isImageSrc, isLogoSrc, type CoverStyleInfo, type ReportDesign, type ReportFamily } from "../reportDesign";

const INK = "#1a1a1a";
const MUTED = "#5b6068";
const HAIRLINE = "#c9cdd3";
const CODE_FILL = "#f3f4f6";

const PAPER: Record<string, { width: number; height: number }> = {
  a4: { width: 595, height: 842 },
  letter: { width: 612, height: 792 },
};
// The margin presets in points, and the heading scales as factors over the
// body size, as the template maps them.
const MARGIN: Record<string, number> = { narrow: 48, regular: 62, wide: 78 };
const SCALE: Record<string, [number, number]> = { compact: [1.2, 1.05], regular: [1.3, 1.12], generous: [1.45, 1.2] };

// pt is a length in points of the paper, in the preview's own scale.
const pt = (n: number) => `calc(var(--rd-pt) * ${Number(n.toFixed(3))})`;

// LOGO_TALL is the most a logo is drawn high, 14 mm in points; it is at
// most 3.5 times that wide.
const LOGO_TALL = 39.7;

// The sample's facts, as CheckReportDesign expands the slots with them.
const SAMPLE = { title: "Quarterly review", subtitle: "Twelve services, three findings", author: "The review team", date: "4 November 2026" };

export function ReportDesignPreview({ design, check, families, coverStyles, logo = "", coverImage = "", customCover = "" }: {
  design: ReportDesign;
  check: app.ReportDesignCheck | null;
  families: ReportFamily[];
  coverStyles: CoverStyleInfo[];
  logo?: string;
  coverImage?: string;
  customCover?: string;
}) {
  const paper = PAPER[design.paper] ?? PAPER.a4;
  const margin = MARGIN[design.margins] ?? MARGIN.regular;
  const [h1, h2] = SCALE[design.headingScale] ?? SCALE.regular;
  const accent = cssColour(design.accent);
  const secondary = design.secondary ? cssColour(design.secondary, accent) : "";
  const tinted = cssColour(check?.tint ?? "", "#ffffff");
  const body = 10.5;
  const headingFont = cssFamily(design.fonts.headings, families);
  const bodyFont = cssFamily(design.fonts.body, families);
  const logoSrc = isLogoSrc(logo) ? logo : "";
  const coverSrc = isImageSrc(coverImage) ? coverImage : "";
  const brand = design.brand;

  const page = (extra: CSSProperties = {}): CSSProperties => ({
    // 100cqw is the frame's width; one point of this paper is that over its width.
    ["--rd-pt" as string]: `calc(100cqw / ${paper.width})`,
    aspectRatio: `${paper.width} / ${paper.height}`,
    padding: pt(margin),
    paddingTop: pt(design.header.style === "none" ? margin : margin - 8),
    paddingBottom: pt(36),
    background: "#ffffff",
    color: INK,
    fontFamily: bodyFont,
    fontSize: pt(body),
    lineHeight: 1.45,
    display: "flex",
    flexDirection: "column",
    boxShadow: "0 1px 3px rgba(0,0,0,0.18)",
    textAlign: "left",
    overflow: "hidden",
    position: "relative",
    ...extra,
  });

  // A treated heading: levels 1 and 2 of the sections (the title is apart).
  const heading = (level: 1 | 2, text: string, num: string): ReactNode => {
    const base: CSSProperties = {
      fontFamily: headingFont,
      fontWeight: 700,
      fontSize: pt(body * (level === 1 ? h1 : h2)),
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
      return (
        <div>
          <div aria-hidden style={{ width: "2.2em", height: pt(3), background: accent, marginTop: pt(body * 1.3), fontSize: pt(body) }} />
          <Tag style={{ ...base, marginTop: pt(4) }}>{text}</Tag>
        </div>
      );
    }
    return <Tag style={base}>{text}</Tag>;
  };

  const titleStyle: CSSProperties = { fontFamily: headingFont, fontWeight: 700, fontSize: pt(body * 2), lineHeight: 1.15, margin: 0, color: INK };
  const beside: CSSProperties = logoSrc ? { display: "flex", alignItems: "center", justifyContent: "space-between", gap: pt(14) } : {};
  const logoImg = (chip: boolean, tall = LOGO_TALL): ReactNode =>
    logoSrc ? (
      <img src={logoSrc} alt="Your logo" data-logo style={{
        display: "block", flex: "none", width: "auto", height: "auto", maxHeight: pt(tall), maxWidth: pt(tall * 3.5),
        ...(chip ? { background: "#ffffff", padding: pt(4), borderRadius: pt(2), boxSizing: "content-box" as const } : {}),
      }} />
    ) : null;
  let title: ReactNode;
  switch (design.titleBlock) {
    case "band":
      title = (
        <div style={{ background: accent, padding: `${pt(16)} ${pt(14)}`, ...beside }}>
          <h1 style={{ ...titleStyle, color: "#ffffff" }}>{SAMPLE.title}</h1>
          {logoImg(true)}
        </div>
      );
      break;
    case "bar":
      title = (
        <div style={{ borderLeft: `${pt(4)} solid ${accent}`, padding: `${pt(4)} 0 ${pt(4)} ${pt(12)}`, ...beside }}>
          <h1 style={titleStyle}>{SAMPLE.title}</h1>
          {logoImg(false)}
        </div>
      );
      break;
    case "ruled":
      title = (
        <div style={{ borderTop: `${pt(1)} solid ${accent}`, borderBottom: `${pt(0.5)} solid ${accent}`, padding: `${pt(body * 0.5)} 0`, ...beside }}>
          <h1 style={titleStyle}>{SAMPLE.title}</h1>
          {logoImg(false)}
        </div>
      );
      break;
    default:
      title = logoSrc ? (
        <div style={beside}>
          <h1 style={titleStyle}>{SAMPLE.title}</h1>
          {logoImg(false)}
        </div>
      ) : (
        <h1 style={titleStyle}>{SAMPLE.title}</h1>
      );
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

  // The running header, as the template draws it from page 2: this likeness
  // is the first page, so it carries the header to show its look.
  const hslots = check?.header ?? ["", "", ""];
  const small: CSSProperties = { fontSize: pt(8), color: MUTED, display: "flex", alignItems: "center", gap: pt(8), minHeight: pt(12) };
  const slotSpan = (text: string, align: "left" | "center" | "right", flex = "1 1 0"): ReactNode => (
    <span style={{ flex, textAlign: align, overflow: "hidden", whiteSpace: "nowrap", textOverflow: "ellipsis" }}>{text}</span>
  );
  let header: ReactNode = null;
  switch (design.header.style) {
    case "rule":
      header = <div aria-hidden data-header style={{ ...small, borderTop: `${pt(0.5)} solid ${accent}`, paddingTop: pt(3), marginBottom: pt(10) }}>{slotSpan(hslots[0], "left")}{slotSpan(hslots[1], "center")}{slotSpan(hslots[2], "right")}</div>;
      break;
    case "brandLine":
      header = <div aria-hidden data-header style={{ ...small, marginBottom: pt(10) }}>{slotSpan(brand.name, "left")}{slotSpan(hslots[2] || hslots[0], "right")}</div>;
      break;
    case "titleAndPage":
      header = <div aria-hidden data-header style={{ ...small, marginBottom: pt(10) }}>{slotSpan(SAMPLE.title, "left")}{slotSpan("2", "right", "0 0 auto")}</div>;
      break;
    case "logoLeft":
      header = (
        <div aria-hidden data-header style={{ ...small, marginBottom: pt(10) }}>
          {logoSrc ? logoImg(false, 14) : <span style={{ width: pt(30), height: pt(12), border: `${pt(0.5)} dashed ${HAIRLINE}`, display: "inline-block" }} />}
          {slotSpan(hslots[2] || hslots[0], "right")}
        </div>
      );
      break;
    default:
      header = null;
  }

  const fslots = check?.footer ?? ["", "", ""];
  const numbers = design.footer.pageNumbers === "n" ? "1" : design.footer.pageNumbers === "nOfN" ? "1 of 4" : "";
  const footer = numbers !== "" || fslots.some((s) => s !== "");

  const drawsCover = design.cover.when !== "never" && coverStyles.some((s) => s.name === design.cover.style);

  return (
    <div style={{ containerType: "inline-size", width: "100%", display: "flex", flexDirection: "column", gap: "12px" }}>
      {drawsCover && (
        <Cover design={design} accent={accent} secondary={secondary} paper={paper} page={page} headingFont={headingFont} bodyFont={bodyFont}
          logoSrc={logoSrc} coverSrc={coverSrc} customCover={isLogoSrc(customCover) ? customCover : ""} />
      )}
      <div
        role="img"
        aria-label={`A likeness of a report's first page in this design: ${design.fonts.headings} headings, ${design.fonts.body} text, the accent ${design.accent}${design.secondary ? ` and the secondary ${design.secondary}` : ""}.`}
        style={page()}
      >
        <div aria-hidden style={{ flex: "1 1 auto", minHeight: 0 }}>
          {header}
          {title}
          <p style={{ margin: `${pt(body * 1.4)} 0 ${pt(body * 0.7)}` }}>
            This sample shows how a report design lays out a page: the title block, the headings, a table, a quote and
            code. The words are the same in every design.
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
          <div aria-hidden data-footer style={{ ...small, borderTop: `${pt(0.5)} solid ${accent}`, paddingTop: pt(3) }}>
            {slotSpan(fslots[0], "left")}
            {slotSpan(fslots[1], "center")}
            {slotSpan(fslots[2], "right")}
            {numbers !== "" && <span style={{ flex: "0 0 auto" }}>{numbers}</span>}
          </div>
        )}
      </div>
    </div>
  );
}

// Cover is the cover page's likeness in the design's style, with the facts
// it shows: the title in white on a band, the brand on a split panel in the
// secondary, accent and secondary shapes on geometric, the title at display
// size on largeType, a hairline frame, or the cover image across the top of
// a plate (a band without one). A custom cover is its last render.
function Cover({ design, accent, secondary, paper, page, headingFont, bodyFont, logoSrc, coverSrc, customCover }: {
  design: ReportDesign;
  accent: string;
  secondary: string;
  paper: { width: number; height: number };
  page: (extra?: CSSProperties) => CSSProperties;
  headingFont: string;
  bodyFont: string;
  logoSrc: string;
  coverSrc: string;
  customCover: string;
}) {
  const shows = (fact: string) => design.cover.shows.includes(fact);
  const body = 10.5;
  const brand = design.brand;
  const label = `A likeness of the cover page in the ${design.cover.style} style.`;
  if (design.cover.style === "custom") {
    return customCover ? (
      <img src={customCover} alt="Your custom cover's last render" data-cover="custom" style={{ width: "100%", height: "auto", aspectRatio: `${paper.width} / ${paper.height}`, objectFit: "contain", background: "#ffffff", boxShadow: "0 1px 3px rgba(0,0,0,0.18)" }} />
    ) : (
      <div role="img" aria-label="A custom cover with no picture of it yet; a band cover stands in" data-cover="custom" style={{ ...page(), alignItems: "center", justifyContent: "center", color: MUTED, fontSize: pt(10) }}>
        No picture of the custom cover yet
      </div>
    );
  }
  const titleStyle = (colour: string, size = 2.6): CSSProperties => ({ fontFamily: headingFont, fontWeight: 700, fontSize: pt(body * size), lineHeight: 1.1, margin: 0, color: colour });
  const line = (text: string, colour = INK, size = 1): ReactNode => (text ? <p style={{ margin: `${pt(3)} 0 0`, color: colour, fontSize: pt(body * size), fontFamily: bodyFont }}>{text}</p> : null);
  const facts = (colour = INK): ReactNode => (
    <>
      {shows("subtitle") && line(SAMPLE.subtitle, colour, 1.2)}
      {shows("author") && line(SAMPLE.author, colour)}
      {shows("date") && line(SAMPLE.date, colour)}
    </>
  );
  const brandBlock = (colour = INK): ReactNode =>
    shows("brand") && (brand.name || brand.strapline || brand.contact.length > 0 || brand.website) ? (
      <div style={{ marginTop: pt(14), color: colour, fontSize: pt(body * 0.9), fontFamily: bodyFont }}>
        {brand.name && <div style={{ fontWeight: 700 }}>{brand.name}</div>}
        {brand.strapline && <div>{brand.strapline}</div>}
        {brand.contact.map((c) => <div key={c}>{c}</div>)}
        {brand.website && <div>{brand.website}</div>}
      </div>
    ) : null;
  const confidentiality = (colour = MUTED): ReactNode =>
    shows("confidentiality") && brand.confidentiality ? <p style={{ margin: `${pt(10)} 0 0`, color: colour, fontSize: pt(body * 0.85), fontFamily: bodyFont }}>{brand.confidentiality}</p> : null;
  const logoImg = (tall: number): ReactNode =>
    shows("logo") && logoSrc ? <img src={logoSrc} alt="" data-logo style={{ display: "block", maxHeight: pt(tall), maxWidth: pt(tall * 3.5), width: "auto", height: "auto", marginBottom: pt(10) }} /> : null;
  const base = page({ padding: 0, paddingTop: 0, paddingBottom: 0 });
  const inner: CSSProperties = { padding: pt(MARGIN[design.margins] ?? 62), flex: "1 1 auto", display: "flex", flexDirection: "column" };

  switch (design.cover.style) {
    case "band":
    case "plate": {
      const plate = design.cover.style === "plate" && coverSrc;
      return (
        <div role="img" aria-label={label} data-cover={design.cover.style} style={base}>
          {plate ? (
            <img src={coverSrc} alt="" style={{ width: "100%", height: "50%", objectFit: "cover", display: "block" }} />
          ) : (
            <div style={{ background: accent, height: "34%", display: "flex", alignItems: "flex-end", padding: pt(40) }}>
              <h1 style={titleStyle("#ffffff")}>{SAMPLE.title}</h1>
            </div>
          )}
          <div style={inner}>
            {plate && <h1 style={titleStyle(INK)}>{SAMPLE.title}</h1>}
            {facts()}
            {logoImg(LOGO_TALL)}
            {brandBlock()}
            <div style={{ flex: "1 1 auto" }} />
            {confidentiality()}
          </div>
        </div>
      );
    }
    case "split": {
      const panel = secondary || accent;
      return (
        <div role="img" aria-label={label} data-cover="split" style={{ ...base, flexDirection: "row" }}>
          <div style={{ background: panel, width: "34%", padding: pt(28), color: "#ffffff", display: "flex", flexDirection: "column" }}>
            {logoImg(LOGO_TALL)}
            {brandBlock("#ffffff")}
            <div style={{ flex: "1 1 auto" }} />
            {confidentiality("#ffffff")}
          </div>
          <div style={{ ...inner, justifyContent: "center" }}>
            <h1 style={titleStyle(INK)}>{SAMPLE.title}</h1>
            {facts()}
          </div>
        </div>
      );
    }
    case "geometric":
      return (
        <div role="img" aria-label={label} data-cover="geometric" style={base}>
          <div aria-hidden style={{ position: "absolute", right: "-12%", top: "-10%", width: "55%", aspectRatio: "1", borderRadius: "50%", background: accent, opacity: 0.9 }} />
          <div aria-hidden style={{ position: "absolute", left: "-8%", bottom: "8%", width: "40%", aspectRatio: "1", background: secondary || accent, opacity: secondary ? 0.9 : 0.35, transform: "rotate(12deg)" }} />
          <div style={{ ...inner, justifyContent: "center", position: "relative" }}>
            <h1 style={titleStyle(INK)}>{SAMPLE.title}</h1>
            {facts()}
            {logoImg(LOGO_TALL)}
            {brandBlock()}
            {confidentiality()}
          </div>
        </div>
      );
    case "largeType":
      return (
        <div role="img" aria-label={label} data-cover="largeType" style={base}>
          <div style={{ ...inner }}>
            <h1 style={{ ...titleStyle(accent, 4.2), wordBreak: "break-word" }}>{SAMPLE.title}</h1>
            <div style={{ flex: "1 1 auto" }} />
            {facts()}
            {logoImg(LOGO_TALL)}
            {brandBlock()}
            {confidentiality()}
          </div>
        </div>
      );
    default:
      return (
        <div role="img" aria-label={label} data-cover="framed" style={base}>
          <div style={{ ...inner, margin: pt(24), border: `${pt(0.6)} solid ${INK}`, alignItems: "center", justifyContent: "center", textAlign: "center" }}>
            {logoImg(LOGO_TALL)}
            <h1 style={titleStyle(INK)}>{SAMPLE.title}</h1>
            {facts()}
            {brandBlock()}
            {confidentiality()}
          </div>
        </div>
      );
  }
}
