// Settings → Connections (plan mcp-oauth-connections.md D17, D20, D23;
// agentry-desktop#666): every connection kept on this computer, each for
// one QuickAgent version and one MCP server, with Disconnect, which removes
// it and asks the vendor to revoke it where the vendor offers that; and the
// person's own OAuth clients, one per vendor, with Forget. The CLI keeps the
// same store (`agentry connections`), so what either made is listed here.
// Nothing here carries a token or a client secret: Go's views have none.
import { useEffect, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { main, run } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Modal, Notice } from "../ui";
import { clientWords, dateOf, hostOf, scopeWords } from "../connections";
export { connectionsStatus } from "../connections";

export function ConnectionsSettings() {
  const [view, setView] = useState<main.KeptConnectionsView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState<{ conn?: run.KeptConnection; client?: run.KeptClient } | null>(null);

  const read = () => {
    RunAPI.Connections().then(setView).catch((e) => setError(String(e)));
  };
  useEffect(read, []);

  async function disconnect(c: run.KeptConnection) {
    setConfirm(null);
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      const outcome = await RunAPI.Disconnect(c.ref, c.digest, c.server);
      setNote(`Disconnected ${c.ref} from ${c.server} (revocation at the vendor: ${outcome}).`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
      read();
    }
  }

  async function forget(c: run.KeptClient) {
    setConfirm(null);
    setError(null);
    setNote(null);
    try {
      await RunAPI.ForgetOwnClient(c.issuer);
      setNote(`Forgot your OAuth client for ${hostOf(c.issuer)}. The connections made with it ask again at their next run.`);
    } catch (e) {
      setError(String(e));
    } finally {
      read();
    }
  }

  return (
    <>
      <section id="settings-part-connections" className="settings-part">
        <h2>Connections</h2>
        <p className="muted small">
          Each one is an OAuth grant you approved on a vendor&apos;s consent screen, for one QuickAgent version and one MCP
          server. It is kept on this computer, its refresh token in the OS keychain, and reused by that version&apos;s runs;
          each run is given an access token, never the refresh token. The run setup&apos;s Connect tools step makes them, and
          so does <span className="mono">agentry connect</span>.
        </p>
        {view && view.connections.length === 0 && <p className="muted small">No connections kept.</p>}
        {view && view.connections.length > 0 && (
          <div className="sign-ins">
            {view.connections.map((c) => (
              <Card key={`${c.ref}|${c.digest}|${c.server}`}>
                <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
                  <span className="selectable"><span className="mono">{c.ref}</span> → <span className="mono">{c.server}</span></span>
                  <Button variant="danger" onClick={() => setConfirm({ conn: c })} disabled={busy}>Disconnect</Button>
                </div>
                <p className="small" style={{ marginBottom: 0 }}>
                  {c.state === "broken"
                    ? `No longer works (${c.broken}). Its next run asks for consent again.`
                    : c.label ? `Connected as ${c.label}.` : "Connected."}
                </p>
                <p className="muted small selectable" style={{ marginTop: 4 }}>
                  Signs in at {hostOf(c.issuer)}{clientWords(c.client) ? `, as ${clientWords(c.client)}` : ""} · scopes {scopeWords(c.scopes)}
                  {" · "}since {dateOf(c.createdAt)} · {c.lastUsedAt ? `last used ${dateOf(c.lastUsedAt)}` : "not used yet"}
                  {" · "}kept in the {c.keptIn === "keychain" ? "OS keychain" : "runner home, in a file only you can read"}
                </p>
              </Card>
            ))}
          </div>
        )}
      </section>
      <section id="settings-part-own-clients" className="settings-part">
        <h2>Your OAuth clients</h2>
        <p className="muted small">
          A vendor that lets no app register itself, such as Google, needs an OAuth client of your own. You keep one per vendor
          on this computer, and every QuickAgent version you connect there uses it. Its secret is in the OS keychain and is never
          shown here.
        </p>
        {view && view.clients.length === 0 && <p className="muted small">None kept. Connect tools asks for one when a vendor needs it.</p>}
        {view && view.clients.length > 0 && (
          <div className="sign-ins">
            {view.clients.map((c) => (
              <Card key={c.issuer}>
                <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
                  <span className="selectable"><span className="mono">{c.issuer}</span></span>
                  <Button variant="danger" onClick={() => setConfirm({ client: c })}>Forget</Button>
                </div>
                <p className="muted small selectable" style={{ marginBottom: 0 }}>
                  Client <span className="mono">{c.clientId}</span> · {c.secretIn === "keychain" ? "secret kept in the OS keychain" : c.secretIn === "file" ? "secret kept in a file only you can read" : "no secret"}
                </p>
              </Card>
            ))}
          </div>
        )}
      </section>
      {note && <Notice tone="ok">{note}</Notice>}
      <ErrorBox error={error} />
      {confirm?.conn && (
        <Modal title="Disconnect?" onClose={() => setConfirm(null)}>
          <p>
            The connection of <span className="mono">{confirm.conn.ref}</span> to <span className="mono">{confirm.conn.server}</span> is
            removed from this computer, and Agentry asks {hostOf(confirm.conn.issuer)} to revoke it where it offers that. That
            version&apos;s next run asks for your consent again.
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setConfirm(null)}>Cancel</Button>
            <Button variant="danger" onClick={() => void disconnect(confirm.conn!)}>Disconnect</Button>
          </div>
        </Modal>
      )}
      {confirm?.client && (
        <Modal title="Forget this OAuth client?" onClose={() => setConfirm(null)}>
          <p>
            Your client for <span className="mono">{confirm.client.issuer}</span> is removed from this computer; it stays at the
            vendor, where you made it. The connections made with it stop working and ask again at their next run, when you can
            enter a client again.
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setConfirm(null)}>Cancel</Button>
            <Button variant="danger" onClick={() => void forget(confirm.client!)}>Forget</Button>
          </div>
        </Modal>
      )}
    </>
  );
}
