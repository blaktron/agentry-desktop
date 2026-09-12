import { useCallback, useEffect, useState, type ReactNode } from "react";
import * as InstallAPI from "../../wailsjs/go/main/InstallAPI";
import { install, installed } from "../../wailsjs/go/models";
import {
  AgentRow, Button, EmptyState, ErrorBox, Modal, Notice, PageHeader, Spinner, fmtDate,
} from "../ui";
import InstallWizard from "./InstallWizard";

type UpdateMap = Record<string, install.UpdateInfo>;

function keyOf(r: { registry: string; namespace: string; name: string; target: string; dir: string }): string {
  return [r.registry, r.namespace, r.name, r.target, r.dir].join(" ");
}

export default function Installed({
  onBrowse, onOpenAgent,
}: {
  onBrowse: () => void;
  onOpenAgent: (registry: string, ns: string, name: string) => void;
}) {
  const [records, setRecords] = useState<install.Item[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [updates, setUpdates] = useState<UpdateMap>({});
  const [checking, setChecking] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState<install.Item | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [updating, setUpdating] = useState<{ rec: install.Item; dir: string; ref: string } | null>(null);

  const refresh = useCallback(() => {
    InstallAPI.ListInstalled().then(setRecords).catch((e) => setError(String(e)));
  }, []);
  useEffect(refresh, [refresh]);

  async function checkUpdates() {
    setChecking(true);
    setError(null);
    try {
      const infos = await InstallAPI.CheckUpdates();
      const map: UpdateMap = {};
      for (const u of infos) map[keyOf(u.key)] = u;
      setUpdates(map);
    } catch (e) {
      setError(String(e));
    } finally {
      setChecking(false);
    }
  }

  async function uninstall(rec: install.Item) {
    setConfirmRemove(null);
    setError(null);
    try {
      const msg = await InstallAPI.Uninstall(
        new installed.Key({ registry: rec.registry, namespace: rec.namespace, name: rec.name, target: rec.target, dir: rec.dir }),
      );
      setNotice(msg);
      refresh();
    } catch (e) {
      setError(String(e));
    }
  }

  // Forget drops the record of a directory that is gone; the engine refuses
  // it for a directory that still exists, so this can never leave files
  // behind unrecorded (agentry-desktop#63).
  async function forget(rec: install.Item) {
    setError(null);
    try {
      await InstallAPI.Forget(
        new installed.Key({ registry: rec.registry, namespace: rec.namespace, name: rec.name, target: rec.target, dir: rec.dir }),
      );
      setNotice(`Forgot ${rec.namespace}/${rec.name}; nothing on disk was touched.`);
      refresh();
    } catch (e) {
      setError(String(e));
    }
  }

  async function startUpdate(rec: install.Item) {
    setError(null);
    try {
      const dir = await InstallAPI.UpdateDir(rec);
      const latest = updates[keyOf(rec)]?.latest;
      const ref = latest ? `${rec.namespace}/${rec.name}@${latest}` : `${rec.namespace}/${rec.name}`;
      setUpdating({ rec, dir, ref });
    } catch (e) {
      setError(String(e));
    }
  }

  function capsSummary(rec: install.Item): string {
    const c = rec.capabilities;
    const bits: string[] = [];
    if (c.egressHosts?.length) bits.push(`net: ${c.egressHosts.join(", ")}`);
    if (c.execCommands?.length) bits.push(`exec: ${c.execCommands.join(", ")}`);
    if (c.fsWrite && c.fsWrite !== "none") bits.push(`fs write: ${c.fsWrite}`);
    if (c.fsRead && c.fsRead !== "none") bits.push(`fs read: ${c.fsRead}`);
    if (c.secrets?.length) bits.push(`secrets: ${c.secrets.join(", ")}`);
    return bits.length ? bits.join(" · ") : "no capabilities accepted";
  }

  const managed = (records ?? []).filter((r) => r.manageable && r.present);
  const consents = (records ?? []).filter((r) => !r.manageable && r.present);
  const gone = (records ?? []).filter((r) => !r.present);

  function section(title: string, items: install.Item[], row: (rec: install.Item) => ReactNode, empty?: string) {
    return (
      <section>
        <h3 className="muted small" style={{ marginTop: "1rem" }}>{title}</h3>
        {items.length === 0 && empty && <p className="muted small">{empty}</p>}
        {items.map(row)}
      </section>
    );
  }

  return (
    <div>
      <PageHeader
        title="Installed"
        sub={<>What this machine has accepted — shared state with the <span className="mono">agentry</span> CLI (<span className="mono">~/.config/agentry/installed.yaml</span>).</>}
        actions={<Button onClick={checkUpdates} busy={checking} busyLabel="Checking…" disabled={managed.length === 0}>Check for updates</Button>}
      />

      <ErrorBox error={error} />
      {notice && <Notice tone="info">{notice}</Notice>}
      {!records && <Spinner />}
      {records && records.length === 0 && (
        <EmptyState title="Nothing installed yet" action={<Button variant="primary" onClick={onBrowse}>Browse the registry</Button>}>
          Install a BriefAgent from the registry to see it here.
        </EmptyState>
      )}

      {/* Three sections (agentry-desktop#63): what this app can manage, the
          run consents the CLI wrote into the same file (shown, not managed),
          and records whose directory is gone (Forget removes the line). */}
      {records && records.length > 0 && (
        <>
          {section(
            "Installed here",
            managed,
            (rec) => {
              const u = updates[keyOf(rec)];
              return (
                <AgentRow
                  key={keyOf(rec)}
                  title={`${rec.namespace}/${rec.name}`}
                  badges={<>
                    <span className="mono small">v{rec.version}</span>
                    <span className="badge">{rec.target}</span>
                    {u?.updateAvailable && <span className="badge warn">update: {u.latest}</span>}
                    {u && !u.updateAvailable && !u.error && <span className="badge ok">up to date</span>}
                  </>}
                  summary={<span className="mono small selectable">{rec.dir}</span>}
                  meta={<>{capsSummary(rec)} · installed {fmtDate(rec.installedAt as unknown as string)} · {rec.registry.replace(/^https?:\/\//, "")}{u?.error ? ` · update check failed: ${u.error}` : ""}</>}
                  actions={<>
                    <Button onClick={() => onOpenAgent(rec.registry, rec.namespace, rec.name)}>Details</Button>
                    {u?.updateAvailable && <Button variant="primary" onClick={() => startUpdate(rec)}>Update</Button>}
                    <Button variant="danger" onClick={() => setConfirmRemove(rec)}>Uninstall</Button>
                  </>}
                  onClick={() => onOpenAgent(rec.registry, rec.namespace, rec.name)}
                />
              );
            },
            "Nothing this app installed. Run consents and gone directories are listed below.",
          )}
          {consents.length > 0 && section(
            "Run consents recorded by the CLI",
            consents,
            (rec) => (
              <AgentRow
                key={keyOf(rec)}
                title={`${rec.namespace}/${rec.name}`}
                badges={<><span className="mono small">v{rec.version}</span><span className="badge">{rec.target}</span></>}
                summary={<span className="mono small selectable">{rec.dir}</span>}
                meta={<>{capsSummary(rec)} · accepted {fmtDate(rec.installedAt as unknown as string)} · recorded by <span className="mono">agentry run</span>; this app does not manage it</>}
              />
            ),
          )}
          {gone.length > 0 && section(
            "No longer on disk",
            gone,
            (rec) => (
              <AgentRow
                key={keyOf(rec)}
                title={`${rec.namespace}/${rec.name}`}
                badges={<><span className="mono small">v{rec.version}</span><span className="badge">{rec.target}</span><span className="badge warn">directory missing</span></>}
                summary={<span className="mono small selectable">{rec.dir}</span>}
                meta={<>recorded {fmtDate(rec.installedAt as unknown as string)} · the directory is gone, so there is nothing to uninstall; Forget removes only this record</>}
                actions={<Button onClick={() => forget(rec)}>Forget</Button>}
              />
            ),
          )}
        </>
      )}

      {confirmRemove && (
        <Modal title={`Uninstall ${confirmRemove.namespace}/${confirmRemove.name}?`} onClose={() => setConfirmRemove(null)}>
          {confirmRemove.managed ? (
            <p>
              This removes <span className="mono selectable">{confirmRemove.dir}</span>
              {confirmRemove.target === "mcp" && " and its entry in the project's .mcp.json"}
              {confirmRemove.target === "agents-md" && " and its section in the project's AGENTS.md"}.
            </p>
          ) : (
            <p>
              The directory <span className="mono selectable">{confirmRemove.dir}</span> was chosen by you, so its
              files are <strong>left in place</strong>; only the install record is removed.
            </p>
          )}
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setConfirmRemove(null)}>Cancel</Button>
            <Button variant="danger" onClick={() => uninstall(confirmRemove)}>Uninstall</Button>
          </div>
        </Modal>
      )}

      {updating && (
        <InstallWizard
          registry={updating.rec.registry}
          agentRef={updating.ref}
          presetTarget={updating.rec.target}
          presetDir={updating.dir}
          onClose={() => setUpdating(null)}
          onInstalled={() => { refresh(); setUpdates({}); }}
        />
      )}
    </div>
  );
}
