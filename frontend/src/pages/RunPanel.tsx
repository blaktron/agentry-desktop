// The run surface (RUNNER-PLAN-2 §P6): run a BriefAgent from the app with
// the same lifecycle `agentry run` executes. Before anything runs, the
// panel shows the declared contract and this machine's setup state; when
// the release declares keys and no MCP profile exists yet, the guided
// setup (§P3) appears inline as a form — masked inputs, names only, an
// empty field means "resolve from the environment at run time". Log lines
// stream live; the §R8 envelope renders at the end, receipts included.
import { useEffect, useMemo, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { ChooseDirectory, ChooseFiles } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { main, trust } from "../../wailsjs/go/models";
import { ErrorBox, Spinner, shortDigest } from "../ui";

export default function RunPanel({
  registry, ns, name, version, attestation, onClose,
}: {
  registry: string;
  ns: string;
  name: string;
  version: string;
  /** The page's provenance verification (declared digest); the run
   * re-verifies the downloaded bytes either way. */
  attestation: trust.AttestationResult | null;
  onClose: () => void;
}) {
  const agentRef = `${ns}/${name}@${version}`;

  const [setup, setSetup] = useState<main.SetupStatusView | null>(null);
  const [setupErr, setSetupErr] = useState<string | null>(null);
  const [profile, setProfile] = useState<main.ProfileStatusView | null>(null);
  const [profileErr, setProfileErr] = useState<string | null>(null);

  // Guided profile form (§P3 as a form).
  const [formOpen, setFormOpen] = useState(false);
  const [formValues, setFormValues] = useState<Record<string, string>>({});
  const [formKeep, setFormKeep] = useState(true);
  const [savingProfile, setSavingProfile] = useState(false);

  // Run form.
  const [task, setTask] = useState("");
  const [inputs, setInputs] = useState<string[]>([]);
  const [outDir, setOutDir] = useState("");
  const [keep, setKeep] = useState(true);
  const [timeoutSec, setTimeoutSec] = useState("");
  const [maxTurns, setMaxTurns] = useState("");
  const [acceptUnsigned, setAcceptUnsigned] = useState(false);

  // Run state.
  const [running, setRunning] = useState(false);
  const [log, setLog] = useState<string[]>([]);
  const [result, setResult] = useState<main.RunResultView | null>(null);
  const [runErr, setRunErr] = useState<string | null>(null);
  const logRef = useRef<HTMLPreElement | null>(null);

  useEffect(() => {
    RunAPI.SetupStatus().then(setSetup).catch((e) => setSetupErr(String(e)));
    RunAPI.ProfileStatus(registry, agentRef)
      .then((p) => {
        setProfile(p);
        setKeep(p.keep);
        setFormKeep(p.keep);
        if (p.declaredSecrets.length > 0 && !p.exists) setFormOpen(true);
      })
      .catch((e) => setProfileErr(String(e)));
  }, [registry, agentRef]);

  useEffect(() => {
    const off = EventsOn("runner:run-log", (line: string) => {
      setLog((prev) => [...prev, line]);
    });
    return off;
  }, []);

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight });
  }, [log]);

  // Fail closed on provenance: until the page's verification resolves,
  // the Run button stays off. Unsigned needs the explicit checkbox (the
  // install wizard's gate); mismatch/unverifiable never runs — the Go
  // side re-verifies the downloaded bytes and refuses those regardless.
  const provPending = attestation == null;
  const unsigned = attestation?.state === "unsigned";
  const unverifiable = attestation != null && attestation.state !== "verified" && !unsigned;
  const needsProfile = !!profile && profile.declaredSecrets.length > 0 && !profile.exists;
  const canRun =
    !running &&
    !!setup?.dockerOk &&
    !needsProfile &&
    !provPending &&
    !unverifiable &&
    (!unsigned || acceptUnsigned);

  const defaultOutHint = useMemo(
    () => `~/Downloads/agentry-out/${ns}-${name}-<runid>`,
    [ns, name],
  );

  async function saveProfile() {
    if (!profile) return;
    setSavingProfile(true);
    setProfileErr(null);
    try {
      const p = await RunAPI.SaveProfile(registry, agentRef, formValues, formKeep);
      setProfile(p);
      setKeep(p.keep);
      setFormOpen(false);
      setFormValues({}); // the values leave this component the moment they are stored
    } catch (e) {
      setProfileErr(String(e));
    } finally {
      setSavingProfile(false);
    }
  }

  async function addFiles() {
    try {
      const files = await ChooseFiles("Choose input files for this run");
      if (files.length) setInputs((prev) => [...prev, ...files.filter((f) => !prev.includes(f))]);
    } catch (e) {
      setRunErr(String(e));
    }
  }

  async function addFolder() {
    try {
      const dir = await ChooseDirectory("Choose an input folder for this run");
      if (dir) setInputs((prev) => (prev.includes(dir) ? prev : [...prev, dir]));
    } catch (e) {
      setRunErr(String(e));
    }
  }

  async function pickOutDir() {
    try {
      const dir = await ChooseDirectory("Choose the output folder");
      if (dir) setOutDir(dir);
    } catch (e) {
      setRunErr(String(e));
    }
  }

  async function runAgent() {
    setRunning(true);
    setRunErr(null);
    setResult(null);
    setLog([]);
    try {
      const t = timeoutSec.trim() === "" ? 0 : Math.max(0, parseInt(timeoutSec, 10) || 0);
      const mt = maxTurns.trim() === "" ? -1 : Math.max(0, parseInt(maxTurns, 10) || 0);
      const r = await RunAPI.RunAgent(registry, agentRef, task, inputs, outDir.trim(), keep, t, mt);
      setResult(r);
    } catch (e) {
      setRunErr(String(e));
    } finally {
      setRunning(false);
    }
  }

  return (
    <div className="modal-backdrop" onClick={() => !running && onClose()}>
      <div className="modal modal-wide" onClick={(e) => e.stopPropagation()}>
        <div className="spread">
          <h2 style={{ margin: 0 }}>Run {agentRef}</h2>
          <button onClick={onClose} disabled={running} title={running ? "A run is in progress" : ""}>✕</button>
        </div>
        <p className="muted small">
          Runs in an isolated container; tools come only from a per-run Docker MCP Gateway
          serving exactly the declared contract. Reports and receipts land in the output folder.
        </p>

        {/* Machine readiness (§P1) */}
        {setupErr && <ErrorBox error={setupErr} />}
        {setup && !setup.dockerOk && (
          <div className="error-box selectable">
            Docker is not available — BriefAgent runs need a docker engine on this machine.
            {setup.dockerErr ? ` (${setup.dockerErr})` : ""} Configure it, then reopen this panel.
          </div>
        )}
        {setup && setup.dockerOk && !setup.imagesReady && (
          <div className="notice">
            The gateway/harness/runner images are not all present yet — this run will install
            them first (it can take a few minutes), or prepare them once in Settings → Agent runs.
          </div>
        )}

        {/* Declared contract: what a Run click accepts. */}
        {profile && (
          <>
            <h3>Declared contract</h3>
            <div className="card selectable">
              <div className="kv"><span className="k">release</span><span className="mono">{profile.ref}</span></div>
              <div className="kv"><span className="k">digest</span><span className="mono" title={profile.digest}>{shortDigest(profile.digest)}</span></div>
              {profile.capabilityLines.map((l) => (
                <div key={l} className="mono small">{l}</div>
              ))}
            </div>
          </>
        )}
        {profileErr && <ErrorBox error={profileErr} />}

        {/* Guided MCP-profile setup (§P3). */}
        {profile && profile.declaredSecrets.length > 0 && (
          <>
            <h3>Keys &amp; identities (MCP profile)</h3>
            {!formOpen && profile.exists && (
              <div className="row">
                <span className="muted small">
                  Stored for this exact release{profile.storedSecretNames.length > 0
                    ? `: ${profile.storedSecretNames.join(", ")}`
                    : " (all resolve from the environment at run time)"}
                  {" · "}{profile.keep ? "kept after runs" : "removed when a run completes"}
                </span>
                <button onClick={() => { setFormOpen(true); setFormValues({}); }}>Reconfigure…</button>
              </div>
            )}
            {formOpen && (
              <div className="card">
                <p className="muted small" style={{ marginTop: 0 }}>
                  This release declares the keys below. They are stored once per exact release
                  (0600, this machine only), injected only into the run&apos;s tool container, and
                  never shown or logged. Leave a field empty to resolve it from the environment
                  at run time.
                </p>
                {profile.declaredSecrets.map((s) => (
                  <div className="row" key={s} style={{ marginBottom: 8 }}>
                    <span className="mono" style={{ minWidth: 220 }}>{s}</span>
                    <input
                      type="password"
                      className="grow"
                      autoComplete="off"
                      placeholder="empty = resolve from the environment at run time"
                      value={formValues[s] ?? ""}
                      onChange={(e) => setFormValues({ ...formValues, [s]: e.target.value })}
                    />
                  </div>
                ))}
                <label className="check-row">
                  <input type="checkbox" checked={formKeep} onChange={(e) => setFormKeep(e.target.checked)} />
                  <span>
                    <strong>Keep this profile for future runs of this exact version.</strong>{" "}
                    <span className="muted">Unchecked, it is removed when the run completes.</span>
                  </span>
                </label>
                <div className="row" style={{ justifyContent: "flex-end" }}>
                  {profile.exists && <button onClick={() => setFormOpen(false)} disabled={savingProfile}>Cancel</button>}
                  <button className="primary" onClick={saveProfile} disabled={savingProfile}>
                    {savingProfile ? "Saving…" : "Save keys"}
                  </button>
                </div>
              </div>
            )}
          </>
        )}

        {/* Run form */}
        <h3>Task</h3>
        <textarea
          className="task-input mono"
          rows={4}
          placeholder="What should this BriefAgent do? (empty = the packaged instruction runs with no extra task)"
          value={task}
          onChange={(e) => setTask(e.target.value)}
          disabled={running}
        />

        <h3>Input files</h3>
        <p className="muted small" style={{ margin: "2px 0 6px" }}>
          Provided read-only under <span className="mono">/inputs</span> in the tool container —
          the BriefAgent reads them with <span className="mono">list_inputs</span> / <span className="mono">read_input</span>.
        </p>
        {inputs.length > 0 && (
          <div>
            {inputs.map((p) => (
              <div key={p} className="row" style={{ marginBottom: 4 }}>
                <span className="mono small grow selectable">{p}</span>
                <button disabled={running} onClick={() => setInputs(inputs.filter((x) => x !== p))}>Remove</button>
              </div>
            ))}
          </div>
        )}
        <div className="row">
          <button onClick={addFiles} disabled={running}>Add files…</button>
          <button onClick={addFolder} disabled={running}>Add folder…</button>
        </div>

        <h3>Output &amp; limits</h3>
        <div className="row">
          <input
            type="text"
            className="grow mono"
            placeholder={`output folder — default ${defaultOutHint}`}
            value={outDir}
            onChange={(e) => setOutDir(e.target.value)}
            disabled={running}
          />
          <button onClick={pickOutDir} disabled={running}>Choose…</button>
        </div>
        <div className="row" style={{ marginTop: 8 }}>
          <label className="row" style={{ gap: 6 }}>
            <span className="muted small">timeout (s)</span>
            <input
              type="number" min={1} style={{ width: 110 }}
              placeholder={setup ? String(setup.timeoutSeconds) : ""}
              value={timeoutSec} onChange={(e) => setTimeoutSec(e.target.value)} disabled={running}
            />
          </label>
          <label className="row" style={{ gap: 6 }}>
            <span className="muted small">max turns (0 = unlimited)</span>
            <input
              type="number" min={0} style={{ width: 110 }}
              placeholder={setup ? String(setup.maxTurns) : ""}
              value={maxTurns} onChange={(e) => setMaxTurns(e.target.value)} disabled={running}
            />
          </label>
          <label className="check-row" style={{ margin: 0 }}>
            <input type="checkbox" checked={keep} onChange={(e) => setKeep(e.target.checked)} disabled={running} />
            <span>keep the MCP profile after this run</span>
          </label>
        </div>

        {unsigned && (
          <label className="check-row">
            <input type="checkbox" checked={acceptUnsigned} onChange={(e) => setAcceptUnsigned(e.target.checked)} />
            <span>
              <strong>Run an unsigned release.</strong>{" "}
              <span className="muted">
                This registry published no attestation for this version — nothing cryptographically
                vouches for these bytes. The run&apos;s receipts are marked unsigned.
              </span>
            </span>
          </label>
        )}
        {unverifiable && (
          <div className="error-box selectable">
            Attestation {attestation?.state}: {attestation?.detail} — the runner re-verifies before
            executing and refuses a release whose signature cannot be verified.
          </div>
        )}
        {provPending && (
          <p className="muted small">
            Waiting for the provenance check on this version — if this persists, the registry
            could not answer; close and reopen the page.
          </p>
        )}
        {needsProfile && (
          <p className="muted small">Save the declared keys above (or leave them to the environment) before running.</p>
        )}

        <ErrorBox error={runErr} />
        <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
          <button onClick={onClose} disabled={running}>Close</button>
          <button className="primary" disabled={!canRun} onClick={runAgent}>
            {running ? "Running…" : "Run"}
          </button>
        </div>

        {/* Live log */}
        {(running || log.length > 0) && (
          <>
            <h3>Run log</h3>
            <pre ref={logRef} className="run-log selectable">{log.join("\n")}{running ? "\n…" : ""}</pre>
            {running && <Spinner label="Running — the container topology streams its progress above." />}
          </>
        )}

        {/* Result envelope (§R8) */}
        {result && (
          <>
            <h3>Result</h3>
            <div className="card selectable">
              <div className="row">
                {result.status === "ok"
                  ? <span className="badge ok">ok</span>
                  : <span className="badge bad">error (exit {result.exitCode})</span>}
                <span className={`badge ${result.attestationState === "verified" ? "ok" : "warn"}`}>
                  attestation: {result.attestationState}
                </span>
                <span className="muted small">
                  {result.turns} turns · {result.inputTokens + result.outputTokens} tokens ·{" "}
                  {(result.wallMs / 1000).toFixed(1)}s{result.costUsd > 0 ? ` · $${result.costUsd.toFixed(4)}` : ""}
                </span>
              </div>
              {result.error && <ErrorBox error={result.error} />}
              <ResultBody raw={result.resultJson} />
              <div className="kv"><span className="k">output folder</span><span className="mono">{result.outDir}</span></div>
              <div className="kv"><span className="k">receipts</span><span className="mono">{result.receiptsDir}</span></div>
              <div className="kv"><span className="k">run plan sha256</span><span className="mono" title={result.runPlanSha}>{result.runPlanSha.slice(0, 12)}…</span></div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

/** Renders the envelope's result: a JSON string result as text, structured
 * JSON pretty-printed, anything else verbatim. */
function ResultBody({ raw }: { raw: string }) {
  if (!raw) return null;
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return <pre className="run-result selectable">{raw}</pre>;
  }
  if (typeof parsed === "string") {
    if (parsed === "") return <p className="muted small">The BriefAgent produced no final text (see reports and receipts).</p>;
    return <pre className="run-result selectable">{parsed}</pre>;
  }
  return <pre className="run-result selectable">{JSON.stringify(parsed, null, 2)}</pre>;
}
