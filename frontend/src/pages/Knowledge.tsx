// The Knowledge page: two cards, as the Build and Voice pages'. Local
// Knowledge opens the packs kept on this computer; Cloud Knowledge the
// packs the account keeps in Agentry, and its organizations' shared packs.
// Each card and page has Create knowledge, preset to its place; a pack is
// copied to the other place with Copy to Cloud or Copy to Local, the
// store's Upload and Download, which never remove the copy they start
// from; a pack kept in both places is on both pages, marked so, and
// Delete on a page removes that place's copy alone.
//
// A pack is kept sealed in the runner home the CLI shares, encrypted to a
// key of its own that the registry wraps for the account, so the words say
// what holds that key, as Go words it by the wrapper's kind
// (internal/knowledge.KeyHolder) — the account's key, which Agentry holds,
// on SecureAgentry; a key a core registry keeps for the account; or, signed
// out, this computer's — and never call a pack "safe", "secure" or "private"
// (the copy check holds the line).
//
// Every gate is in Go (services_knowledge.go): Export and both Deletes are
// asked in native dialogs, Export's folder comes from the native picker, the
// documents of a build only from the native file and folder pickers, and a
// build takes the run lock. This page shows what the bindings answer.
import { type ReactNode, useCallback, useEffect, useState } from "react";
import * as KnowledgeAPI from "../../wailsjs/go/app/KnowledgeAPI";
import * as SettingsAPI from "../../wailsjs/go/app/SettingsAPI";
import { knowledge } from "../../wailsjs/go/models";
import { Button, EmptyState, ErrorBox, Notice, PageHeader, Spinner, fmtBytes, fmtStorage } from "../ui";
import { AGENTRY_MAX_BYTES, fmtCap, pagesOf, scanTone } from "../knowledge";
import {
  CLOUD_CARD_BLURB, KNOWLEDGE_TAGLINE, LOCAL_CARD_BLURB, allocationPhrase, cloudPacks, cloudStatus, knowledgeDay, localPacks, localStatus,
  otherPlace, sharedGroups, type Allocation,
} from "../knowledgeCards";
import { WayCard } from "./BuildWays";
import { CloudKnowledgeFigure, LocalKnowledgeFigure } from "./KnowledgeFigures";
import KnowledgePreview from "./KnowledgePreview";

export type KnowledgeView = "start" | "local" | "cloud";
export type KnowledgePlace = "local" | "agentry";

// A pack made on this computer may hold up to 1 GB; Agentry keeps packs of
// at most 10 MB, so a larger one offers no Copy to Cloud.
const pastAgentryCap = (p: knowledge.Pack) => p.bytes > AGENTRY_MAX_BYTES;
// A copy Agentry is scanning, or that holds nothing yet, cannot be copied here.
const copyableHere = (p: knowledge.Pack) => !p.local && p.agentryScanState !== "scanning" && p.agentryScanState !== "empty";

