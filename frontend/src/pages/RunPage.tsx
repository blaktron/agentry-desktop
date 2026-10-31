// The run page: a run Recent runs lists, opened as a page with the
// result view the run dialog draws, and a way back.
//
// A run on this computer, and a run In Agentry whose output this app
// downloaded, is read by its id through the history Go keeps
// (RunAPI.RecordedRun), never by a path; how it ended is the history's,
// from the engine's result as Go read it, and only the result itself comes
// from its folder, rendered under the hostile rules every result is
// (RunResultView, AgentMarkdown). A run In Agentry is read live from the
// registry (RunAPI.HostedRecord), and one whose output is not here offers
// Download output, which costs no credits. A built-in's job links to its
// own place. A folder that has gone is said, with a way to remove
// the entry; it is never looked for.
import { useEffect, useRef, useState } from "react";
import * as BuildAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/buildapi";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { Button, Card, ErrorBox, Notice, PageHeader, Spinner } from "../ui";
import { RunResultView } from "./RunResult";
import { HostedResult } from "./HostedRun";
import { type BuiltinPage, builtinPlace, headline, howItEnded, isBuiltin, rowLine, versionOf, whatRan } from "../recentRuns";
import type { Tone as JobTone } from "../jobProgress";
import { ReviewReport } from "./ReviewFlow";

export type RunPageSource = "local" | "hosted";

type Tone = Exclude<JobTone, "">;
const tone = (t?: string): Tone => (t === "ok" || t === "bad" ? t : "warn");

export default function RunPage({ source, id, registry, backLabel, onBack, onOpenPlace, onOpenDraft }: {
  source: RunPageSource;
  id: string;
  // The registry a run In Agentry is read from.
  registry: string;
  backLabel: string;
  onBack: () => void;
  onOpenPlace: (page: BuiltinPage) => void;
  onOpenDraft: (draft: app.DraftView) => void;
}) {
  return source === "hosted"
    ? <HostedRunPage id={id} registry={registry} backLabel={backLabel} onBack={onBack} onOpenPlace={onOpenPlace} onOpenDraft={onOpenDraft} />
    : <LocalRunPage registry={registry} id={id} backLabel={backLabel} onBack={onBack} onOpenPlace={onOpenPlace} onOpenDraft={onOpenDraft} />;
}

function Back({ label, onBack }: { label: string; onBack: () => void }) {
  return <Button variant="link" onClick={onBack}>{label}</Button>;
}

