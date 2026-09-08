// The first-run guided setup (agentry-desktop#41, plan §5). A linear,
// skippable, re-runnable flow that only ORCHESTRATES services the app
// already has — SettingsAPI sign-in and RunAPI runtime setup — and adds no
// state of its own beyond the persisted `onboarded` flag (set by the App
// through SettingsAPI.MarkOnboarded when the wizard closes). It opens over
// the app on first start (App gates it) and is re-runnable from Settings.
import { useEffect, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { main } from "../../wailsjs/go/models";
import { Button, ErrorBox, Input, Mark, Notice } from "../ui";
import { RuntimeSetup } from "./RuntimeSetup";

const STEPS = ["Welcome", "Sign in", "Runtime", "Done"];

const norm = (u: string) => u.trim().replace(/\/+$/, "");

export default function Wizard({
  settings, activeRegistry, onChanged, onSelectRegistry, onFinish, initialStep = 0,
}: {
  settings: main.SettingsView;
  activeRegistry: string;
  onChanged: () => Promise<main.SettingsView>;
  onSelectRegistry: (url: string) => void;
  // Dismiss the wizard; the App marks onboarding done so it does not reopen.
  onFinish: () => void;
  // Harness-only: render straight to a given step for screenshots.
  initialStep?: number;
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

        {step === 0 && <Welcome onSetUp={() => setStep(1)} onSkip={onFinish} />}
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
        {step === 3 && <DoneStep settings={settings} activeRegistry={activeRegistry} onBack={() => setStep(2)} onFinish={onFinish} />}
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
          Browse scanned, signed BriefAgents from a registry, install them with digest and
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
      setNote(`Signed in as ${res.user.handle}.`);
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
      setNote(`Signed in as ${res.user.handle}.`);
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
          Sign in to install and run agents; the credential lands in the OS keychain, shared with
          the <span className="mono">agentry</span> CLI. You can also browse anonymously and sign in
          later.
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
                    type="password" className="grow mono" placeholder="agentry_pat_…"
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
  return (
    <>
      <div className="wizard-body">
        <RuntimeSetup
          heading="Set up the runtime"
          intro={
            <>
              BriefAgents run in isolated containers with tools served by a per-run Docker MCP
              Gateway. Pick the LLM provider those runs drive — only the env-var name is stored,
              never the key. Preparing the images is optional now; your first run prepares them if
              you skip it.
            </>
          }
        />
      </div>
      <div className="wizard-foot">
        <Button variant="ghost" onClick={onBack}>Back</Button>
        <div className="grow" />
        <Button variant="primary" onClick={onNext}>Continue</Button>
      </div>
    </>
  );
}

function DoneStep({
  settings, activeRegistry, onBack, onFinish,
}: {
  settings: main.SettingsView;
  activeRegistry: string;
  onBack: () => void;
  onFinish: () => void;
}) {
  const [status, setStatus] = useState<main.SetupStatusView | null>(null);
  useEffect(() => {
    RunAPI.SetupStatus().then(setStatus).catch(() => setStatus(null));
  }, []);

  const signedReg = settings.registries.find((r) => r.hasToken);
  const rows: { ok: boolean; text: string }[] = [
    signedReg
      ? { ok: true, text: `Signed in to ${signedReg.url}` }
      : { ok: false, text: "Browsing anonymously — sign in any time from Settings" },
    status?.dockerOk
      ? { ok: true, text: "Docker is available" }
      : { ok: false, text: "Docker not detected — install it from Settings before a run" },
    status?.policyExists
      ? { ok: true, text: `Provider ${status.provider}${status.model ? ` (${status.model})` : ""} written to the runner policy` }
      : { ok: false, text: "Provider defaults will be written on your first run" },
    status?.imagesReady
      ? { ok: true, text: "Runtime images are prepared" }
      : { ok: false, text: "Runtime images will be prepared on your first run" },
  ];

  return (
    <>
      <div className="wizard-body">
        <h1>You're set up</h1>
        <p className="page-sub">Here's where things stand. Anything deferred happens on your first run, or from Settings.</p>
        <ul className="wizard-recap">
          {rows.map((r, i) => (
            <li key={i} className={r.ok ? "ok" : "pending"}>
              <span className="wizard-recap-mark">{r.ok ? "✓" : "•"}</span>
              {r.text}
            </li>
          ))}
        </ul>
        {!activeRegistry && <Notice tone="info">No active registry yet — pick one from Settings to browse.</Notice>}
      </div>
      <div className="wizard-foot">
        <Button variant="ghost" onClick={onBack}>Back</Button>
        <div className="grow" />
        <Button variant="primary" onClick={onFinish}>Finish</Button>
      </div>
    </>
  );
}
