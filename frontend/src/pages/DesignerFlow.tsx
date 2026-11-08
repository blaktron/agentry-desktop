// The Report Designer's run, on this computer or In Agentry, followed above
// the pages in its run window (RunWindow.tsx), as a QuickAgent's run is: App
// mounts this, and Design with AI hands the run over once it has begun. On
// this computer the window shows the time used against the Report
// Designer's limit, its turns and tokens, and its Run log, and Stop stops
// only the Report Designer (DesignerAPI.StopHere); In Agentry it shows where
// the run stands in the registry's words, and Cancel run asks the registry.
//
// Once a run on this computer is over, the window is what it made, before
// anything is saved: only Add to my library puts it in the library
// (DesignerAPI.SaveResult), under a free name, never the default, and
// Dismiss forgets it. A run In Agentry saves what it made itself; the window
// says so. Open Report design opens the library, where Use and Edit are.
import { useEffect, useState } from "react";
import * as DesignerAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/designerapi";
import * as SettingsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/settingsapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { type DesignerJob, documentLine, hostedOutcome, lookedLine, savePrompt } from "../designer";
import { cancelDesignerRun, designerSaved, dismissDesignerRun, showDesignerRun, stopDesignerRun, useDesignerRun } from "../designerRun";
import type { Tone } from "../jobProgress";
import { ProgressView } from "../progress";
import { logLines } from "../runLog";
import { ErrorBox, Notice } from "../ui";
import { ResultCard } from "./DesignWithAI";
import { RunWindow, useFollowed } from "./RunWindow";

export default function DesignerFlow({ onOpenDesigns, initialMinimized }: {
  /** Opens Settings → Report design, the library. */
  onOpenDesigns: () => void;
  /** Harness-only: open as the corner card. */
  initialMinimized?: boolean;
}) {
  const s = useDesignerRun();
  const followed = useFollowed({ raised: s.raised, asCard: s.asCard, initialMinimized });
  const j = s.job;
  // What a run on this computer made, once added to the library.
  const [saved, setSaved] = useState<app.ReportDesignView | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [opts, setOpts] = useState<app.ReportDesignOptions | null>(null);
  const token = j?.result?.token ?? "";
  useEffect(() => {
    setSaved(null);
    setError(null);
  }, [token]);
  // The page's likeness needs the families and cover styles; read once a
  // result is there to draw.
  useEffect(() => {
    if (!token || opts) return;
    SettingsAPI.ReportDesignOptions().then(setOpts).catch(() => setOpts(null));
  }, [token, opts]);
  if (!s.shown || !j) return null;

  const here = j.where === "here";
  const h = j.hosted;
  const running = !j.over;
  const { job, images, urls, document } = j.meta;
  const r = j.result;

  let line = "";
  let tone: Tone = "";
  if (!running && here) {
    if (r) {
      line = saved ? `“${r.name}” is in your library as a new design.` : `The Report Designer made “${r.name}”. It is not in your library yet.`;
      tone = "ok";
    } else if (j.stopped) {
      line = "You stopped the Report Designer, so nothing was made.";
      tone = "warn";
    } else {
      line = "The Report Designer did not finish, so nothing was made.";
      tone = "bad";
    }
  } else if (!running && h) {
    line = hostedOutcome(h);
    tone = h.applied ? "ok" : h.status === "cancelled" ? "warn" : "bad";
  } else if (running && h?.cancelRequested) line = "Cancel asked for.";

  const dismiss = () => {
    if (r && !saved) DesignerAPI.DiscardResult(r.token).catch(() => {});
    dismissDesignerRun();
  };
  const save = () => {
    if (!r) return;
    setSaving(true);
    setError(null);
    DesignerAPI.SaveResult(j.meta.registry, r.token)
      .then((v) => { setSaved(v); designerSaved(); })
      .catch((e) => setError(String(e)))
      .finally(() => setSaving(false));
  };
  const openDesigns = () => {
    dismiss();
    onOpenDesigns();
  };
  const made = (here && !!saved) || (!here && !!h?.applied);

  return (
    <RunWindow
      {...followed}
      onOpen={showDesignerRun}
      onDismiss={dismiss}
      label={here ? "The Report Designer on this computer" : "The Report Designer In Agentry"}
      title={job === "cover" ? "Design a cover page with AI" : "Design with AI"}
      cardTitle={running ? `The Report Designer is running${here ? "" : " In Agentry"}` : "The Report Designer: the run is over"}
      running={running}
      spinner={here
        ? (j.stopping ? "Stopping the Report Designer…" : "The Report Designer is running on this computer…")
        : h?.cancelRequested ? "Cancel asked for: Agentry is stopping the run…" : h?.statusLine || h?.state || "The Report Designer is running In Agentry…"}
      about={here ? <>
        It reads your brief{images > 0 ? " and your images" : ""}{document ? ", your document" : ""}{urls > 0 ? ", and the pages at your web addresses once AI Review has read them" : ""}, writes
        the {job === "cover" ? "cover page in Typst" : "design"}, renders it and looks at what it made. Nothing reaches your library until you say so.
      </> : <>
        When it ends, what it made is saved to your library as a new design, never your default.
      </>}
      status={!here && h?.pagesLine && <p className="muted small">{h.pagesLine}</p>}
      progress={here ? (compact) => (
        <ProgressView progress={j.progress} job="run" compact={compact} fact={(p) => (p.turns === 1 ? "1 turn" : `${p.turns} turns`)} />
      ) : undefined}
      background={here ? <>
        In the background, the Report Designer goes on in a card in the corner and the rest of the app stays usable. Stop
        ends it, and nothing is made.
      </> : <>
        In the background, the Report Designer goes on In Agentry and this app follows it in a card in the corner. Cancel
        run asks Agentry to stop it and settle what it used.
      </>}
      stop={here
        ? { label: "Stop the Report Designer", cardLabel: "Stop", stopping: j.stopping, onStop: () => void stopDesignerRun() }
        : h && !h.cancelRequested ? { label: "Cancel run", stopping: false, onStop: () => void cancelDesignerRun() } : null}
      log={here ? logLines(j.log.lines, j.log.dropped) : null}
      line={line}
      tone={tone}
      error={here && !r ? j.error : null}
      outcome={r ? (
        <DesignMade r={r} job={job} saved={saved} opts={opts} line={line} error={error} />
      ) : h?.applied ? (
        <>
          <Notice tone="ok">{line} It is not your default: Use, on the Report design page, makes it so.</Notice>
          {h.document && <p className="small" data-designer-document-line>It was built around your document {h.document}.</p>}
        </>
      ) : undefined}
      primary={made
        ? { label: "Open Report design", onClick: openDesigns }
        : r ? { label: "Add to my library", onClick: save, busy: saving, busyLabel: "Adding…" } : null}
    />
  );
}

