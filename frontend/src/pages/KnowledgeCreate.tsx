// Create knowledge and Add documents (plan knowledge.md §4.8; D19;
// agentry-desktop#273): a stepped flow on the step chrome the first-run
// wizard and Build share (primitives.tsx) — the documents, the name, then a
// review that says, before anything runs, that the Knowledge Creator runs
// on this computer against the runner policy's model and that nothing it
// reads or writes is scanned — then the build's progress and what it made.
//
// The documents come only from the native file and folder pickers; Go
// refuses any other path (D-030). A build on this computer takes the run
// lock, as a run does, so it is refused while a run or another build holds
// it.
//
// Where the registry builds knowledge (Knowledge M6, agentry-desktop#274),
// a Where step offers In Agentry beside this computer: the Knowledge
// Creator runs on the platform against its model, for credits, the pack is
// kept in Agentry and scanned there, then downloaded here. The step states
// the hold, the scan's credit inside it, the caps and the balance before
// anything is uploaded; the build is followed until it is over, and one
// that stopped short offers Continue. Closing the dialog while it runs keeps
// it in the corner card (below) until it is over; an app started while a
// build In Agentry runs follows it again when Create knowledge opens.
//
// A build on this computer is long work (agentry-desktop#352): the review
// step says so and how long it may run (at least 60 minutes, Go's
// CreatorBudgets), and the build shows its progress as it goes (elapsed
// against the timeout, turns, the pages written so far) over its log.
//
// A build on this computer takes up to 1 GB of documents, a build In
// Agentry up to 10 MB (agentry-desktop#354, agentry-cli#429): the Documents
// step shows what the documents come to, warns past 10 MB that the build
// can take a very long time and that its pack stays on this computer, and
// refuses past 1 GB; past 10 MB In Agentry is not choosable.
//
// A build needs no documents since agentry-desktop#498 (plan
// knowledge-and-drafter.md §7, D15, D16): the Documents step takes
// documents, a brief (what the pack should cover and for whom, at most
// 8 KiB), or both, with the plan's warning that specific documents make the
// best knowledge, louder when none is chosen. Its Search the web choice is
// on by default for a new pack with no documents and off otherwise, and
// follows the documents until the person sets it. On this computer it takes
// the person's own AI Review key and a search key (Go's CreatorWeb), and the
// step says when a build here would run without the web; In Agentry the
// registry searches, and its note says searching can raise the credits a
// build uses. The choice is sent as made either way.
//
// The Local and Cloud Knowledge pages each open this flow preset to their
// place (plan knowledge-cards.md §4.5, D5; agentry-desktop#619): `place`
// chooses the Where step's answer, which the person can still change. Where
// In Agentry cannot be chosen (Standard, DEV-162; signed out; a registry
// that builds nothing; documents past its cap), the answer falls back to
// this computer, the Where step says why, and a pack made here from the
// Cloud page is offered Copy to Cloud when it is done.
//
// A running build does not take over the app (agentry-desktop#353): App
// mounts this flow above the pages, and closing the dialog while a build
// runs, or Run in the background, shrinks it to a card in the bottom-right
// corner. The card keeps the build's progress, opens the full view again,
// stops the build (on this computer: KnowledgeAPI.StopBuild, after which
// the pages written that pass the apply step are kept sealed, the seal
// outliving the stop; In Agentry: the registry's cancel), and, once
// the build is over, says how it ended and offers Preview (and Continue, for
// a build In Agentry that stopped short). The run lock is unchanged: a run
// started meanwhile is refused with the reason, as it always was.
import { useEffect, useRef, useState } from "react";
import * as KnowledgeAPI from "../../wailsjs/go/main/KnowledgeAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { ChooseDirectory, ChooseFiles, OpenExternal } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { knowledge, main } from "../../wailsjs/go/models";
import { Button, ErrorBox, Field, Input, Modal, Notice, Recap, Spinner, StepFoot, StepHeading, StepList, Textarea, fmtBytes } from "../ui";
import type { Tone } from "../jobProgress";
import { buildHoldLine, creatorUseLine } from "../usage";
import { CornerCard, FoldedLog, LogPane, ProgressView, RunningView, StopButton } from "../progress";
import { BuildProgress, fmtCap, largeDocumentsWarning, buildCreditsLine, buildTimeWarning, inAgentryNote, nameProblem, pagesOf, unscannedWarning } from "../knowledge";
import {
  MAX_BRIEF_BYTES, agentryWebLine, documentsRecap, localWebLine, readsLine, searches, sourcesProblem, sourcesWarning, webDefault, webRecap, type LocalWeb,
} from "../knowledgeSources";

type Step = "documents" | "name" | "where" | "review";

const LABEL: Record<Step, string> = {
  documents: "Documents",
  name: "Name",
  where: "Where",
  review: "Review",
};

