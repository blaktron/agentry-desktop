// The hints Describe it asks for beside the description, and the lines the
// Drafter reads for them: each labelled so it knows which hint it is, and a
// blank one left out. Pure, so the page and its tests share it.

/** How the agent runs, as the Describe it page asks. "persistent" is an
 *  agent that stays running. */
export type RunWay = "" | "autonomous" | "interactive" | "persistent";

export type Hints = { services: string; mode: RunWay; example: string };

export const NO_HINTS: Hints = { services: "", mode: "", example: "" };

/** The choices of "How it runs", in the order the select lists them. */
export const RUN_WAYS: { value: RunWay; label: string }[] = [
  { value: "", label: "Either, or not sure" },
  { value: "autonomous", label: "On its own, from a task" },
  { value: "interactive", label: "In a conversation with a person" },
  { value: "persistent", label: "An agent that stays running" },
];

/** What an agent that stays running is, said beside the choice. */
export const PERSISTENT_WAY_NOTE =
  "It keeps its conversation and its memory from one session to the next, and runs on this computer beside the built-in agents. " +
  "The draft lists what it may do, its persona and any QuickAgents it starts, so you can check them before you submit it.";

/** The line the Drafter reads when the person picks an agent that stays
 *  running. It uses the words the Drafter drafts a persistent agent for: it
 *  stays running, keeps its conversation, remembers between days and works
 *  on a schedule. */
export const PERSISTENT_HINT =
  "It is an agent that stays running on the person's computer, beside the built-in agents. It keeps its conversation and its memory from one session to the next, and can work on a schedule for the person. Draft it as a persistent agent.";

/** The Drafter takes one hint of at most this many bytes, its label included. */
export const MAX_HINT_BYTES = 500;

function one(s: string): string {
  return s.replace(/\s+/g, " ").trim();
}

/** The hint lines for the Drafter, in the order the page asks for them. */
export function hintLines(h: Hints): string[] {
  const out: string[] = [];
  if (one(h.services)) out.push(`The services or sites it works with: ${one(h.services)}`);
  if (h.mode === "autonomous") out.push("It runs on its own, from a task, with no conversation.");
  else if (h.mode === "interactive") out.push("It runs in a conversation with a person.");
  else if (h.mode === "persistent") out.push(PERSISTENT_HINT);
  if (one(h.example)) out.push(`An example of the output wanted: ${one(h.example)}`);
  return out;
}
