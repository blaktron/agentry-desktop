// The run page (plan desktop-home-and-library.md §4.7, D7, D13; D-068;
// agentry-desktop#659): a run Recent runs lists, opened as a page with the
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
// own place (D13). A folder that has gone is said, with a way to remove
// the entry; it is never looked for.
import { useEffect, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Notice, PageHeader, Spinner } from "../ui";
import { RunResultView } from "./RunResult";
import { HostedResult } from "./HostedRun";
import { builtinPlace, headline, howItEnded, isBuiltin, rowLine, versionOf, whatRan } from "../recentRuns";

export type RunPageSource = "local" | "hosted";

type Tone = "ok" | "bad" | "warn";
const tone = (t?: string): Tone => (t === "ok" || t === "bad" ? t : "warn");

export default function RunPage({ source, id, registry, backLabel, onBack, onOpenPlace }: {
  source: RunPageSource;
  id: string;
  // The registry a run In Agentry is read from.
  registry: string;
  backLabel: string;
  onBack: () => void;
  onOpenPlace: (page: "build" | "knowledge" | "voice") => void;
}) {
  return source === "hosted"
    ? <HostedRunPage id={id} registry={registry} backLabel={backLabel} onBack={onBack} onOpenPlace={onOpenPlace} />
    : <LocalRunPage id={id} backLabel={backLabel} onBack={onBack} onOpenPlace={onOpenPlace} />;
}

function Back({ label, onBack }: { label: string; onBack: () => void }) {
  return <Button variant="link" onClick={onBack}>{label}</Button>;
}

function PlaceButton({ kind, onOpenPlace }: { kind: string; onOpenPlace: (page: "build" | "knowledge" | "voice") => void }) {
  const place = builtinPlace(kind);
  return place ? <Button data-run-place={place.page} onClick={() => onOpenPlace(place.page)}>{place.label}</Button> : null;
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

function LocalRunPage({ id, backLabel, onBack, onOpenPlace }: {
  id: string;
  backLabel: string;
  onBack: () => void;
  onOpenPlace: (page: "build" | "knowledge" | "voice") => void;
}) {
  const [view, setView] = useState<main.RecordedRunView | null>(null);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    let live = true;
    RunAPI.RecordedRun(id).then((v) => live && setView(v)).catch((e) => live && setErr(String(e)));
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
      <PageHeader title={whatRan(row)} sub={rowLine(row, new Date())} actions={<PlaceButton kind={row.kind} onOpenPlace={onOpenPlace} />} />
      {view.folderState === "gone" && <GoneFolder id={row.id} error={view.folderError} onRemoved={onBack} />}
      {view.folderState === "present" ? (
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
function RunFacts({ row, version, folder }: { row: main.RecentRunView; version: string; folder?: string }) {
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

function HostedRunPage({ id, registry, backLabel, onBack, onOpenPlace }: {
  id: string;
  registry: string;
  backLabel: string;
  onBack: () => void;
  onOpenPlace: (page: "build" | "knowledge" | "voice") => void;
}) {
  const [view, setView] = useState<main.HostedRecordView | null>(null);
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
      <PageHeader title={whatRan(row)} sub={`In Agentry · ${run.state || run.status}`} actions={<PlaceButton kind={view.kind} onOpenPlace={onOpenPlace} />} />
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
