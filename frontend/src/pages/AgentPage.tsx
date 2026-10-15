import { useCallback, useEffect, useState, type MouseEvent } from "react";
import * as BrowseAPI from "../../wailsjs/go/app/BrowseAPI";
import * as RunAPI from "../../wailsjs/go/app/RunAPI";
import * as InstallAPI from "../../wailsjs/go/app/InstallAPI";
import * as SettingsAPI from "../../wailsjs/go/app/SettingsAPI";
import { install, installed, app, trust } from "../../wailsjs/go/models";
import {
  Button, Card, CapabilityBadges, ErrorBox, Excerpt, Modal, ModeBadges,
  Notice, PageHeader, QuarantineBadge, ServerEnv, Sev, SignedBadge, Spinner, fmtDate, shortDigest,
} from "../ui";
import InstallWizard from "./InstallWizard";
import { codingAgentName } from "./CodingAgents";
import RunPanel from "./RunPanel";
import { advisoryWords } from "../advisory";
import ToolsPanel from "./ToolsPanel";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import { heldForm, offersEdit } from "../edit";
import ReviseDialog from "./ReviseDialog";
import ForkDialog from "./ForkDialog";
import { ORIGIN_GONE, forkable, forkCopyNotice, forkCountLine, forkOfLabel } from "../fork";
import { FORK_STAYS_PRIVATE } from "../publisher";
import { PinButton, useHomeLayout } from "./home/PinButton";
import { instructionsNote } from "../instructions";
import { RevealText } from "../reveal";

