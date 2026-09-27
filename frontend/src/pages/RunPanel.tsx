// The run surface (RUNNER-PLAN-2 §P6): run a BriefAgent from the app with
// the same lifecycle `agentry run` executes.
//
// Since the guided run setup (plan guided-runs.md §4, M3;
// agentry-desktop#226) the panel asks one step at a time, on the first-run
// wizard's shell: Before you start (what it does, what to have ready, where
// to run, this machine's readiness), Files, Connect tools (the declared MCP
// servers and the keys of the MCP profile, §P3: masked inputs, names only,
// an empty field means "resolve from the environment at run time"),
// Details (the intake questions, then the task) and Review and start (the
// recap, the output folder, Advanced, the consents and Run). A step the
// release does not need is left out. Every gate is where it was: the
// blockers the Run button reads, the native consent dialog on Run
// (runGates), and paths from native pickers only (D-030). Once a run
// starts, the steps give way to the run itself: the log, the chat or the
// hosted run's progress, and the result.
import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import * as KnowledgeAPI from "../../wailsjs/go/main/KnowledgeAPI";
import { ChooseDirectory, ChooseFiles } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { knowledge, main, trust } from "../../wailsjs/go/models";
import { Button, Card, DeclaredServersTable, ErrorBox, Notice, SignedBadge, Spinner, Textarea, fmtBytes, shortDigest } from "../ui";
import { STEP_LABEL, needList, setupSteps, type StepKey } from "../runSetup";
import SessionChat from "./SessionChat";
import { HostedCredits, HostedProgress, HostedResult, HostedUnavailable } from "./HostedRun";
import { RunResultView } from "./RunResult";
import { AgentMarkdown } from "../AgentMarkdown";
import { GuideText, connectHelp } from "../GuideText";
import { attachable, attachableInAgentry, inAgentryLine, pagesOf, scanTone, tooManyPacks } from "../knowledge";

