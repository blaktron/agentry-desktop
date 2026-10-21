import { useEffect, useMemo, useState, type ReactNode } from "react";
import * as HomeAPI from "../../wailsjs/go/app/HomeAPI";
import * as InstallAPI from "../../wailsjs/go/app/InstallAPI";
import * as RunAPI from "../../wailsjs/go/app/RunAPI";
import * as ScheduleAPI from "../../wailsjs/go/app/ScheduleAPI";
import * as SettingsAPI from "../../wailsjs/go/app/SettingsAPI";
import { install, app } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { Button, Notice, PageHeader } from "../ui";
import { cardsToDraw, HOME_DEFAULTS, scheduledOnHome, type DrawnCard, type HomeCardKind, type LayoutCard } from "../homeCards";
import { greeting } from "../homeLines";
import { signedInTo } from "./Settings";
import { InstalledCard } from "./home/InstalledCard";
import { ProfileCard } from "./home/ProfileCard";
import { RunDefaultsCard } from "./home/RunDefaultsCard";
import { RecentRunsCard } from "./home/RecentRunsCard";
import { PinnedCard } from "./home/PinnedCard";
import { DeliverablesCard } from "./home/DeliverablesCard";
import { ScheduledCard } from "./home/ScheduledCard";
import { MainCard } from "./home/MainCard";
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

// The default cards' places when the layout cannot be read at all.
const FALLBACK: LayoutCard[] = HOME_DEFAULTS.map((kind) => ({ kind }));

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
  // Main's card: Settings → Persistent Agents, and Settings → Provider
  // where no model is set up.
  persistentAgents: () => void;
  provider: () => void;
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
  // null until the settings are read.
  const signedIn = settings ? signedInTo(settings, registry) : null;

  // Read again when an install or a run Agentry Helper asked for ends.
  const [changes, setChanges] = useState(0);
  useEffect(() => {
    // The harness has no runtime.
    try { return EventsOn("applink:home-changed", () => setChanges((n) => n + 1)); } catch { return undefined; }
  }, []);

  useEffect(() => {
    let live = true;
    InstallAPI.ListInstalled().then(...settle(() => live)(setRecords));
    return () => { live = false; };
  }, [changes]);

  useEffect(() => {
    let live = true;
    const put = settle(() => live);
    HomeAPI.Layout().then(...put(setLayout));
    // The launch check's snapshot: reading it never starts a check.
    InstallAPI.UpdatesStatus().then((v) => { if (live) { setUpdates(v.updates ?? []); setUpdateCount(v.available); } }).catch(() => {});
    SettingsAPI.RunDefaults().then(...put(setDefaults));
    return () => { live = false; };
  }, []);

  // Recent runs: this computer's history and, signed in, the account's runs
  // In Agentry; read again when the registry or the sign-in changes. It
  // reads, and starts nothing.
  useEffect(() => {
    if (signedIn === null) return;
    let live = true;
    RunAPI.RecentRuns(registry, RUNS_READ).then(...settle(() => live)(setRecent));
    return () => { live = false; };
  }, [registry, signedIn, changes]);

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
  }, [wantsDeliverables, changes]);

  useEffect(() => {
    if (!signedIn) { setProfile(pending); return; }
    let live = true;
    SettingsAPI.Profile(registry).then(...settle(() => live)(setProfile));
    return () => { live = false; };
  }, [registry, signedIn]);

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
  }, [wantsScheduled, weeks, registry]);

  // A pin whose QuickAgent is gone from this computer is unpinned from its
  // card; Go answers with the layout as it now is.
  const unpin = (pin: Pin) => {
    HomeAPI.Unpin(pin.ref, pin.registry ?? "").then((v) => setLayout({ value: v, error: null })).catch(() => {});
  };
  // A plain object, not createFrom.
  const recentView = recent.value ? ({ ...recent.value, runs: recent.value.runs.slice(0, RECENT_ROWS) } as app.RecentRunsView) : null;
  const renderers: Partial<Record<HomeCardKind, (c: DrawnCard) => ReactNode>> = {
    main: () => <MainCard onOpenSettings={open.persistentAgents} onSetUpModel={open.provider} />,
    deliverables: () => (
      <DeliverablesCard view={deliverables.value} error={deliverables.error} now={new Date()} onOpenSettings={open.deliverables} />
    ),
    "recent-runs": () => (
      <RecentRunsCard view={recentView} error={recent.error} now={new Date()} onOpenRun={open.run} onOpenSettings={open.runHistory} />
    ),
    installed: () => (
      <InstalledCard records={records.value} updates={updateCount} error={records.error}
        onOpenLibrary={open.library} onOpenBrowse={open.browse} onOpenAgent={open.agent} />
    ),
    profile: () => (
      <ProfileCard registry={registry} signedIn={signedIn} profile={profile.value} error={profile.error}
        onOpenProfile={open.profile} onSignIn={open.signIn}
        onTopUp={() => { SettingsAPI.OpenTopUp(registry).catch(() => {}); }} />
    ),
    "run-defaults": () => <RunDefaultsCard defaults={defaults.value} error={defaults.error} onOpen={open.runDefaults} />,
    pinned: (c) => (
      <PinnedCard pins={c.pins} records={records.value ?? (records.error ? [] : null)} updates={updates}
        runs={recent.value?.runs ?? (recent.error ? [] : null)} now={new Date()}
        onOpenLibrary={open.library} onOpenAgent={open.agent} onOpenRun={open.run} onUnpin={unpin}
        onRun={(rec) => (rec.local ? setRunningLocal(rec) : open.runAgent(rec.origin, rec.namespace, rec.name))} />
    ),
    scheduled: () => <ScheduledCard view={scheduled.value} error={scheduled.error} weeks={weeks} title={monthTitle(now.getFullYear(), now.getMonth())} onOpen={open.scheduled} />,
  };
  const cards = layout.value || layout.error
    ? cardsToDraw(layout.value?.cards ?? FALLBACK, new Set(Object.keys(renderers)))
    : [];

  return (
    <div>
      <PageHeader title="Home" sub={greeting(profile.value?.user ?? null) ?? "Your recent runs, what is installed on this computer, your account and how your runs are set up."}
        actions={<Button variant="link" data-home-settings onClick={open.settings}>Home settings</Button>} />
      {banner}
      {layout.error && <Notice tone="warn">Home's layout could not be read, so the default cards are shown: {layout.error}</Notice>}
      {layout.value?.unreadable && (
        <Notice tone="warn">
          The settings file's Home arrangement cannot be read by this Agentry Desktop, so the default cards are shown and
          the file is left as it is.
        </Notice>
      )}
      {layout.value?.newer && (
        <Notice tone="info">
          Home was arranged by a newer Agentry Desktop. This one shows the default cards and leaves that arrangement as it is.
        </Notice>
      )}
      {layout.value && cards.length === 0 && (
        <p className="muted" data-home-none>
          Every card is hidden. <Button variant="link" onClick={open.settings}>Home settings</Button> shows them again.
        </p>
      )}
      <div className="build-ways home-cards">
        {cards.map((c) => <div key={c.key} className="home-card-cell" data-home-card={c.kind}>{renderers[c.kind]?.(c)}</div>)}
      </div>
      {runningLocal && (
        <RunPanel registry={registry} ns={runningLocal.namespace} name={runningLocal.name} version={runningLocal.version}
          attestation={localAttestation} local onClose={() => setRunningLocal(null)} />
      )}
    </div>
  );
}
