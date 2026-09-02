// Build (desktop D-013): author a BriefAgent as a form, save it as a real
// workspace (the same files `agentry init` writes), validate it with the
// registry's own scanners, and publish it through the full pipeline. The
// form is an editor for agent.yaml — the manifest stays the contract, and
// nothing here bypasses a publication gate.
import { useEffect, useRef, useState } from "react";
import * as BuildAPI from "../../wailsjs/go/main/BuildAPI";
import { ChooseDirectory } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { build, validate } from "../../wailsjs/go/models";
import { ErrorBox, Sev, Spinner } from "../ui";

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
      <h1>Build</h1>
      <p className="page-sub">
        Author a BriefAgent, run the registry&apos;s own checks on it, and publish it. The form
        edits a real workspace (<span className="mono">agent.yaml</span> + instructions) that the{" "}
        <span className="mono">agentry</span> CLI and git work on too.
      </p>

      <ErrorBox error={error} />
      {notice && <div className="notice selectable">{notice}</div>}

      <div className="card">
        <div className="row">
          <button onClick={pickFolder} disabled={!!busy}>Choose folder…</button>
          {dir ? <span className="mono small selectable">{dir}</span> : <span className="muted small">Pick where the workspace lives (an existing BriefAgent loads into the form).</span>}
        </div>
      </div>

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
          <label className="small">kind
            <select value={m.kind} onChange={(e) => set({ kind: e.target.value })}>
              <option value="skill">skill</option>
              <option value="prompt-agent">prompt-agent</option>
            </select>
          </label>
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
            <button
              type="button"
              onClick={() => set({ mcpServers: [...(m.mcpServers ?? []), new build.MCPServerRow({ source: "npm", package: "", version: "", tools: [], secrets: [], egress: [] })] })}
            >
              + add server
            </button>
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
                  <button type="button" onClick={() => set({ mcpServers: m.mcpServers.filter((_, j) => j !== i) })}>remove</button>
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

      <h2>Instructions ({m.kind === "skill" ? "SKILL.md" : "AGENT.md"})</h2>
      <div className="card">
        <textarea
          className="mono"
          style={{ width: "100%", minHeight: 260 }}
          value={m.instructions}
          onChange={(e) => set({ instructions: e.target.value })}
        />
      </div>

      <div className="row" style={{ marginTop: 12 }}>
        <button disabled={!dir || !!busy} onClick={save}>{busy === "save" ? "Saving…" : "Save"}</button>
        <button disabled={!dir || !!busy} onClick={runValidate}>{busy === "validate" ? "Validating…" : "Save + Validate"}</button>
        <select value={visibility} onChange={(e) => setVisibility(e.target.value)}>
          <option value="private">private</option>
          <option value="public">public</option>
        </select>
        <button className="primary" disabled={!dir || !!busy} onClick={publish}>
          {busy === "publish" ? "Publishing…" : "Save + Publish"}
        </button>
      </div>
      {!dir && <p className="muted small">Choose a folder first — the form edits real files.</p>}

      {result && (
        <div className="card" style={{ marginTop: 12 }}>
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
        </div>
      )}

      {(log.length > 0 || pubResult) && (
        <div className="card" style={{ marginTop: 12 }}>
          <pre ref={logRef} className="log" style={{ maxHeight: 180, overflow: "auto" }}>{log.join("\n")}</pre>
          {pubResult && (
            <div className="selectable">
              <strong>
                {pubResult.status === "published" && `published ${m.namespace}/${m.name}@${pubResult.version}`}
                {pubResult.status === "needs_review" && `submitted — ${m.namespace}/${m.name}@${pubResult.version} needs human review`}
                {pubResult.status === "rejected" && `rejected — ${m.namespace}/${m.name}@${pubResult.version}`}
              </strong>
              {pubResult.detail && <p className="muted small">{pubResult.detail}</p>}
              {pubResult.findingCounts && Object.keys(pubResult.findingCounts).length > 0 && (
                <p className="muted small">
                  findings: {Object.entries(pubResult.findingCounts).map(([s, n]) => `${s} ${n}`).join(" · ")}
                </p>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
