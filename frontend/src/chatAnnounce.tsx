// What a screen reader hears from a conversation: each reply once, when it
// is complete, and the warnings and the end, never the streamed chunks of a
// reply on its way, the person's own lines, the tool calls or the runner's
// notes, and none of the conversation that was there when it was opened.
// The transcript itself stays a log a person can read back, with its own
// live announcing off.
import { useEffect, useRef, useState } from "react";
import type { TermBlock } from "./persistentAgents";

/** The texts to say for the items appended since `seen` items were said. */
export function newAnnouncement<T>(seen: number, items: T[], say: (item: T) => string | null): string {
  return items.slice(seen).map(say).filter((t): t is string => !!t && t.trim() !== "").join(" ");
}

/** Says, in a polite status line, what `say` makes of each item appended to
 *  `items` after the first render. */
export function ChatAnnouncer<T>({ items, say }: { items: T[]; say: (item: T) => string | null }) {
  const seen = useRef<number | null>(null);
  const [text, setText] = useState("");
  useEffect(() => {
    if (seen.current === null || items.length < seen.current) {
      seen.current = items.length;
      return;
    }
    const next = newAnnouncement(seen.current, items, say);
    seen.current = items.length;
    if (next) setText(next);
  }, [items, say]);
  return <p className="sr-only" role="status" data-chat-said>{text}</p>;
}

/** What a screen reader hears of a persistent agent's new block: its
 *  answers, by its name, and the warnings. */
export const sayTermBlock = (agent: string) => (b: TermBlock): string | null =>
  b.kind === "agent" ? `${agent}: ${b.text}` : b.kind === "warn" ? b.text : null;
