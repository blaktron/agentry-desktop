// The install flow: choose target/directory → Prepare (resolve, download,
// digest + signature verification, capability diff — nothing written) →
// review → Confirm. The two consent gates (unsigned release, widened or
// egress/exec capabilities) are explicit checkboxes; the primary button
// stays disabled until every applicable one is ticked.
import { useEffect, useMemo, useState } from "react";
import * as InstallAPI from "../../wailsjs/go/main/InstallAPI";
import { ChooseDirectory } from "../../wailsjs/go/main/App";
import { install } from "../../wailsjs/go/models";
import { ErrorBox, Spinner, shortDigest } from "../ui";

const TARGETS: { id: string; label: string; needsDir: boolean; hint: string }[] = [
  { id: "claude-skill", label: "Claude skill", needsDir: false, hint: "installs into ~/.claude/skills (or a folder you pick)" },
  { id: "mcp", label: "MCP server", needsDir: true, hint: "extracts into <project>/agents/ and merges <project>/.mcp.json" },
  { id: "agents-md", label: "AGENTS.md prompt agent", needsDir: true, hint: "extracts into <project>/agents/ and updates <project>/AGENTS.md" },
];

function defaultTargetFor(kind: string): string {
  if (kind === "mcp-server") return "mcp";
  if (kind === "prompt-agent") return "agents-md";
  return "claude-skill";
}

