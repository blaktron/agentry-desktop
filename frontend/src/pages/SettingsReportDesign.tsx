import { useEffect, useRef, useState } from "react";
import * as SettingsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/settingsapi";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import * as reportdesign from "../../bindings/github.com/blaktron/agentry/pkg/reportdesign/models";
import { Button, Card, ErrorBox, Modal, Notice, Select, Textarea } from "../ui";
import {
  canonical, COVER_FACT_LABELS, COVER_STYLE_LABELS, COVER_WHEN_LABELS, cssColour, edit, fieldLabel, fontsFor, HEADER_STYLE_LABELS,
  HEADING_LABELS, HEADING_SCALE_LABELS, insertToken, isImageSrc, isLogoSrc, KIT_LOOKS, labelOf, librarySentence, MARGIN_LABELS,
  fallbackSentence, newFromKit, PAGE_NUMBER_LABELS, renderSentence, PAPER_LABELS, QUOTE_LABELS, ratioText, sameDesign, savedSentence, showFact, SLOT_NAMES,
  startFrom, TITLE_BLOCK_LABELS, toDocument, TOKEN_LABELS, tokenText, type FontRole, type ReportDesign,
} from "../reportDesign";
import { ReportDesignPreview } from "./ReportDesignPreview";
import { PdfViewer } from "../PdfViewer";
import * as DesignerAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/designerapi";
import { EventsOn } from "../host";
import { DesignWithAI } from "./DesignWithAI";
import { followDesignerHosted, showDesignerHosted, takeDesignerAsk, useDesignerAsk, useDesignerRun } from "../designerRun";
import type { DesignerJob } from "../designer";

// Settings → Report design: the account's library of report designs and the
// editor for one of them, the same library the site edits. The library is
// cards, one per design, with the cover thumbnail of the kit it started
// from, its name, whether it is the default, and Use, Edit, Duplicate and
// Delete; New from a kit offers the ten built-in kits as thumbnails the
// runner's template drew, and loads one whole into a new design. The editor's
// controls are exactly the fields of version 3 (reportDesign.ts CONTROLS);
// beside them a live HTML likeness. The core's pkg/reportdesign judges every
// state through CheckReportDesign: the contrast guard refuses a colour the
// core would refuse and offers the nearest passing shade, and Save waits
// until the design passes. There is no form element, so Enter in a field
// presses nothing.
//
// The images have parts of their own: Choose opens the system's file picker
// in Go and uploads what is picked, Remove takes it off and keeps the design.
// Save never sends them, so the core keeps them. The preview and the parts
// draw each only as the data: URL Go's view built from the core's answer.
// Brand from logo asks the core to read the logo's two colours and offers
// them; nothing applies them until the person takes them.
//
// Design with AI (DesignWithAI.tsx) runs the built-in Report Designer, In
// Agentry or on this computer: from the library for a whole design, from the
// editor's custom cover card for a cover page of the design open there. A
// run In Agentry is followed here, so the page says it is under way when
// its dialog is closed, and a library opened again finds it.
//
// Render lays the sample report out in the design the editor holds, unsaved
// changes included, through the runner's own PDF export on this computer,
// and shows the real pages in the app's PDF viewer. Go keeps the PDF once it
// passes the checks a run's PDF passes and hands the viewer its bytes by the
// render's token; the page never chooses them. A design whose own cover was
// not drawn says so, with the style drawn in its place.
/** What the page is doing against the registry, one thing at a time. */
type Busy = "" | "save" | "default" | "delete" | "duplicate" | "create" | "logo" | "unlogo" | "cover" | "uncover" | "uncustom" | "suggest" | "open";

