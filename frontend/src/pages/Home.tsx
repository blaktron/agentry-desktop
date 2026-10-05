import { useEffect, useState, type ReactNode } from "react";
import * as HomeAPI from "../../wailsjs/go/main/HomeAPI";
import * as InstallAPI from "../../wailsjs/go/main/InstallAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import { install, main } from "../../wailsjs/go/models";
import { Notice, PageHeader } from "../ui";
import { cardsToDraw, type HomeCardKind, type LayoutCard } from "../homeCards";
import { greeting } from "../homeLines";
import { signedInTo } from "./Settings";
import { InstalledCard } from "./home/InstalledCard";
import { ProfileCard } from "./home/ProfileCard";
import { RunDefaultsCard } from "./home/RunDefaultsCard";
import { RecentRunsCard } from "./home/RecentRunsCard";

// Home (plan desktop-home-and-library.md §4.3, D2, D4, D9, D-067;
// agentry-desktop#658; Recent runs, D-068, agentry-desktop#659): the page the app opens on, a card per thing that
// is the person's own, in the start pages' card language. Go keeps which
// cards and in what order (HomeAPI); each card reads its own bindings and a
// read that fails says so in its card, never the page's. Home starts
// nothing: no run, install, update or spend happens on load, and every
// card's action opens the page that does it.

// The default cards' places when the layout cannot be read at all.
const FALLBACK: LayoutCard[] = [{ kind: "recent-runs" }, { kind: "installed" }, { kind: "profile" }, { kind: "run-defaults" }];

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
  run: (row: main.RecentRunView) => void;
  runHistory: () => void;
};

// How many rows Recent runs shows (plan §4.3).
const RECENT_ROWS = 5;

export default function Home({ registry, settings, open, banner }: {
  registry: string;
  settings: main.SettingsView | null;
  open: HomeOpen;
  // The "Setup isn't finished" notice, when App has one to show.
  banner?: ReactNode;
}) {
  const [layout, setLayout] = useState<Got<main.HomeLayoutView>>(pending);
  const [records, setRecords] = useState<Got<install.Item[]>>(pending);
  const [updates, setUpdates] = useState(0);
  const [profile, setProfile] = useState<Got<main.ProfileView>>(pending);
  const [defaults, setDefaults] = useState<Got<main.RunDefaultsView>>(pending);
  const [recent, setRecent] = useState<Got<main.RecentRunsView>>(pending);
  // null until the settings are read.
  const signedIn = settings ? signedInTo(settings, registry) : null;

  useEffect(() => {
    let live = true;
    const put = settle(() => live);
    HomeAPI.Layout().then(...put(setLayout));
    InstallAPI.ListInstalled().then(...put(setRecords));
    // The launch check's snapshot: reading it never starts a check.
    InstallAPI.UpdatesStatus().then((v) => live && setUpdates(v.available)).catch(() => {});
    SettingsAPI.RunDefaults().then(...put(setDefaults));
    return () => { live = false; };
  }, []);

  // Recent runs: this computer's history and, signed in, the account's runs
  // In Agentry; read again when the registry or the sign-in changes. It
  // reads, and starts nothing.
  useEffect(() => {
    if (signedIn === null) return;
    let live = true;
    RunAPI.RecentRuns(registry, RECENT_ROWS).then(...settle(() => live)(setRecent));
    return () => { live = false; };
  }, [registry, signedIn]);

  useEffect(() => {
    if (!signedIn) { setProfile(pending); return; }
    let live = true;
    SettingsAPI.Profile(registry).then(...settle(() => live)(setProfile));
    return () => { live = false; };
  }, [registry, signedIn]);

  const renderers: Partial<Record<HomeCardKind, () => ReactNode>> = {
    "recent-runs": () => (
      <RecentRunsCard view={recent.value} error={recent.error} now={new Date()} onOpenRun={open.run} onOpenSettings={open.runHistory} />
    ),
    installed: () => (
      <InstalledCard records={records.value} updates={updates} error={records.error}
        onOpenLibrary={open.library} onOpenBrowse={open.browse} onOpenAgent={open.agent} />
    ),
    profile: () => (
      <ProfileCard registry={registry} signedIn={signedIn} profile={profile.value} error={profile.error}
        onOpenProfile={open.profile} onSignIn={open.signIn}
        onTopUp={() => { SettingsAPI.OpenTopUp(registry).catch(() => {}); }} />
    ),
    "run-defaults": () => <RunDefaultsCard defaults={defaults.value} error={defaults.error} onOpen={open.runDefaults} />,
  };
  const cards = layout.value || layout.error
    ? cardsToDraw(layout.value?.cards ?? FALLBACK, new Set(Object.keys(renderers)))
    : [];

  return (
    <div>
      <PageHeader title="Home" sub={greeting(profile.value?.user ?? null) ?? "Your recent runs, what is installed on this computer, your account and how your runs are set up."} />
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
      <div className="build-ways home-cards">
        {cards.map((c) => <div key={c.key} className="home-card-cell" data-home-card={c.kind}>{renderers[c.kind]?.()}</div>)}
      </div>
    </div>
  );
}
