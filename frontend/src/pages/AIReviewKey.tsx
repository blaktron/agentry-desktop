// The person's own AI Review key: their TypeSafe Jev key, which Describe
// it, on this computer scans with. It is kept in the OS keychain beside
// the model key (RunAPI.StoreJevKey, the one call that carries it), shown
// as saved and never as its value, and Test key makes one small call and
// says TypeSafe's answer in plain words. The card leads with the
// recommendation to get a key and the link to TypeSafe
// (keyRecommendation.ts), which every surface that offers the key shares.
// The recommendation speaks of submitting, which needs no key, so the
// paragraph beneath it says so.
import { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { OpenExternal } from "../../wailsjs/go/main/App";
import { main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Notice } from "../ui";
import { GET_A_KEY_LABEL, KEY_RECOMMENDATION, TYPESAFE_URL, keyStanding } from "../keyRecommendation";

/** The link to TypeSafe, where a person gets a key: the system browser
 *  opens it (App.OpenExternal), at the one address keyRecommendation.ts
 *  holds. */
export function GetAKeyLink() {
  return <Button variant="link" onClick={() => void OpenExternal(TYPESAFE_URL)}>{GET_A_KEY_LABEL}</Button>;
}

/** Where the key stands, in one sentence. */
export function jevKeySentence(k: main.JevKeyView): string {
  switch (keyStanding(k)) {
    case "env":
      return `$${k.envName} is set where this app was started, and a draft uses it before the keychain.`;
    case "saved":
      return "Saved in this computer's keychain.";
    case "unread":
      return "A key is in the keychain, but the runner policy does not read it from there. Save it again to fix that.";
    default:
      return "No key saved. Drafting on this computer is off until you add one.";
  }
}

/** What the card knows, for a caller that decides on it (the first-run
 *  wizard's Skip, and its footer held while a save is in flight). It never
 *  carries the key: status is the view Go returns, which has none. */
export type ReviewKeyState = { status: main.JevKeyView | null; busy: boolean };

export function AIReviewKeyCard({ onState }: { onState?: (s: ReviewKeyState) => void } = {}) {
  const [status, setStatus] = useState<main.JevKeyView | null>(null);
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tested, setTested] = useState<main.JevKeyTestView | null>(null);

  useEffect(() => {
    RunAPI.JevKeyStatus().then(setStatus).catch((e) => setError(String(e)));
  }, []);
  // The caller's callback, read through a ref, as HarnessChooser reads
  // its own: a fresh function on each of the caller's renders must not
  // re-report an unchanged state.
  const report = useRef(onState);
  useEffect(() => { report.current = onState; }, [onState]);
  useEffect(() => { report.current?.({ status, busy: busy !== null }); }, [status, busy]);

  async function act(label: string, fn: () => Promise<void>) {
    setBusy(label);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  const save = () =>
    act("save", async () => {
      const k = await RunAPI.StoreJevKey(key);
      setKey("");
      setTested(null);
      setStatus(k);
    });

  const test = () =>
    act("test", async () => {
      setTested(await RunAPI.TestJevKey());
    });

  const forget = () =>
    act("forget", async () => {
      setTested(null);
      setStatus(await RunAPI.ForgetJevKey());
    });

  return (
    <Card>
      <p style={{ marginTop: 0 }}>
        {KEY_RECOMMENDATION} <GetAKeyLink />
      </p>
      <p className="muted small">
        Your own TypeSafe Jev key. When you describe a QuickAgent and draft it on this computer, AI Review reads your
        description, and every page and search result the draft reads, with this key before the model sees any of it.
        What it flags is held back. Submitting needs no key of yours: Agentry scans every submission itself. The key is
        kept in this computer&apos;s keychain next to your model key: not in a file, and never shown here.
      </p>
      {status && <p className="small" role="status">{jevKeySentence(status)}</p>}
      {status?.keyStoreErr && <Notice tone="warn">The keychain could not be asked: {status.keyStoreErr}.</Notice>}
      <label className="field">
        <span className="field-label">Jev key</span>
        <div className="row">
          <Input
            type="password" className="grow mono" autoComplete="off" aria-label="Jev key"
            placeholder={status?.stored ? "saved on this computer — paste a new one to replace it" : "Paste your TypeSafe Jev key"}
            value={key} onChange={(e) => setKey(e.target.value)} disabled={!!busy}
          />
          <Button variant="primary" disabled={!!busy || !key.trim()} onClick={save}>{busy === "save" ? "Saving…" : "Save"}</Button>
        </div>
      </label>
      <div className="row">
        <Button disabled={!!busy || !status?.ready} title={status?.ready ? "one small call to TypeSafe with the key a draft would use" : "save a key first"} onClick={test}>
          {busy === "test" ? "Testing…" : "Test key"}
        </Button>
        {status?.stored && (
          <Button disabled={!!busy} onClick={forget}>{busy === "forget" ? "Forgetting…" : "Forget the saved key"}</Button>
        )}
      </div>
      {tested && (
        <Notice tone={tested.ok ? "info" : "warn"}>
          {tested.message}
          {tested.from ? <span className="muted"> (tested the key from {tested.from})</span> : null}
        </Notice>
      )}
      <ErrorBox error={error} />
    </Card>
  );
}