/** A README link goes to the OS browser (http/https) or nowhere; never this window. */
function openReadmeLinkOutside(e: MouseEvent<HTMLDivElement>) {
  const a = (e.target as HTMLElement | null)?.closest?.("a[href]") as HTMLAnchorElement | null;
  if (!a) return;
  e.preventDefault();
  const href = a.getAttribute("href") ?? "";
  if (/^https?:\/\//i.test(href)) BrowserOpenURL(href);
}

type Tab = "agent" | "mcp" | "report" | "versions" | "provenance";

const TAB_LABELS: Record<Tab, string> = {
  agent: "Agent",
  mcp: "MCP servers & tools",
  report: "security report",
  versions: "versions",
  provenance: "provenance",
};

export default function AgentPage({
  registry, ns, name, onBack, backLabel = "← back to Browse", onEdit, onOpenReviewKey, openRun = false, openSchedule = false, onFork, onOpenAgent,
}: {
  registry: string;
  ns: string;
  name: string;
  onBack: () => void;
  backLabel?: string;
  /** Edit: the Build tab on the current release,
   *  under this name at its next minor. Offered where the registry says
   *  this sign-in may add a version (canPublish). */
  onEdit?: () => void;
  /** Settings at the AI Review key, for Edit with AI on this computer. */
  onOpenReviewKey?: () => void;
  /** Open with the run setup open, once the release is read: a pin's Run
   *  on Home. */
  openRun?: boolean;
  /** With openRun: the setup opens with When on Repeat (Schedule a new
   *  run). */
  openSchedule?: boolean;
  /** Fork, given only while this registry is signed
   *  in: called with the fork the registry made, which App follows in the
   *  Publisher section. */
  onFork?: (namespace: string, name: string, sentence: string) => void;
  /** Another QuickAgent's page: a fork's origin, from its Fork of line. */
  onOpenAgent?: (ns: string, name: string) => void;
}) {
  const [detail, setDetail] = useState<app.AgentDetailView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>("agent");
  const [forkOpen, setForkOpen] = useState(false);
  // Bumped when a fork's copy ends, so the page is read again whole (its
  // first version, readme and report arrive with it).
  const [rev, setRev] = useState(0);
  const [readme, setReadme] = useState<string | null>(null);
  // The release's instruction file, under the README on the Agent tab.
  const [instructions, setInstructions] = useState<app.InstructionsView | null>(null);
  const [instructionsErr, setInstructionsErr] = useState<string | null>(null);
  const [report, setReport] = useState<app.ReportView | null>(null);
  const [reportErr, setReportErr] = useState<string | null>(null);
  const [prov, setProv] = useState<trust.AttestationResult | null>(null);
  // The coding-agent export (the wizard) is the one-off behind a link;
  // the primary Install is one click into the runner home.
  const [wizardOpen, setWizardOpen] = useState(false);
  // The link is named for the coding agent Settings → Coding agents holds
  // and opens the wizard on the export it implies;
  // read once, since nothing on this page changes that setting.
  const [codingAgents, setCodingAgents] = useState<app.CodingAgentsView | null>(null);
  useEffect(() => {
    SettingsAPI.CodingAgents().then(setCodingAgents).catch(() => setCodingAgents(null));
  }, []);
  const [installBusy, setInstallBusy] = useState(false);
  const [installError, setInstallError] = useState<string | null>(null);
  // What a successful one-click install did not do (the coding-agent
  // export it could not write): a page with no result screen says it here.
  const [installWarning, setInstallWarning] = useState<string | null>(null);
  const [runOpen, setRunOpen] = useState(false);
  // A pin's Run opens the run setup once, when there is a version to run.
  const [ranOnOpen, setRanOnOpen] = useState(false);
  // The setup opened by Schedule a new run starts on Repeat; one opened by
  // Run later does not.
  const [asSchedule, setAsSchedule] = useState(false);
  const runnable = !!detail?.latest;
  useEffect(() => {
    if (openRun && runnable && !ranOnOpen) { setRanOnOpen(true); setRunOpen(true); setAsSchedule(openSchedule); }
  }, [openRun, runnable, ranOnOpen, openSchedule]);
  // Pin to Home, offered while this QuickAgent is installed here.
  const [homeLayout, setHomeLayout, rereadHomeLayout] = useHomeLayout();
  // Edit replaces the Build tab's unsaved form, so it asks first when the
  // tab holds one (as the site asks before replacing an unsent draft).
  const [confirmEdit, setConfirmEdit] = useState<string | null>(null);
  // Edit with AI's form, offered beside Edit.
  const [reviseOpen, setReviseOpen] = useState(false);
  function edit() {
    const held = heldForm();
    if (held) setConfirmEdit(held);
    else onEdit?.();
  }
  const [toolsOpen, setToolsOpen] = useState(false);

  // What THIS machine has accepted for this agent (shared install state) —
  // the installed strip below, with uninstall in place.
  const [installs, setInstalls] = useState<install.Item[]>([]);
  const [confirmRemove, setConfirmRemove] = useState<install.Item | null>(null);
  const [removeMsg, setRemoveMsg] = useState<string | null>(null);
  const refreshInstalls = useCallback(() =>
    InstallAPI.ListInstalled()
      // Only what this app can uninstall and that is still on disk — the
      // runner-home install among them; gone directories
      // belong to the Library's other section.
      .then((all) => setInstalls(all.filter((r) => r.namespace === ns && r.name === name && r.manageable && r.present)))
      .catch(() => setInstalls([])),
  [ns, name]);
  useEffect(() => { void refreshInstalls(); }, [refreshInstalls]);

  // The runner-home record for this QuickAgent, if any: what the primary
  // button reads to say Install, Installing…, or Update. Whether Update is
  // enabled is the check's answer: opening the
  // page asks the record's registry once, with the one "newer" rule the
  // app has (semver, through the record's origin) — never a string compare
  // against the version this page happens to show.
  const runnerInstall = installs.find((r) => r.target === "runner");
  // The install a pin from this page names: the one from this page's
  // registry, else the runner home's, else any.
  const pinnable = installs.find((r) => !r.local && r.origin.replace(/\/+$/, "") === registry.replace(/\/+$/, "")) ?? runnerInstall ?? installs[0];
  const installedVersion = runnerInstall?.version;
  const [runnerUpdate, setRunnerUpdate] = useState<install.UpdateInfo | null>(null);
  const [runnerUpdateErr, setRunnerUpdateErr] = useState<string | null>(null);
  useEffect(() => {
    setRunnerUpdate(null);
    setRunnerUpdateErr(null);
    if (!installedVersion) return;
    InstallAPI.CheckUpdate(ns, name)
      .then((u) => { setRunnerUpdate(u); setRunnerUpdateErr(u?.error ?? null); })
      .catch((e) => setRunnerUpdateErr(String(e)));
  }, [ns, name, installedVersion]);

  // One click: resolve, verify, lay down in the runner home. The
  // gates — an unsigned or unscanned release, a contract to accept — are
  // native dialogs raised by the backend; a refusal or a failure is shown
  // here, in place, and the button goes back to Install.
  async function installOneClick() {
    if (!detail?.latest) return;
    setInstallBusy(true);
    setInstallError(null);
    setInstallWarning(null);
    try {
      // An update goes to the version the check named, from the registry
      // the check asked (the record's origin); a first install to the
      // latest this page shows, from this page's registry.
      const update = runnerInstall && runnerUpdate?.updateAvailable;
      const res = await InstallAPI.InstallRunner(update ? runnerInstall.origin : registry, `${ns}/${name}@${update ? runnerUpdate.latest : detail.latest}`);
      if (res.warning) setInstallWarning(res.warning);
      // Awaited, so the button reads Update as soon as it is enabled again
      // rather than flashing Install first.
      await refreshInstalls();
    } catch (e) {
      setInstallError(String(e));
    } finally {
      setInstallBusy(false);
    }
  }

  async function uninstall(rec: install.Item) {
    setConfirmRemove(null);
    try {
      const msg = await InstallAPI.Uninstall(
        new installed.Key({ registry: rec.registry, namespace: rec.namespace, name: rec.name, target: rec.target, dir: rec.dir }),
      );
      setRemoveMsg(msg);
      refreshInstalls();
      rereadHomeLayout();
    } catch (e) {
      setRemoveMsg(String(e));
    }
  }

  useEffect(() => {
    setDetail(null);
    setError(null);
    setReadme(null);
    setInstructions(null);
    setInstructionsErr(null);
    setReport(null);
    setProv(null);
    BrowseAPI.Agent(registry, ns, name).then(async (d) => {
      setDetail(d);
      if (d.latest) {
        BrowseAPI.Readme(registry, ns, name, d.latest).then(setReadme).catch(() => setReadme(""));
        BrowseAPI.Instructions(registry, ns, name, d.latest).then(setInstructions).catch((e) => setInstructionsErr(String(e)));
        BrowseAPI.Report(registry, ns, name, d.latest).then(setReport).catch((e) => setReportErr(String(e)));
        BrowseAPI.Provenance(registry, ns, name, d.latest, d.latestDigest ?? "", d.latestScanStatus ?? "").then(setProv).catch(() => {});
      }
    }).catch((e) => setError(String(e)));
  }, [registry, ns, name, rev]);

  // A fork being copied: the page updates by itself, as the site's does,
  // every four seconds until the copy is ready or failed. A read that does
  // not answer stops it; the page stays as it was.
  useEffect(() => {
    if (detail?.agent.forkState !== "copying") return;
    const t = setTimeout(() => {
      BrowseAPI.Agent(registry, ns, name).then((d) => {
        if (d.agent.forkState === "copying") setDetail(d);
        else setRev((n) => n + 1);
      }).catch(() => {});
    }, 4000);
    return () => clearTimeout(t);
  }, [detail, registry, ns, name]);

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
  // The one primary button: Install, then Installing…, then Update
  // — enabled only when the check named a newer version.
  let installButton: React.ReactNode;
  if (installBusy) {
    installButton = <Button variant="primary" disabled>Installing…</Button>;
  } else if (!runnerInstall) {
    installButton = <Button variant="primary" onClick={installOneClick}>Install {detail.latest}</Button>;
  } else if (runnerUpdate?.updateAvailable) {
    installButton = <Button variant="primary" onClick={installOneClick} title={`installed ${runnerInstall.version}`}>Update to {runnerUpdate.latest}</Button>;
  } else {
    installButton = <Button variant="primary" disabled title={runnerUpdateErr ? `${runnerInstall.version} is installed on this machine; the registry could not be asked: ${runnerUpdateErr}` : `${runnerInstall.version} is installed on this machine; nothing newer is known`}>Update</Button>;
  }
  const counts = report?.findingCounts ?? {};
  const totalFindings = Object.values(counts).reduce((n, v) => n + v, 0);
  const codingAgent = codingAgentName(codingAgents);
  const copyNotice = forkCopyNotice(a);
  const instructionsLine = instructionsNote(instructions, instructionsErr);

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
                {onEdit && offersEdit(detail) && <Button onClick={edit} title={`Open ${detail.latest} in the Build tab, to publish its next version`}>Edit</Button>}
                {onEdit && offersEdit(detail) && <Button onClick={() => setReviseOpen(true)} title={`Have the Drafter draft the next version of ${detail.latest}`}>Edit with AI</Button>}
                {installButton}
                {onFork && forkable(a, !!detail.latest, true) && (
                  <Button onClick={() => setForkOpen(true)} title={`Make your own private copy of ${a.namespace}/${a.name}`}>Fork</Button>
                )}
                {pinnable && <PinButton agentRef={`${ns}/${name}`} registry={pinnable.local ? "" : pinnable.origin} layout={homeLayout} onLayout={setHomeLayout} />}
              </>
            )
            : <span className="badge">no published version</span>}
        </>}
      />
      <div className="row" style={{ margin: "0 0 8px" }}>
        <CapabilityBadges caps={detail.latestCapabilities} />
        <SignedBadge att={prov} />
        {detail.latest && (
          <Button variant="link" onClick={() => setWizardOpen(true)} disabled={installBusy}>Install into {codingAgent}…</Button>
        )}
      </div>
      {/* A fork's origin and its copy, and a public QuickAgent's fork count:
          the site's lines. */}
      {a.forkedFrom && (
        <p className="small" data-fork-of>
          {a.forkedFrom.available && onOpenAgent
            ? <Button variant="link" onClick={() => onOpenAgent(a.forkedFrom!.namespace, a.forkedFrom!.name)}>{forkOfLabel(a)}</Button>
            : <>{forkOfLabel(a)}{!a.forkedFrom.available && ` ${ORIGIN_GONE}`}</>}
          . {FORK_STAYS_PRIVATE}
        </p>
      )}
      {copyNotice && (
        <Notice tone={copyNotice.failed ? "warn" : "info"}>
          <strong>{copyNotice.title}</strong> {copyNotice.text}
        </Notice>
      )}
      {forkCountLine(a.forkCount) && <p className="muted small" data-fork-count>{forkCountLine(a.forkCount)}</p>}
      {installBusy && <p className="muted small">Resolving, verifying and installing into the runner home…</p>}
      {runnerInstall && runnerUpdateErr && !installBusy && <p className="muted small">Installed {runnerInstall.version}; could not check for a newer version: {runnerUpdateErr}</p>}
      <ErrorBox error={installError} />
      {installWarning && <Notice tone="warn">{installWarning}</Notice>}

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

      {forkOpen && onFork && detail.latest && (
        <ForkDialog registry={registry} ns={a.namespace} name={a.name} version={detail.latest}
          onClose={() => setForkOpen(false)}
          onForked={(fns, fname, sentence) => { setForkOpen(false); onFork(fns, fname, sentence); }} />
      )}
      {reviseOpen && detail.latest && (
        <ReviseDialog registry={registry} ns={a.namespace} name={a.name} version={detail.latest}
          onClose={() => setReviseOpen(false)} onOpenSettings={onOpenReviewKey} />
      )}

      {confirmEdit && (
        <Modal title={`Edit ${a.namespace}/${a.name}?`} onClose={() => setConfirmEdit(null)}>
          <p>
            The Build tab&apos;s form holds <span className="mono">{confirmEdit}</span>. Edit replaces it with{" "}
            {a.namespace}/{a.name} {detail.latest}. Files you saved stay where you saved them.
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setConfirmEdit(null)}>Cancel</Button>
            <Button variant="primary" onClick={() => { setConfirmEdit(null); onEdit?.(); }}>Replace it</Button>
          </div>
        </Modal>
      )}

      {confirmRemove && (
        <Modal title={`Remove ${confirmRemove.namespace}/${confirmRemove.name}?`} onClose={() => setConfirmRemove(null)}>
          <p>
            {confirmRemove.target === "runner"
              ? <>This removes <span className="mono selectable">{confirmRemove.dir}</span> — the runner home's copy of this QuickAgent — and its record. The next Run fetches it again.</>
              : confirmRemove.managed
                ? <>This removes <span className="mono selectable">{confirmRemove.dir}</span> and its host-config entry.</>
                : <>You chose this directory, so its files are <strong>left in place</strong>; only the install record is removed.</>}
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setConfirmRemove(null)}>Cancel</Button>
            <Button variant="danger" onClick={() => uninstall(confirmRemove)}>Remove</Button>
          </div>
        </Modal>
      )}

      <div className="tabs">
        {(["agent", "mcp", "report", "versions", "provenance"] as Tab[]).map((t) => (
          <button key={t} className={`tab ${tab === t ? "active" : ""}`} onClick={() => setTab(t)}>
            {t === "report" ? `security report${report ? ` (${totalFindings})` : ""}` : TAB_LABELS[t]}
          </button>
        ))}
      </div>

      {tab === "agent" && (
        <>
          <Card>
            {readme === null && <Spinner />}
            {readme === "" && <div className="empty">No README.</div>}
            {/* Server-sanitized HTML: the API strips scripts/handlers
                and marks invisible characters; the client never parses raw
                markdown from the archive. The window's CSP (internal/webview)
                is the second layer. Links never navigate this window: an
                http(s) link opens in the OS browser, anything else is
                inert — the page that runs here has the machine as its API. */}
            {readme && (
              <div className="readme selectable" onClick={openReadmeLinkOutside} dangerouslySetInnerHTML={{ __html: readme }} />
            )}
          </Card>
          {detail.latest && (
            <Card className="agent-instructions">
              {/* The path is the author's manifest's, so it is revealed too. */}
              <h3 className="mono selectable">{instructions ? <RevealText text={instructions.path} /> : "Instruction file"}</h3>
              <p className="muted small">What the agent is told to do, as the release has it.</p>
              {!instructions && !instructionsErr && <Spinner />}
              {instructionsLine && <p className="muted small" data-instructions-note><RevealText text={instructionsLine} /></p>}
              {/* The file as text, never rendered: React escapes it,
                  its links are not live, and every invisible character is
                  drawn as a chip (the GlassWorm attack). */}
              {instructions && !instructions.binary && instructions.content.trim() && (
                <pre className="instructions-text mono selectable"><RevealText text={instructions.content} /></pre>
              )}
            </Card>
          )}
        </>
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
                  The registry admitted this version without a scan: no findings, no report and no attestation stand
                  behind it. Its organization&apos;s administrator took the QuickAgent off the pipeline, and no scanner
                  looked at its contents. Installing or running it asks for its own consent; accepting an unsigned release
                  does not cover it.
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
          presetTarget={codingAgents?.export.target}
          presetDir={codingAgents?.export.target === "agents-md" ? codingAgents.export.dir : undefined}
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
          summary={a.summary}
          attestation={prov}
          initialWhen={asSchedule ? "repeat" : undefined}
          onClose={() => { setRunOpen(false); setAsSchedule(false); }}
        />
      )}
    </div>
  );
}

