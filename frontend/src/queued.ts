// A message the person sent while the agent was on a turn: it waits for the
// agent's next turn and is sent by itself once the turn ends. Until then the
// person can send it now (the turn in flight is stopped and the message is
// the next turn), take it back into the composer to change it, or, where the
// turn takes one, add it to the turn in flight. One message waits at a time:
// another sent meanwhile joins it. The QuickAgent chat's store
// (sessionChat.ts) and the persistent agents' store (agents.ts) both keep
// their queue by these rules.
import type { Line } from "./sessionChat";

/** QUEUED_WORDS label a queued message where it is drawn. */
export const QUEUED_WORDS = "queued for the next turn";

/** QUEUED_SAID is what a screen reader hears when a message is queued. */
export const QUEUED_SAID = "Your message is queued. It is sent when the agent finishes this turn.";

/** QUEUE_NO_STEER_NOTE says the turn in flight took no message, so the
 *  queued one waits for the next turn. */
export const QUEUE_NO_STEER_NOTE = "This turn takes no message while it runs. Yours is sent when it ends.";

/** QUEUE_NOT_SENT_NOTE says a queued message could not be sent when the turn
 *  ended, so it waits for the person rather than being tried again. */
export const QUEUE_NOT_SENT_NOTE = "Your queued message was not sent. Send it now, or edit it.";

/** queueJoin is the queue once `then` follows `first`: one message, each
 *  part on a line of its own. Either may be absent. */
export function queueJoin(first: string | null, then: string | null): string | null {
  const parts = [first, then].map((s) => (s ?? "").trim()).filter((s) => s !== "");
  return parts.length ? parts.join("\n") : null;
}

/** editText is what the composer holds once the queued message is taken back
 *  into it: the queued text first, then whatever the person had begun to
 *  type. A single-line composer gets the lines joined by spaces, since its
 *  field would drop the line breaks. */
export function editText(queued: string, draft: string, multiline: boolean): string {
  const text = queueJoin(queued, draft) ?? "";
  return multiline ? text : text.replace(/\s*\n\s*/g, " ");
}

/** personLines counts the person's lines in a chat, so a send that failed can
 *  tell whether its turn was drawn (and ran) or never reached the agent. */
export function personLines(lines: Line[]): number {
  let n = 0;
  for (const l of lines) if (l.kind === "user" && !l.synthetic && !l.task && !l.upkeep) n++;
  return n;
}
