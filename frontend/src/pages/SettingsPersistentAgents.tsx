// Settings → Persistent Agents: each agent the app offers, with its Enable
// switch, Start or Stop, Configure and Reset. Agentry Helper (main) has none of the switch,
// Open or Configure: it is on while its Home card is, and what it is given
// is fixed. Configure is a card over the
// page with everything the agent is given: its name and personality, its
// share folder, its knowledge packs and MCP servers, and its roster's
// switches. A start, a switch or an addition that turns on what the person
// has not agreed to on this computer asks in Go's native dialog; Reset asks
// here, then natively. A folder comes from the native picker alone.
import { useEffect, useState } from "react";
import * as KnowledgeAPI from "../../wailsjs/go/app/KnowledgeAPI";
import * as PersistentAPI from "../../wailsjs/go/app/PersistentAPI";
import * as RunAPI from "../../wailsjs/go/app/RunAPI";
import type { app, run } from "../../wailsjs/go/models";
import { forgetChat, refreshAgents, startAgent, stopStart, updateAgent, useAgents } from "../agents";
import { AGENT_STATES, CODER_AGENT, CODER_WARNING, MAIN_AGENT, agentStateWords, enableOutcome, portFormOK, shareCheckWords } from "../persistentAgents";
import { Button, Card, ErrorBox, Field, Input, Modal, Notice, Textarea } from "../ui";
import { ConfigCards } from "./ConfigCards";
import { TasksView } from "./TasksView";
import { StartFailure } from "./StartFailure";

/** The longest personality the agent takes, as the CLI holds it. */
const MAX_PERSONALITY_BYTES = 32 << 10;

