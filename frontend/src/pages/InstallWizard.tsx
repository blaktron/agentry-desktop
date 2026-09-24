// The install flow: choose target/directory → Prepare (resolve, download,
// digest + signature verification, capability diff — nothing written) →
// review → Confirm. The two consent gates (unsigned release, widened or
// egress/exec capabilities) are explicit checkboxes; the primary button
// stays disabled until every applicable one is ticked.
import { useEffect, useMemo, useState } from "react";
import * as InstallAPI from "../../wailsjs/go/main/InstallAPI";
import { ChooseDirectory } from "../../wailsjs/go/main/App";
import { install } from "../../wailsjs/go/models";
import { DeclaredServersTable, ErrorBox, SignedBadge, Spinner, shortDigest } from "../ui";

const TARGETS: { id: string; label: string; needsDir: boolean; hint: string }[] = [
  { id: "claude-skill", label: "Claude skill", needsDir: false, hint: "installs into ~/.claude/skills (or a folder you pick)" },
  { id: "agents-md", label: "AGENTS.md prompt agent", needsDir: true, hint: "extracts into <project>/agents/ and updates <project>/AGENTS.md" },
];

export default function InstallWizard({
  registry, agentRef, onClose, presetTarget, presetDir, startAtReview = false, onInstalled,
}: {
  registry: string;
  agentRef: string;
  onClose: () => void;
  // presetTarget and presetDir fill the first step: the agent page's
  // one-off link opens it on the export Settings → Coding agents implies
  // and its folder (agentry-desktop#148); Installed's Update passes the
  // record's own and, with startAtReview, goes straight to Prepare.
  presetTarget?: string;
  presetDir?: string;
  startAtReview?: boolean;
  onInstalled?: () => void;
}) {
  // The wizard is the coding-agent export (D-031: the one-click Install
  // on the agent page is the runner home); its default is the Claude skill,
  // said here rather than left to Go, whose default is the runner target.
  const [target, setTarget] = useState(presetTarget || TARGETS[0].id);
  const [dir, setDir] = useState(presetDir ?? "");
  const [plan, setPlan] = useState<install.Plan | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<install.Result | null>(null);
  const [acceptUnsigned, setAcceptUnsigned] = useState(false);
  // Its own consent (M23): an unscanned release is not an unsigned one,
  // and ticking the unsigned box says nothing about it.
  const [acceptUnscanned, setAcceptUnscanned] = useState(false);
  const [acceptCaps, setAcceptCaps] = useState(false);

  const effectiveTarget = target;
  const targetInfo = useMemo(() => TARGETS.find((t) => t.id === effectiveTarget) ?? TARGETS[0], [effectiveTarget]);

  // An update from Installed goes straight to Prepare: its target and
  // directory are the record's.
  useEffect(() => {
    if (startAtReview && presetTarget) void prepare(presetTarget, presetDir ?? "");
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
      const r = await InstallAPI.Confirm(plan.id, acceptUnsigned, acceptCaps, acceptUnscanned);
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
    (!plan?.unscanned || acceptUnscanned) &&
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
                <input type="radio" name="target" checked={effectiveTarget === t.id} onChange={() => setTarget(t.id)} />
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
              {plan.paths.agentsMd && <div className="kv"><span className="k">updates</span><span className="mono">{plan.paths.agentsMd}</span></div>}
              {plan.prevVersion && <div className="kv"><span className="k">installed here</span><span className="mono">{plan.prevVersion}</span></div>}
            </div>

            {plan.targetWarning && <div className="notice">{plan.targetWarning}</div>}
            {plan.vulnerabilityRefusal && (
              <div className="notice selectable">
                <strong>Runs of this release are refused by default.</strong> {plan.vulnerabilityRefusal}.
                Installing records the declared servers as they are; the run panel has the override,
                which is recorded in the run&apos;s receipt.
              </div>
            )}
            {!plan.vulnerabilityRefusal && plan.vulnerabilityLine && (
              <p className="muted small">Declared MCP servers {plan.vulnerabilityLine}.</p>
            )}

            <h3>Declared capabilities</h3>
            <CapsTable caps={plan.capabilities} />
            {(plan.mcpServers ?? []).length > 0 && (
              <>
                <h3>Declared MCP servers</h3>
                <DeclaredServersTable servers={plan.mcpServers} />
              </>
            )}

            {plan.diff && (plan.diff.widened?.length || plan.diff.narrowed?.length) ? (
              <>
                <h3>Changes vs {plan.prevVersion}</h3>
                {(plan.diff.widened ?? []).map((w, i) => <div key={`w${i}`} className="diff-line widen">+ this update {w}</div>)}
                {(plan.diff.narrowed ?? []).map((n, i) => <div key={`n${i}`} className="diff-line narrow">− this update {n}</div>)}
              </>
            ) : plan.prevVersion ? (
              <p className="muted">Capability contract unchanged vs {plan.prevVersion}.</p>
            ) : null}

            {plan.unscanned && (
              <div className="notice notice-unscanned mark-unscanned">
                <p style={{ margin: 0 }}>
                  <SignedBadge att={plan.attestation} />{" "}
                  <strong>The registry admitted this version without a scan.</strong>{" "}
                  <span className="muted">
                    Its organization&apos;s administrator took the agent off the pipeline: no findings, no report and no attestation stand behind it. No scanner looked at its contents.
                  </span>
                </p>
                <label className="check-row">
                  <input type="checkbox" checked={acceptUnscanned} onChange={(e) => setAcceptUnscanned(e.target.checked)} />
                  <span>
                    <strong>Install an unscanned release.</strong>{" "}
                    <span className="muted">Accepting an unsigned release does not cover this; it is its own decision.</span>
                  </span>
                </label>
              </div>
            )}
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
