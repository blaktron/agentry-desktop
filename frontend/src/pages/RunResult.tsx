// A finished run's result, first: how it ended in a sentence, the agent's final
// message rendered, the files it wrote with their previews, and Show in
// folder. Everything else about the run (the badges, the usage, the
// receipt, the paths, the log) is under Run details, collapsed.
//
// The same view serves a run on this machine, an ended session and a run
// In Agentry, whose outputs have already been unpacked into its folder.
// The files come from RunAPI.RunOutputs, which lists the run's folder by
// its run id and never a path the page gives it; nothing here opens a file
// a run wrote: a preview is text, and Show in folder opens the
// folder. One exception, PDFs the run exported: Go holds each
// PDF to the envelope's pdfs record, and one it lists with the same SHA-256
// opens in the app's viewer (PdfViewer) or, as a fresh copy, in the
// system's; any other PDF is Save only.
import { useEffect, useState } from "react";
import * as RunAPI from "../../wailsjs/go/app/RunAPI";
import { app } from "../../wailsjs/go/models";
import { AgentMarkdown } from "../AgentMarkdown";
import { RevealText, plainName } from "../reveal";
import { Button, Card, ErrorBox, fmtBytes, shortDigest } from "../ui";
import { localUseLine } from "../usage";
import { PdfViewer } from "../PdfViewer";
import { pdfActions } from "../reportPdf";

