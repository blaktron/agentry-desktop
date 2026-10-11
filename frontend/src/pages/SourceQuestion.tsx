// A repository or url intake question in the run setup's Details step
// (plan input-sources.md §5, D3, D5, D13; agentry-desktop#794): the
// address (and, for a repository, the branch, tag or commit), then the
// access token. A key saved for the address's host is offered and used
// unless another token is typed for this run; a typed token can be saved
// for the host in the system keychain, where the CLI and a schedule's fire
// read it too. Go checks the address with the CLI's own parser and never
// returns a token; the token typed here goes to Go with the run, for its
// fetch step alone, and is not kept with the setup's answers.
import { useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { Button } from "../ui";
import { joinRepository, splitRepository, tokenLine, type SourceKey } from "../sources";

export function SourceQuestion({
  q, value, onChange, sourceKey, onKey, typed, onTyped, disabled, onError,
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
}) {
  const repository = q.type === "repository";
  const { url, ref } = repository ? splitRepository(value) : { url: value, ref: "" };
  // With a key saved, the field opens only on "Use another token".
  const [another, setAnother] = useState(typed.trim() !== "");
  const [saving, setSaving] = useState(false);
  const answered = value.trim() !== "";
  const host = sourceKey && !sourceKey.problem ? sourceKey.host : undefined;
  const showField = !!host && (!sourceKey?.saved || another);

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
      {host && (
        <>
          {showField && (
            <div className="row" style={{ marginBottom: 6 }}>
              <span style={{ minWidth: 260 }} className="muted small">Access token for {host}</span>
              <input
                type="password"
                className="grow mono"
                autoComplete="off"
                placeholder={sourceKey?.saved ? "a token for this run only" : "optional"}
                aria-label={`${q.prompt}: the access token for ${host}`}
                value={typed}
                onChange={(e) => onTyped(e.target.value)}
                disabled={disabled}
              />
              {!sourceKey?.keychainOff && (
                <Button onClick={save} disabled={disabled || typed.trim() === ""} busy={saving} busyLabel="Saving…">
                  {sourceKey?.saved ? `Replace the saved key` : `Save for ${host}`}
                </Button>
              )}
            </div>
          )}
          <p className="muted small" style={{ margin: "0 0 4px" }}>
            {tokenLine(sourceKey, typed, repository)}{" "}
            The token is handed to the step that fetches the address before the run, on its standard input, and is not given to the agent or its tools.
          </p>
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
