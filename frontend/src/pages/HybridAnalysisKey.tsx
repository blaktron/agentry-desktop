// The person's own Hybrid Analysis key (#530; plan file-handling.md §7, D8,
// D17): a run on this computer looks the risky files among its inputs up on
// CrowdStrike's Hybrid Analysis with it, by their fingerprints (SHA-256)
// alone, never the files (agentry-cli#564). It is kept in the OS keychain
// beside the model key (RunAPI.StoreHybridKey, the one call that carries
// it), shown as saved and never as its value, and Test key looks one known
// fingerprint up and says the answer in plain words. With no key, runs are
// not looked up and nothing warns: the run's receipt says so. The card has
// AIReviewKey.tsx's shape, as WebSearchKey.tsx does. The kinds of file it
// names are the CLI's (pkg/runner/hybrid's risky types), said in words.
import { useEffect, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { OpenExternal } from "../../wailsjs/go/main/App";
import { main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Notice } from "../ui";
import { keyStanding } from "../keyRecommendation";

/** Where a person gets a key: Hybrid Analysis's sign-up, in the system
 *  browser. */
export const HYBRID_ANALYSIS_URL = "https://hybrid-analysis.com/signup";

/** Where the key stands, in one sentence. */
export function hybridKeySentence(k: main.HybridKeyView): string {
  switch (keyStanding(k)) {
    case "env":
      return `$${k.envName} is set where this app was started, and a run uses it before the keychain.`;
    case "saved":
      return "Saved in this computer's keychain.";
    case "unread":
      return "A key is in the keychain, but the runner policy does not read it from there. Save it again to fix that.";
    default:
      return "No key saved. Runs on this computer do not look files up on Hybrid Analysis; ClamAV still checks every file.";
  }
}

export function HybridAnalysisKeyCard() {
  const [status, setStatus] = useState<main.HybridKeyView | null>(null);
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tested, setTested] = useState<main.HybridKeyTestView | null>(null);

  useEffect(() => {
    RunAPI.HybridKeyStatus().then(setStatus).catch((e) => setError(String(e)));
  }, []);

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
      const k = await RunAPI.StoreHybridKey(key);
      setKey("");
      setTested(null);
      setStatus(k);
    });

  const test = () =>
    act("test", async () => {
      setTested(await RunAPI.TestHybridKey());
    });

  const forget = () =>
    act("forget", async () => {
      setTested(null);
      setStatus(await RunAPI.ForgetHybridKey());
    });

  return (
    <Card>
      <p style={{ marginTop: 0 }}>
        Only the fingerprints (SHA-256) of risky files are sent to Hybrid Analysis, never the files.{" "}
        <Button variant="link" onClick={() => void OpenExternal(HYBRID_ANALYSIS_URL)}>Get a free key</Button>
      </p>
      <p className="muted small">
        Your own CrowdStrike Hybrid Analysis API key. Before a run on this computer starts, the fingerprints of the
        files you give it that could carry malware (programs, scripts, PDF, RTF, older and macro Office files, disk
        images and shortcuts, inside archives too) are looked up. A file Hybrid Analysis calls malicious stops the run;
        one it calls suspicious is noted in the run&apos;s receipt. Without a key nothing is looked up, and ClamAV still
        checks every file. The key is kept in this computer&apos;s keychain next to your model key: not in a file, and
        never shown here.
      </p>
      {status && <p className="small" role="status">{hybridKeySentence(status)}</p>}
      {status?.keyStoreErr && <Notice tone="warn">The keychain could not be asked: {status.keyStoreErr}.</Notice>}
      <label className="field">
        <span className="field-label">Hybrid Analysis API key</span>
        <div className="row">
          <Input
            type="password" className="grow mono" autoComplete="off" aria-label="Hybrid Analysis API key"
            placeholder={status?.stored ? "saved on this computer — paste a new one to replace it" : "Paste your Hybrid Analysis API key"}
            value={key} onChange={(e) => setKey(e.target.value)} disabled={!!busy}
          />
          <Button variant="primary" disabled={!!busy || !key.trim()} onClick={save}>{busy === "save" ? "Saving…" : "Save"}</Button>
        </div>
      </label>
      <div className="row">
        <Button disabled={!!busy || !status?.ready} title={status?.ready ? "one lookup of a known fingerprint with the key a run would use" : "save a key first"} onClick={test}>
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
