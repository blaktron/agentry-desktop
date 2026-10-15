import { useEffect, useRef, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/app/SettingsAPI";
import { Button, ErrorBox, Modal, Notice, Textarea } from "../ui";
import { bugReportOutcome, bugTextProblem } from "../bugReport";

// Report bug: the card the sidebar's link above the version opens. Signed
// out, it says so and offers Settings' sign-in instead of the box. Submit
// files the
// text with the active registry as the person (SettingsAPI.ReportBug),
// with the app's version and OS; filed, it thanks them and closes itself.
// The text is plain text and is shown back only in the box.
export default function BugReportDialog({ registry, signedIn, onClose, onSignIn }: {
  registry: string;
  signedIn: boolean;
  onClose: () => void;
  onSignIn: () => void;
}) {
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<ReturnType<typeof bugReportOutcome> | null>(null);
  const [error, setError] = useState<string | null>(null);
  const filed = outcome?.tone === "ok";

  // Filed: the thanks stays a moment, then the card closes itself. The
  // latest onClose is read through a ref, so a re-render of the app does
  // not restart the wait.
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    if (!filed) return;
    const t = setTimeout(() => close.current(), 2500);
    return () => clearTimeout(t);
  }, [filed]);

  async function submit() {
    setBusy(true);
    setError(null);
    setOutcome(null);
    try {
      setOutcome(bugReportOutcome(await SettingsAPI.ReportBug(registry, text)));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  const problem = bugTextProblem(text);
  return (
    <Modal title="Report a bug" onClose={onClose}>
      {!signedIn ? (
        <>
          <p>Sign in to report a bug, so we can email you when it is fixed.</p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={onClose}>Cancel</Button>
            <Button variant="primary" onClick={onSignIn}>Sign in</Button>
          </div>
        </>
      ) : filed ? (
        <Notice tone="ok">{outcome.sentence}</Notice>
      ) : (
        <>
          <p className="muted">
            Tell us what went wrong and what you were doing. We note this app&apos;s version and your operating system with it, and staff will email you when it is fixed.
          </p>
          <Textarea
            aria-label="What went wrong"
            rows={8}
            value={text}
            onChange={(e) => setText(e.target.value)}
            autoFocus
          />
          {problem && <p className="small muted">{problem}</p>}
          {outcome && <Notice tone={outcome.tone}>{outcome.sentence}</Notice>}
          <ErrorBox error={error} />
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={onClose}>Cancel</Button>
            {outcome?.signIn ? (
              <Button variant="primary" onClick={onSignIn}>Sign in</Button>
            ) : (
              <Button variant="primary" onClick={submit} disabled={busy || !!problem || !text.trim()} busy={busy} busyLabel="Sending…">
                Submit
              </Button>
            )}
          </div>
        </>
      )}
    </Modal>
  );
}
