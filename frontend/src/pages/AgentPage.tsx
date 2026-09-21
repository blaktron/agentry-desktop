import { useCallback, useEffect, useState, type MouseEvent } from "react";
import * as BrowseAPI from "../../wailsjs/go/main/BrowseAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import * as InstallAPI from "../../wailsjs/go/main/InstallAPI";
import { installed, main, trust } from "../../wailsjs/go/models";
import {
  Button, Card, CapabilityBadges, ErrorBox, Excerpt, Modal, ModeBadges,
  Notice, PageHeader, QuarantineBadge, Sev, SignedBadge, Spinner, fmtDate, shortDigest,
} from "../ui";
import InstallWizard from "./InstallWizard";
import RunPanel from "./RunPanel";
import ToolsPanel from "./ToolsPanel";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";

/** A README link goes to the OS browser (http/https) or nowhere; never this window. */
function openReadmeLinkOutside(e: MouseEvent<HTMLDivElement>) {
  const a = (e.target as HTMLElement | null)?.closest?.("a[href]") as HTMLAnchorElement | null;
  if (!a) return;
  e.preventDefault();
  const href = a.getAttribute("href") ?? "";
  if (/^https?:\/\//i.test(href)) BrowserOpenURL(href);
}

type Tab = "readme" | "mcp" | "report" | "versions" | "provenance";

const TAB_LABELS: Record<Tab, string> = {
  readme: "readme",
  mcp: "MCP servers & tools",
  report: "security report",
  versions: "versions",
  provenance: "provenance",
};

export default function AgentPage({
  registry, ns, name, onBack, backLabel = "← back to search",
}: {
  registry: string;
  ns: string;
  name: string;
  onBack: () => void;
  backLabel?: string;
}) {
  const [detail, setDetail] = useState<main.AgentDetailView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>("readme");
  const [readme, setReadme] = useState<string | null>(null);
  const [report, setReport] = useState<main.ReportView | null>(null);
  const [reportErr, setReportErr] = useState<string | null>(null);
  const [prov, setProv] = useState<trust.AttestationResult | null>(null);
  // The coding-agent export (the wizard) is the one-off behind a link;
  // the primary Install is one click into the runner home (D-031).
  const [wizardOpen, setWizardOpen] = useState(false);
  const [installBusy, setInstallBusy] = useState(false);
  const [installError, setInstallError] = useState<string | null>(null);
  const [runOpen, setRunOpen] = useState(false);
  const [toolsOpen, setToolsOpen] = useState(false);

  // What THIS machine has accepted for this agent (shared install state) —
  // the installed strip below, with uninstall in place.
  const [installs, setInstalls] = useState<installed.Record[]>([]);
  const [confirmRemove, setConfirmRemove] = useState<installed.Record | null>(null);
  const [removeMsg, setRemoveMsg] = useState<string | null>(null);
  const refreshInstalls = useCallback(() =>
    InstallAPI.ListInstalled()
      // Only what this app can uninstall and that is still on disk — the
      // runner-home install among them since D-031; gone directories
      // belong to the Installed page's other section (agentry-desktop#63).
      .then((all) => setInstalls(all.filter((r) => r.namespace === ns && r.name === name && r.manageable && r.present)))
      .catch(() => setInstalls([])),
  [ns, name]);
  useEffect(() => { void refreshInstalls(); }, [refreshInstalls]);

  // The runner-home record for this BriefAgent, if any: what the primary
  // button reads to say Install, Installing…, or Update.
  const runnerInstall = installs.find((r) => r.target === "runner");

  // One click (D-031): resolve, verify, lay down in the runner home. The
  // gates — an unsigned or unscanned release, a contract to accept — are
  // native dialogs raised by the backend; a refusal or a failure is shown
  // here, in place, and the button goes back to Install.
  async function installOneClick() {
    if (!detail?.latest) return;
    setInstallBusy(true);
    setInstallError(null);
    try {
      await InstallAPI.InstallRunner(registry, `${ns}/${name}@${detail.latest}`);
      // Awaited, so the button reads Update as soon as it is enabled again
      // rather than flashing Install first.
      await refreshInstalls();
    } catch (e) {
      setInstallError(String(e));
    } finally {
      setInstallBusy(false);
    }
  }

  async function uninstall(rec: installed.Record) {
    setConfirmRemove(null);
    try {
      const msg = await InstallAPI.Uninstall(
        new installed.Key({ registry: rec.registry, namespace: rec.namespace, name: rec.name, target: rec.target, dir: rec.dir }),
      );
      setRemoveMsg(msg);
      refreshInstalls();
    } catch (e) {
      setRemoveMsg(String(e));
    }
  }

  useEffect(() => {
    setDetail(null);
    setError(null);
    setReadme(null);
    setReport(null);
    setProv(null);
    BrowseAPI.Agent(registry, ns, name).then(async (d) => {
      setDetail(d);
      if (d.latest) {
        BrowseAPI.Readme(registry, ns, name, d.latest).then(setReadme).catch(() => setReadme(""));
        BrowseAPI.Report(registry, ns, name, d.latest).then(setReport).catch((e) => setReportErr(String(e)));
        BrowseAPI.Provenance(registry, ns, name, d.latest, d.latestDigest ?? "", d.latestScanStatus ?? "").then(setProv).catch(() => {});
      }
    }).catch((e) => setError(String(e)));
  }, [registry, ns, name]);

  if (error) {
    return (
      <div>
        <Button variant="link" onClick={onBack}>{backLabel}</Button>
        <ErrorBox error={error} />
      </div>
    );
  }
  if (!detail) return <Spinner label={`Loading ${ns}/${name}…`} />;

  const a = detail.agent;
  // The one primary button (D-031): Install, then Installing…, then Update
  // — disabled when the installed version is the latest the page shows.
  let installButton: React.ReactNode;
  if (installBusy) {
    installButton = <Button variant="primary" disabled>Installing…</Button>;
  } else if (!runnerInstall) {
    installButton = <Button variant="primary" onClick={installOneClick}>Install {detail.latest}</Button>;
  } else if (runnerInstall.version !== detail.latest) {
    installButton = <Button variant="primary" onClick={installOneClick} title={`installed ${runnerInstall.version}`}>Update to {detail.latest}</Button>;
  } else {
    installButton = <Button variant="primary" disabled title={`${runnerInstall.version} is installed on this machine; nothing newer is known`}>Update</Button>;
  }
  const counts = report?.findingCounts ?? {};
  const totalFindings = Object.values(counts).reduce((n, v) => n + v, 0);

  return (
    <div>
      <Button variant="link" onClick={onBack}>{backLabel}</Button>
      <PageHeader
        title={<span className="selectable">{a.namespace}/{a.name}</span>}
        sub={<span className="selectable">{a.summary}</span>}
        actions={<>
          <QuarantineBadge state={a.quarantine} />
          <ModeBadges modes={a.modes} />
          {detail.latest
            ? (
              <>
                <Button variant="primary" onClick={() => setRunOpen(true)}>Run</Button>
                <Button onClick={() => setToolsOpen(true)}>Set up tools</Button>
                {installButton}
              </>
            )
            : <span className="badge">no published version</span>}
        </>}
      />
      <div className="row" style={{ margin: "0 0 8px" }}>
        <CapabilityBadges caps={detail.latestCapabilities} />
        <SignedBadge att={prov} />
        {detail.latest && (
          <Button variant="link" onClick={() => setWizardOpen(true)} disabled={installBusy}>Install into a coding agent…</Button>
        )}
      </div>
      {installBusy && <p className="muted small">Resolving, verifying and installing into the runner home…</p>}
      <ErrorBox error={installError} />

      {removeMsg && <Notice tone="info">{removeMsg}</Notice>}
      {detail.dependencyAlerts.length > 0 && (
        <Card className="selectable">
          <strong>Open dependency alerts</strong>
          <p className="muted small" style={{ margin: "4px 0 6px" }}>
            Versions you published declare an MCP server version with a newly known high or
            critical vulnerability. An alert closes when the version is yanked, quarantined or
            blocked, or when the advisory is withdrawn; publishing a fixed version does not close it.
          </p>
          {detail.dependencyAlerts.map((al) => (
            <div key={al.id} className="spread" style={{ marginTop: 4 }}>
              <div className="grow">
                <span className="mono">v{al.version}</span>{" "}
                <span className="mono">{al.ecosystem}:{al.package}@{al.packageVersion}</span>{" "}
                <Sev severity={al.severity} />{" "}
                <span className="small">
                  {al.advisoryIds.join(", ")}
                  {al.fixedIn.length > 0 ? `; fixed in ${al.fixedIn.join(", ")}` : ""}
                </span>
              </div>
              <span className="muted small">{fmtDate(al.createdAt)}</span>
            </div>
          ))}
        </Card>
      )}
      {installs.length > 0 && (
        <Card>
          <strong>Installed on this machine</strong>
          {installs.map((rec) => (
            <div key={`${rec.target} ${rec.dir}`} className="spread" style={{ marginTop: 6 }}>
              <div className="grow">
                <span className="mono">v{rec.version}</span>{" "}
                <span className="badge">{rec.target}</span>{" "}
                <span className="muted small mono selectable">{rec.dir}</span>
              </div>
              <Button variant="danger" onClick={() => setConfirmRemove(rec)}>Remove</Button>
            </div>
          ))}
        </Card>
      )}

      {confirmRemove && (
        <Modal title={`Remove ${confirmRemove.namespace}/${confirmRemove.name}?`} onClose={() => setConfirmRemove(null)}>
          <p>
            {confirmRemove.target === "runner"
              ? <>This removes <span className="mono selectable">{confirmRemove.dir}</span> — the runner home's copy of this BriefAgent — and its record. The next Run fetches it again.</>
              : confirmRemove.managed
                ? <>This removes <span className="mono selectable">{confirmRemove.dir}</span> and its host-config entry.</>
                : <>The directory was chosen by you, so its files are <strong>left in place</strong>; only the install record is removed.</>}
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setConfirmRemove(null)}>Cancel</Button>
            <Button variant="danger" onClick={() => uninstall(confirmRemove)}>Remove</Button>
          </div>
        </Modal>
      )}

      <div className="tabs">
        {(["readme", "mcp", "report", "versions", "provenance"] as Tab[]).map((t) => (
          <button key={t} className={`tab ${tab === t ? "active" : ""}`} onClick={() => setTab(t)}>
            {t === "report" ? `security report${report ? ` (${totalFindings})` : ""}` : TAB_LABELS[t]}
          </button>
        ))}
      </div>

      {tab === "readme" && (
        <Card>
          {readme === null && <Spinner />}
          {readme === "" && <div className="empty">No README.</div>}
          {/* Server-sanitized HTML (§10.3): the API strips scripts/handlers
              and marks invisible characters; the client never parses raw
              markdown from the archive. The window's CSP (internal/webview)
              is the second layer. Links never navigate this window: an
              http(s) link opens in the OS browser, anything else is
              inert — the page that runs here has the machine as its API. */}
          {readme && (
            <div className="readme selectable" onClick={openReadmeLinkOutside} dangerouslySetInnerHTML={{ __html: readme }} />
          )}
        </Card>
      )}

      {tab === "mcp" && (
        <Card>
          <MCPSection registry={registry} ns={ns} name={name} hasPublished={!!detail.latest} />
        </Card>
      )}

      {tab === "report" && (
        <Card>
          <ReportCard report={report} error={reportErr} version={detail.latest} />
        </Card>
      )}

      {tab === "versions" && (
        <Card>
          <table className="plain selectable">
            <thead>
              <tr><th>version</th><th>channel</th><th>status</th><th>published</th><th>digest</th></tr>
            </thead>
            <tbody>
              {detail.versions.map((v) => (
                <tr key={v.version}>
                  <td className="mono">{v.version}{v.yanked && <span className="badge bad" style={{ marginLeft: 6 }}>yanked</span>}{v.scanStatus === "skipped" && <span className="badge unscanned" style={{ marginLeft: 6 }}>unscanned</span>}</td>
                  <td>{v.channel}</td>
                  <td>{v.status}</td>
                  <td>{fmtDate(v.publishedAt || v.createdAt)}</td>
                  <td className="mono" title={v.archiveDigest}>{shortDigest(v.archiveDigest)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}

      {tab === "provenance" && (
        <Card className={`selectable${prov?.state === "unscanned" ? " mark-unscanned" : ""}`}>
          {!detail.latest && <div className="empty">No published version.</div>}
          {detail.latest && (
            <>
              <div className="kv"><span className="k">version</span><span className="mono">{detail.latest}</span></div>
              <div className="kv"><span className="k">archive digest</span><span className="mono">{detail.latestDigest || "unknown"}</span></div>
              <div className="kv"><span className="k">attestation</span><SignedBadge att={prov} /></div>
              {prov?.state === "verified" && (
                <>
                  <div className="kv"><span className="k">signing key</span><span className="mono">{prov.keyId}</span></div>
                  {prov.rulesetVersion && <div className="kv"><span className="k">scanned with</span>ruleset {prov.rulesetVersion}{prov.judgeModel ? `, judge ${prov.judgeModel}` : ""}</div>}
                  <p className="muted small">
                    The DSSE signature covers the digest the registry declares for this version.
                    Installing re-verifies it against the bytes actually downloaded to this machine.
                  </p>
                </>
              )}
              {prov?.state === "unscanned" && (
                <p className="muted small">
                  The registry admitted this version without a scan: its organization&apos;s administrator took the agent off
                  the pipeline, so no findings, no report and no attestation stand behind it, and no scanner looked at its
                  contents. Installing or running it asks for that consent, its own — accepting an unsigned release does not
                  cover it.
                </p>
              )}
              {prov && prov.state !== "verified" && prov.state !== "unscanned" && <p className="muted small">{prov.detail}</p>}
            </>
          )}
        </Card>
      )}

      {wizardOpen && detail.latest && (
        <InstallWizard
          registry={registry}
          agentRef={`${ns}/${name}@${detail.latest}`}
          onClose={() => setWizardOpen(false)}
          onInstalled={refreshInstalls}
        />
      )}

      {toolsOpen && detail.latest && (
        <ToolsPanel
          registry={registry}
          ns={ns}
          name={name}
          version={detail.latest}
          onClose={() => setToolsOpen(false)}
        />
      )}

      {runOpen && detail.latest && (
        <RunPanel
          registry={registry}
          ns={ns}
          name={name}
          version={detail.latest}
          attestation={prov}
          onClose={() => setRunOpen(false)}
        />
      )}
    </div>
  );
}

/** The "MCP servers & tools" section (RUNNER-PLAN-2 §P6, req 3/4): the
 * tool surface the latest version's declared contract becomes — through
 * the runner's per-run gateway, and as host built-ins in an embedded
 * install. Same information as the site's section. */
function MCPSection({
  registry, ns, name, hasPublished,
}: {
  registry: string;
  ns: string;
  name: string;
  hasPublished: boolean;
}) {
  const [info, setInfo] = useState<main.MCPView | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!hasPublished) return;
    setInfo(null);
    setError(null);
    RunAPI.GatewayRequirements(registry, ns, name).then(setInfo).catch((e) => setError(String(e)));
  }, [registry, ns, name, hasPublished]);

  if (!hasPublished) return <div className="empty">No published version.</div>;
  if (error) return <ErrorBox error={error} />;
  if (!info) return <Spinner label="Deriving the tool surface…" />;

  return (
    <div className="selectable">
      <p className="muted small" style={{ marginTop: 0 }}>
        <span className="mono">agentry run</span> (and Run here) serves exactly the tools this
        contract declares, through a per-run MCP gateway. An undeclared capability is a tool
        that does not exist at run time.
      </p>

      <h3>Tools the runner grants</h3>
      <table className="plain">
        <thead>
          <tr><th>tool</th><th>granted by</th></tr>
        </thead>
        <tbody>
          {info.toolRows.map((r) => (
            <tr key={r.tool}>
              <td className="mono">{r.tool}</td>
              <td>{r.grantedBy}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h3>MCP servers involved</h3>
      {info.servers.map((s) => (
        <p key={s} style={{ margin: "4px 0" }}>{s}</p>
      ))}
      {info.vulnerabilities.length > 0 && (
        <>
          <h3>Declared servers, per the registry</h3>
          <table className="plain">
            <thead>
              <tr><th>server</th><th>status</th><th>advisories</th></tr>
            </thead>
            <tbody>
              {info.vulnerabilities.map((v) => (
                <tr key={v.ref}>
                  <td className="mono">{v.ref}</td>
                  <td>
                    {v.status}
                    {v.maxSeverity ? <> <Sev severity={v.maxSeverity} /></> : null}
                    {v.error ? <span className="muted small"> {v.error}</span> : null}
                  </td>
                  <td className="small">
                    {(v.advisories ?? []).map((a) => (
                      <div key={a.id}>
                        <span className="mono">{a.id}</span> {a.severity}
                        {a.fixedIn.length > 0 ? `, fixed in ${a.fixedIn.join(", ")}` : ""}
                      </div>
                    ))}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {info.refusal
            ? (
              <div className="error-box selectable">
                Runs are refused by default: {info.refusal}. The run panel has the override; it is
                recorded in the run&apos;s receipt.
              </div>
            )
            : <p className="muted small">Declared MCP servers {info.vulnerabilityLine}.</p>}
        </>
      )}
      {info.dataAccess.length > 0 && (
        <p className="muted small">
          Operator-cataloged MCP servers may be attached for the declared dataAccess{" "}
          {info.dataAccess.length > 1 ? "categories" : "category"} (
          <span className="mono">{info.dataAccess.join(", ")}</span>); undeclared categories are
          never wired in.
        </p>
      )}

      {info.secretNames.length > 0 && (
        <>
          <h3>Keys &amp; identities</h3>
          <p className="mono">{info.secretNames.join(", ")}</p>
          <p className="muted small">
            The runner&apos;s guided setup collects these once into the BriefAgent&apos;s MCP
            profile, kept or removed per your choice. Values are never published or shown.
          </p>
        </>
      )}

      <h3>In an embedded install</h3>
      <p className="muted small">
        Installed into an existing coding agent, the same contract maps to the host&apos;s
        built-in tools — enforcement there is the host&apos;s. Allowed tools:{" "}
        {info.embeddedTools.length > 0
          ? <span className="mono">{info.embeddedTools.join(", ")}</span>
          : <span>none declared (host defaults apply)</span>}
        .
      </p>
    </div>
  );
}

export function ReportCard({
  report, error, version,
}: {
  report: main.ReportView | null;
  error: string | null;
  version: string;
}) {
  if (error) return <ErrorBox error={error} />;
  if (!version) return <div className="empty">No published version to report on.</div>;
  if (!report) return <Spinner label="Loading report…" />;

  const counts = report.findingCounts ?? {};
  const total = Object.values(counts).reduce((n, v) => n + v, 0);
  const order = ["critical", "high", "medium", "low", "info"];

  // Unscanned (core M21): the registry admitted the version without a
  // scan, and an empty report must never read as "scanned, 0 findings".
  if (report.verdict === "unscanned" || report.scanStatus === "skipped") {
    return (
      <div className="notice notice-unscanned mark-unscanned">
        <div className="row">
          <span className="badge unscanned">unscanned</span>
          <strong>The registry admitted this version without a scan.</strong>
        </div>
        <p className="muted small" style={{ marginBottom: 0 }}>
          Its organization&apos;s administrator took the agent off the pipeline: no findings, no report and no attestation
          stand behind it. No scanner looked at its contents. Installing or running it asks for that consent, its own.
          Status {report.versionStatus}.
        </p>
      </div>
    );
  }

  return (
    <div>
      <div className="row">
        <strong>scanned — {total === 0 ? "0 findings" : `${total} finding${total === 1 ? "" : "s"}`}</strong>
        {order.filter((s) => counts[s]).map((s) => (
          <span key={s} className="row" style={{ gap: 4 }}>
            <Sev severity={s} /> {counts[s]}
          </span>
        ))}
      </div>
      <p className="muted small">
        {report.rulesetVersion && <>ruleset {report.rulesetVersion} · </>}
        {report.finishedAt && <>scanned {fmtDate(report.finishedAt)} · </>}
        status {report.versionStatus}
        {report.judgeModel && <> · judge {report.judgeModel}</>}
      </p>
      {report.findings.length > 0 && (
        <div>
          {report.findings.map((f, i) => (
            <div key={i} className="finding">
              <div className="row">
                <Sev severity={f.severity} />
                <span className="mono">{f.scanner}</span>
                <strong>{f.title}</strong>
                {f.verdict && f.verdict !== "open" && <span className="badge">{f.verdict}</span>}
              </div>
              {f.filePath && (
                <div className="loc mono">
                  {f.filePath}{f.lineStart ? `:${f.lineStart}` : ""}
                </div>
              )}
              <Excerpt text={f.excerpt} />
              {f.verdictReason && <div className="muted small">reviewer: {f.verdictReason}</div>}
            </div>
          ))}
        </div>
      )}
      <p className="muted small">
        A scan is evidence, not a verdict: it means these detectors ran and this is what they found.
      </p>
    </div>
  );
}
