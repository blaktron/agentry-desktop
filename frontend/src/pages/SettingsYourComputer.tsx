// Your computer: the person's approved applications, each approved by them
// for its MCP server, its windows, both or neither; Add from the library
// and Add another application; computer use's switches and what is kept;
// the runs using the computer now, with Pause and Stop; and the runs that
// used it. Every state comes from Go, which reads it from this computer, and
// every approval, loosened class and changed server is asked again in a
// native dialog the page cannot answer.
import { useEffect, useState } from "react";
import * as YourComputerAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/yourcomputerapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import type * as linkservice from "../../bindings/github.com/blaktron/agentry-desktop/internal/linkservice/models";
import { Button, Card, ErrorBox, Notice } from "../ui";
import { EventsOn } from "../host";
import { classWords, fromWords, recentLine, stateLine } from "../yourComputer";

export function YourComputerSettings() {
  const [view, setView] = useState<app.YourComputerView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [adding, setAdding] = useState<"" | "library" | "other">("");
  const [editing, setEditing] = useState<string | null>(null);

  const read = () => {
    YourComputerAPI.State().then(setView).catch((e) => setError(String(e)));
  };
  useEffect(() => {
    read();
    return EventsOn("yourcomputer:changed", read);
  }, []);

  async function act(f: () => Promise<unknown>): Promise<boolean> {
    setBusy(true);
    setError(null);
    try {
      await f();
      return true;
    } catch (e) {
      const msg = String(e);
      // A No in the native dialog is the person's answer, not a failure.
      if (!/not confirmed/.test(msg)) setError(msg);
      return false;
    } finally {
      setBusy(false);
      read();
    }
  }

  const errorBox = error && <ErrorBox error={error} onRetry={() => { setError(null); read(); }} />;
  const editingApp = view?.applications.find((a) => a.id === editing) ?? null;
  return (
    <>
      <section id="settings-part-applications" className="settings-part" data-your-applications>
        <h2>Applications</h2>
        <p className="muted small">
          The applications on this computer agents may use. A QuickAgent names an application by a short id, and Agentry Desktop
          uses the one of yours with that id, or the one you choose. Nothing is approved until you say so: turn on its MCP server,
          its windows, or both. The agent stays in its sandbox; Agentry Desktop starts the server as you and answers each call,
          and a call that runs code, or a tool nobody has classed, asks you first. The server runs on your computer with your files
          and your network, outside the run&apos;s rules, and the run&apos;s receipt says so.
        </p>
        {errorBox}
        {view?.listError && <Notice tone="warn">{view.listError}</Notice>}
        {!view && !error && <p className="muted small">Reading this computer…</p>}
        {view && view.applications.length === 0 && <p className="muted small">No application is on your list yet.</p>}
        {view?.applications.map((s) =>
          editing === s.id && editingApp ? (
            <ApplicationForm key={s.id} busy={busy} initial={editingApp} onCancel={() => setEditing(null)}
              onSave={async (n) => { if (await act(() => YourComputerAPI.EditApplication(s.id, n))) setEditing(null); }}
              onTest={(n) => YourComputerAPI.TestServer(n)} />
          ) : (
            <ApplicationCard key={s.id} s={s} busy={busy} act={act} onEdit={() => setEditing(s.id)} />
          ),
        )}
        <div className="row" style={{ gap: "0.5rem", flexWrap: "wrap" }}>
          <Button onClick={() => setAdding(adding === "library" ? "" : "library")} disabled={busy} data-add-library>Add from the library</Button>
          <Button onClick={() => setAdding(adding === "other" ? "" : "other")} disabled={busy} data-add-other>Add another application</Button>
          <button type="button" className="link small" onClick={read} disabled={busy}>Check again</button>
        </div>
        {adding === "library" && view && (
          <Card>
            <strong>The library</strong>
            <p className="muted small">
              Popular applications whose set-up Agentry knows. Adding one puts it on your list with nothing approved.
            </p>
            {view.library.length === 0 && <p className="muted small">Every application of the library is on your list.</p>}
            {view.library.map((p) => (
              <div key={p.id} className="small" data-library-preset={p.id} style={{ marginBottom: "0.75rem" }}>
                <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
                  <span><strong>{p.application}</strong> <span className="muted">(id <span className="mono">{p.id}</span>, server <span className="mono selectable">{p.package}</span>)</span></span>
                  <Button onClick={async () => { if (await act(() => YourComputerAPI.AddFromLibrary(p.id))) setAdding(""); }} disabled={busy}>Add</Button>
                </div>
                {p.exec.length > 0 && <p className="muted small">{p.exec.join(", ")} {p.exec.length === 1 ? "runs" : "run"} code inside {p.application}, asked first.</p>}
                <ol className="small">{p.steps.map((step) => <li key={step} className="selectable">{step}</li>)}</ol>
              </div>
            ))}
          </Card>
        )}
        {adding === "other" && (
          <ApplicationForm busy={busy} onCancel={() => setAdding("")}
            onSave={async (n, tools) => { if (await act(() => YourComputerAPI.AddApplication(n, tools))) setAdding(""); }}
            onTest={(n) => YourComputerAPI.TestServer(n)} />
        )}
        {view?.socketError && (
          <Notice tone="warn">
            A run from the <span className="mono">agentry</span> command cannot use your computer while this lasts: {view.socketError}
          </Notice>
        )}
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
            {(view.computer.support.permissions ?? []).length > 0 && (
              <div className="small" data-computer-permissions>
                {(view.computer.support.permissions ?? []).map((p) => (
                  <div key={p.key} className="row" style={{ justifyContent: "space-between", alignItems: "baseline", gap: "0.5rem", flexWrap: "wrap" }}>
                    <span className="selectable">
                      {p.name}: {p.granted ? "allowed" : "not allowed"} <span className="muted">({p.why})</span>
                    </span>
                    {!p.granted && (
                      <button type="button" className="link small" disabled={busy} onClick={() => act(() => YourComputerAPI.OpenComputerPrivacy(p.key))}>
                        Open System Settings
                      </button>
                    )}
                  </div>
                ))}
                {(view.computer.support.permissions ?? []).some((p) => !p.granted) && (
                  <p className="muted small">
                    Turning computer use on asks macOS for both. After you allow Screen &amp; System Audio Recording in System Settings,
                    quit Agentry Desktop and open it again: macOS applies it only to a new start.
                  </p>
                )}
              </div>
            )}
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

function ApplicationCard({ s, busy, act, onEdit }: {
  s: linkservice.AppState; busy: boolean; act: (f: () => Promise<unknown>) => Promise<boolean>; onEdit: () => void;
}) {
  const [showTools, setShowTools] = useState(false);
  return (
    <Card>
      <div data-application={s.id}>
        <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline", gap: "0.5rem", flexWrap: "wrap" }}>
          <span><strong>{s.application}</strong> <span className="muted small">(id <span className="mono">{s.id}</span>, {fromWords(s)})</span></span>
          {s.hasServer && <span className={s.ready ? "small" : "small warn"}>{s.ready ? "Ready" : "Not ready"}</span>}
        </div>
        <label className="check-row small">
          <input type="checkbox" role="switch" data-approve-mcp checked={s.mcp} disabled={busy || !s.hasServer}
            onChange={(e) => act(() => YourComputerAPI.SetApproval(s.id, "mcp", e.target.checked))} />
          Agents may use its MCP server{!s.hasServer && <span className="muted"> (none set up)</span>}
        </label>
        <label className="check-row small">
          <input type="checkbox" role="switch" data-approve-computer checked={s.computer} disabled={busy || s.windows.length === 0}
            onChange={(e) => act(() => YourComputerAPI.SetApproval(s.id, "computer", e.target.checked))} />
          Agents may see and use its windows{s.windows.length === 0 ? <span className="muted"> (no window identifiers)</span> : <span className="muted"> (<span className="mono selectable">{s.windows.join(", ")}</span>)</span>}
        </label>
        {s.hasServer && <p className="small selectable">{stateLine(s)}</p>}
        {s.hasServer && (
          <p className="muted small">
            Its server{s.package && <>: <span className="mono selectable">{s.package}</span></>}
            {s.launch && <>, started as <span className="mono selectable">{s.launch}</span></>}
            {s.endpoint && <>, at <span className="mono selectable">{s.endpoint}</span></>}.
            {s.egress.length > 0 && <> It is known to reach {s.egress.join(", ")} from your computer.</>}
          </p>
        )}
        {s.env.some((e) => e.note) && (
          <ul className="muted small">
            {s.env.filter((e) => e.note).map((e) => <li key={e.name}><span className="mono">{e.name}={e.value}</span>: {e.note}</li>)}
          </ul>
        )}
        {!s.ready && s.hasServer && s.steps.length > 0 && (
          <ol className="small">{s.steps.map((step) => <li key={step} className="selectable">{step}</li>)}</ol>
        )}
        {s.tools.length > 0 && (
          <button type="button" className="link small" onClick={() => setShowTools(!showTools)}>
            {showTools ? "Hide its tools" : `Its tools (${s.tools.length})`}
          </button>
        )}
        {showTools && (
          <table className="small" data-tools>
            <tbody>
              {s.tools.map((t) => (
                <tr key={t.name}>
                  <td className="mono selectable">{t.name}</td>
                  <td>
                    <select value={t.class} disabled={busy || t.withheld} aria-label={`${t.name}'s class`}
                      onChange={(e) => act(() => YourComputerAPI.SetToolClass(s.id, t.name, e.target.value))}>
                      <option value="read">read: {classWords("read")}</option>
                      <option value="act">act: {classWords("act")}</option>
                      <option value="exec">exec: {classWords("exec")}</option>
                    </select>
                    {!t.classed && !t.withheld && <span className="muted"> not classed yet, so it asks</span>}
                  </td>
                  <td>
                    <label className="check-row">
                      <input type="checkbox" checked={t.withheld} disabled={busy}
                        onChange={(e) => act(() => YourComputerAPI.SetToolWithheld(s.id, t.name, e.target.checked))} />
                      never given
                    </label>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div className="row" style={{ gap: "0.5rem" }}>
          <button type="button" className="link small" disabled={busy} onClick={onEdit}>Edit</button>
          <button type="button" className="link small danger" disabled={busy} onClick={() => act(() => YourComputerAPI.RemoveApplication(s.id))}>Remove</button>
        </div>
      </div>
    </Card>
  );
}

// splitCommand splits a command line into its program and arguments, on
// spaces, keeping what single or double quotes hold together.
export function splitCommand(line: string): string[] {
  const out: string[] = [];
  let cur = "";
  let quote: string | null = null;
  let started = false;
  for (const ch of line) {
    if (quote) {
      if (ch === quote) quote = null;
      else cur += ch;
    } else if (ch === "'" || ch === '"') {
      quote = ch;
      started = true;
    } else if (/\s/.test(ch)) {
      if (started || cur) out.push(cur);
      cur = "";
      started = false;
    } else {
      cur += ch;
      started = true;
    }
  }
  if (started || cur) out.push(cur);
  return out;
}

// parseEnv reads NAME=value lines.
export function parseEnv(text: string): Record<string, string> {
  const env: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const i = line.indexOf("=");
    if (i > 0) env[line.slice(0, i).trim()] = line.slice(i + 1);
  }
  return env;
}

function ApplicationForm({ busy, initial, onCancel, onSave, onTest }: {
  busy: boolean;
  initial?: linkservice.AppState;
  onCancel: () => void;
  onSave: (n: app.NewApplication, tools: string[]) => void;
  onTest: (n: app.NewApplication) => Promise<app.TestedServer>;
}) {
  const [name, setName] = useState(initial?.application ?? "");
  const [id, setID] = useState(initial?.id ?? "");
  const [windows, setWindows] = useState((initial?.windows ?? []).join(", "));
  const [kind, setKind] = useState<string>(initial ? (initial.hasServer ? initial.attachment ?? "" : "") : "stdio");
  const [command, setCommand] = useState(initial?.launch ? initial.launch.split(" ").filter((p) => !/^[A-Za-z_][A-Za-z0-9_]*=/.test(p)).join(" ") : "");
  const [env, setEnv] = useState((initial?.env ?? []).map((e) => `${e.name}=${e.value}`).join("\n"));
  const [endpoint, setEndpoint] = useState(initial?.endpoint ?? "http://127.0.0.1:");
  const [listener, setListener] = useState(initial?.port ? String(initial.port) : "");
  const [tested, setTested] = useState<app.TestedServer | null>(null);
  const [testing, setTesting] = useState(false);
  const [picking, setPicking] = useState<linkservice.RunningWindow[] | null>(null);
  const [pickError, setPickError] = useState<string | null>(null);

  const form = (): app.NewApplication => ({
    name, id: initial ? initial.id : id,
    windows: windows.split(",").map((w) => w.trim()).filter(Boolean),
    server: kind, command: kind === "stdio" ? splitCommand(command) : [],
    env: kind === "stdio" ? parseEnv(env) : {},
    endpoint: kind === "loopback-http" ? endpoint : "",
    listener: Number(listener) || 0,
  });
  async function test() {
    setTesting(true);
    try {
      setTested(await onTest(form()));
    } catch (e) {
      const msg = String(e);
      if (!/not confirmed/.test(msg)) setTested({ tools: [], error: msg });
    } finally {
      setTesting(false);
    }
  }
  async function pick() {
    setPickError(null);
    try {
      setPicking(await YourComputerAPI.RunningWindows());
    } catch (e) {
      setPickError(String(e));
    }
  }
  const tools = tested?.tools ?? [];
  return (
    <Card>
      <div data-application-form>
        <strong>{initial ? `Edit ${initial.application}` : "Add another application"}</strong>
        <div className="field">
          <label className="field-label">Its name</label>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="My CAD" />
        </div>
        {!initial && (
          <div className="field">
            <label className="field-label">Its id</label>
            <input value={id} onChange={(e) => setID(e.target.value)} placeholder="made from its name" className="mono" />
            <div className="field-hint muted small">What a QuickAgent names it by: lowercase letters and digits, words joined by hyphens.</div>
          </div>
        )}
        <div className="field">
          <label className="field-label">Its windows, for computer use</label>
          <input value={windows} onChange={(e) => setWindows(e.target.value)} className="mono" placeholder="none: no computer use" />
          <div className="field-hint muted small">
            The identifiers its windows carry, separated by commas: on Linux its window class or Wayland app id, on macOS its
            bundle identifier, on Windows its executable&apos;s file name.{" "}
            <button type="button" className="link small" onClick={pick}>Pick a running window</button>
          </div>
          {pickError && <p className="small warn">{pickError}</p>}
          {picking && (
            <ul className="small" data-running-windows>
              {picking.length === 0 && <li className="muted">No window is open that Agentry Desktop can read.</li>}
              {picking.map((w, i) => (
                <li key={i}>
                  <button type="button" className="link small" onClick={() => { setWindows(w.identifiers.join(", ")); if (!name) setName(w.title); setPicking(null); }}>
                    {w.title || "(no title)"}
                  </button>{" "}
                  <span className="mono muted">{w.identifiers.join(", ")}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
        <div className="field">
          <label className="field-label">Its MCP server</label>
          <select value={kind} onChange={(e) => { setKind(e.target.value); setTested(null); }}>
            <option value="">None: its windows only</option>
            <option value="stdio">A command Agentry Desktop starts</option>
            <option value="loopback-http">An address on this computer the application serves</option>
          </select>
        </div>
        {kind === "stdio" && (
          <>
            <div className="field">
              <label className="field-label">The command</label>
              <input value={command} onChange={(e) => { setCommand(e.target.value); setTested(null); }} className="mono" placeholder="npx -y my-cad-mcp@1.2.3" />
            </div>
            <div className="field">
              <label className="field-label">Its environment variables</label>
              <textarea value={env} onChange={(e) => setEnv(e.target.value)} className="mono" rows={3} placeholder="NAME=value, one per line" />
            </div>
          </>
        )}
        {kind === "loopback-http" && (
          <div className="field">
            <label className="field-label">The address</label>
            <input value={endpoint} onChange={(e) => { setEndpoint(e.target.value); setTested(null); }} className="mono" />
          </div>
        )}
        {kind !== "" && (
          <div className="field">
            <label className="field-label">The application&apos;s port, checked before a run starts</label>
            <input value={listener} onChange={(e) => setListener(e.target.value.replace(/[^0-9]/g, ""))} className="mono" placeholder="none" />
          </div>
        )}
        {kind !== "" && (
          <p className="small">
            <Button onClick={test} disabled={busy || testing}>{testing ? "Testing…" : "Test"}</Button>{" "}
            {tested?.error && <span className="warn">{tested.error}</span>}
            {tested && !tested.error && <span>It lists {tools.length} {tools.length === 1 ? "tool" : "tools"}: <span className="mono selectable">{tools.join(", ")}</span>. None is classed yet, so each asks before its calls until you class it.</span>}
          </p>
        )}
        <div className="row" style={{ gap: "0.5rem" }}>
          <Button onClick={() => onSave(form(), tools)} disabled={busy || !name.trim()}>{initial ? "Save" : "Add"}</Button>
          <button type="button" className="link small" onClick={onCancel}>Cancel</button>
        </div>
        <p className="muted small">Nothing is approved when you add it: turn on what agents may use on its card.</p>
      </div>
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
