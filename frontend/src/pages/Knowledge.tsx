// The Knowledge panel (plan knowledge.md §4.8; agentry-desktop#273, D-039):
// the knowledge packs kept on this computer and, signed in, the ones the
// account keeps in Agentry. A pack is kept sealed in the runner home the
// CLI shares, encrypted to a key of its own that the registry wraps for
// the account, so the words say what holds that key — "your account's key,
// which Agentry holds", or this computer's while signed out — and none of
// the words plan §3 rules out (the copy check holds the line).
//
// Every gate is in Go (services_knowledge.go): Export and Delete are asked
// in native dialogs, Export's folder comes from the native picker, the
// documents of a build only from the native file and folder pickers, and a
// build takes the run lock. This page shows what the bindings answer.
import { useCallback, useEffect, useState } from "react";
import * as KnowledgeAPI from "../../wailsjs/go/main/KnowledgeAPI";
import { knowledge } from "../../wailsjs/go/models";
import { Button, EmptyState, ErrorBox, Modal, Notice, PageHeader, Spinner, fmtBytes } from "../ui";
import { pagesOf, scanTone } from "../knowledge";
import KnowledgeCreate from "./KnowledgeCreate";
import KnowledgePreview from "./KnowledgePreview";

type Flow = { mode: "create" } | { mode: "add"; name: string };

export default function Knowledge({
  registry, onOpenSettings, initialFlow, initialPreview,
}: {
  registry: string;
  onOpenSettings: () => void;
  /** Harness-only: open with Create knowledge or Add documents showing. */
  initialFlow?: Flow;
  /** Harness-only: open with this pack's preview showing. */
  initialPreview?: string;
}) {
  const [list, setList] = useState<knowledge.List | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  // The one action in flight, as "<action>:<name>", so its button reads busy.
  const [busy, setBusy] = useState<string | null>(null);
  const [flow, setFlow] = useState<Flow | null>(initialFlow ?? null);
  const [preview, setPreview] = useState<string | null>(initialPreview ?? null);
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
  useEffect(refresh, [refresh]);

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
        sub={<>Knowledge packs a run can search: cited pages the Knowledge Creator compiles from your documents, kept with the documents themselves, at most 10 MB each. The <span className="mono">agentry knowledge</span> CLI keeps the same packs.</>}
        actions={<Button variant="primary" onClick={() => setFlow({ mode: "create" })}>Create knowledge</Button>}
      />

      {list && (list.signedIn ? (
        <Notice tone="info">
          Packs made here are kept on this computer, encrypted to a key {host} keeps for your account, so they open on any
          computer you sign in on. Each pack below says what holds its key.
        </Notice>
      ) : (
        <Notice tone="warn">
          <strong>Not signed in{host ? ` to ${host}` : ""}.</strong> Packs made now are encrypted to this computer&apos;s key, in its
          keychain: they open only on this computer until you sign in, which moves them to your account&apos;s key.{" "}
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
        <EmptyState title="No knowledge packs yet" action={<Button variant="primary" onClick={() => setFlow({ mode: "create" })}>Create knowledge</Button>}>
          Make one from your documents. The Knowledge Creator runs on this computer against the model your runner policy names.
        </EmptyState>
      )}

      {packs.map((p) => {
        // The row's actions sit on their own line under what the pack is:
        // five of them beside the text would leave it a narrow column.
        const actions = p.local ? (
          <>
            <Button onClick={() => setPreview(p.name)}>Preview</Button>
            <Button onClick={() => setFlow({ mode: "add", name: p.name })}>Add documents</Button>
            {list?.signedIn && (!p.inAgentry || p.changed) && (
              <Button busy={is("upload", p.name)} busyLabel="Uploading…" onClick={() => act("upload", p.name, () => KnowledgeAPI.Upload(registry, p.name))}>
                Upload to Agentry
              </Button>
            )}
            <Button busy={is("export", p.name)} busyLabel="Exporting…" onClick={() => exportPack(p)}>Export unencrypted…</Button>
            <Button variant="danger" busy={is("delete", p.name)} busyLabel="Deleting…" onClick={() => (p.inAgentry ? setDeleting(p) : deletePack(p, false))}>
              Delete
            </Button>
          </>
        ) : (
          list?.signedIn && p.scanState !== "scanning" && p.scanState !== "empty" && (
            <Button busy={is("download", p.name)} busyLabel="Downloading…" onClick={() => act("download", p.name, () => KnowledgeAPI.Download(registry, p.name))}>
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
                  {p.changed && <span className="badge warn" title="kept in both places, and changed on this computer since its upload">changed since upload</span>}
                </span>
              </div>
              <div className="agent-row-summary muted">{pagesOf(p.pages)} · {fmtBytes(p.bytes)}</div>
              <div className="agent-row-meta muted small">
                {p.local ? (
                  <>
                    Encrypted to {p.key}
                    {p.wrapper === "registry" && (p.opensOffline ? ". Opens offline: its key is cached in this computer's keychain." : `. Opens once ${p.registry} unwraps its key.`)}
                    {p.wrapper === "machine" && ". Opens on this computer only."}
                  </>
                ) : (
                  <>Kept in Agentry{list?.signedIn && p.scanState !== "scanning" && p.scanState !== "empty" ? "; download it to attach it to a run on this computer." : "."}</>
                )}
                {p.note && <> {p.note}</>}
              </div>
              {actions && <div className="row knowledge-row-actions">{actions}</div>}
            </div>
          </div>
        );
      })}
      {list?.agentry && <p className="muted small">{list.agentry}</p>}

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
      {flow && (
        <KnowledgeCreate
          registry={registry}
          addTo={flow.mode === "add" ? flow.name : undefined}
          onClose={() => {
            setFlow(null);
            refresh();
          }}
          onPreview={(name) => {
            setFlow(null);
            refresh();
            setPreview(name);
          }}
        />
      )}
      {preview && <KnowledgePreview registry={registry} name={preview} onClose={() => setPreview(null)} />}
    </div>
  );
}
