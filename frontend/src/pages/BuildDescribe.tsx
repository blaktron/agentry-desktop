// Describe it (plan builder-rework.md §7.1, D5, D22, D25; #282): the Build
// tab's third way in. A description, three optional hints, and a draft made
// on this computer by the built-in Drafter against the runner policy's
// model, with the person's own AI Review key reading every input before the
// model does, for no credits. BuildAPI.DescribeOnThisComputer refuses
// without a key and asks in a native dialog before anything leaves this
// computer; the page only says so first.
//
// DraftNotes is what the Drafter said, shown beside Review once a draft is
// in the form. All of it is agent-written: the notes render through the
// run-output reader (AgentMarkdown: no raw HTML, no image, links only to the
// system browser), a withheld address is plain text and never a link, and
// every other field is text.
import { useEffect, useRef, useState } from "react";
import * as BuildAPI from "../../wailsjs/go/main/BuildAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { main } from "../../wailsjs/go/models";
import { AgentMarkdown, ExternalLink, webHref } from "../AgentMarkdown";
import { RevealText } from "../reveal";
import { Button, Card, ErrorBox, Notice, Textarea } from "../ui";

const HINTS = [
  { key: "services", label: "The services or sites it works with", placeholder: "e.g. GitHub pull requests, a CSV I add" },
  { key: "how", label: "Whether it runs on its own or in a conversation", placeholder: "e.g. on its own, from the files I add" },
  { key: "example", label: "An example of the output you want", placeholder: "e.g. a table of problems per column" },
] as const;

// The description the page keeps while the tab is away, as Build keeps its
// form (the tab unmounts on every change).
let keptDescription = { text: "", hints: ["", "", ""] };

