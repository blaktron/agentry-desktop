import React, { useEffect, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import * as SelfUpdateAPI from "../../wailsjs/go/main/SelfUpdateAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { main } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { Button, Card, ErrorBox, Input, Modal, Notice, PageHeader, Spinner, Toolbar, fmtDuration } from "../ui";
import { rewrapLine } from "../knowledge";
import { RuntimeSetup, PrereqCard, providerLabel } from "./RuntimeSetup";
import { CodingAgentsCard, codingAgentName, exportLabel } from "./CodingAgents";
import { SecretsCard } from "./HostedRun";
import { AIReviewKeyCard } from "./AIReviewKey";
import { keyStanding } from "../keyRecommendation";
import type { RunWhere } from "../runSetup";
import { ProfileSettings } from "./SettingsProfile";
import { DockerImagesCard } from "./DockerImages";

// Settings as five cards (plan settings-cards.md M3, D1, D3, D12;
// agentry-desktop#346): the landing is a card per section, each card the
// whole of a link to its section page, with one sentence, a status line
// and its parts as links; each section page holds the cards Settings held
// before, moved unchanged, under a "← Settings" link back.
// Profile (agentry-desktop#423) is a sixth, offered only while the active
// registry holds a token.
export type SettingsSection = "profile" | "registry" | "updates" | "coding-agents" | "run-defaults" | "provider";

// The parts a landing link or a deep link can name; each section page
// renders its part under the anchor settings-part-<id>.
export type SettingsPart =
  | "profile" | "subscription"
  | "registries" | "add-registry" | "desktop" | "quickagents"
  | "budgets" | "where" | "output" | "unsafe" | "docker" | "images" | "provider" | "ai-review-key";
type Part = { id: SettingsPart; label: string };

// SECTIONS is the landing's order and each section's parts. A part's id is
// the anchor its section page scrolls to when the landing's link names it.
export const SECTIONS: { id: SettingsSection; title: string; sentence: string; parts: Part[] }[] = [
  {
    id: "profile", title: "Profile",
    sentence: "Who you are signed in as, your display name, and your subscription, as your account page on the site shows them.",
    parts: [{ id: "profile", label: "Profile" }, { id: "subscription", label: "Subscription" }],
  },
  {
    id: "registry", title: "Registry settings",
    sentence: "The registries this app talks to, your sign-in to each, and the secrets runs In Agentry use where a registry keeps them.",
    parts: [{ id: "registries", label: "Registries" }, { id: "add-registry", label: "Add a registry" }],
  },
  {
    id: "updates", title: "Update settings",
    sentence: "Whether this app looks for a newer Agentry Desktop, and how QuickAgents installed here are updated.",
    parts: [{ id: "desktop", label: "Agentry Desktop" }, { id: "quickagents", label: "QuickAgents" }],
  },
  {
    id: "coding-agents", title: "Coding agents",
    sentence: "The coding agent on this computer, and whether Install also puts a QuickAgent where it reads.",
    parts: [],
  },
  {
    id: "run-defaults", title: "Run defaults",
    sentence: "How long a run may take, where runs go and where their files land, and the Docker engine runs on this computer use, with Agentry's images on it.",
    parts: [
      { id: "budgets", label: "Timeout and turns" }, { id: "where", label: "Where to run" },
      { id: "output", label: "Output folder" }, { id: "unsafe", label: "Unsafe Mode" },
      { id: "docker", label: "Docker engine" }, { id: "images", label: "Docker images" },
    ],
  },
  {
    id: "provider", title: "Provider settings",
    sentence: "The AI provider and model runs use, and your own AI Review key.",
    parts: [{ id: "provider", label: "Model provider" }, { id: "ai-review-key", label: "AI Review key" }],
  },
];

// Where each place that opens Settings goes (agentry-desktop#346): straight
// to the section it means rather than to the landing.
export const SETTINGS_FOR = {
  // the sidebar's line for a newer Agentry Desktop
  selfUpdate: { section: "updates", part: "desktop" },
  // the "Setup isn't finished" notice on Browse
  setupNotice: { section: "provider" },
  // Knowledge's "Not signed in" notice
  knowledgeSignIn: { section: "registry", part: "registries" },
  // Build's Describe it, whose drafting needs the AI Review key
  describeKey: { section: "provider", part: "ai-review-key" },
  // the sidebar's Unsafe Mode badge
  unsafeMode: { section: "run-defaults", part: "unsafe" },
} satisfies Record<string, { section: SettingsSection; part?: SettingsPart }>;

export const sectionTitle = (s: SettingsSection) => SECTIONS.find((x) => x.id === s)?.title ?? "Settings";

export default function Settings({
  settings, activeRegistry, onChanged, onSelectRegistry, onRunSetup, section, part, onOpen,
}: {
  settings: main.SettingsView | null;
  activeRegistry: string;
  onChanged: () => Promise<main.SettingsView>;
  onSelectRegistry: (url: string) => void;
  // Re-open the first-run guided setup. Optional so the page renders in the
  // harness without the App shell (agentry-desktop#41).
  onRunSetup?: () => void;
  // The section page shown, or the landing when absent; part is the anchor
  // within it the landing's link named.
  section?: SettingsSection;
  part?: SettingsPart;
  // Opens a section (and a part of it), or the landing with none.
  onOpen: (section?: SettingsSection, part?: SettingsPart) => void;
}) {
  // A part named by the landing's link is scrolled to once its section has
  // rendered; the page's own scroll starts at the top otherwise.
  useEffect(() => {
    if (!section || !settings) return;
    const el = part ? document.getElementById(`settings-part-${part}`) : null;
    if (el) el.scrollIntoView({ block: "start" });
  }, [section, part, settings]);

  if (!settings) return <div className="empty">Loading settings…</div>;

  if (!section) {
    return <SettingsLanding settings={settings} activeRegistry={activeRegistry} onRunSetup={onRunSetup} onOpen={onOpen} />;
  }

  const meta = SECTIONS.find((s) => s.id === section);
  return (
    <div data-settings-section={section}>
      <p className="small settings-back"><Button variant="link" onClick={() => onOpen()}>← Settings</Button></p>
      <PageHeader title={meta?.title ?? "Settings"} sub={meta?.sentence} />
      {section === "profile" && (
        <ProfileSettings registry={activeRegistry} signedIn={signedInTo(settings, activeRegistry)} onOpenRegistry={() => onOpen("registry")} />
      )}
      {section === "registry" && (
        <RegistrySettings settings={settings} activeRegistry={activeRegistry} onChanged={onChanged} onSelectRegistry={onSelectRegistry} onOpenProfile={() => onOpen("profile")} />
      )}
      {section === "updates" && (
        <>
          <PartSection id="desktop" title="Agentry Desktop"><SelfUpdateCard /></PartSection>
          <PartSection id="quickagents" title="QuickAgents">
            <Card>
              <label className="row gap">
                <input type="checkbox" checked={false} disabled onChange={() => {}} />
                <span>Update QuickAgents automatically</span>
              </label>
              <div className="muted small">
                Not offered yet. The app checks the ones installed here for newer versions on launch and when you
                open a QuickAgent; Update on its page is yours to click.
              </div>
            </Card>
          </PartSection>
        </>
      )}
      {section === "coding-agents" && <CodingAgentsCard />}
      {section === "run-defaults" && <RunDefaultsSettings onChanged={onChanged} />}
      {section === "provider" && (
        <>
          <div id="settings-part-provider" className="settings-part">
            <RuntimeSetup advanced heading="Model provider" onOpenPrereqs={() => onOpen("run-defaults")} />
          </div>
          <PartSection id="ai-review-key" title="AI Review key"><AIReviewKeyCard /></PartSection>
        </>
      )}
    </div>
  );
}

function PartSection({ id, title, children }: { id: SettingsPart; title: string; children: React.ReactNode }) {
  return (
    <section id={`settings-part-${id}`} className="settings-part">
      <h2>{title}</h2>
      {children}
    </section>
  );
}

// The landing's status lines, one per card, from bindings the page already
// had and that read only this computer: the settings file, the self-update
// status the background check left, the coding-agent export, the runner
// policy and Docker, the AI Review key's presence. None calls a registry,
// so none carries a token. A read that fails drops its line, never its card.
type Statuses = {
  update?: main.SelfUpdateView | null;
  coding?: main.CodingAgentsView | null;
  setup?: main.SetupStatusView | null;
  jev?: main.JevKeyView | null;
  defaults?: main.RunDefaultsView | null;
};

function SettingsLanding({ settings, activeRegistry, onRunSetup, onOpen }: {
  settings: main.SettingsView;
  activeRegistry: string;
  onRunSetup?: () => void;
  onOpen: (section?: SettingsSection, part?: SettingsPart) => void;
}) {
  const [st, setSt] = useState<Statuses>({});
  useEffect(() => {
    let live = true;
    const put = <K extends keyof Statuses>(k: K) => (v: Statuses[K]) => { if (live) setSt((cur) => ({ ...cur, [k]: v })); };
    SelfUpdateAPI.SelfUpdateStatus().then(put("update")).catch(() => put("update")(null));
    SettingsAPI.CodingAgents().then(put("coding")).catch(() => put("coding")(null));
    RunAPI.SetupStatus().then(put("setup")).catch(() => put("setup")(null));
    RunAPI.JevKeyStatus().then(put("jev")).catch(() => put("jev")(null));
    SettingsAPI.RunDefaults().then(put("defaults")).catch(() => put("defaults")(null));
    return () => { live = false; };
  }, []);

  const status: Record<SettingsSection, React.ReactNode> = {
    profile: null,
    registry: registryStatus(settings, activeRegistry),
    updates: st.update ? updateStatus(st.update) : null,
    "coding-agents": st.coding ? codingStatus(st.coding) : null,
    "run-defaults": st.defaults ? runDefaultsStatus(st.defaults, st.setup ?? null) : null,
    provider: st.setup || st.jev ? providerStatus(st.setup ?? null, st.jev ?? null) : null,
  };

  return (
    <div data-settings-section="landing">
      <PageHeader
        title="Settings"
        sub="Each card opens one group of settings."
        actions={onRunSetup && <Button onClick={onRunSetup}>Run setup again</Button>}
      />
      <div className="settings-cards">
        {SECTIONS.filter((s) => s.id !== "profile" || signedInTo(settings, activeRegistry)).map((s) => (
          <div key={s.id} className="card settings-card">
            {/* The title's button is stretched over the whole card, so the
                card is one link; the part links sit above it. */}
            <button type="button" className="settings-card-link" onClick={() => onOpen(s.id)}>
              <strong>{s.title}</strong>
            </button>
            <p className="muted small">{s.sentence}</p>
            {status[s.id] && <p className="small settings-card-status" role="status">{status[s.id]}</p>}
            {s.parts.length > 1 && (
              <p className="small settings-card-parts">
                {s.parts.map((p, i) => (
                  <React.Fragment key={p.id}>
                    {i > 0 && " · "}
                    <Button variant="link" onClick={() => onOpen(s.id, p.id)}>{p.label}</Button>
                  </React.Fragment>
                ))}
              </p>
            )}
          </div>
        ))}
      </div>
      <p className="muted small selectable">
        Settings file: <span className="mono">{settings.configPath}</span>. Install records and pinned
        signing keys are shared with the CLI under the same directory.
      </p>
    </div>
  );
}

const hostOf = (url: string) => url.replace(/^https?:\/\//, "");

// signedInTo: whether the registry holds a token here, which is when the
// Profile card is offered (agentry-desktop#423). Nothing when not signed in.
export const signedInTo = (settings: main.SettingsView, url: string) =>
  settings.registries.some((r) => r.url === url && r.hasToken);

export function registryStatus(settings: main.SettingsView, activeRegistry: string): string {
  const r = settings.registries.find((x) => x.url === activeRegistry);
  if (!r) return settings.registries.length ? "No registry in use." : "No registry added yet.";
  return `${hostOf(r.url)}, ${r.hasToken ? "signed in" : "not signed in"}.`;
}

export function updateStatus(v: main.SelfUpdateView): string {
  if (!v.channel) return "This build has no update channel.";
  if (v.available) return v.available.retired
    ? `${v.available.version} exists, signed by a retired key, and will not be installed.`
    : `Agentry Desktop ${v.available.version} is available. You are running ${v.current}.`;
  if (!v.enabled) return "Not looking for newer versions.";
  if (v.err) return "The last check did not complete.";
  if (v.checked && !v.nothing) return `Up to date (${v.current}) when last checked.`;
  return `Running ${v.current}.`;
}

export function codingStatus(v: main.CodingAgentsView): string {
  const x = v.export;
  if (!x.enabled) return "Install puts QuickAgents in the runner home only.";
  return `Install also writes each QuickAgent as ${exportLabel(x.target)} for ${codingAgentName(v)}.`;
}

// The Run defaults card's line: the timeout and where runs go, and Docker
// when it is what stands in the way.
export function runDefaultsStatus(d: main.RunDefaultsView, s: main.SetupStatusView | null): string {
  const where = d.where === "agentry" ? "In Agentry where it is offered" : "on this computer";
  const line = `Runs stop after ${fmtDuration(d.timeoutSeconds)}, ${where}.${d.unsafeMode ? " Unsafe Mode is on." : ""}`;
  return s && !s.dockerOk && d.where !== "agentry" ? `${line} Docker is not ready yet.` : line;
}

export function runStatus(s: main.SetupStatusView): string {
  if (!s.dockerOk) return "Docker is not ready, so QuickAgents cannot run on this computer yet.";
  if (!s.imagesReady) return "Docker answers. The run images are fetched on the first run.";
  return "Docker answers and the run images are ready.";
}

// The key's line says where it comes from and never what it is (D-040):
// "saved" only when the keychain holds it and the policy reads it there.
export function providerStatus(s: main.SetupStatusView | null, k: main.JevKeyView | null): string {
  const parts: string[] = [];
  if (s) parts.push(s.policyExists && s.model ? `${providerLabel(s.provider)}, ${s.model}` : "No provider chosen yet");
  if (k) {
    const standing = keyStanding(k);
    parts.push(standing === "env" ? `AI Review key from $${k.envName}` : standing === "saved" ? "AI Review key saved" : "No AI Review key");
  }
  return parts.join(" · ") + ".";
}

// RunDefaultsSettings is Settings → Run defaults (plan settings-cards.md
// D2, D13; agentry-desktop#347): the timeout and max turns the runner policy
// holds, which the CLI runs with too; where the run setup starts; the output
// folder it fills in; and the Docker engine's prerequisites. None of them is
// a consent: every run still asks what it asks. Unsafe Mode's switch is here
// too (agentry-desktop#418): the page asks Go to turn it on, and Go asks the
// person in a native dialog and pulls the image before it reads on.
function RunDefaultsSettings({ onChanged }: { onChanged: () => Promise<unknown> }) {
  const [setup, setSetup] = useState<main.SetupStatusView | null>(null);
  const [defaults, setDefaults] = useState<main.RunDefaultsView | null>(null);
  const [timeoutText, setTimeoutText] = useState("");
  const [turns, setTurns] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const read = () => RunAPI.SetupStatus().then(setSetup).catch((e) => setErr(String(e)));
  // The fields take the saved budgets on the first read and after a budgets
  // save only, so choosing where to run keeps a timeout typed and not saved.
  const show = (v: main.RunDefaultsView, budgets: boolean) => {
    setDefaults(v);
    if (!budgets) return;
    setTimeoutText(String(v.timeoutSeconds));
    setTurns(String(v.maxTurns));
  };
  useEffect(() => {
    read();
    SettingsAPI.RunDefaults().then((v) => show(v, true)).catch((e) => setErr(String(e)));
  }, []);

  async function act(fn: () => Promise<main.RunDefaultsView>, said?: string) {
    setBusy(true);
    setErr(null);
    setSaved(null);
    try {
      show(await fn(), !!said);
      if (said) setSaved(said);
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }

  // Unsafe Mode: the pull's lines while Go turns it on (runner:unsafe-progress),
  // and the shell's settings read again after, so the edge and the badge follow.
  const [unsafeBusy, setUnsafeBusy] = useState(false);
  const [unsafeLog, setUnsafeLog] = useState<string[]>([]);
  const [unsafeErr, setUnsafeErr] = useState<string | null>(null);
  const [unsafeSaid, setUnsafeSaid] = useState<string | null>(null);
  // docker prints a line per layer and step; the last lines are the state.
  const UNSAFE_LOG_LINES = 200;
  useEffect(() => EventsOn("runner:unsafe-progress", (line: string) => setUnsafeLog((p) => [...p.slice(1 - UNSAFE_LOG_LINES), line])), []);
  async function setUnsafeMode(on: boolean) {
    setUnsafeBusy(true);
    setUnsafeErr(null);
    setUnsafeSaid(null);
    setUnsafeLog([]);
    try {
      show(await RunAPI.SetUnsafeMode(on), false);
      await onChanged();
    } catch (e) {
      // A No in the native dialog is the person's choice, not a failure.
      if (String(e).includes("not confirmed")) setUnsafeSaid("Nothing was changed.");
      else setUnsafeErr(String(e));
    } finally {
      setUnsafeBusy(false);
    }
  }

  const timeoutSeconds = parseInt(timeoutText, 10);
  const maxTurns = parseInt(turns, 10);
  const budgetsValid = /^\d+$/.test(timeoutText.trim()) && timeoutSeconds >= 1 && /^\d+$/.test(turns.trim()) && maxTurns >= 0;
  const budgetsChanged = !!defaults && (timeoutSeconds !== defaults.timeoutSeconds || maxTurns !== defaults.maxTurns);

  return (
    <>
      <ErrorBox error={err} />
      {defaults && (
        <>
          <PartSection id="budgets" title="Timeout and turns">
            <Card>
              <div className="row">
                <label className="row" style={{ gap: 6 }}>
                  <span className="small">Timeout (seconds)</span>
                  <Input type="number" min={1} style={{ width: 120 }} value={timeoutText} disabled={busy}
                    onChange={(e) => { setTimeoutText(e.target.value); setSaved(null); }} />
                </label>
                <label className="row" style={{ gap: 6 }}>
                  <span className="small">Max turns (0 = unlimited)</span>
                  <Input type="number" min={0} style={{ width: 120 }} value={turns} disabled={busy}
                    onChange={(e) => { setTurns(e.target.value); setSaved(null); }} />
                </label>
                <Button variant="primary" disabled={busy || !budgetsValid || !budgetsChanged}
                  title={!budgetsValid ? "a timeout of at least 1 second, and 0 or more turns" : ""}
                  onClick={() => act(() => SettingsAPI.SetRunBudgets(timeoutSeconds, maxTurns), "Saved.")}>
                  Save
                </Button>
              </div>
              <p className="muted small" style={{ marginBottom: 0 }}>
                A run stops when it reaches either. These are the runner policy&apos;s
                (<span className="mono selectable">{defaults.policyPath}</span>), so the <span className="mono">agentry</span> CLI
                runs with them too. A run&apos;s own fields under Advanced can only ask for less. A draft made on this
                computer has at least 30 minutes and a knowledge build at least 60, however short the timeout here.
              </p>
              {saved && <p className="small" role="status" style={{ marginBottom: 0 }}>{saved}</p>}
            </Card>
          </PartSection>
          <PartSection id="where" title="Where to run">
            <Card>
              <div className="row" style={{ gap: 14 }}>
                {([["here", "This computer"], ["agentry", "In Agentry"]] as [RunWhere, string][]).map(([w, label]) => (
                  <label key={w} className="check-row" style={{ margin: 0 }}>
                    <input type="radio" name="run-where" checked={defaults.where === w} disabled={busy}
                      onChange={() => act(() => SettingsAPI.SetRunWhere(w))} />
                    <span>{label}</span>
                  </label>
                ))}
              </div>
              <p className="muted small" style={{ marginBottom: 0 }}>
                The run setup starts on this choice, and you can change it for any run. Where a run In Agentry is not
                offered for a QuickAgent, it starts on this computer and says why. A run In Agentry still shows the credits
                it holds before you start it, and every consent is asked on every run.
              </p>
            </Card>
          </PartSection>
          <PartSection id="output" title="Output folder">
            <Card>
              <div className="row">
                <span className="grow mono selectable">
                  {defaults.outputDir || <span className="muted">None chosen: each run writes under ~/Downloads/agentry-out.</span>}
                </span>
                <Button disabled={busy} onClick={() => act(SettingsAPI.ChooseDefaultOutputDir)}>
                  {defaults.outputDir ? "Choose another…" : "Choose…"}
                </Button>
                {defaults.outputDir && <Button disabled={busy} onClick={() => act(SettingsAPI.ClearDefaultOutputDir)}>Clear</Button>}
              </div>
              <p className="muted small" style={{ marginBottom: 0 }}>
                Chosen here, through the file dialog. The run setup fills it in and you can pick another for one run.
                Each run writes into a folder of its own inside it. A run never reads from it unless you add it as an input.
              </p>
            </Card>
          </PartSection>
          <PartSection id="unsafe" title="Unsafe Mode">
            <Card>
              <label className="check-row" style={{ margin: 0 }}>
                <input type="checkbox" role="switch" checked={defaults.unsafeMode} disabled={busy || unsafeBusy}
                  onChange={(e) => void setUnsafeMode(e.target.checked)} />
                <span>
                  <strong>Unsafe Mode</strong>{" "}
                  <span className={`badge ${defaults.unsafeMode ? "bad" : ""}`} data-unsafe-state>{defaults.unsafeMode ? "On" : "Off"}</span>
                </span>
              </label>
              <p className="small">
                While this is on, a QuickAgent run on this computer can use commands this computer has not approved. Each
                command the QuickAgent declares is installed from Kali Linux&apos;s repository when it first runs, or from
                PyPI for a tool Kali does not package. It runs with that image&apos;s own PATH, in an image that keeps
                everything it installed, so it can start any program the image holds. A command that cannot be installed is
                left out, and the run goes on without it.
              </p>
              <p className="muted small">
                <span className="mono">run_command</span> still takes only the commands a QuickAgent declares. The
                container&apos;s isolation, its network limits and every consent stay as they are, and a run In Agentry never
                uses Unsafe Mode. Turning it on asks you to confirm, then downloads the Unsafe Mode image, a few hundred
                megabytes. While it is on, a red edge runs round the window and the sidebar says so. Turning it off applies
                from the next run or chat. One already running keeps Unsafe Mode, with the red edge, until it ends. Turning
                it off keeps the images: Docker images, below, removes them.
              </p>
              {defaults.unsafeImage && (
                <p className="muted small" style={{ marginBottom: 0 }}>
                  The image: <span className="mono selectable">{defaults.unsafeImage}</span>
                </p>
              )}
              {unsafeBusy && !defaults.unsafeMode && (
                <div role="status" className="small" style={{ marginTop: 8 }}>
                  <Spinner label={unsafeLog.length ? "Downloading the Unsafe Mode image…" : "Waiting for your answer in the dialog…"} />
                  {unsafeLog.length > 0 && <pre className="run-log selectable" data-unsafe-log>{unsafeLog.join("\n")}</pre>}
                </div>
              )}
              {unsafeSaid && <p className="small" role="status" style={{ marginBottom: 0 }}>{unsafeSaid}</p>}
              <ErrorBox error={unsafeErr} />
            </Card>
          </PartSection>
        </>
      )}
      <PartSection id="docker" title="Docker engine">
        {setup && (setup.dockerOk
          ? <Card><p className="small" style={{ margin: 0 }}>{runStatus(setup)}</p></Card>
          : <PrereqCard onDone={async () => { await read(); }} />)}
      </PartSection>
      <PartSection id="images" title="Docker images"><DockerImagesCard /></PartSection>
    </>
  );
}

// RegistrySettings is Settings → Registry settings: the registries with
// their sign-in, account and organizations lines, the account's secrets,
// and Add a registry — the cards that opened the one Settings page before.
function RegistrySettings({ settings, activeRegistry, onChanged, onSelectRegistry, onOpenProfile }: {
  settings: main.SettingsView;
  activeRegistry: string;
  onChanged: () => Promise<main.SettingsView>;
  onSelectRegistry: (url: string) => void;
  onOpenProfile: () => void;
}) {
  const [newURL, setNewURL] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [signIn, setSignIn] = useState<{ url: string; token: string } | null>(null);
  const [busy, setBusy] = useState(false);
  // The registry a browser sign-in is waiting on; shows the Cancel control.
  const [browserWait, setBrowserWait] = useState<string | null>(null);
  // The account behind each stored credential, for its "signed in as" line;
  // the tier, the credits and the organizations are the Profile card's
  // since agentry-desktop#423. Read after the registries render, so a slow
  // registry never holds the page; a read that fails leaves the card as it
  // was.
  const [accounts, setAccounts] = useState<Record<string, main.UserView>>({});
  useEffect(() => {
    let cancelled = false;
    const signedIn = settings.registries.filter((r) => r.hasToken);
    Promise.all(signedIn.map(async (r) => [r.url, await SettingsAPI.Whoami(r.url).catch(() => null)] as const)).then((pairs) => {
      if (cancelled) return;
      const next: Record<string, main.UserView> = {};
      for (const [url, user] of pairs) if (user) next[url] = user;
      setAccounts(next);
    });
    return () => { cancelled = true; };
  }, [settings.registries]);

  async function wrap(fn: () => Promise<void>) {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await fn();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  const addRegistry = () =>
    wrap(async () => {
      await SettingsAPI.AddRegistry(newURL.trim());
      setNewURL("");
      await onChanged();
    });

  const doSignIn = () =>
    wrap(async () => {
      if (!signIn) return;
      const res = await SettingsAPI.SignIn(signIn.url, signIn.token.trim());
      setNotice(
        `Signed in to ${signIn.url} as ${res.user.handle} — PAT stored in ${res.tokenStore === "keyring" ? "the OS keychain" : "the settings file (0600; no keychain available)"}. Signing keys: ${res.pinNote}${rewrapLine(res)}`,
      );
      setSignIn(null);
      await onChanged();
    });

  return (
    <>
      <p className="muted small">
        Credentials live in the OS keychain under the same convention the <span className="mono">agentry</span> CLI
        uses, so a sign-in here works there too.
      </p>
      <ErrorBox error={error} />
      {browserWait && (
        <Notice tone="info">
          Waiting for your browser to authorize this app for <span className="mono">{browserWait}</span>.
          Sign in there and click “Authorize the desktop app”. If the browser did not open, or you
          closed it, cancel and try again.{" "}
          <Button onClick={() => void SettingsAPI.CancelBrowserSignIn()}>Cancel sign-in</Button>
        </Notice>
      )}
      {notice && <Notice tone="ok">{notice}</Notice>}

      <section id="settings-part-registries" className="settings-part">
      <h2>Registries</h2>
      {settings.registries.map((r) => (
        <Card key={r.url}>
          <div className="spread">
            <div className="grow">
              <div className="row">
                <strong className="mono selectable">{r.url}</strong>
                {r.isDefault && <span className="badge kind">default</span>}
                {r.url === activeRegistry && <span className="badge ok">active</span>}
                {r.hasToken
                  ? <span className="badge">signed in ({r.tokenSource})</span>
                  : <span className="badge warn">anonymous</span>}
              </div>
              {/* Who the credential is; the account and the subscription
                  are the Profile card's since agentry-desktop#423. */}
              {accounts[r.url] && (
                <div className="muted small" style={{ marginTop: 4 }}>
                  signed in as <span className="selectable">{accounts[r.url].handle}</span>
                  {r.url === activeRegistry
                    ? <> · <Button variant="link" onClick={onOpenProfile}>Profile and subscription</Button></>
                    : " · Use it to see its profile and subscription"}
                </div>
              )}
            </div>
            <div className="row nowrap">
              {r.url !== activeRegistry && (
                <Button onClick={() => onSelectRegistry(r.url)}>Use</Button>
              )}
              {!r.isDefault && (
                <Button disabled={busy} onClick={() => wrap(async () => { await SettingsAPI.SetDefault(r.url); await onChanged(); })}>
                  Make default
                </Button>
              )}
              {r.hasToken ? (
                <Button disabled={busy} onClick={() => wrap(async () => { await SettingsAPI.SignOut(r.url); await onChanged(); setNotice(`Signed out of ${r.url}.`); })}>
                  Sign out
                </Button>
              ) : (
                <>
                  <Button
                    variant="primary"
                    disabled={busy}
                    title="Opens the registry in your browser — sign in there (the directory service, MFA included) and approve the app"
                    onClick={() => wrap(async () => {
                      setBrowserWait(r.url);
                      try {
                        const res = await SettingsAPI.SignInViaBrowser(r.url);
                        setNotice(
                          `Signed in to ${r.url} as ${res.user.handle} — PAT stored in ${res.tokenStore === "keyring" ? "the OS keychain" : "the settings file (0600)"}. Signing keys: ${res.pinNote}${rewrapLine(res)}`,
                        );
                        await onChanged();
                      } catch (e) {
                        // Cancelled by the user (Cancel sign-in): a notice, not an error.
                        if (String(e).includes("cancelled")) {
                          setNotice(`Sign-in to ${r.url} cancelled.`);
                          return;
                        }
                        throw e;
                      } finally {
                        setBrowserWait(null);
                      }
                    })}
                  >
                    Sign in via browser
                  </Button>
                  <Button disabled={busy} onClick={() => setSignIn({ url: r.url, token: "" })}>
                    Paste token
                  </Button>
                </>
              )}
              <Button variant="danger" disabled={busy} onClick={() => wrap(async () => { await SettingsAPI.RemoveRegistry(r.url); await onChanged(); })}>
                Remove
              </Button>
            </div>
          </div>
        </Card>
      ))}
      </section>
      {/* The account's secrets for runs In Agentry, where the active
          registry keeps them (agentry-desktop#300). */}
      {activeRegistry && <SecretsCard registry={activeRegistry} />}

      <section id="settings-part-add-registry" className="settings-part">
      <h2>Add a registry</h2>
      <Toolbar>
        <Input
          type="text"
          className="grow mono"
          placeholder="https://secureagentry.net — or http://agentry:8080 for a dev stack"
          value={newURL}
          onChange={(e) => setNewURL(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && newURL.trim() && addRegistry()}
        />
        <Button variant="primary" disabled={busy || !newURL.trim()} onClick={addRegistry}>Add</Button>
      </Toolbar>
      </section>

      {signIn && (
        <Modal title={`Sign in to ${signIn.url}`} onClose={() => setSignIn(null)}>
            <p className="muted">
              Paste a personal access token. Create one on the registry's site under Settings →
              Authentication (<span className="mono">/ui/account/tokens</span> on a registry without the
              site), or with <span className="mono">agentry login</span>. Signing in also pins the registry's
              signing keys on first use (TOFU).
            </p>
            <Input
              type="password"
              style={{ width: "100%" }}
              placeholder="agy_…"
              value={signIn.token}
              onChange={(e) => setSignIn({ ...signIn, token: e.target.value })}
              onKeyDown={(e) => e.key === "Enter" && signIn.token.trim() && doSignIn()}
              autoFocus
            />
            <div className="row" style={{ marginTop: 14, justifyContent: "flex-end" }}>
              <Button onClick={() => setSignIn(null)}>Cancel</Button>
              <Button variant="primary" disabled={busy || !signIn.token.trim()} onClick={doSignIn}>
                {busy ? "Checking…" : "Sign in"}
              </Button>
            </div>
        </Modal>
      )}
    </>
  );
}

// SelfUpdateCard shows whether a newer Agentry Desktop exists and lets the
// person stop the app looking (agentry-desktop#87).
//
// Notify only, deliberately: there is no Install button here. Applying an
// update is M7 on Windows and macOS, and on Linux it stays the package
// manager's job (D7) — so the card says where the release is and leaves the
// choice where it already was.
function SelfUpdateCard() {
  const [view, setView] = useState<main.SelfUpdateView | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    SelfUpdateAPI.SelfUpdateStatus().then(setView).catch(() => setView(null));
  }, []);

  const run = async (fn: () => Promise<main.SelfUpdateView>) => {
    setBusy(true);
    try {
      setView(await fn());
    } catch {
      // The card keeps what it had; a failed read is not worth a dialog.
    } finally {
      setBusy(false);
    }
  };

  if (!view) return null;

  // Installing needs a platform that installs AND a copy this person can
  // replace; the Go side has already decided both.
  const canInstall = view.canApply && !view.available?.retired;

  // A build anyone can make from the open core has no channel to check, which
  // is a fact to state rather than a failure to report.
  if (!view.channel) {
    return (
      <Card>
        <div className="muted small">
          This build has no update channel, so it does not check for a newer Agentry Desktop.
          Builds from the open core are like this; the ones on the download page are not.
        </div>
      </Card>
    );
  }

  return (
    <Card>
      <label className="row gap">
        <input
          type="checkbox"
          checked={view.enabled}
          disabled={busy}
          onChange={(e) => run(() => SelfUpdateAPI.SetSelfUpdateCheckEnabled(e.target.checked))}
        />
        <span>Check for newer versions</span>
      </label>
      <div className="muted small">
        On launch and once a day, from <span className="mono">{view.channel}</span>. The signature on
        a release is checked against the key this build carries before anything is shown.
      </div>

      {view.available && (
        <Notice>
          <strong>Agentry Desktop {view.available.version} is available.</strong>{" "}
          You are running {view.current}. Signed with the {view.available.signedBy} key
          {view.available.retired && <> — which has since been retired, so this release is genuine but out of date and will not be installed</>}
          {"."}
          {/* Installing is offered only where the app can do it: not on Linux,
              where the package manager owns the installed files (D7), and not
              on a copy an administrator installed for everyone — there the
              card explains instead of offering a button that would fail. */}
          {canInstall && (
            <div className="row gap install-row">
              <Button variant="primary" disabled={busy} onClick={() => run(SelfUpdateAPI.InstallSelfUpdate)}>
                {view.applying ? "Installing…" : "Install and relaunch"}
              </Button>
              <span className="muted small">The app closes and reopens on the new version.</span>
            </div>
          )}
          {!canInstall && view.cannotApplyWhy && (
            <div className="muted small">{view.cannotApplyWhy} Download it instead:{" "}
              <span className="mono selectable">{view.downloadUrl}</span>.
            </div>
          )}
          {!canInstall && !view.cannotApplyWhy && (
            <div className="muted small">
              {view.downloadUrl
                ? <>Get it from <span className="mono selectable">{view.downloadUrl}</span>.</>
                : <>Get it from the site you downloaded this copy from.</>}{" "}
              On this platform a newer version is installed by your package manager, not by the app.
            </div>
          )}
        </Notice>
      )}

      {canInstall && (
        <label className="row gap">
          <input
            type="checkbox"
            checked={view.onQuit}
            disabled={busy}
            onChange={(e) => run(() => SelfUpdateAPI.SetInstallOnQuit(e.target.checked))}
          />
          <span>Install automatically when I quit</span>
        </label>
      )}

      {view.err && <ErrorBox error={`The last check did not complete: ${view.err}`} />}

      {view.nothing && (
        <div className="muted small">
          This channel publishes no signed release yet, so there is nothing to compare against.
        </div>
      )}

      {view.enabled && !view.available && !view.err && !view.nothing && view.checked && (
        <div className="muted small">No newer version when last checked.</div>
      )}

      <Toolbar>
        <Button disabled={busy || !view.enabled} onClick={() => run(SelfUpdateAPI.SelfUpdateCheckNow)}>
          {busy ? "Checking…" : "Check now"}
        </Button>
      </Toolbar>
    </Card>
  );
}
