// Design with AI: the built-in Report Designer writes a whole report design
// from a brief (Design) or a custom cover page for one of the library's
// designs (Cover), In Agentry or on this computer, chosen in the dialog as
// Edit with AI chooses. In Agentry it runs at the registry's flat price and
// saves what it made to the library itself, a new design never the default.
// On this computer it runs against the person's own model for no credits,
// and only Add to my library puts what it made in the library, under a free
// name, never the default. Either way this dialog hands the run to its run
// window (DesignerFlow) once it has begun and closes. Use, on the library's
// card, is what makes a design the default.
//
// The images come only from the system's picker (Go marks what it returns,
// and a run takes nothing else). The brief, the counts and the addresses are
// checked here so the dialog can say so first, and again in Go before
// anything runs or is uploaded. A document to design from has its place,
// drawn only where the Report Designer reads one.
import { useEffect, useRef, useState } from "react";
import * as DesignerAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/designerapi";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import * as SettingsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/settingsapi";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { EventsOn } from "../host";
import { Button, Card, ErrorBox, Modal, Notice, Select, Textarea } from "../ui";
import { followDesignerHosted, startDesignerHere } from "../designerRun";
import {
  addressLines, briefBytes, designerPriceLine, designerProblem, documentLine, firstWhere, hostedOutcome, lookedLine, maxImages, savePrompt, takesDocument,
  type DesignerJob, type DesignerWhere,
} from "../designer";
import { isImageSrc, startFrom } from "../reportDesign";
import { ReportDesignPreview } from "./ReportDesignPreview";

