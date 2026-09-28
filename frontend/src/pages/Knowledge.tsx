// The Knowledge panel (plan knowledge.md §4.8; agentry-desktop#273, D-039):
// the knowledge packs kept on this computer and, signed in, the ones the
// account keeps in Agentry. A pack is kept sealed in the runner home the
// CLI shares, encrypted to a key of its own that the registry wraps for
// the account, so the words say what holds that key, as Go words it by the
// wrapper's kind (internal/knowledge.KeyHolder) — the account's key, which
// Agentry holds, on SecureAgentry; a key a core registry keeps for the
// account; or, signed out, this computer's — and none of the words plan §3
// rules out (the copy check holds the line).
//
// Every gate is in Go (services_knowledge.go): Export and Delete are asked
// in native dialogs, Export's folder comes from the native picker, the
// documents of a build only from the native file and folder pickers, and a
// build takes the run lock. This page shows what the bindings answer.
import { useCallback, useEffect, useState } from "react";
import * as KnowledgeAPI from "../../wailsjs/go/main/KnowledgeAPI";
import { knowledge } from "../../wailsjs/go/models";
import { Button, EmptyState, ErrorBox, Modal, Notice, PageHeader, Spinner, fmtBytes } from "../ui";
import { AGENTRY_MAX_BYTES, pagesOf, scanTone } from "../knowledge";

// A pack made on this computer may hold up to 1 GB; Agentry keeps packs of
// at most 10 MB, so a larger one offers no Upload (agentry-desktop#354).
const pastAgentryCap = (p: knowledge.Pack) => p.bytes > AGENTRY_MAX_BYTES;
import KnowledgePreview from "./KnowledgePreview";

