// A finished run's result, first (plan guided-runs.md §5, M4;
// agentry-desktop#227): how it ended in a sentence, the agent's final
// message rendered, the files it wrote with their previews, and Show in
// folder. Everything else about the run (the badges, the usage, the
// receipt, the paths, the log) is under Run details, collapsed.
//
// The same view serves a run on this machine, an ended session and a run
// In Agentry, whose outputs have already been unpacked into its folder.
// The files come from RunAPI.RunOutputs, which lists the run's folder by
// its run id and never a path the page gives it; nothing here opens a file
// a run wrote (plan D8): a preview is text, and Show in folder opens the
// folder.
import { useEffect, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { main } from "../../wailsjs/go/models";
import { AgentMarkdown } from "../AgentMarkdown";
import { RevealText, plainName } from "../reveal";
import { Button, ErrorBox, fmtBytes } from "../ui";

export function RunResultView({
  runId, headline, tone, error, resultJson, knowledge, children,
}: {
  runId: string;
  headline: string;
  tone: "ok" | "bad" | "warn";
  error?: string;
  /** The run/v1 envelope's result, as JSON text. */
  resultJson: string;
  /** The receipt's knowledge line: the packs this run could search, and how
   *  they were mounted (plan knowledge.md §4.5); unset when none was. */
  knowledge?: string;
  /** Run details: what the view used to show first. */
  children?: React.ReactNode;
}) {
  const [outputs, setOutputs] = useState<main.RunOutputsView | null>(null);
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
        {files.map((f) => (
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

      <details className="advanced run-details">
        <summary className="muted small">Run details: the badges, the usage, the receipt and the paths</summary>
        {children}
      </details>
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

