// Create knowledge and Add documents (plan knowledge.md §4.8; D19;
// agentry-desktop#273): a stepped flow on the step chrome the first-run
// wizard and Build share (primitives.tsx) — the documents, the name, then a
// review that says, before anything runs, that the Knowledge Creator runs
// on this computer against the runner policy's model and that nothing it
// reads or writes is scanned — then the build's progress and what it made.
//
// The documents come only from the native file and folder pickers; Go
// refuses any other path (D-030). A build takes the run lock, as a run
// does, so it is refused while a run or another build holds it. Making a
// pack In Agentry, for credits and with a scan, is Knowledge M6.
import { useEffect, useRef, useState } from "react";
import * as KnowledgeAPI from "../../wailsjs/go/main/KnowledgeAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { ChooseDirectory, ChooseFiles } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { knowledge, main } from "../../wailsjs/go/models";
import { Button, ErrorBox, Field, Input, Modal, Notice, Recap, Spinner, StepFoot, StepHeading, StepList } from "../ui";
import { fmtBytes, nameProblem, pagesOf, unscannedWarning } from "../knowledge";

type Step = "documents" | "name" | "review";

const LABEL: Record<Step, string> = {
  documents: "Documents",
  name: "Name",
  review: "Review",
};

export default function KnowledgeCreate({
  registry, addTo, onClose, onPreview, initialStep, initialDocuments, initialName, initialResult, initialBuilding,
}: {
  registry: string;
  /** Add documents to this kept pack; unset makes a new one. */
  addTo?: string;
  onClose: () => void;
  onPreview: (name: string) => void;
  /** Harness-only: open on this step with these answers. */
  initialStep?: Step;
  initialDocuments?: string[];
  initialName?: string;
  /** Harness-only: open on a finished build, or one in progress. */
  initialResult?: knowledge.BuildResult;
  initialBuilding?: boolean;
}) {
  const steps: Step[] = addTo ? ["documents", "review"] : ["documents", "name", "review"];
  const [at, setAt] = useState(Math.max(0, steps.indexOf(initialStep ?? "documents")));
  const [docs, setDocs] = useState<string[]>(initialDocuments ?? []);
  const [name, setName] = useState(initialName ?? addTo ?? "");
  const [problem, setProblem] = useState<string | null>(null);
  const [model, setModel] = useState("");
  const [setup, setSetup] = useState<main.SetupStatusView | null>(null);
  const [building, setBuilding] = useState(initialBuilding ?? false);
  const [log, setLog] = useState<string[]>([]);
  const [result, setResult] = useState<knowledge.BuildResult | null>(initialResult ?? null);
  const [error, setError] = useState<string | null>(null);
  const heading = useRef<HTMLHeadingElement | null>(null);
  const logRef = useRef<HTMLPreElement | null>(null);
  // Focus follows the step once the person has moved, not on opening.
  const moved = useRef(false);
  const step = steps[at];

  useEffect(() => {
    KnowledgeAPI.CreatorModel().then(setModel).catch(() => setModel("none configured"));
    RunAPI.SetupStatus().then(setSetup).catch(() => setSetup(null));
  }, []);
  useEffect(() => EventsOn("knowledge:build-log", (line: string) => setLog((p) => [...p, line])), []);
  useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [log]);
  useEffect(() => {
    if (moved.current) heading.current?.focus();
  }, [at]);

  const goTo = (i: number) => {
    moved.current = true;
    setProblem(null);
    setAt(Math.max(0, Math.min(steps.length - 1, i)));
  };
  function next() {
    const p = step === "documents" && docs.length === 0 ? "Add at least one document or folder." : step === "name" ? nameProblem(name) : null;
    if (p) {
      setProblem(p);
      heading.current?.focus();
      return;
    }
    goTo(at + 1);
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
  if (setup && !setup.dockerOk) blockers.push(`Docker is not ready: ${setup.dockerErr || "it did not answer"}. Settings → Runtime says how to set it up.`);
  if (model === "none configured") blockers.push("No model is set up for runs on this computer: choose one in Settings → Runtime.");

  async function build() {
    setBuilding(true);
    setError(null);
    setLog([]);
    try {
      setResult(addTo ? await KnowledgeAPI.Add(registry, addTo, docs) : await KnowledgeAPI.Create(registry, name, docs));
    } catch (e) {
      setError(String(e));
    } finally {
      setBuilding(false);
    }
  }

  const title = addTo ? `Add documents to ${addTo}` : "Create knowledge";
  const running = building || result || error;
  return (
    <Modal title={title} onClose={building ? () => {} : onClose} wide>
      {!running && (
        <>
          <StepList labels={steps.map((s) => LABEL[s])} at={at} />
          <section aria-labelledby="knowledge-step-heading">
            <StepHeading ref={heading} id="knowledge-step-heading" at={at} total={steps.length} label={LABEL[step]} />

            {step === "documents" && (
              <>
                <p className="muted small">
                  Markdown, text, HTML, CSV, JSON, YAML and source files are read as they are; PDF, DOCX, ODT, EPUB, RTF, PPTX
                  and XLSX, and PNG, JPEG, TIFF or WebP images, as the text decoded from them on this computer first. The
                  documents are kept in the pack beside its pages; a pack, its documents included, is at most 10 MB.
                  {addTo && <> The Creator reads {addTo} as it stands and rewrites the pages the new documents touch.</>}
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
                <div className="row">
                  <Button onClick={addFiles}>Add files…</Button>
                  <Button onClick={addFolder}>Add folder…</Button>
                </div>
              </>
            )}

            {step === "name" && (
              <Field label="Name" htmlFor="knowledge-name" hint={<>A run finds the pack at <span className="mono">/knowledge/{name || "name"}</span>. Lowercase letters, digits, &apos;.&apos;, &apos;-&apos; and &apos;_&apos;.</>}>
                <Input id="knowledge-name" value={name} autoFocus onChange={(e) => setName(e.target.value.trim())} onKeyDown={(e) => e.key === "Enter" && next()} />
              </Field>
            )}

            {step === "review" && (
              <>
                <Recap
                  rows={[
                    { label: "Documents", value: docs.length === 1 ? docs[0] : `${docs.length} files and folders`, onEdit: () => goTo(steps.indexOf("documents")) },
                    ...(addTo ? [] : [{ label: "Name", value: name, onEdit: () => goTo(steps.indexOf("name")) }]),
                    { label: "Where", value: "This computer, against your runner policy's model" },
                  ]}
                />
                <Notice tone="warn">{unscannedWarning(model || "…")}</Notice>
                {blockers.map((b) => (
                  <p key={b} className="step-problem" role="alert">{b}</p>
                ))}
              </>
            )}
          </section>
          <StepFoot problem={problem} onBack={at > 0 ? () => goTo(at - 1) : undefined}>
            {step === "review" ? (
              <Button variant="primary" disabled={blockers.length > 0 || !setup} onClick={build}>
                {addTo ? "Add documents" : "Create knowledge"}
              </Button>
            ) : (
              <Button variant="primary" onClick={next}>Next: {LABEL[steps[at + 1]]}</Button>
            )}
          </StepFoot>
        </>
      )}

      {building && (
        <>
          <Spinner label={addTo ? `Adding documents to ${addTo}…` : `Building ${name}…`} />
          <p className="muted small">The Knowledge Creator is decoding the documents and writing the pages. A large pack takes a few minutes.</p>
          <pre ref={logRef} className="run-log selectable" aria-label="Build log">{log.join("\n")}</pre>
        </>
      )}

      {!building && error && (
        <>
          <h3 className="run-outcome run-outcome-bad">{addTo ? `Nothing was added to ${addTo}` : "No pack was made"}</h3>
          <ErrorBox error={error} />
          {log.length > 0 && <pre ref={logRef} className="run-log selectable" aria-label="Build log">{log.join("\n")}</pre>}
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => { setError(null); setLog([]); }}>Back</Button>
            <Button onClick={onClose}>Close</Button>
          </div>
        </>
      )}

      {!building && result && <BuildDone result={result} added={!!addTo} onClose={onClose} onPreview={() => onPreview(result.name)} />}
    </Modal>
  );
}

