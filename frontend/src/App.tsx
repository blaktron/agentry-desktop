import { useCallback, useEffect, useState } from "react";
import * as SettingsAPI from "../wailsjs/go/main/SettingsAPI";
import { AppVersion } from "../wailsjs/go/main/App";
import { main } from "../wailsjs/go/models";
import Browse from "./pages/Browse";
import AgentPage from "./pages/AgentPage";
import Installed from "./pages/Installed";
import Validate from "./pages/Validate";
import Build from "./pages/Build";
import Settings from "./pages/Settings";
import Wizard from "./pages/Wizard";
import { Wordmark } from "./ui";

export type Route =
  | { page: "browse" }
  // registry: an installed record's own registry may differ from the active
  // one; back: where the ← link returns to.
  | { page: "agent"; ns: string; name: string; registry?: string; back?: "browse" | "installed" }
  | { page: "installed" }
  | { page: "validate" }
  | { page: "build" }
  | { page: "settings" };

export default function App() {
  const [route, setRoute] = useState<Route>({ page: "browse" });
  const [settings, setSettings] = useState<main.SettingsView | null>(null);
  const [registry, setRegistry] = useState<string>("");
  const [version, setVersion] = useState<string>("");
  const [showWizard, setShowWizard] = useState(false);

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

  // Closing the wizard (finished or skipped) marks onboarding done so it
  // does not reopen on the next start; Settings can reopen it on demand.
  const finishWizard = useCallback(async () => {
    try {
      await SettingsAPI.MarkOnboarded();
    } catch {
      // Non-fatal: the flag is a convenience, not a gate on functionality.
    }
    await refreshSettings().catch(() => {});
    setShowWizard(false);
  }, [refreshSettings]);

  const nav = (r: Route) => setRoute(r);
  const navItem = (label: string, page: Route["page"]) => (
    <button
      className={`nav-item ${route.page === page || (page === "browse" && route.page === "agent") ? "active" : ""}`}
      onClick={() => nav({ page } as Route)}
    >
      {label}
    </button>
  );

  return (
    <div className="shell">
      <div className="sidebar">
        <div className="brand">
          <Wordmark sub="Desktop" />
        </div>
        {navItem("Browse", "browse")}
        {navItem("Installed", "installed")}
        {navItem("Build", "build")}
        {navItem("Validate", "validate")}
        {navItem("Settings", "settings")}
        <div className="foot">
          {version && <div>v{version}</div>}
          <div className="nowrap" title={registry}>{registry.replace(/^https?:\/\//, "")}</div>
        </div>
      </div>
      <div className="content">
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
        {route.page === "build" && <Build registry={registry} />}
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
