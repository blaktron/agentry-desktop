// Connect tools' sign-ins: each declared MCP server that signs in by OAuth,
// with Connect, which opens the vendor's consent screen in the person's
// browser the first time a run of this QuickAgent version needs it, and the
// state it leaves ("Connected …", the next run asking nothing). The consent
// address is also shown, with a field for the address the browser ended
// on, for a browser on another computer. A vendor that lets no app
// register itself (Google) asks for the person's own OAuth client, with
// Agentry's steps for making one shown first; the secret goes to Go
// in one call and is never shown back. Disconnect removes the connection
// and revokes it at the vendor where it offers that.
import { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import { OpenExternal } from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/app";
import * as run from "../../bindings/github.com/blaktron/agentry-desktop/internal/run/models";
import { EventsOn } from "../host";
import { Button, Card, ErrorBox, Input, Modal, Notice } from "../ui";
import { AuthorHelp } from "../KeyHelp";
import { connectionLine, hostOf, pastedProblem, scopeWords, sharedGrantWarning, type ServerConn } from "../connections";

/**
 * The own-client ask: Agentry's steps for the vendor, then the release's own
 * help for signing in to that server where it carries some, framed as the
 * author's, then the form.
 */
export function OwnClientSteps({ guide, server }: { guide: run.OwnClientGuide; server?: string }) {
  return (
    <>
      <p style={{ marginTop: 0 }}>
        <strong>{hostOf(guide.issuer)} lets no app register itself</strong>, so signing in there needs an OAuth client of your own.
        {guide.known ? ` Make one at ${guide.vendor} once; these are Agentry's steps:` : " Make one in the vendor's developer settings:"}
      </p>
      <ol className="own-client-steps selectable">
        {guide.steps.map((s, i) => <li key={i}>{s}</li>)}
      </ol>
      {guide.note && <p className="muted small">{guide.note}</p>}
      {guide.docs && (
        <p className="small">
          <Button variant="link" onClick={() => void OpenExternal(guide.docs!)}>The same steps on the docs page</Button>
        </p>
      )}
      {guide.help && <AuthorHelp name={server || hostOf(guide.issuer)} help={guide.help} />}
    </>
  );
}

export function ServerSignIns({ registry, agentRef, release, onChanged }: {
  registry: string;
  agentRef: string;
  /** The release's servers that sign in, as Go last reported them. */
  release: run.ReleaseConnections;
  /** Asks the panel to read the connections again. */
  onChanged: () => void;
}) {
  // The server whose consent is under way, the address the browser was
  // sent to, and what the consent said.
  const [busy, setBusy] = useState<string | null>(null);
  const [consentURL, setConsentURL] = useState<string | null>(null);
  const [said, setSaid] = useState<string[]>([]);
  const [paste, setPaste] = useState("");
  const [pasteProblem, setPasteProblem] = useState<string | null>(null);
  // The own-client ask, for the server it came from.
  const [own, setOwn] = useState<{ server: string; guide: run.OwnClientGuide } | null>(null);
  const [clientID, setClientID] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<run.ServerConnection | null>(null);
  const live = useRef(true);
  useEffect(() => () => { live.current = false; }, []);

  async function connect(server: string) {
    setBusy(server);
    setError(null);
    setNote(null);
    setConsentURL(null);
    setSaid([]);
    setPaste("");
    setPasteProblem(null);
    const offURL = EventsOn("connect:url", (v: { server: string; url: string }) => {
      if (v.server === server && live.current) setConsentURL(v.url);
    });
    const offSay = EventsOn("connect:say", (v: { server: string; line: string }) => {
      if (v.server === server && live.current) setSaid((s) => [...s, v.line]);
    });
    try {
      const r = await RunAPI.Connect(registry, agentRef, server);
      if (!live.current) return;
      if (r.needsOwnClient) {
        setOwn({ server, guide: r.needsOwnClient });
      } else if (r.connection) {
        setOwn(null);
        setNote(`Connected ${server}. This version's later runs reuse it without asking.`);
        onChanged();
      } else if (r.cancelled) {
        setNote("The sign-in was cancelled. Nothing was kept.");
      }
    } catch (e) {
      if (live.current) setError(String(e));
    } finally {
      offURL();
      offSay();
      if (live.current) {
        setBusy(null);
        setConsentURL(null);
      }
    }
  }

  async function sendPaste() {
    const p = pastedProblem(paste);
    setPasteProblem(p);
    if (p) return;
    try {
      await RunAPI.PasteRedirect(paste.trim());
      setPaste("");
    } catch (e) {
      setPasteProblem(String(e));
    }
  }

  async function keepClient() {
    if (!own) return;
    const issuer = own.guide.issuer;
    const server = own.server;
    const id = clientID.trim();
    const secret = clientSecret;
    // The secret leaves the form before the call returns: nothing of it
    // stays in the page's state.
    setClientSecret("");
    setError(null);
    try {
      await RunAPI.KeepOwnClient(issuer, id, secret);
      setClientID("");
      setOwn(null);
      await connect(server);
    } catch (e) {
      setError(String(e));
    }
  }

  async function disconnect(c: run.ServerConnection) {
    setConfirm(null);
    setError(null);
    setNote(null);
    try {
      const outcome = await RunAPI.Disconnect(release.ref, release.digest, c.server);
      setNote(`Disconnected ${c.server} (revocation at the vendor: ${outcome}).`);
      onChanged();
    } catch (e) {
      setError(String(e));
    }
  }

  return (
    <>
      <h4>Sign-ins</h4>
      <p className="muted small">
        These servers sign you in on the vendor&apos;s own consent screen. What you approve is kept on this computer for this
        version alone, its refresh token in the OS keychain; each run is given an access token, never the refresh token or a
        client secret. Another version, or another QuickAgent, asks again.
      </p>
      <div className="sign-ins">
        {release.servers.map((c) => (
          <Card key={c.server}>
            <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
              <span className="mono selectable">{c.what}</span>
              {c.state === "connected" && busy !== c.server && (
                <span className="row">
                  <Button onClick={() => void connect(c.server)} disabled={busy !== null}>Connect again</Button>
                  <Button variant="danger" onClick={() => setConfirm(c)} disabled={busy !== null}>Disconnect</Button>
                </span>
              )}
              {c.state !== "connected" && !c.unsupported && busy !== c.server && (
                <Button variant="primary" onClick={() => void connect(c.server)} disabled={busy !== null}>Connect</Button>
              )}
            </div>
            <p className="small" role="status" style={{ marginBottom: 0 }}>{connectionLine(c)}</p>
            <p className="muted small" style={{ marginTop: 4 }}>It asks for: {scopeWords(c.scopes)}.</p>
            {busy === c.server && (
              <div className="consent-wait">
                <Notice>Approve the sign-in in your browser. This waits up to ten minutes.</Notice>
                {said.map((l, i) => <p key={i} className="muted small selectable">{l}</p>)}
                {consentURL && (
                  <>
                    <p className="small">If the browser did not open, or it is on another computer, open this address there:</p>
                    <p className="mono small selectable consent-url">{consentURL}</p>
                    <label className="small">
                      If that browser is on another computer, the page it ends on does not load. Paste that page&apos;s whole address here:
                      <Input
                        aria-label="The address the browser ended on"
                        type="text"
                        autoComplete="off"
                        value={paste}
                        placeholder="http://127.0.0.1:…/callback?code=…"
                        onChange={(e) => setPaste(e.target.value)}
                      />
                    </label>
                    {pasteProblem && <p className="small step-problem" role="alert">{pasteProblem}</p>}
                    <div className="row">
                      <Button onClick={() => void sendPaste()} disabled={paste.trim() === ""}>Use this address</Button>
                    </div>
                  </>
                )}
                <div className="row" style={{ justifyContent: "flex-end" }}>
                  <Button onClick={() => void RunAPI.CancelConnect()}>Cancel the sign-in</Button>
                </div>
              </div>
            )}
            {own && own.server === c.server && busy === null && (
              <div className="own-client">
                <OwnClientSteps guide={own.guide} server={c.server} />
                <label className="small">
                  Client ID
                  <Input aria-label="Client ID" type="text" autoComplete="off" value={clientID} onChange={(e) => setClientID(e.target.value)} />
                </label>
                <label className="small">
                  Client secret
                  <Input aria-label="Client secret" type="password" autoComplete="off" value={clientSecret}
                    onChange={(e) => setClientSecret(e.target.value)} />
                </label>
                <p className="muted small">
                  Kept once for {hostOf(own.guide.issuer)} on this computer, the secret in the OS keychain. It is never shown
                  again, never logged, and never sent into a run. Settings → Connections lists it, with Forget.
                </p>
                <div className="row" style={{ justifyContent: "flex-end" }}>
                  <Button onClick={() => { setOwn(null); setClientSecret(""); }}>Cancel</Button>
                  <Button variant="primary" onClick={() => void keepClient()} disabled={clientID.trim() === ""}>Keep it and connect</Button>
                </div>
              </div>
            )}
          </Card>
        ))}
      </div>
      {note && <Notice tone="ok">{note}</Notice>}
      <ErrorBox error={error} />
      {confirm && (
        <Modal title={`Disconnect ${hostOf(confirm.server) || confirm.server}?`} onClose={() => setConfirm(null)}>
          <p>
            The connection of this version to <span className="mono">{confirm.server}</span> is removed from this computer, and
            Agentry asks the vendor to revoke it where the vendor offers that. Its next run asks for your consent again.
          </p>
          {sharedGrantWarning(hostOf(confirm.issuer), confirm.sharesGrantWith) && (
            <Notice tone="warn">{sharedGrantWarning(hostOf(confirm.issuer), confirm.sharesGrantWith)}</Notice>
          )}
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setConfirm(null)}>Cancel</Button>
            <Button variant="danger" onClick={() => void disconnect(confirm)}>Disconnect</Button>
          </div>
        </Modal>
      )}
    </>
  );
}

/** The connection state of conns, for the pure helpers. */
export function asConns(conns: run.ServerConnection[] | null | undefined): ServerConn[] {
  return conns ?? [];
}
