import { useEffect, useState } from "react";
import { inBuild, navRows, type NavAgent, type NavPage, type NavRow, type NavSection } from "./navTree";

// The sidebar, one component shared by the app and the design harness so
// the harness shows the navigation the app has. Which rows there are, and
// which is lit, is navTree.ts's.

export type { NavAgent, NavPage, NavSection };

export function SidebarNav({ page, onNav, installedUpdates = 0, signedIn = false, agents = [], agent, onOpenAgent, meetings = false, sections = [], section, onOpenSection }: {
  page: NavPage;
  onNav?: (page: NavPage) => void;
  installedUpdates?: number;
  /** The active registry holds a token: Publisher is shown. */
  signedIn?: boolean;
  /** The enabled persistent agents; agent the one whose page is open. */
  agents?: NavAgent[];
  agent?: string;
  onOpenAgent?: (id: string) => void;
  /** This copy records meetings here: Meeting notes is shown. */
  meetings?: boolean;
  /** Settings' sections, listed beneath Settings while it is open; section
   *  the one open. */
  sections?: NavSection[];
  section?: string;
  onOpenSection?: (id: string) => void;
}) {
  // Build is expanded by the person, and by opening one of its pages; it
  // stays as they left it while they move around the rest of the app.
  const [buildOpen, setBuildOpen] = useState(() => inBuild(page));
  useEffect(() => { if (inBuild(page)) setBuildOpen(true); }, [page]);

  const click = (r: NavRow) => {
    const t = r.target;
    if (t.kind === "page") onNav?.(t.page);
    else if (t.kind === "agent") onOpenAgent?.(t.id);
    else if (t.kind === "section") onOpenSection?.(t.id);
    else setBuildOpen((o) => !o);
  };

  const draw = (r: NavRow, depth: number) => {
    const agentRow = r.target.kind === "agent";
    const toggle = r.target.kind === "toggle";
    const className = [
      "nav-item",
      depth > 0 ? "nav-sub-item" : "",
      agentRow ? "nav-agent" : "",
      toggle ? "nav-group" : "",
      r.lit ? "active" : "",
      r.parent ? "parent" : "",
    ].filter(Boolean).join(" ");
    return (
      <div key={r.key} className="nav-row">
        <button
          className={className}
          aria-current={r.current ? "page" : undefined}
          aria-expanded={toggle ? r.expanded : undefined}
          title={agentRow ? `${r.label}: ${r.running ? "running" : "stopped"}` : undefined}
          data-nav={r.key}
          onClick={() => click(r)}
        >
          {agentRow && <span className={`agent-dot${r.running ? " on" : ""}`} aria-hidden="true" />}
          {agentRow ? <span className="nav-agent-label">{r.label}</span> : r.label}
          {toggle && <span className={`nav-chevron${r.expanded ? " open" : ""}`} aria-hidden="true" />}
          {r.key === "library" && installedUpdates > 0 && (
            <span className="badge warn nav-count" title={`${installedUpdates} installed QuickAgent${installedUpdates === 1 ? " has" : "s have"} a newer version`}>{installedUpdates}</span>
          )}
        </button>
        {r.children && (
          <div className="nav-sub" role="group" aria-label={r.childrenLabel}>
            {r.children.map((c) => draw(c, depth + 1))}
          </div>
        )}
      </div>
    );
  };

  const rows = navRows({ page, signedIn, agents, agent, meetings, sections, section, buildOpen });
  return (
    <nav className="sidebar-nav" aria-label="Pages">
      {rows.map((r) => draw(r, 0))}
    </nav>
  );
}
