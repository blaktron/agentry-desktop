// The first-run guided setup (agentry-desktop#41, plan §5). A linear,
// skippable, re-runnable flow that only ORCHESTRATES services the app
// already has — SettingsAPI sign-in and RunAPI runtime setup — and adds no
// state of its own beyond the persisted `onboarded` flag (set by the App
// through SettingsAPI.MarkOnboarded when the wizard closes). It opens over
// the app on first start (App gates it) and is re-runnable from Settings.
import { useEffect, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { ClipboardSetText } from "../../wailsjs/runtime/runtime";
import { main } from "../../wailsjs/go/models";
import { Button, ErrorBox, Input, Mark, Notice } from "../ui";
import { rewrapLine } from "../knowledge";
import { RuntimeSetup } from "./RuntimeSetup";
import { AIReviewKeyCard, type ReviewKeyState } from "./AIReviewKey";
import { HarnessChooser, harnessLabel, type HarnessState } from "./CodingAgents";
import { keyRow, missingForNotice, nothingSetUp, providerRow, type RecapRow } from "../wizardRecap";
import { keyStanding } from "../keyRecommendation";

const STEPS = ["Welcome", "Sign in", "Runtime", "AI Review key", "Coding agent", "Done"];

const norm = (u: string) => u.trim().replace(/\/+$/, "");

// How the wizard closed (plan desktop-installers.md D10). Either way the
// App marks onboarding done (D-019); "later" also raises the Browse banner
// naming what is missing, for this session only.
export type WizardOutcome = { kind: "done" } | { kind: "later"; missing: string[] };

export default function Wizard({
  settings, activeRegistry, onChanged, onSelectRegistry, onFinish, initialStep = 0, probeOnEntry = false,
}: {
  settings: main.SettingsView;
  activeRegistry: string;
  onChanged: () => Promise<main.SettingsView>;
  onSelectRegistry: (url: string) => void;
  // Dismiss the wizard; the App marks onboarding done so it does not reopen.
  onFinish: (outcome: WizardOutcome) => void;
  // Harness-only: render straight to a given step for screenshots, and
  // let the Coding agent step probe on entry so its results can be shown.
  initialStep?: number;
  probeOnEntry?: boolean;
}) {
  const [step, setStep] = useState(initialStep);

  return (
    <div className="wizard-backdrop">
      <div className="wizard" role="dialog" aria-modal="true" aria-label="Agentry setup">
        <div className="wizard-head">
          <div className="row">
            <Mark size={24} />
            <strong>Set up Agentry Desktop</strong>
          </div>
          <div className="wizard-steps" aria-hidden>
            {STEPS.map((label, i) => (
              <span key={label} className={`wizard-dot${i === step ? " active" : ""}${i < step ? " done" : ""}`} title={label} />
            ))}
          </div>
        </div>

        {step === 0 && <Welcome onSetUp={() => setStep(1)} onSkip={() => onFinish({ kind: "done" })} />}
        {step === 1 && (
          <SignInStep
            settings={settings}
            activeRegistry={activeRegistry}
            onChanged={onChanged}
            onSelectRegistry={onSelectRegistry}
            onBack={() => setStep(0)}
            onNext={() => setStep(2)}
          />
        )}
        {step === 2 && <RuntimeStep onBack={() => setStep(1)} onNext={() => setStep(3)} />}
        {step === 3 && <ReviewKeyStep onBack={() => setStep(2)} onNext={() => setStep(4)} />}
        {step === 4 && <CodingAgentStep onBack={() => setStep(3)} onNext={() => setStep(5)} probeOnEntry={probeOnEntry} />}
        {step === 5 && (
          <DoneStep
            settings={settings}
            activeRegistry={activeRegistry}
            onBack={() => setStep(4)}
            onStartOver={() => setStep(1)}
            onFinish={onFinish}
          />
        )}
      </div>
    </div>
  );
}

function Welcome({ onSetUp, onSkip }: { onSetUp: () => void; onSkip: () => void }) {
  return (
    <>
      <div className="wizard-body wizard-welcome">
        <Mark size={56} />
        <h1>Welcome to Agentry Desktop</h1>
        <p className="page-sub">
          Browse scanned, signed QuickAgents from a registry, install them with digest and
          signature verification on this machine, and run them in isolated containers. This quick
          setup signs you in and prepares the runtime — you can skip it and do either later.
        </p>
      </div>
      <div className="wizard-foot">
        <Button variant="ghost" onClick={onSkip}>Skip for now</Button>
        <Button variant="primary" onClick={onSetUp}>Set up</Button>
      </div>
    </>
  );
}

function SignInStep({
  settings, activeRegistry, onChanged, onSelectRegistry, onBack, onNext,
}: {
  settings: main.SettingsView;
  activeRegistry: string;
  onChanged: () => Promise<main.SettingsView>;
  onSelectRegistry: (url: string) => void;
  onBack: () => void;
  onNext: () => void;
}) {
  const target = activeRegistry || settings.defaultRegistry || settings.registries[0]?.url || "";
  const [regURL, setRegURL] = useState(target);
  const [token, setToken] = useState("");
  const [pasteMode, setPasteMode] = useState(false);
  const [busy, setBusy] = useState(false);
  const [waiting, setWaiting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);

  const current = settings.registries.find((r) => norm(r.url) === norm(regURL));
  const signedIn = !!current?.hasToken;

  async function ensureRegistry(): Promise<string> {
    const u = norm(regURL);
    if (!u) throw new Error("Enter a registry URL first.");
    if (!settings.registries.some((r) => norm(r.url) === u)) {
      await SettingsAPI.AddRegistry(u);
    }
    onSelectRegistry(u);
    return u;
  }

  async function signInBrowser() {
    setBusy(true); setWaiting(true); setError(null); setNote(null);
    try {
      const u = await ensureRegistry();
      const res = await SettingsAPI.SignInViaBrowser(u);
      await onChanged();
      setNote(`Signed in as ${res.user.handle}.${rewrapLine(res)}`);
    } catch (e) {
      // A user cancel is a notice, not a failure (mirrors Settings).
      if (String(e).includes("cancelled")) setNote("Sign-in cancelled.");
      else setError(String(e));
    } finally {
      setBusy(false); setWaiting(false);
    }
  }

  async function signInPaste() {
    setBusy(true); setError(null); setNote(null);
    try {
      const u = await ensureRegistry();
      const res = await SettingsAPI.SignIn(u, token.trim());
      await onChanged();
      setToken(""); setPasteMode(false);
      setNote(`Signed in as ${res.user.handle}.${rewrapLine(res)}`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="wizard-body">
        <h1>Sign in to a registry</h1>
        <p className="page-sub">
          Sign in to install and run QuickAgents. The credential is kept in the OS keychain and
          shared with the <span className="mono">agentry</span> CLI. You can also browse anonymously
          and sign in later.
        </p>

        <ErrorBox error={error} />
        {note && <Notice tone="ok">{note}</Notice>}

        {signedIn ? (
          <Notice tone="ok">
            Signed in to <span className="mono">{norm(regURL)}</span> ({current?.tokenSource}).
          </Notice>
        ) : (
          <>
            <label className="field">
              <span className="field-label">Registry</span>
              <Input
                type="text" className="mono" placeholder="https://secureagentry.net"
                value={regURL} onChange={(e) => setRegURL(e.target.value)} disabled={busy}
              />
            </label>

            {waiting ? (
              <Notice tone="info">
                Waiting for your browser to authorize this app. Sign in there and approve it.{" "}
                <Button onClick={() => void SettingsAPI.CancelBrowserSignIn()}>Cancel</Button>
              </Notice>
            ) : (
              <div className="row">
                <Button variant="primary" disabled={busy || !regURL.trim()} onClick={signInBrowser}>
                  Sign in via browser
                </Button>
                <Button variant="link" disabled={busy} onClick={() => setPasteMode((v) => !v)}>
                  {pasteMode ? "Hide token field" : "Paste a token instead"}
                </Button>
              </div>
            )}

            {pasteMode && !waiting && (
              <label className="field mt">
                <span className="field-label">Personal access token</span>
                <div className="row">
                  <Input
                    type="password" className="grow mono" placeholder="agy_…"
                    value={token} onChange={(e) => setToken(e.target.value)} disabled={busy}
                    onKeyDown={(e) => e.key === "Enter" && token.trim() && signInPaste()}
                  />
                  <Button variant="primary" disabled={busy || !token.trim()} onClick={signInPaste}>
                    Sign in
                  </Button>
                </div>
              </label>
            )}
          </>
        )}
      </div>
      <div className="wizard-foot">
        <Button variant="ghost" onClick={onBack}>Back</Button>
        <div className="grow" />
        {!signedIn && <Button onClick={onNext} disabled={busy}>Skip, browse anonymously</Button>}
        <Button variant="primary" onClick={onNext} disabled={busy}>Continue</Button>
      </div>
    </>
  );
}

function RuntimeStep({ onBack, onNext }: { onBack: () => void; onNext: () => void }) {
  // Held here while the images prepare (RuntimeSetup's onBusy, desktop#145).
  const [busy, setBusy] = useState(false);
  return (
    <>
      <div className="wizard-body">
        <RuntimeSetup
          onBusy={setBusy}
          heading="Set up the runtime"
          intro={
            <>
              QuickAgents run in isolated containers with tools served by a per-run Docker MCP
              Gateway. Choose the AI provider those runs use and how to pay for it: sign in with a
              plan you already have (ChatGPT, Claude, GitHub Copilot) or connect your account, or use
              an API key, kept on this machine. Preparing the images is optional now; your first run
              prepares them if you skip it.
            </>
          }
        />
      </div>
      <div className="wizard-foot">
        <Button variant="ghost" onClick={onBack} disabled={busy}>Back</Button>
        <div className="grow" />
        <Button variant="primary" onClick={onNext} disabled={busy}>Continue</Button>
      </div>
    </>
  );
}

// ReviewKeyStep (agentry-desktop#462; plan desktop-review-key-and-run-cards.md
// D3): the person's own AI Review key, asked for where setup is. It is the
// card Settings → Provider settings shows, recommendation and link included,
// so the two cannot drift. The step keeps nothing of the key: what the card
// reports is where the key stands and whether a save is in flight, and the
// key itself goes to the keychain through the card's one call. A step of
// its own, not a card under the provider form, because saving the key takes
// the lock an image pull holds, and the Runtime step is where images are
// pulled. The footer is held while a save is in flight, as the Runtime
// step's is while images pull (desktop#145): a refusal that arrived after
// the person moved on would never be shown.
function ReviewKeyStep({ onBack, onNext }: { onBack: () => void; onNext: () => void }) {
  const [state, setState] = useState<ReviewKeyState>({ status: null, busy: false });
  const { status, busy } = state;
  return (
    <>
      <div className="wizard-body">
        <h1>Your AI Review key</h1>
        <AIReviewKeyCard onState={setState} />
      </div>
      <div className="wizard-foot">
        <Button variant="ghost" onClick={onBack} disabled={busy}>Back</Button>
        <div className="grow" />
        {!status?.ready && <Button onClick={onNext} disabled={busy}>Skip</Button>}
        <Button variant="primary" onClick={onNext} disabled={busy}>Continue</Button>
      </div>
    </>
  );
}

// Recap is the wizard's list of where things stand: a check for what is
// true, a bullet for what is not.
function Recap({ rows }: { rows: RecapRow[] }) {
  return (
    <ul className="wizard-recap">
      {rows.map((r, i) => (
        <li key={i} className={r.ok ? "ok" : "pending"}>
          <span className="wizard-recap-mark">{r.ok ? "✓" : "•"}</span>
          {r.text}
        </li>
      ))}
    </ul>
  );
}

// CodingAgentStep (agentry-desktop#89, plan desktop-installers.md §9, D9):
// which of Claude Code and Codex are on this machine, per the same probe
// `agentry doctor --adapter` runs; choosing one records where it lives in
// the operator policy so `agentry run --adapter <name>` finds it. The
// agentry-mcp hook-up — the harness driving Agentry — is shown as the one
// documented command to copy, because it needs Node and the agentry CLI,
// which this app neither ships nor installs (D14).
//
// Looking is a click, not the step opening (review of #89): the probe runs
// each harness's version flag, and a binary named claude on the PATH of a
// person who never asked for coding agents should not run just because the
// wizard reached this page — the CLI's doctor probes only when asked, too.
// Until then the step shows what the policy already records, read without
// executing anything.
function CodingAgentStep({ onBack, onNext, probeOnEntry = false }: { onBack: () => void; onNext: () => void; probeOnEntry?: boolean }) {
  const [state, setState] = useState<HarnessState>({ harnesses: null, chosen: null, busy: false });
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const { harnesses, chosen, busy } = state;

  async function copy(name: string, text: string) {
    // The webview's clipboard through the Wails runtime; the command stays
    // selectable on screen whatever happens.
    const ok = await ClipboardSetText(text).catch(() => false);
    setCopied(ok ? name : null);
    if (ok) setTimeout(() => setCopied((c) => (c === name ? null : c)), 2000);
    else setError("Could not reach the clipboard — select the line and copy it.");
  }

  return (
    <>
      <div className="wizard-body">
        <h1>Your coding agent</h1>
        <p className="page-sub">
          Agentry can run QuickAgents through a coding agent you already have — Claude Code or Codex
          — with <span className="mono">agentry run --adapter claude</span> (or{" "}
          <span className="mono">codex</span>). Choosing one here records where it lives in the runner
          policy; this app's own runs use the container harness either way. Skip if you have neither,
          or want to decide later.
        </p>
        <ErrorBox error={error} />
        <HarnessChooser probeOnEntry={probeOnEntry} onState={setState} />

        {harnesses && harnesses.length > 0 && (
          <div className="mt">
            <h2>Let your coding agent drive Agentry</h2>
            <p className="muted">
              The other direction: Claude Code or Codex searching, verifying, installing and running
              QuickAgents through the <span className="mono">agentry-mcp</span> server. It needs Node
              20 or newer, the <span className="mono">agentry</span> CLI on your PATH and{" "}
              <span className="mono">python3</span>; this app installs none of them and runs nothing
              here — copy the line for your host and run it yourself, with the path to your build of
              the server.
            </p>
            {harnesses.filter((h) => h.mcpCommand).map((h) => (
              <div key={h.name} className="row mt">
                <pre className="mono grow wizard-command" tabIndex={0}>{h.mcpCommand}</pre>
                <Button onClick={() => void copy(h.name, h.mcpCommand)}>{copied === h.name ? "Copied" : "Copy"}</Button>
              </div>
            ))}
          </div>
        )}
      </div>
      <div className="wizard-foot">
        <Button variant="ghost" onClick={onBack}>Back</Button>
        <div className="grow" />
        {!harnesses?.some((h) => h.chosen) && !chosen?.length && <Button onClick={onNext} disabled={busy}>Skip</Button>}
        <Button variant="primary" onClick={onNext} disabled={busy}>Continue</Button>
      </div>
    </>
  );
}

function DoneStep({
  settings, activeRegistry, onBack, onStartOver, onFinish,
}: {
  settings: main.SettingsView;
  activeRegistry: string;
  onBack: () => void;
  onStartOver: () => void;
  onFinish: (outcome: WizardOutcome) => void;
}) {
  const [status, setStatus] = useState<main.SetupStatusView | null>(null);
  const [prereq, setPrereq] = useState<main.PrereqView | null>(null);
  // Read from the policy, never probed: the recap runs no harness.
  const [chosenNames, setChosenNames] = useState<main.HarnessView[]>([]);
  const [keyStatus, setKeyStatus] = useState<main.JevKeyView | null>(null);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    Promise.allSettled([
      RunAPI.JevKeyStatus().then(setKeyStatus).catch(() => setKeyStatus(null)),
      RunAPI.SetupStatus().then(setStatus).catch(() => setStatus(null)),
      RunAPI.PrereqStatus().then(setPrereq).catch(() => setPrereq(null)),
      RunAPI.ChosenHarnesses().then(setChosenNames).catch(() => setChosenNames([])),
    ]).then(() => setLoaded(true));
  }, []);

  const signedReg = settings.registries.find((r) => r.hasToken);
  const chosen = chosenNames.length > 0 ? chosenNames.map(harnessLabel).join(" and ") : null;
  const rows: RecapRow[] = [
    signedReg
      ? { ok: true, text: `Signed in to ${signedReg.url}` }
      : { ok: false, text: "Browsing anonymously — sign in any time from Settings" },
    // The docker row names the state (agentry-desktop#64): a pending restart
    // is a step done, waiting on the session, not a missing engine.
    status?.dockerOk
      ? { ok: true, text: "Docker is available" }
      : prereq?.state === "pendingRestart"
        ? { ok: false, text: "Docker is installed — restart your session before the first run" }
        : prereq?.state === "groupMissing"
          ? { ok: false, text: "Docker is installed — add yourself to the docker group from Settings, then restart your session" }
          : { ok: false, text: "Docker not detected — install it from Settings before a run" },
    // By whether a provider was set up, not by whether the policy exists:
    // the AI Review key's save writes the policy from defaults (#462).
    providerRow(status),
    status?.imagesReady
      ? { ok: true, text: "Runtime images are prepared" }
      : { ok: false, text: "Runtime images will be prepared on your first run" },
    chosen
      ? { ok: true, text: `Coding agent: ${chosen} recorded in the runner policy` }
      : { ok: false, text: "No coding agent chosen — runs use the container harness; choose one any time by running setup again" },
  ];
  // The key's row comes last and is an aside (D13): a recommendation, so
  // Browse's notice never lists it.
  const keyLine = keyRow(keyStatus && { standing: keyStanding(keyStatus), envName: keyStatus.envName });
  if (keyLine) rows.push(keyLine);

  // Nothing set up (D10): the person did none of the three things this
  // wizard is for — a sign-in, a provider set up, a coding agent chosen
  // (wizardRecap.ts has the rule, and what does not count). Until the
  // reads settle the recap is the ordinary one.
  const nothing = loaded && nothingSetUp({ signedIn: !!signedReg, setup: status, codingAgentChosen: !!chosen });
  const missing = missingForNotice(rows);

  if (nothing) {
    return (
      <>
        <div className="wizard-body">
          <h1>Nothing is set up yet</h1>
          <p className="page-sub">
            Agentry Desktop can browse a public registry as it is, but it cannot sign in, run or build
            anything until setup is done. Every step is still here, and Settings has them all too.
          </p>
          <Recap rows={rows} />
        </div>
        <div className="wizard-foot">
          <Button variant="ghost" onClick={onBack}>Back</Button>
          <div className="grow" />
          <Button onClick={() => onFinish({ kind: "later", missing })}>I'll do it later</Button>
          <Button variant="primary" onClick={onStartOver}>Set up now</Button>
        </div>
      </>
    );
  }

  return (
    <>
      <div className="wizard-body">
        <h1>You're set up</h1>
        <p className="page-sub">Here's where things stand. Anything deferred happens on your first run, or from Settings.</p>
        <Recap rows={rows} />
        {!activeRegistry && <Notice tone="info">No active registry yet — pick one from Settings to browse.</Notice>}
      </div>
      <div className="wizard-foot">
        <Button variant="ghost" onClick={onBack}>Back</Button>
        <div className="grow" />
        <Button variant="primary" onClick={() => onFinish({ kind: "done" })}>Finish</Button>
      </div>
    </>
  );
}
