// The Assistant's Tasks view: every task, its schedule in words and as
// written, its zone, its next and last fire and the first line of that
// answer, and who made it. A task is paused, resumed or removed here, through
// the agent's owner whether it runs here, elsewhere or not at all; adding one
// is asked of the Assistant in conversation. Every name, instruction and
// answer is drawn as text.
import { useEffect, useState } from "react";
import * as PersistentAPI from "../../wailsjs/go/app/PersistentAPI";
import type { app } from "../../wailsjs/go/models";
import { taskOps, taskRow } from "../tasksView";
import { Button, ErrorBox, Modal } from "../ui";

export function TasksView({ agent, onClose }: { agent: app.PersistentAgentView; onClose: () => void }) {
  const [tasks, setTasks] = useState<app.PersistentTaskView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [removing, setRemoving] = useState<string | null>(null);
  useEffect(() => {
    PersistentAPI.Tasks(agent.name).then((ts) => setTasks(ts ?? [])).catch((e) => setError(String(e)));
  }, [agent.name]);
  const change = async (id: string, op: string) => {
    setBusy(true);
    setError(null);
    try {
      setTasks((await PersistentAPI.ChangeTask(agent.name, id, op)) ?? []);
      setRemoving(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal title={`${agent.displayName}'s tasks`} onClose={onClose} wide>
      <ErrorBox error={error} />
      <p className="small muted">
        Ask {agent.displayName} in conversation to add a task or change when one runs. Its heartbeat runs them while it runs
        {agent.roster.some((e) => e.id === "heartbeat" && !e.on) ? ", and its heartbeat is off now: no task runs until you switch it on" : ""}.
      </p>
      {tasks === null && !error && <p className="small muted">Reading its tasks…</p>}
      {tasks !== null && tasks.length === 0 && <p className="small muted">No tasks yet.</p>}
      <ul className="tasks-list">
        {(tasks ?? []).map((t) => {
          const r = taskRow(t);
          return (
            <li key={r.id} className={`tasks-row${r.paused ? " paused" : ""}`}>
              <div className="tasks-row-head">
                <strong>{r.name}</strong>
                <span className="tasks-row-ops">
                  {taskOps(t).map((o) =>
                    o.op === "remove" ? (
                      <Button key={o.op} variant="ghost" className="small" disabled={busy} onClick={() => setRemoving(r.id)}>{o.label}</Button>
                    ) : (
                      <Button key={o.op} variant="ghost" className="small" disabled={busy} onClick={() => void change(r.id, o.op)}>{o.label}</Button>
                    ),
                  )}
                </span>
              </div>
              <div className="small">
                {r.schedule} · <span className="mono">{r.cron}</span> · {r.zone}
              </div>
              <div className="small muted">Next: {r.next} · Last: {r.last} · {r.madeBy}</div>
              {r.answer && <div className="small tasks-row-answer">{r.answer}</div>}
              <details className="small">
                <summary>What it does</summary>
                <p className="tasks-row-instruction">{r.instruction}</p>
              </details>
              {removing === r.id && (
                <div className="config-card-confirm small" role="group" aria-label="Remove">
                  <span>Remove the task {r.name}?</span>
                  <Button variant="danger" className="small" disabled={busy} onClick={() => void change(r.id, "remove")}>Remove</Button>
                  <Button variant="ghost" className="small" onClick={() => setRemoving(null)}>Cancel</Button>
                </div>
              )}
            </li>
          );
        })}
      </ul>
      <div className="row gap" style={{ marginTop: 12 }}>
        <Button variant="primary" onClick={onClose}>Done</Button>
      </div>
    </Modal>
  );
}
