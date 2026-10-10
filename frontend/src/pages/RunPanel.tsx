// The run surface (RUNNER-PLAN-2 §P6): run a QuickAgent from the app with
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
// (runGates), and paths from native pickers only (D-030). Once a run or a
// chat starts, this setup hands it to its flow above the pages (RunFlow,
// HostedRunFlow, ChatFlow; agentry-desktop#465–#467) and closes: it shows
// only the setup, a refusal, and the missing-secret ask.
import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import * as KnowledgeAPI from "../../wailsjs/go/main/KnowledgeAPI";
import * as VoiceAPI from "../../wailsjs/go/main/VoiceAPI";
import * as ScheduleAPI from "../../wailsjs/go/main/ScheduleAPI";
import { ChooseDirectory, ChooseFiles } from "../../wailsjs/go/main/App";
import { knowledge, main, run, schedules, trust } from "../../wailsjs/go/models";
import { Button, Card, DeclaredServersTable, ErrorBox, Notice, SignedBadge, Textarea, fmtBytes, shortDigest } from "../ui";
import { STEP_LABEL, inAgentryByDefault, needList, setupSteps, vulnerableServerStep, whereFallback, type RunForm, type StepKey, requiredOf, goesWithoutSentence } from "../runSetup";
import { followHostedChat, followHostedRunFound, startHostedRun, startLocalChat, startLocalRun, useChat, useHostedRun } from "../runs";
import { HostedConnectAsk, HostedCredits, HostedSecretsAsk, HostedSecretsNote, HostedSignIns, HostedUnavailable, PublisherEarnsLine, SessionCredits, SessionUnavailable } from "./HostedRun";
import { AgentMarkdown } from "../AgentMarkdown";
import { WRITE_IN_MY_VOICE, WRITE_IN_MY_VOICE_HINT, voiceChoice } from "../voice";
import { GuideText, connectHelp } from "../GuideText";
import { AuthorHelp } from "../KeyHelp";
import { attachable, attachableInAgentry, inAgentryLine, pagesOf, scanTone, tooManyPacks } from "../knowledge";
import { ServerSignIns, asConns } from "./ConnectTools";
import { connectionBlockers, signInRecap } from "../connections";
import { WhenStep, type Availability, type CheckState } from "./WhenStep";
import { defaultWhen, hostedGapHeading, whenProblem, whenSpec, whenUnavailable, type WhenChoice, type WhenForm } from "../scheduled";

/** Announced when the setup made a schedule: App opens the Scheduled
 *  page on it (agentry-desktop#743). */
export const SCHEDULED_EVENT = "agentry:scheduled";

