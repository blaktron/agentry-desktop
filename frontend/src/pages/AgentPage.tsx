import { useEffect, useState } from "react";
import * as BrowseAPI from "../../wailsjs/go/main/BrowseAPI";
import { main, trust } from "../../wailsjs/go/models";
import {
  CapabilityBadges, ErrorBox, Excerpt, KindBadge, QuarantineBadge, Sev,
  SignedBadge, Spinner, fmtDate, shortDigest,
} from "../ui";
import InstallWizard from "./InstallWizard";

type Tab = "readme" | "report" | "versions" | "provenance";

export default function AgentPage({
  registry, ns, name, onBack,
}: {
  registry: string;
  ns: string;
  name: string;
  onBack: () => void;
}) {
  const [detail, setDetail] = useState<main.AgentDetailView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>("readme");
  const [readme, setReadme] = useState<string | null>(null);
  const [report, setReport] = useState<main.ReportView | null>(null);
  const [reportErr, setReportErr] = useState<string | null>(null);
  const [prov, setProv] = useState<trust.AttestationResult | null>(null);
  const [installing, setInstalling] = useState(false);

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
        <button className="link" onClick={onBack}>← back to search</button>
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
      <button className="link" onClick={onBack}>← back to search</button>
      <div className="spread" style={{ marginTop: 10 }}>
        <h1 className="selectable">{a.namespace}/{a.name}</h1>
        <div className="row">
          <QuarantineBadge state={a.quarantine} />
          <KindBadge kind={a.kind} />
          {detail.latest
            ? <button className="primary" onClick={() => setInstalling(true)}>Install {detail.latest}</button>
            : <span className="badge">no published version</span>}
        </div>
      </div>
      <p className="page-sub selectable">{a.summary}</p>
      <div className="row">
        <CapabilityBadges caps={detail.latestCapabilities} />
        <SignedBadge att={prov} />
      </div>

      <div className="tabs">
        {(["readme", "report", "versions", "provenance"] as Tab[]).map((t) => (
          <button key={t} className={`tab ${tab === t ? "active" : ""}`} onClick={() => setTab(t)}>
            {t === "report" ? `security report${report ? ` (${totalFindings})` : ""}` : t}
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
          kind={a.kind}
          onClose={() => setInstalling(false)}
        />
      )}
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
