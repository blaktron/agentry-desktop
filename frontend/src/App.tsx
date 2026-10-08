import { useCallback, useEffect, useRef, useState } from "react";
import * as SettingsAPI from "../wailsjs/go/main/SettingsAPI";
import { AppVersion } from "../wailsjs/go/main/App";
import { DismissSelfUpdate, SelfUpdateStatus } from "../wailsjs/go/main/SelfUpdateAPI";
import { UpdatesStatus } from "../wailsjs/go/main/InstallAPI";
import { UnsafeLive } from "../wailsjs/go/main/RunAPI";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { main } from "../wailsjs/go/models";
import Browse from "./pages/Browse";
import Home from "./pages/Home";
import AgentPage from "./pages/AgentPage";
import Library from "./pages/Library";
import Publisher, { type FollowFork } from "./pages/Publisher";
import Knowledge, { type KnowledgePlace } from "./pages/Knowledge";
import Voice from "./pages/Voice";
import KnowledgeCreate from "./pages/KnowledgeCreate";
import DraftFlow from "./pages/DraftFlow";
import { stageRecordedDraft } from "./drafts";
import RunFlow from "./pages/RunFlow";
import ReviewFlow from "./pages/ReviewFlow";
import HostedRunFlow from "./pages/HostedRunFlow";
import ChatFlow from "./pages/ChatFlow";
import RunPanel from "./pages/RunPanel";
import RunPage, { type RunPageSource } from "./pages/RunPage";
import { type RunSetup, attachHostedRun } from "./runs";
import { CornerHost } from "./progress";
import { attachHosted } from "./drafts";
import Validate from "./pages/Validate";
import Build from "./pages/Build";
import Settings, { SETTINGS_FOR, SettingsPart, SettingsSection } from "./pages/Settings";
import Wizard, { WizardOutcome } from "./pages/Wizard";
import { Recover } from "./Recover";
import { SidebarNav } from "./nav";
import { UnsafeBadge, UnsafeGlow } from "./unsafe";
import { Button, Notice, Wordmark } from "./ui";
import { requestEdit, sameRegistry } from "./edit";

export type Route =
  // The page the app opens on: a card per thing that is the person's own
  // (plan desktop-home-and-library.md D2; agentry-desktop#658).
  | { page: "home" }
  // The Library: what is installed on this computer, with Browse beneath it
  // (plan desktop-home-and-library.md D3).
  | { page: "library" }
  // q: the Library's search, carried to Browse and searched there.
  | { page: "browse"; q?: string }
  // registry: an installed record's own registry may differ from the active
  // one; back: where the ← link returns to.
  // run: open the page with its run setup open (a pin's Run on Home,
  // agentry-desktop#660).
  | { page: "agent"; ns: string; name: string; registry?: string; back?: "browse" | "library" | "home" | "publisher"; run?: boolean }
  // A run Recent runs lists, as a page (plan desktop-home-and-library.md
  // §4.7, D7; agentry-desktop#659): read by its id, from the history Go
  // keeps (local) or the registry (hosted).
  | { page: "run"; source: RunPageSource; id: string; registry?: string; back?: "home" }
  | { page: "knowledge" }
  | { page: "voice" }
  | { page: "validate" }
  | { page: "build" }
  // The QuickAgents the signed-in person publishes (plan
  // forks-and-publisher.md D12; agentry-desktop#705).
  // follow: a fork just made from an agent page, which the section opens on
  // and follows (agentry-desktop#706).
  | { page: "publisher"; follow?: FollowFork }
  // section: one of Settings' five section pages, or the landing's cards
  // when absent; part: the anchor within it (settings-cards.md D12).
  | { page: "settings"; section?: SettingsSection; part?: SettingsPart };

// The agent page's way back, by where it was opened from.
const BACK_LABEL = { browse: "← back to Browse", library: "← back to Library", home: "← back to Home", publisher: "← back to Publisher" } as const;

// The one Create knowledge or Add documents flow, above the pages so a build
// goes on in its corner card while the person uses the rest of the app
// (agentry-desktop#353). One at a time, as the run lock allows one build.
type KnowledgeFlow = { registry: string; addTo?: string; place?: KnowledgePlace };

