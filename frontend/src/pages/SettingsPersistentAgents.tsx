// Settings → Persistent Agents: each agent the app offers, with its Enable
// switch, Start or Stop, Configure and Reset. Main has none of the switch,
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
import type { app, build } from "../../wailsjs/go/models";
import { forgetChat, refreshAgents, updateAgent, useAgents } from "../agents";
import { AGENT_STATES, MAIN_AGENT, agentStateWords, canAddServer, serverRowOf, type ServerForm } from "../persistentAgents";
import { Button, Card, ErrorBox, Field, Input, Modal, Notice, Select, Textarea } from "../ui";
import { RosterList } from "./AgentRoster";

/** The longest personality the agent takes, as the CLI holds it. */
const MAX_PERSONALITY_BYTES = 32 << 10;

export default function PersistentAgentsSettings({ registry, onOpenAgent }: { registry: string; onOpenAgent: (name: string) => void }) {
  const agents = useAgents();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [configuring, setConfiguring] = useState<string | null>(null);
  const [resetting, setResetting] = useState<app.PersistentAgentView | null>(null);

  useEffect(() => {
    void refreshAgents();
  }, []);

  const act = async (name: string, f: () => Promise<app.PersistentAgentView>) => {
    setBusy(name);
    setError(null);
    try {
      updateAgent(await f());
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  };

  return (
    <section id="settings-part-persistent-agents" className="settings-part">
      <p className="lead">
        A persistent agent keeps its conversation and its own files between the times you talk to it, and shares one folder
        with you. It runs on this computer with your model, in its own containers, beside your QuickAgents.
      </p>
      <ErrorBox error={error} />
      {agents.map((a) => {
        const enabled = a.state !== AGENT_STATES.disabled;
        const running = a.state === AGENT_STATES.running;
        const isMain = a.name === MAIN_AGENT;
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
                  onChange={(e) => void act(a.name, () => PersistentAPI.SetEnabled(a.name, e.target.checked))}
                />
                Enabled
              </label>}
            </div>
            {isMain && <p className="small">Its card on Home is where you talk to it; Settings → Home shows or hides the card.</p>}
            <p className="small">{a.share && <>Shares <span className="mono">{a.share}</span> with you.</>}</p>
            {a.state === AGENT_STATES.elsewhere && (
              <Notice>It is running in {a.holder || "another client"}. Stop it there to start it here.</Notice>
            )}
            <div className="row gap">
              {(a.state === AGENT_STATES.stopped || (isMain && !enabled)) && <Button variant="primary" disabled={busy !== null} onClick={() => void act(a.name, () => PersistentAPI.Start(a.name))}>Start</Button>}
              {a.state === AGENT_STATES.starting && <Button disabled>Starting…</Button>}
              {running && <Button disabled={busy !== null} onClick={() => void act(a.name, () => PersistentAPI.Stop(a.name))}>Stop</Button>}
              {enabled && !isMain && <Button variant="ghost" onClick={() => onOpenAgent(a.name)}>Open</Button>}
              {!isMain && <Button variant="ghost" onClick={() => setConfiguring(a.name)}>Configure</Button>}
              <Button variant="ghost" disabled={busy !== null || running || a.state === AGENT_STATES.elsewhere} onClick={() => setResetting(a)}>Reset</Button>
            </div>
          </Card>
        );
      })}
      {configuring && <AgentConfigModal name={configuring} registry={registry} onClose={() => setConfiguring(null)} />}
      {resetting && (
        <Modal title={`Reset ${resetting.displayName === resetting.title ? `the ${resetting.title}` : resetting.displayName}?`} onClose={() => setResetting(null)}>
          <p>
            This wipes its conversation and its own files. Its share folder and the receipts of what it did are kept, and its
            next start begins a new conversation.
          </p>
          <div className="row gap">
            <Button
              variant="danger"
              onClick={() => {
                const name = resetting.name;
                setResetting(null);
                void act(name, async () => {
                  const v = await PersistentAPI.Reset(name);
                  forgetChat(name);
                  return v;
                });
              }}
            >
              Reset
            </Button>
            <Button variant="ghost" onClick={() => setResetting(null)}>Cancel</Button>
          </div>
        </Modal>
      )}
    </section>
  );
}

