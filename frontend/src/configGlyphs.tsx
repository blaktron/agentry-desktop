// The Configuration cards' icons, drawn inline: Agentry's own line glyphs
// for the built-ins' servers and the agent's abilities (no brand's mark is
// drawn), a server's kept icon as a data: URL, or a monogram. Nothing is
// fetched by the page and no icon font is used; the stroke is the page's
// own token (style.css, .config-glyph), so a glyph reads in either theme.
import type { ReactNode } from "react";
import type { CardIcon, GlyphKey } from "./configCards";

// Each glyph on a 24 by 24 grid, as stroked paths.
const PATHS: Record<GlyphKey, ReactNode> = {
  // A branch: two commits and a merge.
  git: <><circle cx="6" cy="5" r="2" /><circle cx="6" cy="19" r="2" /><circle cx="18" cy="8" r="2" /><path d="M6 7v10M18 10c0 5-6 4-11 8" /></>,
  // An open book with a letter.
  encyclopedia: <><path d="M3 5c3-1 6-1 9 1 3-2 6-2 9-1v14c-3-1-6-1-9 1-3-2-6-2-9-1z" /><path d="M12 6v14M6.5 9.5l1.5-3 1.5 3M7 8.5h2" /></>,
  // A page of text.
  docs: <><path d="M6 3h8l4 4v14H6z" /><path d="M14 3v4h4M9 11h6M9 14h6M9 17h4" /></>,
  // A shield with a tick.
  scan: <><path d="M12 3l7 3v5c0 5-3 8-7 10-4-2-7-5-7-10V6z" /><path d="M9 12l2 2 4-4" /></>,
  // A cloud over a sun.
  weather: <><circle cx="9" cy="8" r="3" /><path d="M9 2v1M3 8h1M4.8 3.8l.7.7M13.2 3.8l-.7.7" /><path d="M8 19h9a3 3 0 0 0 0-6 4 4 0 0 0-7.6-1A3.5 3.5 0 0 0 8 19z" /></>,
  // A pin on a folded map.
  map: <><path d="M3 6l6-2 6 2 6-2v14l-6 2-6-2-6 2z" /><path d="M9 4v14M15 6v14" /></>,
  // An envelope.
  mail: <><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M3 7l9 6 9-6" /></>,
  // A magnifier.
  search: <><circle cx="10" cy="10" r="6" /><path d="M15 15l6 6" /></>,
  // A server's rack.
  server: <><rect x="4" y="4" width="16" height="7" rx="1.5" /><rect x="4" y="13" width="16" height="7" rx="1.5" /><path d="M8 7.5h.01M8 16.5h.01" /></>,
  folder: <path d="M3 6a1 1 0 0 1 1-1h5l2 2h9a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z" />,
  // A closed box: the agent's own files.
  box: <><path d="M3 8l9-4 9 4v9l-9 4-9-4z" /><path d="M3 8l9 4 9-4M12 12v9" /></>,
  // A notebook: what it remembers.
  memory: <><rect x="5" y="3" width="14" height="18" rx="1.5" /><path d="M9 3v18M12 8h4M12 12h4" /></>,
  // A clock: its heartbeat and tasks.
  clock: <><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></>,
  terminal: <><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M7 9l3 3-3 3M12 15h5" /></>,
  globe: <><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18" /></>,
  browser: <><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M3 9h18M6.5 6.5h.01M9 6.5h.01" /></>,
  // A host the agent deploys to.
  host: <><rect x="3" y="4" width="18" height="12" rx="1.5" /><path d="M8 20h8M12 16v4" /></>,
  // A plug: a port it publishes.
  plug: <><path d="M9 3v5M15 3v5M7 8h10v3a5 5 0 0 1-10 0z" /><path d="M12 16v5" /></>,
  // A book: a knowledge pack.
  book: <><path d="M5 4h11a3 3 0 0 1 3 3v13H8a3 3 0 0 1-3-3z" /><path d="M5 17a3 3 0 0 1 3-3h11" /></>,
  // A spark: a skill.
  skill: <path d="M12 3l2.2 5.8L20 11l-5.8 2.2L12 19l-2.2-5.8L4 11l5.8-2.2z" />,
  key: <><circle cx="8" cy="15" r="4" /><path d="M11 12l8-8M16 7l2 2M14 9l2 2" /></>,
  person: <><circle cx="12" cy="8" r="4" /><path d="M4 21c1-4 4-6 8-6s7 2 8 6" /></>,
  // A microphone on its stand.
  mic: <><rect x="9" y="3" width="6" height="11" rx="3" /><path d="M5.5 11a6.5 6.5 0 0 0 13 0M12 17.5V21M8.5 21h7" /></>,
  // A calendar's page: a calendar feed.
  calendar: <><rect x="3" y="5" width="18" height="16" rx="2" /><path d="M3 10h18M8 3v4M16 3v4M7.5 14h.01M12 14h.01M16.5 14h.01M7.5 17.5h.01M12 17.5h.01" /></>,
};

/** One card's icon, 32 by 32, hidden from a screen reader: the card's name
 *  says what it is. */
export function ConfigIcon({ icon }: { icon: CardIcon }) {
  if (icon.kind === "image") {
    return <img className="config-icon config-icon-image" src={icon.src} alt="" width={32} height={32} draggable={false} />;
  }
  if (icon.kind === "monogram") {
    return <span className="config-icon config-icon-monogram" aria-hidden="true">{icon.text}</span>;
  }
  return (
    <svg className="config-icon config-glyph" viewBox="0 0 24 24" width={32} height={32} aria-hidden="true" focusable="false">
      {PATHS[icon.key as GlyphKey] ?? PATHS.server}
    </svg>
  );
}

/** The Add card's plus. */
export function AddGlyph() {
  return (
    <svg className="config-icon config-glyph" viewBox="0 0 24 24" width={32} height={32} aria-hidden="true" focusable="false">
      <path d="M12 5v14M5 12h14" />
    </svg>
  );
}
