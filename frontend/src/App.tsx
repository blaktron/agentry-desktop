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
import Validate from "./pages/Validate";
import Build from "./pages/Build";
import Settings from "./pages/Settings";
import Wizard, { WizardOutcome } from "./pages/Wizard";
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
  | { page: "settings" };

export default function App() {
  const [route, setRoute] = useState<Route>({ page: "browse" });
  const [settings, setSettings] = useState<main.SettingsView | null>(null);
  const [registry, setRegistry] = useState<string>("");
  const [version, setVersion] = useState<string>("");
  // A newer Agentry Desktop, when the last check found one (#87). Reading the
  // status never starts a check: the app looks on launch and daily in the
  // background, and this only renders what it found.
  const [newer, setNewer] = useState<main.SelfUpdateView | null>(null);
  // How many installed BriefAgents have a newer version, from the launch
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
    };
    read();
    const t = setInterval(read, 60_000);
    return () => clearInterval(t);
  }, []);

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
  const navItem = (label: string, page: Route["page"], count = 0) => (
    <button
      className={`nav-item ${route.page === page || (page === "browse" && route.page === "agent") ? "active" : ""}`}
      onClick={() => nav({ page } as Route)}
    >
      {label}
      {count > 0 && <span className="badge warn nav-count" title={`${count} installed BriefAgent${count === 1 ? " has" : "s have"} a newer version`}>{count}</span>}
    </button>
  );

  return (
    <div className="shell">
      <div className="sidebar">
        <div className="brand">
          <Wordmark sub="Desktop" />
        </div>
        {navItem("Browse", "browse")}
        {navItem("Installed", "installed", installedUpdates)}
        {navItem("Knowledge", "knowledge")}
        {navItem("Build", "build")}
        {navItem("Validate", "validate")}
        {navItem("Settings", "settings")}
        <div className="foot">
          {version && <div>v{version}</div>}
          {newer?.available && !newer.dismissed && (
            <div className="update-line">
              <button
                type="button"
                onClick={() => setRoute({ page: "settings" })}
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
        {route.page === "browse" && setupMissing && (
          <div className="setup-banner">
            <Notice tone="warn">
              <strong>Setup isn't finished.</strong> Agentry Desktop can browse a public registry, but
              it cannot sign in, run or build anything until it is:
              <ul className="setup-banner-list">
                {setupMissing.map((m) => <li key={m}>{m}</li>)}
              </ul>
              <div className="row">
                <Button variant="primary" onClick={() => nav({ page: "settings" })}>Open Settings</Button>
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
            onBrowse={() => nav({ page: "browse" })}
            onOpenAgent={(reg, ns, name) => nav({ page: "agent", ns, name, registry: reg, back: "installed" })}
          />
        )}
        {route.page === "knowledge" && <Knowledge registry={registry} onOpenSettings={() => nav({ page: "settings" })} />}
        {route.page === "build" && (
          <Build registry={registry} onOpenAgent={(ns, name) => nav({ page: "agent", ns, name })} onOpenSettings={() => nav({ page: "settings" })} />
        )}
        {route.page === "validate" && <Validate />}
        {route.page === "settings" && (
          <Settings
            settings={settings}
            activeRegistry={registry}
            onChanged={refreshSettings}
            onSelectRegistry={setRegistry}
            onRunSetup={() => setShowWizard(true)}
          />
        )}
      </div>
      {showWizard && settings && (
        <Wizard
          settings={settings}
          activeRegistry={registry}
          onChanged={refreshSettings}
          onSelectRegistry={setRegistry}
          onFinish={finishWizard}
        />
      )}
    </div>
  );
}