/** The "MCP servers & tools" section: the
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
  const [info, setInfo] = useState<app.MCPView | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!hasPublished) return;
    setInfo(null);
    setError(null);
    RunAPI.GatewayRequirements(registry, ns, name).then(setInfo).catch((e) => setError(String(e)));
  }, [registry, ns, name, hasPublished]);

  if (!hasPublished) return <div className="empty">No published version.</div>;
  if (error) return <ErrorBox error={error} />;
  if (!info) return <Spinner label="Working out the tools this version grants…" />;

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
      {info.servers.map((s, i) => {
        // Servers ends with one line per declared server, in order
        // (services_run.go MCPView): a declared server's env goes beneath
        // its own line.
        const declared = info.declaredServers ?? [];
        const d = declared[i - (info.servers.length - declared.length)];
        return (
          <div key={s}>
            <p style={{ margin: "4px 0" }}>{s}</p>
            {d && <ServerEnv env={d.env} />}
          </div>
        );
      })}
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
                    {v.rating ? <> <Sev severity={v.rating} /></> : null}
                    {v.error ? <span className="muted small"> {v.error}</span> : null}
                  </td>
                  <td className="small">
                    {(v.advisories ?? []).map((a) => (
                      <div key={a.id}>
                        <span className="mono">{a.id}</span> {advisoryWords(a)}
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
                Runs are refused by default: {info.refusal}. In Unsafe Mode the run panel asks your
                consent for the run, and the receipt names it.
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
            The runner&apos;s guided setup collects these once into the QuickAgent&apos;s MCP
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
  report: app.ReportView | null;
  error: string | null;
  version: string;
}) {
  if (error) return <ErrorBox error={error} />;
  if (!version) return <div className="empty">No published version to report on.</div>;
  if (!report) return <Spinner label="Loading report…" />;

  const counts = report.findingCounts ?? {};
  const total = Object.values(counts).reduce((n, v) => n + v, 0);
  const order = ["critical", "high", "medium", "low", "info"];

  // Unscanned: the registry admitted the version without a
  // scan, and an empty report must never read as "scanned, 0 findings".
  if (report.verdict === "unscanned" || report.scanStatus === "skipped") {
    return (
      <div className="notice notice-unscanned mark-unscanned">
        <div className="row">
          <span className="badge unscanned">unscanned</span>
          <strong>The registry admitted this version without a scan.</strong>
        </div>
        <p className="muted small" style={{ marginBottom: 0 }}>
          No findings, no report and no attestation stand behind it. Its organization&apos;s administrator took the
          QuickAgent off the pipeline, and no scanner looked at its contents. Installing or running it asks for its own
          consent. Status {report.versionStatus}.
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
