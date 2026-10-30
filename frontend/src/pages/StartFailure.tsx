// How an agent's last start failed, on its page and its Settings card: the
// step it stopped in, the error, the reason and the log, drawn as text.
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { startFailureLines } from "../persistentAgents";
import { Notice } from "../ui";

export function StartFailure({ agent }: { agent: Pick<app.PersistentAgentView, "state" | "failure"> }) {
  const failure = startFailureLines(agent);
  if (!failure) return null;
  return (
    <Notice tone="warn">
      <strong>{failure.title}</strong>
      {failure.lines.map((l, i) => <div key={i} className="small">{l}</div>)}
    </Notice>
  );
}