export function ReportDesignSettings({ registry, signedIn, onOpenRegistry }: {
  registry: string;
  signedIn: boolean;
  onOpenRegistry: () => void;
}) {
  const [opts, setOpts] = useState<app.ReportDesignOptions | null>(null);
  const [lib, setLib] = useState<app.ReportDesignLibrary | null>(null);
  const [view, setView] = useState<app.ReportDesignView | null>(null);
  const [d, setD] = useState<ReportDesign | null>(null);
  const [check, setCheck] = useState<{ key: string; c: app.ReportDesignCheck } | null>(null);
  const [busy, setBusy] = useState<Busy>("");
  const [err, setErr] = useState<string | null>(null);
  const [said, setSaid] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<app.ReportDesignCard | null>(null);
  const [newKit, setNewKit] = useState<{ id: string; name: string; makeDefault: boolean } | null>(null);
  const [suggestion, setSuggestion] = useState<app.ReportColourSuggestion | null>(null);
  const [designAI, setDesignAI] = useState<{ job: DesignerJob; from: string } | null>(null);
  const [hosted, setHosted] = useState<app.DesignerHostedView | null>(null);

  useEffect(() => {
    let live = true;
    SettingsAPI.ReportDesignOptions().then((o) => { if (live) setOpts(o); }).catch((e) => { if (live) setErr(String(e)); });
    return () => { live = false; };
  }, []);

  const readLibrary = (reg: string) =>
    SettingsAPI.ReportDesignLibrary(reg).then((l) => { setLib(l); return l; });
  useEffect(() => {
    if (!signedIn) return;
    let live = true;
    setLib(null);
    setView(null);
    setD(null);
    SettingsAPI.ReportDesignLibrary(registry)
      .then((l) => { if (live) setLib(l); })
      .catch((e) => { if (live) setErr(String(e)); });
    return () => { live = false; };
  }, [registry, signedIn]);

  // A Report Designer run In Agentry: followed through its events, and
  // found again when the page opens while one is under way.
  // Found under way, it is also followed in its run window, as its card.
  useEffect(() => EventsOn("designer:hosted", (v: app.DesignerHostedView) => setHosted(v)), []);
  useEffect(() => {
    if (!signedIn) return;
    DesignerAPI.AttachHosted(registry).then((r) => {
      if (!r.run) return;
      setHosted(r.run);
      if (!r.run.final) followDesignerHosted(hostedMeta(registry, r.run), r.run, true);
    }).catch(() => {});
  }, [registry, signedIn]);
  // The run window's ask: Edit it opens a design the run made in the
  // editor; Try again opens Design with AI for the same job.
  const designerAsk = useDesignerAsk();
  useEffect(() => {
    // Only once the page draws the library: before then this render
    // returned early, and the editor's open is not there to call.
    if (!designerAsk || !signedIn || !opts || !lib?.available) return;
    const a = takeDesignerAsk();
    if (!a) return;
    if ("edit" in a) void open(a.edit);
    else setDesignAI({ job: a.again, from: "" });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [designerAsk, signedIn, opts, lib?.available]);
  // A Report Designer run that lands, or a design it made added from its
  // run window, changes the library.
  const designerLanded = useDesignerRun().landed;
  useEffect(() => {
    if (designerLanded > 0 && signedIn) void SettingsAPI.ReportDesignLibrary(registry).then(setLib).catch(() => {});
  }, [designerLanded, registry, signedIn]);
  const hostedSaved = hosted?.final && hosted.applied ? hosted.designId : "";
  useEffect(() => {
    if (hostedSaved && signedIn) void SettingsAPI.ReportDesignLibrary(registry).then(setLib).catch(() => {});
  }, [hostedSaved, registry, signedIn]);

  // The core's verdict on each state; only the latest counts.
  const seq = useRef(0);
  useEffect(() => {
    if (!d) return;
    const n = ++seq.current;
    const key = canonical(d);
    SettingsAPI.CheckReportDesign(reportdesign.Design.createFrom(toDocument(d)))
      .then((c) => { if (n === seq.current) setCheck({ key, c }); })
      .catch((e) => setErr(String(e)));
  }, [d]);

  if (!signedIn) {
    return (
      <Card>
        <p>Your report designs are kept on your account. Runs from this app use Plain while you are not signed in.</p>
        <Button onClick={onOpenRegistry}>Sign in</Button>
      </Card>
    );
  }
  if (lib && !lib.available) {
    return <Notice tone="info">This registry does not keep report designs. Runs from this app use Plain.</Notice>;
  }
  if (!opts || !lib) {
    return (
      <>
        <ErrorBox error={err} />
        {!err && <p className="muted small">Reading your report designs…</p>}
      </>
    );
  }

  const kitOf = (id: string) => opts.kits.find((k) => k.id === id);
  const kitName = (id: string) => kitOf(id)?.design.name ?? id;
  const defaultName = lib.designs.find((x) => x.id === lib.default)?.name ?? null;
  const full = lib.designs.length >= lib.max;

  // An action against the registry: its failure is shown, its answer taken.
  async function act<T>(kind: Busy, run: () => Promise<T>, then: (v: T) => void) {
    setBusy(kind);
    setErr(null);
    setSaid(null);
    try {
      then(await run());
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy("");
    }
  }
  const openView = (v: app.ReportDesignView) => {
    setView(v);
    setSuggestion(null);
    setD(v.design ? startFrom(v.design) : null);
  };
  const open = (id: string) => act("open", () => SettingsAPI.OpenReportDesign(registry, id), openView);
  const close = () => { setView(null); setD(null); setSaid(null); setErr(null); setSuggestion(null); void readLibrary(registry).catch((e) => setErr(String(e))); };
  const use = (id: string) => act("default", () => SettingsAPI.SetDefaultReportDesign(registry, id), (l) => {
    setLib(l);
    const name = l.designs.find((x) => x.id === id)?.name;
    setSaid(id ? `${name} is your default: runs that pick no design use it.` : "No default: runs that pick no design use Plain.");
    if (view && view.id === id) setView({ ...view, default: true });
    else if (view) setView({ ...view, default: false });
  });
  const duplicate = (id: string) => act("duplicate", () => SettingsAPI.DuplicateReportDesign(registry, id), (v) => {
    void readLibrary(registry).catch((e) => setErr(String(e)));
    setSaid(`${v.design?.name ?? "The copy"} is added to your library, with the original's images.`);
  });
  const remove = (card: app.ReportDesignCard) => act("delete", () => SettingsAPI.DeleteReportDesign(registry, card.id), (l) => {
    setConfirmDelete(null);
    setLib(l);
    if (view?.id === card.id) { setView(null); setD(null); }
    setSaid(`${card.name} is deleted${card.default ? ", and you have no default: runs that pick no design use Plain" : ""}.`);
  });
  const create = () => {
    if (!newKit) return;
    const kit = kitOf(newKit.id);
    if (!kit) return;
    const doc = newFromKit(kit.design as unknown as ReportDesign, newKit.name);
    void act("create", () => SettingsAPI.CreateReportDesign(registry, reportdesign.Design.createFrom(toDocument(doc)), newKit.makeDefault), (v) => {
      setNewKit(null);
      void readLibrary(registry).catch((e) => setErr(String(e)));
      openView(v);
      setSaid(`${v.design?.name ?? "The design"} is added to your library${v.default ? " as your default" : ""}. Edit it here, or go back to the library.`);
    });
  };

  return (
    <div className="report-design">
      {!view ? (
        <Library lib={lib} opts={opts} busy={busy} err={err} said={said} defaultName={defaultName} full={full}
          kitName={kitName} kitOf={kitOf}
          onOpen={(id) => void open(id)} onUse={(id) => void use(id)} onDuplicate={(id) => void duplicate(id)}
          onDelete={setConfirmDelete} onNew={() => setNewKit({ id: opts.defaultKitId, name: "", makeDefault: lib.designs.length === 0 })}
          hosted={designAI ? null : hosted} onDesignAI={() => setDesignAI({ job: "design", from: "" })}
          onShowHosted={() => { if (hosted) showDesignerHosted(hostedMeta(registry, hosted), hosted); }} />
      ) : (
        <Editor registry={registry} opts={opts} lib={lib} view={view} d={d} setD={setD} check={check} busy={busy} err={err} said={said}
          suggestion={suggestion} setSuggestion={setSuggestion} kitName={kitName}
          onClose={close} onUse={(id) => void use(id)} onDelete={() => setConfirmDelete(lib.designs.find((x) => x.id === view.id) ?? { ...app.ReportDesignCard.createFrom({ id: view.id, name: view.design?.name ?? "This design", basedOn: view.design?.basedOn ?? "", version: 0, default: view.default, hasLogo: false, hasCoverImage: false, hasCustomCover: false, updatedAt: "" }) })}
          onSaved={(v) => { setView(v); if (v.design) setD(startFrom(v.design)); setSaid(savedSentence(v.label ?? v.design?.name ?? "the design", v.default)); void readLibrary(registry).catch((e) => setErr(String(e))); }}
          act={act} onCoverAI={() => setDesignAI({ job: "cover", from: view.id })} />
      )}

      {designAI && (
        <DesignWithAI registry={registry} lib={lib} initialJob={designAI.job} initialFrom={designAI.from}
          hosted={hosted} onHosted={setHosted}
          onClose={() => setDesignAI(null)} />
      )}

      {confirmDelete && (
        <Modal title={`Delete ${confirmDelete.name}?`} onClose={() => setConfirmDelete(null)}>
          <p>
            {confirmDelete.name} is removed from your library with its logo, its cover image and its custom cover.
            {confirmDelete.default ? " It is your default: runs that pick no design then use Plain." : ""} A run that already drew it keeps its PDFs.
          </p>
          <div className="row">
            <Button onClick={() => setConfirmDelete(null)} disabled={!!busy}>Cancel</Button>
            <Button variant="danger" busy={busy === "delete"} busyLabel="Deleting…" disabled={!!busy} onClick={() => void remove(confirmDelete)}>Delete</Button>
          </div>
        </Modal>
      )}

      {newKit && (
        <Modal title="New from a kit" wide onClose={() => setNewKit(null)}>
          <p className="muted small">
            A kit is a whole look: faces, colours, title block, header, footer and cover page, drawn here by the runner's own template from the
            same sample report. Choose one and name your design; every setting can be changed after.
          </p>
          <ul className="report-design-kits" role="list">
            {opts.kits.map((k) => {
              const current = newKit.id === k.id;
              return (
                <li key={k.id}>
                  <button type="button" className={`report-design-kit${current ? " current" : ""}`} data-kit={k.id} aria-pressed={current}
                    disabled={!!busy} onClick={() => setNewKit({ ...newKit, id: k.id })}>
                    <span className="report-design-kit-pages">
                      {k.cover && <img src={k.cover} alt={`The ${k.design.name} kit's cover page`} />}
                      {k.page && <img src={k.page} alt={`The ${k.design.name} kit's first page`} />}
                    </span>
                    <strong>{k.design.name}</strong>
                    <span className="muted small">{KIT_LOOKS[k.id] ?? ""}</span>
                  </button>
                </li>
              );
            })}
          </ul>
          <label className="field">
            <span className="field-label">Name</span>
            <input type="text" value={newKit.name} maxLength={opts.maxName} placeholder={kitName(newKit.id)} autoFocus
              onChange={(e) => setNewKit({ ...newKit, name: e.target.value })} />
            <span className="muted small">A run's receipt names the design it used by this. Left empty, the kit's own name is used.</span>
          </label>
          <label className="check-row">
            <input type="checkbox" checked={newKit.makeDefault} onChange={(e) => setNewKit({ ...newKit, makeDefault: e.target.checked })} />
            <span>Make it my default: runs that pick no design use it</span>
          </label>
          <ErrorBox error={err} />
          <div className="row">
            <Button onClick={() => setNewKit(null)} disabled={!!busy}>Cancel</Button>
            <Button variant="primary" busy={busy === "create"} busyLabel="Adding…" disabled={!!busy || full} onClick={create}>
              Add to my library
            </Button>
          </div>
        </Modal>
      )}
    </div>
  );
}

// Library is the cards.
function Library({ lib, opts, busy, err, said, defaultName, full, kitName, kitOf, onOpen, onUse, onDuplicate, onDelete, onNew, hosted, onDesignAI, onShowHosted }: {
  lib: app.ReportDesignLibrary;
  opts: app.ReportDesignOptions;
  busy: string;
  err: string | null;
  said: string | null;
  defaultName: string | null;
  full: boolean;
  kitName: (id: string) => string;
  kitOf: (id: string) => app.ReportDesignKit | undefined;
  onOpen: (id: string) => void;
  onUse: (id: string) => void;
  onDuplicate: (id: string) => void;
  onDelete: (card: app.ReportDesignCard) => void;
  onNew: () => void;
  /** The Report Designer run In Agentry the page follows, while its dialog is closed. */
  hosted: app.DesignerHostedView | null;
  onDesignAI: () => void;
  /** Brings up the run window of the run In Agentry under way. */
  onShowHosted: () => void;
}) {
  const hostedRunning = !!hosted && !hosted.final;
  return (
    <section id="settings-part-library" className="settings-part">
      <h2>Your library</h2>
      <Card>
        <p className="muted small">
          How the PDFs your runs export look. A run makes a PDF when you ask for one, or when its report is meant for someone else; the
          QuickAgent chooses only the document's form, a report, a letter or a memo, never the look. Up to {lib.max} designs, one of them the
          default; a run's setup lets you pick another.
        </p>
        <div className="row report-design-actions">
          <Button variant="primary" disabled={!!busy || full} onClick={onNew}>New from a kit…</Button>
          <span className="report-design-ai">
            <Button disabled={!!busy || full} onClick={onDesignAI} data-design-ai>Design with AI…</Button>
            <span className="muted small">The Report Designer writes a design from your brief, your logo and the pages you show it, In Agentry or on this computer.</span>
          </span>
        </div>
        {hostedRunning && (
          <Notice tone="info">
            The Report Designer is running In Agentry: {hosted.statusLine}{" "}
            <Button variant="link" onClick={onShowHosted}>Show it</Button>
          </Notice>
        )}
        {hosted?.final && (
          <Notice tone={hosted.applied ? "ok" : "warn"}>
            {hosted.applied
              ? `The Report Designer saved “${hosted.name ?? "its design"}” to your library. It is not your default: Use makes it so.`
              : hosted.error || hosted.note || hosted.outcomeLine || hosted.statusLine}
          </Notice>
        )}
        {full && <p className="muted small" data-library-full>Your library holds {lib.max} designs, the most it can: delete one to add another.</p>}
        {lib.designs.length > 0 && (
          <ul className="report-design-cards" role="list">
            {lib.designs.map((c) => {
              const kit = kitOf(c.basedOn);
              return (
                <li key={c.id} className={`card report-design-card${c.default ? " default" : ""}`} data-design={c.id}>
                  {kit?.cover ? (
                    <img className="report-design-card-cover" src={kit.cover} alt={`The ${kitName(c.basedOn)} kit's cover page, which ${c.name} starts from`} />
                  ) : (
                    <div className="report-design-card-cover report-design-card-nocover" aria-hidden />
                  )}
                  <div className="report-design-card-body">
                    <strong>{c.name}</strong>
                    <span className="muted small">From {kitName(c.basedOn)}, revision {c.version}.</span>
                    <span className="report-design-card-badges">
                      {c.default && <span className="badge kind">Default</span>}
                      {c.hasLogo && <span className="badge">Logo</span>}
                      {c.hasCoverImage && <span className="badge">Cover image</span>}
                      {c.hasCustomCover && <span className="badge">Custom cover</span>}
                    </span>
                    <span className="row report-design-card-actions">
                      <Button variant="link" disabled={!!busy || c.default} onClick={() => onUse(c.id)} aria-label={`Use ${c.name} as the default`}>
                        {c.default ? "In use" : "Use"}
                      </Button>
                      <Button variant="link" disabled={!!busy} onClick={() => onOpen(c.id)} aria-label={`Edit ${c.name}`}>Edit</Button>
                      <Button variant="link" disabled={!!busy || full} onClick={() => onDuplicate(c.id)} aria-label={`Duplicate ${c.name}`}>Duplicate</Button>
                      <Button variant="link" disabled={!!busy} onClick={() => onDelete(c)} aria-label={`Delete ${c.name}`}>Delete</Button>
                    </span>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
        <ErrorBox error={err} />
        {said && <Notice tone="ok">{said}</Notice>}
        <p className="muted small" data-standing>{librarySentence(lib.designs.length, defaultName, lib.max)}</p>
      </Card>
    </section>
  );
}

// Editor is one design's controls beside the preview, and its images.
function Editor({ registry, opts, view, d, setD, check, busy, err, said, suggestion, setSuggestion, kitName, onClose, onUse, onDelete, onSaved, act, onCoverAI }: {
  registry: string;
  opts: app.ReportDesignOptions;
  lib: app.ReportDesignLibrary;
  view: app.ReportDesignView;
  d: ReportDesign | null;
  setD: (f: (x: ReportDesign | null) => ReportDesign | null) => void;
  check: { key: string; c: app.ReportDesignCheck } | null;
  busy: string;
  err: string | null;
  said: string | null;
  suggestion: app.ReportColourSuggestion | null;
  setSuggestion: (s: app.ReportColourSuggestion | null) => void;
  kitName: (id: string) => string;
  onClose: () => void;
  onUse: (id: string) => void;
  onDelete: () => void;
  onSaved: (v: app.ReportDesignView) => void;
  act: <T>(kind: Busy, run: () => Promise<T>, then: (v: T) => void) => Promise<void>;
  onCoverAI: () => void;
}) {
  const [rendering, setRendering] = useState(false);
  const back = (
    <div className="row report-design-back">
      <Button onClick={onClose} disabled={!!busy}>Back to the library</Button>
    </div>
  );
  if (!d) {
    return (
      <section id="settings-part-design-editor" className="settings-part">
        {back}
        <Notice tone="warn">
          This design is one this app cannot read ({view.unreadable || "unknown document"}). Runs from this app that pick it use Plain until the app
          is updated. The site may still edit it.
        </Notice>
        <div className="row"><Button variant="danger" disabled={!!busy} onClick={onDelete}>Delete</Button></div>
      </section>
    );
  }
  const c = check?.key === canonical(d) ? check.c : null;
  const set = (field: string, v: string | boolean) => setD((x) => (x ? edit(x, field, v) : x));
  const unchangedSinceSave = !!view.design && sameDesign(d, view.design as unknown as ReportDesign);
  const problem = c?.problem && c.problem.field !== "accent" && c.problem.field !== "secondary" ? c.problem : null;
  const colourProblem = c?.problem && (c.problem.field === "accent" || c.problem.field === "secondary") ? c.problem : null;
  const id = view.id;

  const save = () => act("save", () => SettingsAPI.SaveReportDesign(registry, id, reportdesign.Design.createFrom(toDocument(d))), onSaved);
  const image = (kind: "logo" | "unlogo" | "cover" | "uncover" | "uncustom") => act(kind, () => {
    switch (kind) {
      case "logo": return SettingsAPI.ChooseReportLogo(registry, id);
      case "unlogo": return SettingsAPI.RemoveReportLogo(registry, id);
      case "cover": return SettingsAPI.ChooseReportCoverImage(registry, id);
      case "uncover": return SettingsAPI.RemoveReportCoverImage(registry, id);
      default: return SettingsAPI.RemoveReportCustomCover(registry, id);
    }
  }, (v) => {
    if (!v) return; // the picker was cancelled
    onSaved({ ...v, design: v.design } as app.ReportDesignView);
  });
  const suggest = () => act("suggest", () => SettingsAPI.SuggestReportColours(registry, id), setSuggestion);

  return (
    <>
      {back}
      <div className="report-design-grid">
        <section id="settings-part-design-editor" className="settings-part">
          <h2>{d.name}</h2>
          <Card>
            <p className="muted small" data-starts-from>
              From {kitName(d.basedOn)}, revision {d.version}.{" "}
              {c ? (c.unedited ? "Unchanged from the kit." : "Changed: Save keeps your changes, and the kit stays as it is.") : ""}
              {view.default ? " Your default." : ""}
            </p>

            <label className="field">
              <span className="field-label">Name</span>
              <input type="text" value={d.name} maxLength={opts.maxName} onChange={(e) => set("name", e.target.value)} />
              <span className="muted small">A run's receipt names the design it used by this.</span>
            </label>

            <fieldset className="report-design-set">
              <legend className="field-label">Brand</legend>
              <label className="field">
                <span>Name</span>
                <input type="text" value={d.brand.name} maxLength={opts.maxName} placeholder="None" onChange={(e) => set("brand.name", e.target.value)} />
                <span className="muted small">What {tokenText("brand")} writes in a header, a footer or a cover.</span>
              </label>
              <label className="field">
                <span>Strapline</span>
                <input type="text" value={d.brand.strapline} maxLength={opts.maxLine} placeholder="None" onChange={(e) => set("brand.strapline", e.target.value)} />
              </label>
              <label className="field">
                <span>Contact lines</span>
                <Textarea value={d.brand.contact.join("\n")} rows={3} onChange={(e) => set("brand.contact", e.target.value)} />
                <span className="muted small">Up to {opts.maxContactLines} lines of {opts.maxLine} characters, one per line, drawn on a cover that shows your brand.</span>
              </label>
              <label className="field">
                <span>Website</span>
                <input type="text" value={d.brand.website} maxLength={opts.maxLine} placeholder="example.com" spellCheck={false}
                  onChange={(e) => set("brand.website", e.target.value)} />
                <span className="muted small">A host name or an https address, printed as text, never a link.</span>
              </label>
              <label className="field">
                <span>Confidentiality line</span>
                <input type="text" value={d.brand.confidentiality} maxLength={opts.maxLine} placeholder="None"
                  onChange={(e) => set("brand.confidentiality", e.target.value)} />
                <span className="muted small">What {tokenText("confidentiality")} writes, for example Internal, or Confidential.</span>
              </label>
            </fieldset>

            <fieldset className="report-design-set">
              <legend className="field-label">Colours</legend>
              <div className="row">
                <span className="report-design-colour-name">Accent</span>
                <input type="color" aria-label="Accent colour picker" value={cssColour(d.accent)} onChange={(e) => set("accent", e.target.value)} />
                <input type="text" className="mono report-design-hex" aria-label="Accent colour, #rrggbb" value={d.accent} maxLength={7}
                  spellCheck={false} onChange={(e) => set("accent", e.target.value)} />
              </div>
              {c && <ColourLine which="accent" colour={d.accent} check={c.accent} onUse={(shade) => set("accent", shade)} />}
              <div className="row">
                <span className="report-design-colour-name">Secondary</span>
                <input type="color" aria-label="Secondary colour picker" value={cssColour(d.secondary, "#ffffff")} onChange={(e) => set("secondary", e.target.value)} />
                <input type="text" className="mono report-design-hex" aria-label="Secondary colour, #rrggbb, or empty for none" value={d.secondary} maxLength={7}
                  spellCheck={false} placeholder="None" onChange={(e) => set("secondary", e.target.value)} />
                {d.secondary !== "" && <Button variant="link" onClick={() => set("secondary", "")}>None</Button>}
              </div>
              {c && d.secondary !== "" && <ColourLine which="secondary" colour={d.secondary} check={c.secondary} onUse={(shade) => set("secondary", shade)} />}
              <span className="muted small">
                Body text stays near-black whatever the colours. The tint behind table headers and callouts is the accent at 12% on white; the
                secondary colours a split cover's panel and a geometric cover's shapes.
              </span>
              <div className="row">
                <Button busy={busy === "suggest"} busyLabel="Reading…" disabled={!!busy || !isLogoSrc(view.logo)} onClick={() => void suggest()}
                  title={isLogoSrc(view.logo) ? undefined : "Choose a logo first"}>
                  Brand from logo
                </Button>
                <span className="muted small">Reads your logo's two main colours and offers them here; nothing changes until you take them.</span>
              </div>
              {suggestion && (
                <p className="small report-design-suggestion" data-suggestion>
                  Your logo reads as <Swatch colour={suggestion.accent} /> {suggestion.accent}
                  {suggestion.secondary ? <> and <Swatch colour={suggestion.secondary} /> {suggestion.secondary}</> : " and one colour only"}
                  {suggestion.fromAccent !== suggestion.accent || (suggestion.secondary && suggestion.fromSecondary !== suggestion.secondary)
                    ? ", darkened where needed to read on white" : ""}.{" "}
                  <Button variant="link" onClick={() => { set("accent", suggestion.accent); if (suggestion.secondary) setD((x) => (x ? edit(edit(x, "accent", suggestion.accent), "secondary", suggestion.secondary) : x)); }}>
                    Use {suggestion.secondary ? "both" : "it"}
                  </Button>
                  {suggestion.secondary && <> · <Button variant="link" onClick={() => set("accent", suggestion.accent)}>Accent only</Button></>}
                  {" "}· <Button variant="link" onClick={() => setSuggestion(null)}>Dismiss</Button>
                </p>
              )}
            </fieldset>

            <fieldset className="report-design-set">
              <legend className="field-label">Fonts</legend>
              {(["headings", "body", "code"] as FontRole[]).map((role) => (
                <label key={role} className="row report-design-font">
                  <span>{role === "headings" ? "Headings" : role === "body" ? "Body" : "Code"}</span>
                  <Select value={d.fonts[role]} onChange={(e) => set(`fonts.${role}`, e.target.value)} aria-label={`${role} font`}>
                    {fontsFor(role, opts).map((f) => (
                      <option key={f} value={f}>
                        {f}{role === "headings" && opts.families.find((x) => x.name === f)?.headingsOnly ? " (headings only)" : ""}
                      </option>
                    ))}
                  </Select>
                </label>
              ))}
              <span className="muted small">A script a face lacks, Chinese or Arabic say, is set in Noto in the PDF.</span>
            </fieldset>

            <Radios legend="Paper" field="paper" value={d.paper} values={opts.papers} labels={PAPER_LABELS} onChange={set} inline />
            <Radios legend="Title block" field="titleBlock" value={d.titleBlock} values={opts.titleBlocks} labels={TITLE_BLOCK_LABELS} onChange={set} />
            <Radios legend="Headings" field="headings" value={d.headings} values={opts.headingStyles} labels={HEADING_LABELS} onChange={set} />
            <Radios legend="Heading scale" field="headingScale" value={d.headingScale} values={opts.headingScales} labels={HEADING_SCALE_LABELS} onChange={set} inline />
            <Radios legend="Margins" field="margins" value={d.margins} values={opts.margins} labels={MARGIN_LABELS} onChange={set} inline />

            <fieldset className="report-design-set">
              <legend className="field-label">Tables</legend>
              <label className="row">
                <input type="checkbox" checked={d.tables.tinted} onChange={(e) => set("tables.tinted", e.target.checked)} />
                Tint the header row
              </label>
              <label className="row">
                <input type="checkbox" checked={d.tables.banded} onChange={(e) => set("tables.banded", e.target.checked)} />
                Tint every other row
              </label>
            </fieldset>

            <Radios legend="Quotes" field="quotes" value={d.quotes} values={opts.quoteStyles} labels={QUOTE_LABELS} onChange={set} />

            <fieldset className="report-design-set" data-part="header">
              <legend className="field-label">Header</legend>
              <Radios legend="Style" field="header.style" value={d.header.style} values={opts.headerStyles} labels={HEADER_STYLE_LABELS} onChange={set} sub />
              <span className="muted small">Drawn from page 2 on a report, on every page of a memo, never on a letter.</span>
              <Slots which="header" slots={d.header.slots} tokens={opts.tokens} max={opts.maxLine} onChange={set} />
            </fieldset>

            <fieldset className="report-design-set" data-part="footer">
              <legend className="field-label">Footer</legend>
              <Radios legend="Page numbers" field="footer.pageNumbers" value={d.footer.pageNumbers} values={opts.pageNumbers}
                labels={PAGE_NUMBER_LABELS} onChange={set} inline sub />
              <Slots which="footer" slots={d.footer.slots} tokens={opts.tokens} max={opts.maxLine} onChange={set} />
            </fieldset>

            <fieldset className="report-design-set" data-part="cover">
              <legend className="field-label">Cover page</legend>
              <Radios legend="Style" field="cover.style" value={d.cover.style} values={opts.coverStyles.map((s) => s.name)} labels={COVER_STYLE_LABELS}
                onChange={set} sub disabledValues={view.customCover ? [] : ["custom"]} />
              {d.cover.style === "plate" && !isImageSrc(view.coverImage) && (
                <span className="muted small">Without a cover image, a plate cover draws as a band.</span>
              )}
              <fieldset className="report-design-set inline">
                <legend className="small">Shows</legend>
                {opts.coverFacts.map((fact) => (
                  <label key={fact} className="row">
                    <input type="checkbox" checked={d.cover.shows.includes(fact)}
                      onChange={(e) => setD((x) => (x ? showFact(x, opts.coverFacts, fact, e.target.checked) : x))} />
                    {labelOf(COVER_FACT_LABELS, fact)}
                  </label>
                ))}
              </fieldset>
              <Radios legend="When" field="cover.when" value={d.cover.when} values={opts.coverWhens} labels={COVER_WHEN_LABELS} onChange={set} inline sub />
              <span className="muted small">A long report is a titled report past the length the runner decides; a letter and a memo never get a cover.</span>
            </fieldset>

            {problem && (
              <p role="alert" className="report-design-problem">
                This design cannot be saved yet: {fieldLabel(problem.field)} {problem.message}.
              </p>
            )}
            {colourProblem && !problem && (
              <p className="small muted" data-colour-holds>Save waits until {fieldLabel(colourProblem.field)} passes the contrast rule above.</p>
            )}
            <ErrorBox error={err} />
            <div className="row">
              <Button variant="primary" busy={busy === "save"} busyLabel="Saving…"
                disabled={!c || !!c.problem || !!busy || unchangedSinceSave} onClick={() => void save()}>
                Save
              </Button>
              {!view.default && (
                <Button busy={busy === "default"} busyLabel="Setting…" disabled={!!busy} onClick={() => onUse(id)}>Use as my default</Button>
              )}
              <Button variant="danger" disabled={!!busy} onClick={onDelete}>Delete</Button>
            </div>
            {said && <Notice tone="ok">{said}</Notice>}
          </Card>
        </section>

        <section id="settings-part-design-preview" className="settings-part report-design-preview">
          <h2>Preview</h2>
          <Card>
            <p className="muted small">A likeness drawn from the controls; the kits' pictures are the real render.</p>
            <div className="report-design-paper">
              <ReportDesignPreview design={d} check={c ?? check?.c ?? null} families={opts.families} coverStyles={opts.coverStyles}
                logo={view.logo ?? ""} coverImage={view.coverImage ?? ""} customCover={view.customCover?.thumbnail ?? ""} />
            </div>
            <div className="row report-design-render">
              <Button variant="primary" disabled={!!busy || !c || !!c.problem} onClick={() => setRendering(true)}>Render the sample…</Button>
            </div>
            <p className="muted small">
              Render lays a sample report out in this design on this computer, through the same export a run's PDF goes through, and shows
              the real pages. Unsaved changes are included; nothing is saved.
            </p>
          </Card>
        </section>
      </div>
      {rendering && <RenderModal registry={registry} id={id} design={d} onClose={() => setRendering(false)} />}

      <section id="settings-part-design-images" className="settings-part">
        <h2>Images</h2>
        <div className="report-design-images">
          <Card>
            <h3>Logo</h3>
            <p className="muted small">
              Drawn at the right of the title on the first page of every PDF, on a cover that shows it, and small at the left of a logo-left
              header. On a band it sits on a small white ground. The QuickAgent never chooses or changes it.
            </p>
            {isLogoSrc(view.logo) ? <img className="report-design-logo" src={view.logo} alt="Your logo" /> : <p className="muted small">No logo yet.</p>}
            <div className="row">
              <Button variant="primary" busy={busy === "logo"} busyLabel="Uploading…" disabled={!!busy} onClick={() => void image("logo")}>
                {isLogoSrc(view.logo) ? "Replace the logo…" : "Choose a logo…"}
              </Button>
              {isLogoSrc(view.logo) && (
                <Button variant="danger" busy={busy === "unlogo"} busyLabel="Removing…" disabled={!!busy} onClick={() => void image("unlogo")}>
                  Remove the logo
                </Button>
              )}
            </div>
            <p className="muted small">
              A PNG or a JPEG of at most 512 KB. The registry decodes it, scales it to fit 1024 pixels each way and encodes it again from the
              pixels, so nothing of the file but its picture is kept. SVG is not accepted. Saving the design keeps the logo.
            </p>
          </Card>
          <Card>
            <h3>Cover image</h3>
            <p className="muted small">Drawn across the top half of a plate cover, and by a custom cover that asks for it.</p>
            {isImageSrc(view.coverImage) ? <img className="report-design-cover-image" src={view.coverImage} alt="Your cover image" /> : <p className="muted small">No cover image yet.</p>}
            <div className="row">
              <Button variant="primary" busy={busy === "cover"} busyLabel="Uploading…" disabled={!!busy} onClick={() => void image("cover")}>
                {isImageSrc(view.coverImage) ? "Replace the cover image…" : "Choose a cover image…"}
              </Button>
              {isImageSrc(view.coverImage) && (
                <Button variant="danger" busy={busy === "uncover"} busyLabel="Removing…" disabled={!!busy} onClick={() => void image("uncover")}>
                  Remove the cover image
                </Button>
              )}
            </div>
            <p className="muted small">A PNG or a JPEG of at most 2 MB, scaled to fit 2048 pixels each way and encoded again from the pixels.</p>
          </Card>
          <Card>
            <h3>Custom cover</h3>
            {view.customCover ? (
              <>
                <p className="muted small">
                  This design has a cover of its own, {view.customCover.sourceBytes.toLocaleString()} characters of code
                  {view.customCover.images > 0 ? ` with ${view.customCover.images} image${view.customCover.images === 1 ? "" : "s"}` : ""}, drawn when the
                  cover style is Custom. One that cannot be drawn falls back to the band style, and the run's receipt says so.
                </p>
                {isLogoSrc(view.customCover.thumbnail) ? (
                  <img className="report-design-custom-cover" src={view.customCover.thumbnail} alt="Your custom cover's last render" />
                ) : (
                  <p className="muted small">No picture of it yet.</p>
                )}
                <div className="row">
                  <Button disabled={!!busy} onClick={onCoverAI}>Another cover with AI…</Button>
                  <Button variant="danger" busy={busy === "uncustom"} busyLabel="Removing…" disabled={!!busy} onClick={() => void image("uncustom")}>
                    Remove the custom cover
                  </Button>
                </div>
              </>
            ) : (
              <>
                <p className="muted small" data-no-custom-cover>
                  None. A custom cover is written in code by the Report Designer. Design a cover page with AI makes a new design: this one with
                  that cover. This design stays as it is.
                </p>
                <div className="row">
                  <Button disabled={!!busy} onClick={onCoverAI} data-cover-ai>Design a cover page with AI…</Button>
                </div>
              </>
            )}
          </Card>
        </div>
      </section>
    </>
  );
}

// RenderModal is one render of the sample in the design: it starts when the
// modal opens and stops when the modal closes; the PDF is drawn by the app's
// viewer from the bytes Go kept.
function RenderModal({ registry, id, design, onClose }: { registry: string; id: string; design: ReportDesign; onClose: () => void }) {
  const [rendered, setRendered] = useState<app.ReportRenderView | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [doc] = useState(() => toDocument(design));
  useEffect(() => {
    let live = true;
    SettingsAPI.RenderReportDesign(registry, id, reportdesign.Design.createFrom(doc))
      .then((v) => { if (live) setRendered(v); })
      .catch((e) => { if (live) setErr(String(e)); });
    return () => {
      live = false;
      void SettingsAPI.CancelReportRender();
    };
  }, [registry, id, doc]);
  return (
    <Modal title={`Render: ${design.name}`} wide onClose={onClose}>
      {!rendered && !err && (
        <p className="muted" data-rendering>
          Laying the sample report out in {design.name} on this computer… The first render can take a few minutes while the runner's image is
          prepared.
        </p>
      )}
      <ErrorBox error={err ? `The sample could not be rendered: ${err}` : null} />
      {rendered && (
        <>
          <p className="muted small" data-rendered>{renderSentence(rendered)}</p>
          {rendered.fallback && <Notice tone="warn">{fallbackSentence(rendered)}</Notice>}
          <div className="report-render-viewer">
            <PdfViewer runId="report-render" path={rendered.token} name={`${design.name}, the sample report`} load={renderedPDF} />
          </div>
        </>
      )}
      <div className="row">
        <Button onClick={onClose}>Close</Button>
      </div>
    </Modal>
  );
}

// renderedPDF is the viewer's source for a render: the bytes Go kept, by
// the render's token.
const renderedPDF = (_runId: string, token: string) => SettingsAPI.ReportRenderPDF(token);

// ColourLine is the contrast guard for one colour: its ratio on white and
// what it needs, or the core's refusal with the nearest shade that passes.
function ColourLine({ which, colour, check, onUse }: { which: "accent" | "secondary"; colour: string; check: app.ReportColourCheck; onUse: (shade: string) => void }) {
  const use = check.onText ? "colours text" : "colours only graphics";
  const name = which === "accent" ? "accent" : "secondary";
  if (!check.valid) {
    return <p className="small report-design-problem" role="alert">Write the {name} as #rrggbb, for example #1f6feb{which === "secondary" ? ", or leave it empty for none" : ""}.</p>;
  }
  if (check.ok) {
    return <p className="muted small" id={`${which}-contrast`}>{ratioText(check.ratio)} on white. This design's {name} {use}, which needs {check.need.toFixed(1)}:1.</p>;
  }
  return (
    <p className="small report-design-problem" id={`${which}-contrast`} role="alert">
      {colour} reaches {ratioText(check.ratio)} on white, and a {name} that {use} needs {check.need.toFixed(1)}:1, so it cannot be
      saved. The nearest shade that does is {check.nearest}.{" "}
      <Button variant="link" onClick={() => onUse(check.nearest)}>Use {check.nearest}</Button>
    </p>
  );
}

function Swatch({ colour }: { colour: string }) {
  return <span className="report-design-swatch" aria-hidden style={{ background: cssColour(colour) }} />;
}

// Slots are a header's or a footer's three templates, each with a token
// picker that writes a token where the caret is.
function Slots({ which, slots, tokens, max, onChange }: {
  which: "header" | "footer";
  slots: string[];
  tokens: string[];
  max: number;
  onChange: (field: string, v: string) => void;
}) {
  const refs = useRef<(HTMLInputElement | null)[]>([]);
  return (
    <div className="report-design-slots">
      {SLOT_NAMES.map((name, i) => {
        const field = `${which}.slots.${i}`;
        const value = slots[i] ?? "";
        return (
          <div key={name} className="report-design-slot">
            <label className="field">
              <span>{name}</span>
              <input type="text" value={value} maxLength={max} placeholder="Empty" spellCheck={false} ref={(el) => { refs.current[i] = el; }}
                onChange={(e) => onChange(field, e.target.value)} />
            </label>
            <Select aria-label={`Insert a token into the ${which}'s ${name.toLowerCase()} slot`} value="" className="report-design-token-picker"
              onChange={(e) => {
                const token = e.target.value;
                if (!token) return;
                const el = refs.current[i];
                const at = el?.selectionStart ?? value.length;
                onChange(field, insertToken(value, at, token));
                e.target.value = "";
              }}>
              <option value="">Insert…</option>
              {tokens.map((t) => <option key={t} value={t}>{tokenText(t)}: {TOKEN_LABELS[t] ?? t}</option>)}
            </Select>
          </div>
        );
      })}
      <span className="muted small">
        Text and tokens, up to {max} characters each, printed as written: a token such as {tokenText("page")} is replaced, anything else in braces
        is kept.
      </span>
    </div>
  );
}

function Radios({ legend, field, value, values, labels, onChange, inline = false, sub = false, disabledValues = [] }: {
  legend: string;
  field: string;
  value: string;
  values: string[];
  labels: Record<string, string>;
  onChange: (field: string, v: string) => void;
  inline?: boolean;
  sub?: boolean;
  disabledValues?: string[];
}) {
  return (
    <fieldset className={`report-design-set${inline ? " inline" : ""}`}>
      <legend className={sub ? "small" : "field-label"}>{legend}</legend>
      {values.map((v) => (
        <label key={v} className="row">
          <input type="radio" name={field} value={v} checked={value === v} disabled={disabledValues.includes(v) && value !== v} onChange={() => onChange(field, v)} />
          {labelOf(labels, v)}
        </label>
      ))}
    </fieldset>
  );
}

// hostedMeta is what the run window says of a Report Designer run In
// Agentry the page found: its job and what it was given.
function hostedMeta(registry: string, h: app.DesignerHostedView) {
  return { registry, job: (h.op === "cover" ? "cover" : "design") as DesignerJob, images: 0, urls: h.urls?.length ?? 0, document: !!h.document };
}
