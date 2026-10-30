// The views a persistent agent's page offers in its View menu, as logic.
// Go keeps the choice for every agent (PersistentAPI.PageView, SetPageView;
// desktop.yaml's agentView key) and its settings.AgentViews is the same list.

export type AgentViewId = "terminal" | "compact" | "chat" | "focus" | "workspace";

export type AgentViewChoice = {
  id: AgentViewId;
  title: string;
  line: string;
  /** The chat in a terminal's manner, with a ">" prompt; otherwise in bubbles. */
  term: boolean;
  /** The usage and the roster beside the chat; otherwise behind the header. */
  panes: boolean;
};

export const AGENT_VIEWS: AgentViewChoice[] = [
  { id: "terminal", title: "Terminal", line: "The chat in a terminal's manner, with the usage and what it has beside it.", term: true, panes: true },
  { id: "compact", title: "Compact", line: "The terminal, denser: smaller lines closer together and the usage in the header.", term: true, panes: true },
  { id: "chat", title: "Chat", line: "The app's own chat, in bubbles, with the usage and what it has beside it.", term: false, panes: true },
  { id: "focus", title: "Focus", line: "The conversation alone in a reading column; what it has opens from the header.", term: false, panes: false },
  { id: "workspace", title: "Workspace", line: "The chat beside a wide column of everything it has, its additions and its tasks.", term: false, panes: true },
];

export const DEFAULT_AGENT_VIEW: AgentViewId = "terminal";

export const isAgentView = (v: unknown): v is AgentViewId => typeof v === "string" && AGENT_VIEWS.some((a) => a.id === v);

/** The view by its id; a name this app lacks is Terminal. */
export function agentView(id: string): AgentViewChoice {
  return AGENT_VIEWS.find((a) => a.id === id) ?? AGENT_VIEWS[0];
}

// A usage value that says nothing worth a place in the header's one line.
const SAYS_NOTHING = new Set(["—", "not running", "not reported", "unknown", "none yet"]);

/** The header's one-line usage, for the views without a usage pane: the
 *  rows' values that say something, by label, in their order. */
export function usageLine(rows: { label: string; value: string }[], labels: string[]): string {
  return labels
    .map((l) => rows.find((r) => r.label === l))
    .filter((r): r is { label: string; value: string } => !!r && !SAYS_NOTHING.has(r.value))
    .map((r) => `${r.label.toLowerCase()} ${r.value}`)
    .join(" · ");
}
