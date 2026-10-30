// A repository or url intake question in the run setup's Details step: the
// address (and, for a repository, the branch, tag or commit), then the
// access token, drawn from the start so a private source's token has a
// place before the address is checked. A key saved
// for the address's host is offered and used
// unless another token is typed for this run; a typed token can be saved
// for the host in the system keychain, where the CLI and a schedule's fire
// read it too. Go checks the address with the CLI's own parser and never
// returns a token; the token typed here goes to Go with the run, for its
// fetch step alone, and is not kept with the setup's answers.
import { useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import { Button } from "../ui";
import { joinRepository, splitRepository, tokenFieldLabel, tokenLine, type SourceKey } from "../sources";

export function SourceQuestion({
  q, value, onChange, sourceKey, onKey, typed, onTyped, disabled, onError, inAgentry = false,
}: {
  q: { id: string; prompt: string; type: string; required: boolean };
  value: string;
  onChange: (value: string) => void;
  /** Go's word on the current answer; null while it is being checked. */
  sourceKey: SourceKey | null;
  /** A save or a removal answers the key's new standing. */
  onKey: (key: SourceKey) => void;
  typed: string;
  onTyped: (token: string) => void;
  disabled: boolean;
  onError: (e: string) => void;
  /** The run goes In Agentry: its token is sent with the start. */
  inAgentry?: boolean;
}) {
  const repository = q.type === "repository";
  const { url, ref } = repository ? splitRepository(value) : { url: value, ref: "" };
  // With a key saved, the field opens only on "Use another token".
  const [another, setAnother] = useState(typed.trim() !== "");
  const [saving, setSaving] = useState(false);
  const answered = value.trim() !== "";
  const host = sourceKey && !sourceKey.problem ? sourceKey.host : undefined;
  // Drawn whatever the address holds; with a key saved for its host, only on
  // "Use another token".
  const showField = !sourceKey?.saved || another;

  async function save() {
    setSaving(true);
    try {
      const k = await RunAPI.SaveSourceKey(q.type, value.trim(), typed);
      onTyped("");
      setAnother(false);
      onKey(k);
    } catch (e) {
      onError(String(e));
    } finally {
      setSaving(false);
    }
  }

  async function forget() {
    setSaving(true);
    try {
      onKey(await RunAPI.ForgetSourceKey(q.type, value.trim()));
    } catch (e) {
      onError(String(e));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="source-question" style={{ marginBottom: 12 }}>
      <div className="row" style={{ marginBottom: 6 }}>
        <span style={{ minWidth: 260 }}>{q.prompt}{q.required ? " *" : ""}</span>
        <input
          type="text"
          className="grow mono"
          autoComplete="off"
          spellCheck={false}
          placeholder={repository ? "https://github.com/owner/repository" : "https://example.com/report.pdf"}
          aria-label={repository ? `${q.prompt}: the repository's address` : `${q.prompt}: the file's address`}
          value={url}
          onChange={(e) => onChange(repository ? joinRepository(e.target.value, ref) : e.target.value)}
          disabled={disabled}
        />
      </div>
      {repository && (
        <div className="row" style={{ marginBottom: 6 }}>
          <span style={{ minWidth: 260 }} className="muted small">Branch, tag or commit</span>
          <input
            type="text"
            className="grow mono"
            autoComplete="off"
            spellCheck={false}
            placeholder="empty for the default branch"
            aria-label={`${q.prompt}: the branch, tag or commit`}
            value={ref}
            onChange={(e) => onChange(joinRepository(url, e.target.value))}
            disabled={disabled}
          />
        </div>
      )}
      {answered && sourceKey?.problem && <p className="small" style={{ color: "var(--color-sev-high)", margin: "0 0 6px" }}>{sourceKey.problem}</p>}
      {answered && !sourceKey && <p className="muted small" style={{ margin: "0 0 6px" }}>Checking the address…</p>}
      {showField && (
        <div className="row" style={{ marginBottom: 6 }}>
          <span style={{ minWidth: 260 }} className="muted small">{tokenFieldLabel(sourceKey, repository)} (optional)</span>
          <input
            type="password"
            className="grow mono"
            autoComplete="off"
            placeholder={sourceKey?.saved ? "a token for this run only" : "optional"}
            aria-label={`${q.prompt}: ${tokenFieldLabel(sourceKey, repository).toLowerCase()}`}
            value={typed}
            onChange={(e) => onTyped(e.target.value)}
            disabled={disabled}
          />
          {host && !sourceKey?.keychainOff && (
            <Button onClick={save} disabled={disabled || typed.trim() === ""} busy={saving} busyLabel="Saving…">
              {sourceKey?.saved ? `Replace the saved key` : `Save for ${host}`}
            </Button>
          )}
        </div>
      )}
      <p className="muted small" style={{ margin: "0 0 4px" }}>
        {tokenLine(sourceKey, typed, repository, inAgentry)}{" "}
        {inAgentry
          ? "On the sandbox host the token is handed to the step that fetches the address before the run, and is not given to the agent or its tools."
          : "The token is handed to the step that fetches the address before the run, on its standard input, and is not given to the agent or its tools."}
      </p>
      {host && (
        <>
          {sourceKey?.saved && (
            <div className="row" style={{ gap: 8 }}>
              {another ? (
                <Button variant="link" onClick={() => { onTyped(""); setAnother(false); }} disabled={disabled}>Use the saved key</Button>
              ) : (
                <Button variant="link" onClick={() => setAnother(true)} disabled={disabled}>Use another token for this run</Button>
              )}
              <Button variant="link" onClick={forget} disabled={disabled || saving}>Remove the saved key</Button>
            </div>
          )}
          {sourceKey?.keyStoreErr && <p className="muted small">The system keychain could not be read: {sourceKey.keyStoreErr}</p>}
        </>
      )}
    </div>
  );
}