export default function Knowledge({
  registry, onOpenSettings, onCreate, rev, previewRequest, onPreviewRequestSeen, initialPreview, initialView, initialAllocation,
}: {
  registry: string;
  onOpenSettings: () => void;
  /** Create knowledge, or Add documents to addTo, preset to a place. The
   *  flow lives in App, not here, so a build goes on in its corner card
   *  while the person moves between pages. */
  onCreate: (addTo?: string, place?: KnowledgePlace) => void;
  /** Changes when a build has ended, so the list is read again. */
  rev?: number;
  /** A pack whose preview a finished build asked for, from App, with the
   *  registry the build kept it for. */
  previewRequest?: { registry: string; name: string } | null;
  onPreviewRequestSeen?: () => void;
  /** Harness-only: open with this pack's preview showing, on this page,
   *  with this allocation. */
  initialPreview?: string;
  initialView?: KnowledgeView;
  initialAllocation?: Allocation | null;
}) {
  const [page, setPage] = useState<KnowledgeView>(initialView ?? "start");
  const [list, setList] = useState<knowledge.List | null>(null);
  const [alloc, setAlloc] = useState<Allocation | null>(initialAllocation ?? null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  // The one action in flight, as "<action>:<name>", so its button reads busy.
  const [busy, setBusy] = useState<string | null>(null);
  const [preview, setPreview] = useState<{ registry: string; name: string } | null>(
    initialPreview ? { registry, name: initialPreview } : null,
  );

  const refresh = useCallback(() => {
    KnowledgeAPI.List(registry)
      .then((l) => {
        setList(l);
        setError(null);
        // The allocation is the account view's (Settings › Profile reads
        // the same), asked only signed in; a registry with no tiers states
        // none, and the line goes without it.
        if (l.signedIn && initialAllocation === undefined) {
          SettingsAPI.Profile(registry)
            .then((v) => setAlloc(v.subscription?.knowledge ?? null))
            .catch(() => setAlloc(null));
        }
      })
      .catch((e) => setError(String(e)));
  }, [registry, initialAllocation]);
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
  // Each page reads the list again: Agentry's copies change elsewhere (the
  // site, the CLI, a scan that ended).
  function open(next: KnowledgeView) {
    setNotice(null);
    setError(null);
    setPage(next);
    refresh();
  }

  const packs = list?.packs ?? [];
  const is = (action: string, name: string) => busy === `${action}:${name}`;
  const exportPack = (p: knowledge.Pack) =>
    act("export", p.name, async () => {
      const v = await KnowledgeAPI.Export(registry, p.name);
      return `Exported ${p.name} unencrypted into ${v.dir}. That copy is not encrypted; the one kept here still is.`;
    });
  const copyToCloud = (p: knowledge.Pack) => act("upload", p.name, () => KnowledgeAPI.Upload(registry, p.name));
  const copyToLocal = (p: knowledge.Pack) =>
    act("download", p.name, () => (p.shared && p.org ? KnowledgeAPI.DownloadShared(registry, p.org, p.name) : KnowledgeAPI.Download(registry, p.name)));

  const messages = (
    <>
      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}
      {(list?.problems ?? []).length > 0 && (
        <Notice tone="warn">
          Some entries of the store could not be read, and are not listed:
          <ul className="setup-banner-list">{list!.problems!.map((p) => <li key={p}>{p}</li>)}</ul>
        </Notice>
      )}
      {!list && !error && <Spinner />}
    </>
  );
  const previewModal = preview && <KnowledgePreview registry={preview.registry} name={preview.name} onClose={() => setPreview(null)} />;
  const back = <Button onClick={() => open("start")}>Back to Knowledge</Button>;

  if (page === "local") {
    const here = localPacks(packs);
    return (
      <div data-page="knowledge" data-view="local">
        <PageHeader
          title="Local Knowledge"
          sub={<>{LOCAL_CARD_BLURB} A pack made here holds up to 1 GB.</>}
          actions={<>{back}<Button variant="primary" onClick={() => onCreate(undefined, "local")}>Create knowledge</Button></>}
        />
        {list && (list.signedIn ? (
          <Notice tone="info">
            Packs made here are encrypted to a key {hostOf(registry)} keeps for your account, so they open on any computer you sign in on.
            Each pack below says what holds its key.
          </Notice>
        ) : (
          <Notice tone="warn">
            <strong>Not signed in{hostOf(registry) ? ` to ${hostOf(registry)}` : ""}.</strong> Packs made now are encrypted to this computer&apos;s key, in its
            keychain: they open only on this computer until you sign in, which moves them to a key that registry keeps for your account, and
            Copy to Cloud needs you signed in.{" "}
            <Button variant="link" onClick={onOpenSettings}>Open Settings</Button>
          </Notice>
        ))}
        {messages}
        {list && here.length === 0 && (
          <EmptyState title="No knowledge on this computer yet" action={<Button variant="primary" onClick={() => onCreate(undefined, "local")}>Create knowledge</Button>}>
            Make one from your documents or a brief. The Knowledge Creator runs on this computer against the model your runner policy names,
            for no credits.
          </EmptyState>
        )}
        {here.map((p) => (
          <PackRow
            key={`${p.name} ${p.id}`}
            p={p}
            page="local"
            meta={<>
              Encrypted to {p.key}
              {p.wrapper === "registry" && p.shared && ". Opens only while you are a member and signed in: its unwrapped key is asked for every time, never kept on this computer."}
              {p.wrapper === "registry" && !p.shared && (p.opensOffline ? ". Opens offline: its key is cached in this computer's keychain." : `. Opens once ${p.registry} unwraps its key.`)}
              {p.shared && !p.canExport && " Only its admins export it unencrypted, unless one of them marks it exportable."}
              {p.wrapper === "machine" && ". Opens on this computer only."}
            </>}
            actions={<>
              <Button onClick={() => setPreview({ registry, name: p.name })}>Preview</Button>
              {!p.shared && <Button onClick={() => onCreate(p.name, "local")}>Add documents</Button>}
              {!p.shared && list?.signedIn && (!p.inAgentry || p.changed) && !pastAgentryCap(p) && (
                <Button busy={is("upload", p.name)} busyLabel="Copying…" onClick={() => copyToCloud(p)}>Copy to Cloud</Button>
              )}
              {p.canExport && <Button busy={is("export", p.name)} busyLabel="Exporting…" onClick={() => exportPack(p)}>Export unencrypted…</Button>}
              <Button
                variant="danger"
                busy={is("delete", p.name)}
                busyLabel="Deleting…"
                onClick={() => act("delete", p.name, async () => {
                  await KnowledgeAPI.Delete(registry, p.name);
                  return `Deleted ${p.name} from this computer.${p.inAgentry && !p.shared ? " Agentry keeps its copy." : ""}`;
                })}
              >
                Delete
              </Button>
            </>}
          />
        ))}
        {list && here.length > 0 && (
          <p className="muted small">
            Copy to Cloud keeps a copy in your Agentry account, where AI Review scans it and it counts toward your storage there; the copy here
            stays.
          </p>
        )}
        {previewModal}
      </div>
    );
  }

  if (page === "cloud") {
    const own = cloudPacks(packs);
    const groups = sharedGroups(packs);
    const listed = !!list?.signedIn && !list.agentry;
    return (
      <div data-page="knowledge" data-view="cloud">
        <PageHeader
          title="Cloud Knowledge"
          sub={CLOUD_CARD_BLURB}
          actions={<>{back}<Button variant="primary" onClick={() => onCreate(undefined, "agentry")}>Create knowledge</Button></>}
        />
        {list && !list.signedIn && (
          <Notice tone="info">
            Sign in to <span className="mono">{hostOf(registry) || "a registry"}</span> to keep knowledge in your account, where AI Review scans it and
            runs In Agentry can search it. <Button variant="link" onClick={onOpenSettings}>Open Settings</Button>
          </Notice>
        )}
        {list?.signedIn && list.agentry && <Notice tone="warn">{list.agentry}</Notice>}
        {messages}
        {listed && alloc && <p className="small" data-testid="knowledge-allocation">Storage in Agentry: {allocationPhrase(alloc, fmtStorage)}.</p>}
        {listed && own.length === 0 && (
          <EmptyState title="No knowledge in your account yet" action={<Button variant="primary" onClick={() => onCreate(undefined, "agentry")}>Create knowledge</Button>}>
            Create it In Agentry, or Copy to Cloud a pack from Local Knowledge. AI Review scans every pack Agentry keeps.
          </EmptyState>
        )}
        {own.map((p) => (
          <PackRow
            key={`${p.name} ${p.agentryId}`}
            p={p}
            page="cloud"
            meta={<>Kept in Agentry{p.local ? "; a copy is kept on this computer too." : "; Copy to Local to attach it to a run on this computer or to read it here."}</>}
            actions={<>
              {p.local && <Button onClick={() => setPreview({ registry, name: p.name })}>Preview</Button>}
              {/* Not while Agentry scans it, nor for a copy here changed since it
                  was copied: the build would start from pages this computer
                  does not hold (Go's hostedTarget refuses it too). */}
              {p.agentryScanState !== "scanning" && !(p.local && p.changed) && <Button onClick={() => onCreate(p.name, "agentry")}>Add documents</Button>}
              {copyableHere(p) && (
                <Button busy={is("download", p.name)} busyLabel="Copying…" onClick={() => copyToLocal(p)}>Copy to Local</Button>
              )}
              <Button
                variant="danger"
                busy={is("delete-agentry", p.name)}
                busyLabel="Deleting…"
                onClick={() => act("delete-agentry", p.name, async () => {
                  await KnowledgeAPI.DeleteFromAgentry(registry, p.agentryId!);
                  return `Deleted ${p.name} from Agentry.${p.local ? " The copy on this computer stays." : ""}`;
                })}
              >
                Delete
              </Button>
            </>}
          />
        ))}
        {groups.map((g) => (
          <section key={g.org} className="knowledge-shared" aria-labelledby={`shared-${g.org}`}>
            <h2 id={`shared-${g.org}`} className="knowledge-shared-title">Shared by {g.org}</h2>
            {g.packs.map((p) => (
              <PackRow
                key={`${p.name} ${p.agentryId}`}
                p={p}
                page="cloud"
                meta={<>
                  {g.org}&apos;s shared pack: its members read it, its admins change it in Agentry.
                  {p.local ? " A copy is kept on this computer, which opens only while you are a member and signed in." : ""}
                </>}
                actions={<>
                  {p.local && <Button onClick={() => setPreview({ registry, name: p.name })}>Preview</Button>}
                  {copyableHere(p) && (
                    <Button busy={is("download", p.name)} busyLabel="Copying…" onClick={() => copyToLocal(p)}>Copy to Local</Button>
                  )}
                </>}
              />
            ))}
          </section>
        ))}
        {list?.shared && <p className="muted small">{list.shared}</p>}
        {listed && own.length > 0 && (
          <p className="muted small">Copy to Local keeps a copy on this computer; the copy in Agentry stays.</p>
        )}
        {previewModal}
      </div>
    );
  }

  return (
    <div data-page="knowledge" data-view="start">
      <PageHeader title="Knowledge" sub={KNOWLEDGE_TAGLINE} />
      {messages}
      <div className="build-ways" data-testid="knowledge-ways">
        <WayCard title="Local Knowledge" blurb={LOCAL_CARD_BLURB} figure={<LocalKnowledgeFigure />} onOpen={() => open("local")}>
          {list && <span data-testid="knowledge-local-status">{localStatus(packs, fmtStorage)}</span>}{" "}
          <button type="button" className="link build-way-part" onClick={() => onCreate(undefined, "local")}>Create knowledge</button>
        </WayCard>
        <WayCard title="Cloud Knowledge" blurb={CLOUD_CARD_BLURB} figure={<CloudKnowledgeFigure />} onOpen={() => open("cloud")}>
          {list && <span data-testid="knowledge-cloud-status">{cloudStatus(list, packs, fmtStorage, alloc)}</span>}{" "}
          <button type="button" className="link build-way-part" onClick={() => onCreate(undefined, "agentry")}>Create knowledge</button>
        </WayCard>
      </div>
      {previewModal}
    </div>
  );
}

