// Home's agent card and the Welcome's Agents step, as logic: which agent
// the card draws, what it says before the first message, the agents the
// person can put on it, and the tiles the Welcome draws. Pure, so the pages
// and node's test runner share it.
import type { app } from "../wailsjs/go/models";
import { AGENT_STATES, CODER_AGENT, MAIN_AGENT } from "./persistentAgents.ts";

type AgentLike = Pick<app.PersistentAgentView, "name" | "state">;

/** homeAgentDrawn is the agent Home's card draws: the one the person picked
 *  while this app lists it and it is on, else Agentry Helper, which is
 *  always there to talk to. */
export function homeAgentDrawn(chosen: string, agents: AgentLike[]): string {
  if (chosen === MAIN_AGENT) return MAIN_AGENT;
  const a = agents.find((x) => x.name === chosen);
  return a && a.state !== AGENT_STATES.disabled ? a.name : MAIN_AGENT;
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

/** An agent's tile: its name, one line on what it does, and its glyph. */
export type AgentTile = { name: string; title: string; line: string; glyph: "mark" | "person" | "terminal" };

/** The tiles the Welcome's Agents step and Settings → Home draw, in the
 *  order the app lists the agents. Agentry Helper is always on. */
export const AGENT_TILES: AgentTile[] = [
  { name: MAIN_AGENT, title: "Agentry Helper", glyph: "mark",
    line: "Finds, installs, runs and schedules QuickAgents for you. Always on." },
  { name: "assistant", title: "Assistant", glyph: "person",
    line: "Your own assistant: remembers what you tell it, keeps tasks, and does all the Helper does." },
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
 *  on: Agentry Helper and every agent that is on. */
export function homeChoices(agents: AgentLike[]): string[] {
  return agents.filter((a) => a.name === MAIN_AGENT || a.state !== AGENT_STATES.disabled).map((a) => a.name);
}

/** isOn says whether an agent's tile reads as on: Agentry Helper always. */
export function isOn(a: AgentLike): boolean {
  return a.name === MAIN_AGENT || a.state !== AGENT_STATES.disabled;
}
