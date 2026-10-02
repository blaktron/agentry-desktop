// The author drop-zone: point at a local agent folder and run the same
// checks the registry runs at publish time (D-12 parity by construction —
// the same Go packages, embedded in this app). For a folder chosen with the
// picker it also offers Review Your QuickAgents locally (agentry-desktop#469):
// the built-in Examiner over the same folder, after a native dialog, followed
// in its own dialog and card. Validate itself is the same with or without it.
import { useState } from "react";
import * as ValidateAPI from "../../wailsjs/go/main/ValidateAPI";
import { ChooseDirectory } from "../../wailsjs/go/main/App";
import { validate } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Excerpt, Input, ModeBadges, Notice, PageHeader, Sev, Spinner, Toolbar } from "../ui";
import { startReview } from "../reviews";
import { ReviewAction } from "./ReviewAction";

export default function Validate({ registry, onOpenSettings }: {
  /** The registry a review asks for its public policy and server records. */
  registry: string;
  /** Settings at the AI Review key, for a review without one. */
  onOpenSettings?: () => void;
}) {
  const [dir, setDir] = useState("");
  // picked is the folder the picker gave: a review takes only that (D-030),
  // so a path typed into the field is validated but not reviewed.
  const [picked, setPicked] = useState("");
  const [reviewing, setReviewing] = useState(false);
  const [reviewError, setReviewError] = useState<string | null>(null);
  const [result, setResult] = useState<validate.Result | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function pick() {
    try {
      const chosen = await ChooseDirectory("Choose a QuickAgent folder to validate");
      if (chosen) {
        setDir(chosen);
        setPicked(chosen);
        await run(chosen);
      }
    } catch (e) {
      setError(String(e));
    }
  }

  async function run(d = dir) {
    if (!d) return;
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      setResult(await ValidateAPI.Run(d));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function review() {
    setReviewing(true);
    setReviewError(null);
    try {
      await startReview(registry, picked);
    } catch (e) {
      setReviewError(String(e));
    } finally {
      setReviewing(false);
    }
  }

  return (
    <div>
      <PageHeader
        title="Validate"
        sub="Run a local QuickAgent folder through the manifest rules, the S1 inventory (content-sniffed binaries, nested archives, encoding), and the S3/S4 hidden-content and injection-signature scanners — the same code the registry runs at publish time."
      />

      <Toolbar>
        <Input
          type="text"
          className="grow mono"
          placeholder="Path to a QuickAgent folder (it contains agent.yaml)"
          value={dir}
          onChange={(e) => setDir(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && run()}
        />
        <Button onClick={pick}>Choose…</Button>
        <Button variant="primary" disabled={!dir} busy={busy} busyLabel="Scanning…" onClick={() => run()}>Validate</Button>
      </Toolbar>

      <ErrorBox error={error} />
      {dir && dir === picked && (
        <>
          <ReviewAction onReview={review} busy={reviewing} disabled={busy} onOpenSettings={onOpenSettings} what="this folder's QuickAgent" />
          <ErrorBox error={reviewError} />
        </>
      )}
      {dir && dir !== picked && (
        <p className="muted small">To review this folder on this computer with Review Your QuickAgents locally, choose it with Choose…: a review reads only a folder you picked.</p>
      )}
      {busy && <Spinner label="Scanning…" />}

      {result && <ResultView r={result} />}
    </div>
  );
}

function ResultView({ r }: { r: validate.Result }) {
  // Go sends a nil slice as null; read every list as Build's review card does
  // (agentry-desktop#534: a clean folder's null inventory stopped the page).
  const issues = r.issues ?? [];
  const inventory = r.inventory ?? [];
  const findings = r.findings ?? [];
  const readErrors = r.readErrors ?? [];
  const totalProblems = r.errorCount + r.inventoryFailing + findings.length;
  return (
    <div>
      <Card>
        <div className="row">
          {r.passed
            ? <span className="badge ok">PASSED</span>
            : <span className="badge bad">FAILED — {totalProblems || r.warnCount} problem{(totalProblems || r.warnCount) === 1 ? "" : "s"}</span>}
          {r.manifestName && <strong className="selectable">{r.manifestName}</strong>}
          <ModeBadges modes={r.manifestModes} />
          {r.manifestVersion && <span className="mono">v{r.manifestVersion}</span>}
        </div>
        <p className="muted small" style={{ marginBottom: 0 }}>
          {r.fileCount} file{r.fileCount === 1 ? "" : "s"} scanned · rule bundle {r.bundleVersion} ({r.bundleSource})
          {r.warnCount > 0 && <> · {r.warnCount} manifest warning{r.warnCount === 1 ? "" : "s"}</>}
        </p>
        {r.manifestError && <Notice tone="warn">{r.manifestError}</Notice>}
        {readErrors.map((e, i) => <div key={i} className="error-box">{e}</div>)}
      </Card>

      {issues.length > 0 && (
        <Card>
          <h3 style={{ marginTop: 0 }}>Manifest rules (public-publish context)</h3>
          {issues.map((iss, i) => (
            <div key={i} className="finding">
              <div className="row">
                <Sev severity={iss.warning ? "medium" : "high"} />
                <span className="mono">{iss.rule}{iss.path ? ` (${iss.path})` : ""}</span>
                <span className="badge">{iss.warning ? "warning" : "error"}</span>
              </div>
              <div className="selectable">{iss.message}</div>
            </div>
          ))}
        </Card>
      )}

      <Card>
        <h3 style={{ marginTop: 0 }}>Inventory (S1)</h3>
        {inventory.length === 0
          ? <p className="muted" style={{ marginBottom: 0 }}>Clean — no binaries, nested archives, or encoding problems.</p>
          : inventory.map((f, i) => (
            <div key={i} className="finding">
              <div className="row">
                <Sev severity={f.severity} />
                <span className="mono">{f.scanner}</span>
                <strong>{f.title}</strong>
              </div>
              <div className="loc mono">{f.filePath}</div>
              <div className="muted small">{f.detail}</div>
            </div>
          ))}
      </Card>

      <Card>
        <h3 style={{ marginTop: 0 }}>Content scan (S3 hidden-content + S4 signatures)</h3>
        {findings.length === 0
          ? <p className="muted" style={{ marginBottom: 0 }}>Clean — {r.fileCount} file{r.fileCount === 1 ? "" : "s"}, no findings.</p>
          : findings.map((f, i) => (
            <div key={i} className="finding">
              <div className="row">
                <Sev severity={f.severity} />
                <span className="mono">{f.ruleId}</span>
                <strong>{f.title}</strong>
                <span className="badge">{f.confidence}</span>
              </div>
              <div className="loc mono">{f.filePath}{f.lineStart ? `:${f.lineStart}` : ""}</div>
              <Excerpt text={f.excerpt} />
            </div>
          ))}
      </Card>

      <p className="muted small">
        A passing scan means these detectors found nothing — the same claim, no more,
        that the registry&apos;s report card makes.
      </p>
    </div>
  );
}
