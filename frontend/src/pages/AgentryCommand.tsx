// The agentry command on macOS: the bundle carries the CLI, and this card
// puts it on PATH (a link in ~/.local/bin, the Go side's cliinstall.Link)
// and says what a new terminal runs as `agentry`. Settings shows it under
// Agentry Desktop, and the first-run wizard's last step offers it. It shows
// nothing where the app does not carry the command: Windows and Linux carry
// it in their installers, and a build from a PR carries none.
import { useEffect, useState } from "react";
import * as ScheduleAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/scheduleapi";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { Button, Card, ErrorBox, Notice } from "../ui";
import { commandLines } from "../agentryCommand";

export function AgentryCommandCard({ compact = false }: { compact?: boolean }) {
  const [view, setView] = useState<app.CLICommandView | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notes, setNotes] = useState<string[]>([]);

  const read = () => ScheduleAPI.CLICommand().then(setView).catch(() => setView(null));
  useEffect(() => {
    read();
  }, []);

  if (!view?.offered) return null;

  const install = async () => {
    setBusy(true);
    setError(null);
    setNotes([]);
    try {
      const r = await ScheduleAPI.InstallCLI();
      if (r) setNotes(r.notes ?? []);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
      read();
    }
  };

  const body = (
    <>
      {!compact && <div><strong>The agentry command</strong></div>}
      {commandLines(view).map((l) => (
        <div key={l} className="muted small">{l}</div>
      ))}
      {notes.length > 0 && (
        <Notice tone="ok">
          {notes.map((n) => <div key={n}>{n}</div>)}
        </Notice>
      )}
      <ErrorBox error={error} />
      {view.label && (
        <div className="row gap">
          <Button onClick={install} busy={busy} busyLabel="Installing…">{view.label}</Button>
        </div>
      )}
    </>
  );
  return compact ? <div className="agentry-command">{body}</div> : <Card>{body}</Card>;
}