export default function App() {
  const [route, setRoute] = useState<Route>({ page: "home" });
  const [knowledgeFlow, setKnowledgeFlow] = useState<KnowledgeFlow | null>(null);
  const [flowMinimized, setFlowMinimized] = useState(false);
  const [flowInFlight, setFlowInFlight] = useState(false);
  // Bumped when a build ends (and when its flow closes), so the Knowledge
  // list is read again.
  const [knowledgeRev, setKnowledgeRev] = useState(0);
  // Bumped by the draft's "Open the draft", so the Build tab opens the
  // draft that landed whatever view it is on (agentry-desktop#410).
  const [openDraftRev, setOpenDraftRev] = useState(0);
  // Bumped by the QuickAgent page's Edit, so the Build tab opens the
  // release it asked for (edit.ts; agentry-desktop#500).
  const [openEditRev, setOpenEditRev] = useState(0);
  // The corner the running flows' cards stack in (progress.tsx).
  const [corner, setCorner] = useState<HTMLElement | null>(null);
  // A run on this computer's setup, opened again by Back to the setup from
  // the run's flow (agentry-desktop#465), with the answers as they were.
  const [reopened, setReopened] = useState<RunSetup | null>(null);
  const [previewRequest, setPreviewRequest] = useState<{ registry: string; name: string } | null>(null);
  const clearPreviewRequest = useCallback(() => setPreviewRequest(null), []);
  const [settings, setSettings] = useState<main.SettingsView | null>(null);
  const [registry, setRegistry] = useState<string>("");
  const [version, setVersion] = useState<string>("");
  // A newer Agentry Desktop, when the last check found one (#87). Reading the
  // status never starts a check: the app looks on launch and daily in the
  // background, and this only renders what it found.
  const [newer, setNewer] = useState<main.SelfUpdateView | null>(null);
  // How many installed QuickAgents have a newer version, from the launch
  // check and the agent page's own checks (agentry-desktop#147, D8): the
  // number on Library. Reading it never starts a check.
  const [installedUpdates, setBriefUpdates] = useState(0);
  const [showWizard, setShowWizard] = useState(false);
  // The "nothing set up" reminder (plan desktop-installers.md D10): raised
  // when the wizard closed with "I'll do it later", shown on Home until
  // dismissed (opening Settings from it leaves it standing for the return
  // trip), and never persisted — the next launch starts clean, and D-019's
  // flag keeps the wizard closed.
  const [setupMissing, setSetupMissing] = useState<string[] | null>(null);
  // A run or chat compiled in Unsafe Mode that is still live on this computer
  // (agentry-desktop#479): turning the switch off changes the next run, not
  // this one, so the edge and the badge stay until Go says it has ended.
  // The event is the newer word: a poll's answer that set out before the
  // latest event arrived is dropped, not allowed to undo it.
  const [unsafeLive, setUnsafeLive] = useState(false);
  const unsafeEvents = useRef(0);
  useEffect(() => EventsOn("runner:unsafe-live", (live: boolean) => {
    unsafeEvents.current++;
    setUnsafeLive(live);
  }), []);

  const refreshSettings = useCallback(async () => {
    const s = await SettingsAPI.Get();
    setSettings(s);
    setRegistry((cur) => {
      if (cur && s.registries.some((r) => r.url === cur)) return cur;
      return s.defaultRegistry || (s.registries[0]?.url ?? "");
    });
    return s;
  }, []);

  // A draft In Agentry already running for this account is followed from
  // the start, in its corner card (agentry-desktop#410).
  useEffect(() => {
    if (registry) attachHosted(registry);
  }, [registry]);
  // So is a run or a chat In Agentry, in its own card (agentry-desktop#466,
  // #467).
  useEffect(() => {
    if (registry) void attachHostedRun(registry);
  }, [registry]);

  useEffect(() => {
    refreshSettings()
      .then((s) => {
        // First-run gate: open the guided setup when the machine is
        // unconfigured — not yet onboarded and nothing signed in (#41).
        if (!s.onboarded && !s.registries.some((r) => r.hasToken)) setShowWizard(true);
      })
      .catch(() => setSettings(null));
    AppVersion().then(setVersion).catch(() => {});
  }, [refreshSettings]);

  // The background check writes its result as it finishes, so the chrome asks
  // again on a slow cadence rather than once: a line that only ever appeared
  // on the next launch would be a line most people never saw.
  useEffect(() => {
    const read = () => {
      SelfUpdateStatus().then(setNewer).catch(() => {});
      UpdatesStatus().then((v) => setBriefUpdates(v.available)).catch(() => {});
      // The settings file too: the red edge and the badge follow the Unsafe
      // Mode switch Go reads, which another window of this app can change.
      refreshSettings().catch(() => {});
      const seen = unsafeEvents.current;
      UnsafeLive().then((live) => seen === unsafeEvents.current && setUnsafeLive(live)).catch(() => {});
    };
    read();
    const t = setInterval(read, 60_000);
    return () => clearInterval(t);
  }, [refreshSettings]);

  // Closing the wizard (finished or skipped) marks onboarding done so it
  // does not reopen on the next start; Settings can reopen it on demand.
  const finishWizard = useCallback(async (outcome: WizardOutcome) => {
    try {
      await SettingsAPI.MarkOnboarded();
    } catch {
      // Non-fatal: the flag is a convenience, not a gate on functionality.
    }
    await refreshSettings().catch(() => {});
    setShowWizard(false);
    setSetupMissing(outcome.kind === "later" ? outcome.missing : null);
  }, [refreshSettings]);

  const nav = (r: Route) => setRoute(r);
  // Home shows it; Browse showed it while Browse was the landing.
  const setupBanner = setupMissing && (
    <div className="setup-banner">
      <Notice tone="warn">
        <strong>Setup isn't finished.</strong> Agentry Desktop can browse a public registry, but
        it cannot sign in, run or build anything until it is:
        <ul className="setup-banner-list">
          {setupMissing.map((m) => <li key={m}>{m}</li>)}
        </ul>
        <div className="row">
          <Button variant="primary" onClick={() => nav({ page: "settings", ...SETTINGS_FOR.setupNotice })}>Open Settings</Button>
          <Button variant="ghost" onClick={() => setSetupMissing(null)}>Dismiss</Button>
        </div>
      </Notice>
    </div>
  );
  const unsafeShown = !!settings?.unsafeMode || unsafeLive;
  // The active registry holds a token: the sidebar shows Publisher.
  const signedIn = !!settings?.registries.some((r) => r.url === registry && r.hasToken);

  return (
    <CornerHost.Provider value={corner}>
      <div className="shell">
        <div className="sidebar">
          <div className="brand">
            <Wordmark sub="Desktop" />
          </div>
          <SidebarNav page={route.page} onNav={(page) => nav({ page } as Route)} installedUpdates={installedUpdates} signedIn={signedIn} />
          <div className="foot">
            {/* Unsafe Mode in words, beside the red edge below (D-050). */}
            {unsafeShown && (
              <UnsafeBadge running={!settings?.unsafeMode} onOpen={() => setRoute({ page: "settings", ...SETTINGS_FOR.unsafeMode })} />
            )}
            {version && <div>v{version}</div>}
            {newer?.available && !newer.dismissed && (
              <div className="update-line">
                <button
                  type="button"
                  onClick={() => setRoute({ page: "settings", ...SETTINGS_FOR.selfUpdate })}
                  title={
                    newer.available.retired
                      ? `Agentry Desktop ${newer.available.version} is signed with a key that has since been retired: genuine but out of date, and it will not be installed — open Settings`
                      : `Agentry Desktop ${newer.available.version} is available — open Settings`
                  }
                >
                  {/* The caveat Settings gives, in the chrome too: a release
                      signed by a retired key is shown, never offered as an
                      update (agentry-desktop#124). */}
                  {newer.available.version} is available{newer.available.retired && " (retired key, not installed)"}
                </button>
                <button
                  type="button"
                  className="dismiss"
                  aria-label="Not now"
                  title="Not now — this comes back next time the app starts"
                  onClick={() => DismissSelfUpdate().then(setNewer).catch(() => {})}
                >
                  ×
                </button>
              </div>
            )}
            <div className="nowrap" title={registry}>{registry.replace(/^https?:\/\//, "")}</div>
          </div>
        </div>
        <div className="content">
          {/* A render fault shows its way back here, under a sidebar that
              still works (agentry-desktop#351). */}
          <Recover subject="This page" resetKey={JSON.stringify(route)}>
            {route.page === "home" && (
              <Home
                registry={registry}
                settings={settings}
                banner={setupBanner}
                open={{
                  library: () => nav({ page: "library" }),
                  browse: () => nav({ page: "browse" }),
                  agent: (reg, ns, name) => nav({ page: "agent", ns, name, registry: reg, back: "home" }),
                  profile: () => nav({ page: "settings", ...SETTINGS_FOR.homeProfile }),
                  signIn: () => nav({ page: "settings", ...SETTINGS_FOR.homeSignIn }),
                  runDefaults: () => nav({ page: "settings", ...SETTINGS_FOR.homeRunDefaults }),
                  run: (row) => nav({ page: "run", source: row.source === "hosted" ? "hosted" : "local", id: row.id, registry: row.registry, back: "home" }),
                  runHistory: () => nav({ page: "settings", ...SETTINGS_FOR.homeRunHistory }),
                  runAgent: (reg, ns, name) => nav({ page: "agent", ns, name, registry: reg, back: "home", run: true }),
                  settings: () => nav({ page: "settings", ...SETTINGS_FOR.homeSettings }),
                }}
              />
            )}
            {route.page === "run" && (
              <RunPage
                key={`${route.source}:${route.registry || registry}:${route.id}`}
                source={route.source}
                id={route.id}
                registry={route.registry || registry}
                backLabel={BACK_LABEL[route.back ?? "home"]}
                onBack={() => nav({ page: route.back ?? "home" })}
                onOpenPlace={(page) => nav({ page })}
                onOpenDraft={(draft) => {
                  stageRecordedDraft(draft);
                  setRegistry(route.registry || registry);
                  setOpenDraftRev((n) => n + 1);
                  nav({ page: "build" });
                }}
              />
            )}
            {route.page === "browse" && (
              // Browse takes its query only when it mounts, so the key gives
              // a new search from the Library a fresh Browse.
              <Browse key={route.q ?? ""} registry={registry} initialQuery={route.q} onOpenAgent={(ns, name) => nav({ page: "agent", ns, name })} />
            )}
            {route.page === "agent" && (
              <AgentPage
                registry={route.registry ?? registry}
                ns={route.ns}
                name={route.name}
                backLabel={BACK_LABEL[route.back ?? "browse"]}
                onBack={() => nav({ page: route.back ?? "browse" })}
                openRun={route.run}
                // Edit opens in the Build tab, which submits to the app's
                // registry: offered only for that registry's QuickAgents.
                onOpenReviewKey={() => nav({ page: "settings", ...SETTINGS_FOR.reviewKey })}
                // Fork goes into the app's registry, so it is offered there
                // only, and only while it is signed in (agentry-desktop#706).
                onFork={signedIn && sameRegistry(route.registry ?? registry, registry)
                  ? (fns, fname, sentence) => nav({ page: "publisher", follow: { namespace: fns, name: fname, sentence } })
                  : undefined}
                onOpenAgent={(ons, oname) => nav({ page: "agent", ns: ons, name: oname, registry: route.registry, back: route.back })}
                onEdit={!sameRegistry(route.registry ?? registry, registry) ? undefined : () => {
                  requestEdit({ registry: route.registry ?? registry, namespace: route.ns, name: route.name });
                  setOpenEditRev((n) => n + 1);
                  nav({ page: "build" });
                }}
              />
            )}
            {route.page === "library" && (
              <Library
                registry={registry}
                onBrowse={(q) => nav({ page: "browse", q })}
                onOpenAgent={(reg, ns, name) => nav({ page: "agent", ns, name, registry: reg, back: "library" })}
              />
            )}
            {route.page === "publisher" && (
              <Publisher
                key={route.follow ? `${route.follow.namespace}/${route.follow.name}` : "publisher"}
                registry={registry}
                follow={route.follow}
                onOpenAgent={(ns, name) => nav({ page: "agent", ns, name, back: "publisher" })}
                onEdit={(ns, name) => {
                  requestEdit({ registry, namespace: ns, name });
                  setOpenEditRev((n) => n + 1);
                  nav({ page: "build" });
                }}
                onOpenSettings={() => nav({ page: "settings", ...SETTINGS_FOR.publisherSignIn })}
              />
            )}
            {route.page === "voice" && (
              <Voice
                registry={registry}
                onOpenSettings={(which) =>
                  nav({ page: "settings", ...(which === "here" ? SETTINGS_FOR.voiceHere : which === "account" ? SETTINGS_FOR.voiceAccount : SETTINGS_FOR.knowledgeSignIn) })
                }
              />
            )}
            {route.page === "knowledge" && (
            <Knowledge
              registry={registry}
              onOpenSettings={() => nav({ page: "settings", ...SETTINGS_FOR.knowledgeSignIn })}
              onCreate={(addTo, place) => {
                // A build in flight is brought back rather than a second
                // started (one build at a time); a flow that is over, or not
                // yet started, gives way to the one asked for, preset to the
                // place whose page asked (plan knowledge-cards.md D5).
                if (!knowledgeFlow || !flowInFlight) setKnowledgeFlow({ registry, addTo, place });
                setFlowMinimized(false);
              }}
              rev={knowledgeRev}
              previewRequest={previewRequest}
              onPreviewRequestSeen={clearPreviewRequest}
            />
          )}
            {route.page === "build" && (
              <Build registry={registry} openDraftRev={openDraftRev} openEditRev={openEditRev} onOpenAgent={(ns, name) => nav({ page: "agent", ns, name })} onOpenSettings={() => nav({ page: "settings", ...SETTINGS_FOR.describeKey })} onOpenReviewKey={() => nav({ page: "settings", ...SETTINGS_FOR.reviewKey })} onOpenLibrary={() => nav({ page: "library" })} />
            )}
            {route.page === "validate" && <Validate registry={registry} onOpenSettings={() => nav({ page: "settings", ...SETTINGS_FOR.reviewKey })} />}
            {route.page === "settings" && (
              <Settings
                settings={settings}
                activeRegistry={registry}
                onChanged={refreshSettings}
                onSelectRegistry={setRegistry}
                onRunSetup={() => setShowWizard(true)}
                section={route.section}
                part={route.part}
                onOpen={(section, part) => nav({ page: "settings", section, part })}
              />
            )}
          </Recover>
        </div>
        <DraftFlow
          registry={registry}
          onOpenDraft={() => {
            setRoute({ page: "build" });
            setOpenDraftRev((n) => n + 1);
          }}
        />
        <RunFlow onBackToSetup={setReopened} />
        <HostedRunFlow onBackToSetup={setReopened} />
        <ChatFlow onBackToSetup={setReopened} />
        <ReviewFlow />
        {reopened && (
          <RunPanel
            {...reopened.panel}
            initialForm={reopened.form}
            onClose={() => setReopened(null)}
          />
        )}
        {knowledgeFlow && (
          <KnowledgeCreate
            registry={knowledgeFlow.registry}
            addTo={knowledgeFlow.addTo}
            place={knowledgeFlow.place}
            minimized={flowMinimized}
            onMinimize={() => setFlowMinimized(true)}
            onRestore={() => setFlowMinimized(false)}
            onInFlight={(v) => {
              setFlowInFlight(v);
              if (!v) setKnowledgeRev((n) => n + 1);
            }}
            onClose={() => {
              setKnowledgeFlow(null);
              setFlowMinimized(false);
              setFlowInFlight(false);
              setKnowledgeRev((n) => n + 1);
            }}
            onPreview={(name) => {
              setKnowledgeFlow(null);
              setFlowMinimized(false);
              setFlowInFlight(false);
              setKnowledgeRev((n) => n + 1);
              // The pack is the build's registry's, which the person may
              // have switched away from while it ran.
              setPreviewRequest({ registry: knowledgeFlow.registry, name });
              setRoute({ page: "knowledge" });
            }}
          />
        )}
        {showWizard && settings && (
          <Wizard
            settings={settings}
            activeRegistry={registry}
            onChanged={refreshSettings}
            onSelectRegistry={setRegistry}
            onFinish={finishWizard}
          />
        )}
        <div className="corner-cards" ref={setCorner} />
        {unsafeShown && <UnsafeGlow />}
      </div>
    </CornerHost.Provider>
  );
}