export default function Knowledge({
  registry, onOpenSettings, onCreate, rev, previewRequest, onPreviewRequestSeen, initialPreview,
}: {
  registry: string;
  onOpenSettings: () => void;
  /** Create knowledge, or Add documents to addTo. The flow lives in App, not
   *  here, so a build goes on in its corner card while the person moves
   *  between pages (agentry-desktop#353). */
  onCreate: (addTo?: string) => void;
  /** Changes when a build has ended, so the list is read again. */
  rev?: number;
  /** A pack whose preview a finished build asked for, from App, with the
   *  registry the build kept it for. */
  previewRequest?: { registry: string; name: string } | null;
  onPreviewRequestSeen?: () => void;
  /** Harness-only: open with this pack's preview showing. */
  initialPreview?: string;
}) {
  const [list, setList] = useState<knowledge.List | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  // The one action in flight, as "<action>:<name>", so its button reads busy.
  const [busy, setBusy] = useState<string | null>(null);
  const [preview, setPreview] = useState<{ registry: string; name: string } | null>(
    initialPreview ? { registry, name: initialPreview } : null,
  );
  // A pack kept in both places asks here which copies to delete; the native
  // dialog then asks again, in Go.
  const [deleting, setDeleting] = useState<knowledge.Pack | null>(null);

  const refresh = useCallback(() => {
    KnowledgeAPI.List(registry)
      .then((l) => {
        setList(l);
        setError(null);
      })
      .catch((e) => setError(String(e)));
  }, [registry]);
  useEffect(refresh, [refresh, rev]);
  useEffect(() => {
    if (previewRequest) {
      setPreview(previewRequest);
      onPreviewRequestSeen?.();
    }
  }, [previewRequest, onPreviewRequestSeen]);

  async function act(action: string, name: string, call: () => Promise<string | void>) {
    setBusy(`${action}:${name}`);
    setError(null);
    setNotice(null);
    try {
      const msg = await call();
      if (msg) setNotice(msg);
      refresh();
    } catch (e) {
      const s = String(e);
      // A No in the native dialog, or a cancelled picker, is the person's
      // choice, not a failure.
      if (s.includes("not confirmed") || s.includes("cancelled")) setNotice("Nothing was changed.");
      else setError(s);
    } finally {
      setBusy(null);
    }
  }

  const exportPack = (p: knowledge.Pack) =>
    act("export", p.name, async () => {
      const v = await KnowledgeAPI.Export(registry, p.name);
      return `Exported ${p.name} unencrypted into ${v.dir}. That copy is not encrypted; the one kept here still is.`;
    });
  const deletePack = (p: knowledge.Pack, everywhere: boolean) => {
    setDeleting(null);
    return act("delete", p.name, async () => {
      await KnowledgeAPI.Delete(registry, p.name, everywhere);
      return everywhere ? `Deleted ${p.name} from this computer and from Agentry.` : `Deleted ${p.name} from this computer.`;
    });
  };

  const host = registry.replace(/^https?:\/\//, "");
  const packs = list?.packs ?? [];
  const is = (action: string, name: string) => busy === `${action}:${name}`;

  return (
    <div data-page="knowledge">
      <PageHeader
        title="Knowledge"
        sub={<>Knowledge packs a run can search: cited pages the Knowledge Creator compiles from your documents, kept with the documents themselves. A pack made on this computer holds up to 1 GB; Agentry keeps packs of at most 10 MB. The <span className="mono">agentry knowledge</span> CLI keeps the same packs.</>}
        actions={<Button variant="primary" onClick={() => onCreate()}>Create knowledge</Button>}
      />

      {list && (list.signedIn ? (
        <Notice tone="info">
          Packs made here are kept on this computer, encrypted to a key {host} keeps for your account, so they open on any
          computer you sign in on. Each pack below says what holds its key.
        </Notice>
      ) : (
        <Notice tone="warn">
          <strong>Not signed in{host ? ` to ${host}` : ""}.</strong> Packs made now are encrypted to this computer&apos;s key, in its
          keychain: they open only on this computer until you sign in, which moves them to a key that registry keeps for your account.{" "}
          <Button variant="link" onClick={onOpenSettings}>Open Settings</Button>
        </Notice>
      ))}
      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}
      {(list?.problems ?? []).length > 0 && (
        <Notice tone="warn">
          Some entries of the store could not be read, and are not listed:
          <ul className="setup-banner-list">{list!.problems!.map((p) => <li key={p}>{p}</li>)}</ul>
        </Notice>
      )}
      {!list && !error && <Spinner />}

      {list && packs.length === 0 && (
        <EmptyState title="No knowledge packs yet" action={<Button variant="primary" onClick={() => onCreate()}>Create knowledge</Button>}>
          Make one from your documents. The Knowledge Creator runs on this computer against the model your runner policy names.
        </EmptyState>
      )}

      {packs.map((p) => {
        // The row's actions sit on their own line under what the pack is:
        // five of them beside the text would leave it a narrow column.
        // An organization's shared pack (Knowledge M8) is read here and
        // written in Agentry by its admins: no Add documents or Upload,
        // Export only as the organization allows, and Delete removes this
        // computer's copy alone.
        const actions = p.local ? (
          <>
            <Button onClick={() => setPreview({ registry, name: p.name })}>Preview</Button>
            {!p.shared && <Button onClick={() => onCreate(p.name)}>Add documents</Button>}
            {!p.shared && list?.signedIn && (!p.inAgentry || p.changed) && !pastAgentryCap(p) && (
              <Button busy={is("upload", p.name)} busyLabel="Uploading…" onClick={() => act("upload", p.name, () => KnowledgeAPI.Upload(registry, p.name))}>
                Upload to Agentry
              </Button>
            )}
            {p.canExport && (
              <Button busy={is("export", p.name)} busyLabel="Exporting…" onClick={() => exportPack(p)}>Export unencrypted…</Button>
            )}
            <Button variant="danger" busy={is("delete", p.name)} busyLabel="Deleting…" onClick={() => (p.inAgentry && !p.shared ? setDeleting(p) : deletePack(p, false))}>
              Delete
            </Button>
          </>
        ) : (
          list?.signedIn && p.scanState !== "scanning" && p.scanState !== "empty" && (
            <Button
              busy={is("download", p.name)}
              busyLabel="Downloading…"
              onClick={() => act("download", p.name, () => (p.shared && p.org ? KnowledgeAPI.DownloadShared(registry, p.org, p.name) : KnowledgeAPI.Download(registry, p.name)))}
            >
              Download
            </Button>
          )
        );
        return (
          <div key={`${p.name} ${p.local ? p.id : p.agentryId}`} className="agent-row knowledge-row">
            <div className="agent-row-main">
              <div className="agent-row-head">
                <span className="agent-row-title mono">{p.name}</span>
                <span className="row agent-row-badges">
                  <span className={`badge ${scanTone(p)}`.trim()}>{p.scan}</span>
                  <span className="badge">{p.where}</span>
                  {p.shared && <span className="badge kind" title="an organization's shared pack: its members read it, its admins change it in Agentry">shared by {p.org}</span>}
                  {p.shared && p.exportable && <span className="badge">members may export</span>}
                  {p.changed && <span className="badge warn" title="kept in both places, and changed on this computer since its upload">changed since upload</span>}
                </span>
              </div>
              <div className="agent-row-summary muted">{pagesOf(p.pages)} · {fmtBytes(p.bytes)}</div>
              {p.local && !p.shared && pastAgentryCap(p) && (
                <div className="muted small">
                  Past the 10 MB Agentry keeps, so it stays on this computer: it cannot be uploaded, shared or attached to a run In Agentry.
                </div>
              )}
              <div className="agent-row-meta muted small">
                {p.local ? (
                  <>
                    Encrypted to {p.key}
                    {p.wrapper === "registry" && p.shared && ". Opens only while you are a member and signed in: its unwrapped key is asked for every time, never kept on this computer."}
                    {p.wrapper === "registry" && !p.shared && (p.opensOffline ? ". Opens offline: its key is cached in this computer's keychain." : `. Opens once ${p.registry} unwraps its key.`)}
                    {p.shared && !p.canExport && " Only its admins export it unencrypted, unless one of them marks it exportable."}
                    {p.wrapper === "machine" && ". Opens on this computer only."}
                  </>
                ) : (
                  <>{p.shared ? `Shared by ${p.org} in Agentry` : "Kept in Agentry"}{list?.signedIn && p.scanState !== "scanning" && p.scanState !== "empty" ? "; download it to attach it to a run on this computer." : "."}</>
                )}
                {p.note && <> {p.note}</>}
              </div>
              {actions && <div className="row knowledge-row-actions">{actions}</div>}
            </div>
          </div>
        );
      })}
      {list?.agentry && <p className="muted small">{list.agentry}</p>}
      {list?.shared && <p className="muted small">{list.shared}</p>}

      {deleting && (
        <Modal title={`Delete ${deleting.name}?`} onClose={() => setDeleting(null)}>
          <p>
            Agentry keeps a copy of this pack. Delete it from this computer only, and Download brings it back; or delete
            both copies, and neither can be recovered.
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setDeleting(null)}>Cancel</Button>
            <Button onClick={() => deletePack(deleting, false)}>From this computer</Button>
            <Button variant="danger" onClick={() => deletePack(deleting, true)}>From this computer and Agentry</Button>
          </div>
        </Modal>
      )}
      {preview && <KnowledgePreview registry={preview.registry} name={preview.name} onClose={() => setPreview(null)} />}
    </div>
  );
}
