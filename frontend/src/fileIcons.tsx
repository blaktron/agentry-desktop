// The file icons of Home's Deliverables card, drawn inline: a page with a
// folded corner and a band naming the kind. Nothing is fetched and no icon
// font is used. The colours are the page's own tokens (style.css, .file-icon),
// so an icon reads in the light and the dark theme.
import type { IconKind } from "./deliverables";

const BAND: Record<IconKind, string> = {
  pdf: "PDF", html: "HTML", document: "DOC", spreadsheet: "XLS", presentation: "PPT", image: "", text: "TXT", other: "",
};

// A picture's glyph: a sun over two hills.
function Picture() {
  return (
    <g className="file-icon-accent">
      <circle cx="15" cy="25" r="3" />
      <path d="M9 40l8-9 5 5 4-4 7 8z" />
    </g>
  );
}

// A page's glyph: short lines of text, two above a band and three on a plain page.
function Lines({ banded }: { banded: boolean }) {
  return (
    <g className="file-icon-stroke">
      <path d={banded ? "M11 19h18M11 25h12" : "M11 20h18M11 27h18M11 34h11"} />
    </g>
  );
}

/** One icon, 40 by 48, with no text of its own for a screen reader: the
 * button that holds it names the file. `revealOnly` marks a file a click
 * only shows in its folder, with a small folder at its corner. */
export function FileIcon({ kind, revealOnly = false }: { kind: IconKind; revealOnly?: boolean }) {
  const band = BAND[kind];
  return (
    <svg className={`file-icon file-icon-${kind}`} viewBox="0 0 40 48" width="40" height="48" aria-hidden="true" focusable="false">
      <path className="file-icon-page" d="M6 2h19l10 10v32a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V4a2 2 0 0 1 1-2z" />
      <path className="file-icon-fold" d="M25 2v10h10" />
      {kind === "image" ? <Picture /> : <Lines banded={band !== ""} />}
      {band && (
        <>
          <rect className="file-icon-band" x="5" y="30" width="30" height="12" rx="1" />
          <text className="file-icon-label" x="20" y="39" textAnchor="middle">{band}</text>
        </>
      )}
      {revealOnly && (
        <g className="file-icon-reveal">
          <circle cx="31" cy="38" r="8" />
          <path d="M26 35.5h3l1.2 1.4H36v5.6H26z" />
        </g>
      )}
    </svg>
  );
}