export function RunResultView({
  runId, headline, tone, error, resultJson, knowledge, decode, unsafeExec, ask, children,
}: {
  runId: string;
  headline: string;
  tone: "ok" | "bad" | "warn";
  error?: string;
  /** The run/v1 envelope's result, as JSON text. */
  resultJson: string;
  /** The receipt's knowledge line: the packs this run could search, and how
   *  they were mounted; unset when none was. */
  knowledge?: string;
  /** The receipt's decode line: how the input files were read, and
   *  what ClamAV and the Hybrid Analysis lookup made of each; unset for a
   *  run with no files. */
  decode?: string;
  /** The receipt's exec line of an Unsafe Mode run:
   *  what was installed from where, and what was not; unset otherwise. */
  unsafeExec?: string;
  /** An ask after the result, before Run details: a hosted run's star ask. */
  ask?: React.ReactNode;
  /** Run details, drawn under the result. */
  children?: React.ReactNode;
}) {
  const [outputs, setOutputs] = useState<app.RunOutputsView | null>(null);
  const [outputsErr, setOutputsErr] = useState<string | null>(null);
  const [folderErr, setFolderErr] = useState<string | null>(null);

  useEffect(() => {
    if (!runId) return;
    let live = true;
    RunAPI.RunOutputs(runId)
      .then((v) => live && setOutputs(v))
      .catch((e) => live && setOutputsErr(String(e)));
    return () => {
      live = false;
    };
  }, [runId]);

  const files = outputs?.files ?? [];
  return (
    <div className="run-result-view">
      <h3 className={`run-outcome run-outcome-${tone}`}>{headline}</h3>
      {error && <ErrorBox error={error} />}

      <section aria-labelledby="run-result-message">
        <h4 id="run-result-message">The result</h4>
        <ResultBody raw={resultJson} />
      </section>

      {knowledge && (
        <section aria-labelledby="run-result-knowledge">
          <h4 id="run-result-knowledge">Knowledge this run could search</h4>
          <p className="mono small selectable">{knowledge}</p>
        </section>
      )}

      {decode && (
        <section aria-labelledby="run-result-decode">
          <h4 id="run-result-decode">How your files were read and checked</h4>
          <p className="mono small selectable" data-decode>{decode}</p>
        </section>
      )}

      {/* The line names every package the build added, hundreds for a tool
          like Ghidra, so it is said in a sentence and read in a box of its
          own height. */}
      {unsafeExec && (
        <section aria-labelledby="run-result-unsafe">
          <h4 id="run-result-unsafe"><span className="badge bad">Unsafe Mode</span> This run&apos;s commands</h4>
          <p className="small">
            This run was compiled in Unsafe Mode: its commands were installed from Kali Linux&apos;s repository or PyPI, and
            this computer&apos;s approved list was not asked.{" "}
            {unsafeExec.includes("NOT installed at image build")
              ? <strong>At least one did not install, and the run went on without it.</strong>
              : "Every package it asked for installed."}{" "}
            The receipt&apos;s line, below, names each command&apos;s source and every package the build added.
          </p>
          <pre className="run-log selectable" data-unsafe-exec>{unsafeExec}</pre>
        </section>
      )}

      <section aria-labelledby="run-result-files">
        <div className="spread">
          <h4 id="run-result-files">Files in the run folder</h4>
          {runId && (
            <Button onClick={() => RunAPI.ShowRunFolder(runId).catch((e) => setFolderErr(String(e)))}>
              Show in folder
            </Button>
          )}
        </div>
        <ErrorBox error={folderErr} />
        <ErrorBox error={outputsErr} />
        <p className="muted small">What is under the folder&apos;s outputs/ and reports/: if the folder was used before, earlier files are here too.</p>
        {outputs && files.length === 0 && <p className="muted small">There is nothing in outputs/ or reports/.</p>}
        {files.map((f) => f.pdf ? (
          <PdfFile key={f.path} runId={runId} file={f} />
        ) : (
          <div key={f.path} className="run-file">
            <div className="run-file-head">
              <span className="mono small">
                <RevealText text={f.path} />
              </span>
              <span className="muted small">{fmtBytes(f.bytes)}</span>
            </div>
            <div className="run-file-body" aria-label={`Preview of ${plainName(f.path)}`}>
              {f.previewKind === "markdown" ? (
                <AgentMarkdown text={f.preview ?? ""} />
              ) : f.previewKind === "text" ? (
                <pre className="run-result selectable">
                  <RevealText text={f.preview ?? ""} />
                </pre>
              ) : (
                <p className="muted small">
                  {f.noPreview === "large"
                    ? "Too large to preview here. Show in folder to find it."
                    : "Not text, so there is no preview. Show in folder to find it."}
                </p>
              )}
            </div>
          </div>
        ))}
        {(outputs?.skipped ?? 0) > 0 && (
          <p className="muted small">
            {outputs!.skipped === 1 ? "One entry" : `${outputs!.skipped} entries`} in the folder{" "}
            {outputs!.skipped === 1 ? "is" : "are"} not shown: a link, something that is not a regular file, one that
            could not be read, or one past the list's limit{outputs!.truncated ? " (the list stopped at its limit)" : ""}.
          </p>
        )}
      </section>

      {ask}

      <details className="advanced run-details">
        <summary className="muted small">Run details: the badges, the usage, the receipt and the paths</summary>
        {children}
      </details>
    </div>
  );
}

/** A PDF the run left: View (the app's viewer), Open in system viewer and
 *  Save for one the run's PDF export made; Save alone for any other. */
function PdfFile({ runId, file }: { runId: string; file: app.OutputFileView }) {
  const [viewing, setViewing] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const a = pdfActions(file.pdf!);
  const name = plainName(file.path);
  return (
    <div className="run-file" data-pdf={file.pdf!.exported ? "exported" : "other"}>
      <div className="run-file-head">
        <span className="mono small">
          <RevealText text={file.path} />
        </span>
        <span className="row nowrap">
          <span className="muted small">{fmtBytes(file.bytes)}</span>
          {a.view && (
            <Button variant="primary" onClick={() => setViewing((v) => !v)} aria-expanded={viewing} aria-label={`${viewing ? "Hide" : "View"} ${name}`}>
              {viewing ? "Hide" : "View"}
            </Button>
          )}
          {a.openInSystemViewer && (
            <Button variant="primary" onClick={() => { setErr(null); RunAPI.OpenRunPDF(runId, file.path).catch((e) => setErr(String(e))); }} aria-label={`Open ${name} in the system viewer`}>
              Open in system viewer
            </Button>
          )}
          {a.save && (
            <Button variant="primary" onClick={() => { setErr(null); setSaved(null); RunAPI.SaveRunPDF(runId, file.path).then((p) => p && setSaved(p)).catch((e) => setErr(String(e))); }} aria-label={`Save ${name}`}>
              Save
            </Button>
          )}
        </span>
      </div>
      <div className="run-file-body">
        <p className="muted small">{a.line}</p>
        {saved && <p className="small">Saved to <span className="mono selectable">{saved}</span>.</p>}
        <ErrorBox error={err} />
        {viewing && runId && <PdfViewer runId={runId} path={file.path} name={name} />}
      </div>
    </div>
  );
}

