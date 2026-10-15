import React, { useEffect, useState } from "react";
import * as RunAPI from "../../wailsjs/go/app/RunAPI";
import { OpenExternal } from "../../wailsjs/go/app/App";
import { ClipboardSetText, EventsOn } from "../../wailsjs/runtime/runtime";
import { Button, ErrorBox, Notice, Spinner } from "../ui";
import { keptFor, keptLine, removeLabel, type KeptSignIn, type Way } from "../modelSetup";

/** The sign-in half of model setup: one way's sign-in or connect, made
 * through the CLI's own packages on the Go side, with nothing for the
 * person to copy or paste but GitHub's own code, which the app copies for
 * them. While it waits it says so, offers the page again should the
 * browser not have opened, and can be cancelled. No token, key or
 * authorization code ever reaches this page. */
export function SignInWay({ way, kept, disabled, onSignedIn, onRemoved }: {
  way: Way;
  kept: KeptSignIn[];
  disabled?: boolean;
  /** Told when a sign-in was kept, with the models a ChatGPT plan offers. */
  onSignedIn: (models: string[]) => void;
  onRemoved: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [lines, setLines] = useState<string[]>([]);
  const [page, setPage] = useState<string | null>(null);
  const [code, setCode] = useState<{ page: string; code: string; copied: boolean } | null>(null);
  const k = keptFor(way, kept);

  useEffect(() => {
    if (!busy) return;
    const offs = [
      EventsOn("signin:say", (v: { vendor: string; line: string }) => {
        if (v.vendor === way.vendor) setLines((prev) => [...prev.slice(-5), v.line]);
      }),
      EventsOn("signin:url", (v: { vendor: string; url: string }) => {
        if (v.vendor === way.vendor) setPage(v.url);
      }),
      EventsOn("signin:code", (v: { vendor: string; page: string; code: string }) => {
        if (v.vendor !== way.vendor) return;
        setCode({ page: v.page, code: v.code, copied: false });
        ClipboardSetText(v.code).then((ok) => setCode({ page: v.page, code: v.code, copied: ok })).catch(() => undefined);
      }),
    ];
    return () => offs.forEach((off) => off());
  }, [busy, way.vendor]);

  async function signIn() {
    setBusy(true);
    setError(null);
    setNotice(null);
    setLines([]);
    setPage(null);
    setCode(null);
    try {
      const v = way.id === "chatgpt" ? await RunAPI.SignInChatGPT(false)
        : way.id === "claude" ? await RunAPI.SignInClaude()
          : way.id === "copilot" ? await RunAPI.SignInCopilot(false)
            : await RunAPI.ConnectAccount(way.vendor ?? "");
      if (v.cancelled) {
        setNotice("Cancelled. Nothing was kept.");
      } else {
        onSignedIn(v.signedIn?.models ?? []);
      }
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
      setCode(null);
    }
  }

  async function remove() {
    if (!k) return;
    setBusy(true);
    setError(null);
    try {
      const outcome = await RunAPI.RemoveModelSignIn(k.vendor);
      setNotice(outcome ? outcome.charAt(0).toUpperCase() + outcome.slice(1) + "." : "Removed.");
      onRemoved();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="signin-way">
      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}
      {busy ? (
        <div className="signin-waiting">
          {code ? (
            <>
              <p>GitHub&apos;s page asks for this code{code.copied ? " — it is copied, so paste it there and continue" : ""}:</p>
              <p className="signin-code mono selectable">{code.code}</p>
              <Button onClick={() => RunAPI.CancelSignIn()}>Cancel</Button>{" "}
              <Button variant="link" onClick={() => { void ClipboardSetText(code.code); setCode({ ...code, copied: true }); }}>Copy again</Button>
            </>
          ) : (
            <>
              <Spinner label="Waiting for you in your browser…" />
              <Button onClick={() => RunAPI.CancelSignIn()}>Cancel</Button>
            </>
          )}
          {page && (
            <p className="muted small">
              No browser opened? <Button variant="link" onClick={() => OpenExternal(page)}>Open the page again</Button>
            </p>
          )}
          {lines.length > 0 && <p className="muted small">{lines[lines.length - 1]}</p>}
        </div>
      ) : k ? (
        <div className="row" style={{ alignItems: "center", gap: 8 }}>
          <span className={k.broken ? "badge warn" : "badge ok"}>{keptLine(k)}</span>
          <span className="grow" />
          <Button variant="link" disabled={disabled} onClick={signIn}>{way.id === "connect" ? "Connect again" : "Sign in again"}</Button>
          <Button disabled={disabled} onClick={remove}>{removeLabel(k)}</Button>
        </div>
      ) : (
        <Button variant="primary" disabled={disabled} onClick={signIn}>{way.label}</Button>
      )}
      <p className="muted small field-hint">{way.hint}</p>
    </div>
  );
}

/** Everything this computer keeps — Settings lists it, so a sign-in or a
 * connected account not in use can be removed too. */
export function KeptSignIns({ kept, onChanged }: { kept: KeptSignIn[]; onChanged: () => void }) {
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  if (kept.length === 0) return null;
  async function remove(k: KeptSignIn) {
    setBusy(k.vendor);
    setError(null);
    try {
      const outcome = await RunAPI.RemoveModelSignIn(k.vendor);
      setNotice(outcome ? outcome.charAt(0).toUpperCase() + outcome.slice(1) + "." : "Removed.");
      onChanged();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }
  return (
    <div className="kept-signins">
      <span className="field-label">Kept on this computer</span>
      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}
      {kept.map((k) => (
        <div key={k.vendor} className="row" style={{ alignItems: "center", gap: 8 }}>
          <span>{VENDOR_NAMES[k.vendor] ?? k.vendor}</span>
          <span className={k.broken ? "badge warn" : "badge"}>{keptLine(k)}</span>
          <span className="grow" />
          <Button disabled={!!busy} busy={busy === k.vendor} busyLabel="Removing…" onClick={() => remove(k)}>{removeLabel(k)}</Button>
        </div>
      ))}
    </div>
  );
}

const VENDOR_NAMES: Record<string, string> = {
  chatgpt: "ChatGPT", claude: "Claude (Claude Code)", copilot: "GitHub Copilot",
  openrouter: "OpenRouter", poe: "Poe", huggingface: "Hugging Face",
};