// DesignMade is what a run on this computer made: the design as the library
// would draw it, what the Designer said of its own looking, what it took from
// the document and the pages, and where its folder is.
function DesignMade({ r, job, saved, opts, line, error }: {
  r: app.DesignerResultView;
  job: DesignerJob;
  saved: app.ReportDesignView | null;
  opts: app.ReportDesignOptions | null;
  line: string;
  error: string | null;
}) {
  const looked = lookedLine(r.looked);
  const view = saved ?? r.view;
  return (
    <>
      {saved ? <Notice tone="ok">{line} It is not your default: Use, on the Report design page, makes it so.</Notice> : (
        <p>
          The Report Designer made “{r.name}”{r.previews > 0 ? `, after ${r.previews} ${r.previews === 1 ? "render" : "renders"} it looked at` : ""}.
        </p>
      )}
      {looked && <Notice tone="warn">{looked}</Notice>}
      {documentLine(r) && <p className="small" data-designer-document-line>{documentLine(r)}</p>}
      {r.job === "cover" && !r.thumbnail && <p className="muted small">Its last render was not of the cover it kept, so the design has no thumbnail of it.</p>}
      {view && opts && <ResultCard view={view} opts={opts} />}
      <p className="muted small selectable">It is written to <span className="mono">{r.dir}</span>, with the run&apos;s reports and receipts beside it.</p>
      {r.pages && r.pages.length > 0 && (
        <ul className="small">
          {r.pages.map((p) => <li key={p.url}><span className="mono">{p.url}</span>: {p.withheld ? `withheld by AI Review (${p.withheld})` : p.rendered ? "read, rendered first" : "read"}</li>)}
        </ul>
      )}
      {!saved && <p className="small">{savePrompt(job)}</p>}
      <ErrorBox error={error} />
    </>
  );
}
