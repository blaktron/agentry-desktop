import { useEffect, useMemo, useState, type ReactNode } from "react";
import * as HomeAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/homeapi";
import * as InstallAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/installapi";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import * as ScheduleAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/scheduleapi";
import * as SettingsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/settingsapi";
import * as install from "../../bindings/github.com/blaktron/agentry-desktop/internal/install/models";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { EventsOn } from "../host";
import { Button, Notice, PageHeader } from "../ui";
import { asStart, cardsToDraw, scheduledOnHome, startLayout, type DrawnCard, type HomeCardKind, type ShortcutKind } from "../homeCards";
import * as KnowledgeAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/knowledgeapi";
import * as RunhostsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runhostsapi";
import * as knowledgeModels from "../../bindings/github.com/blaktron/agentry-desktop/internal/knowledge/models";
import { getStartedRows, type GetStartedAction } from "../getStarted";
import { keyStanding } from "../keyRecommendation";
import { GetStartedCard } from "./home/GetStartedCard";
import { KnowledgeCard } from "./home/KnowledgeCard";
import { LearnCard } from "./home/LearnCard";
import { RunhostCard } from "./home/RunhostCard";
import { ShortcutCard } from "./home/ShortcutCard";
import type { KnowledgePlace } from "./Knowledge";
import { greeting } from "../homeLines";
import { signedInTo } from "./Settings";
import { signInState } from "../signInState";
import { InstalledCard } from "./home/InstalledCard";
import { ProfileCard } from "./home/ProfileCard";
import { RunDefaultsCard } from "./home/RunDefaultsCard";
import { RecentRunsCard } from "./home/RecentRunsCard";
import { PinnedCard } from "./home/PinnedCard";
import { DeliverablesCard } from "./home/DeliverablesCard";
import { ScheduledCard } from "./home/ScheduledCard";
import { MeetingsCard } from "./home/MeetingsCard";
import * as MeetingAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/meetingapi";
import { AgentCard } from "./home/AgentCard";
import { useAgents, useHomeAgent } from "../agents";
import { homeAgentDrawn } from "../homeAgent";
import { MAIN_AGENT } from "../persistentAgents";
import { HomeGrid } from "./home/HomeGrid";
import { gridRange, monthGrid, monthTitle } from "../scheduled";
import RunPanel from "./RunPanel";
import { localAttestation } from "./Library";
import type { Pin } from "../pins";

// Home: the page the app opens on, a card per thing that is the person's
// own, in the start pages' card language. Go keeps which
// cards and in what order (HomeAPI); each card reads its own bindings and a
// read that fails says so in its card, never the page's. Home starts
// nothing: no run, install, update or spend happens on load, and every
// card's action opens the page that does it. Main's card reads Main's
// state and its last lines; its containers start on the first message sent
// to it.

// The cards' places when the layout cannot be read at all: the start for
// runs on this computer, the one with every shortcut.
const FALLBACK = startLayout("local");

type Got<T> = { value: T | null; error: string | null };
const pending = <T,>(): Got<T> => ({ value: null, error: null });
// settle(live)(set) is a read's two callbacks, which put its answer or its
// error while the page still wants it.
const settle = (live: () => boolean) => <T,>(set: (g: Got<T>) => void) => [
  (v: T) => { if (live()) set({ value: v, error: null }); },
  (e: unknown) => { if (live()) set({ value: null, error: String(e) }); },
] as const;

export type HomeOpen = {
  library: () => void;
  browse: () => void;
  agent: (registry: string, ns: string, name: string) => void;
  profile: () => void;
  signIn: () => void;
  runDefaults: () => void;
  // Recent runs: a row's run page, and Run defaults' Recent runs part.
  run: (row: app.RecentRunView) => void;
  runHistory: () => void;
  // Deliverables: Run defaults' Deliverables part.
  deliverables: () => void;
  // A pin's Run for a QuickAgent from a registry: its page, with the run
  // setup open (a folder install's opens here, as the Library's does).
  runAgent: (registry: string, ns: string, name: string) => void;
  // Settings → Home: which cards, the pins, Reset.
  settings: () => void;
  // The Scheduled card's page.
  scheduled: () => void;
  // The Meeting notes card's page.
  meetings: () => void;
  // The agent card's title: Settings → Persistent Agents for Agentry Helper, any
  // other agent's own page, and Settings → Provider where no model is set
  // up.
  persistentAgents: () => void;
  persistentAgent: (name: string) => void;
  provider: () => void;
  // The shortcuts and Get started: Build's Describe it, the pack creator,
  // the Knowledge page, Writing Voice, the setup wizard, Settings →
  // Runhosts, and the AI Review key in Settings → Provider.
  draft: () => void;
  createKnowledge: (place: KnowledgePlace) => void;
  knowledge: () => void;
  voice: () => void;
  setup: () => void;
  runhosts: () => void;
  reviewKey: () => void;
};

