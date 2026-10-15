// The install flow: choose target/directory → Prepare (resolve, download,
// digest + signature verification, capability diff — nothing written) →
// review → Confirm. The two consent gates (unsigned release, widened or
// egress/exec capabilities) are explicit checkboxes; the primary button
// stays disabled until every applicable one is ticked.
import { useEffect, useMemo, useState } from "react";
import * as InstallAPI from "../../wailsjs/go/app/InstallAPI";
import { ChooseDirectory } from "../../wailsjs/go/app/App";
import { install } from "../../wailsjs/go/models";
import { DeclaredServersTable, ErrorBox, SignedBadge, Spinner, fmtBinaryBytes, shortDigest } from "../ui";
import { secretsRow } from "../requiredKeys";
import { browserCap, browserLine } from "../browser";


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
  // and its folder; the Library's Update passes the
  // record's own and, with startAtReview, goes straight to Prepare.
  presetTarget?: string;
  presetDir?: string;
  startAtReview?: boolean;
  onInstalled?: () => void;
}) {
  // The wizard is the coding-agent export (the one-click Install
  // on the agent page is the runner home); its default is the Claude skill,
  // said here rather than left to Go, whose default is the runner target.
  const [target, setTarget] = useState(presetTarget || TARGETS[0].id);
  const [dir, setDir] = useState(presetDir ?? "");
  const [plan, setPlan] = useState<install.Plan | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<install.Result | null>(null);
  const [acceptUnsigned, setAcceptUnsigned] = useState(false);
  // Its own consent: an unscanned release is not an unsigned one,
  // and ticking the unsigned box says nothing about it.
  const [acceptUnscanned, setAcceptUnscanned] = useState(false);
  const [acceptCaps, setAcceptCaps] = useState(false);

  const effectiveTarget = target;
  const targetInfo = useMemo(() => TARGETS.find((t) => t.id === effectiveTarget) ?? TARGETS[0], [effectiveTarget]);

  // An update from the Library goes straight to Prepare: its target and
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
                Installing records the declared servers as they are; in Unsafe Mode the run panel asks
                your consent for a run, and the receipt names it.
              </div>
            )}
            {!plan.vulnerabilityRefusal && plan.vulnerabilityLine && (
              <p className="muted small">Declared MCP servers {plan.vulnerabilityLine}.</p>
            )}

            <h3>Declared capabilities</h3>
            <CapsTable caps={plan.capabilities} shipped={plan.shippedKnowledge} secrets={secretsRow(plan.declaredSecrets, plan.optionalSecrets)} />
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
                    No findings, no report and no attestation stand behind it. Its organization&apos;s administrator took the QuickAgent off the pipeline, and no scanner looked at its contents.
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
                  <strong>{plan.requiresWidenedConfirm ? "Accept the widened capability contract." : `Accept the declared ${declaredWords(plan.capabilities)}.`}</strong>{" "}
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

// declaredWords names what a first install's consent accepts: the network
// and exec capabilities, a target host, a browser, or several: "Accept the
// declared browser."
function declaredWords(caps: install.Plan["capabilities"]): string {
  const what: string[] = [];
  if (caps.egressHosts?.length || caps.execCommands?.length) what.push("network/exec capabilities");
  if (caps.targetHosts?.length) what.push(caps.targetHosts.length === 1 ? "target host" : "target hosts");
  if (caps.browser) what.push("browser");
  if (what.length <= 1) return what[0] ?? "capabilities";
  return `${what.slice(0, -1).join(", ")} and ${what[what.length - 1]}`;
}

function CapsTable({ caps, shipped, secrets }: { caps: install.Plan["capabilities"]; shipped?: install.ShippedPack[]; secrets: string }) {
  const rows: [string, string][] = [];
  const list = (v?: string[]) => (v && v.length ? v.join(", ") : "");
  if (list(caps.egressHosts)) rows.push(["network egress", list(caps.egressHosts)]);
  if (caps.fsRead && caps.fsRead !== "none") rows.push(["filesystem read", caps.fsRead === "paths" ? `paths: ${list(caps.fsReadPaths)}` : caps.fsRead]);
  if (caps.fsWrite && caps.fsWrite !== "none") rows.push(["filesystem write", caps.fsWrite === "paths" ? `paths: ${list(caps.fsWritePaths)}` : caps.fsWrite]);
  if (list(caps.execCommands)) rows.push(["exec", list(caps.execCommands)]);
  // Every declared secret, the release's own and its servers', an optional
  // one marked: moving one between optional and required asks for no new
  // consent.
  if (secrets) rows.push(["secrets", secrets]);
  if (list(caps.dataAccess)) rows.push(["data access", list(caps.dataAccess)]);
  // A target host: a
  // machine the person names when it runs, worked as the account they sign
  // in with, in the CLI's consent words.
  if (list(caps.targetHosts)) {
    rows.push(["target hosts", `${list(caps.targetHosts)}: asks for a server when it runs, and can run any command on it as the account you sign in with`]);
  }
  // A browser: where it
  // may go and the secrets it may type, in the CLI's consent words.
  const browser = browserCap(caps.browser);
  if (browser) rows.push(["browser", browserLine(browser)]);
  // The knowledge packs the release ships in its archive: part of the
  // contract, each with its size from the archive.
  if (list(caps.knowledge)) {
    const sized = (name: string) => {
      const bytes = (shipped ?? []).find((p) => p.name === name)?.bytes;
      return bytes != null ? `${name} (${fmtBinaryBytes(bytes)})` : name;
    };
    rows.push(["knowledge", "ships " + (caps.knowledge ?? []).map(sized).join(", ")]);
  }
  if (rows.length === 0) return <p className="muted">None declared (pure text).</p>;
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
