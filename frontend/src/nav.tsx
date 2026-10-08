import { Fragment } from "react";

// The sidebar's pages, one list shared by the app and the design harness so
// the harness shows the navigation the app has (agentry-desktop#422).
//
// Validate is a sub-item of Build, shown only while the person is in Build
// or on Validate, and Build stays lit on both: the site's sub-menu
// convention, where a section's children show only inside it
// (panel-redesign D7). Browse is a sub-item of Library the same way
// (plan desktop-home-and-library.md D3; agentry-desktop#657).
//
// Publisher (plan forks-and-publisher.md D12; agentry-desktop#705) is shown
// only while the active registry is signed in: what it lists is the
// signed-in person's own.

// The pages a sidebar item can open or stay lit on: App's Route["page"],
// spelled out here because App imports this file.
export type NavPage = "home" | "run" | "library" | "browse" | "agent" | "knowledge" | "voice" | "build" | "validate" | "publisher" | "settings";

// needsSignIn: shown only while the active registry holds a token.
type Item = { label: string; page: NavPage; also?: NavPage[]; sub?: Item[]; needsSignIn?: boolean };

const ITEMS: Item[] = [
  // The page the app opens on (plan desktop-home-and-library.md D2).
  // A run page is opened from Home's Recent runs, so Home stays lit on it.
  { label: "Home", page: "home", also: ["run"] },
  // The QuickAgents installed on this computer, with Browse beneath it for
  // finding more. An agent's page is reached from either, so Library stays
  // lit on it.
  { label: "Library", page: "library", also: ["agent"], sub: [{ label: "Browse", page: "browse" }] },
  { label: "Knowledge", page: "knowledge" },
  // The Writing Voice (plan writing-voice.md §8; agentry-desktop#510).
  { label: "Voice", page: "voice" },
  { label: "Build", page: "build", sub: [{ label: "Validate", page: "validate" }] },
  // The QuickAgents the person publishes, with the owner actions.
  { label: "Publisher", page: "publisher", needsSignIn: true },
  { label: "Settings", page: "settings" },
];

export function SidebarNav({ page, onNav, installedUpdates = 0, signedIn = false }: {
  page: NavPage;
  onNav?: (page: NavPage) => void;
  installedUpdates?: number;
  /** The active registry holds a token: Publisher is shown. */
  signedIn?: boolean;
}) {
  const button = (item: Item, className: string, lit: boolean) => (
    <button
      key={item.page}
      className={`nav-item ${className}${lit ? " active" : ""}`}
      aria-current={item.page === page ? "page" : undefined}
      onClick={() => onNav?.(item.page)}
    >
      {item.label}
      {item.page === "library" && installedUpdates > 0 && (
        <span className="badge warn nav-count" title={`${installedUpdates} installed QuickAgent${installedUpdates === 1 ? " has" : "s have"} a newer version`}>{installedUpdates}</span>
      )}
    </button>
  );
  return (
    <nav className="sidebar-nav" aria-label="Pages">
      {ITEMS.filter((item) => !item.needsSignIn || signedIn).map((item) => {
        // A parent whose sub-item is open stays lit in its colour without
        // the fill the open page carries (style.css .nav-item.parent).
        const subOpen = item.sub?.some((s) => s.page === page) ?? false;
        const lit = item.page === page || (item.also?.includes(page) ?? false) || subOpen;
        return (
          <Fragment key={item.page}>
            {button(item, subOpen ? "parent" : "", lit)}
            {item.sub && (item.page === page || subOpen) && (
              <div className="nav-sub" role="group" aria-label={`${item.label} sections`}>
                {item.sub.map((s) => button(s, "nav-sub-item", s.page === page))}
              </div>
            )}
          </Fragment>
        );
      })}
    </nav>
  );
}
