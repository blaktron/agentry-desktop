// The Build form's capabilities.desktop: the applications on the person's
// computer a QuickAgent uses through their installed servers, picked from the
// catalogue of application servers built into this app, each with the tools
// the release narrows it to, and its computer use. Below them, what the install
// consent will ask, in its words (the Go side's DesktopConsent).
import { useEffect, useState } from "react";
import * as BuildAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/buildapi";
import * as build from "../../bindings/github.com/blaktron/agentry-desktop/internal/build/models";

/** The most application servers one release may name (the manifest rules). */
const MAX_SERVERS = 4;

const CLASS_WORDS: Record<string, string> = {
  read: "reads",
  act: "acts",
  exec: "runs code, asked each time",
};

/** The row with a server's tools set: every tool the catalogue gives is
 *  written as no narrowing, so the release takes later tools of the entry
 *  only by a consent diff, as the CLI words it. */
export function withTools(row: build.DesktopRow, id: string, tools: string[], all: string[]): build.DesktopRow {
  const narrowed = all.every((t) => tools.includes(t)) ? [] : all.filter((t) => tools.includes(t));
  return new build.DesktopRow({
    ...row,
    servers: row.servers.map((s) => (s.id === id ? new build.DesktopServerRow({ ...s, tools: narrowed }) : s)),
  });
}

/** The row with an application on or off; null when nothing is left. */
export function withServer(row: build.DesktopRow | undefined, id: string, on: boolean): build.DesktopRow | undefined {
  const cur = row ?? new build.DesktopRow({ servers: [] });
  const servers = on
    ? cur.servers.some((s) => s.id === id) ? cur.servers : [...cur.servers, new build.DesktopServerRow({ id, tools: [] })]
    : cur.servers.filter((s) => s.id !== id);
  if (servers.length === 0 && !cur.computerUse) return undefined;
  return new build.DesktopRow({ ...cur, servers });
}

/** The row with its computer use set; null when nothing is left. */
export function withComputerUse(row: build.DesktopRow | undefined, scope: string): build.DesktopRow | undefined {
  const cur = row ?? new build.DesktopRow({ servers: [] });
  if (cur.servers.length === 0 && !scope) return undefined;
  return new build.DesktopRow({ ...cur, computerUse: scope || undefined });
}

export function DesktopLine({ m, set }: { m: build.Model; set: (patch: Partial<build.Model>) => void }) {
  const [apps, setApps] = useState<build.DesktopApp[]>([]);
  const [consent, setConsent] = useState<string[]>([]);
  useEffect(() => {
    BuildAPI.DesktopApps().then((a) => setApps(a ?? [])).catch(() => setApps([]));
  }, []);
  const row = m.desktop ?? undefined;
  const key = JSON.stringify(row ?? null);
  useEffect(() => {
    if (!row) {
      setConsent([]);
      return;
    }
    BuildAPI.DesktopConsent(new build.Model({ ...m, desktop: row })).then((c) => setConsent(c ?? [])).catch(() => setConsent([]));
    // The consent follows the line alone.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  if (apps.length === 0 && !row) return null;
  const chosen = row?.servers ?? [];
  return (
    <div className="small" style={{ marginTop: 12 }}>
      does it use applications on the person&apos;s computer, through Agentry Desktop?
      <p className="muted small" style={{ margin: "2px 0 4px" }}>
        Each runs on the person&apos;s own computer, never in a run in Agentry. The release names the application; Agentry Desktop
        launches its pinned MCP server and asks the person before any tool that runs code.
      </p>
      {apps.map((app) => {
        const server = chosen.find((s) => s.id === app.id);
        const all = app.tools.map((t) => t.name);
        const allowed = server ? ((server.tools ?? []).length > 0 ? server.tools : all) : [];
        const full = !server && chosen.length >= MAX_SERVERS;
        return (
          <div key={app.id} className="card" style={{ marginTop: 4 }}>
            <label className="row small" style={{ gap: 4 }}>
              <input type="checkbox" checked={!!server} disabled={full}
                onChange={(e) => set({ desktop: withServer(row, app.id, e.target.checked) })} />
              {app.application} <span className="mono muted">({app.id})</span>
            </label>
            {server && (
              <div className="row" style={{ flexWrap: "wrap", marginTop: 4 }}>
                {app.tools.map((t) => (
                  <label key={t.name} className="row small" style={{ gap: 4 }} title={CLASS_WORDS[t.class] ?? t.class}>
                    <input type="checkbox" checked={allowed.includes(t.name)}
                      disabled={allowed.length === 1 && allowed.includes(t.name)}
                      onChange={(e) => set({ desktop: withTools(row!, app.id, e.target.checked ? [...allowed, t.name] : allowed.filter((x) => x !== t.name), all) })} />
                    <span className="mono">{t.name}</span>
                    <span className="muted">{CLASS_WORDS[t.class] ?? t.class}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
        );
      })}
      {chosen.filter((s) => !apps.some((a) => a.id === s.id)).map((s) => (
        <p key={s.id} className="small muted" style={{ marginTop: 4 }}>
          It names <span className="mono">{s.id}</span>, which this app&apos;s catalogue does not hold; kept as it was drafted or loaded.
        </p>
      ))}
      <label className="small" style={{ display: "block", marginTop: 6 }}>does it see and use windows (computer use)?
        <select value={row?.computerUse ?? ""} onChange={(e) => set({ desktop: withComputerUse(row, e.target.value) })}>
          <option value="">no</option>
          <option value="app">the windows of the applications above</option>
          <option value="screen">the whole screen</option>
        </select>
      </label>
      {consent.length > 0 && (
        <div className="muted small" style={{ marginTop: 6 }}>
          The install asks the person to agree to:
          <ul style={{ margin: "2px 0 0" }}>{consent.map((c) => <li key={c}>{c}</li>)}</ul>
        </div>
      )}
    </div>
  );
}