// How many files the Deliverables card lists.
const DELIVERABLE_ROWS = 12;
// How many rows Recent runs shows.
const RECENT_ROWS = 5;
// How many recent runs are read: enough to find each pin's latest run
// (the history keeps 50).
const RUNS_READ = 50;

export default function Home({ registry, settings, open, banner, now: nowProp }: {
  registry: string;
  settings: app.SettingsView | null;
  open: HomeOpen;
  // The "Setup isn't finished" notice, when App has one to show.
  banner?: ReactNode;
  // Harness-only: a fixed now, for the Scheduled card's month.
  now?: Date;
}) {
  const now = useMemo(() => nowProp ?? new Date(), [nowProp]);
  // The Scheduled card's month: this one's grid.
  const weeks = useMemo(() => monthGrid(now.getFullYear(), now.getMonth(), now), [now]);
  const [layout, setLayout] = useState<Got<app.HomeLayoutView>>(pending);
  const [records, setRecords] = useState<Got<install.Item[]>>(pending);
  // The launch check's snapshot: each record's answer for the Pinned
  // cards, and the count of updates this app can apply, which the
  // Installed card says (Go counts only rows it manages).
  const [updates, setUpdates] = useState<install.UpdateInfo[]>([]);
  const [updateCount, setUpdateCount] = useState(0);
  // A folder install's Run, from its pin: the run setup, here.
  const [runningLocal, setRunningLocal] = useState<install.Item | null>(null);
  const [profile, setProfile] = useState<Got<app.ProfileView>>(pending);
  const [defaults, setDefaults] = useState<Got<app.RunDefaultsView>>(pending);
  const [recent, setRecent] = useState<Got<app.RecentRunsView>>(pending);
  const [deliverables, setDeliverables] = useState<Got<app.RecentDeliverablesView>>(pending);
  const [scheduled, setScheduled] = useState<Got<app.ScheduledCardView>>(pending);
  // Whether this copy records meetings: the Meeting notes card is drawn
  // only where it does.
  const [meetings, setMeetings] = useState(false);
  useEffect(() => {
    let live = true;
    MeetingAPI.Status().then((v) => { if (live) setMeetings(v.available); }).catch(() => undefined);
    return () => { live = false; };
  }, []);
  // null until the settings are read.
  const signedIn = settings ? signedInTo(settings, registry) : null;
  // The agent the agent card shows, or null for no card.
  const agents = useAgents();
  const chosenAgent = useHomeAgent();
  const agentDrawn = homeAgentDrawn(chosenAgent ?? MAIN_AGENT, agents);

  // Read again when an install or a run Agentry Helper asked for ends.
  const [changes, setChanges] = useState(0);
  // Each card's Try again: a count per read, bumped to read it again.
  const [tries, setTries] = useState<Record<string, number>>({});
  const retry = (key: string) => () => setTries((t) => ({ ...t, [key]: (t[key] ?? 0) + 1 }));
  useEffect(() => {
    // The harness has no runtime.
    try { return EventsOn("applink:home-changed", () => setChanges((n) => n + 1)); } catch { return undefined; }
  }, []);

  useEffect(() => {
    let live = true;
    InstallAPI.ListInstalled().then(...settle(() => live)(setRecords));
    return () => { live = false; };
  }, [changes, tries.records]);

  useEffect(() => {
    let live = true;
    const put = settle(() => live);
    HomeAPI.Layout().then(...put(setLayout));
    // The launch check's snapshot: reading it never starts a check.
    InstallAPI.UpdatesStatus().then((v) => { if (live) { setUpdates(v.updates ?? []); setUpdateCount(v.available); } }).catch(() => {});
    return () => { live = false; };
  }, []);

  useEffect(() => {
    let live = true;
    SettingsAPI.RunDefaults().then(...settle(() => live)(setDefaults));
    return () => { live = false; };
  }, [tries.defaults]);

  // Recent runs: this computer's history and, signed in, the account's runs
  // In Agentry; read again when the registry or the sign-in changes. It
  // reads, and starts nothing.
  useEffect(() => {
    if (signedIn === null) return;
    let live = true;
    RunAPI.RecentRuns(registry, RUNS_READ).then(...settle(() => live)(setRecent));
    return () => { live = false; };
  }, [registry, signedIn, changes, tries.recent]);

  // The Deliverables card: the runs this computer remembers and each one's
  // envelope, read when Home opens and only while the layout holds the card.
  // It reads, and starts nothing.
  const wantsDeliverables = !layout.value || layout.value.newer || layout.value.unreadable
    || layout.value.cards.some((c) => c.kind === "deliverables" && !c.hidden);
  useEffect(() => {
    if (!wantsDeliverables) return;
    let live = true;
    RunAPI.RecentDeliverables(DELIVERABLE_ROWS).then(...settle(() => live)(setDeliverables));
    return () => { live = false; };
  }, [wantsDeliverables, changes, tries.deliverables]);

  useEffect(() => {
    if (!signedIn) { setProfile(pending); return; }
    let live = true;
    SettingsAPI.Profile(registry).then(...settle(() => live)(setProfile));
    return () => { live = false; };
  }, [registry, signedIn, tries.profile]);

  // The Scheduled card's read, only while the layout holds the card: Home
  // reads the CLI's store and, signed in, the account's schedules In
  // Agentry, and runs nothing.
  const wantsScheduled = !!layout.value && !layout.value.newer && !layout.value.unreadable && scheduledOnHome(layout.value.cards);
  // Read again when a schedule changes or a Run now ends while Home is open.
  useEffect(() => {
    if (!wantsScheduled) return;
    let live = true;
    const { from, to } = gridRange(weeks);
    const read = () => { ScheduleAPI.HomeCard(from, to, registry).then(...settle(() => live)(setScheduled)); };
    read();
    let off: (() => void) | undefined;
    // The harness has no runtime.
    try { off = EventsOn("schedules:changed", read); } catch { off = undefined; }
    return () => { live = false; off?.(); };
  }, [wantsScheduled, weeks, registry, tries.scheduled]);

  // What the layout holds decides what Home reads for the new cards: each
  // read is made only while a card that shows it is on Home.
  const held = layout.value?.cards ?? (layout.error ? FALLBACK : []);
  const holds = (...kinds: HomeCardKind[]) => held.some((c) => kinds.includes(c.kind as HomeCardKind) && !c.hidden);
  const start = asStart(layout.value?.start);
  const wantsSetup = holds("get-started") && start === "local";
  const wantsRunhost = holds("runhost") || (holds("get-started") && start === "remote");
  const wantsPacks = holds("knowledge", "get-started");

  // Get started on this computer: the runtime and model (the runner policy
  // and Docker, read only) and where the AI Review key comes from.
  const [setup, setSetup] = useState<Got<app.SetupStatusView>>(pending);
  const [reviewKey, setReviewKey] = useState<Got<app.JevKeyView>>(pending);
  useEffect(() => {
    if (!wantsSetup) return;
    let live = true;
    const put = settle(() => live);
    RunAPI.SetupStatus().then(...put(setSetup));
    RunAPI.JevKeyStatus().then(...put(setReviewKey));
    return () => { live = false; };
  }, [wantsSetup, tries.setup]);

  // The Runhost card and Remote's Get started: the runhosts this computer
  // keeps, then one status call to the one runs go to.
  const [runhosts, setRunhosts] = useState<Got<app.RunhostView[]>>(pending);
  const [runhostStatus, setRunhostStatus] = useState<Got<app.RunhostStatusView>>(pending);
  useEffect(() => {
    if (!wantsRunhost) return;
    let live = true;
    RunhostsAPI.List().then(...settle(() => live)(setRunhosts));
    return () => { live = false; };
  }, [wantsRunhost, tries.runhost]);
  const where = defaults.value?.where ?? "";
  const named = where.startsWith("runhost:") ? where.slice("runhost:".length) : "";
  const runhost = runhosts.value
    ? (named ? runhosts.value.find((r) => r.name === named) : runhosts.value.find((r) => r.default)) ?? null
    : null;
  useEffect(() => {
    if (!runhost) return;
    let live = true;
    setRunhostStatus(pending);
    RunhostsAPI.Status(runhost.name).then(...settle(() => live)(setRunhostStatus));
    return () => { live = false; };
  }, [runhost?.name, tries.runhost]);

  // The Knowledge packs card and Get started's pack row.
  const [packs, setPacks] = useState<Got<knowledgeModels.List>>(pending);
  useEffect(() => {
    if (!wantsPacks) return;
    let live = true;
    KnowledgeAPI.List(registry).then(...settle(() => live)(setPacks));
    return () => { live = false; };
  }, [wantsPacks, registry, changes, tries.packs]);

  const credits = profile.value?.subscription ? profile.value.subscription.balance : null;
  const getStarted = getStartedRows({
    start,
    signedIn,
    setup: setup.value,
    reviewKey: reviewKey.value ? keyStanding(reviewKey.value) : null,
    runhost: runhosts.value ? (runhost ? { name: runhost.name, known: true, reachable: runhostStatus.value ? runhostStatus.value.reachable : runhostStatus.error ? false : null, signIns: runhost.signIns.map((x) => x.vendor) } : "none") : null,
    credits: signedIn ? credits : null,
    ranOnce: recent.value ? recent.value.runs.length > 0 : null,
    packs: packs.value ? packs.value.packs.length : null,
  });
  // Run a QuickAgent: the Library holds what is installed; with nothing
  // installed, Browse is where to find one.
  const openRun = () => ((records.value ?? []).some((r) => r.present) ? open.library() : open.browse());
  const createKnowledge = () => open.createKnowledge(start === "cloud" ? "agentry" : "local");
  const act: Record<GetStartedAction, () => void> = {
    provider: open.provider,
    reviewKey: open.reviewKey,
    runhosts: open.runhosts,
    signIn: open.signIn,
    topUp: () => { SettingsAPI.OpenTopUp(registry).catch(() => {}); },
    run: openRun,
    knowledge: createKnowledge,
    draft: open.draft,
  };
  const shortcutOpen: Record<ShortcutKind, () => void> = {
    "shortcut-run": openRun,
    "shortcut-draft": open.draft,
    "shortcut-knowledge": createKnowledge,
    "shortcut-meetings": open.meetings,
    "shortcut-voice": open.voice,
    "shortcut-agent": open.persistentAgents,
    "shortcut-setup": open.setup,
  };
  const shortcuts = Object.fromEntries(
    (Object.keys(shortcutOpen) as ShortcutKind[])
      .filter((k) => k !== "shortcut-meetings" || meetings)
      .map((k) => [k, () => <ShortcutCard kind={k} onOpen={shortcutOpen[k]} />]),
  ) as Partial<Record<HomeCardKind, (c: DrawnCard) => ReactNode>>;

  // A move or a resize on the grid: one change Go makes, answered with the
  // layout as it now is, or refused, which the page says above the grid.
  const [arrangeError, setArrangeError] = useState<string | null>(null);
  const [arranging, setArranging] = useState(false);
  const arrange = async (change: () => Promise<app.HomeLayoutView>) => {
    setArranging(true);
    setArrangeError(null);
    try {
      setLayout({ value: await change(), error: null });
    } catch (e) {
      setArrangeError(String(e));
    } finally {
      setArranging(false);
    }
  };
  // A layout this app keeps rather than uses (newer, unreadable), or one it
  // could not read, draws the defaults and is not arranged here.
  const arrangeable = !!layout.value && !layout.value.newer && !layout.value.unreadable;

  // A pin whose QuickAgent is gone from this computer is unpinned from its
  // card; Go answers with the layout as it now is.
  const unpin = (pin: Pin) => {
    HomeAPI.Unpin(pin.ref, pin.registry ?? "").then((v) => setLayout({ value: v, error: null })).catch(() => {});
  };
  // A plain object, not createFrom.
  const recentView = recent.value ? ({ ...recent.value, runs: recent.value.runs.slice(0, RECENT_ROWS) } as app.RecentRunsView) : null;
  const renderers: Partial<Record<HomeCardKind, (c: DrawnCard) => ReactNode>> = {
    // The agent card is drawn while an agent it can show is on.
    ...(agentDrawn ? { main: () => <AgentCard onOpenSettings={open.persistentAgents} onOpenAgent={open.persistentAgent} onSetUpModel={open.provider} /> } : {}),
    deliverables: () => (
      <DeliverablesCard view={deliverables.value} error={deliverables.error} onRetry={retry("deliverables")} now={new Date()} onOpenSettings={open.deliverables} />
    ),
    "recent-runs": () => (
      <RecentRunsCard view={recentView} error={recent.error} onRetry={retry("recent")} now={new Date()} onOpenRun={open.run} onOpenSettings={open.runHistory} />
    ),
    installed: () => (
      <InstalledCard records={records.value} updates={updateCount} error={records.error} onRetry={retry("records")}
        onOpenLibrary={open.library} onOpenBrowse={open.browse} onOpenAgent={open.agent} />
    ),
    profile: () => (
      <ProfileCard registry={registry} signedIn={signedIn} rejected={!!settings && signInState(settings.registries, registry) === "rejected"}
        profile={profile.value} error={profile.error} onRetry={retry("profile")}
        onOpenProfile={open.profile} onSignIn={open.signIn}
        onTopUp={() => { SettingsAPI.OpenTopUp(registry).catch(() => {}); }} />
    ),
    "run-defaults": () => <RunDefaultsCard defaults={defaults.value} error={defaults.error} onRetry={retry("defaults")} onOpen={open.runDefaults} />,
    pinned: (c) => (
      <PinnedCard pins={c.pins} records={records.value ?? (records.error ? [] : null)} updates={updates}
        runs={recent.value?.runs ?? (recent.error ? [] : null)} now={new Date()}
        onOpenLibrary={open.library} onOpenAgent={open.agent} onOpenRun={open.run} onUnpin={unpin}
        onRun={(rec) => (rec.local ? setRunningLocal(rec) : open.runAgent(rec.origin, rec.namespace, rec.name))} />
    ),
    scheduled: () => <ScheduledCard view={scheduled.value} error={scheduled.error} onRetry={retry("scheduled")} weeks={weeks} title={monthTitle(now.getFullYear(), now.getMonth())} onOpen={open.scheduled} />,
    ...(meetings ? { meetings: () => <MeetingsCard onOpen={open.meetings} /> } : {}),
    "get-started": () => <GetStartedCard rows={getStarted} onAction={(a) => act[a]()} onOpen={open.setup} />,
    runhost: () => (
      <RunhostCard runhost={runhost} named={runhost ? "" : named} status={runhostStatus.value} error={runhosts.error}
        statusError={runhostStatus.error} onRetry={retry("runhost")} onOpen={open.runhosts} />
    ),
    knowledge: () => <KnowledgeCard list={packs.value} error={packs.error} onRetry={retry("packs")} onOpen={open.knowledge} onCreate={createKnowledge} />,
    learn: () => <LearnCard />,
    ...shortcuts,
  };
  const cards = layout.value || layout.error
    ? cardsToDraw(layout.value?.cards ?? FALLBACK, new Set(Object.keys(renderers)))
    : [];

  return (
    <div>
      <PageHeader title="Home" sub={greeting(profile.value?.user ?? null) ?? "Where to start, and what your runs have made."}
        actions={<Button variant="link" data-home-settings onClick={open.settings}>Home settings</Button>} />
      {banner}
      {layout.error && <Notice tone="warn">Home's layout could not be read, so the starting cards are shown: {layout.error}</Notice>}
      {layout.value?.unreadable && (
        <Notice tone="warn">
          The settings file's Home arrangement cannot be read by this Agentry Desktop, so the starting cards are shown and
          the file is left as it is.
        </Notice>
      )}
      {layout.value?.newer && (
        <Notice tone="info">
          Home was arranged by a newer Agentry Desktop. This one shows the starting cards and leaves that arrangement as it is.
        </Notice>
      )}
      {layout.value && cards.length === 0 && (
        <p className="muted" data-home-none>
          Every card is hidden. <Button variant="link" onClick={open.settings}>Home settings</Button> shows them again.
        </p>
      )}
      {arrangeError && <Notice tone="warn">Home's cards could not be arranged: {arrangeError}</Notice>}
      <HomeGrid cards={cards} render={(c) => renderers[c.kind]?.(c)} busy={arranging || !arrangeable}
        onMove={(from, kind, to) => arrange(() => HomeAPI.Move(from, kind, to))}
        onResize={(index, kind, size) => arrange(() => HomeAPI.Resize(index, kind, size.cols, size.rows))} />
      {runningLocal && (
        <RunPanel registry={registry} ns={runningLocal.namespace} name={runningLocal.name} version={runningLocal.version}
          attestation={localAttestation} local onClose={() => setRunningLocal(null)} />
      )}
    </div>
  );
}
