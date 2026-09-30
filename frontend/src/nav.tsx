import { Fragment } from "react";

// The sidebar's pages, one list shared by the app and the design harness so
// the harness shows the navigation the app has (agentry-desktop#422).
//
// Validate is a sub-item of Build, shown only while the person is in Build
// or on Validate, and Build stays lit on both: the site's sub-menu
// convention, where a section's children show only inside it
// (panel-redesign D7).

// The pages a sidebar item can open or stay lit on: App's Route["page"],
// spelled out here because App imports this file.
export type NavPage = "browse" | "agent" | "installed" | "knowledge" | "build" | "validate" | "settings";

type Item = { label: string; page: NavPage; also?: NavPage[]; sub?: Item[] };

const ITEMS: Item[] = [
  // An agent's page is reached from Browse, so Browse stays lit on it.
  { label: "Browse", page: "browse", also: ["agent"] },
  { label: "Installed", page: "installed" },
  { label: "Knowledge", page: "knowledge" },
  { label: "Build", page: "build", sub: [{ label: "Validate", page: "validate" }] },
  { label: "Settings", page: "settings" },
];

export function SidebarNav({ page, onNav, installedUpdates = 0 }: {
  page: NavPage;
  onNav?: (page: NavPage) => void;
  installedUpdates?: number;
}) {
  const button = (item: Item, className: string, lit: boolean) => (
    <button
      key={item.page}
      className={`nav-item ${className}${lit ? " active" : ""}`}
      aria-current={item.page === page ? "page" : undefined}
      onClick={() => onNav?.(item.page)}
    >
      {item.label}
      {item.page === "installed" && installedUpdates > 0 && (
        <span className="badge warn nav-count" title={`${installedUpdates} installed QuickAgent${installedUpdates === 1 ? " has" : "s have"} a newer version`}>{installedUpdates}</span>
      )}
    </button>
  );
  return (
    <nav className="sidebar-nav" aria-label="Pages">
      {ITEMS.map((item) => {
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