export default function RunPanel({
  registry, ns, name, version, summary, attestation, onClose, initialMode, initialStep,
}: {
  registry: string;
  ns: string;
  name: string;
  version: string;
  /** The BriefAgent's summary, for Before you start. */
  summary?: string;
  /** The page's provenance verification (declared digest); the run
   * re-verifies the downloaded bytes either way. */
  attestation: trust.AttestationResult | null;
  onClose: () => void;
  /** Harness-only: open with this run mode selected, for screenshots. */
  initialMode?: string;
  /** Harness-only: open on this step, for screenshots. */
  initialStep?: StepKey;
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
  // The files among the inputs, as the native file picker returned them: a
  // file intake question may pick one of these (plan §4, Details), and a
  // folder is never offered for it.
  const [pickedFiles, setPickedFiles] = useState<string[]>([]);
  // The knowledge packs listed for the Knowledge step (plan knowledge.md
  // §4.5) — those kept here can be attached, those kept only in Agentry are
  // shown with where to download them — and the ones to attach, by name; Go
  // opens each from the store after the gates, and the receipt names what
  // was mounted.
  const [packs, setPacks] = useState<knowledge.Pack[] | null>(null);
  const [attach, setAttach] = useState<string[]>([]);
  // A pack kept only here being uploaded from the Knowledge step, so a run
  // In Agentry can attach it once Agentry has scanned it; and what the
  // upload said.
  const [uploading, setUploading] = useState<string | null>(null);
  const [uploadNote, setUploadNote] = useState<string | null>(null);
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

  // The guided setup: which step shows, a step's own sentence when Next is
  // refused, and the heading focus moves to when the step changes.
  const [stepKey, setStepKey] = useState<StepKey>(initialStep ?? "start");
  const [stepProblem, setStepProblem] = useState<string | null>(null);
  const stepHeading = useRef<HTMLHeadingElement | null>(null);
  const moved = useRef(false);
  // The author's guide for this exact release (manifest v2, DEV-141), if any.
  const guide = profile?.guide ?? null;

  useEffect(() => {
    RunAPI.SetupStatus().then(setSetup).catch((e) => setSetupErr(String(e)));
    RunAPI.GatewayRequirements(registry, ns, name).then(setMcp).catch(() => setMcp(null));
    KnowledgeAPI.List(registry).then((l) => setPacks(l.packs)).catch(() => setPacks([]));
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

  // What the release says about knowledge (plan knowledge.md §4.6): the
  // packs it ships, mounted in every run on this computer, and what it asks
  // the person to attach, shown here and never to the agent.
  const ships = profile?.shipsKnowledge ?? [];
  const accepts = profile?.acceptsKnowledge;
  const steps = setupSteps({
    servers: mcp?.declaredServers?.length ?? 0,
    secrets: profile?.declaredSecrets.length ?? 0,
    // In Agentry, only where the registry attaches packs to a hosted run
    // and this computer's token may read them (agentry-desktop#275); on
    // this computer also when the release ships a pack or asks for one.
    knowledge: inAgentry
      ? ((packs?.length ?? 0) > 0 || !!accepts) && !!hosted?.knowledgeAttach
      : (packs?.length ?? 0) > 0 || ships.length > 0 || !!accepts,
  });
  // What a run attaches: the ticked packs it can take where it runs — kept
  // here for this machine, kept and scanned in Agentry for a run there.
  const canAttach = inAgentry ? attachableInAgentry : attachable;
  // Nothing is attached from a step the setup does not show: a pack ticked
  // for this machine does not ride along to a run In Agentry that offers
  // none, and the review shows exactly what goes.
  const attached = steps.includes("knowledge") ? attach.filter((n) => (packs ?? []).some((p) => canAttach(p) && p.name === n)) : [];
  // A release that requires a pack of the person's own cannot run without one.
  const needsOwnPack = !!accepts?.required && steps.includes("knowledge") && attached.length === 0;
  if (needsOwnPack) blockers.push("This release needs a knowledge pack of your own: attach one on the Knowledge step.");
  const canRun = !busy && blockers.length === 0;
  // A step that went away (the tools step, once the release's shape is
  // known) falls back to the first one rather than to nothing.
  const at = Math.max(0, steps.indexOf(stepKey));
  const step = steps[at];
  // Once a run is under way, or has left something to show, the panel is
  // the run, not the setup.
  const started = running || sessionActive || !!result || log.length > 0 || (inAgentry && !!hostedRun);

  // Focus follows the step, and returns to it when the run view gives way
  // to the setup again.
  useEffect(() => {
    if (moved.current && !started) stepHeading.current?.focus();
  }, [step, started]);

  function goTo(i: number) {
    moved.current = true;
    setStepProblem(null);
    setStepKey(steps[Math.max(0, Math.min(steps.length - 1, i))]);
  }

  // What stops Next on this step, as a sentence: the keys while the form is
  // open with none saved, and a required question left unanswered. The rest
  // of the blockers are the review's, beside Run.
  function problemHere(): string | null {
    if (step === "tools" && formOpen) {
      return needsProfile
        ? "Save the keys to go on. A field left empty is read from this machine's environment at run time."
        : "Save the keys or cancel the change to go on.";
    }
    if (step === "knowledge" && needsOwnPack) {
      return "This release needs a knowledge pack of your own. Attach one to go on.";
    }
    if (step === "knowledge" && inAgentry) {
      const p = tooManyPacks(attached.length, hosted?.maxPacks);
      if (p) return p;
    }
    if (step === "details" && profile) {
      const q = profile.intake.find((q) => intakeMissing.includes(q.id));
      if (q) return `${q.type === "file" ? "Choose" : "Answer"}: ${q.prompt}`;
    }
    return null;
  }

  function next() {
    const p = problemHere();
    if (p) {
      setStepProblem(p);
      stepHeading.current?.focus();
      return;
    }
    goTo(at + 1);
  }

  // Back to the setup from a finished run, to change something and run it
  // again. A hosted run in flight keeps the panel on the run.
  function setUpAgain() {
    setResult(null);
    setLog([]);
    setRunErr(null);
    if (!hostedActive) setHostedRun(null);
    setHostedRefusal(null);
    moved.current = true;
    setStepKey("review");
  }

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
      if (files.length) {
        setInputs((prev) => [...prev, ...files.filter((f) => !prev.includes(f))]);
        setPickedFiles((prev) => [...prev, ...files.filter((f) => !prev.includes(f))]);
      }
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

  // In Agentry: the inputs go up, the run is created, and the panel follows
  // where it stands until the outputs come back. A refused create is a
  // sentence, not an error.
  async function runInAgentry() {
    setStarting(true);
    setRunErr(null);
    setResult(null);
    setHostedRefusal(null);
    setHostedRun(null);
    setLog([]);
    try {
      const started = await RunAPI.StartHostedRun(registry, agentRef, task, inputs, outDir.trim(), intakeAnswers(), attached);
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

  // Upload a pack kept here so a run In Agentry can attach it (the
  // Knowledge panel's own Upload): Agentry scans it first, so it can be
  // ticked once the list, read again, says it is ready there.
  async function uploadForAgentry(name: string) {
    setUploading(name);
    setUploadNote(null);
    try {
      const said = await KnowledgeAPI.Upload(registry, name);
      const l = await KnowledgeAPI.List(registry);
      setPacks(l.packs);
      const now = l.packs.find((p) => p.name === name && p.inAgentry);
      setUploadNote(
        now && now.agentryReady
          ? `${name} is in Agentry and scanned; it can be attached.`
          : `${said || `${name} was uploaded.`} Agentry scans it before a run can attach it; come back to this step once the scan ends.`,
      );
    } catch (e) {
      setUploadNote(String(e));
    } finally {
      setUploading(null);
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
      const r = await RunAPI.RunAgent(registry, agentRef, task, inputs, outDir.trim(), keepTouched ? keep : null, t, mt, unsigned && acceptUnsigned, intakeAnswers(), refused && allowVulnerableMCP, unscanned && acceptUnscanned, attached);
      setResult(r);
    } catch (e) {
      setRunErr(String(e));
    } finally {
      setRunning(false);
    }
  }

  const runLabel = inAgentry
    ? starting ? "Starting…" : hostedActive ? "Running In Agentry…" : "Run In Agentry"
    : running ? "Running…" : mode === "interactive" ? "Start session" : "Run";
  const modeWord = inAgentry ? "In Agentry" : mode === "interactive" ? "an interactive session on this machine" : "on this machine, autonomously";
  const answered = (profile?.intake ?? []).filter((q) => q.type !== "secret");

  return (
    <div className="modal-backdrop" onClick={() => !running && !sessionActive && onClose()}>
      <div className="modal modal-wide run-panel" role="dialog" aria-modal="true" aria-label={`Run ${agentRef}`} onClick={(e) => e.stopPropagation()}>
        <div className="spread">
          <h2 style={{ margin: 0 }}>Run {agentRef}</h2>
          <Button onClick={onClose} disabled={running || sessionActive} title={running || sessionActive ? "A run is in progress" : ""} aria-label="Close">✕</Button>
        </div>

        {!started && (
          <ol className="run-steps" aria-label="Steps">
            {steps.map((k, i) => (
              <li key={k} aria-current={i === at ? "step" : undefined} className={i === at ? "active" : i < at ? "done" : ""}>
                <span className={`wizard-dot${i === at ? " active" : ""}${i < at ? " done" : ""}`} aria-hidden />
                <span>{STEP_LABEL[k]}</span>
              </li>
            ))}
          </ol>
        )}

        {!started && (
          <section className="run-step" aria-labelledby="run-step-heading">
            <h3 id="run-step-heading" ref={stepHeading} tabIndex={-1} className="run-step-heading">
              Step {at + 1} of {steps.length}: {STEP_LABEL[step]}
            </h3>

            {step === "start" && (
              <>
                <h4>What it does</h4>
                {/* The author's guide (manifest v2, DEV-141; agentry-desktop#228)
                    speaks first when the release has one: the overview is
                    markdown under the same rules as a run's output, and the
                    prepare list is shown as text. Without one, the summary and
                    the derived list stand in. */}
                {guide?.overview ? (
                  <AgentMarkdown text={guide.overview} />
                ) : (
                  <p className="selectable">{summary || "Its author wrote no summary. Its README is on its page."}</p>
                )}
                <h4>What to have ready</h4>
                <ul className="run-need">
                  {(guide && guide.prepare.length > 0
                    ? guide.prepare
                    : needList({ intake: profile?.intake ?? [], secrets: profile?.declaredSecrets ?? [], servers: mcp?.declaredServers?.length ?? 0, files: guide?.files })
                  ).map((line, i) => <li key={i}><GuideText text={line} /></li>)}
                </ul>
                <h4>Where to run it</h4>
                {/* Run mode (plan §7): the modes the release declares, plus In
                    Agentry where the registry runs it (hosted-runs.md §4.7). */}
                {profile && (profile.modes ?? []).length > 0 && (
                  <>
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
                        Interactive: you can talk to this agent and give it files; you cannot give it more access than this release declares.
                      </p>
                    )}
                    {hosted && <HostedUnavailable hosted={hosted} />}
                    {inAgentry && hosted && (
                      <>
                        <p className="muted small" style={{ marginBottom: 2 }}>
                          The task, your answers and the input files go to {registry}, which runs this release under the same
                          contract and sends back the outputs and the receipt; it keeps no log of the run. This machine needs no
                          docker engine and no operator policy for it.
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
                {profile && profile.capabilityLines.length > 0 && (
                  <>
                    <h4>What it may reach</h4>
                    <ul className="run-need selectable">
                      {profile.capabilityLines.map((l) => <li key={l}>{l}</li>)}
                    </ul>
                  </>
                )}
                {unscanned && hosted?.offered && (
                  <p className="muted small">A run In Agentry is not offered for this release: the registry admitted it without a scan.</p>
                )}
                <details className="advanced">
                  <summary className="muted small">The declared contract, with its digest</summary>
                  {/* Declared contract: what a Run click accepts. */}
                  {profile && (
                    <>
                      <Card className="selectable">
                        <div className="kv"><span className="k">release</span><span className="mono">{profile.ref}</span></div>
                        <div className="kv"><span className="k">digest</span><span className="mono" title={profile.digest}>{shortDigest(profile.digest)}</span></div>
                        {profile.capabilityLines.map((l) => (
                          <div key={l} className="mono small">{l}</div>
                        ))}
                      </Card>
                    </>
                  )}
                </details>
              </>
            )}

            {step === "files" && (
              <>
                {guide?.files && (
                  <p>
                    <GuideText text={guide.files} />
                  </p>
                )}
                <p className="muted small">
                  Add the files the task is about. They are provided read-only under{" "}
                  <span className="mono">/inputs</span>, and the BriefAgent reads them with{" "}
                  <span className="mono">list_inputs</span> and <span className="mono">read_input</span>. If it needs none, go on
                  without any.
                  {inAgentry && hosted && (
                    <> In Agentry they are uploaded when the run starts: at most {hosted.maxInputFiles} files,{" "}
                    {Math.round(hosted.maxInputBytes / (1024 * 1024))} MB together.</>
                  )}
                </p>
                {inputs.length > 0 ? (
                  <div>
                    {inputs.map((p) => (
                      <div key={p} className="row" style={{ marginBottom: 4 }}>
                        <span className="mono small grow selectable">{p}</span>
                        <Button
                          onClick={() => {
                            setInputs(inputs.filter((x) => x !== p));
                            setPickedFiles(pickedFiles.filter((x) => x !== p));
                          }}
                          aria-label={`Remove ${p}`}
                        >
                          Remove
                        </Button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="muted small">No files chosen.</p>
                )}
                <div className="row">
                  <Button onClick={addFiles}>Add files…</Button>
                  <Button onClick={addFolder}>Add folder…</Button>
                </div>
              </>
            )}

            {step === "knowledge" && (
              <>
                {accepts && (
                  <p className="selectable">
                    <strong>This release asks:</strong> {accepts.prompt}
                    {accepts.required ? " (required)" : ""}
                  </p>
                )}
                {!inAgentry && ships.length > 0 && (
                  <p className="muted small">
                    The release ships {ships.length === 1 ? "a knowledge pack" : "knowledge packs"} of its own:{" "}
                    {ships.map((n, i) => (
                      <Fragment key={n}>
                        {i > 0 ? ", " : ""}
                        <span className="mono">{n}</span>
                      </Fragment>
                    ))}
                    . {ships.length === 1 ? "It is" : "Each is"} mounted read-only at <span className="mono">/knowledge/&lt;name&gt;</span> from the release&apos;s own
                    files, the ones its attestation covers, in every run; the packs below are yours to add beside them.
                  </p>
                )}
                {(packs?.length ?? 0) === 0 && (
                  <p className="muted small">No knowledge pack is kept on this computer. Make one on the Knowledge page.</p>
                )}
                {inAgentry ? (
                  <p className="muted small">
                    Attach knowledge packs Agentry keeps for your account. Each is mounted read-only at{" "}
                    <span className="mono">/knowledge/&lt;name&gt;</span> on the run&apos;s tools container only, on
                    Agentry&apos;s sandbox host, from the copy Agentry keeps and has scanned, and the BriefAgent reads it
                    with <span className="mono">knowledge_list</span>, <span className="mono">knowledge_search</span> and{" "}
                    <span className="mono">knowledge_read</span>; commands the release declares run in that container too,
                    so they can read it. Attaching one changes nothing the release may reach, and the receipt names each pack
                    and its digest.
                    {hosted?.maxPacks ? ` A run In Agentry attaches at most ${hosted.maxPacks}.` : ""}
                    {accepts?.required ? "" : " A run needs none."}
                  </p>
                ) : (
                  <p className="muted small">
                    Attach knowledge packs kept on this computer. Each is mounted read-only at{" "}
                    <span className="mono">/knowledge/&lt;name&gt;</span> on the run&apos;s tools container only, and the BriefAgent
                    reads it with <span className="mono">knowledge_list</span>, <span className="mono">knowledge_search</span> and{" "}
                    <span className="mono">knowledge_read</span>; commands the release declares run in that container too, so
                    they can read it. Attaching one changes nothing the release may reach, and the receipt names each pack and
                    its digest.{accepts?.required ? "" : " A run needs none."}
                  </p>
                )}
                <div className="knowledge-attach-list">
                {(packs ?? []).map((p) => (
                  <label key={`${p.name} ${p.local ? "here" : "agentry"}`} className="knowledge-attach">
                    <input
                      type="checkbox"
                      disabled={!canAttach(p)}
                      checked={canAttach(p) && attach.includes(p.name)}
                      onChange={(e) => setAttach(e.target.checked ? [...attach, p.name] : attach.filter((n) => n !== p.name))}
                    />
                    <span className="grow">
                      <span className="row nowrap knowledge-attach-head">
                        <span className="mono">{p.name}</span>
                        <span className={`badge ${scanTone(p)}`.trim()}>{p.scan}</span>
                      </span>
                      <span className="muted small" style={{ display: "block" }}>
                        {pagesOf(p.pages)} · {fmtBytes(p.bytes)} ·{" "}
                        {inAgentry
                          ? inAgentryLine(p)
                          : !attachable(p)
                            ? "kept in Agentry only: download it on the Knowledge page to attach it to a run here"
                            : p.opensOffline ? "opens offline" : `opens after ${p.registry} unwraps its key`}
                      </span>
                      {inAgentry && p.local && !p.shared && (!p.inAgentry || p.changed || (!p.agentryReady && p.scanState !== "scanning")) && (
                        <Button
                          variant="link"
                          disabled={uploading !== null}
                          onClick={(e) => {
                            e.preventDefault();
                            void uploadForAgentry(p.name);
                          }}
                        >
                          {uploading === p.name ? "Uploading…" : "Upload to Agentry"}
                        </Button>
                      )}
                    </span>
                  </label>
                ))}
                </div>
                {uploadNote && <p className="muted small" role="status">{uploadNote}</p>}
              </>
            )}

            {step === "tools" && (
              <>
                {(mcp?.declaredServers?.length ?? 0) > 0 && (
                  <>
                    <p className="muted small">
                      The MCP servers this release declares. They start with the run, serve only the tools listed, and stop
                      with it.
                    </p>
                    <DeclaredServersTable servers={mcp?.declaredServers} />
                  </>
                )}
                {/* Guided MCP-profile setup (§P3). */}
                {profile && profile.declaredSecrets.length > 0 && (
                  <>
                    <h4>Keys</h4>
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
                          <div key={s} className="run-key">
                            <span className="mono">{s}</span>
                            <input
                              type="password"
                              autoComplete="off"
                              aria-label={s}
                              placeholder="empty = resolve from the environment at run time"
                              value={formValues[s] ?? ""}
                              onChange={(e) => setFormValues({ ...formValues, [s]: e.target.value })}
                            />
                            {/* The author's help for this key: where to create
                                it, which access it needs. Text, never markup. */}
                            {connectHelp(guide?.connect, s) && (
                              <p className="muted small run-key-help">
                                <GuideText text={connectHelp(guide?.connect, s)} />
                              </p>
                            )}
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
                {profile && profile.declaredSecrets.length === 0 && (
                  <p className="muted small">This release declares no key, so there is nothing to sign in to.</p>
                )}
              </>
            )}

            {step === "details" && (
              <>
                {/* Up-front questions (manifest v2 intake, plan §3.5): the same form
                    shape as the profile; answers are staged read-only for the agent. */}
                {profile && profile.intake.length > 0 && (
                  <>
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
                          {pickedFiles.length > 0 ? (
                            <select
                              className="grow mono"
                              value={pickedFiles.includes(intakeValues[q.id] ?? "") ? intakeValues[q.id] : ""}
                              onChange={(e) => setIntakeValues({ ...intakeValues, [q.id]: e.target.value })}
                              aria-label={q.prompt}
                            >
                              <option value="">{(intakeValues[q.id] ?? "") && !pickedFiles.includes(intakeValues[q.id]) ? intakeValues[q.id] : "Choose one of the files you added"}</option>
                              {pickedFiles.map((f) => <option key={f} value={f}>{f}</option>)}
                            </select>
                          ) : (
                            <input type="text" className="grow mono" value={intakeValues[q.id] ?? ""} readOnly placeholder="a file on this machine — choose it" aria-label={q.prompt} />
                          )}
                          <Button
                            onClick={async () => {
                              try {
                                const files = await ChooseFiles(q.prompt);
                                if (files.length) setIntakeValues({ ...intakeValues, [q.id]: files[0] });
                              } catch (e) {
                                setRunErr(String(e));
                              }
                            }}
                          >
                            {pickedFiles.length > 0 ? "Another file…" : "Choose…"}
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
                <h4>What should the run do?</h4>
                <Textarea
                  className="mono"
                  rows={4}
                  placeholder={guide?.task || "Say it the way you would to a colleague. Left empty, its own instruction runs with no extra task."}
                  value={task}
                  onChange={(e) => setTask(e.target.value)}
                />
              </>
            )}

            {step === "review" && (
              <>
                <dl className="run-recap">
                  <dt>Where</dt>
                  <dd><span>{modeWord}</span><Button variant="link" onClick={() => goTo(0)} aria-label="Edit where to run">Edit</Button></dd>
                  <dt>Files</dt>
                  <dd>
                    <span className="mono small selectable">{inputs.length === 0 ? "none" : inputs.join(", ")}</span>
                    <Button variant="link" onClick={() => goTo(steps.indexOf("files"))} aria-label="Edit files">Edit</Button>
                  </dd>
                  {steps.includes("knowledge") && (
                    <>
                      <dt>Knowledge</dt>
                      <dd>
                        <span className="mono small selectable">
                          {[...(inAgentry ? [] : ships.map((n) => `${n} (shipped with the release)`)), ...attached].join(", ") || "none"}
                        </span>
                        <Button variant="link" onClick={() => goTo(steps.indexOf("knowledge"))} aria-label="Edit knowledge">Edit</Button>
                      </dd>
                    </>
                  )}
                  {inAgentry && !steps.includes("knowledge") && (packs?.length ?? 0) > 0 && hosted?.knowledgeNote && (
                    <>
                      <dt>Knowledge</dt>
                      <dd><span className="muted small">{hosted.knowledgeNote}</span></dd>
                    </>
                  )}
                  {profile && profile.declaredSecrets.length > 0 && (
                    <>
                      <dt>Keys</dt>
                      <dd>
                        <span>{profile.exists ? "saved for this release" : "not saved yet"}</span>
                        <Button variant="link" onClick={() => goTo(steps.indexOf("tools"))} aria-label="Edit keys">Edit</Button>
                      </dd>
                    </>
                  )}
                  {answered.map((q) => (
                    <Fragment key={q.id}>
                      <dt>{q.prompt}</dt>
                      <dd>
                        <span className="selectable">{(intakeValues[q.id] ?? "").trim() || "no answer"}</span>
                        <Button variant="link" onClick={() => goTo(steps.indexOf("details"))} aria-label={`Edit ${q.prompt}`}>Edit</Button>
                      </dd>
                    </Fragment>
                  ))}
                  <dt>The task</dt>
                  <dd>
                    <span className="selectable run-recap-task">{task.trim() || "none: its own instruction runs"}</span>
                    <Button variant="link" onClick={() => goTo(steps.indexOf("details"))} aria-label="Edit the task">Edit</Button>
                  </dd>
                </dl>
                {inAgentry && hosted && <HostedCredits hosted={hosted} />}
                <h4>Output</h4>
                {inAgentry && (
                  <p className="muted small" style={{ margin: "2px 0 6px" }}>
                    The run&apos;s result, reports, receipts and outputs download into this folder when it finishes. The time and
                    turn ceilings are the tier&apos;s, so Advanced is not offered for a run In Agentry.
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
                {!inAgentry && (
              <>
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
              </>
            )}
                {unscanned && (
                  <div className="notice notice-unscanned mark-unscanned">
                    <p style={{ margin: 0 }}>
                      <SignedBadge att={attestation} />{" "}
                      <strong>The registry admitted this version without a scan.</strong>{" "}
                      <span className="muted">
                        No findings, no report and no attestation stand behind it. Its organization&apos;s administrator took the
                        BriefAgent off the pipeline, and no scanner looked at its contents.
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
              </>
            )}

            {(step === "tools" || step === "start") && <ErrorBox error={profileErr} />}
            <ErrorBox error={runErr} />
            {stepProblem && (
              <p className="run-step-problem" role="alert">{stepProblem}</p>
            )}
            <div className="row run-step-foot">
              {at > 0 && <Button onClick={() => goTo(at - 1)}>Back</Button>}
              <span className="grow" />
              {step === "review" ? (
                <>
                  <Button onClick={onClose} disabled={running}>Close</Button>
                  <Button variant="primary" disabled={!canRun} onClick={runAgent}>{runLabel}</Button>
                </>
              ) : (
                <Button variant="primary" onClick={next}>Next: {STEP_LABEL[steps[at + 1]]}</Button>
              )}
            </div>
          </section>
        )}

        {started && <ErrorBox error={runErr} />}

        {/* The hosted run: where it stands, and what it produced. It keeps
            no log (agentry-desktop#182): the card moves as the run does. */}
        {/* While it is in flight; once it has ended, the result view below
            says how, and this card's facts are under its Run details. */}
        {inAgentry && hostedRun && !hostedRun.final && (
          <>
            <h3>Run In Agentry</h3>
            <HostedProgress run={hostedRun} onCancel={cancelInAgentry} cancelling={cancelling} />
            {hostedActive && (
              <Spinner label={hostedRun.status === "queued"
                ? "Queued In Agentry — this moves to running when a runner takes it."
                : "Running In Agentry — the outputs and the receipt come back when it is done."} />
            )}
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
            knowledge={attached}
            opening={profile?.opening ?? ""}
            onResult={(r, err) => {
              setSessionActive(false);
              if (r) setResult(r);
              if (err) setRunErr(err);
            }}
          />
        )}

        {/* Live log: a run on this machine only (a run In Agentry has none),
            in view while it runs and when it ended with no result to show,
            and under Run details once a result is there. */}
        {!inAgentry && (running || (log.length > 0 && !result)) && (
          <>
            <h3>Run log</h3>
            <pre ref={logRef} className="run-log selectable">{log.join("\n")}{running ? "\n…" : ""}</pre>
            {running && <Spinner label="Running — the container topology streams its progress above." />}
          </>
        )}

        {/* The finished hosted run: the result first, from the outputs
            unpacked into its folder; the card is its Run details. */}
        {inAgentry && hostedRun?.final && (
          <RunResultView
            runId={hostedRun.id}
            headline={hostedHeadline(hostedRun)}
            tone={hostedRun.outcomeTone === "ok" ? "ok" : hostedRun.outcomeTone === "bad" ? "bad" : "warn"}
            error={hostedRun.downloadError || undefined}
            resultJson={hostedRun.resultJson ?? ""}
            knowledge={hostedRun.knowledgeLine || undefined}
          >
            <HostedResult run={hostedRun} />
          </RunResultView>
        )}

        {/* A run or a session on this machine: the result first; the
            envelope's badges, usage, paths and the log are its Run details. */}
        {result && (
          <RunResultView
            runId={result.runId}
            headline={result.status === "ok" ? "Finished." : `It ended in an error (exit ${result.exitCode}).`}
            tone={result.status === "ok" ? "ok" : "bad"}
            error={result.error || undefined}
            resultJson={result.resultJson}
            knowledge={result.knowledge}
          >
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
              <div className="kv"><span className="k">output folder</span><span className="mono">{result.outDir}</span></div>
              <div className="kv"><span className="k">receipts</span><span className="mono">{result.receiptsDir}</span></div>
              <div className="kv"><span className="k">run plan sha256</span><span className="mono" title={result.runPlanSha}>{result.runPlanSha.slice(0, 12)}…</span></div>
              {/* The operator policy the run enforced (agentry-cli#152,
                  agentry-desktop#269): which file, and how its path was
                  chosen, since a relocated runner home is inherited
                  silently from the environment. */}
              {result.policy && (
                <>
                  <div className="kv"><span className="k">operator policy</span><span className="mono">{result.policy.path}</span></div>
                  <div className="kv"><span className="k">policy chosen by</span><span>{policySourceLabel(result.policy.source)}</span></div>
                  {result.policy.sha256 && (
                    <div className="kv"><span className="k">policy digest</span><span className="mono" title={result.policy.sha256}>{shortDigest(result.policy.sha256)}</span></div>
                  )}
                </>
              )}
            </Card>
            {log.length > 0 && (
              <>
                <h4>Run log</h4>
                <pre className="run-log selectable">{log.join("\n")}</pre>
              </>
            )}
          </RunResultView>
        )}

        {started && !busy && (
          <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
            <Button onClick={setUpAgain}>Back to the setup</Button>
            <Button onClick={onClose}>Close</Button>
          </div>
        )}
      </div>
    </div>
  );
}

// policySourceLabel says how the run's policy path was chosen, from the
// envelope's policy.source (agentry-cli#152). The desktop has no
// --policy, so "flag" is never its own; an unknown source is shown as is.
function policySourceLabel(source: string): string {
  switch (source) {
    case "default": return "the runner home's policy.yaml (the default)";
    case "env": return "AGENTRY_RUNNER_HOME, set in the environment this app started from";
    default: return source;
  }
}

// hostedHeadline is how a run In Agentry ended, in a sentence: a pass says
// so, and anything else says what the registry's verdict line says.
function hostedHeadline(run: main.HostedRunView): string {
  if (run.outcome === "succeeded") return "Finished.";
  if (run.status === "cancelled") return "Cancelled before it started.";
  const line = (run.outcomeLine || run.statusLine || "it did not finish").trim();
  return line.charAt(0).toUpperCase() + line.slice(1).replace(/\.?$/, ".");
}