export function DesignWithAI({ registry, lib, initialJob, initialFrom, hosted, onHosted, onClose }: {
  registry: string;
  lib: app.ReportDesignLibrary;
  initialJob: DesignerJob;
  initialFrom: string;
  /** The run In Agentry the page follows, null for none. */
  hosted: app.DesignerHostedView | null;
  onHosted: (v: app.DesignerHostedView | null) => void;
  onClose: () => void;
}) {
  const [job, setJob] = useState<DesignerJob>(initialJob);
  const [brief, setBrief] = useState("");
  const [from, setFrom] = useState(initialFrom);
  const [logo, setLogo] = useState("");
  const [images, setImages] = useState<string[]>([]);
  const [urlsText, setUrlsText] = useState("");
  const [document, setDocument] = useState("");
  const [where, setWhere] = useState<DesignerWhere | null>(null);
  const [avail, setAvail] = useState<app.DesignerAvailabilityView | null>(null);
  const [setup, setSetup] = useState<app.SetupStatusView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);

  useEffect(() => { RunAPI.SetupStatus().then(setSetup).catch(() => setSetup(null)); }, []);
  useEffect(() => {
    let live = true;
    DesignerAPI.Availability(registry, job)
      .then((a) => { if (!live) return; setAvail(a); setWhere((w) => w ?? firstWhere(a)); })
      .catch((e) => { if (live) setError(String(e)); });
    return () => { live = false; };
  }, [registry, job]);

  const urls = job === "design" ? addressLines(urlsText) : [];
  const addrCheck = useAddressCheck(job === "design" ? urlsText : "");

  const designs = lib.designs;
  const fromName = designs.find((d) => d.id === from)?.name ?? "";
  const hostedRunning = !!hosted && !hosted.final;
  const title = job === "cover" ? "Design a cover page with AI" : "Design with AI";

  const limits = avail ?? null;
  const w: DesignerWhere = where ?? "here";
  const inputs = { job, brief, from, logo: job === "design" ? logo : "", images, urls, document: job === "design" ? document : "" };
  const addrProblem = urls.length > 0 ? addressProblemOf(addrCheck) : null;
  const problem = limits ? designerProblem(inputs, w, limitsOf(limits), addrProblem) : "Reading where the Report Designer can run…";
  const dockerBlock = w === "here" && setup !== null && !setup.dockerOk;
  const imageMax = limits ? maxImages(job, w, limitsOf(limits)) : 4;
  const typed = brief.trim() !== "" || images.length > 0 || urls.length > 0 || logo !== "" || document !== "";

  async function start() {
    setError(null);
    const req = app.DesignerRequest.createFrom({ ...inputs, urls });
    const meta = { registry, job, images: images.length, urls: urls.length, document: inputs.document !== "" };
    setStarting(true);
    try {
      if (w === "agentry") {
        const r = await DesignerAPI.StartHosted(registry, req);
        if (r.refusal) setError(r.detail || r.refusal);
        else if (r.run) {
          // The run goes on in its run window (DesignerFlow); the page
          // follows it too, for its own lines.
          onHosted(r.run);
          followDesignerHosted(meta, r.run);
          onClose();
        }
        return;
      }
      // On this computer the dialog closes once the run has begun, and
      // what it made waits in its run window for Add to my library.
      await startDesignerHere(meta, req);
      onClose();
    } catch (e) {
      const s = String(e);
      if (!s.includes("not confirmed")) setError(s);
    } finally {
      setStarting(false);
    }
  }

  async function pickImage() {
    try {
      const p = await DesignerAPI.ChooseImage(job === "cover" ? "An image the cover may draw" : "A reference image");
      if (p && !images.includes(p)) setImages([...images, p]);
    } catch (e) {
      setError(String(e));
    }
  }

  return (
    <Modal title={title} wide onClose={onClose}>
      <p className="muted small">
        The Report Designer is a built-in agent that designs how your PDF reports look. It reads your brief and what you show it, writes the
        design, renders a sample page and its cover, and looks at them before it answers. What it makes is a new design in your library,
        never your default.
      </p>
      <div className="mt" data-designer-job>
        <h3 className="small">What it makes</h3>
        <label className="small" style={{ display: "block" }}>
          <input type="radio" name="designer-job" checked={job === "design"} onChange={() => setJob("design")} />{" "}
          <strong>A whole design</strong> <span className="muted">— colours, faces, title block, header, footer, brand block and cover style.</span>
        </label>
        <label className="small" style={{ display: "block" }}>
          <input type="radio" name="designer-job" checked={job === "cover"} disabled={designs.length === 0}
            onChange={() => { setJob("cover"); if (!from && designs.length > 0) setFrom(lib.default || designs[0].id); }} />{" "}
          <strong>A custom cover page</strong> <span className="muted">— for one of your designs, written in Typst.{designs.length === 0 ? " Add a design first." : ""}</span>
        </label>
      </div>

      <div className="field">
        <label className="field-label" htmlFor="designer-brief">Brief</label>
        <Textarea id="designer-brief" rows={5} value={brief} onChange={(e) => setBrief(e.target.value)}
          placeholder={job === "cover"
            ? "A calm harbour at dawn behind the title, our navy band at the foot, nothing busy."
            : "Quarterly reports for the Northwind Harbour Trust, a small charity: navy and sand, serious but warm, our charity number in the footer."} />
        <span className="muted small">{briefBytes(brief).toLocaleString("en-GB")} of {(limits?.maxBriefBytes ?? 4096).toLocaleString("en-GB")} bytes.</span>
      </div>

      <div className="field">
        <label className="field-label" htmlFor="designer-from">{job === "cover" ? "The design the cover is for" : "Start from (optional)"}</label>
        <Select id="designer-from" value={from} onChange={(e) => setFrom(e.target.value)}>
          {job === "design" && <option value="">Nothing: a design of its own</option>}
          {designs.map((d) => <option key={d.id} value={d.id}>{d.name}{d.default ? " (your default)" : ""}</option>)}
        </Select>
        {job === "cover" && fromName && (
          <span className="muted small">The result is a new design: {fromName} with this cover page. {fromName} itself stays as it is.</span>
        )}
      </div>

      {job === "design" && (
        <div className="field" data-designer-logo>
          <div className="field-label">Your logo (optional)</div>
          <div className="row gap">
            <Button onClick={() => DesignerAPI.ChooseImage("Your logo").then((p) => { if (p) setLogo(p); }).catch((e) => setError(String(e)))}>
              {logo ? "Choose another…" : "Choose a logo…"}
            </Button>
            {logo && <span className="small mono selectable">{logo} <Button variant="link" onClick={() => setLogo("")}>Remove</Button></span>}
          </div>
          <span className="muted small">A PNG or a JPEG. It takes the place of the logo of the design it starts from.</span>
        </div>
      )}

      <div className="field" data-designer-images>
        <div className="field-label">{job === "cover" ? "Images the cover may draw (optional)" : "Reference images (optional)"}</div>
        <div className="row gap">
          <Button disabled={images.length >= imageMax} onClick={() => void pickImage()}>Choose an image…</Button>
          <span className="muted small">{images.length} of {imageMax}</span>
        </div>
        <span className="muted small">
          {job === "cover"
            ? "PNG or JPEG pictures it may place on the cover, beside the design's own logo and cover image."
            : "PNG or JPEG pages or pictures whose look it should follow: a report you like, your letterhead. It looks at them and never keeps them."}
        </span>
        {images.length > 0 && (
          <ul className="small mono selectable">
            {images.map((p) => <li key={p}>{p} <Button variant="link" onClick={() => setImages(images.filter((x) => x !== p))}>Remove</Button></li>)}
          </ul>
        )}
      </div>

      {job === "design" && (
        <div className="field" data-designer-urls>
          <label className="field-label" htmlFor="designer-urls">Web addresses (optional)</label>
          <Textarea id="designer-urls" rows={2} value={urlsText} onChange={(e) => setUrlsText(e.target.value)} placeholder="https://www.example.org/" />
          <span className="muted small">
            One https address a line, up to {limits?.maxUrls ?? 2}: your own site or a page about your brand. Each page is fetched first and read by
            AI Review before the Report Designer sees it{w === "here" ? ", with your own TypeSafe key" : ", through the registry, inside the price"}.
          </span>
          {addrCheck?.check?.lines?.filter((l) => l.problem).map((l) => <span key={l.address} className="small step-problem" role="alert">{l.address}: {l.problem}</span>)}
        </div>
      )}

      {job === "design" && (takesDocument(job, w, limitsOf(limits)) || document !== "") && (
        <div className="field" data-designer-document>
          <div className="field-label">Start from a document (optional)</div>
          <div className="row gap">
            {takesDocument(job, w, limitsOf(limits)) && (
              <Button onClick={() => DesignerAPI.ChooseDocument("A document to design from").then((p) => { if (p) setDocument(p); }).catch((e) => setError(String(e)))}>
                {document ? "Choose another…" : "Choose a PDF or Word document…"}
              </Button>
            )}
            {document && <span className="small mono selectable">{document} <Button variant="link" onClick={() => setDocument("")}>Remove</Button></span>}
          </div>
          {!takesDocument(job, w, limitsOf(limits)) && (
            <span className="small step-problem" role="alert">The Report Designer In Agentry is not given a document from this app: run it on this computer, or remove the document.</span>
          )}
          <span className="muted small">
            One PDF or Word document, up to {Math.round((limits?.maxDocumentBytes ?? 0) / (1024 * 1024))} MB, that already looks the way you want. The
            Report Designer reads it in full and keeps it only for the run. The design can keep its logo and its cover page&apos;s artwork, never a
            whole page or its text.
          </span>
        </div>
      )}

      <h3 className="small">Where it runs</h3>
      {limits?.offered && (
        <Card className={w === "agentry" ? "selected" : ""}>
          <label className="small" style={{ display: "block" }}>
            <input type="radio" name="designer-where" checked={w === "agentry"} disabled={!limits.available} onChange={() => setWhere("agentry")} />{" "}
            <strong>In Agentry</strong>{" "}
            <span className="muted">— on Agentry&apos;s model, for credits. When it ends, the design is saved to your library.</span>
          </label>
          {limits.available && <p className="small" style={{ marginBottom: 0 }} data-designer-price>{designerPriceLine(limits)}</p>}
          {!limits.available && limits.reason && (
            <Notice tone="warn">{limits.reason}{limits.remedy ? ` ${limits.remedy}` : ""}</Notice>
          )}
        </Card>
      )}
      <Card className={w === "here" ? "selected mt" : "mt"}>
        <label className="small" style={{ display: "block" }}>
          <input type="radio" name="designer-where" checked={w === "here"} onChange={() => setWhere("here")} />{" "}
          <strong>On this computer</strong>{" "}
          <span className="muted">
            — on the model your runner policy names ({limits?.model ?? "…"}), for no credits. You see the design before anything is saved.
          </span>
        </label>
        {dockerBlock && (
          <Notice tone="warn">The Report Designer runs in containers, and the container engine is not ready on this computer{setup?.dockerErr ? `: ${setup.dockerErr}` : ""}. Settings → Runs sets it up.</Notice>
        )}
      </Card>

      <ErrorBox error={error} />
      <div className="row gap mt" style={{ justifyContent: "flex-end" }}>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="primary" disabled={problem !== null || dockerBlock || hostedRunning} busy={starting} busyLabel="Starting…" onClick={() => void start()}>
          {job === "cover" ? "Design the cover" : "Design it"}
        </Button>
      </div>
      {problem && typed && <p className="muted small" data-designer-problem>{problem}</p>}
      {hostedRunning && <p className="muted small">A Report Designer run In Agentry is already under way: wait for it to finish, or cancel it.</p>}
    </Modal>
  );
}

