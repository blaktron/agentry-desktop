// The sidebar's items as a tree of rows, worked out from the page that is
// open. Pure, so the node tests can import it; nav.tsx draws the rows.
//
// From the top: Home; Agents, which opens the page that manages the
// persistent agents and always lists the enabled ones beneath it; Scheduled;
// Build, a section that opens no page, with QuickAgent (and Validate beneath
// it), Voice and Knowledge; Library, which opens what is installed, with
// Installed, Browse and Publisher beneath it; Plugins, a landing of the
// optional features, with Meeting notes (where this copy can record) and
// Computer use beneath it; and Settings, which opens its landing and lists
// every section beneath it.
//
// A group's rows show only while its landing or one of its pages is open,
// the site's sub-menu convention, with two exceptions: the agents under
// Agents are always listed, so each is one click away, and Build's are
// shown while Build is expanded, which the person decides and which opening
// one of its pages does too.

// The pages a sidebar row can open or stay lit on: App's Route["page"],
// spelled out here because App imports this file.
export type NavPage = "home" | "agents" | "scheduled" | "run" | "library" | "browse" | "agent" | "persistent" | "plugins" | "meetings" | "computer" | "knowledge" | "voice" | "build" | "validate" | "publisher" | "settings" | "notifications";

/** A persistent agent's entry, under Agents: the Assistant by the name the
 *  person gave it, with a dot for running or stopped. */
export type NavAgent = { id: string; label: string; running: boolean };

/** A Settings section's row: its id and its title on the landing. */
export type NavSection = { id: string; title: string };

// What a row does when clicked.
export type NavTarget =
  | { kind: "page"; page: NavPage }
  | { kind: "agent"; id: string }
  | { kind: "section"; id: string }
  // Build: expands or collapses its rows.
  | { kind: "toggle"; id: "build" };

export type NavRow = {
  key: string;
  label: string;
  target: NavTarget;
  /** In its colour: the open page is this row's, or is beneath it. */
  lit: boolean;
  /** The open page is this row's own (aria-current). */
  current: boolean;
  /** Lit for a page beneath it, so drawn without the open page's fill. */
  parent: boolean;
  /** A toggle's state (aria-expanded). */
  expanded?: boolean;
  /** An agent row's running dot. */
  running?: boolean;
  /** The rows shown beneath this one, indented on a rule. */
  children?: NavRow[];
  /** What the group of children is called to a screen reader. */
  childrenLabel?: string;
};

type Item = { label: string; page: NavPage; key?: string; also?: NavPage[]; sub?: Item[]; needsSignIn?: boolean; needsMeetings?: boolean };

// A run page is opened from Home's Recent runs, so Home stays lit on it.
const HOME: Item = { label: "Home", page: "home", also: ["run"] };
const SCHEDULED: Item = { label: "Scheduled", page: "scheduled" };
// Build's rows. QuickAgent is the page that builds one, with Validate
// beneath it.
const BUILD: Item[] = [
  { label: "QuickAgent", page: "build", sub: [{ label: "Validate", page: "validate" }] },
  { label: "Voice", page: "voice" },
  { label: "Knowledge", page: "knowledge" },
];
// Library opens Installed, its first row. The QuickAgents the person
// publishes are shown only while the active registry is signed in: what
// Publisher lists is the signed-in person's own. A QuickAgent's page keeps
// Library lit, without its rows.
const LIBRARY: Item = {
  label: "Library", page: "library", also: ["agent"],
  sub: [
    { label: "Installed", page: "library", key: "installed" },
    { label: "Browse", page: "browse" },
    { label: "Publisher", page: "publisher", needsSignIn: true },
  ],
};
// Plugins opens its landing; Meeting notes is listed only where this copy
// records meetings.
const PLUGINS: Item = {
  label: "Plugins", page: "plugins",
  sub: [
    { label: "Meeting notes", page: "meetings", needsMeetings: true },
    { label: "Computer use", page: "computer" },
  ],
};

