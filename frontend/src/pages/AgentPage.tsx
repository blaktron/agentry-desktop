import { useCallback, useEffect, useState } from "react";
import * as BrowseAPI from "../../wailsjs/go/main/BrowseAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import * as InstallAPI from "../../wailsjs/go/main/InstallAPI";
import { installed, main, trust } from "../../wailsjs/go/models";
import {
  CapabilityBadges, ErrorBox, Excerpt, ModeBadges, QuarantineBadge, Sev,
  SignedBadge, Spinner, fmtDate, shortDigest,
} from "../ui";
import InstallWizard from "./InstallWizard";
import RunPanel from "./RunPanel";
import ToolsPanel from "./ToolsPanel";

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
  const [installing, setInstalling] = useState(false);
  const [runOpen, setRunOpen] = useState(false);
  const [toolsOpen, setToolsOpen] = useState(false);

  // What THIS machine has accepted for this agent (shared install state) —
  // the installed strip below, with uninstall in place.
  const [installs, setInstalls] = useState<installed.Record[]>([]);
  const [confirmRemove, setConfirmRemove] = useState<installed.Record | null>(null);
  const [removeMsg, setRemoveMsg] = useState<string | null>(null);
  const refreshInstalls = useCallback(() => {
    InstallAPI.ListInstalled()
      .then((all) => setInstalls(all.filter((r) => r.namespace === ns && r.name === name)))
      .catch(() => setInstalls([]));
  }, [ns, name]);
  useEffect(refreshInstalls, [refreshInstalls]);

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
        BrowseAPI.Provenance(registry, ns, name, d.latest, d.latestDigest ?? "").then(setProv).catch(() => {});
      }
    }).catch((e) => setError(String(e)));
  }, [registry, ns, name]);

  if (error) {
    return (
      <div>
        <button className="link" onClick={onBack}>{backLabel}</button>
        <ErrorBox error={error} />
      </div>
    );
  }
  if (!detail) return <Spinner label={`Loading ${ns}/${name}…`} />;

  const a = detail.agent;
  const counts = report?.findingCounts ?? {};
  const totalFindings = Object.values(counts).reduce((n, v) => n + v, 0);

  return (
    <div>
      <button className="link" onClick={onBack}>{backLabel}</button>
      <div className="spread" style={{ marginTop: 10 }}>
        <h1 className="selectable">{a.namespace}/{a.name}</h1>
        <div className="row">
          <QuarantineBadge state={a.quarantine} />
          <ModeBadges modes={a.modes} />
          {detail.latest
            ? (
              <>
                <button className="primary" onClick={() => setRunOpen(true)}>Run</button>
                <button onClick={() => setToolsOpen(true)}>Set up tools</button>
                <button className="primary" onClick={() => setInstalling(true)}>Install {detail.latest}</button>
              </>
            )
            : <span className="badge">no published version</span>}
        </div>
      </div>
      <p className="page-sub selectable">{a.summary}</p>
      <div className="row">
        <CapabilityBadges caps={detail.latestCapabilities} />
        <SignedBadge att={prov} />
      </div>

      {removeMsg && <div className="notice selectable">{removeMsg}</div>}
      {installs.length > 0 && (
        <div className="card">
          <strong>Installed on this machine</strong>
          {installs.map((rec) => (
            <div key={`${rec.target} ${rec.dir}`} className="spread" style={{ marginTop: 6 }}>
              <div className="grow">
                <span className="mono">v{rec.version}</span>{" "}
                <span className="badge">{rec.target}</span>{" "}
                <span className="muted small mono selectable">{rec.dir}</span>
              </div>
              <button className="danger" onClick={() => setConfirmRemove(rec)}>Remove</button>
            </div>
          ))}
        </div>
      )}

      {confirmRemove && (
        <div className="modal-backdrop" onClick={() => setConfirmRemove(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2 style={{ marginTop: 0 }}>Remove {confirmRemove.namespace}/{confirmRemove.name}?</h2>
            <p>
              {confirmRemove.managed
                ? <>This removes <span className="mono selectable">{confirmRemove.dir}</span> and its host-config entry.</>
                : <>The directory was chosen by you, so its files are <strong>left in place</strong>; only the install record is removed.</>}
            </p>
            <div className="row" style={{ justifyContent: "flex-end" }}>
              <button onClick={() => setConfirmRemove(null)}>Cancel</button>
              <button className="danger" onClick={() => uninstall(confirmRemove)}>Remove</button>
            </div>
          </div>
        </div>
      )}

      <div className="tabs">
        {(["readme", "mcp", "report", "versions", "provenance"] as Tab[]).map((t) => (
          <button key={t} className={`tab ${tab === t ? "active" : ""}`} onClick={() => setTab(t)}>
            {t === "report" ? `security report${report ? ` (${totalFindings})` : ""}` : TAB_LABELS[t]}
          </button>
        ))}
      </div>

      {tab === "readme" && (
        <div className="card">
          {readme === null && <Spinner />}
          {readme === "" && <div className="empty">No README.</div>}
          {/* Server-sanitized HTML (§10.3): the API strips scripts/handlers
              and marks invisible characters; the client never parses raw
              markdown from the archive. */}
          {readme && <div className="readme selectable" dangerouslySetInnerHTML={{ __html: readme }} />}
        </div>
      )}

      {tab === "mcp" && (
        <div className="card">
          <MCPSection registry={registry} ns={ns} name={name} hasPublished={!!detail.latest} />
        </div>
      )}

      {tab === "report" && (
        <div className="card">
          <ReportCard report={report} error={reportErr} version={detail.latest} />
        </div>
      )}

      {tab === "versions" && (
        <div className="card">
          <table className="plain selectable">
            <thead>
              <tr><th>version</th><th>channel</th><th>status</th><th>published</th><th>digest</th></tr>
            </thead>
            <tbody>
              {detail.versions.map((v) => (
                <tr key={v.version}>
                  <td className="mono">{v.version}{v.yanked && <span className="badge bad" style={{ marginLeft: 6 }}>yanked</span>}</td>
                  <td>{v.channel}</td>
                  <td>{v.status}</td>
                  <td>{fmtDate(v.publishedAt || v.createdAt)}</td>
                  <td className="mono" title={v.archiveDigest}>{shortDigest(v.archiveDigest)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === "provenance" && (
        <div className="card selectable">
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
              {prov && prov.state !== "verified" && <p className="muted small">{prov.detail}</p>}
            </>
          )}
        </div>
      )}

      {installing && detail.latest && (
        <InstallWizard
          registry={registry}
          agentRef={`${ns}/${name}@${detail.latest}`}
          onClose={() => setInstalling(false)}
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