export default function PersistentAgentsSettings({ registry, onOpenAgent }: { registry: string; onOpenAgent: (name: string) => void }) {
  const agents = useAgents();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [configuring, setConfiguring] = useState<string | null>(null);
  const [resetting, setResetting] = useState<app.PersistentAgentView | null>(null);
  const [exported, setExported] = useState<string | null>(null);
  const [exporting, setExporting] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  // Start Agentry when you sign in: read when the page opens, as the login
  // item stands (null until read).
  const [atSignIn, setAtSignIn] = useState<boolean | null>(null);
  // Notify me when a task runs: read when the page opens (null until read).
  const [taskNotes, setTaskNotes] = useState<boolean | null>(null);

  useEffect(() => {
    void refreshAgents();
    PersistentAPI.StartsAtSignIn().then(setAtSignIn).catch((e) => { setAtSignIn(false); setError(String(e)); });
    PersistentAPI.TaskNotifications().then(setTaskNotes).catch((e) => { setTaskNotes(true); setError(String(e)); });
  }, []);

  const act = async (name: string, f: () => Promise<app.PersistentAgentView>, asked = false) => {
    setBusy(name);
    setError(null);
    setNote(null);
    try {
      const v = await f();
      updateAgent(v);
      setNote(enableOutcome(asked, v));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  };

  // A start waits for the whole start, which can take minutes; Stop, while
  // it starts, cancels it, and is not held back by the busy start.
  const start = async (name: string) => {
    setBusy(name);
    setError(null);
    setNote(null);
    setError(await startAgent(name));
    setBusy(null);
  };
  const cancelStart = async (name: string) => {
    setError(null);
    setError(await stopStart(name));
  };

  return (
    <section id="settings-part-persistent-agents" className="settings-part">
      <p className="lead">
        A persistent agent keeps its conversation and its own files between the times you talk to it, and shares one folder
        with you. It runs on this computer with your model, in its own containers, beside your QuickAgents.
      </p>
      <ErrorBox error={error} />
      {note && <p className="small" role="status">{note}</p>}
      {agents.map((a) => {
        const enabled = a.state !== AGENT_STATES.disabled;
        const running = a.state === AGENT_STATES.running;
        const isMain = a.name === MAIN_AGENT;
        const isCoder = a.name === CODER_AGENT;
        return (
          <Card key={a.name} className="persistent-agent-card">
            <div className="persistent-agent-head">
              <div>
                <h3 style={{ margin: 0 }}>{a.displayName}{a.displayName !== a.title && <span className="small muted"> · {a.title}</span>}</h3>
                <span className="small muted">
                  <span className={`agent-dot ${running ? "on" : ""}`} aria-hidden="true" /> {isMain && !enabled ? "Starts when you message it on Home" : agentStateWords(a)}
                </span>
              </div>
              {!isMain && <label className="check-row">
                <input
                  type="checkbox"
                  role="switch"
                  data-persistent-enable={a.name}
                  checked={enabled}
                  disabled={busy !== null || running || a.state === AGENT_STATES.elsewhere}
                  onChange={(e) => {
                    const on = e.target.checked;
                    void act(a.name, () => PersistentAPI.SetEnabled(a.name, on), on && isCoder);
                  }}
                />
                Enabled
              </label>}
            </div>
            {isMain && <p className="small">Its card on Home is where you talk to it; Settings → Home shows or hides the card.</p>}
            {isCoder && !enabled && <Notice tone="warn">{CODER_WARNING} Turning it on asks you first.</Notice>}
            <p className="small">{a.share && <>Shares <span className="mono">{a.share}</span> with you.</>}</p>
            <StartFailure agent={a} />
            {a.state === AGENT_STATES.elsewhere && (
              <Notice>It is running in {a.holder || "another client"}. Stop it there to start it here.</Notice>
            )}
            <div className="row gap">
              {(a.state === AGENT_STATES.stopped || (isMain && !enabled)) && <Button variant="primary" disabled={busy !== null} onClick={() => void start(a.name)}>Start</Button>}
              {a.state === AGENT_STATES.starting && <Button data-stop-start={a.name} onClick={() => void cancelStart(a.name)}>Stop</Button>}
              {running && <Button disabled={busy !== null} onClick={() => void act(a.name, () => PersistentAPI.Stop(a.name))}>Stop</Button>}
              {enabled && !isMain && <Button variant="ghost" onClick={() => onOpenAgent(a.name)}>Open</Button>}
              {!isMain && <Button variant="ghost" onClick={() => setConfiguring(a.name)}>Configure</Button>}
              <Button variant="ghost" data-persistent-reset={a.name} disabled={busy !== null || a.state === AGENT_STATES.starting || a.state === AGENT_STATES.elsewhere} onClick={() => setResetting(a)}>Reset…</Button>
            </div>
          </Card>
        );
      })}
      <Card className="persistent-agent-card">
        <label className="check-row">
          <input
            type="checkbox"
            role="switch"
            data-start-at-sign-in
            checked={!!atSignIn}
            disabled={atSignIn === null || busy !== null}
            onChange={(e) => {
              const on = e.target.checked;
              setError(null);
              PersistentAPI.SetStartsAtSignIn(on).then(setAtSignIn).catch((err) => setError(String(err)));
            }}
          />
          Start Agentry when you sign in
        </label>
        <p className="small muted">
          Agentry starts with its window hidden, and the agents set to run when Agentry starts run in the background. While
          the Assistant or the Coding agent is on, closing the window keeps Agentry running; its icon opens the window or quits.
        </p>
      </Card>
      <Card className="persistent-agent-card">
        <label className="check-row">
          <input
            type="checkbox"
            role="switch"
            data-task-notifications
            checked={!!taskNotes}
            disabled={taskNotes === null}
            onChange={(e) => {
              const on = e.target.checked;
              setError(null);
              PersistentAPI.SetTaskNotifications(on).then(setTaskNotes).catch((err) => setError(String(err)));
            }}
          />
          Notify me when a task runs
        </label>
        <p className="small muted">
          When a task the Assistant runs on its own answers, your computer shows a notification with the task's name and the
          first line of the answer, unless the Assistant's page is in front of you. Clicking it opens that page. It shows only
          while the Assistant runs in this app.
        </p>
      </Card>
      {configuring && <AgentConfigModal name={configuring} registry={registry} onClose={() => setConfiguring(null)} />}
      {resetting && (
        <Modal title={`Reset ${resetting.displayName === resetting.title ? `the ${resetting.title}` : resetting.displayName}?`} onClose={() => setResetting(null)}>
          <p>
            This puts it back to its defaults. It wipes its conversation, its own files, its name and personality, the servers
            and knowledge packs added to it, its switches, its dev server and ports, whether it starts with Agentry, and what
            you agreed to, so its next start asks you again. It goes back to its default share folder.
          </p>
          <p>The files in the share folder you chose, the receipts of what it did and a dev server's sign-in in your keychain are kept.</p>
          {resetting.state === AGENT_STATES.running && <p>It is running: Reset stops it first.</p>}
          {resetting.keepsMemory && (
            <div className="row gap">
              <p className="small">Its memory goes with its files. Export it first to keep a copy.</p>
              <Button disabled={busy !== null || exporting} busy={exporting} busyLabel="Exporting…" onClick={() => {
                setExported(null);
                setExporting(true);
                PersistentAPI.ExportMemory(resetting.name)
                  .then((v) => setExported(v.folder ? exportWords(v) : null))
                  .catch((e) => setExported(String(e)))
                  .finally(() => setExporting(false));
              }}>Export memory…</Button>
            </div>
          )}
          {exported && <p className="small" role="status">{exported}</p>}
          <div className="row gap">
            <Button
              variant="danger"
              disabled={exporting}
              onClick={() => {
                const name = resetting.name;
                setResetting(null);
                setExported(null);
                void act(name, async () => {
                  const v = await PersistentAPI.Reset(name);
                  forgetChat(name);
                  return v;
                });
              }}
            >
              Reset
            </Button>
            <Button variant="ghost" disabled={exporting} onClick={() => { setResetting(null); setExported(null); }}>Cancel</Button>
          </div>
        </Modal>
      )}
    </section>
  );
}

/** What an export of the memory says it did. */
function exportWords(v: app.PersistentExportView): string {
  const files = `${v.files.toLocaleString("en-US")} file${v.files === 1 ? "" : "s"}`;
  const left = v.skipped ? `; ${v.skipped} entr${v.skipped === 1 ? "y" : "ies"} that were neither a file nor a folder left out` : "";
  return `Its memory is copied into ${v.folder}: ${files}${left}.`;
}

/** The agent's Configuration, over the page. Each change is saved as it is
 *  made; a running agent takes it at its next turn. */
export function AgentConfigModal({ name, registry, onClose }: { name: string; registry: string; onClose: () => void }) {
  const agent = useAgents().find((a) => a.name === name) ?? null;
  const [displayName, setDisplayName] = useState("");
  const [personality, setPersonality] = useState("");
  const [packs, setPacks] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [dev, setDev] = useState<{ host: string; keyFile: string; passphrase: string; password: string }>({ host: "", keyFile: "", passphrase: "", password: "" });
  const [port, setPort] = useState<{ name: string; port: string }>({ name: "", port: "" });
  const [showTasks, setShowTasks] = useState(false);

  useEffect(() => {
    if (!agent) return;
    setDisplayName(agent.displayName === agent.title ? "" : agent.displayName);
    setPersonality(agent.personality ?? "");
    // Only when the dialog opens on an agent: an update while the person
    // types must not overwrite what they typed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agent?.name]);
  useEffect(() => {
    KnowledgeAPI.List(registry)
      // The packs kept on this computer: the ones a segment can attach.
      .then((l) => setPacks((l?.packs ?? []).filter((p) => p.local).map((p) => p.name)))
      .catch(() => setPacks([]));
  }, [registry]);

  if (!agent) return null;
  // Says whether the change was saved, so a form keeps what was typed when
  // it was not.
  const change = async (c: Partial<app.PersistentChange>, what: string): Promise<boolean> => {
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      updateAgent(await PersistentAPI.Configure(name, c as app.PersistentChange));
      setSaved(agent.state === AGENT_STATES.running ? `${what} saved. The agent takes it at its next turn.` : `${what} saved.`);
      return true;
    } catch (e) {
      setError(String(e));
      return false;
    } finally {
      setBusy(false);
    }
  };
  const call = async (f: () => Promise<app.PersistentAgentView>, what: string) => {
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      updateAgent(await f());
      setSaved(`${what} saved.`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };
  const personalityBytes = new TextEncoder().encode(personality).length;

  const isCoder = agent.name === CODER_AGENT;
  const saveDev = () => {
    // The sign-in goes to Go and is never read back: the fields are emptied.
    const sign = { keyFile: dev.keyFile, passphrase: dev.passphrase, password: dev.keyFile ? "" : dev.password } as run.HostSignIn;
    void call(() => PersistentAPI.SetDevHost(name, dev.host.trim(), sign), "The dev server").then(() =>
      setDev({ host: "", keyFile: "", passphrase: "", password: "" }));
  };

  return (
    <Modal title={`Configure the ${agent.title}`} onClose={onClose} wide>
      <ErrorBox error={error} />
      {saved && <p className="small ok" role="status">{saved}</p>}
      <label className="check-row">
        <input type="checkbox" role="switch" checked={agent.runOnStart} disabled={busy}
          onChange={(e) => void change({ runOnStart: e.target.checked }, "Run when Agentry starts")} />
        Run when Agentry starts
      </label>
      {(agent.keepsTime || agent.keepsMemory) && (
        <div className="row gap">
          {agent.keepsTime && <Button disabled={busy} onClick={() => setShowTasks(true)}>Tasks…</Button>}
          {agent.keepsMemory && (
            <Button disabled={busy} onClick={() => void (async () => {
              setBusy(true);
              setError(null);
              setSaved(null);
              try {
                const v = await PersistentAPI.ExportMemory(name);
                if (v.folder) setSaved(exportWords(v));
              } catch (e) {
                setError(String(e));
              } finally {
                setBusy(false);
              }
            })()}>Export memory…</Button>
          )}
        </div>
      )}
      {showTasks && <TasksView agent={agent} onClose={() => setShowTasks(false)} />}
      {isCoder && (
        <>
          <h3>Dev server</h3>
          {agent.devHost ? (
            <div className="row between">
              <span className="small"><span className="mono">{agent.devHost}</span>, host key <span className="mono">{agent.devHostKey}</span> pinned. Its sign-in is in your system keychain.</span>
              <Button variant="ghost" disabled={busy} onClick={() => void change({ clearDevHost: true }, "The dev server's removal")}>Remove</Button>
            </div>
          ) : (
            <p className="small muted">None. The Coding agent deploys over SSH to a server you give it here, as the account you name.</p>
          )}
          <div className="persistent-server-form">
            <Field label="Server" hint="user@host, or user@host:port.">
              <Input type="text" value={dev.host} placeholder="deploy@devbox.lan" onChange={(e) => setDev({ ...dev, host: e.target.value })} />
            </Field>
            <Field label="Key file" hint={dev.keyFile ? dev.keyFile : "A private key from the file dialog, or a password below."}>
              <Button disabled={busy} onClick={() => {
                RunAPI.ChooseHostKeyFile(dev.host.trim()).then((p) => { if (p) setDev({ ...dev, keyFile: p }); }).catch((e) => setError(String(e)));
              }}>Choose a key file…</Button>
            </Field>
            {dev.keyFile ? (
              <Field label="Passphrase" hint="Only if the key has one.">
                <Input type="password" autoComplete="off" value={dev.passphrase} onChange={(e) => setDev({ ...dev, passphrase: e.target.value })} />
              </Field>
            ) : (
              <Field label="Password">
                <Input type="password" autoComplete="off" value={dev.password} onChange={(e) => setDev({ ...dev, password: e.target.value })} />
              </Field>
            )}
            <Button disabled={busy || !dev.host.trim()} onClick={saveDev}>Check and save</Button>
            <p className="small muted">Check and save reaches the server, puts a host key it has not seen to you, and keeps the sign-in in your system keychain.</p>
          </div>
        </>
      )}
      {isCoder && (
        <>
          <h3>Ports</h3>
          {agent.ports.length === 0 && (
            <p className="small muted">None. A port you add here opens what the Coding agent serves from its sandbox, such as a dev server it started, in your browser on this computer.</p>
          )}
          {agent.ports.map((q) => (
            <div key={q.name} className="row between">
              <span className="small"><strong>{q.name}</strong> at <span className="mono">http://127.0.0.1:{q.port}</span></span>
              <Button variant="ghost" disabled={busy} onClick={() => void change({ removePort: q.name }, `The port ${q.name}'s removal`)}>Remove</Button>
            </div>
          ))}
          <div className="persistent-server-form">
            <Field label="Name" hint="Lower-case letters, digits and hyphens, such as web.">
              <Input type="text" value={port.name} maxLength={32} placeholder="web" onChange={(e) => setPort({ ...port, name: e.target.value })} />
            </Field>
            <Field label="Port" hint="The number it serves on, from 1024 to 65535. Your computer opens it at the same number.">
              <Input type="text" inputMode="numeric" value={port.port} placeholder="3000" onChange={(e) => setPort({ ...port, port: e.target.value })} />
            </Field>
            <Button disabled={busy || !portFormOK(port.name, port.port)} onClick={() =>
              void change({ addPort: { name: port.name.trim(), port: Number(port.port) } } as Partial<app.PersistentChange>, `The port ${port.name.trim()}`).then((ok) => { if (ok) setPort({ name: "", port: "" }); })}>
              Add port
            </Button>
          </div>
        </>
      )}
      {!isCoder && <h3>Name and personality</h3>}
      {!isCoder && <><Field label="Name" hint="What it goes by, in the sidebar and in conversation. Empty is its own name.">
        <Input value={displayName} maxLength={64} onChange={(e) => setDisplayName(e.target.value)} />
      </Field>
      <Field label="Personality" hint={`How it should sound and behave, in your words. ${personalityBytes.toLocaleString("en-US")} of ${MAX_PERSONALITY_BYTES.toLocaleString("en-US")} bytes.`}>
        <Textarea rows={5} value={personality} onChange={(e) => setPersonality(e.target.value)} />
      </Field>
      <div className="row gap">
        <Button
          disabled={busy || personalityBytes > MAX_PERSONALITY_BYTES}
          onClick={() => void change({ name: displayName.trim(), personality }, "The name and personality")}
        >
          Save name and personality
        </Button>
      </div></>}

      <h3>Share folder</h3>
      <p className="small">
        <span className="mono">{agent.share}</span>{agent.defaultShare && " (its default)"}. The agent reads and writes it as you
        do{shareCheckWords(isCoder, agent.malwareCheck)}.
      </p>
      <div className="row gap">
        <Button disabled={busy} onClick={() => void call(() => PersistentAPI.ChooseShare(name), "The share folder")}>Choose a folder…</Button>
        {!agent.defaultShare && <Button variant="ghost" disabled={busy} onClick={() => void change({ defaultShare: true }, "The share folder")}>Use its default</Button>}
      </div>

      <h3>Knowledge packs</h3>
      {packs === null && <p className="small muted">Reading your packs…</p>}
      {packs !== null && packs.length === 0 && agent.knowledge.length === 0 && <p className="small muted">No knowledge pack is kept on this computer yet.</p>}
      {[...new Set([...(packs ?? []), ...agent.knowledge])].map((p) => {
        const on = agent.knowledge.includes(p);
        return (
          <label key={p} className="check-row">
            <input
              type="checkbox"
              checked={on}
              disabled={busy}
              onChange={() => void change(on ? { removeKnowledge: p } : { addKnowledge: p }, `The pack ${p}`)}
            />
            {p}
          </label>
        );
      })}

      <ConfigCards agent={agent} onChanged={updateAgent} onError={setError}
        onSaved={(what) => setSaved(agent.state === AGENT_STATES.running ? `${what} saved. The agent takes it at its next turn.` : `${what} saved.`)} />
      <div className="row gap" style={{ marginTop: 16 }}>
        <Button variant="primary" onClick={onClose}>Done</Button>
      </div>
    </Modal>
  );
}