// PackRow is one pack on a place's page: what it is, its scan, the badges
// about the other place, its size and date there, and its actions on their
// own line under it (five beside the text would leave it a narrow column).
function PackRow({ p, page, meta, actions }: { p: knowledge.Pack; page: "local" | "cloud"; meta: ReactNode; actions: ReactNode }) {
  // Each page shows its own place's copy: on the Cloud page, Agentry's scan,
  // pages and size, which for a pack changed here are the older copy's.
  const cloud = page === "cloud";
  const updated = knowledgeDay(cloud ? p.agentryUpdated : p.updated);
  const scan = cloud ? p.agentryScan || p.scan : p.scan;
  const tone = scanTone(cloud ? { ...p, scanState: p.agentryScanState || p.scanState } as knowledge.Pack : p);
  const pages = cloud && p.agentryScanState ? p.agentryPages ?? 0 : p.pages;
  const bytes = cloud && p.agentryScanState ? p.agentryBytes ?? 0 : p.bytes;
  return (
    <div className="agent-row knowledge-row">
      <div className="agent-row-main">
        <div className="agent-row-head">
          <span className="agent-row-title mono">{p.name}</span>
          <span className="row agent-row-badges">
            <span className={`badge ${tone}`.trim()}>{scan}</span>
            {otherPlace(p, page).map((b) => (
              <span key={b.label} className={`badge ${b.tone}`.trim()} title={b.title}>{b.label}</span>
            ))}
            {p.shared && page === "local" && (
              <span className="badge kind" title="an organization's shared pack: its members read it, its admins change it in Agentry">shared by {p.org}</span>
            )}
            {p.shared && p.exportable && <span className="badge">members may export</span>}
          </span>
        </div>
        <div className="agent-row-summary muted">
          {pagesOf(pages)} · {fmtBytes(bytes)}
          {updated && <> · updated {updated}</>}
        </div>
        {page === "local" && !p.shared && pastAgentryCap(p) && (
          <div className="muted small">
            Past the {fmtCap(AGENTRY_MAX_BYTES)} Agentry keeps, so it stays on this computer: it cannot be copied to the Cloud, shared or attached
            to a run In Agentry.
          </div>
        )}
        <div className="agent-row-meta muted small">
          {meta}
          {p.note && <> {p.note}</>}
        </div>
        <div className="row knowledge-row-actions">{actions}</div>
      </div>
    </div>
  );
}

function hostOf(registry: string): string {
  return registry.replace(/^https?:\/\//, "");
}