export default function KnowledgeCreate({
  registry, addTo, place, onClose, onPreview, minimized, onMinimize, onRestore, onInFlight, initialStep, initialDocuments, initialName, initialResult, initialBuilding, initialAvailability, initialHosted, initialWhere,
  initialBrief, initialWeb,
}: {
  registry: string;
  /** Add documents to this kept pack; unset makes a new one. */
  addTo?: string;
  /** Which page opened the flow: its place presets the Where step. */
  place?: "local" | "agentry";
  onClose: () => void;
  onPreview: (name: string) => void;
  /** Shown as the corner card rather than the dialog; App owns it. */
  minimized?: boolean;
  /** Shrink to the corner card, and back. */
  onMinimize?: () => void;
  onRestore?: () => void;
  /** Told whenever the build starts or ends, so App can refresh the list
   *  when it ends and replace a flow that is over. */
  onInFlight?: (inFlight: boolean) => void;
  /** Harness-only: open on this step with these answers. */
  initialStep?: Step;
  initialDocuments?: string[];
  initialName?: string;
  /** Harness-only: open on a finished build, or one in progress. */
  initialResult?: knowledge.BuildResult;
  initialBuilding?: boolean;
  /** Harness-only: the In Agentry answer, and a build In Agentry to show. */
  initialAvailability?: main.KnowledgeHostedAvailabilityView;
  initialHosted?: main.KnowledgeHostedBuildView;
  initialWhere?: "local" | "agentry";
  /** Harness-only: the brief, and this computer's answer on the web. */
  initialBrief?: string;
  initialWeb?: LocalWeb;
}) {
  const [avail, setAvail] = useState<main.KnowledgeHostedAvailabilityView | null>(initialAvailability ?? null);
  const [where, setWhere] = useState<"local" | "agentry">(initialWhere ?? place ?? "local");
  // availRead: the registry's answer on In Agentry has come, or failed.
  const [availRead, setAvailRead] = useState(!!initialAvailability);
  const [hosted, setHosted] = useState<main.KnowledgeHostedBuildView | null>(initialHosted ?? null);
  const [refusal, setRefusal] = useState<string | null>(null);
  // starting: a build In Agentry's documents are uploading and the build
  // is being created; the button waits rather than starting a second.
  const [starting, setStarting] = useState(false);
  const steps: Step[] = [...(addTo ? ["documents"] : ["documents", "name"]), ...(avail?.offered ? ["where"] : []), "review"] as Step[];
  const [at, setAt] = useState(Math.max(0, steps.indexOf(initialStep ?? "documents")));
  const [docs, setDocs] = useState<string[]>(initialDocuments ?? []);
  const [name, setName] = useState(initialName ?? addTo ?? "");
  const [brief, setBrief] = useState(initialBrief ?? "");
  // webChoice is Search the web as the person set it; null follows the
  // default, which follows the documents.
  const [webChoice, setWebChoice] = useState<boolean | null>(null);
  const [localWeb, setLocalWeb] = useState<LocalWeb | null>(initialWeb ?? null);
  const [problem, setProblem] = useState<string | null>(null);
  const [model, setModel] = useState("");
  const [setup, setSetup] = useState<main.SetupStatusView | null>(null);
  const [building, setBuilding] = useState(initialBuilding ?? false);
  const [log, setLog] = useState<string[]>([]);
  const [budget, setBudget] = useState<main.CreatorBudgetView | null>(null);
  const [size, setSize] = useState<main.DocumentsView | null>(null);
  const [progress, setProgress] = useState<BuildProgress | null>(null);
  const [result, setResult] = useState<knowledge.BuildResult | null>(initialResult ?? null);
  const [error, setError] = useState<string | null>(null);
  const heading = useRef<HTMLHeadingElement | null>(null);
  // Focus follows the step once the person has moved, not on opening.
  const moved = useRef(false);
  const step = steps[at];
  // The step the person is on, by name: the Where step appears once the
  // registry answers, and the one they are on must not change under them.
  const stepName = useRef<Step>(step);
  useEffect(() => {
    const i = steps.indexOf(stepName.current);
    if (i >= 0 && i !== at) setAt(i);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [avail?.offered]);
  useEffect(() => {
    stepName.current = step;
  }, [step]);

  useEffect(() => {
    KnowledgeAPI.CreatorModel().then(setModel).catch(() => setModel("none configured"));
    KnowledgeAPI.CreatorBudget().then(setBudget).catch(() => setBudget(null));
    RunAPI.SetupStatus().then(setSetup).catch(() => setSetup(null));
    if (!initialWeb) KnowledgeAPI.CreatorWeb().then(setLocalWeb).catch(() => setLocalWeb(null));
    if (!initialAvailability)
      KnowledgeAPI.HostedAvailability(registry)
        .then(setAvail)
        .catch(() => setAvail(null))
        .finally(() => setAvailRead(true));
    // A build In Agentry already in flight is followed rather than started
    // again: the registry takes one at a time.
    if (!initialHosted && !initialResult && !initialBuilding) {
      KnowledgeAPI.AttachHostedBuild(registry).then((v) => v.build && setHosted(v.build)).catch(() => {});
    }
  }, []);
  useEffect(() => EventsOn("knowledge:hosted-build", (v: main.KnowledgeHostedBuildView) => setHosted(v)), []);
  // sizing: the documents are being sized, so Next waits for the answer;
  // an answer for documents since changed is dropped.
  const [sizing, setSizing] = useState(false);
  useEffect(() => {
    setSize(null);
    if (docs.length === 0) return;
    let live = true;
    setSizing(true);
    KnowledgeAPI.DocumentsSize(docs)
      .then((v) => live && setSize(v))
      .catch(() => live && setSize(null))
      .finally(() => live && setSizing(false));
    return () => {
      live = false;
    };
  }, [docs]);
  const pastAgentry = !!size && size.bytes > size.agentryMaxBytes;
  const pastLocal = !!size && size.bytes > size.localMaxBytes;
  const hasBrief = brief.trim() !== "";
  const webOn = webChoice ?? webDefault(!!addTo, docs.length);
  const agentryWeb = !!avail?.web;
  // A registry from before briefs builds In Agentry from documents only,
  // and takes no brief beside them either.
  const agentryNoBrief = !!avail && !avail.takesBrief && (docs.length === 0 || hasBrief);
  // The brief's bound: 8 KiB, or less where the registry says so.
  const maxBrief = Math.min(MAX_BRIEF_BYTES, avail?.maxBriefBytes || MAX_BRIEF_BYTES);
  // The warning follows whether the build will search, where it runs: a
  // ticked box on a computer without the keys does not search.
  const warning = sourcesWarning(docs.length, searches(webOn, where, localWeb, agentryWeb));
  // In Agentry preset where it cannot be chosen falls back to this
  // computer, as its radio is disabled then (D5).
  const agentryChoosable = !!avail?.offered && avail.available && !pastAgentry && !agentryNoBrief;
  // Add documents from the Cloud page does not fall back: the pack may not
  // be kept here, and documents added here would change only this
  // computer's copy, so In Agentry stays chosen and the reason blocks it.
  useEffect(() => {
    if (!addTo && where === "agentry" && availRead && !agentryChoosable) setWhere("local");
  }, [addTo, where, availRead, agentryChoosable]);
  const fellBack = place === "agentry" && where === "local" && availRead && !agentryChoosable;
  useEffect(() => EventsOn("knowledge:build-log", (line: string) => setLog((p) => [...p, line])), []);
  useEffect(() => EventsOn("knowledge:build-progress", (p: BuildProgress) => setProgress(p)), []);
  useEffect(() => {
    if (moved.current) heading.current?.focus();
  }, [at]);

  const goTo = (i: number) => {
    moved.current = true;
    setProblem(null);
    setAt(Math.max(0, Math.min(steps.length - 1, i)));
  };
  function next() {
    const p =
      step === "documents" ? sourcesProblem(docs.length, brief, maxBrief) ?? documentsProblem()
      : step === "name" ? nameProblem(name)
      : null;
    if (p) {
      setProblem(p);
      heading.current?.focus();
      return;
    }
    goTo(at + 1);
  }
  function documentsProblem(): string | null {
    return docs.length > 0 && sizing ? "Still adding up the documents' size; try again in a moment."
      : pastLocal ? `The documents come to ${fmtBytes(size!.bytes)}; a knowledge pack made on this computer, its documents included, is at most ${fmtCap(size!.localMaxBytes)}. Remove some.`
      : null;
  }

  async function addFiles() {
    const picked = await ChooseFiles("Documents for the knowledge pack");
    if (picked?.length) setDocs((d) => [...d, ...picked.filter((p) => !d.includes(p))]);
  }
  async function addFolder() {
    const dir = await ChooseDirectory("A folder of documents for the knowledge pack");
    if (dir) setDocs((d) => (d.includes(dir) ? d : [...d, dir]));
  }

  // What stops the build on this computer, as sentences.
  const blockers: string[] = [];
  const inAgentry = where === "agentry";
  if (inAgentry) {
    if (availRead && !avail?.offered) blockers.push("This registry does not build knowledge In Agentry.");
    else if (avail && !avail.available) blockers.push(avail.reason + (avail.remedy ? ` ${avail.remedy}` : ""));
    if (pastAgentry) blockers.push(`A build In Agentry takes at most ${fmtCap(size!.agentryMaxBytes)} of documents; these come to ${fmtBytes(size!.bytes)}. Build it on this computer.`);
    if (agentryNoBrief) blockers.push(AGENTRY_NO_BRIEF);
  } else if (setup && !setup.dockerOk) blockers.push(`Docker is not ready: ${setup.dockerErr || "it did not answer"}. Settings → Run defaults says how to set it up.`);
  if (!inAgentry && model === "none configured") blockers.push("No model is set up for runs on this computer: choose one in Settings → Provider settings.");

  async function startHosted() {
    setError(null);
    setRefusal(null);
    setStarting(true);
    try {
      const v = await KnowledgeAPI.StartHostedBuild(registry, addTo ? "" : name, addTo ?? "", docs, brief, searches(webOn, "agentry", localWeb, agentryWeb));
      if (v.build) setHosted(v.build);
      else setRefusal(v.detail || v.refusal || "The registry refused the build.");
    } catch (e) {
      setError(String(e));
    } finally {
      setStarting(false);
    }
  }
  async function continueHosted(id: string) {
    setRefusal(null);
    try {
      const v = await KnowledgeAPI.ContinueHostedBuild(registry, id);
      if (v.build) setHosted(v.build);
      else setRefusal(v.detail || v.refusal || "The registry refused to continue the build.");
    } catch (e) {
      setError(String(e));
    }
  }

  const [stopping, setStopping] = useState(false);
  // In flight: the dialog's close shrinks it to the corner card instead.
  const inFlight = building || (!!hosted && !hosted.final);
  useEffect(() => {
    onInFlight?.(inFlight);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [inFlight]);
  async function stopLocal() {
    setStopping(true);
    try {
      await KnowledgeAPI.StopBuild();
    } catch {
      // The build ended as Stop was pressed: its own result or error is
      // what the view shows, not this.
      setStopping(false);
    }
  }
  const cancelHosted = () => KnowledgeAPI.CancelHostedBuild().then(setHosted).catch((e) => setError(String(e)));

  async function build() {
    if (inAgentry) return startHosted();
    setBuilding(true);
    setStopping(false);
    setError(null);
    setLog([]);
    setProgress(null);
    try {
      setResult(addTo ? await KnowledgeAPI.Add(registry, addTo, docs, brief, webOn) : await KnowledgeAPI.Create(registry, name, docs, brief, webOn));
    } catch (e) {
      setError(String(e));
    } finally {
      setBuilding(false);
    }
  }

  const title = addTo ? `Add documents to ${addTo}` : "Create knowledge";
  const running = building || result || error || hosted || refusal;
  const minimize = onMinimize ?? (() => {});

  if (minimized) {
    const packName = hosted?.name || addTo || name;
    let line: string;
    let tone: Tone = "";
    if (building) line = "";
    else if (hosted && !hosted.final) line = hosted.statusLine || hosted.state;
    else if (hosted) {
      line = hosted.applied ? `${hosted.name}: ${pagesOf(hosted.pages)}` : hosted.outcomeLine || hosted.state;
      tone = hosted.outcome === "succeeded" ? "ok" : hosted.canContinue ? "warn" : "bad";
    } else if (result) {
      line = `${addTo ? "Added to" : "Made"} ${result.name}: ${pagesOf(result.pages)}${result.stopped ? ", stopped before it finished" : ""}`;
      tone = result.stopped ? "warn" : "ok";
    } else if (error) {
      line = addTo ? `Nothing was added to ${addTo}` : "No pack was made";
      tone = "bad";
    } else if (refusal) {
      line = "The registry did not start the build";
      tone = "bad";
    } else line = "";
    return (
      <CornerCard
        label={`Knowledge build: ${packName}`}
        running={inFlight}
        title={<>
          {inFlight ? (addTo ? `Adding documents to ${addTo}` : `Creating ${packName}`) : `${packName}: the build is over`}
          {inFlight && hosted ? " In Agentry" : ""}
        </>}
        line={line}
        tone={tone}
        actions={<>
          {building && <StopButton stopping={stopping} onStop={stopLocal} />}
          {hosted && !hosted.final && !hosted.cancelRequested && hosted.status !== "finished" && hosted.status !== "cancelled" && (
            <Button onClick={cancelHosted}>Cancel the build</Button>
          )}
          {!inFlight && result && <Button onClick={() => onPreview(result.name)}>Preview</Button>}
          {hosted?.final && hosted.downloaded && <Button onClick={() => onPreview(hosted.name)}>Preview</Button>}
          {hosted?.final && hosted.canContinue && <Button onClick={() => continueHosted(hosted.id)}>Continue</Button>}
          {!inFlight && <Button onClick={onClose}>Dismiss</Button>}
          <Button variant="primary" onClick={onRestore}>Open</Button>
        </>}
      >
        {building && <BuildProgressView progress={progress} compact />}
      </CornerCard>
    );
  }

  return (
    <Modal title={title} onClose={inFlight ? minimize : onClose} wide>
      {!running && (
        <>
          <StepList labels={steps.map((s) => LABEL[s])} at={at} />
          <section aria-labelledby="knowledge-step-heading">
            <StepHeading ref={heading} id="knowledge-step-heading" at={at} total={steps.length} label={LABEL[step]} />

            {step === "documents" && (
              <>
                <p className="small">Choose documents, write a brief, or both.</p>
                <p className="muted small">
                  Markdown, text, HTML, CSV, JSON, YAML and source files are read as they are; PDF, DOCX, ODT, EPUB, RTF, PPTX
                  and XLSX, and PNG, JPEG, TIFF or WebP images, as the text decoded from them on this computer first. The
                  documents are kept in the pack beside its pages. A pack made on this computer holds up to 1 GB, documents included; one built In
                  Agentry, 10 MB.
                  {addTo && <> The Creator reads {addTo} as it stands and rewrites the pages the new documents or the brief touch.</>}
                </p>
                {docs.length > 0 ? (
                  <div>
                    {docs.map((p) => (
                      <div key={p} className="row" style={{ marginBottom: 4 }}>
                        <span className="mono small grow selectable">{p}</span>
                        <Button onClick={() => setDocs(docs.filter((x) => x !== p))} aria-label={`Remove ${p}`}>Remove</Button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="muted small">No documents chosen.</p>
                )}
                {size && docs.length > 0 && (
                  <p className="small">
                    {docs.length === 1 ? "1 file or folder" : `${docs.length} files and folders`}, {fmtBytes(size.bytes)} together.
                  </p>
                )}
                {pastAgentry && !pastLocal && <Notice tone="warn">{largeDocumentsWarning(size!.bytes, size!.agentryMaxBytes)}</Notice>}
                <div className="row" style={{ marginBottom: 16 }}>
                  <Button onClick={addFiles}>Add files…</Button>
                  <Button onClick={addFolder}>Add folder…</Button>
                </div>
                <Field
                  label={<>A brief <span className="muted">(optional with documents)</span></>}
                  htmlFor="knowledge-brief"
                  hint={<>What the pack should cover, and for whom. It is kept in the pack as <span className="mono">sources/brief.md</span>, up to {maxBrief >> 10} KiB.</>}
                >
                  <Textarea
                    id="knowledge-brief"
                    rows={4}
                    value={brief}
                    placeholder={addTo ? "What the pack should add, and for whom." : "What the pack should cover, and for whom."}
                    onChange={(e) => setBrief(e.target.value)}
                  />
                </Field>
                <Notice tone={warning.tone}>{warning.text}</Notice>
                <label className="check-row">
                  <input type="checkbox" checked={webOn} onChange={(e) => setWebChoice(e.target.checked)} aria-describedby="knowledge-web-note" />
                  <span>
                    <strong>Search the web</strong>{" "}
                    <span className="muted">— {addTo ? "off unless you tick it" : "on by default only without documents"}</span>
                  </span>
                </label>
                <div id="knowledge-web-note" className="muted small">
                  <p>
                    The Creator is told to search only for what your documents and the pack do not hold, and the searches are
                    drawn from them and the brief. Searching can raise what a build costs.
                  </p>
                  {webOn && <p>{localWebLine(localWeb)}</p>}
                  {webOn && avail?.offered && <p>{agentryWebLine(agentryWeb, avail.webNote)}</p>}
                </div>
              </>
            )}

            {step === "name" && (
              <Field label="Name" htmlFor="knowledge-name" hint={<>A run finds the pack at <span className="mono">/knowledge/{name || "name"}</span>. Lowercase letters, digits, &apos;.&apos;, &apos;-&apos; and &apos;_&apos;.</>}>
                <Input id="knowledge-name" value={name} autoFocus onChange={(e) => setName(e.target.value.trim())} onKeyDown={(e) => e.key === "Enter" && next()} />
              </Field>
            )}

            {step === "where" && avail && (
              <>
                <div role="radiogroup" aria-label="Where to build it">
                  <label className="check-row">
                    <input type="radio" name="knowledge-where" checked={!inAgentry} onChange={() => setWhere("local")} />
                    <span>
                      <strong>On this computer</strong>{" "}
                      <span className="muted">— against your runner policy&apos;s model ({model || "…"}); no credits, and the pack is not scanned</span>
                    </span>
                  </label>
                  <label className="check-row">
                    <input type="radio" name="knowledge-where" checked={inAgentry} disabled={!avail.available || pastAgentry || agentryNoBrief} onChange={() => setWhere("agentry")} />
                    <span>
                      <strong>In Agentry</strong>{" "}
                      <span className="muted">— on the platform, for credits; the pack is kept in Agentry and scanned, then copied here</span>
                    </span>
                  </label>
                </div>
                {avail.available && pastAgentry && (
                  <p className="muted small">
                    In Agentry takes at most {fmtCap(size!.agentryMaxBytes)} of documents; these come to {fmtBytes(size!.bytes)}, so this pack is built on this computer.
                  </p>
                )}
                {avail.available && agentryNoBrief && <p className="muted small">{AGENTRY_NO_BRIEF}</p>}
                {!avail.available && avail.reason && (
                  <p className="muted small">
                    In Agentry is not available: {avail.reason}
                    {avail.remedy ? ` ${avail.remedy}` : ""}
                    {/* Creating knowledge in Agentry is Pro and above (DEV-162,
                        agentry-desktop#328): the registry's subscription page, in
                        the system browser, never inside the app. */}
                    {avail.upgradeUrl && (
                      <>
                        {" "}
                        <Button variant="link" onClick={() => void OpenExternal(avail.upgradeUrl!)}>
                          See Pro
                        </Button>
                      </>
                    )}
                  </p>
                )}
                {inAgentry && <HostedCost avail={avail} />}
              </>
            )}

            {step === "review" && (
              <>
                <Recap
                  rows={[
                    { label: "Documents", value: documentsRecap(docs), onEdit: () => goTo(steps.indexOf("documents")) },
                    ...(hasBrief ? [{ label: "Brief", value: clip(brief.trim(), 280), onEdit: () => goTo(steps.indexOf("documents")) }] : []),
                    { label: "Search the web", value: webRecap(webOn, where, localWeb, agentryWeb), onEdit: () => goTo(steps.indexOf("documents")) },
                    ...(addTo ? [] : [{ label: "Name", value: name, onEdit: () => goTo(steps.indexOf("name")) }]),
                    {
                      label: "Where",
                      value: inAgentry ? "In Agentry, against the platform's model, for credits" : "This computer, against your runner policy's model",
                      onEdit: steps.includes("where") ? () => goTo(steps.indexOf("where")) : undefined,
                    },
                  ]}
                />
                {docs.length === 0 && <Notice tone="warn">{warning.text}</Notice>}
                {fellBack && !avail?.offered && (
                  <Notice tone="info">
                    This registry does not build knowledge In Agentry, so the pack is made on this computer. Copy to Cloud keeps a copy in
                    your account once it is made.
                  </Notice>
                )}
                {inAgentry && avail ? (
                  <>
                    <Notice tone="info">{inAgentryNote(registry, docs.length, hasBrief)}</Notice>
                    <HostedCost avail={avail} />
                  </>
                ) : (
                  <>
                    <Notice tone="warn">{unscannedWarning(model || "…")}</Notice>
                    {budget && <Notice tone="info">{buildTimeWarning(budget.timeoutSeconds, budget.maxTurns)}</Notice>}
                  </>
                )}
                {blockers.map((b) => (
                  <p key={b} className="step-problem" role="alert">{b}</p>
                ))}
              </>
            )}
          </section>
          <StepFoot problem={problem} onBack={at > 0 ? () => goTo(at - 1) : undefined}>
            {step === "review" ? (
              <Button variant="primary" disabled={starting || blockers.length > 0 || (!inAgentry && !setup)} onClick={build}>
                {starting ? "Uploading the documents…" : <>{addTo ? "Add documents" : "Create knowledge"}{inAgentry ? " In Agentry" : ""}</>}
              </Button>
            ) : (
              <Button variant="primary" onClick={next}>Next: {LABEL[steps[at + 1]]}</Button>
            )}
          </StepFoot>
        </>
      )}

      {building && (
        <RunningView
          spinner={addTo ? `Adding documents to ${addTo}…` : `Building ${name}…`}
          about={<>
            The Knowledge Creator is reading {readsLine(docs.length, hasBrief)} and writing the pages. This can take a long time; the pages it has
            written count up as it goes, and are checked when it finishes, which may drop some.
          </>}
          actions={<>
            <StopButton stopping={stopping} onStop={stopLocal} label="Stop the build" />
            {onMinimize && <Button variant="primary" onClick={onMinimize}>Run in the background</Button>}
          </>}
          background={<>
            In the background, the build goes on in a card in the corner and the rest of the app stays usable. Stop keeps
            the pages written so far that pass their checks.
          </>}
        >
          <BuildProgressView progress={progress} />
          <FoldedLog label="Build log" lines={log} />
        </RunningView>
      )}

      {!building && error && (
        <>
          <h3 className="run-outcome run-outcome-bad">{addTo ? `Nothing was added to ${addTo}` : "No pack was made"}</h3>
          <ErrorBox error={error} />
          {log.length > 0 && <LogPane label="Build log" lines={log} />}
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => { setError(null); setLog([]); }}>Back</Button>
            <Button onClick={onClose}>Close</Button>
          </div>
        </>
      )}

      {!building && result && (
        <BuildDone
          result={result}
          added={!!addTo}
          onClose={onClose}
          onPreview={() => onPreview(result.name)}
          copyToCloud={place === "agentry" && !addTo ? () => KnowledgeAPI.Upload(registry, result.name) : undefined}
        />
      )}

      {refusal && !hosted && (
        <>
          <h3 className="run-outcome run-outcome-bad">The registry did not start the build</h3>
          <ErrorBox error={refusal} />
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setRefusal(null)}>Back</Button>
            <Button onClick={onClose}>Close</Button>
          </div>
        </>
      )}

      {hosted && !hosted.final && (
        <>
          <Spinner label={hosted.statusLine || hosted.state} />
          <p className="muted small">
            The Knowledge Creator runs In Agentry on {readsLine(hosted.documents?.length ?? 0, !!hosted.brief)}. Run in the background keeps it in a card
            in the corner while you use the rest of the app; the build goes on in Agentry either way.
          </p>
          <p className="small">{buildCreditsLine(hosted)}</p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            {!hosted.cancelRequested && hosted.status !== "finished" && hosted.status !== "cancelled" && (
              <Button onClick={cancelHosted}>Cancel the build</Button>
            )}
            {(hosted.status === "finished" || hosted.status === "cancelled") && (
              <Button onClick={cancelHosted}>Stop waiting</Button>
            )}
            {onMinimize ? <Button variant="primary" onClick={onMinimize}>Run in the background</Button> : <Button onClick={onClose}>Close</Button>}
          </div>
        </>
      )}

      {hosted && hosted.final && (
        <HostedDone
          v={hosted}
          onContinue={() => continueHosted(hosted.id)}
          onClose={onClose}
          onPreview={() => onPreview(hosted.name)}
        />
      )}
    </Modal>
  );
}

// clip shortens a long text for a recap row.
function clip(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max).trimEnd()}…` : text;
}

// AGENTRY_NO_BRIEF is why In Agentry cannot take the brief: the registry
// predates briefs (agentry-enterprise#296), and Go refuses one to it too.
const AGENTRY_NO_BRIEF = "This registry builds In Agentry from documents only, without a brief: leave the brief out, or build it on this computer.";

// BuildProgressView is how far a build on this computer has got: the shared
// view, with the pages the Creator has written so far as its own fact. The
// pages are counted as written: the apply step checks them after the run.
function BuildProgressView({ progress, compact }: { progress: BuildProgress | null; compact?: boolean }) {
  return (
    <ProgressView
      progress={progress}
      job="build"
      compact={compact}
      fact={(p) => `${pagesOf(p.pages)} written`}
      facts={(p) => (
        <>
          <dt>Pages written</dt>
          <dd>{p.pages}</dd>
        </>
      )}
    />
  );
}

// HostedCost is what a build In Agentry holds and may take, before it
// starts: the registry's numbers, and the balance it is paid from.
function HostedCost({ avail }: { avail: main.KnowledgeHostedAvailabilityView }) {
  const short = avail.balanceKnown && !avail.unmetered && avail.holdCredits > 0 && avail.balance < avail.holdCredits;
  return (
    <div className="small">
      <p>
        {buildHoldLine(avail.holdCredits, avail.scanCredits)}
        {avail.balanceKnown && !avail.unmetered && avail.holdCredits > 0
          ? avail.pool
            ? ` ${avail.pool}'s pool has ${avail.balance} credits.`
            : ` This account has ${avail.balance} credits.`
          : ""}
      </p>
      {avail.timeoutSeconds > 0 && (
        <p className="muted">
          A build may run for up to {Math.round(avail.timeoutSeconds / 60)} minutes and {avail.maxTurns} turns. At most {avail.maxInputFiles} documents,{" "}
          {fmtBytes(avail.maxInputBytes)} together.
        </p>
      )}
      {short && (
        <Notice tone="warn">
          {avail.pool ? `${avail.pool}'s pool has` : "This account has"} fewer credits than a build holds, so the registry will refuse this one.
        </Notice>
      )}
    </div>
  );
}

