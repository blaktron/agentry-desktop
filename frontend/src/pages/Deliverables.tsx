// The files a run was for, drawn the same on a run's result and on Home's
// Deliverables card. Go lists them (RunAPI.RunDeliverables,
// RecentDeliverables), each only after its bytes hash to the run's record,
// and does every action: this draws a row and its three buttons.
//
//   - Open hands the type's own program a copy Go checked (a type on a short
//     list, its bytes what the type says); a type that is on no list has no
//     Open and says why.
//   - View draws a PDF in the app's viewer, Markdown with raw HTML skipped,
//     and text or CSV as plain text, all from bytes Go just checked.
//   - Show in folder shows the file in the file manager and opens nothing.
import { useEffect, useState } from "react";
import * as RunAPI from "../../wailsjs/go/app/RunAPI";
import { AgentMarkdown } from "../AgentMarkdown";
import { PdfViewer } from "../PdfViewer";
import { actions, copiedLine, heading, typeLabel, type DeliverableFacts } from "../deliverables";
import { RevealText, plainName } from "../reveal";
import { Button, ErrorBox, fmtBytes } from "../ui";

export type DeliverableFile = DeliverableFacts & { path: string };

function Viewer({ runId, file }: { runId: string; file: DeliverableFile }) {
  const [text, setText] = useState<{ kind: string; text: string } | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const pdf = file.viewKind === "pdf";
  useEffect(() => {
    if (pdf) return;
    let live = true;
    RunAPI.RunDeliverableText(runId, file.path)
      .then((v) => live && setText({ kind: v.kind, text: v.text }))
      .catch((e) => live && setErr(String(e)));
    return () => { live = false; };
  }, [runId, file.path, pdf]);
  if (pdf) {
    return <PdfViewer runId={runId} path={file.path} name={plainName(file.name)} load={RunAPI.RunDeliverablePDF} />;
  }
  if (err) return <ErrorBox error={err} />;
  if (!text) return <p className="muted small">Reading it…</p>;
  return (
    <div className="run-file-body" aria-label={`The text of ${plainName(file.name)}`}>
      {text.kind === "markdown"
        ? <AgentMarkdown text={text.text} />
        : <pre className="run-result selectable"><RevealText text={text.text} /></pre>}
    </div>
  );
}

export function DeliverableItem({ runId, file, on, compact = false, context }: {
  runId: string;
  file: DeliverableFile;
  /** The Deliverables switch, for the line that says where the file is. */
  on: boolean;
  /** A row of Home's card: link-style buttons, no copy line. */
  compact?: boolean;
  /** What a card row adds under the title (the QuickAgent and when). */
  context?: React.ReactNode;
}) {
  const [viewing, setViewing] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const a = actions(file);
  const name = plainName(file.name);
  const run = (f: () => Promise<void>) => { setErr(null); f().catch((e) => setErr(String(e))); };
  const btn = (label: string, onClick: () => void, extra: Record<string, string | boolean> = {}, primary = false) =>
    compact
      ? <button type="button" className="link build-way-part" onClick={onClick} {...extra}>{label}</button>
      : <Button variant={primary ? "primary" : "default"} onClick={onClick} {...extra}>{label}</Button>;
  const buttons = (
    <span className={compact ? "pinned-actions small" : "row nowrap"}>
      {a.open && btn("Open", () => run(() => RunAPI.OpenRunDeliverable(runId, file.path)), { "aria-label": `Open ${name}` }, true)}
      {a.view && btn(viewing ? "Hide" : "View", () => setViewing((v) => !v), { "aria-expanded": viewing, "aria-label": `${viewing ? "Hide" : "View"} ${name}` })}
      {btn("Show in folder", () => run(() => RunAPI.ShowRunDeliverable(runId, file.path)), { "aria-label": `Show ${name} in its folder` })}
    </span>
  );
  return (
    <div className={compact ? "deliverable-row" : "run-file"} data-deliverable={file.path} data-deliverable-open={a.open ? "" : undefined}>
      <div className={compact ? undefined : "run-file-head"}>
        <span>
          <strong><RevealText text={heading(file)} /></strong>{" "}
          <span className="muted small mono"><RevealText text={file.name} /></span>
        </span>
        {!compact && <span className="row nowrap">
          <span className="badge">{typeLabel(file.type)}</span>
          <span className="muted small">{fmtBytes(file.bytes)}</span>
          {buttons}
        </span>}
      </div>
      {compact && <div className="muted small">{typeLabel(file.type)} · {fmtBytes(file.bytes)}{context ? <> · {context}</> : null}</div>}
      {compact && buttons}
      {!compact && (
        <div className="run-file-body">
          <p className="muted small" data-deliverable-copy>{copiedLine(file, on)}</p>
          {a.note && <p className="small" data-deliverable-note>{a.note}</p>}
        </div>
      )}
      {compact && a.note && <p className="muted small" data-deliverable-note>{a.note}</p>}
      <ErrorBox error={err} />
      {viewing && <Viewer runId={runId} file={file} />}
    </div>
  );
}
