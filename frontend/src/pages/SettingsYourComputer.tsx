// Settings → Your computer: the applications a QuickAgent may use on this
// computer through the MCP servers installed for them, each with its state
// here and the step to take; the per-application script-screening switch, whose
// turning off Go asks about again in a native dialog; the runs using the
// computer now, with Pause and Stop; and the runs that used it. Every
// state comes from Go, which reads it from this computer.
import { useEffect, useState } from "react";
import * as YourComputerAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/yourcomputerapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import type * as linkservice from "../../bindings/github.com/blaktron/agentry-desktop/internal/linkservice/models";
import { Button, Card, ErrorBox, Notice } from "../ui";
import { EventsOn } from "../host";
import { classWords, execCount, recentLine, stateLine } from "../yourComputer";

export function YourComputerSettings() {
  const [view, setView] = useState<app.YourComputerView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const read = () => {
    YourComputerAPI.State().then(setView).catch((e) => setError(String(e)));
  };
  useEffect(() => {
    read();
    return EventsOn("yourcomputer:changed", read);
  }, []);

  async function act(f: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await f();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
      read();
    }
  }

  const errorBox = error && <ErrorBox error={error} onRetry={() => { setError(null); read(); }} />;
  return (
    <>
      <section id="settings-part-applications" className="settings-part">
        <h2>Applications</h2>
        <p className="muted small">
          A QuickAgent that says it uses an application on your computer reaches it only through Agentry Desktop, while
          Agentry Desktop is running: the agent stays in its sandbox, and Agentry Desktop starts the MCP server Agentry
          vetted for that application, as you, and answers each of the agent&apos;s calls. A call that runs code inside the
          application asks you first. The server runs on your computer with your files and your network, outside the
          run&apos;s rules, and the run&apos;s receipt says so.
        </p>
        {errorBox}
        {!view && !error && <p className="muted small">Reading this computer…</p>}
        {view?.applications.map((s) => <ApplicationCard key={s.id} s={s} busy={busy} onScreening={(on) => act(() => YourComputerAPI.SetScreening(s.id, on))} />)}
        {view?.socketError && (
          <Notice tone="warn">
            A run from the <span className="mono">agentry</span> command cannot use your computer while this lasts: {view.socketError}
          </Notice>
        )}
        <button type="button" className="link small" onClick={read} disabled={busy}>Check again</button>
      </section>
      <section id="settings-part-computer-use" className="settings-part" data-computer-use>
        <h2>Computer use</h2>
        <p className="muted small">
          A QuickAgent that says it does computer use sees and uses the windows of the applications it names, on this computer,
          while it runs: it takes screenshots of those windows and clicks and types in them. It never sees or uses another window
          unless it asks for your whole screen and your policy, Unsafe Mode or the run&apos;s <span className="mono">--allow-screen</span>{" "}
          allows that. While it works, Agentry Desktop shows it in the tray and in a banner with Pause and Stop
          {view ? <>, and <span className="mono">{view.computer.stopKey}</span> stops it</> : null}.
        </p>
        {view && (
          <>
            <p className="small selectable" data-computer-support>
              On this computer: {view.computer.support.session}. {view.computer.support.ready ? view.computer.support.holds : view.computer.support.problem}
            </p>
            <label className="check-row">
              <input type="checkbox" role="switch" data-computer-use-on checked={view.computer.on} disabled={busy}
                onChange={(e) => act(() => YourComputerAPI.SetComputerUse(e.target.checked))} />
              Let QuickAgents do computer use on this computer
            </label>
            <label className="check-row">
              <input type="checkbox" role="switch" data-computer-use-keep checked={view.computer.keep} disabled={busy}
                onChange={(e) => act(() => YourComputerAPI.SetKeepWhatAgentSaw(e.target.checked))} />
              Keep what the agent saw: save each screenshot in the run&apos;s folder, under <span className="mono">reports/desktop</span>
            </label>
            <p className="muted small">
              Turning computer use on is asked again in a window of its own. Your desktop may also ask, the first time a run starts,
              which window to share and whether to allow control of the pointer and the keyboard; it remembers your answer.{" "}
              <button type="button" className="link small" disabled={busy} onClick={() => act(() => YourComputerAPI.ForgetComputerChoices())}>
                Forget those answers
              </button>
            </p>
          </>
        )}
      </section>
      <section id="settings-part-using" className="settings-part">
        <h2>Using your computer now</h2>
        {errorBox}
        {view && view.using.length === 0 && <p className="muted small">No run is using your computer.</p>}
        {view?.using.map((b) => (
          <Card key={b.run}>
            <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline", gap: "0.5rem", flexWrap: "wrap" }}>
              <span className="selectable">
                <span className="mono">{b.agent || b.ref || b.run}</span> is using {usingWords(b)}
                {b.state === "paused" ? " (paused)" : b.state === "stopped" ? " (stopped)" : ""}
                {b.execForRun ? "; every exec call allowed for this run" : ""}
              </span>
              <span className="row" style={{ gap: "0.5rem" }}>
                {b.state === "active" && <Button onClick={() => act(() => YourComputerAPI.Pause(b.run))} disabled={busy}>Pause</Button>}
                {b.state === "paused" && <Button onClick={() => act(() => YourComputerAPI.Resume(b.run))} disabled={busy}>Resume</Button>}
                {b.state !== "stopped" && <Button variant="danger" onClick={() => act(() => YourComputerAPI.Stop(b.run))} disabled={busy}>Stop</Button>}
              </span>
            </div>
            <p className="muted small" style={{ marginBottom: 0 }}>
              Pause refuses its calls until you resume. Stop ends the calls in flight and the application&apos;s server, and
              refuses every later call of the run; the run goes on without your computer.
            </p>
          </Card>
        ))}
      </section>
      <section id="settings-part-recent-computer" className="settings-part">
        <h2>Runs that used your computer</h2>
        {view && view.recent.length === 0 && <p className="muted small">None yet.</p>}
        {view && view.recent.length > 0 && (
          <ul className="small">
            {view.recent.map((r) => (
              <li key={`${r.run}|${r.started}`} className="selectable">
                <span className="mono">{r.agent || r.ref || r.run}</span>, {new Date(r.started).toLocaleString()}: {recentLine(r)}.
              </li>
            ))}
          </ul>
        )}
        <p className="muted small">
          Each call is a line of the run&apos;s own <span className="mono">receipts/desktop-&lt;application&gt;.log</span> in its
          run folder; this list keeps only the counts.
        </p>
      </section>
    </>
  );
}