// HostedDone is a build In Agentry's last word: how it ended, what it did
// to the pack, what it cost, and — once the pack is downloaded — its scan.
function HostedDone({ v, onContinue, onClose, onPreview }: {
  v: main.KnowledgeHostedBuildView;
  onContinue: () => void;
  onClose: () => void;
  onPreview: () => void;
}) {
  const tone = v.outcome === "succeeded" ? "ok" : v.canContinue ? "warn" : "bad";
  return (
    <div>
      <h3 className={`run-outcome run-outcome-${tone}`}>
        {v.applied ? `${v.name}: ${pagesOf(v.pages)}, ${fmtBytes(v.bytes)} with its documents` : v.outcomeLine || v.state}
      </h3>
      {v.applied && v.outcomeLine && <p className="muted small">{v.outcomeLine}</p>}
      {v.note && <p>{v.note}</p>}
      {v.error && <ErrorBox error={v.error} />}
      {v.webLine && <p className="small">{v.webLine}</p>}
      <p className="small">{buildCreditsLine(v)}</p>
      {v.downloaded && (
        <p>
          {v.kept} {v.scan && <span className="muted">Agentry&apos;s scan: {v.scan}.</span>}
        </p>
      )}
      {v.downloadError && <Notice tone="warn">{v.downloadError}</Notice>}
      {v.canContinue && (
        <Notice tone="info">
          This build stopped before it finished. Continue starts a new build from the pack as it is now, with {v.brief ? "this brief and " : ""}these
          documents{v.webLine ? ", searching the web as this one could" : ""}.
        </Notice>
      )}
      <div className="row" style={{ justifyContent: "flex-end" }}>
        {v.canContinue && <Button onClick={onContinue}>Continue</Button>}
        {v.downloaded && <Button onClick={onPreview}>Preview</Button>}
        <Button variant="primary" onClick={onClose}>Done</Button>
      </div>
    </div>
  );
}

