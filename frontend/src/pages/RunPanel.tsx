// The run surface (RUNNER-PLAN-2 §P6): run a BriefAgent from the app with
// the same lifecycle `agentry run` executes. Before anything runs, the
// panel shows the declared contract and this machine's setup state; when
// the release declares keys and no MCP profile exists yet, the guided
// setup (§P3) appears inline as a form — masked inputs, names only, an
// empty field means "resolve from the environment at run time". Log lines
// stream live; the §R8 envelope renders at the end, receipts included.
import { useEffect, useMemo, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import { ChooseDirectory, ChooseFiles } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { main, trust } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Notice, SignedBadge, Spinner, Textarea, shortDigest } from "../ui";
import SessionChat from "./SessionChat";
import { HostedCredits, HostedProgress, HostedResult, HostedUnavailable } from "./HostedRun";

export default function RunPanel({
  registry, ns, name, version, attestation, onClose, initialMode,
}: {
  registry: string;
  ns: string;
  name: string;
  version: string;
  /** The page's provenance verification (declared digest); the run
   * re-verifies the downloaded bytes either way. */
  attestation: trust.AttestationResult | null;
  onClose: () => void;
  /** Harness-only: open with this run mode selected, for screenshots. */
  initialMode?: string;
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
  const [intakeValues, setIntakeValues] = useState<Record<string, string>>({});
  const [mode, setMode] = useState("");
  const [sessionActive, setSessionActive] = useState(false);
  const [inputs, setInputs] = useState<string[]>([]);
  const [outDir, setOutDir] = useState("");
  const [keep, setKeep] = useState(true);
  // D-012: the keep toggle is sent only once the user touches it; an
  // untouched toggle sends null, so a no-secret release does not grow an
  // empty profile per run, while an explicit keep is recorded.
  const [keepTouched, setKeepTouched] = useState(false);
  const [timeoutSec, setTimeoutSec] = useState("");
  const [maxTurns, setMaxTurns] = useState("");
  const [acceptUnsigned, setAcceptUnsigned] = useState(false);
  // Its own consent (M23): an unscanned release is not an unsigned one.
  const [acceptUnscanned, setAcceptUnscanned] = useState(false);
  // The registry's verdict on the declared MCP servers (agentry#177): a
  // known high/critical advisory refuses the run unless this override
  // (recorded in the receipt) or the operator policy allows it.
  const [mcp, setMcp] = useState<main.MCPView | null>(null);
  const [allowVulnerableMCP, setAllowVulnerableMCP] = useState(false);

  // Run state.
  const [running, setRunning] = useState(false);
  const [log, setLog] = useState<string[]>([]);
  const [result, setResult] = useState<main.RunResultView | null>(null);
  const [runErr, setRunErr] = useState<string | null>(null);
  const logRef = useRef<HTMLPreElement | null>(null);

  // In Agentry (hosted runs M6): whether the mode is offered here, the run
  // the registry is executing, and a refused create rendered as a sentence.
  const [hosted, setHosted] = useState<main.HostedAvailabilityView | null>(null);
  const [hostedRun, setHostedRun] = useState<main.HostedRunView | null>(null);
  const [hostedRefusal, setHostedRefusal] = useState<main.HostedStartView | null>(null);
  const [starting, setStarting] = useState(false);
  const [cancelling, setCancelling] = useState(false);

  useEffect(() => {
    RunAPI.SetupStatus().then(setSetup).catch((e) => setSetupErr(String(e)));
    RunAPI.GatewayRequirements(registry, ns, name).then(setMcp).catch(() => setMcp(null));
    RunAPI.ProfileStatus(registry, agentRef)
      .then((p) => {
        setProfile(p);
        setMode(initialMode ?? ((p.modes ?? []).includes("autonomous") || (p.modes ?? []).length === 0 ? "autonomous" : p.modes[0]));
        setKeep(p.keep);
        setKeepTouched(false);
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

  // In Agentry: is the mode offered for this release, and does this
  // account already have a run the panel should reattach to (D11)?
  useEffect(() => {
    let live = true;
    RunAPI.HostedAvailability(registry, agentRef)
      .then((h) => {
        if (!live) return;
        setHosted(h);
        // Only when the mode can actually be used: listing this account's
        // runs needs run:read, and a token without it must not be sent to
        // a route that would refuse it.
        if (!h.available) return;
        return RunAPI.AttachHostedRun(registry, outDir.trim()).then((a) => {
          if (!live || !a.run) return;
          setMode("hosted");
          setHostedRun(a.run);
        });
      })
      .catch(() => setHosted(null));
    return () => { live = false; };
    // The attach reads the folder as it stands when the panel opens; a
    // later change applies to the next run.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [registry, agentRef]);

  useEffect(() => {
    const off = EventsOn("runner:hosted-run", (v: main.HostedRunView) => setHostedRun(v));
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
  // Unscanned: the registry admitted the version without a scan (plan
  // organization-subscriptions §4) — its own verdict, its own checkbox,
  // enforced in Go like the unsigned one. A run In Agentry is not
  // offered for one: the platform records no such consent yet
  // (agentry-enterprise#55).
  const unscanned = attestation?.state === "unscanned";
  const unverifiable = attestation != null && attestation.state !== "verified" && !unsigned && !unscanned;
  const refused = !!mcp?.refusal;
  const needsProfile = !!profile && profile.declaredSecrets.length > 0 && !profile.exists;
  const intakeMissing = (profile?.intake ?? []).filter((q) => q.required && !(intakeValues[q.id] ?? "").trim()).map((q) => q.id);
  // In Agentry needs no docker, no images and no operator policy on this
  // machine: the setup status gates the local modes only.
  const inAgentry = mode === "hosted";
  const hostedActive = !!hostedRun && (hostedRun.status === "queued" || hostedRun.status === "claimed" || hostedRun.status === "running");
  // What still stands between this form and a run, each in a sentence the
  // line above the button shows (agentry-desktop#167): the same conditions
  // the button reads, so the two cannot disagree. Busy is not a blocker.
  const busy = running || sessionActive || (inAgentry && (starting || hostedActive));
  const blockers: string[] = [];
  if (inAgentry) {
    if (!hosted) blockers.push("Checking whether this release can run In Agentry.");
    else if (!hosted.available) blockers.push(hosted.reason || "A run In Agentry is not available for this release.");
  } else if (!setup) {
    blockers.push("Checking this machine's setup.");
  } else if (!setup.dockerOk) {
    blockers.push("Docker is not ready — set it up in Settings → Agent runs" + (hosted?.offered ? ", or run this release In Agentry." : "."));
  }
  if (needsProfile && profile) {
    const servers = mcp?.servers ?? [];
    const server = servers.length === 1 ? servers[0] : "this release";
    blockers.push(`Save the keys for ${server}: ${profile.declaredSecrets.join(", ")} (or leave them to the environment).`);
  }
  if (intakeMissing.length > 0 && profile) {
    for (const q of profile.intake.filter((q) => intakeMissing.includes(q.id))) {
      blockers.push(`${q.type === "file" ? "Choose" : "Answer"}: ${q.prompt}`);
    }
  }
  if (provPending) blockers.push("Waiting for the provenance check on this version — if this persists, the registry could not answer; close and reopen the page.");
  if (unverifiable) blockers.push("The attestation could not be verified; the runner refuses such a release.");
  if (!inAgentry && unsigned && !acceptUnsigned) blockers.push("Tick \u201cRun an unsigned release\u201d.");
  if (unscanned && inAgentry) blockers.push("A run In Agentry is not offered for an unscanned release.");
  if (unscanned && !inAgentry && !acceptUnscanned) blockers.push("Tick \u201cRun an unscanned release\u201d.");
  if (refused && !allowVulnerableMCP) blockers.push("Tick \u201cAllow vulnerable MCP servers for this run\u201d.");
  const canRun = !busy && blockers.length === 0;

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

  function intakeAnswers(): Record<string, string> {
    const answers: Record<string, string> = {};
    for (const [k, v] of Object.entries(intakeValues)) if (v.trim() !== "") answers[k] = v;
    return answers;
  }

  function startSession() {
    setRunErr(null);
    setResult(null);
    setLog([]);
    setSessionActive(true);
  }

  // In Agentry: the inputs go up, the run is created, and the log streams
  // back into the same view. A refused create is a sentence, not an error.
  async function runInAgentry() {
    setStarting(true);
    setRunErr(null);
    setResult(null);
    setHostedRefusal(null);
    setHostedRun(null);
    setLog([]);
    try {
      const started = await RunAPI.StartHostedRun(registry, agentRef, task, inputs, outDir.trim(), intakeAnswers());
      if (started.refusal) {
        setHostedRefusal(started);
        return;
      }
      setHostedRun(started.run ?? null);
    } catch (e) {
      setRunErr(String(e));
    } finally {
      setStarting(false);
    }
  }

  async function cancelInAgentry() {
    setCancelling(true);
    try {
      setHostedRun(await RunAPI.CancelHostedRun());
    } catch (e) {
      setRunErr(String(e));
    } finally {
      setCancelling(false);
    }
  }

  async function runAgent() {
    if (mode === "hosted") {
      await runInAgentry();
      return;
    }
    if (mode === "interactive") {
      startSession();
      return;
    }
    setRunning(true);
    setRunErr(null);
    setResult(null);
    setLog([]);
    try {
      const t = timeoutSec.trim() === "" ? 0 : Math.max(0, parseInt(timeoutSec, 10) || 0);
      const mt = maxTurns.trim() === "" ? -1 : Math.max(0, parseInt(maxTurns, 10) || 0);
      const r = await RunAPI.RunAgent(registry, agentRef, task, inputs, outDir.trim(), keepTouched ? keep : null, t, mt, unsigned && acceptUnsigned, intakeAnswers(), refused && allowVulnerableMCP, unscanned && acceptUnscanned);
      setResult(r);
    } catch (e) {
      setRunErr(String(e));
    } finally {
      setRunning(false);
    }
  }

  return (
    <div className="modal-backdrop" onClick={() => !running && !sessionActive && onClose()}>
      <div className="modal modal-wide" onClick={(e) => e.stopPropagation()}>
        <div className="spread">
          <h2 style={{ margin: 0 }}>Run {agentRef}</h2>
          <Button onClick={onClose} disabled={running || sessionActive} title={running || sessionActive ? "A run is in progress" : ""}>✕</Button>
        </div>
        <p className="muted small">
          Runs in an isolated container; tools come only from a per-run Docker MCP Gateway
          serving exactly the declared contract. Reports and receipts land in the output folder.
        </p>

        {/* Machine readiness (§P1) — the local modes only; a run In
            Agentry needs nothing from this machine. */}
        {setupErr && !inAgentry && <ErrorBox error={setupErr} />}
        {setup && !setup.dockerOk && !inAgentry && (
          <div className="error-box selectable">
            Docker is not available — a BriefAgent run on this machine needs a docker engine.
            {setup.dockerErr ? ` (${setup.dockerErr})` : ""} Configure it, then reopen this panel
            {hosted?.offered ? ", or run this release In Agentry" : ""}.
          </div>
        )}
        {setup && setup.dockerOk && !setup.imagesReady && !inAgentry && (
          <Notice tone="warn">
            The gateway/harness/runner images are not all present yet — this run will install
            them first (it can take a few minutes), or prepare them once in Settings → Agent runs.
          </Notice>
        )}

        {/* Declared contract: what a Run click accepts. */}
        {profile && (
          <>
            <h3>Declared contract</h3>
            <Card className="selectable">
              <div className="kv"><span className="k">release</span><span className="mono">{profile.ref}</span></div>
              <div className="kv"><span className="k">digest</span><span className="mono" title={profile.digest}>{shortDigest(profile.digest)}</span></div>
              {profile.capabilityLines.map((l) => (
                <div key={l} className="mono small">{l}</div>
              ))}
            </Card>
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
                <Button onClick={() => { setFormOpen(true); setFormValues({}); }}>Reconfigure…</Button>
              </div>
            )}
            {formOpen && (
              <Card>
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
                  {profile.exists && <Button onClick={() => setFormOpen(false)} disabled={savingProfile}>Cancel</Button>}
                  <Button variant="primary" onClick={saveProfile} disabled={savingProfile}>
                    {savingProfile ? "Saving…" : "Save keys"}
                  </Button>
                </div>
              </Card>
            )}
          </>
        )}

        {/* Up-front questions (manifest v2 intake, plan §3.5): the same form
            shape as the profile; answers are staged read-only for the agent. */}
        {profile && profile.intake.length > 0 && (
          <>
            <h3>Questions from this BriefAgent</h3>
            <Card>
              <p className="muted small" style={{ marginTop: 0 }}>
                Answered before the run starts and staged read-only at <span className="mono">/inputs/intake.json</span>;
                a secret answer is injected like a declared key and never shown to the agent.
              </p>
              {profile.intake.map((q) => (
                <div className="row" key={q.id} style={{ marginBottom: 8 }}>
                  <span style={{ minWidth: 260 }}>{q.prompt}{q.required ? " *" : ""}</span>
                  {q.type === "choice" ? (
                    <select value={intakeValues[q.id] ?? ""} onChange={(e) => setIntakeValues({ ...intakeValues, [q.id]: e.target.value })} disabled={running}>
                      <option value="">—</option>
                      {q.choices.map((c) => <option key={c} value={c}>{c}</option>)}
                    </select>
                  ) : q.type === "file" ? (
                    <>
                      <input type="text" className="grow mono" value={intakeValues[q.id] ?? ""} readOnly placeholder="a file on this machine — choose it" disabled={running} />
                      <Button
                        disabled={running}
                        onClick={async () => {
                          try {
                            const files = await ChooseFiles(q.prompt);
                            if (files.length) setIntakeValues({ ...intakeValues, [q.id]: files[0] });
                          } catch (e) {
                            setRunErr(String(e));
                          }
                        }}
                      >
                        Choose…
                      </Button>
                    </>
                  ) : (
                    <input
                      type={q.type === "secret" ? "password" : "text"}
                      className="grow"
                      autoComplete="off"
                      value={intakeValues[q.id] ?? ""}
                      onChange={(e) => setIntakeValues({ ...intakeValues, [q.id]: e.target.value })}
                      disabled={running}
                    />
                  )}
                </div>
              ))}
              {intakeMissing.length > 0 && <p className="muted small">Required: {intakeMissing.join(", ")}</p>}
            </Card>
          </>
        )}

        {/* Run mode (plan §7): the modes the release declares, plus In
            Agentry where the registry runs it (hosted-runs.md §4.7). */}
        {profile && (profile.modes ?? []).length > 0 && (
          <>
            <h3>Run mode</h3>
            <div className="row" style={{ gap: 14 }}>
              {(["autonomous", "interactive"] as const).filter((m) => profile.modes.includes(m)).map((m) => (
                <label key={m} className="check-row" style={{ margin: 0 }}>
                  <input type="radio" name="run-mode" checked={mode === m} onChange={() => setMode(m)} disabled={running || sessionActive || hostedActive} />
                  <span>
                    <strong>{m}</strong>{" "}
                    <span className="muted">{m === "autonomous" ? "— runs from its files and ends" : "— a chat in this window under exactly the same controls"}</span>
                  </span>
                </label>
              ))}
              {hosted?.offered && (
                <label className="check-row" style={{ margin: 0 }}>
                  <input
                    type="radio" name="run-mode" checked={inAgentry} onChange={() => setMode("hosted")}
                    disabled={running || sessionActive || !hosted.available || unscanned}
                  />
                  <span>
                    <strong>In Agentry</strong>{" "}
                    <span className="muted">— runs on the platform, autonomously; nothing runs on this machine</span>
                  </span>
                </label>
              )}
            </div>
            {mode === "interactive" && (
              <p className="muted small">
                Interactive: you can talk to this agent and give it files; you cannot give it more access than listed above.
              </p>
            )}
            {hosted && <HostedUnavailable hosted={hosted} />}
            {inAgentry && hosted && (
              <>
                <p className="muted small" style={{ marginBottom: 2 }}>
                  The task, your answers and the input files go to {registry}, which runs this release under the same
                  contract and sends back the log, the outputs and the receipt. This machine needs no docker engine and
                  no operator policy for it.
                </p>
                <HostedCredits hosted={hosted} />
                {unsigned && (
                  <Notice tone="warn">
                    This registry published no attestation for this version. A run In Agentry verifies the release
                    itself before executing it, so this one is refused there and the credits it held are refunded.
                  </Notice>
                )}
              </>
            )}
          </>
        )}

        {/* Run form */}
        <h3>Task</h3>
        <Textarea
          className="mono"
          rows={4}
          placeholder="What should this BriefAgent do? (empty = the packaged instruction runs with no extra task)"
          value={task}
          onChange={(e) => setTask(e.target.value)}
          disabled={running}
        />

        {/* Input files are never required: no manifest declares a need for
            them, and a release that needs one asks a file intake question.
            So they are a disclosure, opened when a file is chosen and left
            to the person otherwise (agentry-desktop#167). */}
        <details className="advanced input-files" open={inputs.length > 0 || undefined}>
          <summary className="muted small">Input files (optional)</summary>
          <p className="muted small" style={{ margin: "2px 0 6px" }}>
            Provided read-only under <span className="mono">/inputs</span> in the tool container —
            the BriefAgent reads them with <span className="mono">list_inputs</span> / <span className="mono">read_input</span>.
            {inAgentry && hosted && (
              <> In Agentry they are uploaded before the run is created: at most {hosted.maxInputFiles} files,{" "}
              {Math.round(hosted.maxInputBytes / (1024 * 1024))} MB together.</>
            )}
          </p>
          {inputs.length > 0 && (
            <div>
              {inputs.map((p) => (
                <div key={p} className="row" style={{ marginBottom: 4 }}>
                  <span className="mono small grow selectable">{p}</span>
                  <Button disabled={running} onClick={() => setInputs(inputs.filter((x) => x !== p))}>Remove</Button>
                </div>
              ))}
            </div>
          )}
          <div className="row">
            <Button onClick={addFiles} disabled={running}>Add files…</Button>
            <Button onClick={addFolder} disabled={running}>Add folder…</Button>
          </div>
        </details>

        <h3>Output</h3>
        {inAgentry && (
          <p className="muted small" style={{ margin: "2px 0 6px" }}>
            The run&apos;s result, reports, receipts and outputs download into this folder when it finishes. The time and
            turn ceilings are the tier&apos;s, not this form&apos;s.
          </p>
        )}
        <div className="row">
          {/* Filled by the picker only: Go accepts a path a native dialog
              returned in this process and refuses one the page typed (D-030). */}
          <input
            type="text"
            className="grow mono"
            placeholder={`output folder — default ${defaultOutHint}`}
            value={outDir}
            readOnly
            disabled={running}
          />
          <Button onClick={pickOutDir} disabled={running}>Choose…</Button>
          {outDir && <Button onClick={() => setOutDir("")} disabled={running}>Default</Button>}
        </div>
        {/* The ceilings have the policy's defaults and the keep toggle its
            recorded choice: none is needed for a run, so they sit under
            Advanced (agentry-desktop#167). */}
        <details className="advanced">
          <summary className="muted small">Advanced</summary>
        <div className="row">
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
            <input type="checkbox" checked={keep} onChange={(e) => { setKeep(e.target.checked); setKeepTouched(true); }} disabled={running} />
            <span>keep the MCP profile after this run{profile && profile.declaredSecrets.length === 0 ? " (no keys declared; ticking records the choice)" : ""}</span>
          </label>
        </div>
        </details>

        {unscanned && (
          <div className="notice notice-unscanned mark-unscanned">
            <p style={{ margin: 0 }}>
              <SignedBadge att={attestation} />{" "}
              <strong>The registry admitted this version without a scan.</strong>{" "}
              <span className="muted">
                Its organization&apos;s administrator took the agent off the pipeline: no findings, no report and no
                attestation stand behind it. Nothing about its contents was checked.
                {hosted?.offered ? " A run In Agentry is not offered for it." : ""}
              </span>
            </p>
            <label className="check-row">
              <input type="checkbox" checked={acceptUnscanned} onChange={(e) => setAcceptUnscanned(e.target.checked)} />
              <span>
                <strong>Run an unscanned release.</strong>{" "}
                <span className="muted">
                  Its own decision — accepting an unsigned release does not cover it. The run&apos;s receipts are
                  marked unscanned and name this consent.
                </span>
              </span>
            </label>
          </div>
        )}
        {unsigned && !inAgentry && (
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
        {refused && (
          <label className="check-row">
            <input type="checkbox" checked={allowVulnerableMCP} onChange={(e) => setAllowVulnerableMCP(e.target.checked)} />
            <span>
              <strong>Allow vulnerable MCP servers for this run.</strong>{" "}
              <span className="muted selectable">
                {mcp?.refusal}. The override is recorded in the run&apos;s receipt.
              </span>
            </span>
          </label>
        )}
        {!refused && mcp?.vulnerabilityLine && (
          <p className="muted small">Declared MCP servers {mcp.vulnerabilityLine}.</p>
        )}
        {unverifiable && (
          <div className="error-box selectable">
            Attestation {attestation?.state}: {attestation?.detail} — the runner re-verifies before
            executing and refuses a release whose signature cannot be verified.
          </div>
        )}
        <ErrorBox error={runErr} />

        {/* A grey button explains itself: what is still missing, from the
            conditions the button reads (agentry-desktop#167). */}
        {!busy && blockers.length > 0 && (
          <div className="muted small run-blockers">
            <strong>Before you can run:</strong>
            <ul style={{ margin: "2px 0 0", paddingLeft: 18 }}>
              {blockers.map((b) => <li key={b}>{b}</li>)}
            </ul>
          </div>
        )}

        {/* A refused create, as a sentence — and, for credits, the way
            onward the Build page already offers (desktop#74). */}
        {hostedRefusal?.refusal && (
          <Notice tone="warn">
            <strong>
              {hostedRefusal.refusal === "credits-exhausted"
                ? "Not enough credits for this run."
                : hostedRefusal.refusal === "run-active"
                  ? "This account already has a run In Agentry."
                  : "The registry refused this run."}
            </strong>{" "}
            <span className="selectable">{hostedRefusal.detail}</span>
            {hostedRefusal.refusal === "credits-exhausted" && (
              <p className="small" style={{ marginBottom: 0 }}>
                {/* The destination is the registry's to name and the app's
                    to check, so this asks for the registry and never hands
                    a URL to be opened (agentry-enterprise#41). */}
                <Button onClick={() => SettingsAPI.OpenUpgrade(registry).catch((e) => setRunErr(String(e)))}>
                  Credits and upgrade
                </Button>
              </p>
            )}
          </Notice>
        )}

        <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
          <Button onClick={onClose} disabled={running}>Close</Button>
          <Button variant="primary" disabled={!canRun} onClick={runAgent}>
            {inAgentry
              ? starting ? "Starting…" : hostedActive ? "Running In Agentry…" : "Run In Agentry"
              : running ? "Running…" : mode === "interactive" ? "Start session" : "Run"}
          </Button>
        </div>

        {/* The hosted run: where it stands, and what it produced. */}
        {inAgentry && hostedRun && (
          <>
            <h3>Run In Agentry</h3>
            <HostedProgress run={hostedRun} onCancel={cancelInAgentry} cancelling={cancelling} />
          </>
        )}

        {/* Interactive session: the chat replaces the log pane. */}
        {sessionActive && (
          <SessionChat
            registry={registry}
            agentRef={agentRef}
            task={task}
            inputs={inputs}
            outDir={outDir.trim()}
            keepProfile={keepTouched ? keep : null}
            timeoutSec={timeoutSec.trim() === "" ? 0 : Math.max(0, parseInt(timeoutSec, 10) || 0)}
            maxTurns={maxTurns.trim() === "" ? -1 : Math.max(0, parseInt(maxTurns, 10) || 0)}
            acceptUnsigned={unsigned && acceptUnsigned}
            acceptUnscanned={unscanned && acceptUnscanned}
            allowVulnerableMCP={refused && allowVulnerableMCP}
            intakeAnswers={intakeAnswers()}
            opening={profile?.opening ?? ""}
            onResult={(r, err) => {
              setSessionActive(false);
              if (r) setResult(r);
              if (err) setRunErr(err);
            }}
          />
        )}

        {/* Live log — the same view for either kind of run. */}
        {(running || hostedActive || log.length > 0) && (
          <>
            <h3>Run log</h3>
            <pre ref={logRef} className="run-log selectable">{log.join("\n")}{running || hostedActive ? "\n…" : ""}</pre>
            {running && <Spinner label="Running — the container topology streams its progress above." />}
            {hostedActive && (
              <Spinner label={hostedRun?.status === "queued"
                ? "Queued In Agentry — the log starts when a worker takes it."
                : "Running In Agentry — the platform streams its log above."} />
            )}
          </>
        )}

        {/* The finished hosted run: the same facts a local result shows. */}
        {inAgentry && hostedRun?.final && (
          <>
            <h3>Result</h3>
            <HostedResult run={hostedRun}>
              <ResultBody raw={hostedRun.resultJson ?? ""} />
            </HostedResult>
          </>
        )}

        {/* Result envelope (§R8) */}
        {result && (
          <>
            <h3>Result</h3>
            <Card className="selectable">
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
            </Card>
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
