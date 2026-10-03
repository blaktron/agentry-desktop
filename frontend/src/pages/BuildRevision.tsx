// Edit with AI's revision, landed (agentry-desktop#501; plan
// drafter-revisions.md §5): the QuickAgent and the versions, the Drafter's
// notes, a diff of agent.yaml and the instruction file against the release
// (both written by the form's own writer, so it is what Publish sends), and
// the two actions the site gives (agentry-web#862):
//   - Publish this version: Go saves the revision as drafted (its own copy,
//     never the form's) into an empty folder the native picker gives and
//     submits it, naming the run when it was drafted In Agentry;
//   - Open in the Builder: the form, where it is the person's own change and
//     Submit names no run.
// Every text the Drafter or the registry wrote is drawn as text, invisible
// characters revealed (DEV-138).
import type { ReactNode } from "react";
import { main } from "../../wailsjs/go/models";
import { RevealText } from "../reveal";
import { Button, Card, Notice } from "../ui";
import { DraftNotes } from "./BuildDescribe";

export function RevisionView({
  v, busy, outcome, onPublish, onOpenInBuilder, onOpenAgent,
}: {
  v: main.DraftView;
  busy: string | null;
  outcome: ReactNode;
  onPublish: () => void;
  onOpenInBuilder: () => void;
  onOpenAgent?: (ns: string, name: string) => void;
}) {
  const r = v.revision!;
  const where = r.hosted ? "In Agentry" : "on this computer";
  return (
    <div>
      <h2 style={{ marginBottom: 4 }}>A revision of {r.namespace}/{r.name} {r.from}</h2>
      <p className="muted small" style={{ marginTop: 0 }}>
        Edit with AI drafted version {r.version} {where}.{" "}
        {onOpenAgent && <Button variant="link" onClick={() => onOpenAgent(r.namespace, r.name)}>Open its page</Button>}
      </p>
      <p className="small">
        {r.hosted
          ? "Published as drafted, it names the draft it came from, and the registry checks the files against that draft's record before the rug-pull check (S9) gives it the benefit of the doubt. Changed in the Builder, it is your own change and is scanned as any new version is."
          : "A revision made on this computer leaves no record in Agentry, so it is scanned as any new version is."}
      </p>
      {v.unmentioned.length > 0 && (
        <Notice tone="warn">
          It reaches {v.unmentioned.length === 1 ? "a site" : "sites"} the release did not and your texts do not name:{" "}
          <span className="mono selectable">{v.unmentioned.join(", ")}</span>. Check that you want this before you publish it.
        </Notice>
      )}

      <h3>What changed</h3>
      {r.diff.map((f) => (
        <Card key={f.path} className="mt">
          <strong className="mono"><RevealText text={f.path} /></strong>
          {f.same
            ? <p className="muted small" style={{ marginBottom: 0 }}>Unchanged.</p>
            : (
              <pre className="revision-diff selectable" aria-label={`Changes to ${f.path}`}>
                {f.lines.map((l, i) => (
                  <div key={i} className={l.op === "+" ? "add" : l.op === "-" ? "del" : l.op === "~" ? "fold" : "same"}>
                    <span className="op" aria-hidden="true">{l.op === "~" ? "…" : l.op}</span>
                    {l.op === "+" && <span className="sr-only">added: </span>}
                    {l.op === "-" && <span className="sr-only">removed: </span>}
                    <RevealText text={l.text} />
                  </div>
                ))}
              </pre>
            )}
        </Card>
      ))}

      <details className="card mt build-drafted">
        <summary><strong>How it was drafted</strong></summary>
        <DraftNotes v={v} onOpenAgent={onOpenAgent} />
      </details>

      <div className="row mt">
        <Button variant="primary" disabled={!!busy} onClick={onPublish}>{busy === "submit" ? "Publishing…" : "Publish this version"}</Button>
        <Button disabled={!!busy} onClick={onOpenInBuilder}>Open in the Builder</Button>
      </div>
      <ul className="small">
        <li><strong>Publish this version</strong> saves it as drafted into an empty folder you choose and submits {r.namespace}/{r.name}@{r.version} to the security pipeline. It first says what the submission costs, and sends nothing until you agree.</li>
        <li><strong>Open in the Builder</strong> puts it in the form to change before you save or submit it.</li>
      </ul>
      {outcome}
    </div>
  );
}
