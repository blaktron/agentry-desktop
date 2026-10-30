import React, { useEffect, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/app/SettingsAPI";
import * as SelfUpdateAPI from "../../wailsjs/go/app/SelfUpdateAPI";
import * as WhatsNewAPI from "../../wailsjs/go/app/WhatsNewAPI";
import { OpenExternal } from "../../wailsjs/go/app/App";
import * as RunAPI from "../../wailsjs/go/app/RunAPI";
import * as HomeAPI from "../../wailsjs/go/app/HomeAPI";
import * as DestinationsAPI from "../../wailsjs/go/app/DestinationsAPI";
import * as PersistentAPI from "../../wailsjs/go/app/PersistentAPI";
import { app, destinations } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { Button, Card, ErrorBox, Input, Modal, Notice, PageHeader, Spinner, Toolbar } from "../ui";
import { fmtDuration } from "../usage";
import { rewrapLine } from "../knowledge";
import { RuntimeSetup, PrereqCard, EngineWarnings, CHECKING_ENGINE, providerLabel } from "./RuntimeSetup";
import { paidBy } from "../modelSetup";
import { CodingAgentsCard, codingAgentName, exportLabel } from "./CodingAgents";
import { SecretsCard } from "./HostedRun";
import { AIReviewKeyCard } from "./AIReviewKey";
import { HybridAnalysisKeyCard } from "./HybridAnalysisKey";
import { WebSearchKeyCard } from "./WebSearchKey";
import { keyStanding } from "../keyRecommendation";
import type { RunWhere } from "../runSetup";
import { ProfileSettings } from "./SettingsProfile";
import { ReportDesignSettings } from "./SettingsReportDesign";
import { DockerImagesCard } from "./DockerImages";
import { AgentryCommandCard } from "./AgentryCommand";
import WhatsNew from "./WhatsNew";
import type { WhatsNewData } from "../whatsNew";
import { VoiceSwitch } from "./Voice";
import { followPart } from "../followPart";
import { HomeSettings } from "./SettingsHome";
import { homeStatus } from "../homeCards";
import { settingsLine as deliverablesLine } from "../deliverables";
import { ConnectionsSettings, connectionsStatus } from "./SettingsConnections";
import { DestinationsSettings, destinationsStatus } from "./SettingsDestinations";
import { LicencesSettings } from "./SettingsLicences";
import PersistentAgentsSettings from "./SettingsPersistentAgents";
import { persistentAgentsStatus } from "../persistentAgents";
import { asSentence, signInLabel, signInLine, twoSignIns, type SignInKind } from "../signIns";

// Settings as five cards: the landing is a card per section, each card the
// whole of a link to its section page, with one sentence, a status line
// and its parts as links; each section page holds that section's cards,
// under a "← Settings" link back.
// Profile is a sixth, offered only while the active
// registry holds a token. Home is the last: which
// cards Home shows, its pins, and Reset Home. Connections
// sits before Home: the OAuth connections kept on this
// computer, with Disconnect, and the person's own OAuth clients.
//
// Report design comes second, after Profile, as on the site: the account's
// report design and its editor.
export type SettingsSection = "profile" | "report-design" | "registry" | "updates" | "coding-agents" | "persistent-agents" | "run-defaults" | "provider" | "connections" | "destinations" | "home" | "licences";

// The parts a landing link or a deep link can name; each section page
// renders its part under the anchor settings-part-<id>.
export type SettingsPart =
  | "profile" | "subscription" | "account-voice"
  | "themes" | "design-editor" | "design-preview"
  | "registries" | "add-registry" | "desktop" | "quickagents"
  | "budgets" | "chats" | "where" | "output" | "deliverables" | "history" | "voice" | "unsafe" | "malware" | "docker" | "images" | "provider" | "ai-review-key" | "web-search" | "hybrid-analysis-key"
  | "connections" | "own-clients" | "destinations"
  | "theme" | "home-agent" | "home-cards" | "pins" | "reset-home" | "licences";
type Part = { id: SettingsPart; label: string };

// SECTIONS is the landing's order and each section's parts. A part's id is
// the anchor its section page scrolls to when the landing's link names it.
// needsSignIn: the landing offers the card, and the sidebar the row, only
// while the active registry is signed in.
export const SECTIONS: { id: SettingsSection; title: string; sentence: string; parts: Part[]; needsSignIn?: boolean }[] = [
  {
    id: "profile", title: "Profile", needsSignIn: true,
    sentence: "Who you are signed in as, your display name, and your subscription, as your account page on the site shows them.",
    parts: [{ id: "profile", label: "Profile" }, { id: "subscription", label: "Subscription" }, { id: "account-voice", label: "Writing Voice" }],
  },
  {
    id: "report-design", title: "Report design",
    sentence: "How the PDFs your runs export look: a theme to start from, the accent, the fonts and the paper, kept on your account.",
    parts: [{ id: "themes", label: "Themes" }, { id: "design-editor", label: "Editor" }, { id: "design-preview", label: "Preview" }],
  },
  {
    id: "registry", title: "Registry settings",
    sentence: "The registries this app talks to, your sign-in to each, and the secrets runs In Agentry use where a registry keeps them.",
    parts: [{ id: "registries", label: "Registries" }, { id: "add-registry", label: "Add a registry" }],
  },
  {
    id: "updates", title: "Update settings",
    sentence: "Whether this app looks for a newer Agentry Desktop, the agentry command it carries on a Mac, and how QuickAgents installed here are updated.",
    parts: [{ id: "desktop", label: "Agentry Desktop" }, { id: "quickagents", label: "QuickAgents" }],
  },
  {
    id: "coding-agents", title: "Coding tools",
    sentence: "The coding tool on this computer (Claude Code or Codex), and whether Install also puts a QuickAgent where it reads.",
    parts: [],
  },
  {
    id: "persistent-agents", title: "Persistent Agents",
    sentence: "The agents that keep their conversation and their own files between the times you talk to them: turn them on, start and stop them, and configure what each is given.",
    parts: [],
  },
  {
    id: "run-defaults", title: "Run defaults",
    sentence: "How long a run may take, when a chat ends, where runs go and where their files land, where the files a run was for are copied, which runs are remembered, and the container engine runs on this computer use, with Agentry's images on it.",
    parts: [
      { id: "budgets", label: "Timeout and turns" }, { id: "chats", label: "Chat idle timeout" }, { id: "where", label: "Where to run" },
      { id: "output", label: "Output folder" }, { id: "deliverables", label: "Deliverables" }, { id: "history", label: "Recent runs" }, { id: "unsafe", label: "Unsafe Mode" },
      { id: "malware", label: "Malware check" }, { id: "docker", label: "Container engine" }, { id: "images", label: "Container images" }, { id: "voice", label: "Writing Voice" },
    ],
  },
  {
    id: "provider", title: "Provider settings",
    sentence: "The AI provider and model runs use, your own AI Review key, your own web search key, and your own Hybrid Analysis key.",
    parts: [
      { id: "provider", label: "Model provider" }, { id: "ai-review-key", label: "AI Review key" },
      { id: "web-search", label: "Web search" }, { id: "hybrid-analysis-key", label: "Hybrid Analysis key" },
    ],
  },
  {
    id: "connections", title: "Connections",
    sentence: "The sign-ins to MCP servers kept on this computer, one per QuickAgent version, with Disconnect, and your own OAuth clients.",
    parts: [{ id: "connections", label: "Connections" }, { id: "own-clients", label: "Your OAuth clients" }],
  },
  {
    id: "destinations", title: "Destinations",
    sentence: "The webhooks and S3 buckets a run may send its result and deliverables to, each with its credentials kept in the system keychain.",
    parts: [{ id: "destinations", label: "Destinations" }],
  },
  {
    id: "home", title: "Home",
    sentence: "Which cards Home shows, the QuickAgents pinned to it, Reset Home, and the theme Agentry Desktop is drawn in.",
    parts: [{ id: "theme", label: "Theme" }, { id: "home-agent", label: "Agent on Home" }, { id: "home-cards", label: "Cards" }, { id: "pins", label: "Pinned" }, { id: "reset-home", label: "Reset Home" }],
  },
  {
    id: "licences", title: "Licences",
    sentence: "The licences of the open-source software, fonts and logos inside Agentry Desktop.",
    parts: [],
  },
];

// Where each place that opens Settings goes: straight
// to the section it means rather than to the landing.
export const SETTINGS_FOR = {
  // the sidebar's line for a newer Agentry Desktop
  selfUpdate: { section: "updates", part: "desktop" },
  // the "Setup isn't finished" notice on Home
  setupNotice: { section: "provider" },
  // Home's Profile, Sign in and Run defaults cards
  homeProfile: { section: "profile" },
  homeSignIn: { section: "registry", part: "registries" },
  homeRunDefaults: { section: "run-defaults" },
  // Home's Deliverables card
  homeDeliverables: { section: "run-defaults", part: "deliverables" },
  // Home's Recent runs card
  homeRunHistory: { section: "run-defaults", part: "history" },
  // Home's own Home settings link, and the line when every card is hidden
  //
  homeSettings: { section: "home" },
  // Knowledge's "Not signed in" notice
  knowledgeSignIn: { section: "registry", part: "registries" },
  // the Publisher section's Sign in, and its "sign in again" notice for a
  // sign-in without agent:manage
  publisherSignIn: { section: "registry", part: "registries" },
  // the Report bug card's Sign in, signed out or refused
  bugReportSignIn: { section: "registry", part: "registries" },
  // Build's Describe it, whose drafting needs the AI Review key
  describeKey: { section: "provider", part: "ai-review-key" },
  // Review Your QuickAgents locally, which needs the AI Review key
  reviewKey: { section: "provider", part: "ai-review-key" },
  // the sidebar's Unsafe Mode badge
  unsafeMode: { section: "run-defaults", part: "unsafe" },
  // Home's Main card's title, and its "Set up a model" link
  homeMain: { section: "persistent-agents" },
  mainModel: { section: "provider", part: "provider" },
} satisfies Record<string, { section: SettingsSection; part?: SettingsPart }>;

export const sectionTitle = (s: SettingsSection) => SECTIONS.find((x) => x.id === s)?.title ?? "Settings";

export default function Settings({
  settings, activeRegistry, onChanged, onSelectRegistry, onRunSetup, section, part, onOpen, onOpenAgent,
}: {
  settings: app.SettingsView | null;
  activeRegistry: string;
  onChanged: () => Promise<app.SettingsView>;
  onSelectRegistry: (url: string) => void;
  // Re-open the first-run guided setup. Optional so the page renders in the
  // harness without the App shell.
  onRunSetup?: () => void;
  // The section page shown, or the landing when absent; part is the anchor
  // within it the landing's link named.
  section?: SettingsSection;
  part?: SettingsPart;
  // Opens a section (and a part of it), or the landing with none.
  onOpen: (section?: SettingsSection, part?: SettingsPart) => void;
  // Opens a persistent agent's page.
  onOpenAgent?: (name: string) => void;
}) {
  // A part named by a link is scrolled to once its section has rendered, and
  // followed while the section's own reads fill it in;
  // the page's own scroll starts at the top otherwise. Only the view's first
  // load counts, so a save that refreshes it does not pull the page back.
  const loaded = !!settings;
  useEffect(() => {
    if (!section || !loaded || !part) return;
    return followPart(`settings-part-${part}`);
  }, [section, part, loaded]);

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
        <>
          <ProfileSettings registry={activeRegistry} signedIn={signedInTo(settings, activeRegistry)} onOpenRegistry={() => onOpen("registry")} />
          {signedInTo(settings, activeRegistry) && (
            <PartSection id="account-voice" title="Writing Voice"><VoiceSwitch registry={activeRegistry} place="account" /></PartSection>
          )}
        </>
      )}
      {section === "report-design" && (
        <ReportDesignSettings registry={activeRegistry} signedIn={signedInTo(settings, activeRegistry)} onOpenRegistry={() => onOpen("registry", "registries")} />
      )}
      {section === "registry" && (
        <RegistrySettings settings={settings} activeRegistry={activeRegistry} onChanged={onChanged} onSelectRegistry={onSelectRegistry} onOpenProfile={() => onOpen("profile")} />
      )}
      {section === "updates" && (
        <>
          <PartSection id="desktop" title="Agentry Desktop"><SelfUpdateCard /><AgentryCommandCard /></PartSection>
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
      {section === "persistent-agents" && <PersistentAgentsSettings registry={activeRegistry} onOpenAgent={(name) => onOpenAgent?.(name)} />}
      {section === "run-defaults" && (
        <>
          <RunDefaultsSettings onChanged={onChanged} />
          <PartSection id="voice" title="Writing Voice"><VoiceSwitch registry={activeRegistry} place="here" /></PartSection>
        </>
      )}
      {section === "provider" && (
        <>
          <div id="settings-part-provider" className="settings-part">
            <RuntimeSetup advanced heading="Model provider" onOpenPrereqs={() => onOpen("run-defaults")} />
          </div>
          <PartSection id="ai-review-key" title="AI Review key"><AIReviewKeyCard /></PartSection>
          <PartSection id="web-search" title="Web search"><WebSearchKeyCard /></PartSection>
          <PartSection id="hybrid-analysis-key" title="Hybrid Analysis key"><HybridAnalysisKeyCard /></PartSection>
        </>
      )}
      {section === "connections" && <ConnectionsSettings />}
      {section === "destinations" && <DestinationsSettings />}
      {section === "home" && <HomeSettings />}
      {section === "licences" && <LicencesSettings />}
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
// policy and Docker, the AI Review key's presence, Home's layout in the
// settings file. None calls a registry,
// so none carries a token. A read that fails drops its line, never its card.
type Statuses = {
  update?: app.SelfUpdateView | null;
  coding?: app.CodingAgentsView | null;
  setup?: app.SetupStatusView | null;
  jev?: app.JevKeyView | null;
  defaults?: app.RunDefaultsView | null;
  home?: app.HomeLayoutView | null;
  connections?: app.KeptConnectionsView | null;
  destinations?: destinations.Status | null;
  agents?: app.PersistentAgentView[] | null;
};

function SettingsLanding({ settings, activeRegistry, onRunSetup, onOpen }: {
  settings: app.SettingsView;
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
    HomeAPI.Layout().then(put("home")).catch(() => put("home")(null));
    // Through a promise, so a binding an older harness entry lacks drops
    // the line, not the landing.
    Promise.resolve().then(() => RunAPI.Connections()).then(put("connections")).catch(() => put("connections")(null));
    Promise.resolve().then(() => DestinationsAPI.List()).then(put("destinations")).catch(() => put("destinations")(null));
    Promise.resolve().then(() => PersistentAPI.List()).then(put("agents")).catch(() => put("agents")(null));
    return () => { live = false; };
  }, []);

  const status: Record<SettingsSection, React.ReactNode> = {
    profile: null,
    "report-design": null,
    registry: registryStatus(settings, activeRegistry),
    updates: st.update ? updateStatus(st.update) : null,
    "coding-agents": st.coding ? codingStatus(st.coding) : null,
    "persistent-agents": st.agents ? persistentAgentsStatus(st.agents) : null,
    "run-defaults": st.defaults ? runDefaultsStatus(st.defaults, st.setup ?? null) : null,
    provider: st.setup || st.jev ? providerStatus(st.setup ?? null, st.jev ?? null) : null,
    connections: st.connections ? connectionsStatus(st.connections) : null,
    destinations: st.destinations ? destinationsStatus(st.destinations) : null,
    home: st.home ? homeStatus(st.home) : null,
    licences: null,
  };

  return (
    <div data-settings-section="landing">
      <PageHeader
        title="Settings"
        sub="Each card opens one group of settings."
        actions={onRunSetup && <Button onClick={onRunSetup}>Run setup again</Button>}
      />
      <div className="settings-cards">
        {SECTIONS.filter((s) => !s.needsSignIn || signedInTo(settings, activeRegistry)).map((s) => (
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

export const hostOf = (url: string) => url.replace(/^https?:\/\//, "");

// signedInTo: whether the registry holds a token here, which is when the
// Profile card is offered. Nothing when not signed in.
export const signedInTo = (settings: app.SettingsView, url: string) =>
  settings.registries.some((r) => r.url === url && r.hasToken);

export function registryStatus(settings: app.SettingsView, activeRegistry: string): string {
  const r = settings.registries.find((x) => x.url === activeRegistry);
  if (!r) return settings.registries.length ? "No registry in use." : "No registry added yet.";
  return `${hostOf(r.url)}, ${r.hasToken ? "signed in" : "not signed in"}.`;
}

export function updateStatus(v: app.SelfUpdateView): string {
  if (!v.channel) return "This build has no update channel.";
  if (v.available) return v.available.retired
    ? `${v.available.version} exists, signed by a retired key, and will not be installed.`
    : `Agentry Desktop ${v.available.version} is available. You are running ${v.current}.`;
  if (!v.enabled) return "Not looking for newer versions.";
  if (v.err) return "The last check did not complete.";
  if (v.checked && !v.nothing) return `Up to date (${v.current}) when last checked.`;
  return `Running ${v.current}.`;
}

export function codingStatus(v: app.CodingAgentsView): string {
  const x = v.export;
  if (!x.enabled) return "Install puts QuickAgents in the runner home only.";
  return `Install also writes each QuickAgent as ${exportLabel(x.target)} for ${codingAgentName(v)}.`;
}

// The Run defaults card's line: the timeout and where runs go, the built-in
// agents' switch when it is on, and Docker when it is what stands in the way.
export function runDefaultsStatus(d: app.RunDefaultsView, s: app.SetupStatusView | null): string {
  const where = d.where === "agentry" ? "In Agentry where it is offered" : "on this computer";
  const builtins = d.builtinNoLimit ? " The built-in agents have no time or turn limit here." : "";
  const line = `Runs stop after ${fmtDuration(d.timeoutSeconds)}, ${where}.${builtins} ${chatLimitLine(d.chatIdleTimeoutSeconds)}${d.unsafeMode ? " Unsafe Mode is on." : ""}`;
  return s && !s.dockerOk && d.where !== "agentry" ? `${line} The container engine is not ready yet.` : line;
}

// historyLine says what the run history holds.
function historyLine(d: app.RunDefaultsView): string {
  if (!d.rememberRuns) return "Off: no run is remembered.";
  if (d.runHistoryCount < 0) return "On. The run history cannot be read; Clear run history starts it again.";
  if (d.runHistoryCount === 0) return "On. No run is remembered yet.";
  return `On. ${d.runHistoryCount} run${d.runHistoryCount === 1 ? " is" : "s are"} remembered.`;
}

// A chat on this computer has no limit but the Chat idle timeout, none by
// default.
export function chatLimitLine(idleSeconds: number): string {
  return idleSeconds > 0
    ? `A chat on this computer ends after ${fmtDuration(idleSeconds)} without a message from you.`
    : "A chat on this computer ends when you end it or quit Agentry Desktop.";
}

// The longest Chat idle timeout Go accepts, a week, in the minutes the
// field takes (settings.CheckChatIdleTimeout).
const MAX_CHAT_IDLE_MINUTES = 7 * 24 * 60;

// The field's text for a saved Chat idle timeout: whole minutes, or empty
// for none (and for a value Go would refuse, edited into desktop.yaml).
const idleField = (seconds: number) => (seconds > 0 && seconds % 60 === 0 ? String(seconds / 60) : "");

export function runStatus(s: app.SetupStatusView): string {
  if (!s.dockerOk) return "No container engine answers, so QuickAgents cannot run on this computer yet.";
  const engine = s.engine?.summary ? `${s.engine.summary}.` : "The container engine answers.";
  if (!s.imagesReady) return `${engine} The run images are fetched on the first run.`;
  return `${engine} The run images are ready.`;
}

// The key's line says where it comes from and never what it is:
// "saved" only when the keychain holds it and the policy reads it there.
export function providerStatus(s: app.SetupStatusView | null, k: app.JevKeyView | null): string {
  const parts: string[] = [];
  if (s) parts.push(s.policyExists && s.provider === "agentry" ? "Paid with your Agentry credits"
    : s.policyExists && s.model ? `${providerLabel(s.provider)}, ${s.model}${paidBy(s.apiKeyKind ?? "") ? `, paid by ${paidBy(s.apiKeyKind ?? "")}` : ""}` : "No provider chosen yet");
  if (k) {
    const standing = keyStanding(k);
    parts.push(standing === "env" ? `AI Review key from $${k.envName}` : standing === "saved" ? "AI Review key saved" : "No AI Review key");
  }
  return parts.join(" · ") + ".";
}

// RunDefaultsSettings is Settings → Run defaults: the timeout and max turns the
// runner policy holds, which the CLI runs with too; where the run setup starts;
// the output folder it fills in; and the container engine's prerequisites. None of
// them is a consent: every run still asks what it asks. Unsafe Mode's switch is
// here too: the page asks Go to turn it on, and Go asks the person in a native
// dialog and pulls the image before it reads on. The built-in agents' switch
// (on by default) sits with the timeout and turns it lifts, for drafts, builds,
// reviews and voices on this computer.
function RunDefaultsSettings({ onChanged }: { onChanged: () => Promise<unknown> }) {
  const [setup, setSetup] = useState<app.SetupStatusView | null>(null);
  const [defaults, setDefaults] = useState<app.RunDefaultsView | null>(null);
  const [timeoutText, setTimeoutText] = useState("");
  const [turns, setTurns] = useState("");
  const [idleText, setIdleText] = useState("");
  const [idleSaved, setIdleSaved] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const read = () => RunAPI.SetupStatus().then(setSetup).catch((e) => setErr(String(e)));
  // The fields take the saved budgets on the first read and after a budgets
  // save only, so choosing where to run keeps a timeout typed and not saved;
  // the Chat idle timeout's field likewise, on the first read and after its
  // own save.
  const show = (v: app.RunDefaultsView, budgets: boolean) => {
    setDefaults(v);
    if (!budgets) return;
    setTimeoutText(String(v.timeoutSeconds));
    setTurns(String(v.maxTurns));
  };
  useEffect(() => {
    read();
    SettingsAPI.RunDefaults().then((v) => { show(v, true); setIdleText(idleField(v.chatIdleTimeoutSeconds)); }).catch((e) => setErr(String(e)));
  }, []);

  // act returns Run defaults as Go now holds them, or null when it refused.
  async function act(fn: () => Promise<app.RunDefaultsView>, said?: string): Promise<app.RunDefaultsView | null> {
    setBusy(true);
    setErr(null);
    setSaved(null);
    try {
      const v = await fn();
      show(v, !!said);
      if (said) setSaved(said);
      return v;
    } catch (e) {
      setErr(String(e));
      return null;
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
  // The Chat idle timeout in whole minutes, a week at most; Go keeps seconds.
  const idleMinutes = parseInt(idleText, 10);
  const idleValid = /^\d+$/.test(idleText.trim()) && idleMinutes >= 1 && idleMinutes <= MAX_CHAT_IDLE_MINUTES;
  async function setChatIdle(seconds: number) {
    setIdleSaved(null);
    const v = await act(() => SettingsAPI.SetChatIdleTimeout(seconds));
    if (!v) return;
    setIdleText(idleField(v.chatIdleTimeoutSeconds));
    setIdleSaved(seconds > 0 ? "Saved. The next chat uses it." : "Saved. The next chat has no idle timeout.");
  }

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
                runs with them too. A run&apos;s own fields under Advanced apply to that run only. The built-in agents have no
                limit here while the switch below is on; with it off, a draft made on this computer has at least 30 minutes
                and a knowledge build at least 60, however short the timeout here.
              </p>
              {saved && <p className="small" role="status" style={{ marginBottom: 0 }}>{saved}</p>}
              <label className="check-row" style={{ margin: "12px 0 0" }}>
                <input type="checkbox" role="switch" checked={defaults.builtinNoLimit} disabled={busy} data-builtin-no-limit
                  onChange={(e) => void act(() => SettingsAPI.SetBuiltinNoLimit(e.target.checked))} />
                <span><strong>No time or turn limit for the built-in agents</strong></span>
              </label>
              <p className="muted small" style={{ marginBottom: 0 }}>
                On unless you turn it off. While it is on, drafts, knowledge builds, local reviews and Writing Voice runs on
                this computer have no timeout and no turn limit: each runs until it finishes or you stop it, and only a
                safety cap far past what any of them needs ends one that loops. The runner policy&apos;s cost cap still
                applies, unless it is 0. A draft&apos;s repair turn has no timeout too, with at most eight tool calls and
                only the remaining turns and cost. Turned off, they keep the max turns above and their own floors; a
                timed draft&apos;s repair gets at most five minutes within its remaining time. Your QuickAgents keep the limits above, and nothing
                changes for the <span className="mono">agentry</span> CLI or for anything run In Agentry. It applies from
                the next one you start.
              </p>
            </Card>
          </PartSection>
          <PartSection id="chats" title="Chat idle timeout">
            <Card>
              <div className="row">
                <label className="row" style={{ gap: 6 }}>
                  <span className="small">Minutes without a message from you</span>
                  <Input type="number" min={1} max={MAX_CHAT_IDLE_MINUTES} style={{ width: 120 }} value={idleText} disabled={busy}
                    placeholder="none" data-chat-idle
                    onChange={(e) => { setIdleText(e.target.value); setIdleSaved(null); }} />
                </label>
                <Button variant="primary" disabled={busy || !idleValid || idleMinutes * 60 === defaults.chatIdleTimeoutSeconds}
                  title={!idleValid ? "1 minute to a week" : ""}
                  onClick={() => void setChatIdle(idleMinutes * 60)}>
                  Save
                </Button>
                {defaults.chatIdleTimeoutSeconds > 0 && (
                  <Button disabled={busy} onClick={() => void setChatIdle(0)}>No timeout</Button>
                )}
              </div>
              <p className="small" style={{ margin: "8px 0 0" }} data-chat-idle-state>
                {defaults.chatIdleTimeoutSeconds === 0 && "None. "}{chatLimitLine(defaults.chatIdleTimeoutSeconds)}
              </p>
              <p className="muted small" style={{ marginBottom: 0 }}>
                A chat here has no other time limit and no limit on your messages. The max turns above still limits the
                QuickAgent&apos;s tool calls, unless it is 0. This is kept by Agentry Desktop, so chats with the <span className="mono">agentry</span> CLI
                keep the runner policy&apos;s limits, and a chat In Agentry keeps Agentry&apos;s idle close.
              </p>
              {idleSaved && <p className="small" role="status" style={{ marginBottom: 0 }}>{idleSaved}</p>}
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
          <PartSection id="deliverables" title="Deliverables">
            <Card>
              <label className="check-row" style={{ margin: 0 }}>
                <input type="checkbox" role="switch" checked={defaults.deliverables} disabled={busy} data-deliverables-switch
                  onChange={(e) => void act(() => SettingsAPI.SetDeliverables(e.target.checked))} />
                <span><strong>Copy each run&apos;s deliverables to a folder</strong></span>
              </label>
              <p className="small" data-deliverables-state>{deliverablesLine(defaults)}</p>
              <div className="row">
                <span className="grow mono selectable" data-deliverables-folder>
                  {defaults.deliverablesDir || <span className="muted">No folder: choose one.</span>}
                </span>
                <Button disabled={busy} data-choose-deliverables onClick={() => act(SettingsAPI.ChooseDeliverablesDir)}>
                  {defaults.deliverablesDefault ? "Choose…" : "Choose another…"}
                </Button>
                {!defaults.deliverablesDefault && (
                  <Button disabled={busy} data-default-deliverables onClick={() => act(SettingsAPI.UseDefaultDeliverablesDir)}>Use the default</Button>
                )}
              </div>
              <p className="muted small" style={{ marginBottom: 0 }}>
                A deliverable is a file the run was for: a report, a document, a table. When a run ends, Agentry Desktop copies
                its deliverables into a folder named for the QuickAgent and the time, inside this one, and Home&apos;s
                Deliverables card and the run&apos;s result list them first. The run&apos;s own folder keeps everything: Its
                receipts note what was copied, and nothing else in it changes. The folder is chosen here, through the file dialog; until you do, it is an Agentry
                folder inside your Documents folder. On Windows and macOS a copy is marked as a download, so
                the system treats it with the care it gives a file from the internet; Linux has no such mark. It applies from the next run; a run In Agentry is copied when its output comes down.
              </p>
            </Card>
          </PartSection>
          <PartSection id="history" title="Recent runs">
            <Card>
              <label className="check-row" style={{ margin: 0 }}>
                <input type="checkbox" role="switch" checked={defaults.rememberRuns} disabled={busy} data-remember-runs
                  onChange={(e) => void act(() => SettingsAPI.SetRememberRuns(e.target.checked))} />
                <span><strong>Remember recent runs</strong></span>
              </label>
              <p className="small" data-run-history-state>{historyLine(defaults)}</p>
              <p className="muted small">
                Agentry Desktop keeps the last 50 runs on this computer, drafts, reviews and knowledge and voice builds
                included, so Home&apos;s Recent runs can list them and open each one&apos;s result again. For each it keeps
                the QuickAgent, when it ran, how it ended and its output folder, never the task, your inputs, the result or
                the log. Turning this off forgets them all and keeps nothing from then on. Neither removes a run&apos;s folder.
              </p>
              {defaults.rememberRuns && defaults.runHistoryCount !== 0 && (
                <Button disabled={busy} data-clear-run-history onClick={() => act(SettingsAPI.ClearRunHistory)}>Clear run history</Button>
              )}
            </Card>
          </PartSection>
          <PartSection id="malware" title="Malware check">
            <Card>
              <label className="check-row" style={{ margin: 0 }}>
                <input type="checkbox" role="switch" checked={defaults.malwareCheck} disabled={busy} data-malware-check
                  onChange={(e) => void act(() => SettingsAPI.SetMalwareCheck(e.target.checked))} />
                <span><strong>Check every file a run is given for malware</strong></span>
              </label>
              <p className="muted small" style={{ marginBottom: 0 }} data-malware-check-state>
                {defaults.malwareCheck
                  ? "On. Every file you give a run, and a repository's programs, PDFs and Office files, is checked with ClamAV, and with Hybrid Analysis when you have saved its key, as In Agentry."
                  : "Off. A run checks only the programs among its files, and only when the QuickAgent runs commands that could start them."}
              </p>
              <p className="muted small" style={{ marginBottom: 0 }}>
                Either way, every file a run is given has no permission to execute, and malware stops a run only when the
                QuickAgent runs commands. A QuickAgent that runs none, such as one that reviews a codebase, is given the
                flagged file unopened, and the receipt names it. Runs In Agentry are always checked. This is the runner
                policy&apos;s, so the <span className="mono">agentry</span> CLI checks the same way, and it applies from
                the next run.
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
                left out, and the run goes on without it. A declared MCP server the registry&apos;s gate rates high or
                critical for a known vulnerability can run too, only in Unsafe Mode and only with your consent for each run.
              </p>
              <p className="muted small">
                <span className="mono">run_command</span> still takes only the commands a QuickAgent declares. The
                container&apos;s isolation, its network limits and every other consent stay as they are, and a run In Agentry never
                uses Unsafe Mode. Turning it on asks you to confirm, then downloads the Unsafe Mode image, a few hundred
                megabytes. While it is on, a red edge runs round the window and the sidebar says so. Turning it off applies
                from the next run or chat and keeps the images (Docker images, below, removes them). One already running
                keeps Unsafe Mode, with the red edge, until it ends.
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
      <PartSection id="docker" title="Container engine">
        {setup && (setup.dockerOk
          ? <EngineCard setup={setup} onChanged={read} />
          : <PrereqCard onDone={async () => { await read(); }} />)}
      </PartSection>
      <PartSection id="images" title="Container images"><DockerImagesCard /></PartSection>
    </>
  );
}

// EngineCard is the container engine found, with Re-detect: the detection
// run again from nothing, for a person who changed engines, and what it
// found adopted (with an engine chosen, that engine asked again). The traps detection saw stand beside the line.
// Choose an engine lists every engine that answers, for a computer with
// more than one, and the one chosen is the engine runs drive from then on.
function EngineCard({ setup, onChanged }: { setup: app.SetupStatusView; onChanged: () => Promise<unknown> }) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [found, setFound] = useState<app.EngineView | null>(null);
  const [choices, setChoices] = useState<app.EngineChoicesView | null>(null);
  const [chose, setChose] = useState<app.EngineView | null>(null);
  async function listEngines() {
    setBusy(true);
    setErr(null);
    setFound(null);
    try {
      setChoices(await RunAPI.ListEngines());
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }
  // Use this, or Use automatic: remembered and adopted at once; a choice
  // that does not answer is refused in Go's words and the engine stays.
  async function choose(choice: string) {
    setBusy(true);
    setErr(null);
    setFound(null);
    setChose(null);
    try {
      setChose(await RunAPI.SetEngineChoice(choice));
    } catch (e) {
      setErr(String(e));
      setBusy(false);
      return;
    }
    // The choice is made; a listing or a status read that fails afterwards
    // is said as itself.
    try {
      setChoices(await RunAPI.ListEngines());
      await onChanged();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }
  async function redetect() {
    setBusy(true);
    setErr(null);
    setFound(null);
    setChose(null);
    try {
      setFound(await RunAPI.RedetectEngine());
      await onChanged();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }
  // Remove Agentry's engine: the app's own engine stopped and deleted with
  // its data; the card then shows what detection finds, if anything.
  async function removeManaged() {
    setBusy(true);
    setErr(null);
    setFound(null);
    try {
      await RunAPI.RemoveManagedEngine();
      await onChanged();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Card>
      <p className="small" style={{ margin: 0 }} data-engine-line>{runStatus(setup)}</p>
      <EngineWarnings warnings={setup.engine?.warnings} />
      {found && !found.found && <Notice tone="warn">Re-detect found no engine: {found.err}</Notice>}
      {found && found.found && <p className="small" role="status">Re-detect found {found.summary}.</p>}
      {chose && chose.found && <p className="small" role="status">Runs now drive {chose.summary}.</p>}
      {choices && <EnginePicker choices={choices} busy={busy} onChoose={(c) => void choose(c)} />}
      <ErrorBox error={err} />
      <div className="row" style={{ justifyContent: "flex-end", marginTop: 8 }}>
        {!busy && setup.engine?.managed && (
          <Button variant="danger" onClick={() => void removeManaged()} title="Stops the virtual machine and deletes it with its data and the three binaries; runs then need another engine" data-testid="remove-managed-engine">Remove Agentry&apos;s engine</Button>
        )}
        {!busy && !choices && (
          <Button onClick={() => void listEngines()} title="Lists every container engine that answers on this computer, to choose the one runs drive" data-testid="choose-engine">Choose an engine…</Button>
        )}
        {busy ? <Spinner label={CHECKING_ENGINE} /> : <Button onClick={() => void redetect()} title={setup.engine?.chosen ? "Asks the engine you chose again, and adopts it if it answers" : "Runs the whole detection again, from nothing, and adopts what it finds"}>Re-detect</Button>}
      </div>
    </Card>
  );
}

// engineFacts is what answers at a place, without how it was found, which
// the row's own label says.
function engineFacts(e: app.EngineView): string {
  const who = e.cli && e.cli !== e.kind ? `${e.kind} through the ${e.cli} CLI` : e.kind;
  return `${who} ${e.version}, ${e.rootless ? "rootless" : "rootful"}, at ${e.endpoint}`;
}

// EnginePicker lists Automatic and every engine that answers, the one in
// use and the one chosen marked, with Use this on the others. An app
// started with its engine named in its environment drives that one and
// lists nothing.
function EnginePicker({ choices, busy, onChoose }: { choices: app.EngineChoicesView; busy: boolean; onChoose: (choice: string) => void }) {
  if (choices.locked) return <Notice tone="info">{asSentence(choices.locked)}</Notice>;
  const row = { borderTop: "1px solid var(--border)", padding: "8px 0", gap: 12, alignItems: "center" } as const;
  return (
    <div data-engine-picker style={{ marginTop: 10 }}>
      <p className="muted small" style={{ marginBottom: 4 }}>
        The engines that answer on this computer. Runs drive the one chosen here, and the app asks it alone from then on:
        if it stops answering, the app says so rather than switching to another. Automatic takes the first that answers,
        in the app&apos;s order.
      </p>
      <div className="row" style={row} data-engine-choice="">
        <span className="grow small">
          <strong>Automatic</strong> <span className="muted">the first engine that answers</span>
          {choices.choice === "" && <> <span className="badge">your choice</span></>}
        </span>
        {choices.choice !== "" && <Button disabled={busy} onClick={() => onChoose("")} data-testid="engine-use-automatic">Use automatic</Button>}
      </div>
      {choices.candidates.map((c) => (
        <div key={c.choice} className="row" style={row} data-engine-choice={c.choice}>
          <span className="grow small">
            <strong>{c.label.charAt(0).toUpperCase() + c.label.slice(1)}</strong>
            {c.current && <> <span className="badge">in use</span></>}
            {c.choice === choices.choice && <> <span className="badge">your choice</span></>}
            <br />
            <span className="muted">{engineFacts(c.engine)}</span>
          </span>
          {c.choice !== choices.choice && <Button disabled={busy} onClick={() => onChoose(c.choice)}>Use this</Button>}
        </div>
      ))}
      {choices.candidates.length === 0 && <p className="small">No container engine answers on this computer right now.</p>}
      {choices.tried.length > 0 && (
        <details className="small">
          <summary>Where the app looked</summary>
          <ul className="muted small">{choices.tried.map((t) => <li key={t}>{t}</li>)}</ul>
        </details>
      )}
    </div>
  );
}

// RegistrySettings is Settings → Registry settings: the registries with
// their sign-in, account and organizations lines, the account's secrets,
// and Add a registry — the cards that opened the one Settings page before.
function RegistrySettings({ settings, activeRegistry, onChanged, onSelectRegistry, onOpenProfile }: {
  settings: app.SettingsView;
  activeRegistry: string;
  onChanged: () => Promise<app.SettingsView>;
  onSelectRegistry: (url: string) => void;
  onOpenProfile: () => void;
}) {
  const [newURL, setNewURL] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [signIn, setSignIn] = useState<{ url: string; token: string; kind: SignInKind } | null>(null);
  const [busy, setBusy] = useState(false);
  // The registry a browser sign-in is waiting on; shows the Cancel control.
  const [browserWait, setBrowserWait] = useState<string | null>(null);
  // The account behind each stored credential, for its "signed in as" line; the
  // tier, the credits and the organizations are the Profile card's. Read after
  // the registries render, so a slow registry never holds the page; a read that
  // fails leaves the card as it was.
  const [accounts, setAccounts] = useState<Record<string, app.UserView>>({});
  useEffect(() => {
    let cancelled = false;
    const signedIn = settings.registries.filter((r) => r.hasToken);
    Promise.all(signedIn.map(async (r) => [r.url, await SettingsAPI.Whoami(r.url).catch(() => null)] as const)).then((pairs) => {
      if (cancelled) return;
      const next: Record<string, app.UserView> = {};
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
        {twoSignIns(settings)} Credentials live in the OS keychain under the same convention
        the <span className="mono">agentry</span> CLI uses, so a sign-in here works there too.
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
                <span className={r.signIn ? "badge kind" : "badge warn"}>{signInLabel(r.signIn as SignInKind)}</span>
                {r.isDefault && <span className="badge kind">default</span>}
                {r.url === activeRegistry && <span className="badge ok">active</span>}
                {r.signIn && (r.hasToken
                  ? <span className="badge">signed in ({r.tokenSource})</span>
                  : <span className="badge warn">anonymous</span>)}
              </div>
              {/* A registry the app keeps no sign-in for says why, and
                  offers only Remove. */}
              {r.signInRefused && (
                <div className="muted small" style={{ marginTop: 4 }}>{asSentence(r.signInRefused)}</div>
              )}
              {/* Who the credential is; the account and the subscription
                  are the Profile card's. */}
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
              ) : r.signIn && (
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
                  <Button disabled={busy} onClick={() => setSignIn({ url: r.url, token: "", kind: r.signIn as SignInKind })}>
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
          registry keeps them. */}
      {activeRegistry && <SecretsCard registry={activeRegistry} />}

      <section id="settings-part-add-registry" className="settings-part">
      <h2>Add a registry</h2>
      <p className="muted small">
        {settings.selfHostedRegistry
          ? <>Your self-hosted registry is <span className="mono">{settings.selfHostedRegistry}</span>. Remove it above to add another.</>
          : "Add your self-hosted registry here to sign in to it beside SecureAgentry."}
      </p>
      <Toolbar>
        <Input
          type="text"
          className="grow mono"
          placeholder="https://agentry.example.com — or http://localhost:8080 for a dev stack"
          value={newURL}
          onChange={(e) => setNewURL(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && newURL.trim() && addRegistry()}
        />
        <Button variant="primary" disabled={busy || !newURL.trim()} onClick={addRegistry}>Add</Button>
      </Toolbar>
      </section>

      {signIn && (
        <Modal title={`${signInLabel(signIn.kind)}: ${signIn.url}`} onClose={() => setSignIn(null)}>
            <p className="muted">{signInLine(settings, signIn.url)}</p>
            <p className="muted">
              Paste a personal access token. Create one on the registry's site under Settings →
              Authentication, or with <span className="mono">agentry login</span>. Signing in also pins the
              registry's signing keys on first use (TOFU).
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

// WhatsNewButton opens What's new again: the newest versions' highlights up
// to the one running. Closing it records nothing.
function WhatsNewButton() {
  const [data, setData] = useState<WhatsNewData | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const open = () => {
    setErr(null);
    WhatsNewAPI.Recent()
      .then((v) => (v.cards.length > 0 ? setData(v) : setErr("This build carries no highlights to show.")))
      .catch((e) => setErr(String(e)));
  };
  return (
    <>
      <Button onClick={open}>What's new</Button>
      {err && <span className="muted small">{err}</span>}
      {data && <WhatsNew data={data} mode="settings" onClose={() => setData(null)} onOpenReleases={(url) => OpenExternal(url).catch(() => {})} />}
    </>
  );
}

// SelfUpdateCard shows whether a newer Agentry Desktop exists and lets the
// person stop the app looking.
//
// The app applies an update itself only on Windows and macOS, from a copy
// the person can write to; on Linux it stays the package manager's job — so
// there the card says where the release is and leaves the choice where it
// already was.
function SelfUpdateCard() {
  const [view, setView] = useState<app.SelfUpdateView | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    SelfUpdateAPI.SelfUpdateStatus().then(setView).catch(() => setView(null));
  }, []);

  const run = async (fn: () => Promise<app.SelfUpdateView>) => {
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
        <Toolbar><WhatsNewButton /></Toolbar>
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
              where the package manager owns the installed files, and not
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
        <WhatsNewButton />
      </Toolbar>
    </Card>
  );
}
