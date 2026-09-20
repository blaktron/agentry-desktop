// Build (desktop D-013): author a BriefAgent as a form, save it as a real
// workspace (the same files `agentry init` writes), validate it with the
// registry's own scanners, and publish it through the full pipeline. The
// form is an editor for agent.yaml — the manifest stays the contract, and
// nothing here bypasses a publication gate.
import { useEffect, useRef, useState } from "react";
import * as BuildAPI from "../../wailsjs/go/main/BuildAPI";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import { ChooseDirectory } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { build, validate } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Notice, PageHeader, Sev, Spinner, Textarea } from "../ui";

const DATA_ACCESS = ["email", "calendar", "files", "tickets", "code", "messages", "payments", "other"];

/** Comma-separated text input bound to a string array. */
function ListInput({
  value, onChange, placeholder, mono = true,
}: {
  value: string[];
  onChange: (v: string[]) => void;
  placeholder?: string;
  mono?: boolean;
}) {
  return (
    <input
      type="text"
      className={mono ? "mono" : ""}
      style={{ width: "100%" }}
      placeholder={placeholder}
      value={value.join(", ")}
      onChange={(e) => onChange(e.target.value.split(",").map((s) => s.trim()).filter(Boolean))}
    />
  );
}

export default function Build({ registry }: { registry: string }) {
  const [dir, setDir] = useState("");
  const [model, setModel] = useState<build.Model | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [result, setResult] = useState<validate.Result | null>(null);
  const [pubResult, setPubResult] = useState<build.PublishResult | null>(null);
  const [visibility, setVisibility] = useState("private");
  const [log, setLog] = useState<string[]>([]);
  const logRef = useRef<HTMLPreElement | null>(null);

  useEffect(() => {
    BuildAPI.NewModel(registry).then(setModel).catch((e) => setError(String(e)));
  }, [registry]);

  useEffect(() => EventsOn("build:publish-log", (line: string) => setLog((p) => [...p, line])), []);
  useEffect(() => { logRef.current?.scrollTo({ top: logRef.current.scrollHeight }); }, [log]);

  const m = model;
  const set = (patch: Partial<build.Model>) => m && setModel(new build.Model({ ...m, ...patch }));

  async function wrap(label: string, fn: () => Promise<void>) {
    setBusy(label);
    setError(null);
    setNotice(null);
    try {
      await fn();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  const pickFolder = () =>
    wrap("pick", async () => {
      const d = await ChooseDirectory("Choose the BriefAgent workspace folder");
      if (!d) return;
      setDir(d);
      if (await BuildAPI.Exists(d)) {
        setModel(await BuildAPI.Load(d));
        setResult(null);
        setPubResult(null);
        setNotice(`Loaded the existing BriefAgent in ${d}.`);
      }
    });

  const save = () =>
    wrap("save", async () => {
      if (!m) return;
      await BuildAPI.Save(dir, m);
      setNotice(`Saved to ${dir} — agent.yaml, the instructions file, and (first save only) README, LICENSE, .gitignore.`);
    });

  const runValidate = () =>
    wrap("validate", async () => {
      if (!m) return;
      await BuildAPI.Save(dir, m);
      setResult(await BuildAPI.Validate(dir));
    });

  const publish = () =>
    wrap("publish", async () => {
      if (!m) return;
      await BuildAPI.Save(dir, m);
      setLog([]);
      setPubResult(null);
      setPubResult(await BuildAPI.Publish(registry, dir, visibility));
    });

  if (!m) return error ? <ErrorBox error={error} /> : <Spinner />;

  const inputStyle = { width: "100%" };

  return (
    <div>
      <PageHeader
        title="Build"
        sub={<>Author a BriefAgent, run the registry&apos;s own checks on it, and publish it. The form edits a real workspace (<span className="mono">agent.yaml</span> + instructions) that the <span className="mono">agentry</span> CLI and git work on too.</>}
      />

      <ErrorBox error={error} />
      {notice && <Notice tone="info">{notice}</Notice>}

      <Card>
        <div className="row">
          <Button onClick={pickFolder} disabled={!!busy}>Choose folder…</Button>
          {dir ? <span className="mono small selectable">{dir}</span> : <span className="muted small">Pick where the workspace lives (an existing BriefAgent loads into the form).</span>}
        </div>
      </Card>

      <h2>Identity</h2>
      <div className="card">
        <div className="row">
          <label className="small grow">name
            <input style={inputStyle} className="mono" value={m.name} onChange={(e) => set({ name: e.target.value })} placeholder="my-briefagent" />
          </label>
          <label className="small grow">namespace
            <input style={inputStyle} className="mono" value={m.namespace} onChange={(e) => set({ namespace: e.target.value })} />
          </label>
        </div>
        <div className="row" style={{ marginTop: 8 }}>
          <span className="small">run modes
            <span className="row" style={{ gap: 10, marginTop: 4 }}>
              {(["autonomous", "interactive"] as const).map((mode) => (
                <label key={mode} className="check-row" style={{ margin: 0 }}>
                  <input
                    type="checkbox"
                    checked={(m.modes ?? []).includes(mode)}
                    onChange={(e) => set({ modes: e.target.checked ? [...(m.modes ?? []).filter((x) => x !== mode), mode] : (m.modes ?? []).filter((x) => x !== mode) })}
                  />
                  <span title={mode === "autonomous" ? "runs from its files and ends" : "a chat session under the same controls"}>{mode}</span>
                </label>
              ))}
            </span>
          </span>
          <label className="small">version
            <input className="mono" value={m.version} onChange={(e) => set({ version: e.target.value })} />
          </label>
          <label className="small">license (SPDX)
            <input className="mono" value={m.license} onChange={(e) => set({ license: e.target.value })} />
          </label>
        </div>
        <label className="small" style={{ display: "block", marginTop: 8 }}>summary (one line, ≤140 chars)
          <input style={inputStyle} maxLength={140} value={m.summary} onChange={(e) => set({ summary: e.target.value })} />
        </label>
        <label className="small" style={{ display: "block", marginTop: 8 }}>tags
          <ListInput value={m.tags} onChange={(v) => set({ tags: v })} placeholder="comma, separated" />
        </label>
      </div>

      <h2>Capability contract</h2>
      <p className="muted small">
        Declare the least the agent needs: reviewers gate on this, installs prompt on it, and the
        runner enforces it — an undeclared capability is a tool that does not exist.
      </p>
      <div className="card">
        <div className="row">
          <label className="small">network egress
            <select value={m.egressMode} onChange={(e) => set({ egressMode: e.target.value })}>
              <option value="none">none</option>
              <option value="hosts">specific hosts</option>
              <option value="all">unrestricted (*)</option>
            </select>
          </label>
          {m.egressMode === "hosts" && (
            <label className="small grow">hosts
              <ListInput value={m.egressHosts} onChange={(v) => set({ egressHosts: v })} placeholder="api.example.com, github.com" />
            </label>
          )}
          {m.egressMode === "all" && <span className="badge bad">refused at run time unless the operator allows wildcard egress</span>}
        </div>
        <div className="row" style={{ marginTop: 8 }}>
          <label className="small">fs read
            <select value={m.fsRead} onChange={(e) => set({ fsRead: e.target.value })}>
              <option value="none">none</option>
              <option value="workspace">workspace</option>
              <option value="paths">paths</option>
            </select>
          </label>
          {m.fsRead === "paths" && (
            <label className="small grow">read paths
              <ListInput value={m.fsReadPaths} onChange={(v) => set({ fsReadPaths: v })} />
            </label>
          )}
          <label className="small">fs write
            <select value={m.fsWrite} onChange={(e) => set({ fsWrite: e.target.value })}>
              <option value="none">none</option>
              <option value="workspace">workspace</option>
              <option value="paths">paths</option>
            </select>
          </label>
          {m.fsWrite === "paths" && (
            <label className="small grow">write paths
              <ListInput value={m.fsWritePaths} onChange={(v) => set({ fsWritePaths: v })} />
            </label>
          )}
        </div>
        <label className="small" style={{ display: "block", marginTop: 8 }}>exec commands (single tokens; empty = no exec)
          <ListInput value={m.execCommands} onChange={(v) => set({ execCommands: v })} placeholder="git, jq" />
        </label>
        <label className="small" style={{ display: "block", marginTop: 8 }}>secret NAMES (env vars; never values)
          <ListInput value={m.secrets} onChange={(v) => set({ secrets: v })} placeholder="MY_API_TOKEN" />
        </label>
        <div style={{ marginTop: 12 }}>
          <div className="row" style={{ justifyContent: "space-between" }}>
            <span className="small">declared MCP servers (exact version pins; installed per release, DEV-060)</span>
            <Button
              type="button"
              onClick={() => set({ mcpServers: [...(m.mcpServers ?? []), new build.MCPServerRow({ source: "npm", package: "", version: "", tools: [], secrets: [], egress: [] })] })}
            >
              + add server
            </Button>
          </div>
          {(m.mcpServers ?? []).length === 0 && (
            <p className="muted small" style={{ margin: "4px 0 0" }}>
              None. A server declared here is resolved from npm or PyPI at the pinned version and
              installed into the release&apos;s own store; its tools, secret names and egress are
              scoped to that server.
            </p>
          )}
          {(m.mcpServers ?? []).map((srv, i) => {
            const patch = (row: Partial<build.MCPServerRow>) =>
              set({ mcpServers: m.mcpServers.map((r, j) => (j === i ? new build.MCPServerRow({ ...r, ...row }) : r)) });
            return (
              <div className="card" key={i} style={{ marginTop: 8 }}>
                <div className="row">
                  <label className="small">source
                    <select value={srv.source} onChange={(e) => patch({ source: e.target.value })}>
                      <option value="npm">npm</option>
                      <option value="pypi">PyPI</option>
                    </select>
                  </label>
                  <label className="small grow">package
                    <input
                      type="text" className="mono" style={{ width: "100%" }}
                      placeholder={srv.source === "npm" ? "@scope/server-name" : "mcp-server-name"}
                      value={srv.package} onChange={(e) => patch({ package: e.target.value })}
                    />
                  </label>
                  <label className="small">exact version
                    <input
                      type="text" className="mono" style={{ width: 120 }} placeholder="1.2.3"
                      value={srv.version} onChange={(e) => patch({ version: e.target.value })}
                    />
                  </label>
                  <Button type="button" onClick={() => set({ mcpServers: m.mcpServers.filter((_, j) => j !== i) })}>remove</Button>
                </div>
                <div className="row" style={{ marginTop: 6 }}>
                  <label className="small grow">tools (empty = all the server offers)
                    <ListInput value={srv.tools ?? []} onChange={(v) => patch({ tools: v })} placeholder="list_issues, get_issue" />
                  </label>
                  <label className="small grow">secret NAMES for this server
                    <ListInput value={srv.secrets ?? []} onChange={(v) => patch({ secrets: v })} placeholder="GITHUB_PAT" />
                  </label>
                  <label className="small grow">egress hosts for this server (empty = none)
                    <ListInput value={srv.egress ?? []} onChange={(v) => patch({ egress: v })} placeholder="api.github.com" />
                  </label>
                </div>
                {(srv.egress ?? []).includes("*") && (
                  <span className="badge bad">wildcard egress: refused at run time unless the operator allows it</span>
                )}
              </div>
            );
          })}
        </div>
        <label className="small" style={{ display: "block", marginTop: 8 }}>data access categories
          <div className="row" style={{ flexWrap: "wrap" }}>
            {DATA_ACCESS.map((d) => (
              <label key={d} className="row small" style={{ gap: 4 }}>
                <input
                  type="checkbox"
                  checked={m.dataAccess.includes(d)}
                  onChange={(e) => set({ dataAccess: e.target.checked ? [...m.dataAccess, d] : m.dataAccess.filter((x) => x !== d) })}
                />
                {d}
              </label>
            ))}
          </div>
        </label>
      </div>

      <h2>Up-front questions (intake)</h2>
      <p className="muted small">
        Asked before a run starts, in either mode; the answers are staged read-only for the agent
        at <span className="mono">/inputs/intake.json</span>. A <em>secret</em> answer is carried by a declared
        secret of the same upper-cased name and never reaches the agent or a receipt.
      </p>
      <div className="card">
        <div className="row">
          <Button
            type="button"
            onClick={() => set({ intake: [...(m.intake ?? []), new build.IntakeRow({ id: "", prompt: "", type: "text", required: false, choices: [] })] })}
          >
            Add question
          </Button>
          {(m.intake ?? []).length === 0 && <span className="muted small">No questions — the agent starts from its instructions and the task.</span>}
        </div>
        {(m.intake ?? []).map((q, i) => {
          const patch = (row: Partial<build.IntakeRow>) =>
            set({ intake: m.intake.map((r, j) => (j === i ? new build.IntakeRow({ ...r, ...row }) : r)) });
          return (
            <div className="card" key={i} style={{ marginTop: 8 }}>
              <div className="row">
                <label className="small">id
                  <input type="text" className="mono" style={{ width: 140 }} placeholder="dataset" value={q.id} onChange={(e) => patch({ id: e.target.value })} />
                </label>
                <label className="small grow">prompt
                  <input type="text" style={{ width: "100%" }} placeholder="Path to the dataset to analyse" value={q.prompt} onChange={(e) => patch({ prompt: e.target.value })} />
                </label>
                <label className="small">type
                  <select value={q.type || "text"} onChange={(e) => patch({ type: e.target.value })}>
                    <option value="text">text</option>
                    <option value="choice">choice</option>
                    <option value="file">file</option>
                    <option value="secret">secret</option>
                  </select>
                </label>
                <label className="check-row" style={{ margin: 0 }}>
                  <input type="checkbox" checked={q.required} onChange={(e) => patch({ required: e.target.checked })} />
                  <span>required</span>
                </label>
                <Button type="button" onClick={() => set({ intake: m.intake.filter((_, j) => j !== i) })}>remove</Button>
              </div>
              {q.type === "choice" && (
                <label className="small" style={{ display: "block", marginTop: 6 }}>choices
                  <ListInput value={q.choices ?? []} onChange={(v) => patch({ choices: v })} placeholder="emea, apac, amer" />
                </label>
              )}
            </div>
          );
        })}
      </div>

      {(m.modes ?? []).includes("interactive") && (
        <>
          <h2>Interactive session</h2>
          <div className="card">
            <label className="small" style={{ display: "block" }}>opening line (the agent&apos;s first message when a session starts without a task; ≤ 500 chars)
              <input style={inputStyle} maxLength={500} value={m.opening ?? ""} onChange={(e) => set({ opening: e.target.value })} placeholder="What should I look at?" />
            </label>
          </div>
        </>
      )}

      <h2>Instructions ({m.instructionFile || "AGENT.md"})</h2>
      <div className="card">
        <Textarea
          className="mono"
          style={{ minHeight: 260 }}
          value={m.instructions}
          onChange={(e) => set({ instructions: e.target.value })}
        />
      </div>

      <div className="row" style={{ marginTop: 12 }}>
        <Button disabled={!dir || !!busy} onClick={save}>{busy === "save" ? "Saving…" : "Save"}</Button>
        <Button disabled={!dir || !!busy} onClick={runValidate}>{busy === "validate" ? "Validating…" : "Save + Validate"}</Button>
        <select value={visibility} onChange={(e) => setVisibility(e.target.value)}>
          <option value="private">private</option>
          <option value="public">public</option>
        </select>
        <Button variant="primary" disabled={!dir || !!busy} onClick={publish}>
          {busy === "publish" ? "Publishing…" : "Save + Publish"}
        </Button>
      </div>
      {!dir && <p className="muted small">Choose a folder first — the form edits real files.</p>}

      {result && (
        <Card className="mt">
          <strong>{result.passed ? "validate: PASSED" : "validate: FAILED"}</strong>
          {result.manifestError && <div className="error-box selectable">{result.manifestError}</div>}
          {(result.findings ?? []).map((f: any, i: number) => (
            <div key={i} className="finding">
              <div className="row">
                <Sev severity={f.severity} />
                <span className="mono">{f.rule}</span>
                <span>{f.title}</span>
              </div>
              {f.file && <div className="loc mono">{f.file}{f.line ? `:${f.line}` : ""}</div>}
            </div>
          ))}
          {result.passed && <p className="muted small">Same manifest rules and content scanners the registry runs at publish time.</p>}
        </Card>
      )}

      {(log.length > 0 || pubResult) && (
        <Card className="mt">
          <pre ref={logRef} className="run-log" style={{ maxHeight: 180, overflow: "auto" }}>{log.join("\n")}</pre>
          {pubResult && (
            <div className="selectable">
              <strong>
                {pubResult.status === "published" && `published ${m.namespace}/${m.name}@${pubResult.version}`}
                {pubResult.status === "needs_review" && `submitted — ${m.namespace}/${m.name}@${pubResult.version} needs human review`}
                {pubResult.status === "rejected" && `rejected — ${m.namespace}/${m.name}@${pubResult.version}`}
                {pubResult.status === "refused" && "refused — the registry stored nothing"}
              </strong>
              {pubResult.detail && <p className="muted small">{pubResult.detail}</p>}
              {pubResult.status === "refused" && pubResult.account && (
                // Whose subscription the refusal is about (plan
                // organization-subscriptions.md D22): a member publishes to
                // their own namespace but spends from the organization's
                // pool, so the account that is short is often not the one
                // signed in, and the sentence above may name an
                // organization this person has never typed.
                <p className="muted small">the account is <span className="selectable">{pubResult.account}</span></p>
              )}
              {pubResult.status === "refused" &&
                (pubResult.refusal === "credits-exhausted" || pubResult.refusal === "subscription-inactive") && (
                  // The refusal (plan subscription-tiers.md §8,
                  // organization-subscriptions.md §6): the sentence above
                  // names the balance and the reset, or why the
                  // subscription admits nothing; the way onward is the page
                  // the registry named — the organization's console for a
                  // member, the subscription page for a person.
                  <p className="small">
                    <Button onClick={() => SettingsAPI.OpenUpgrade(registry).catch((e) => setError(String(e)))}>
                      {pubResult.refusal === "subscription-inactive" ? "The subscription" : "Credits and upgrade"}
                    </Button>
                  </p>
                )}
              {pubResult.findingCounts && Object.keys(pubResult.findingCounts).length > 0 && (
                <p className="muted small">
                  findings: {Object.entries(pubResult.findingCounts).map(([s, n]) => `${s} ${n}`).join(" · ")}
                </p>
              )}
            </div>
          )}
        </Card>
      )}
    </div>
  );
}
