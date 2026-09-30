import { useCallback, useEffect, useState } from "react";
import * as SettingsAPI from "../wailsjs/go/main/SettingsAPI";
import { AppVersion } from "../wailsjs/go/main/App";
import { DismissSelfUpdate, SelfUpdateStatus } from "../wailsjs/go/main/SelfUpdateAPI";
import { UpdatesStatus } from "../wailsjs/go/main/InstallAPI";
import { main } from "../wailsjs/go/models";
import Browse from "./pages/Browse";
import AgentPage from "./pages/AgentPage";
import Installed from "./pages/Installed";
import Knowledge from "./pages/Knowledge";
import KnowledgeCreate from "./pages/KnowledgeCreate";
import DraftFlow from "./pages/DraftFlow";
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

export type Route =
  | { page: "browse" }
  // registry: an installed record's own registry may differ from the active
  // one; back: where the ← link returns to.
  | { page: "agent"; ns: string; name: string; registry?: string; back?: "browse" | "installed" }
  | { page: "installed" }
  | { page: "knowledge" }
  | { page: "validate" }
  | { page: "build" }
  // section: one of Settings' five section pages, or the landing's cards
  // when absent; part: the anchor within it (settings-cards.md D12).
  | { page: "settings"; section?: SettingsSection; part?: SettingsPart };

// The one Create knowledge or Add documents flow, above the pages so a build
// goes on in its corner card while the person uses the rest of the app
// (agentry-desktop#353). One at a time, as the run lock allows one build.
type KnowledgeFlow = { registry: string; addTo?: string };

export default function App() {
  const [route, setRoute] = useState<Route>({ page: "browse" });
  const [knowledgeFlow, setKnowledgeFlow] = useState<KnowledgeFlow | null>(null);
  const [flowMinimized, setFlowMinimized] = useState(false);
  const [flowInFlight, setFlowInFlight] = useState(false);
  // Bumped when a build ends (and when its flow closes), so the Knowledge
  // list is read again.
  const [knowledgeRev, setKnowledgeRev] = useState(0);
  // Bumped by the draft's "Open the draft", so the Build tab opens the
  // draft that landed whatever view it is on (agentry-desktop#410).
  const [openDraftRev, setOpenDraftRev] = useState(0);
  // The corner the running flows' cards stack in (progress.tsx).
  const [corner, setCorner] = useState<HTMLElement | null>(null);
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
  // number on Installed. Reading it never starts a check.
  const [installedUpdates, setBriefUpdates] = useState(0);
  const [showWizard, setShowWizard] = useState(false);
  // The "nothing set up" reminder (plan desktop-installers.md D10): raised
  // when the wizard closed with "I'll do it later", shown on Browse until
  // dismissed (opening Settings from it leaves it standing for the return
  // trip), and never persisted — the next launch starts clean, and D-019's
  // flag keeps the wizard closed.
  const [setupMissing, setSetupMissing] = useState<string[] | null>(null);

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

  return (
    <CornerHost.Provider value={corner}>
      <div className="shell">
        <div className="sidebar">
          <div className="brand">
            <Wordmark sub="Desktop" />
          </div>
          <SidebarNav page={route.page} onNav={(page) => nav({ page } as Route)} installedUpdates={installedUpdates} />
          <div className="foot">
            {/* Unsafe Mode in words, beside the red edge below (D-050). */}
            {settings?.unsafeMode && <UnsafeBadge onOpen={() => setRoute({ page: "settings", ...SETTINGS_FOR.unsafeMode })} />}
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
            {route.page === "browse" && setupMissing && (
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
            )}
            {route.page === "browse" && (
              <Browse registry={registry} onOpenAgent={(ns, name) => nav({ page: "agent", ns, name })} />
            )}
            {route.page === "agent" && (
              <AgentPage
                registry={route.registry ?? registry}
                ns={route.ns}
                name={route.name}
                backLabel={route.back === "installed" ? "← back to installed" : "← back to search"}
                onBack={() => nav({ page: route.back ?? "browse" })}
              />
            )}
            {route.page === "installed" && (
              <Installed
                registry={registry}
                onBrowse={() => nav({ page: "browse" })}
                onOpenAgent={(reg, ns, name) => nav({ page: "agent", ns, name, registry: reg, back: "installed" })}
              />
            )}
            {route.page === "knowledge" && (
            <Knowledge
              registry={registry}
              onOpenSettings={() => nav({ page: "settings", ...SETTINGS_FOR.knowledgeSignIn })}
              onCreate={(addTo) => {
                // A build in flight is brought back rather than a second
                // started (one build at a time); a flow that is over, or not
                // yet started, gives way to the one asked for.
                if (!knowledgeFlow || !flowInFlight) setKnowledgeFlow({ registry, addTo });
                setFlowMinimized(false);
              }}
              rev={knowledgeRev}
              previewRequest={previewRequest}
              onPreviewRequestSeen={clearPreviewRequest}
            />
          )}
            {route.page === "build" && (
              <Build registry={registry} openDraftRev={openDraftRev} onOpenAgent={(ns, name) => nav({ page: "agent", ns, name })} onOpenSettings={() => nav({ page: "settings", ...SETTINGS_FOR.describeKey })} onOpenInstalled={() => nav({ page: "installed" })} />
            )}
            {route.page === "validate" && <Validate />}
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
        {knowledgeFlow && (
          <KnowledgeCreate
            registry={knowledgeFlow.registry}
            addTo={knowledgeFlow.addTo}
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
        {settings?.unsafeMode && <UnsafeGlow />}
      </div>
    </CornerHost.Provider>
  );
}
