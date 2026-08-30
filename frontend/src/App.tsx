import { useCallback, useEffect, useState } from "react";
import * as SettingsAPI from "../wailsjs/go/main/SettingsAPI";
import { AppVersion } from "../wailsjs/go/main/App";
import { main } from "../wailsjs/go/models";
import Browse from "./pages/Browse";
import AgentPage from "./pages/AgentPage";
import Installed from "./pages/Installed";
import Validate from "./pages/Validate";
import Settings from "./pages/Settings";

export type Route =
  | { page: "browse" }
  | { page: "agent"; ns: string; name: string }
  | { page: "installed" }
  | { page: "validate" }
  | { page: "settings" };

export default function App() {
  const [route, setRoute] = useState<Route>({ page: "browse" });
  const [settings, setSettings] = useState<main.SettingsView | null>(null);
  const [registry, setRegistry] = useState<string>("");
  const [version, setVersion] = useState<string>("");

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
    refreshSettings().catch(() => setSettings(null));
    AppVersion().then(setVersion).catch(() => {});
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
          SecureAgentry
          <span className="sub">Desktop</span>
        </div>
        {navItem("Browse", "browse")}
        {navItem("Installed", "installed")}
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
            registry={registry}
            ns={route.ns}
            name={route.name}
            onBack={() => nav({ page: "browse" })}
          />
        )}
        {route.page === "installed" && <Installed onBrowse={() => nav({ page: "browse" })} />}
        {route.page === "validate" && <Validate />}
        {route.page === "settings" && (
          <Settings
            settings={settings}
            activeRegistry={registry}
            onChanged={refreshSettings}
            onSelectRegistry={setRegistry}
          />
        )}
      </div>
    </div>
  );
}
