// The person's own web search key: the provider they choose, from the
// ones the CLI's runner policy admits (RunAPI.SearchKeyStatus carries them),
// and its key, which Describe it, on this computer searches the web with. It
// is kept in the OS keychain beside the AI Review key (RunAPI.StoreSearchKey,
// the one call that carries it), shown as saved and never as its value, and
// Test key makes one search and says the provider's answer in plain words.
// The same one-card shape as AIReviewKey.tsx.
import { useEffect, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Notice, Select } from "../ui";
import { initialProvider, replaceNote, searchKeySentence } from "../searchKey";

export function WebSearchKeyCard() {
  const [status, setStatus] = useState<main.SearchKeyView | null>(null);
  const [provider, setProvider] = useState("");
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tested, setTested] = useState<main.SearchKeyTestView | null>(null);

  function show(s: main.SearchKeyView) {
    setStatus(s);
    setProvider(initialProvider(s));
  }

  useEffect(() => {
    RunAPI.SearchKeyStatus().then(show).catch((e) => setError(String(e)));
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
      const s = await RunAPI.StoreSearchKey(provider, key);
      setKey("");
      setTested(null);
      show(s);
    });

  const test = () =>
    act("test", async () => {
      setTested(await RunAPI.TestSearchKey());
    });

  const forget = () =>
    act("forget", async () => {
      setTested(null);
      show(await RunAPI.ForgetSearchKey());
    });

  const chosen = status?.providers.find((p) => p.name === provider);
  const note = status ? replaceNote(status, provider) : "";
  const current = !!status && status.provider === provider;
  // The variable a draft reads first: the policy's own for the saved
  // provider (it may name another), else the chosen provider's default.
  const envName = current && status?.envName ? status.envName : chosen?.envName;

  return (
    <Card>
      <p className="muted small" style={{ marginTop: 0 }}>
        Your own key for a web search service. When you describe a QuickAgent and draft it on this computer, the draft
        searches the web with it, and AI Review reads every result before the model does. Without one, the draft reads the
        registry and the pages it already knows. Your searches go to the provider you choose, under that provider&apos;s
        own terms. The key is kept in this computer&apos;s keychain next to your model key: not in a file, and never shown
        here.
      </p>
      {status && <p className="small" role="status">{searchKeySentence(status)}</p>}
      {status?.keyStoreErr && <Notice tone="warn">The keychain could not be asked: {status.keyStoreErr}.</Notice>}
      <label className="field">
        <span className="field-label">Search provider</span>
        <Select value={provider} disabled={!!busy || !status} aria-label="Search provider"
          onChange={(e) => { setProvider(e.target.value); setTested(null); }}>
          {status?.providers.map((p) => <option key={p.name} value={p.name}>{p.label}</option>)}
        </Select>
      </label>
      <label className="field">
        <span className="field-label">{chosen ? `${chosen.label} key` : "Key"}</span>
        <div className="row">
          <Input
            type="password" className="grow mono" autoComplete="off" aria-label="Search key"
            placeholder={current && status?.stored ? "saved on this computer — paste a new one to replace it" : `Paste your ${chosen?.label ?? "search"} key`}
            value={key} onChange={(e) => setKey(e.target.value)} disabled={!!busy}
          />
          <Button variant="primary" disabled={!!busy || !key.trim() || !provider} onClick={save}>{busy === "save" ? "Saving…" : "Save"}</Button>
        </div>
        {envName && <span className="muted small">A draft reads {"$" + envName} first, when it is set where this app was started.</span>}
      </label>
      {note && <Notice tone="warn">{note}</Notice>}
      <div className="row">
        <Button disabled={!!busy || !status?.ready || !current}
          title={status?.ready && current ? "one search with the key a draft would use" : "save a key for this provider first"} onClick={test}>
          {busy === "test" ? "Testing…" : "Test key"}
        </Button>
        {status?.stored && (
          <Button disabled={!!busy} onClick={forget}>{busy === "forget" ? "Forgetting…" : `Forget the saved ${status.providerLabel || "search"} key`}</Button>
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
