// An agent that stays running, in the Build tab's summary and after Submit:
// the rows the summary lists for it, and when it is installed as a
// persistent agent. Pure, so the page and its tests share it. Every text
// here may be the Drafter's, and is drawn as text.
import type * as build from "../bindings/github.com/blaktron/agentry-desktop/internal/build/models";
import { persistentOnly } from "./persistentAgents.ts";

/** Whether the summary draws the persistent part: the draft supports the
 *  persistent mode. */
export function showsPersistent(s: Pick<build.Summary, "persistent">): boolean {
  return !!s.persistent;
}

/** One declared key of capabilities.persistent as a line: what it gives the
 *  agent, in the core's words, and for a list key the names it declares. */
export function capabilityLine(c: Pick<build.SummaryCapability, "words" | "names">): string {
  const names = c.names ?? [];
  const words = c.words.charAt(0).toUpperCase() + c.words.slice(1);
  return names.length > 0 ? `${words}: ${names.join(", ")}.` : `${words}.`;
}

/** The persona's line: its name, or that the agent has none. */
export function personaLine(name: string): string {
  return name.trim() ? `Takes on the persona ${name.trim()}.` : "Names no persona: it is shown under its own name.";
}

/** One subagent as the summary shows it: the QuickAgent, the versions the
 *  agent accepts and the reason it gives, which the person agrees to at
 *  install. */
export function subagentLine(s: Pick<build.SummarySubagent, "ref" | "version" | "why">): { head: string; why: string } {
  const version = s.version.trim();
  return { head: version ? `${s.ref} (version ${version})` : s.ref, why: s.why.trim() };
}

/** The line under the capabilities when the draft declares none. */
export const NO_CAPABILITIES = "It declares nothing beyond an ordinary run.";

/** The line under the subagents when the draft declares none. */
export const NO_SUBAGENTS = "None. It starts no other QuickAgent.";

/** What the summary says of an agent that stays running, near its actions. */
export function persistentActionNote(only: boolean): string {
  return only
    ? "Submit sends it to the registry. When it passes, Install as a persistent agent puts it on this computer beside the built-in agents, after one confirmation that lists everything above. It has no run of its own."
    : "Submit sends it to the registry. When it passes, Install as a persistent agent puts it on this computer beside the built-in agents, after one confirmation that lists everything above.";
}

/** Whether the local, unsigned Install (the Library, where Run starts it) is
 *  offered: not for an agent that runs only as a persistent agent, which has
 *  no run. */
export function offersLibraryInstall(modes?: string[]): boolean {
  return !persistentOnly(modes);
}

/** Whether Install as a persistent agent is offered after a submission: the
 *  draft supports the persistent mode and the pipeline published it. A
 *  submission waiting for review or rejected installs nothing. */
export function offersAgentInstall(modes: string[] | undefined, status: string | undefined): boolean {
  return (modes ?? []).includes("persistent") && status === "published";
}

/** The release a submission made, as the install names it: the exact
 *  version, so what is installed is what was submitted. */
export function submittedRef(m: Pick<build.Model, "namespace" | "name">, version: string): string {
  return `${m.namespace}/${m.name}@${version}`;
}