// ResultCard is a design as the library's card and the editor's preview draw
// it: its name, its custom cover's thumbnail when it has one, and the
// likeness of its pages.
export function ResultCard({ view, opts }: { view: app.ReportDesignView; opts: app.ReportDesignOptions }) {
  if (!view.design) return null;
  const d = startFrom(view.design);
  return (
    <div className="report-design-result" data-designer-result>
      <strong>{view.design.name}</strong>
      <div className="row gap" style={{ alignItems: "flex-start" }}>
        {view.customCover?.thumbnail && isImageSrc(view.customCover.thumbnail) && (
          <img className="report-design-custom-cover" src={view.customCover.thumbnail} alt={`The custom cover page of ${view.design.name}, as the Report Designer last rendered it`} />
        )}
        <div className="report-design-result-pages">
          <ReportDesignPreview design={d} check={null} families={opts.families} coverStyles={opts.coverStyles}
            logo={view.logo ?? ""} coverImage={view.coverImage ?? ""} customCover={view.customCover?.thumbnail ?? ""} />
        </div>
      </div>
    </div>
  );
}

/** The limits the rules read, from Go's answer, before it has answered. */
function limitsOf(a: app.DesignerAvailabilityView | null) {
  return {
    maxBriefBytes: a?.maxBriefBytes ?? 4096, maxReferences: a?.maxReferences ?? 4, maxCoverImages: a?.maxCoverImages ?? 6, maxUrls: a?.maxUrls ?? 2,
    reviewReady: a?.reviewReady ?? false, reviewWhy: a?.reviewWhy, offered: a?.offered ?? false, available: a?.available ?? false,
    hostedMaxImages: a?.hostedMaxImages ?? 0, hostedUrls: a?.hostedUrls ?? false, hostedMaxUrls: a?.hostedMaxUrls,
    documentHere: a?.documentHere ?? false, documentInAgentry: a?.documentInAgentry ?? false,
  };
}

/** Go's word on the addresses: the first refused, or not yet answered. */
function addressProblemOf(c: app.DesignerAddressesView | null): string | null {
  if (!c?.check) return "Checking the web addresses…";
  if (c.check.problem) return c.check.problem;
  const bad = c.check.lines?.find((l) => l.problem);
  return bad ? `${bad.address}: ${bad.problem}` : null;
}

// useAddressCheck is Go's check of the addresses, asked a moment after the
// person stops typing; null until it answers for what is typed now.
function useAddressCheck(text: string): app.DesignerAddressesView | null {
  const [c, setC] = useState<{ text: string; v: app.DesignerAddressesView } | null>(null);
  const seq = useRef(0);
  useEffect(() => {
    const lines = addressLines(text);
    if (lines.length === 0) return;
    const n = ++seq.current;
    const t = setTimeout(() => {
      DesignerAPI.CheckAddresses(lines).then((v) => { if (n === seq.current) setC({ text, v }); }).catch(() => {});
    }, 300);
    return () => clearTimeout(t);
  }, [text]);
  return c && c.text === text ? c.v : null;
}