export function DescribePanel({ registry, onDrafted, onOpenSettings }: {
  registry: string;
  onDrafted: (v: main.DraftView) => void;
  onOpenSettings?: () => void;
}) {
  const [text, setText] = useState(keptDescription.text);
  const [hints, setHints] = useState<string[]>(keptDescription.hints);
  const [status, setStatus] = useState<main.DescribeStatusView | null>(null);
  const [setup, setSetup] = useState<main.SetupStatusView | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [log, setLog] = useState<string[]>([]);
  const logRef = useRef<HTMLPreElement | null>(null);

  useEffect(() => {
    BuildAPI.DescribeStatus().then(setStatus).catch((e) => setError(String(e)));
    RunAPI.SetupStatus().then(setSetup).catch(() => {});
  }, []);
  useEffect(() => { keptDescription = { text, hints }; }, [text, hints]);
  useEffect(() => EventsOn("build:draft-log", (line: string) => setLog((p) => [...p, line])), []);
  useEffect(() => { logRef.current?.scrollTo({ top: logRef.current.scrollHeight }); }, [log]);

  const key = status?.key;
  const dockerOk = setup?.dockerOk ?? false;
  const ready = !!key?.ready && dockerOk && text.trim() !== "";

  async function draft() {
    setBusy(true);
    setError(null);
    setLog([]);
    try {
      const v = await BuildAPI.DescribeOnThisComputer(registry, text, hints.map((h) => h.trim()).filter(Boolean));
      keptDescription = { text: "", hints: ["", "", ""] };
      onDrafted(v);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section aria-labelledby="describe-heading">
      <h2 id="describe-heading">Describe it</h2>
      <p className="muted small">
        Say what the BriefAgent should do, in plain words. The Drafter searches the registry and the web, reads what it
        finds, and drafts the BriefAgent: what it may reach, its instructions, the questions a run asks and what a person
        reads first. The draft opens on Review, and nothing is saved or submitted until you do it.
      </p>
      <Card>
        <label className="small" style={{ display: "block" }}>Describe what the BriefAgent should do
          <Textarea rows={6} style={{ width: "100%" }} value={text} disabled={busy} maxLength={30000}
            onChange={(e) => setText(e.target.value)}
            placeholder="e.g. Check a CSV file I add for missing values, duplicate rows and columns whose values are not all the same format, and report the problems per column." />
        </label>
        <p className="small" style={{ marginBottom: 4 }}>Hints, if you have them</p>
        {HINTS.map((h, i) => (
          <label key={h.key} className="small" style={{ display: "block", marginTop: 6 }}>{h.label}
            <input type="text" style={{ width: "100%" }} value={hints[i]} placeholder={h.placeholder} disabled={busy}
              onChange={(e) => setHints(hints.map((x, j) => (j === i ? e.target.value : x)))} />
          </label>
        ))}
      </Card>

      <h3 className="small">Where it runs</h3>
      <Card>
        <label className="check-row" style={{ margin: 0 }}>
          <input type="radio" name="describe-where" checked readOnly />
          <span>
            <strong>On this computer</strong>{" "}
            <span className="muted">
              — against the model your runner policy names ({status?.model ?? "…"}), for no credits. Your own AI Review
              key reads the description and every result first.
            </span>
          </span>
        </label>
        {key && (key.ready
          ? <p className="small" style={{ marginBottom: 0 }}>AI Review uses your key {key.envSet ? <>from <span className="mono">${key.envName}</span></> : "from this computer's keychain"}.</p>
          : (
            <Notice tone="warn">
              Drafting on this computer needs your own TypeSafe Jev key, because AI Review reads your description and
              everything the draft reads with it before the model does.{" "}
              {onOpenSettings && <Button variant="link" onClick={onOpenSettings}>Add one in Settings</Button>}
            </Notice>
          ))}
        {status && <p className="muted small" style={{ marginBottom: 0 }}>{status.searchSentence}</p>}
        {setup && !dockerOk && (
          <Notice tone="warn">The Drafter runs in containers, and docker is not ready on this computer{setup.dockerErr ? `: ${setup.dockerErr}` : ""}. Settings → Agent runs sets it up.</Notice>
        )}
      </Card>

      <p className="small">
        Your description, the Drafter&apos;s searches and the pages it reads go to TypeSafe (AI Review), to your search
        provider when you have one, and to the sites it reads, so the description should hold no secret. AI Review reads
        every result before the Drafter&apos;s model does, and holds back what it flags.
      </p>
      <div className="row">
        <Button variant="primary" disabled={!ready || busy} onClick={draft}>{busy ? "Drafting…" : "Draft it"}</Button>
        {busy && <span className="muted small">A draft takes a few minutes. It asks first, then runs as this app&apos;s one run.</span>}
      </div>
      {log.length > 0 && <pre ref={logRef} className="run-log" style={{ maxHeight: 200, overflow: "auto", marginTop: 12 }}>{log.join("\n")}</pre>}
      <ErrorBox error={error} />
    </section>
  );
}

/** A registry match's reference split into its page's namespace and name. */
function refParts(ref: string): { ns: string; name: string } | null {
  const m = /^([a-z0-9][a-z0-9-]*)\/([a-z0-9][a-z0-9-]*)(@[^\s]+)?$/.exec(ref.trim());
  return m ? { ns: m[1], name: m[2] } : null;
}

export function DraftNotes({ v, onOpenAgent }: { v: main.DraftView; onOpenAgent?: (ns: string, name: string) => void }) {
  const ai = v.aiReview;
  const heldBack = ai.withheld + ai.notScanned;
  return (
    <Card className="mt">
      <strong>What the Drafter found</strong>
      <p className="small">
        AI Review read {ai.read} {ai.read === 1 ? "result" : "results"} with your key and held back {heldBack}
        {ai.notScanned > 0 ? ` (${ai.notScanned} could not be scanned)` : ""}. The Drafter was given the address and the
        reason for each one held back, never its text.
      </p>
      {(ai.withheldSources ?? []).length > 0 && (
        <ul className="small">
          {ai.withheldSources.map((w, i) => (
            <li key={i}><span className="mono selectable"><RevealText text={w.url} /></span> ({w.tool}): {w.reason}</li>
          ))}
        </ul>
      )}
      {v.manifestError && <Notice tone="warn">The draft is not finished: <RevealText text={v.manifestError} /></Notice>}
      {(v.decodeNotes ?? []).map((n, i) => <Notice key={i} tone="info"><RevealText text={n} /></Notice>)}
      {(v.matches ?? []).length > 0 && (
        <>
          <h4 className="small">Already in the registry</h4>
          {v.matches.map((m, i) => {
            const at = refParts(m.ref);
            return (
              <div key={i} className="small" style={{ marginTop: 6 }}>
                <span className="mono selectable"><RevealText text={m.ref} /></span>: <RevealText text={m.why} />{" "}
                {at && onOpenAgent && <Button variant="link" onClick={() => onOpenAgent(at.ns, at.name)}>Open its page to install it instead</Button>}
              </div>
            );
          })}
        </>
      )}
      {(v.openQuestions ?? []).length > 0 && (
        <>
          <h4 className="small">What the description leaves open</h4>
          <ul className="small">{v.openQuestions.map((q, i) => <li key={i}><RevealText text={q} /></li>)}</ul>
        </>
      )}
      {v.notes.trim() && (
        <>
          <h4 className="small">The Drafter&apos;s notes</h4>
          <AgentMarkdown text={v.notes} />
        </>
      )}
      {(v.sources ?? []).length > 0 && (
        <details className="small">
          <summary>The sources it read ({v.sources.length})</summary>
          <ul>
            {v.sources.map((s, i) => {
              const href = s.withheld ? null : webHref(s.url);
              return (
                <li key={i}>
                  {href ? <ExternalLink href={href}>{href}</ExternalLink> : <span className="mono selectable"><RevealText text={s.url} /></span>}
                  {s.withheld ? " (held back by AI Review)" : ""}{s.used ? <>: <RevealText text={s.used} /></> : null}
                </li>
              );
            })}
          </ul>
        </details>
      )}
      <p className="muted small selectable" style={{ marginBottom: 0 }}>
        The run&apos;s receipts and <span className="mono">draft.json</span> are in <span className="mono">{v.outDir}</span>.
      </p>
    </Card>
  );
}