function ApplicationCard({ s, busy, onScreening }: { s: linkservice.AppState; busy: boolean; onScreening: (on: boolean) => void }) {
  const exec = execCount(s);
  return (
    <Card>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
        <strong>{s.application}</strong>
        <span className={s.ready ? "small" : "small warn"}>{s.ready ? "Ready" : s.offered ? "Not ready" : "Not offered here"}</span>
      </div>
      <p className="small">{stateLine(s)}</p>
      <p className="muted small">
        Its server: <span className="mono selectable">{s.package}</span>
        {s.launch && <>, started as <span className="mono selectable">{s.launch}</span></>}
        {s.endpoint && <>, at <span className="mono selectable">{s.endpoint}</span></>}.{" "}
        {s.tools.length} tools{exec > 0 ? `, ${exec} of which ${exec === 1 ? "runs" : "run"} code inside ${s.application} and ${exec === 1 ? "asks" : "ask"} you first` : ""}:{" "}
        {s.tools.map((t) => `${t.name} (${classWords(t.class)})`).join(", ")}.
        {s.egress.length > 0 && <> It is known to reach {s.egress.join(", ")} from your computer.</>}
      </p>
      {!s.ready && s.steps.length > 0 && (
        <ol className="small">
          {s.steps.map((step) => <li key={step} className="selectable">{step}</li>)}
        </ol>
      )}
      {s.screeningVariable && (
        <label className="row small" style={{ gap: "0.5rem", alignItems: "center" }}>
          <input type="checkbox" checked={!s.screeningOff} disabled={busy} onChange={(e) => onScreening(e.target.checked)} />
          <span>
            Script screening: the server screens each script for file, process, network and persistence calls before it runs
            (<span className="mono">{s.screeningVariable}=1</span>). Turning it off asks first, and applies from the next run.
          </span>
        </label>
      )}
    </Card>
  );
}

// usingWords is what a bound run uses: its applications through their
// servers, and its computer use.
export function usingWords(b: linkservice.Bound): string {
  const parts: string[] = [];
  if (b.applications?.length) parts.push(b.applications.join(", "));
  if (b.computerUse === "screen") parts.push("your whole screen");
  else if (b.computerUse === "app") parts.push(`computer use of ${b.computerApps?.length ? b.computerApps.join(", ") + "'s windows" : "the window you picked"}`);
  return parts.join(" and ") || "your computer";
}