function BuildDone({ result, added, onClose, onPreview }: { result: knowledge.BuildResult; added: boolean; onClose: () => void; onPreview: () => void }) {
  const docs = result.documents ?? [];
  return (
    <div>
      <h3 className={`run-outcome run-outcome-${result.stopped ? "warn" : "ok"}`}>
        {added ? `Added ${docs.length === 1 ? "1 document" : `${docs.length} documents`} to ${result.name}` : `Made ${result.name}`}: {pagesOf(result.pages)}, {fmtBytes(result.bytes)}
      </h3>
      <p>Kept on this computer, encrypted to {result.key}. It is not scanned.</p>
      {result.stopped && (
        <Notice tone="warn">
          The Creator run stopped before it finished ({result.stopped}). The pack holds what it wrote; Add documents with the
          same documents continues it.
        </Notice>
      )}
      {(result.rewrapped ?? []).length > 0 && (
        <p className="muted small">Moved to your account&apos;s key first: {result.rewrapped!.join(", ")}.</p>
      )}
      {result.agentryStale && <p className="muted small">Agentry still keeps the copy from before this build; Upload to Agentry sends this one.</p>}
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
        {(result.inputTokens > 0 || result.outputTokens > 0) && (
          <>
            <dt>The Creator run used</dt>
            <dd>{result.inputTokens} input and {result.outputTokens} output tokens{result.costUsd > 0 ? `, US$${result.costUsd.toFixed(4)} as the provider reported it` : ""}</dd>
          </>
        )}
      </dl>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <Button onClick={onPreview}>Preview</Button>
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