function PlaceButton({ kind, source, id, registry, onOpenPlace, onOpenDraft }: {
  kind: string; source: RunPageSource; id: string; registry: string;
  onOpenPlace: (page: BuiltinPage) => void;
  onOpenDraft: (draft: app.DraftView) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const request = useRef(0);
  useEffect(() => () => { request.current++; }, [source, id, registry]);
  const place = builtinPlace(kind);
  async function open() {
    if (!place) return;
    if (kind !== "draft") { onOpenPlace(place.page); return; }
    const attempt = ++request.current;
    setBusy(true);
    setError(null);
    try {
      const draft = source === "hosted"
        ? await BuildAPI.OpenHostedDraft(registry, id)
        : await BuildAPI.OpenRecordedDraft(id);
      if (attempt !== request.current) return;
      if (!draft?.model) throw new Error("This run returned no draft the form can open.");
      onOpenDraft(draft);
    } catch (e) {
      if (attempt === request.current) setError(String(e));
    } finally {
      if (attempt === request.current) setBusy(false);
    }
  }
  return place ? <div>
    <Button data-run-place={place.page} disabled={busy} onClick={() => void open()}>{busy ? "Opening the draft…" : place.label}</Button>
    <ErrorBox error={error} />
  </div> : null;
}

/** A folder that has gone: said, never looked for, and the entry can go. */
function GoneFolder({ id, error, onRemoved }: { id: string; error?: string; onRemoved: () => void }) {
  const [err, setErr] = useState<string | null>(null);
  return (
    <Notice tone="warn">
      <span data-run-folder-gone>{error || "The run's folder is no longer there."}</span> Agentry Desktop does not look for
      it elsewhere. If you moved it, open it from where it is now.
      <div className="row" style={{ marginTop: 8 }}>
        <Button onClick={() => RunAPI.RemoveRecordedRun(id).then(onRemoved).catch((e) => setErr(String(e)))}>
          Remove from Recent runs
        </Button>
      </div>
      <ErrorBox error={err} />
    </Notice>
  );
}

function LocalRunPage({ registry, id, backLabel, onBack, onOpenPlace, onOpenDraft }: {
  registry: string;
  id: string;
  backLabel: string;
  onBack: () => void;
  onOpenPlace: (page: BuiltinPage) => void;
  onOpenDraft: (draft: app.DraftView) => void;
}) {
  const [view, setView] = useState<app.RecordedRunView | null>(null);
  const [err, setErr] = useState<string | null>(null);
  // An Examiner review's report, which is the review's page.
  const [report, setReport] = useState<app.ReviewView | null>(null);
  const [reportErr, setReportErr] = useState<string | null>(null);
  useEffect(() => {
    let live = true;
    RunAPI.RecordedRun(id).then((v) => {
      if (!live) return;
      setView(v);
      if (v.run.kind === "review" && v.folderState === "present") {
        RunAPI.RecordedReview(id).then((r) => live && setReport(r)).catch((e) => live && setReportErr(String(e)));
      }
    }).catch((e) => live && setErr(String(e)));
    return () => { live = false; };
  }, [id]);
  if (err) {
    return (
      <div>
        <Back label={backLabel} onBack={onBack} />
        <PageHeader title="Run" />
        <ErrorBox error={err} />
      </div>
    );
  }
  if (!view) return <div><Back label={backLabel} onBack={onBack} /><Spinner label="Reading the run…" /></div>;
  const row = view.run;
  const hostedLine = row.where === "agentry" ? `${howItEnded(row)}.` : null;
  const version = versionOf(row.ref);
  return (
    <div className="run-page" data-run-page={row.id}>
      <Back label={backLabel} onBack={onBack} />
      <PageHeader title={whatRan(row)} sub={rowLine(row, new Date())} actions={<PlaceButton source="local" id={id} registry={registry} kind={row.kind} onOpenPlace={onOpenPlace} onOpenDraft={onOpenDraft} />} />
      {view.folderState === "gone" && <GoneFolder id={row.id} error={view.folderError} onRemoved={onBack} />}
      <ErrorBox error={reportErr} />
      {report ? (
        <>
          <h3 className={`run-outcome run-outcome-${tone(row.tone)}`}>{headline(row)}</h3>
          <ReviewReport v={report} log={[]} />
          <details className="advanced run-details">
            <summary className="muted small">Run details: the run and its folder</summary>
            <RunFacts row={row} version={version} folder={view.folder} />
          </details>
        </>
      ) : view.folderState === "present" ? (
        <RunResultView runId={row.id} headline={hostedLine ?? headline(row)} tone={tone(row.tone)} resultJson={view.resultJson ?? ""}>
          <RunFacts row={row} version={version} folder={view.folder} />
        </RunResultView>
      ) : (
        <Card>
          <h3 className={`run-outcome run-outcome-${tone(row.tone)}`}>{hostedLine ?? headline(row)}</h3>
          {view.folderState === "none" && (
            <p className="muted small">
              {isBuiltin(row.kind)
                ? "This job keeps no run folder: what it made is kept in its own place."
                : "This run keeps no folder here."}
            </p>
          )}
          <RunFacts row={row} version={version} folder={view.folder} />
        </Card>
      )}
    </div>
  );
}

/** The run's facts under Run details: its id, release and folder. */
function RunFacts({ row, version, folder }: { row: app.RecentRunView; version: string; folder?: string }) {
  return (
    <Card className="selectable">
      <div className="kv"><span className="k">run</span><span className="mono">{row.id}</span></div>
      {row.ref && <div className="kv"><span className="k">release</span><span className="mono">{row.ref}</span></div>}
      {row.registry && <div className="kv"><span className="k">registry</span><span className="mono">{row.registry}</span></div>}
      {folder && <div className="kv"><span className="k">run folder</span><span className="mono">{folder}</span></div>}
      {row.startedAt && <div className="kv"><span className="k">started</span><span>{new Date(row.startedAt).toLocaleString()}</span></div>}
      {row.endedAt && <div className="kv"><span className="k">ended</span><span>{new Date(row.endedAt).toLocaleString()}</span></div>}
    </Card>
  );
}

function HostedRunPage({ id, registry, backLabel, onBack, onOpenPlace, onOpenDraft }: {
  id: string;
  registry: string;
  backLabel: string;
  onBack: () => void;
  onOpenPlace: (page: BuiltinPage) => void;
  onOpenDraft: (draft: app.DraftView) => void;
}) {
  const [view, setView] = useState<app.HostedRecordView | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [downloadErr, setDownloadErr] = useState<string | null>(null);
  useEffect(() => {
    let live = true;
    RunAPI.HostedRecord(registry, id).then((v) => live && setView(v)).catch((e) => live && setErr(String(e)));
    return () => { live = false; };
  }, [registry, id]);
  async function download() {
    setBusy(true);
    setDownloadErr(null);
    try {
      setView(await RunAPI.DownloadHostedOutput(registry, id));
    } catch (e) {
      setDownloadErr(String(e));
    } finally {
      setBusy(false);
    }
  }
  if (err) {
    return (
      <div>
        <Back label={backLabel} onBack={onBack} />
        <PageHeader title="Run In Agentry" />
        <ErrorBox error={err} />
      </div>
    );
  }
  if (!view?.run) return <div><Back label={backLabel} onBack={onBack} /><Spinner label="Reading the run In Agentry…" /></div>;
  const run = view.run;
  const row = { kind: view.kind, ref: run.ref };
  const said = run.outcomeLine || run.statusLine || run.state || run.status;
  const line = `${said.charAt(0).toUpperCase()}${said.slice(1)}.`;
  return (
    <div className="run-page" data-run-page={run.id} data-run-source="hosted">
      <Back label={backLabel} onBack={onBack} />
      <PageHeader title={whatRan(row)} sub={`In Agentry · ${run.state || run.status}`} actions={<PlaceButton source="hosted" id={id} registry={registry} kind={view.kind} onOpenPlace={onOpenPlace} onOpenDraft={onOpenDraft} />} />
      {view.followed && <Notice tone="info">This run is followed in its card, which brings its output here when it ends.</Notice>}
      {view.folderState === "gone" && <GoneFolder id={run.id} error={view.folderError} onRemoved={onBack} />}
      {view.kept ? (
        <RunResultView runId={run.id} headline={line} tone={tone(run.outcomeTone)} resultJson={run.resultJson ?? ""}>
          <HostedResult run={run} />
        </RunResultView>
      ) : (
        <>
          <Card>
            <h3 className={`run-outcome run-outcome-${tone(run.outcomeTone)}`}>{line}</h3>
            {view.noOutput && <p className="small" data-no-output>{view.noOutput.charAt(0).toUpperCase() + view.noOutput.slice(1)}.</p>}
            {view.downloadable && (
              <>
                <p className="small">
                  Its output is not on this computer. Download output brings it into your output folder, for no credits.
                  Agentry keeps a run&apos;s output for 30 days.
                </p>
                <Button variant="primary" disabled={busy} data-download-output onClick={() => void download()}>
                  {busy ? "Downloading…" : "Download output"}
                </Button>
                <ErrorBox error={downloadErr} />
              </>
            )}
          </Card>
          <details className="advanced run-details">
            <summary className="muted small">Run details: the badges, the credits, the receipt</summary>
            <HostedResult run={run} />
          </details>
        </>
      )}
    </div>
  );
}