/** The agent's Configuration, over the page. Each change is saved as it is
 *  made; a running agent takes it at its next turn. */
export function AgentConfigModal({ name, registry, onClose }: { name: string; registry: string; onClose: () => void }) {
  const agent = useAgents().find((a) => a.name === name) ?? null;
  const [displayName, setDisplayName] = useState("");
  const [personality, setPersonality] = useState("");
  const [packs, setPacks] = useState<string[] | null>(null);
  const [server, setServer] = useState<ServerForm>({ source: "npm", package: "", version: "", url: "", tools: "" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);

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
  const change = async (c: Partial<app.PersistentChange>, what: string) => {
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      updateAgent(await PersistentAPI.Configure(name, c as app.PersistentChange));
      setSaved(agent.state === AGENT_STATES.running ? `${what} saved. The agent takes it at its next turn.` : `${what} saved.`);
    } catch (e) {
      setError(String(e));
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
  const addServer = () => {
    void call(() => PersistentAPI.AddServer(name, serverRowOf(server) as build.MCPServerRow), "The server").then(() =>
      setServer({ source: server.source, package: "", version: "", url: "", tools: "" }));
  };

  return (
    <Modal title={`Configure the ${agent.title}`} onClose={onClose} wide>
      <ErrorBox error={error} />
      {saved && <p className="small ok" role="status">{saved}</p>}
      <h3>Name and personality</h3>
      <Field label="Name" hint="What it goes by, in the sidebar and in conversation. Empty is its own name.">
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
      </div>

      <h3>Share folder</h3>
      <p className="small">
        <span className="mono">{agent.share}</span>{agent.defaultShare && " (its default)"}. The agent reads and writes it as you
        do, and a file you add is checked for malware before it reads it.
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

      <h3>MCP servers</h3>
      {agent.servers.length === 0 && <p className="small muted">None added.</p>}
      {agent.servers.map((s) => (
        <div key={s.id} className="row between">
          <span className="mono small">{s.name}</span>
          <Button variant="ghost" disabled={busy} onClick={() => void change({ removeServer: s.id }, "The server's removal")}>Remove</Button>
        </div>
      ))}
      <div className="persistent-server-form">
        <Field label="Source">
          <Select value={server.source} onChange={(e) => setServer({ ...server, source: e.target.value })}>
            <option value="npm">npm package</option>
            <option value="pypi">PyPI package</option>
            <option value="url">Remote server (https)</option>
          </Select>
        </Field>
        {server.source === "url" ? (
          <Field label="Address"><Input value={server.url} placeholder="https://…" onChange={(e) => setServer({ ...server, url: e.target.value })} /></Field>
        ) : (
          <>
            <Field label="Package"><Input value={server.package} onChange={(e) => setServer({ ...server, package: e.target.value })} /></Field>
            <Field label="Version" hint="An exact version, as a QuickAgent pins it."><Input value={server.version} onChange={(e) => setServer({ ...server, version: e.target.value })} /></Field>
          </>
        )}
        <Field label="Tools" hint="Only these tools, comma-separated; empty is all of them."><Input value={server.tools} onChange={(e) => setServer({ ...server, tools: e.target.value })} /></Field>
        <Button disabled={busy || !canAddServer(server)} onClick={addServer}>Add server</Button>
      </div>

      <h3>What it has</h3>
      <RosterList agent={agent} onChanged={updateAgent} onError={setError} />
      <div className="row gap" style={{ marginTop: 16 }}>
        <Button variant="primary" onClick={onClose}>Done</Button>
      </div>
    </Modal>
  );
}