/** The envelope's result: a string rendered as markdown, structured JSON
 * pretty-printed, and a sentence when there is none. A local run's view
 * carries the result itself; a run In Agentry's carries its whole
 * result.json, the run/v1 envelope, whose `result` is the part shown. */
function ResultBody({ raw }: { raw: string }) {
  if (!raw) return <p className="muted small">The run ended without a final message.</p>;
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
    if (typeof parsed === "object" && parsed !== null && (parsed as { agentry?: unknown }).agentry === "run/v1") {
      parsed = (parsed as { result?: unknown }).result ?? null;
    }
  } catch {
    return (
      <pre className="run-result selectable">
        <RevealText text={raw} />
      </pre>
    );
  }
  if (typeof parsed === "string") {
    if (parsed.trim() === "") return <p className="muted small">The run ended without a final message.</p>;
    return <AgentMarkdown text={parsed} />;
  }
  if (parsed === null) return <p className="muted small">The run ended without a final message.</p>;
  return (
    <pre className="run-result selectable">
      <RevealText text={JSON.stringify(parsed, null, 2)} />
    </pre>
  );
}


/** A run on this computer's Run details, under its result: the envelope's
 *  badges and usage, its folder, receipts and plan digest, the operator
 *  policy it enforced, and its log.
 *  The run's flow and a chat's result both draw it. */
export function LocalRunDetails({ result, log }: { result: app.RunResultView; log: string[] }) {
  return (
    <>
      <Card className="selectable">
        <div className="row">
          {result.status === "ok"
            ? <span className="badge ok">ok</span>
            : <span className="badge bad">error (exit {result.exitCode})</span>}
          <span className={`badge ${result.attestationState === "verified" ? "ok" : "warn"}`}>
            attestation: {result.attestationState}
          </span>
          <span className="muted small">{localUseLine(result)}</span>
        </div>
        <div className="kv"><span className="k">output folder</span><span className="mono">{result.outDir}</span></div>
        <div className="kv"><span className="k">receipts</span><span className="mono">{result.receiptsDir}</span></div>
        <div className="kv"><span className="k">run plan sha256</span><span className="mono" title={result.runPlanSha}>{result.runPlanSha.slice(0, 12)}…</span></div>
        {/* The operator policy the run enforced: which file, and how its path was
            chosen, since a relocated runner home is inherited
            silently from the environment. */}
        {result.policy && (
          <>
            <div className="kv"><span className="k">operator policy</span><span className="mono">{result.policy.path}</span></div>
            <div className="kv"><span className="k">policy chosen by</span><span>{policySourceLabel(result.policy.source)}</span></div>
            {result.policy.sha256 && (
              <div className="kv"><span className="k">policy digest</span><span className="mono" title={result.policy.sha256}>{shortDigest(result.policy.sha256)}</span></div>
            )}
          </>
        )}
      </Card>
      {log.length > 0 && (
        <>
          <h4>Run log</h4>
          <pre className="run-log selectable"><RevealText text={log.join("\n")} /></pre>
        </>
      )}
    </>
  );
}

// policySourceLabel says how the run's policy path was chosen, from the
// envelope's policy.source. The desktop has no --policy, so "flag" is never its
// own; an unknown source is shown as is.
function policySourceLabel(source: string): string {
  switch (source) {
    case "default": return "the runner home's policy.yaml (the default)";
    case "env": return "AGENTRY_RUNNER_HOME, set in the environment this app started from";
    default: return source;
  }
}