export default function RunPanel({
  registry, ns, name, version, summary, attestation, onClose, initialMode, initialStep, local = false, initialForm, initialWhen,
}: {
  registry: string;
  ns: string;
  name: string;
  version: string;
  /** The QuickAgent's summary, for Before you start. */
  summary?: string;
  /** The page's provenance verification (declared digest); the run
   * re-verifies the downloaded bytes either way. */
  attestation: trust.AttestationResult | null;
  onClose: () => void;
  /** Harness-only: open with this run mode selected, for screenshots. */
  initialMode?: string;
  /** Harness-only: open on this step, for screenshots. */
  initialStep?: StepKey;
  /** A QuickAgent installed from a folder on this computer
   * (agentry-desktop#425): no registry holds it, so it is not offered In
   * Agentry, and its run needs the unsigned consent. */
  local?: boolean;
  /** The answers as the person left them, when Back to the setup reopens
   *  the setup of a run that is over (agentry-desktop#465). */
  initialForm?: RunForm;
  /** Schedule a new run opens the setup with When on Repeat (D12). */
  initialWhen?: WhenChoice;
}) {
  const agentRef = `${ns}/${name}@${version}`;
  const f0 = initialForm;

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
  const [task, setTask] = useState(f0?.task ?? "");
  const [intakeValues, setIntakeValues] = useState<Record<string, string>>(f0?.intakeValues ?? {});
  const [mode, setMode] = useState(f0?.mode ?? "");
  const [inputs, setInputs] = useState<string[]>(f0?.inputs ?? []);
  // The files among the inputs, as the native file picker returned them: a
  // file intake question may pick one of these (plan §4, Details), and a
  // folder is never offered for it.
  const [pickedFiles, setPickedFiles] = useState<string[]>(f0?.pickedFiles ?? []);
  // The knowledge packs listed for the Knowledge step (plan knowledge.md
  // §4.5) — those kept here can be attached, those kept only in Agentry are
  // shown with where to download them — and the ones to attach, by name; Go
  // opens each from the store after the gates, and the receipt names what
  // was mounted.
  const [packs, setPacks] = useState<knowledge.Pack[] | null>(null);
  const [attach, setAttach] = useState<string[]>(f0?.attach ?? []);
  // A pack kept only here being uploaded from the Knowledge step, so a run
  // In Agentry can attach it once Agentry has scanned it; and what the
  // upload said.
  const [uploading, setUploading] = useState<string | null>(null);
  const [uploadNote, setUploadNote] = useState<string | null>(null);
  const [outDir, setOutDir] = useState(f0?.outDir ?? "");
  // Settings → Run defaults (agentry-desktop#347): the saved output folder
  // fills the field and where to run pre-selects the Where step, until the
  // person chooses otherwise for this run.
  const [defaults, setDefaults] = useState<main.RunDefaultsView | null>(null);
  // Unsafe Mode (agentry-desktop#418): while Run defaults' switch is on, the
  // line a run of this release on this computer carries, as Go composes it
  // for the native consent dialog too. Shown on Review and start; it gates
  // nothing here, and a run In Agentry never carries it.
  const [unsafeNotice, setUnsafeNotice] = useState<main.UnsafeNoticeView | null>(null);
  useEffect(() => {
    let live = true;
    setUnsafeNotice(null);
    RunAPI.UnsafeNotice(registry, agentRef)
      .then((v) => live && setUnsafeNotice(v))
      // Go words the line, and words one for a release it cannot read too;
      // a call that fails outright says only that the state is not known.
      .catch(() => live && setUnsafeNotice({ on: true, line: "Whether Unsafe Mode is on could not be read. Run defaults shows the switch.", unplaced: [] }));
    return () => { live = false; };
  }, [registry, agentRef]);
  const modeChosen = useRef(!!f0);
  // Whether In Agentry was chosen by the default rather than by the person:
  // if the offer or the release's verdict then rules it out, the step goes
  // back to this computer.
  const byDefault = useRef(false);
  const chooseMode = (m: string) => { modeChosen.current = true; setMode(m); };
  const [keep, setKeep] = useState(f0?.keep ?? true);
  // D-012: the keep toggle is sent only once the user touches it; an
  // untouched toggle sends null, so a no-secret release does not grow an
  // empty profile per run, while an explicit keep is recorded.
  const [keepTouched, setKeepTouched] = useState(f0?.keepTouched ?? false);
  const [timeoutSec, setTimeoutSec] = useState(f0?.timeoutSec ?? "");
  const [maxTurns, setMaxTurns] = useState(f0?.maxTurns ?? "");
  const [acceptUnsigned, setAcceptUnsigned] = useState(f0?.acceptUnsigned ?? false);
  // Its own consent (M23): an unscanned release is not an unsigned one.
  const [acceptUnscanned, setAcceptUnscanned] = useState(f0?.acceptUnscanned ?? false);
  // The registry's verdict on the declared MCP servers (agentry#177): an
  // advisory the gate rates high or critical refuses the run; on this
  // computer only Unsafe Mode, with this consent (recorded in the
  // receipt), lets it through (agentry-desktop#459, D-055).
  const [mcp, setMcp] = useState<main.MCPView | null>(null);
  const [allowVulnerableMCP, setAllowVulnerableMCP] = useState(f0?.allowVulnerableMCP ?? false);
  // The release's servers that sign in by OAuth and the connection this
  // version keeps for each (agentry-desktop#666): Connect tools makes them,
  // and Run waits until each is connected. Null until read; a read that
  // fails leaves the engine's own refusal, which names the server.
  const [conns, setConns] = useState<run.ReleaseConnections | null>(null);
  // The release the panel shows now: a late answer for another one is
  // dropped rather than drawn.
  const shown = useRef(agentRef);
  shown.current = agentRef;
  const readConns = () => {
    const asked = agentRef;
    // Through a promise, so a binding an older harness entry lacks is a
    // read that did not answer, not a throw.
    Promise.resolve()
      .then(() => RunAPI.ReleaseConnections(registry, asked))
      .then((c) => { if (shown.current === asked) setConns(c); })
      .catch(() => { if (shown.current === asked) setConns(null); });
  };
  useEffect(readConns, [registry, agentRef]);
  const signIns = asConns(conns?.servers);

  // Run state. running: a run on this computer is being handed to the
  // run's flow, from Run until Go reports that it has begun (the native
  // consent comes first), when this setup closes (agentry-desktop#465).
  const [running, setRunning] = useState(false);
  const [runErr, setRunErr] = useState<string | null>(null);

  // When (plan quickagent-scheduling.md §7, D12; agentry-desktop#743): Now,
  // or Later or Repeat, which make this run a schedule on this computer.
  // Whether this computer can schedule at all is read once; the zone it
  // reads is a new schedule's default.
  const [availability, setAvailability] = useState<Availability | null>(null);
  const [when, setWhen] = useState<WhenForm>(() => defaultWhen(new Date(), "", initialWhen ?? "now"));
  const [check, setCheck] = useState<CheckState>({ state: "idle" });
  // A schedule In Agentry the registry would not make yet (D8;
  // agentry-desktop#745): its create is the check, and what it names is
  // asked for here, then Schedule is made again.
  const [scheduleGap, setScheduleGap] = useState<main.HostedScheduleMadeView | null>(null);
  useEffect(() => {
    let live = true;
    Promise.resolve()
      .then(() => ScheduleAPI.Availability())
      .then((a) => {
        if (!live) return;
        setAvailability(a);
        setWhen((w) => (w.zone ? w : { ...w, zone: a.zone }));
      })
      .catch((e) => live && setAvailability({ available: false, reason: String(e), zone: "", signedOut: { available: false, reason: String(e) }, lingerOn: false }));
    return () => { live = false; };
  }, []);

  // In Agentry (hosted runs M6): whether the mode is offered here, the run
  // the registry is executing, and a refused create rendered as a sentence.
  const [hosted, setHosted] = useState<main.HostedAvailabilityView | null>(null);
  // Whether the offer has been read (or failed to be): until then Run
  // defaults' fallback sentence has nothing to say.
  const [hostedKnown, setHostedKnown] = useState(false);
  const [hostedRefusal, setHostedRefusal] = useState<main.HostedStartView | null>(null);
  const [starting, setStarting] = useState(false);
  // A start refused connection-missing is made again until Agentry holds
  // the connections (agentry-desktop#667): whether that wait is under way.
  const [awaiting, setAwaiting] = useState(false);

  // The guided setup: which step shows, a step's own sentence when Next is
  // refused, and the heading focus moves to when the step changes.
  const [stepKey, setStepKey] = useState<StepKey>(initialStep ?? (f0 ? "review" : "start"));
  const [stepProblem, setStepProblem] = useState<string | null>(null);
  const stepHeading = useRef<HTMLHeadingElement | null>(null);
  const moved = useRef(false);
  // The author's guide for this exact release (manifest v2, DEV-141), if any.
  const guide = profile?.guide ?? null;

  useEffect(() => {
    RunAPI.SetupStatus().then(setSetup).catch((e) => setSetupErr(String(e)));
    SettingsAPI.RunDefaults()
      .then((d) => {
        setDefaults(d);
        if (d.outputDir && !f0) setOutDir((cur) => cur || d.outputDir || "");
      })
      .catch(() => setDefaults(null));
    // A local install's tool surface is its own (agentry-desktop#425); a
    // registry release's is the one its page shows.
    (local ? RunAPI.RunRequirements(registry, agentRef) : RunAPI.GatewayRequirements(registry, ns, name)).then(setMcp).catch(() => setMcp(null));
    KnowledgeAPI.List(registry).then((l) => setPacks(l.packs)).catch(() => setPacks([]));
    RunAPI.ProfileStatus(registry, agentRef)
      .then((p) => {
        setProfile(p);
        setFormKeep(p.keep);
        // A setup reopened from a run keeps the answers it had.
        if (f0) return;
        setMode(initialMode ?? ((p.modes ?? []).includes("autonomous") || (p.modes ?? []).length === 0 ? "autonomous" : p.modes[0]));
        setKeep(p.keep);
        setKeepTouched(false);
        if (requiredOf(p).length > 0 && !p.exists) setFormOpen(true);
      })
      .catch((e) => setProfileErr(String(e)));
  }, [registry, agentRef]);

  // In Agentry: is the mode offered for this release, and does this
  // account already have a run the panel should reattach to (D11)?
  useEffect(() => {
    let live = true;
    // No registry holds a local install, so nothing In Agentry can run it.
    if (local) {
      setHosted(null);
      setHostedKnown(true);
      return;
    }
    RunAPI.HostedAvailability(registry, agentRef)
      .then((h) => {
        if (!live) return;
        setHosted(h);
        setHostedKnown(true);
        // Only when the mode can actually be used: listing this account's
        // runs needs run:read, and a token without it must not be sent to
        // a route that would refuse it.
        if (!h.available && !h.session?.available) return;
        // The folder a reattached run downloads into is the one the field
        // would hold: Run defaults' saved folder, which may not have been
        // read into it yet.
        return SettingsAPI.RunDefaults()
          .then((d) => d.outputDir ?? "", () => "")
          .then((saved) => RunAPI.AttachHostedRun(registry, outDir.trim() || saved))
          .then((a) => {
          if (!live || !a.run) return;
          // A run or a chat In Agentry is followed above the pages, in its
          // card (agentry-desktop#466, #467).
          void followHostedRunFound(registry, a);
        });
      })
      .catch(() => { setHosted(null); setHostedKnown(true); });
    return () => { live = false; };
    // The attach reads the folder as it stands when the panel opens; a
    // later change applies to the next run.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [registry, agentRef]);


  // Fail closed on provenance: until the page's verification resolves,
  // the Run button stays off. Unsigned needs the explicit checkbox (the
  // install wizard's gate); mismatch/unverifiable never runs — the Go
  // side re-verifies the downloaded bytes and refuses those regardless.
  const provPending = attestation == null;
  // A local install (agentry-desktop#425) is unsigned by definition: the
  // same consent, enforced in Go the same way, with its own words.
  const unsigned = attestation?.state === "unsigned" || local;
  // Unscanned: the registry admitted the version without a scan (plan
  // organization-subscriptions §4) — its own verdict, its own checkbox,
  // enforced in Go like the unsigned one. A run In Agentry is not
  // offered for one: the platform records no such consent yet
  // (agentry-enterprise#55).
  const unscanned = attestation?.state === "unscanned";
  const unverifiable = attestation != null && attestation.state !== "verified" && !unsigned && !unscanned;
  const refused = !!mcp?.refusal;
  // Only a key a run cannot start without holds the run until the keys are
  // saved: an optional one (agentry-desktop#655) can be left out.
  const requiredNames = profile ? requiredOf(profile) : [];
  const optionalNames = profile?.optionalSecrets ?? [];
  const needsProfile = !!profile && requiredNames.length > 0 && !profile.exists;
  const intakeMissing = (profile?.intake ?? []).filter((q) => q.required && !(intakeValues[q.id] ?? "").trim()).map((q) => q.id);
  // In Agentry needs no docker, no images and no operator policy on this
  // machine: the setup status gates the local modes only.
  // Talk to it In Agentry (agentry-desktop#403) is mode "hosted-session": a
  // hosted session of the release's interactive mode, and In Agentry too.
  const sessionInAgentry = mode === "hosted-session";
  const inAgentry = mode === "hosted" || sessionInAgentry;
  // A declared server the gate rates high: Unsafe Mode's consent here,
  // nothing In Agentry (D-055). Go reads the switch itself and asks again.
  const vuln = vulnerableServerStep({ refused, inAgentry, unsafeOn: !!unsafeNotice?.on, consented: allowVulnerableMCP });
  // The offer that governs the mode chosen: the run's, or the session's.
  const offer = sessionInAgentry ? hosted?.session : hosted;
  // Write in my voice (plan writing-voice.md §5, D11; agentry-desktop#510):
  // drawn on Review when the voice this run would use exists (this
  // computer's here, the account's live one In Agentry), ticked from that
  // voice's own switch until the person changes it; a change of where it
  // runs goes back to that voice's switch. A read that did not answer draws
  // no box, and the run goes without the voice.
  const [voices, setVoices] = useState<main.VoiceView | null>(null);
  useEffect(() => {
    // Through a promise, so a binding that is not there (an older build's
    // harness) is a read that did not answer, not a throw.
    Promise.resolve()
      .then(() => VoiceAPI.View(registry))
      .then(setVoices)
      .catch(() => setVoices(null));
  }, [registry]);
  const voice = voiceChoice(voices, inAgentry);
  const [voiceSet, setVoiceSet] = useState<boolean | null>(null);
  useEffect(() => setVoiceSet(null), [inAgentry]);
  const writeInVoice = voice.offered && (voiceSet ?? voice.ticked);
  // Where to run's default, applied once the offer is known and before the
  // person has chosen; a run already attached chose for itself.
  useEffect(() => {
    if (modeChosen.current || initialMode || !profile) return;
    const allowed = inAgentryByDefault(defaults?.where, hosted, unscanned);
    if (allowed && !inAgentry) {
      byDefault.current = true;
      setMode("hosted");
    } else if (!allowed && byDefault.current && inAgentry) {
      byDefault.current = false;
      setMode((profile.modes ?? []).includes("autonomous") || (profile.modes ?? []).length === 0 ? "autonomous" : profile.modes[0]);
    }
  }, [defaults, hosted, unscanned, profile, initialMode, mode]);
  const fallback = modeChosen.current || initialMode || !defaults || !hostedKnown ? null : whereFallback(defaults.where, hosted, unscanned);
  // The run and the chat the app follows above the pages: the registry runs
  // one per account and this app follows one at a time, so while one In
  // Agentry moves, or a start waits for the registry's answer, no setup
  // starts another there; and the app keeps one chat (D-017;
  // agentry-desktop#466, #467).
  const followed = useHostedRun();
  const followedChat = useChat().chat;
  const chatLive = !!followedChat && !followedChat.over;
  const followedMoving = followed.starting || (!!followed.run && !followed.run.final) || (chatLive && followedChat.kind === "hosted");
  // What still stands between this form and a run, each in a sentence the
  // line above the button shows (agentry-desktop#167): the same conditions
  // the button reads, so the two cannot disagree. Busy is not a blocker.
  const busy = running || (inAgentry && starting);
  // A schedule (Later or Repeat): offered only where it can run (on this
  // computer, or In Agentry where the registry and the plan allow), and
  // held until nothing a fire needs is missing (D8).
  const whenBlocked = whenUnavailable({
    modes: profile?.modes ?? [], mode, inAgentry, local, availability, session: sessionInAgentry, hostedOffer: hosted?.schedule ?? null,
  });
  const scheduling = when.choice !== "now" && !whenBlocked;
  const schedulingHere = scheduling && !inAgentry;
  const schedulingThere = scheduling && inAgentry;
  const whenSays = scheduling ? whenProblem(when) : null;
  const blockers: string[] = [];
  if ((mode === "interactive" || sessionInAgentry) && chatLive) {
    blockers.push("A chat is open, in its card in the corner. This app keeps one chat at a time: end it there, then start another.");
  } else if (inAgentry && followedMoving && !schedulingThere) {
    // Making a schedule starts no run, so a run under way does not hold it.
    blockers.push("A run or a chat In Agentry is under way, in its card in the corner. Agentry runs one at a time for an account: wait for it to end, or end it there.");
  } else if (inAgentry) {
    if (!hosted) blockers.push("Checking whether this release can run In Agentry.");
    else if (!offer?.available) blockers.push(offer?.reason || "A run In Agentry is not available for this release.");
  } else if (!setup) {
    blockers.push("Checking this machine's setup.");
  } else if (!setup.dockerOk) {
    blockers.push("Docker is not ready — set it up in Settings → Run defaults" + (hosted?.offered ? ", or run this release In Agentry." : "."));
  }
  // The release's own profile is this machine's: a run In Agentry takes its
  // keys from the account's secrets there instead (agentry-desktop#300).
  if (!inAgentry && needsProfile && profile) {
    const servers = mcp?.servers ?? [];
    const server = servers.length === 1 ? servers[0] : "this release";
    blockers.push(`Save the keys for ${server}: ${requiredNames.join(", ")} (or leave them to the environment).`);
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
  if (vuln.blocker) blockers.push(vuln.blocker);
  blockers.push(...connectionBlockers(signIns, inAgentry, hosted?.vault ?? true));

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
  // In Agentry without the Knowledge step (the registry attaches no pack to a
  // hosted run here), a required ask cannot be met there at all.
  if (inAgentry && accepts?.required && !steps.includes("knowledge")) {
    blockers.push("This release needs a knowledge pack of your own, which a run In Agentry cannot attach here. Run it on this computer.");
  }
  // The ask for the declared secrets the account does not keep has its own
  // Save and start; Run waits while it is open.
  const asking = inAgentry && hostedRefusal?.refusal === "secret-missing" && (hostedRefusal.missing?.length ?? 0) > 0;
  if (asking) blockers.push("Enter the secrets this run needs above, then Save and start — or Cancel.");
  // The servers Agentry keeps no connection to for this version: connected
  // in the browser, then the start is made again (agentry-desktop#667).
  const connecting = inAgentry && hostedRefusal?.refusal === "connection-missing";
  if (connecting) blockers.push("Connect the servers named below in your browser; the run starts once Agentry holds them — or Cancel.");
  // Later or Repeat that this run cannot be holds the button rather than
  // let it start the run now.
  if (when.choice !== "now" && whenBlocked) blockers.push(`When says ${when.choice === "later" ? "Later" : "Repeat"}, which this run cannot be: ${whenBlocked} Choose Now to run it now.`);
  if (scheduling && whenSays) blockers.push(whenSays);
  if (schedulingThere && scheduleGap?.refusal && gapAsksSecrets(scheduleGap)) {
    blockers.push("Enter the secrets above, then Keep and schedule — or Cancel.");
  }
  if (schedulingHere) {
    if (!outDir.trim()) blockers.push("Choose the output folder: each scheduled run gets its own folder in it.");
    if (check.state === "checking" || (check.state === "idle" && !whenSays && outDir.trim())) blockers.push("Checking what a scheduled run needs.");
    if (check.state === "error") blockers.push(`The check could not be made: ${check.error}`);
    if (check.state === "done" && !check.ready) blockers.push(...check.gaps.map((g) => `A scheduled run would not have it: ${g}`));
  }
  const canRun = !busy && blockers.length === 0;
  // A step that went away (the tools step, once the release's shape is
  // known) falls back to the first one rather than to nothing.
  const at = Math.max(0, steps.indexOf(stepKey));
  const step = steps[at];
  // Focus follows the step.
  useEffect(() => {
    if (moved.current) stepHeading.current?.focus();
  }, [step]);

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
    if (step === "tools" && !inAgentry) {
      const c = connectionBlockers(signIns, false);
      if (c.length > 0) return `${c[0]} The vendor's consent screen opens in your browser; this version's later runs reuse what you approve.`;
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

  // The answers as they stand, kept with a run so that Back to the setup
  // reopens this setup as it was (agentry-desktop#465).
  function currentForm(): RunForm {
    return {
      mode, task, inputs, pickedFiles, intakeValues, attach, outDir, timeoutSec, maxTurns, keep, keepTouched,
      acceptUnsigned, acceptUnscanned, allowVulnerableMCP,
    };
  }

  function intakeAnswers(): Record<string, string> {
    const answers: Record<string, string> = {};
    for (const [k, v] of Object.entries(intakeValues)) if (v.trim() !== "") answers[k] = v;
    return answers;
  }

  // The answers as the run's own calls take them.
  function limits(): { timeout: number; turns: number } {
    return {
      timeout: timeoutSec.trim() === "" ? 0 : Math.max(0, parseInt(timeoutSec, 10) || 0),
      turns: maxTurns.trim() === "" ? -1 : Math.max(0, parseInt(maxTurns, 10) || 0),
    };
  }

  // The schedule this setup describes, as Go takes it (schedules.Request):
  // the run's own answers, and When.
  function scheduleRequest(): schedules.Request | null {
    const spec = whenSpec(when);
    if (!spec) return null;
    const { timeout, turns } = limits();
    return schedules.Request.createFrom({
      registry, ref: agentRef, outDir: outDir.trim(), task: task.trim(), files: inputs, knowledge: attached,
      intake: intakeAnswers(), voice: writeInVoice, timeoutSeconds: timeout, maxTurns: turns,
      allowUnsigned: unsigned && acceptUnsigned, allowUnscanned: unscanned && acceptUnscanned, allowVulnerableMcp: vuln.send,
      signedOut: when.signedOut && !!availability?.signedOut.available, when: spec,
    });
  }
  // The check runs again, after a pause in typing, whenever the schedule
  // the setup describes changes; a late answer for an older one is dropped.
  const scheduleKey = schedulingHere && !whenSays && outDir.trim() ? JSON.stringify(scheduleRequest()) : "";
  const checkedKey = useRef("");
  useEffect(() => {
    if (!scheduleKey) {
      checkedKey.current = "";
      setCheck({ state: "idle" });
      return;
    }
    setCheck({ state: "checking" });
    const req = scheduleRequest();
    // Set now, not when the timer fires: an older check's late answer is
    // then dropped as soon as the schedule described changes.
    checkedKey.current = scheduleKey;
    const asked = scheduleKey;
    const timer = setTimeout(() => {
      Promise.resolve()
        .then(() => ScheduleAPI.Check(req!))
        .then((c) => {
          if (checkedKey.current !== asked) return;
          setCheck({ state: "done", ready: c.ready, gaps: c.gaps ?? [], installed: c.installed, describe: c.describe, next: c.next, needsConsent: c.needsConsent });
        })
        .catch((e) => checkedKey.current === asked && setCheck({ state: "error", error: String(e) }));
    }, 600);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scheduleKey]);

  // Schedule: Go checks again, asks the consents in a native dialog, and the
  // agentry command makes the schedule; the setup closes onto the Scheduled
  // page. A refusal stays here in the command's words.
  async function scheduleRun() {
    if (inAgentry) {
      await scheduleInAgentry();
      return;
    }
    const req = scheduleRequest();
    if (!req) return;
    setRunning(true);
    setRunErr(null);
    try {
      const made = await ScheduleAPI.Add(req);
      window.dispatchEvent(new CustomEvent(SCHEDULED_EVENT, { detail: made.id }));
      onClose();
    } catch (e) {
      // A No in the consent dialog is no error.
      const said = String(e);
      if (!/not confirmed/i.test(said)) setRunErr(said);
    } finally {
      setRunning(false);
    }
  }

  // Schedule In Agentry (agentry-desktop#745): the registry's create checks
  // what every fire will need and keeps nothing until it all holds (D8). A
  // gap comes back for this setup to ask for in place (the secrets to keep,
  // the connections to make), then Schedule is made again; the schedule
  // made, the setup closes onto the Scheduled page.
  async function scheduleInAgentry() {
    const spec = whenSpec(when);
    if (!spec) return;
    setRunning(true);
    setRunErr(null);
    setScheduleGap(null);
    try {
      const made = await ScheduleAPI.HostedAdd(main.HostedScheduleRequest.createFrom({
        registry, ref: agentRef, task: task.trim(), files: inputs, knowledge: attached, intake: intakeAnswers(), voice: writeInVoice, when: spec,
      }));
      if (made.refusal) {
        setScheduleGap(made);
        // A server Agentry keeps no connection to: Connect tools opens in
        // the browser now, as a run's start does (agentry-desktop#667).
        if (made.refusal === "connection-missing") RunAPI.OpenHostedConnect(registry, agentRef).catch((e) => setRunErr(String(e)));
        return;
      }
      window.dispatchEvent(new CustomEvent(SCHEDULED_EVENT, { detail: made.id }));
      onClose();
    } catch (e) {
      setRunErr(String(e));
    } finally {
      setRunning(false);
    }
  }
  // A change of where or how it runs, or of When, leaves a gap that was
  // the old schedule's.
  useEffect(() => { setScheduleGap(null); }, [mode, when.choice]);

  // A chat on this computer: the chat's store starts it and its dialog opens
  // at once, with the start (the native consent, the sandbox) under way
  // inside it, and this setup closes (agentry-desktop#467). Go's refusal
  // shows in that dialog, with Back to the setup.
  function startLocalSession() {
    const { turns } = limits();
    try {
      startLocalChat(
        { panel: { registry, ns, name, version, summary, attestation, local }, form: currentForm() },
        agentRef, task, profile?.opening ?? "",
        () => RunAPI.StartSession(registry, agentRef, task, inputs, outDir.trim(), keepTouched ? keep : null, turns, unsigned && acceptUnsigned, intakeAnswers(), vuln.send, unscanned && acceptUnscanned, attached, writeInVoice),
      );
      onClose();
    } catch (e) {
      setRunErr(String(e));
    }
  }

  // In Agentry: the inputs go up, the run is created, and the panel follows
  // where it stands until the outputs come back. A refused create is a
  // sentence, not an error.
  async function runInAgentry() {
    setStarting(true);
    setRunErr(null);
    setHostedRefusal(null);
    try {
      const call = sessionInAgentry
        ? () => RunAPI.StartHostedSession(registry, agentRef, task, inputs, outDir.trim(), intakeAnswers(), attached, writeInVoice)
        : () => RunAPI.StartHostedRun(registry, agentRef, task, inputs, outDir.trim(), intakeAnswers(), attached, writeInVoice);
      const started = await startedInAgentry(call);
      // A server Agentry keeps no connection to for this version: Connect
      // tools opens in the browser now, at Start (plan D5), and the start
      // is made again until Agentry holds it (agentry-desktop#667).
      if (started?.refusal === "connection-missing") {
        RunAPI.OpenHostedConnect(registry, agentRef).catch((e) => setRunErr(String(e)));
        void awaitConnections();
      }
    } catch (e) {
      setRunErr(String(e));
    } finally {
      setStarting(false);
    }
  }

  // A start's answer In Agentry, followed: a chat above the pages once the
  // registry has created it (agentry-desktop#467), a run likewise
  // (agentry-desktop#466), and this setup closes; a refusal stays here, with
  // its ask before any run exists.
  async function startedInAgentry(call: () => Promise<main.HostedStartView>): Promise<main.HostedStartView> {
    const setup = { panel: { registry, ns, name, version, summary, attestation, local }, form: currentForm() };
    if (sessionInAgentry) {
      const started = await call();
      if (started.refusal) setHostedRefusal(started);
      else if (started.run) {
        followHostedChat(setup, started.run, hosted?.session ?? null, profile?.opening ?? "", task);
        onClose();
      }
      return started;
    }
    const started = await startHostedRun(setup, call);
    if (started.refusal) setHostedRefusal(started);
    else if (started.run) onClose();
    return started;
  }

  // The wait for Agentry's connections: the refused start is made again
  // every few seconds until the run is created, refused for another reason
  // (a secret to give, the credits), stopped, or out of time. It runs
  // outside the run store, whose start is a moment, not a quarter of an
  // hour; the run it makes is followed as any start's is.
  async function awaitConnections() {
    setAwaiting(true);
    setRunErr(null);
    try {
      const started = await RunAPI.AwaitHostedConnections(registry, agentRef);
      if (started.refusal) setHostedRefusal(started);
      else if (started.run) {
        setHostedRefusal(null);
        const setup = { panel: { registry, ns, name, version, summary, attestation, local }, form: currentForm() };
        if (started.run.mode === "interactive") {
          followHostedChat(setup, started.run, hosted?.session ?? null, profile?.opening ?? "", task);
        } else {
          await startHostedRun(setup, async () => started);
        }
        onClose();
      }
    } catch (e) {
      setHostedRefusal(null);
      setRunErr(String(e));
    } finally {
      setAwaiting(false);
    }
  }

  // A start held for its connections goes with the panel, and with a change
  // of where or how it runs: the held create is the first start's.
  // Through a promise, so a binding an older harness entry lacks is a call
  // that did not answer, not a throw.
  const quietly = (call: () => Promise<void>) => void Promise.resolve().then(call).catch(() => undefined);
  useEffect(() => () => quietly(RunAPI.DiscardHostedConnect), []);
  const heldMode = useRef(mode);
  useEffect(() => {
    if (heldMode.current === mode) return;
    heldMode.current = mode;
    quietly(RunAPI.DiscardHostedConnect);
    setHostedRefusal((r) => (r?.refusal === "connection-missing" ? null : r));
  }, [mode]);

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

  async function runAgent() {
    if (scheduling) {
      await scheduleRun();
      return;
    }
    if (inAgentry) {
      await runInAgentry();
      return;
    }
    if (mode === "interactive") {
      startLocalSession();
      return;
    }
    setRunning(true);
    setRunErr(null);
    const { timeout: t, turns: mt } = limits();
    const form = currentForm();
    try {
      // The run's flow follows it from here: this setup closes once Go
      // reports the run has begun, and stays, with Go's words, if it never
      // does (the consent declined, another job running, a gate refused).
      await startLocalRun(
        { panel: { registry, ns, name, version, summary, attestation, local }, form },
        agentRef,
        () => RunAPI.RunAgent(registry, agentRef, task, inputs, outDir.trim(), keepTouched ? keep : null, t, mt, unsigned && acceptUnsigned, intakeAnswers(), vuln.send, unscanned && acceptUnscanned, attached, writeInVoice),
      );
      onClose();
    } catch (e) {
      setRunErr(String(e));
    } finally {
      setRunning(false);
    }
  }

  const runLabel = scheduling
    ? running ? "Scheduling…" : "Schedule"
    : sessionInAgentry
    ? starting ? "Starting…" : "Start session In Agentry"
    : inAgentry
    ? starting ? "Starting…" : "Run In Agentry"
    : running ? "Starting…" : mode === "interactive" ? "Start session" : "Run";
  const modeWord = sessionInAgentry ? "an interactive session In Agentry" : inAgentry ? "In Agentry" : mode === "interactive" ? "an interactive session on this machine" : "on this machine, autonomously";
  const answered = (profile?.intake ?? []).filter((q) => q.type !== "secret");

  return (
    <div className="modal-backdrop" onClick={() => !running && onClose()}>
      <div className="modal modal-wide run-panel" role="dialog" aria-modal="true" aria-label={`Run ${agentRef}`} onClick={(e) => e.stopPropagation()}>
        <div className="spread">
          <h2 style={{ margin: 0 }}>Run {agentRef}</h2>
          <Button onClick={onClose} disabled={running} title={running ? "A run is starting" : ""} aria-label="Close">✕</Button>
        </div>

        <ol className="run-steps" aria-label="Steps">
          {steps.map((k, i) => (
            <li key={k} aria-current={i === at ? "step" : undefined} className={i === at ? "active" : i < at ? "done" : ""}>
              <span className={`wizard-dot${i === at ? " active" : ""}${i < at ? " done" : ""}`} aria-hidden />
              <span>{STEP_LABEL[k]}</span>
            </li>
          ))}
        </ol>

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
                <p className="selectable">{summary || (local
                  ? "It was installed from a folder on this computer: its summary and README are in that folder."
                  : "Its author wrote no summary. Its README is on its page.")}</p>
              )}
              <h4>What to have ready</h4>
              <ul className="run-need">
                {(guide && guide.prepare.length > 0
                  ? guide.prepare
                  : needList({ intake: profile?.intake ?? [], secrets: profile?.declaredSecrets ?? [], optional: optionalNames, servers: mcp?.declaredServers?.length ?? 0, files: guide?.files, signIns: signIns.length })
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
                        <input type="radio" name="run-mode" checked={mode === m} onChange={() => chooseMode(m)} disabled={running} />
                        <span>
                          <strong>{m}</strong>{" "}
                          <span className="muted">{m === "autonomous" ? "— runs from its files and ends" : "— a chat in this window under exactly the same controls"}</span>
                        </span>
                      </label>
                    ))}
                    {hosted?.offered && (
                      <label className="check-row" style={{ margin: 0 }}>
                        <input
                          type="radio" name="run-mode" checked={mode === "hosted"} onChange={() => chooseMode("hosted")}
                          disabled={running || !hosted.available || unscanned}
                        />
                        <span>
                          <strong>In Agentry</strong>{" "}
                          <span className="muted">— runs on the platform, autonomously; nothing runs on this machine</span>
                        </span>
                      </label>
                    )}
                    {/* Talk to it In Agentry (agentry-desktop#403): offered only
                        where the registry runs sessions and says this account's
                        payer may start one; a Standard payer sees why. */}
                    {hosted?.session?.offered && (
                      <label className="check-row" style={{ margin: 0 }}>
                        <input
                          type="radio" name="run-mode" checked={sessionInAgentry} onChange={() => chooseMode("hosted-session")}
                          disabled={running || !hosted.session.available || unscanned}
                        />
                        <span>
                          <strong>Talk to it In Agentry</strong>{" "}
                          <span className="muted">— a chat with it running on the platform; nothing runs on this machine</span>
                        </span>
                      </label>
                    )}
                  </div>
                  {(mode === "interactive" || sessionInAgentry) && (
                    <p className="muted small">
                      Interactive: you can talk to this agent and give it files; you cannot give it more access than this release declares.
                    </p>
                  )}
                  {fallback && !inAgentry && <p className="muted small" style={{ margin: "4px 0" }}>{fallback}</p>}
                  {hosted && <HostedUnavailable hosted={hosted} />}
                  {hosted && <SessionUnavailable registry={registry} hosted={hosted} />}
                  {sessionInAgentry && hosted && (
                    <>
                      <p className="muted small" style={{ marginBottom: 2 }}>
                        The task, your answers, the input files and each message go to {registry}, which runs this release
                        under the same contract, relays the conversation to this window and sends back the outputs and the
                        receipt, with the transcript, when the session ends. This machine needs no docker engine and no
                        operator policy for it.
                      </p>
                      <SessionCredits hosted={hosted} />
                      <PublisherEarnsLine hosted={hosted} />
                      <HostedSecretsNote hosted={hosted} />
                    </>
                  )}
                  {mode === "hosted" && hosted && (
                    <>
                      <p className="muted small" style={{ marginBottom: 2 }}>
                        The task, your answers and the input files go to {registry}, which runs this release under the same
                        contract and sends back the outputs and the receipt; it keeps no log of the run. This machine needs no
                        docker engine and no operator policy for it.
                      </p>
                      <HostedCredits hosted={hosted} />
                      <PublisherEarnsLine hosted={hosted} />
                      <HostedSecretsNote hosted={hosted} />
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
                  Docker is not available — a QuickAgent run on this machine needs a docker engine.
                  {setup.dockerErr ? ` (${setup.dockerErr})` : ""} Configure it, then reopen this panel
                  {hosted?.offered ? ", or run this release In Agentry" : ""}.
                </div>
              )}
              {setup && setup.dockerOk && !setup.imagesReady && !inAgentry && (
                <Notice tone="warn">
                  The gateway/harness/runner images are not all present yet — this run will install
                  them first (it can take a few minutes), or prepare them once in Settings → Provider settings.
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
                <span className="mono">/inputs</span>, and the QuickAgent reads them with{" "}
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
                  files, held to the digest of the archive this computer verified, in every run; the packs below are yours to add
                  beside them.
                </p>
              )}
              {!inAgentry && (packs?.length ?? 0) === 0 && (
                <p className="muted small">No knowledge pack is kept on this computer. Make one on the Knowledge page.</p>
              )}
              {inAgentry ? (
                <p className="muted small">
                  Attach knowledge packs Agentry keeps for your account. Each is mounted read-only at{" "}
                  <span className="mono">/knowledge/&lt;name&gt;</span> on the run&apos;s tools container only, on
                  Agentry&apos;s sandbox host, from the copy Agentry keeps and has scanned, and the QuickAgent reads it
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
                  <span className="mono">/knowledge/&lt;name&gt;</span> on the run&apos;s tools container only, and the QuickAgent
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
                          ? "kept in Agentry only: Copy to Local on the Cloud Knowledge page to attach it to a run here"
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
                        {uploading === p.name ? "Copying…" : "Copy to Cloud"}
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
              {/* The servers that sign in by OAuth (agentry-desktop#666): on
                  this computer, Connect; In Agentry, Agentry's own
                  connections, made on the site (agentry-desktop#667). */}
              {conns && conns.servers.length > 0 && !inAgentry && (
                <ServerSignIns registry={registry} agentRef={agentRef} release={conns} onChanged={readConns} />
              )}
              {conns && conns.servers.length > 0 && inAgentry && (hosted?.vault ?? true) && (
                <HostedSignIns registry={registry} agentRef={agentRef} servers={conns.servers.filter((c) => !c.unsupported).map((c) => c.server)} />
              )}
              {/* In Agentry the keys are the account's secrets there, asked
                  for at the start (agentry-desktop#300); the profile below
                  is this machine's. */}
              {inAgentry && hosted && profile && profile.declaredSecrets.length > 0 && (
                <>
                  <h4>Keys</h4>
                  <HostedSecretsNote hosted={hosted} />
                </>
              )}
              {/* Guided MCP-profile setup (§P3). */}
              {!inAgentry && profile && profile.declaredSecrets.length > 0 && (
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
                          {/* The key's name and its Optional mark share the grid's
                              first column: the form is two columns, name and field. */}
                          <span>
                            <span className="mono">{s}</span>
                            {optionalNames.includes(s) && <span className="muted small"> optional</span>}
                          </span>
                          <input
                            type="password"
                            autoComplete="off"
                            aria-label={s}
                            placeholder={optionalNames.includes(s)
                              ? "empty = from the environment, or the run starts without it"
                              : "empty = resolve from the environment at run time"}
                            value={formValues[s] ?? ""}
                            onChange={(e) => setFormValues({ ...formValues, [s]: e.target.value })}
                          />
                          {/* The author's help for this key: where to create
                              it, which access it needs, as markdown under
                              the output reader's rules (DEV-167). */}
                          {connectHelp(guide?.connect, s) && (
                            <div className="run-key-help">
                              <AuthorHelp name={s} help={connectHelp(guide?.connect, s)} />
                            </div>
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
              {profile && profile.declaredSecrets.length === 0 && signIns.length === 0 && (
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
                {signIns.length > 0 && (
                  <>
                    <dt>Sign-ins</dt>
                    <dd>
                      {/* In Agentry, Agentry's own connections, which this
                          app cannot read: Run asks for any it lacks. */}
                      <span>{inAgentry ? "Agentry's connections for this version; Run asks for any it lacks" : signInRecap(signIns)}</span>
                      <Button variant="link" onClick={() => goTo(steps.indexOf("tools"))} aria-label="Edit sign-ins">Edit</Button>
                    </dd>
                  </>
                )}
                {profile && profile.declaredSecrets.length > 0 && (
                  <>
                    <dt>Keys</dt>
                    <dd>
                      <span>
                        {inAgentry
                          ? schedulingThere
                            ? "the secrets your account keeps in Agentry, the only ones a schedule's runs use; one it does not keep is asked for when you schedule, and kept"
                            : requiredNames.length > 0
                            ? "your account's secrets in Agentry; a required one it does not keep is asked for when you start"
                            : "your account's secrets in Agentry"
                          : profile.exists ? "saved for this release" : "not saved yet"}
                        {goesWithoutSentence(profile, inAgentry)}
                      </span>
                      {!inAgentry && (
                        <Button variant="link" onClick={() => goTo(steps.indexOf("tools"))} aria-label="Edit keys">Edit</Button>
                      )}
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
              {voice.offered && (
                <div className="run-voice">
                  <label className="check-row">
                    <input type="checkbox" checked={writeInVoice} onChange={(e) => setVoiceSet(e.target.checked)} />
                    <span>{WRITE_IN_MY_VOICE}</span>
                  </label>
                  <p className="muted small" style={{ margin: "2px 0 8px" }}>
                    {WRITE_IN_MY_VOICE_HINT} {inAgentry ? "In Agentry it is your account's live voice." : "Here it is this computer's voice."}
                  </p>
                </div>
              )}
              <WhenStep form={when} onChange={setWhen} unavailable={whenBlocked} availability={availability} check={check} disabled={running}
                inAgentry={inAgentry && hosted ? { holdCredits: hosted.holdCredits, balance: hosted.balance, balanceKnown: hosted.balanceKnown } : null}
                onUpgrade={inAgentry && !sessionInAgentry && hosted?.schedule?.upgrade ? () => SettingsAPI.OpenUpgrade(registry).catch((e) => setRunErr(String(e))) : undefined} />
              {schedulingThere && scheduleGap?.refusal && (
                <ScheduleGap
                  gap={scheduleGap} registry={registry} agentRef={agentRef} busy={running}
                  onAgain={() => void scheduleInAgentry()}
                  onCancel={() => setScheduleGap(null)}
                  onError={(e) => setRunErr(e)}
                />
              )}
              {!schedulingThere && mode === "hosted" && hosted && <HostedCredits hosted={hosted} />}
              {sessionInAgentry && hosted && <SessionCredits hosted={hosted} />}
              {(mode === "hosted" || sessionInAgentry) && hosted && <PublisherEarnsLine hosted={hosted} />}
              {!schedulingThere && (<>
              <h4>Output</h4>
              {mode === "hosted" && (
                <p className="muted small" style={{ margin: "2px 0 6px" }}>
                  The run&apos;s result, reports, receipts and outputs download into this folder when it finishes. The time and
                  turn ceilings are the tier&apos;s, so Advanced is not offered for a run In Agentry.
                </p>
              )}
              {sessionInAgentry && (
                <p className="muted small" style={{ margin: "2px 0 6px" }}>
                  The session&apos;s result, receipts (with transcript.md) and outputs download into this folder when it
                  ends. A session In Agentry has no ceilings but its idle close, so Advanced is not offered for it.
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
                {outDir !== (defaults?.outputDir ?? "") && (
                  <Button onClick={() => setOutDir(defaults?.outputDir ?? "")} disabled={running}>Default</Button>
                )}
              </div>
              </>)}
              {!inAgentry && (
            <>
              {/* The ceilings have the policy's defaults and the keep toggle its
                      recorded choice: none is needed for a run, so they sit under
                      Advanced (agentry-desktop#167). */}
                  <details className="advanced">
                    <summary className="muted small">Advanced</summary>
                  {mode === "interactive" && (
                    <p className="muted small" style={{ margin: "2px 0 6px" }} data-chat-limits>
                      A chat here has no time limit. It ends when you end it, or after the Chat idle timeout in
                      Settings → Run defaults, if you set one.
                    </p>
                  )}
                  <div className="row">
                    {/* A chat's only time limit is Run defaults' Chat idle timeout,
                        which Go reads itself (agentry-desktop#468). */}
                    {mode !== "interactive" && <label className="row" style={{ gap: 6 }}>
                      <span className="muted small">timeout (s)</span>
                      <input
                        type="number" min={1} style={{ width: 110 }}
                        placeholder={setup ? String(setup.timeoutSeconds) : ""}
                        value={timeoutSec} onChange={(e) => setTimeoutSec(e.target.value)} disabled={running}
                      />
                    </label>}
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
              {unsafeNotice?.on && !inAgentry && (
                <div className="notice notice-unsafe selectable" role="note" data-unsafe-notice>
                  <span className="badge bad">Unsafe Mode</span>{" "}
                  {unsafeNotice.line}
                </div>
              )}
              {unscanned && (
                <div className="notice notice-unscanned mark-unscanned">
                  <p style={{ margin: 0 }}>
                    <SignedBadge att={attestation} />{" "}
                    <strong>The registry admitted this version without a scan.</strong>{" "}
                    <span className="muted">
                      No findings, no report and no attestation stand behind it. Its organization&apos;s administrator took the
                      QuickAgent off the pipeline, and no scanner looked at its contents.
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
                      {local ? (
                        <>It was installed from a folder on this computer: no pipeline scanned it and no one signed it.
                          The run&apos;s receipts are marked local.</>
                      ) : (
                        <>This registry published no attestation for this version — nothing cryptographically
                          vouches for these bytes. The run&apos;s receipts are marked unsigned.</>
                      )}
                    </span>
                  </span>
                </label>
              )}
              {vuln.offerConsent && (
                <label className="check-row">
                  <input type="checkbox" checked={allowVulnerableMCP} onChange={(e) => setAllowVulnerableMCP(e.target.checked)} />
                  <span>
                    <strong>Allow vulnerable MCP servers for this run.</strong>{" "}
                    <span className="muted selectable">
                      {mcp?.refusing}. Unsafe Mode is on, so this run can go ahead: Run asks you again, and the
                      receipt names Unsafe Mode and each server it let through.
                    </span>
                  </span>
                </label>
              )}
              {vuln.showRefusal && (
                <div className="error-box selectable">{mcp?.refusal}.</div>
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
                  <strong>{scheduling ? "Before you can schedule:" : "Before you can run:"}</strong>
                  <ul style={{ margin: "2px 0 0", paddingLeft: 18 }}>
                    {blockers.map((b) => <li key={b}>{b}</li>)}
                  </ul>
                  {/* A gap is fixed in place where the setup has the place
                      (plan D8): the keys on Connect tools; the model key in
                      the keychain is Settings → Model provider. */}
                  {scheduling && check.state === "done" && !check.ready && (
                    <p className="small" style={{ margin: "4px 0 0" }} data-when-fixes>
                      {check.gaps.some((g) => /secret/i.test(g)) && steps.includes("tools") && (
                        <Button variant="link" onClick={() => goTo(steps.indexOf("tools"))}>Save the keys on Connect tools</Button>
                      )}
                      {check.gaps.some((g) => /model key/i.test(g)) && (
                        <span> Keep the model key in this computer&apos;s keychain: Settings → Model provider, with the key saved there (a scheduled run does not see a key in the environment).</span>
                      )}
                    </p>
                  )}
                </div>
              )}
              {/* A refused create, as a sentence — and, for credits, the way
                  onward the Build page already offers (desktop#74). */}
              {/* The declared secrets the account does not keep: asked for
                  here, each with Keep it for later runs unticked, then the
                  start is made again (plan D12, agentry-desktop#300). */}
              {hostedRefusal?.refusal === "secret-missing" && (hostedRefusal.missing?.length ?? 0) > 0 && (
                <HostedSecretsAsk
                  registry={registry}
                  names={hostedRefusal.missing ?? []}
                  onSaved={() => { setHostedRefusal(null); void runInAgentry(); }}
                  onCancel={() => setHostedRefusal(null)}
                />
              )}
              {/* The servers Agentry keeps no connection to for this
                  version: Connect tools is open in the browser, and the
                  start is made again until it holds them (agentry-desktop#667). */}
              {connecting && hostedRefusal && (
                <HostedConnectAsk
                  registry={registry}
                  agentRef={agentRef}
                  missing={hostedRefusal.missing ?? []}
                  detail={hostedRefusal.detail}
                  waiting={awaiting}
                  onWait={() => void awaitConnections()}
                  onStop={() => quietly(RunAPI.StopAwaitingConnections)}
                  onCancel={() => {
                    quietly(RunAPI.DiscardHostedConnect);
                    setHostedRefusal(null);
                  }}
                />
              )}
              {hostedRefusal?.refusal && !(hostedRefusal.refusal === "secret-missing" && (hostedRefusal.missing?.length ?? 0) > 0) && hostedRefusal.refusal !== "connection-missing" && (
                <Notice tone="warn">
                  <strong>
                    {hostedRefusal.refusal === "credits-exhausted"
                      ? `Not enough credits for this ${sessionInAgentry ? "session" : "run"}.`
                      : hostedRefusal.refusal === "run-active"
                        ? "This account already has a run In Agentry."
                        : hostedRefusal.refusal === "session-tier"
                          ? "Talking to an agent In Agentry is not part of this subscription."
                          : hostedRefusal.refusal === "sessions-unavailable"
                            ? "A session In Agentry cannot start right now."
                          : `The registry refused this ${sessionInAgentry ? "session" : "run"}.`}
                  </strong>{" "}
                  <span className="selectable">{hostedRefusal.detail}</span>
                  {(hostedRefusal.refusal === "credits-exhausted" || hostedRefusal.refusal === "session-tier") && (
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
        


      </div>
    </div>
  );
}


/** Whether a schedule's gap is its secrets: missing, or held for one run
 *  only, which the setup asks for, kept. */
function gapAsksSecrets(gap: main.HostedScheduleMadeView): boolean {
  return (gap.refusal === "secret-missing" || gap.refusal === "secret-not-kept") && (gap.missing?.length ?? 0) > 0;
}

/** What the registry would not schedule In Agentry yet, and its fix in
 *  place (plan quickagent-scheduling.md D8; agentry-desktop#745): the
 *  secrets to keep, asked for here; the connections, made in the browser
 *  on Connect tools; the plan, through the upgrade page; anything else in
 *  the registry's own words. */
function ScheduleGap({ gap, registry, agentRef, busy, onAgain, onCancel, onError }: {
  gap: main.HostedScheduleMadeView;
  registry: string;
  agentRef: string;
  busy: boolean;
  onAgain: () => void;
  onCancel: () => void;
  onError: (e: string) => void;
}) {
  if (gapAsksSecrets(gap)) {
    return <HostedSecretsAsk registry={registry} names={gap.missing ?? []} forSchedule onSaved={onAgain} onCancel={onCancel} />;
  }
  return (
    <Notice tone="warn">
      <div data-schedule-gap={gap.refusal}>
        <strong>{hostedGapHeading(gap.refusal ?? "")}</strong> <span className="selectable">{gap.detail}</span>
        {gap.refusal === "connection-missing" && (
          <p className="small">
            Connect tools is open in your browser, signed in as the account this app uses: connect{" "}
            {(gap.missing ?? []).length > 0 ? (gap.missing ?? []).join(", ") : "each server"} there, then Schedule again.
          </p>
        )}
        <p className="small row" style={{ gap: 8, marginBottom: 0 }}>
          {gap.refusal === "connection-missing" && (
            <>
              <Button onClick={() => RunAPI.OpenHostedConnect(registry, agentRef).catch((e) => onError(String(e)))} disabled={busy}>Open Connect tools</Button>
              <Button variant="primary" onClick={onAgain} disabled={busy}>{busy ? "Scheduling…" : "Schedule again"}</Button>
            </>
          )}
          {gap.refusal === "schedule-tier" && (
            <Button onClick={() => SettingsAPI.OpenUpgrade(registry).catch((e) => onError(String(e)))}>Credits and upgrade</Button>
          )}
          <Button onClick={onCancel} disabled={busy}>Dismiss</Button>
        </p>
      </div>
    </Notice>
  );
}
