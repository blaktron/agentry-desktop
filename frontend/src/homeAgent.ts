// Home's agent card and the Welcome's Agents step, as logic: which agent
// the card draws, what it says before the first message, the agents the
// person can put on it, and the tiles the Welcome draws. Pure, so the pages
// and node's test runner share it.
import type { app } from "../wailsjs/go/models";
import { AGENT_STATES, ASSISTANT_AGENT, CODER_AGENT, MAIN_AGENT } from "./persistentAgents.ts";

type AgentLike = Pick<app.PersistentAgentView, "name" | "state">;

/** homeAgentDrawn is the agent Home's card draws: the one the person picked
 *  while this app lists it and it is on, else the Assistant while it is on,
 *  else Agentry Helper while it is on. null is no card: none of them is on,
 *  or the agents are not read yet. */
export function homeAgentDrawn(chosen: string, agents: AgentLike[]): string | null {
  for (const name of [chosen, ASSISTANT_AGENT, MAIN_AGENT]) {
    const a = agents.find((x) => x.name === name);
    if (a && isOn(a)) return a.name;
  }
  return null;
}

/** helperSuperseded says Agentry Helper cannot be turned on: the Assistant,
 *  which does everything the Helper does, is on. */
export function helperSuperseded(agents: AgentLike[]): boolean {
  return agents.some((a) => a.name === ASSISTANT_AGENT && isOn(a));
}

/** The last part of a share folder's path, the name the card says. */
function folderName(path: string): string {
  const parts = path.split(/[\\/]+/).filter(Boolean);
  return parts.length ? parts[parts.length - 1] : "";
}

/** agentCardHint is the line the card shows before the first message: what
 *  to ask this agent, and the share folder its files come from. */
export function agentCardHint(a: Pick<app.PersistentAgentView, "name" | "displayName" | "share">): string {
  const share = folderName(a.share ?? "");
  const where = share ? ` in your ${share} folder` : "";
  switch (a.name) {
    case MAIN_AGENT:
      return `Ask ${a.displayName} to find a QuickAgent, run one on a file${where}, or schedule one.`;
    case CODER_AGENT:
      return `Ask ${a.displayName} to build, run or fix code${where}.`;
    default:
      return `Ask ${a.displayName} anything, or to find, run or schedule a QuickAgent on a file${where}.`;
  }
}

/** An agent's tile: its name, one line on what it does, and its glyph; the
 *  Assistant's carries a Recommended mark. */
export type AgentTile = { name: string; title: string; line: string; glyph: "mark" | "person" | "terminal"; recommended?: boolean };

/** The line on Agentry Helper's tile while the Assistant is on. */
export const HELPER_SUPERSEDED_LINE = "Off while the Assistant is on: the Assistant does everything the Helper does.";

/** The tiles the Welcome's Agents step and Settings → Home draw, in the
 *  order the app lists the agents. Agentry Helper is the lite personal agent
 *  and the Assistant the full one: one of the two is on at a time. */
export const AGENT_TILES: AgentTile[] = [
  { name: MAIN_AGENT, title: "Agentry Helper", glyph: "mark",
    line: "Lite personal agent that works only with Agentry: finds, installs, runs and schedules QuickAgents for you." },
  { name: ASSISTANT_AGENT, title: "Assistant", glyph: "person", recommended: true,
    line: "Your personal assistant: everything the Helper does, plus a memory, tasks, and your mail, calendars and files. Turns the Helper off." },
  { name: CODER_AGENT, title: "Coding agent", glyph: "terminal",
    line: "Builds and runs software in your codingshare folder, with the web and a dev server." },
];

/** tilesFor are the tiles of the agents this app lists, in AGENT_TILES'
 *  order, each with the agent's own name for it where the person gave one. */
export function tilesFor(agents: Pick<app.PersistentAgentView, "name" | "displayName">[]): AgentTile[] {
  return AGENT_TILES.flatMap((t) => {
    const a = agents.find((x) => x.name === t.name);
    return a ? [{ ...t, title: a.displayName || t.title }] : [];
  });
}

/** homeChoices are the agents the card can be given without turning one
 *  on: every agent that is on. */
export function homeChoices(agents: AgentLike[]): string[] {
  return agents.filter(isOn).map((a) => a.name);
}

/** isOn says whether an agent is on. */
export function isOn(a: AgentLike): boolean {
  return a.state !== AGENT_STATES.disabled;
}
