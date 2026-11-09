// The Build form's capabilities.desktop: the applications on the person's
// computer a QuickAgent uses, each named by a short id: the library's presets
// (with the tools the release narrows each to), any other application by its
// id, and one the person chooses at the start; for each, whether the run uses
// its MCP server, its windows or both; and its computer use. Below them, what
// the install consent will ask, in its words (the Go side's DesktopConsent).
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

/** The id grammar a release names an application by. */
export const DESKTOP_ID = /^[a-z0-9]+(-[a-z0-9]+)*$/;

/** The row with an entry's use set: "" for the default (its server, and its
 *  windows where the line declares computer use), "mcp", "computer" or
 *  "both". The chooser is the entry whose id is "". A windows-only entry
 *  carries no narrowing. */
export function withUse(row: build.DesktopRow, id: string, use: string): build.DesktopRow {
  return new build.DesktopRow({
    ...row,
    servers: row.servers.map((s) =>
      (id === "" ? s.choose : !s.choose && s.id === id)
        ? new build.DesktopServerRow({ ...s, use: use || undefined, tools: use === "computer" ? [] : s.tools })
        : s,
    ),
  });
}

/** The row with the entry for an application the person chooses on or
 *  off; null when nothing is left. */
export function withChooser(row: build.DesktopRow | undefined, on: boolean): build.DesktopRow | undefined {
  const cur = row ?? new build.DesktopRow({ servers: [] });
  const servers = on
    ? cur.servers.some((s) => s.choose) ? cur.servers : [...cur.servers, new build.DesktopServerRow({ id: "", tools: [], choose: true })]
    : cur.servers.filter((s) => !s.choose);
  if (servers.length === 0 && !cur.computerUse) return undefined;
  return new build.DesktopRow({ ...cur, servers });
}

/** The row with an entry's narrowing set from a list the author typed:
 *  names separated by commas or spaces, none for every tool. */
export function withNarrowing(row: build.DesktopRow, id: string, text: string): build.DesktopRow {
  const tools = [...new Set(text.split(/[\s,]+/).map((t) => t.trim()).filter(Boolean))];
  return new build.DesktopRow({
    ...row,
    servers: row.servers.map((s) => (!s.choose && s.id === id ? new build.DesktopServerRow({ ...s, tools }) : s)),
  });
}

/** The row with an application on or off; null when nothing is left. */
export function withServer(row: build.DesktopRow | undefined, id: string, on: boolean): build.DesktopRow | undefined {
  const cur = row ?? new build.DesktopRow({ servers: [] });
  const servers = on
    ? cur.servers.some((s) => s.id === id) ? cur.servers : [...cur.servers, new build.DesktopServerRow({ id, tools: [] })]
    : cur.servers.filter((s) => s.choose || s.id !== id);
  if (servers.length === 0 && !cur.computerUse) return undefined;
  return new build.DesktopRow({ ...cur, servers });
}

/** The row with its computer use set; null when nothing is left. */
export function withComputerUse(row: build.DesktopRow | undefined, scope: string): build.DesktopRow | undefined {
  const cur = row ?? new build.DesktopRow({ servers: [] });
  if (cur.servers.length === 0 && !scope) return undefined;
  return new build.DesktopRow({ ...cur, computerUse: scope || undefined });
}

function UseSelect({ value, onChange, computerUse }: { value: string; onChange: (use: string) => void; computerUse: boolean }) {
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} aria-label="what the run uses of it">
      <option value="">{computerUse ? "its MCP server and its windows" : "its MCP server"}</option>
      <option value="mcp">its MCP server only</option>
      <option value="computer" disabled={!computerUse}>its windows only (needs computer use)</option>
      <option value="both" disabled={!computerUse}>its MCP server and its windows</option>
    </select>
  );
}

export function DesktopLine({ m, set }: { m: build.Model; set: (patch: Partial<build.Model>) => void }) {
  const [apps, setApps] = useState<build.DesktopApp[]>([]);
  const [consent, setConsent] = useState<string[]>([]);
  const [newID, setNewID] = useState("");
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
  const chosen = row?.servers ?? [];
  const full = chosen.length >= MAX_SERVERS;
  const computerUse = !!row?.computerUse;
  const chooser = chosen.find((s) => s.choose);
  const others = chosen.filter((s) => !s.choose && !apps.some((a) => a.id === s.id));
  const idOK = DESKTOP_ID.test(newID) && newID.length <= 40 && newID !== "computer" && newID !== "chosen" && !chosen.some((s) => s.id === newID);
  return (
    <div className="small" style={{ marginTop: 12 }} data-build-desktop>
      does it use applications on the person&apos;s computer, through Agentry Desktop?
      <p className="muted small" style={{ margin: "2px 0 4px" }}>
        Each runs on the person&apos;s own computer, never in a run in Agentry. The release names an application by a short id;
        Agentry Desktop uses the person&apos;s application with that id, or the one they choose, and asks them before any call
        that runs code or that they have not classed. The library&apos;s applications below have known ids and tools.
      </p>
      {apps.map((app) => {
        const server = chosen.find((s) => !s.choose && s.id === app.id);
        const all = app.tools.map((t) => t.name);
        const allowed = server ? ((server.tools ?? []).length > 0 ? server.tools : all) : [];
        return (
          <div key={app.id} className="card" style={{ marginTop: 4 }}>
            <label className="row small" style={{ gap: 4 }}>
              <input type="checkbox" checked={!!server} disabled={!server && full}
                onChange={(e) => set({ desktop: withServer(row, app.id, e.target.checked) })} />
              {app.application} <span className="mono muted">({app.id})</span>
            </label>
            {server && <UseSelect value={server.use ?? ""} computerUse={computerUse} onChange={(u) => set({ desktop: withUse(row!, app.id, u) })} />}
            {server && server.use !== "computer" && (
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
      {others.map((s) => (
        <div key={s.id} className="card" style={{ marginTop: 4 }} data-build-desktop-other={s.id}>
          <div className="row small" style={{ justifyContent: "space-between", gap: 4 }}>
            <span>An application called <span className="mono">{s.id}</span></span>
            <button type="button" className="link small" onClick={() => set({ desktop: withServer(row, s.id, false) })}>Remove</button>
          </div>
          <UseSelect value={s.use ?? ""} computerUse={computerUse} onChange={(u) => set({ desktop: withUse(row!, s.id, u) })} />
          {s.use !== "computer" && (
            <label className="small" style={{ display: "block", marginTop: 4 }}>
              only these of its tools (names, separated by commas; none for every tool its server lists):
              <input className="mono" defaultValue={(s.tools ?? []).join(", ")} onBlur={(e) => set({ desktop: withNarrowing(row!, s.id, e.target.value) })} />
            </label>
          )}
        </div>
      ))}
      <div className="row small" style={{ gap: 4, marginTop: 6, alignItems: "baseline" }}>
        another application, by its id:
        <input className="mono" value={newID} placeholder="my-cad" onChange={(e) => setNewID(e.target.value.toLowerCase())} />
        <button type="button" disabled={!idOK || full} onClick={() => { set({ desktop: withServer(row, newID, true) }); setNewID(""); }}>Add</button>
      </div>
      <label className="row small" style={{ gap: 4, marginTop: 6 }}>
        <input type="checkbox" checked={!!chooser} disabled={!chooser && full} onChange={(e) => set({ desktop: withChooser(row, e.target.checked) })} />
        an application the person chooses when the run starts
      </label>
      {chooser && <UseSelect value={chooser.use ?? ""} computerUse={computerUse} onChange={(u) => set({ desktop: withUse(row!, "", u) })} />}
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