/** The pages Build's rows open, its own included: one of them open keeps
 *  Build lit and expands it. */
export function inBuild(page: NavPage): boolean {
  return BUILD.some((i) => i.page === page || (i.sub?.some((s) => s.page === page) ?? false));
}

export type NavState = {
  page: NavPage;
  /** The active registry holds a token: Publisher is shown. */
  signedIn?: boolean;
  agents?: NavAgent[];
  /** The persistent agent whose page is open. */
  agent?: string;
  /** This copy records meetings here: Meeting notes is shown. */
  meetings?: boolean;
  /** Settings' sections, in the landing's order. */
  sections?: NavSection[];
  /** The Settings section open; none for the landing. */
  section?: string;
  /** Whether Build's rows are shown. */
  buildOpen: boolean;
};

type Gates = { signedIn: boolean; meetings: boolean };

function itemRow(item: Item, page: NavPage, g: Gates): NavRow {
  const sub = (item.sub ?? []).filter((s) => (!s.needsSignIn || g.signedIn) && (!s.needsMeetings || g.meetings));
  // A sub-item that opens the item's own page (Library's Installed) is the
  // current one there, with the item lit above it.
  const subOpen = sub.some((s) => s.page === page);
  const own = item.page === page;
  const row: NavRow = {
    key: item.key ?? item.page, label: item.label, target: { kind: "page", page: item.page },
    lit: own || (item.also?.includes(page) ?? false) || subOpen,
    current: own && !subOpen, parent: subOpen,
  };
  if (sub.length && (own || subOpen)) {
    row.children = sub.map((s) => itemRow(s, page, g));
    row.childrenLabel = `${item.label} sections`;
  }
  return row;
}

/** The sidebar's rows, top to bottom. */
export function navRows(s: NavState): NavRow[] {
  const g: Gates = { signedIn: !!s.signedIn, meetings: !!s.meetings };
  const rows: NavRow[] = [itemRow(HOME, s.page, g)];

  const agentOpen = s.page === "persistent" && (s.agents ?? []).some((a) => a.id === s.agent);
  const agents: NavRow = {
    key: "agents", label: "Agents", target: { kind: "page", page: "agents" },
    lit: s.page === "agents" || s.page === "persistent", current: s.page === "agents", parent: agentOpen,
  };
  if (s.agents?.length) {
    agents.children = s.agents.map((a) => {
      const open = s.page === "persistent" && s.agent === a.id;
      return { key: `agent:${a.id}`, label: a.label, target: { kind: "agent", id: a.id }, lit: open, current: open, parent: false, running: a.running };
    });
    agents.childrenLabel = "Agents";
  }
  rows.push(agents);

  rows.push(itemRow(SCHEDULED, s.page, g));

  const buildLit = inBuild(s.page);
  const build: NavRow = {
    key: "build", label: "Build", target: { kind: "toggle", id: "build" },
    lit: buildLit, current: false, parent: buildLit, expanded: s.buildOpen,
  };
  if (s.buildOpen) {
    build.children = BUILD.map((i) => itemRow(i, s.page, g));
    build.childrenLabel = "Build sections";
  }
  rows.push(build);

  rows.push(itemRow(LIBRARY, s.page, g));
  rows.push(itemRow(PLUGINS, s.page, g));

  const inSettings = s.page === "settings";
  const sectionOpen = inSettings && !!s.section;
  const settings: NavRow = {
    key: "settings", label: "Settings", target: { kind: "page", page: "settings" },
    lit: inSettings, current: inSettings && !s.section, parent: sectionOpen,
  };
  if (inSettings && s.sections?.length) {
    settings.children = s.sections.map((x) => {
      const open = sectionOpen && s.section === x.id;
      return { key: `section:${x.id}`, label: x.title, target: { kind: "section", id: x.id }, lit: open, current: open, parent: false };
    });
    settings.childrenLabel = "Settings sections";
  }
  rows.push(settings);
  return rows;
}