export default function InstallWizard({
  registry, agentRef, kind, onClose, presetTarget, presetDir, onInstalled,
}: {
  registry: string;
  agentRef: string;
  kind: string;
  onClose: () => void;
  presetTarget?: string;
  presetDir?: string;
  onInstalled?: () => void;
}) {
  const [target, setTarget] = useState(presetTarget || defaultTargetFor(kind));
  const [dir, setDir] = useState(presetDir ?? "");
  const [plan, setPlan] = useState<install.Plan | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<install.Result | null>(null);
  const [acceptUnsigned, setAcceptUnsigned] = useState(false);
  const [acceptCaps, setAcceptCaps] = useState(false);

  const targetInfo = useMemo(() => TARGETS.find((t) => t.id === target)!, [target]);

  // A preset flow (update from Installed) can go straight to Prepare.
  useEffect(() => {
    if (presetTarget && (presetDir !== undefined)) void prepare(presetTarget, presetDir);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function pickDir() {
    try {
      const chosen = await ChooseDirectory(targetInfo.needsDir ? "Choose the project directory" : "Choose the install directory");
      if (chosen) setDir(chosen);
    } catch (e) {
      setError(String(e));
    }
  }

  async function prepare(t = target, d = dir) {
    setBusy("Resolving, downloading, and verifying…");
    setError(null);
    try {
      const p = await InstallAPI.Prepare(registry, agentRef, t, d);
      setPlan(p);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  async function confirm() {
    if (!plan) return;
    setBusy("Installing…");
    setError(null);
    try {
      const r = await InstallAPI.Confirm(plan.id, acceptUnsigned, acceptCaps);
      setResult(r);
      onInstalled?.();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  function close() {
    if (plan && !result) InstallAPI.Cancel(plan.id).catch(() => {});
    onClose();
  }

  const gatesOK =
    (!plan?.unsigned || acceptUnsigned) &&
    (!(plan?.needsCapabilityConfirm || plan?.requiresWidenedConfirm) || acceptCaps);

  return (
    <div className="modal-backdrop" onClick={close}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="spread">
          <h2 style={{ margin: 0 }}>Install {agentRef}</h2>
          <button onClick={close}>✕</button>
        </div>

        {/* Step 1: target + directory */}
        {!plan && !result && (
          <>
            <h3>Install as</h3>
            {TARGETS.map((t) => (
              <label key={t.id} className="check-row">
                <input type="radio" name="target" checked={target === t.id} onChange={() => setTarget(t.id)} />
                <span>
                  <strong>{t.label}</strong>
                  <span className="muted"> — {t.hint}</span>
                </span>
              </label>
            ))}
            <h3>{targetInfo.needsDir ? "Project directory" : "Directory"}</h3>
            <div className="row">
              <input type="text" className="grow mono" placeholder={targetInfo.needsDir ? "required — pick the project folder" : "default: ~/.claude/skills"} value={dir} onChange={(e) => setDir(e.target.value)} />
              <button onClick={pickDir}>Choose…</button>
            </div>
            <ErrorBox error={error} />
            {busy ? <Spinner label={busy} /> : (
              <div className="row" style={{ marginTop: 14, justifyContent: "flex-end" }}>
                <button onClick={close}>Cancel</button>
                <button className="primary" disabled={targetInfo.needsDir && !dir} onClick={() => prepare()}>
                  Verify &amp; review
                </button>
              </div>
            )}
          </>
        )}

        {/* Step 2: review the verified plan */}
        {plan && !result && (
          <>
            <div className="card selectable">
              <div className="kv"><span className="k">version</span><span className="mono">{plan.version}</span> {plan.yanked && <span className="badge bad">yanked</span>}</div>
              <div className="kv"><span className="k">digest</span><span className="mono" title={plan.digest}>{shortDigest(plan.digest)} ✓ matches registry</span></div>
              <div className="kv">
                <span className="k">attestation</span>
                {plan.attestation.state === "verified"
                  ? <span>DSSE signature valid (key <span className="mono">{plan.attestation.keyId}</span>){plan.attestation.rulesetVersion ? ` — scanned with ruleset ${plan.attestation.rulesetVersion}` : ""}</span>
                  : <span>{plan.attestation.state}: {plan.attestation.detail}</span>}
              </div>
              <div className="kv"><span className="k">destination</span><span className="mono">{plan.paths.dest}</span></div>
              {plan.paths.mcpConfig && <div className="kv"><span className="k">merges into</span><span className="mono">{plan.paths.mcpConfig}</span></div>}
              {plan.paths.agentsMd && <div className="kv"><span className="k">updates</span><span className="mono">{plan.paths.agentsMd}</span></div>}
              {plan.prevVersion && <div className="kv"><span className="k">installed here</span><span className="mono">{plan.prevVersion}</span></div>}
            </div>

            {plan.targetWarning && <div className="notice">{plan.targetWarning}</div>}

            <h3>Declared capabilities</h3>
            <CapsTable caps={plan.capabilities} />

            {plan.diff && (plan.diff.widened?.length || plan.diff.narrowed?.length) ? (
              <>
                <h3>Changes vs {plan.prevVersion}</h3>
                {(plan.diff.widened ?? []).map((w, i) => <div key={`w${i}`} className="diff-line widen">+ this update {w}</div>)}
                {(plan.diff.narrowed ?? []).map((n, i) => <div key={`n${i}`} className="diff-line narrow">− this update {n}</div>)}
              </>
            ) : plan.prevVersion ? (
              <p className="muted">Capability contract unchanged vs {plan.prevVersion}.</p>
            ) : null}

            {plan.unsigned && (
              <label className="check-row">
                <input type="checkbox" checked={acceptUnsigned} onChange={(e) => setAcceptUnsigned(e.target.checked)} />
                <span>
                  <strong>Install an unsigned release.</strong>{" "}
                  <span className="muted">This registry published no attestation for this version — nothing cryptographically vouches for these bytes.</span>
                </span>
              </label>
            )}
            {(plan.needsCapabilityConfirm || plan.requiresWidenedConfirm) && (
              <label className="check-row">
                <input type="checkbox" checked={acceptCaps} onChange={(e) => setAcceptCaps(e.target.checked)} />
                <span>
                  <strong>{plan.requiresWidenedConfirm ? "Accept the widened capability contract." : "Accept the declared network/exec capabilities."}</strong>{" "}
                  <span className="muted">The agent may do what is listed above once your tools run it.</span>
                </span>
              </label>
            )}

            <ErrorBox error={error} />
            {busy ? <Spinner label={busy} /> : (
              <div className="row" style={{ marginTop: 14, justifyContent: "flex-end" }}>
                <button onClick={close}>Cancel</button>
                <button className="primary" disabled={!gatesOK} onClick={confirm}>Install</button>
              </div>
            )}
          </>
        )}

        {/* Step 3: done */}
        {result && (
          <>
            <div className="card selectable">
              <p>{result.message}</p>
              {result.secretsNote && <div className="notice">{result.secretsNote}</div>}
            </div>
            <div className="row" style={{ justifyContent: "flex-end" }}>
              <button className="primary" onClick={close}>Done</button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function CapsTable({ caps }: { caps: install.Plan["capabilities"] }) {
  const rows: [string, string][] = [];
  const list = (v?: string[]) => (v && v.length ? v.join(", ") : "");
  if (list(caps.egressHosts)) rows.push(["network egress", list(caps.egressHosts)]);
  if (caps.fsRead && caps.fsRead !== "none") rows.push(["filesystem read", caps.fsRead === "paths" ? `paths: ${list(caps.fsReadPaths)}` : caps.fsRead]);
  if (caps.fsWrite && caps.fsWrite !== "none") rows.push(["filesystem write", caps.fsWrite === "paths" ? `paths: ${list(caps.fsWritePaths)}` : caps.fsWrite]);
  if (list(caps.execCommands)) rows.push(["exec", list(caps.execCommands)]);
  if (list(caps.secrets)) rows.push(["secrets", list(caps.secrets)]);
  if (list(caps.dataAccess)) rows.push(["data access", list(caps.dataAccess)]);
  if (rows.length === 0) return <p className="muted">None declared (pure text agent).</p>;
  return (
    <table className="caps-table selectable">
      <tbody>
        {rows.map(([k, v]) => (
          <tr key={k}><td>{k}</td><td className="mono">{v}</td></tr>
        ))}
      </tbody>
    </table>
  );
}