function BuildDone({ result, added, onClose, onPreview, copyToCloud }: {
  result: knowledge.BuildResult;
  added: boolean;
  onClose: () => void;
  onPreview: () => void;
  /** Offered when the Cloud page opened the flow and the pack was made
   *  here (plan knowledge-cards.md D5): keeps a copy in the account. */
  copyToCloud?: () => Promise<string>;
}) {
  const docs = result.documents ?? [];
  const [copying, setCopying] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const [copyError, setCopyError] = useState<string | null>(null);
  async function copy() {
    setCopying(true);
    setCopyError(null);
    try {
      setCopied(await copyToCloud!());
    } catch (e) {
      setCopyError(String(e));
    } finally {
      setCopying(false);
    }
  }
  return (
    <div>
      <h3 className={`run-outcome run-outcome-${result.stopped ? "warn" : "ok"}`}>
        {added ? `Added ${docs.length === 1 ? "1 document" : `${docs.length} documents`} to ${result.name}` : `Made ${result.name}`}: {pagesOf(result.pages)}, {fmtBytes(result.bytes)}
      </h3>
      <p>Kept on this computer, encrypted to {result.key}. It is not scanned.</p>
      {result.searched && <p className="muted small">The Creator could search the web, AI Review reading every result first with your AI Review key.</p>}
      {result.withoutWeb && <Notice tone="info">You asked for the web, and this build ran without it: {result.withoutWeb}.</Notice>}
      {result.stopped && (
        <Notice tone="warn">
          The Creator run stopped before it finished ({result.stopped}). The pack holds what it wrote; Add documents with the
          same documents continues it.
        </Notice>
      )}
      {(result.rewrapped ?? []).length > 0 && (
        <p className="muted small">Moved off this computer&apos;s key, to one this registry keeps for your account, first: {result.rewrapped!.join(", ")}.</p>
      )}
      {result.agentryStale && <p className="muted small">Agentry still keeps the copy from before this build; Copy to Cloud sends this one.</p>}
      <dl className="knowledge-facts">
        <dt>Documents read</dt>
        <dd><span className="mono small">{docs.join(", ") || "none"}</span></dd>
        {(result.skipped ?? []).length > 0 && (
          <>
            <dt>Not read</dt>
            <dd><span className="mono small">{result.skipped!.join(", ")}</span> (no decoder reads them, or the decoder failed)</dd>
          </>
        )}
        {(result.dropped ?? []).map((d) => (
          <FragmentRow key={d.path} label={d.kept ? "Kept with a note" : "Page dropped"} value={<><span className="mono small">{d.path}</span>: {d.reason}</>} />
        ))}
        {creatorUseLine(result) && (
          <>
            <dt>The Creator run used</dt>
            <dd>{creatorUseLine(result)}</dd>
          </>
        )}
      </dl>
      {copied && <Notice tone="ok">{copied}</Notice>}
      <ErrorBox error={copyError} />
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <Button onClick={onPreview}>Preview</Button>
        {copyToCloud && !copied && (
          <Button busy={copying} busyLabel="Copying…" onClick={copy}>Copy to Cloud</Button>
        )}
        <Button variant="primary" onClick={onClose}>Done</Button>
      </div>
    </div>
  );
}

function FragmentRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </>
  );
}
