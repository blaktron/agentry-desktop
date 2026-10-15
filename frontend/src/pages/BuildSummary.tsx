// The Build tab's summary, the site's over the desktop's
// form: the one short page a QuickAgent is submitted from, where a draft
// lands and where both form views end. How it works, then what it needs:
// lands and where both form views end. How it works, then what it needs:
// every key with its help, the websites, each MCP server with its tools, and
// each file, command or data grant, drawn from BuildAPI.Summary (the
// capability contract in full, and nothing undeclared: internal/build
// Summarize and its test). Then the three actions below, and See it in
// the form. The plumbing (the Drafter's notes, sources, AI Review's counts,
// the namespace, version, licence, tags, include list, file name and run
// mode) stays in the form views.
//
// It ends on three actions: Save writes the QuickAgent to a folder (a new
// one under the default output folder, no dialog); Install puts it on this
// computer only, local and unsigned, with no registry and no credit; Submit
// sends it to the registry publicly, after a native warning that names what
// it costs. The two dialogs are the Go side's, which the page cannot answer.
// Beside them, Review Your QuickAgents locally saves it
// and reviews the folder with the built-in Examiner, after its own native
// dialog; it holds none of the three.
//
// Every string here may be the Drafter's: the overview and each key's help
// go through AgentMarkdown's rules (AuthorHelp frames the help as the
// author's), and the rest is text through RevealText.
import type { ReactNode, Ref } from "react";
import { build, app } from "../../wailsjs/go/models";
import { AgentMarkdown } from "../AgentMarkdown";
import { AuthorHelp } from "../KeyHelp";
import { ReviewAction } from "./ReviewAction";
import { RevealText } from "../reveal";
import { Button, Card } from "../ui";

function Hosts({ hosts, label }: { hosts: build.SummaryHost[]; label: string }) {
  return (
    <ul className="small build-summary-list" aria-label={label}>
      {hosts.map((h, i) => (
        <li key={i}>
          <span className="mono selectable">{h.host === "*" ? "any host (*)" : <RevealText text={h.host} />}</span>
          {h.unnamed && <span className="badge warn" style={{ marginLeft: 6 }}>your description did not name it</span>}
        </li>
      ))}
    </ul>
  );
}

export function SummaryView({
  s, check, problem, publicProblem, onForm, onSave, onInstall, onSubmit, onReview, onOpenReviewKey, busy, ready, dir, registry, headingRef, outcome,
}: {
  s: build.Summary;
  /** AI Review's check of a draft as one line, or null (a form-written QuickAgent has none). */
  check: app.DraftCheckView["line"] | null;
  /** The first rule the form still fails, and the step that fixes it; null when it passes. */
  problem: { label: string; text: string; fix: () => void } | null;
  /** The rule a public submission still fails (V7), and the step that fixes
   * it; it holds Submit alone, not Save or a local Install. */
  publicProblem: { label: string; text: string; fix: () => void } | null;
  onForm: (v: "steps" | "advanced") => void;
  onSave: () => void;
  onInstall: () => void;
  onSubmit: () => void;
  /** Review Your QuickAgents locally: saves, then reviews the folder. */
  onReview: () => void;
  /** Settings at the AI Review key. */
  onOpenReviewKey?: () => void;
  /** The action under way: "save", "install", "submit" or "review". */
  busy: string | null;
  /** Where Submit sends it. */
  registry: string;
  /** The form's check has answered and nothing else is under way. */
  ready: boolean;
  dir: string;
  headingRef?: Ref<HTMLHeadingElement>;
  /** The publish log and its verdict, once Submit ran. */
  outcome?: ReactNode;
}) {
  return (
    <section aria-labelledby="build-summary-heading" className="build-summary">
      <h2 id="build-summary-heading" ref={headingRef} tabIndex={-1}>Summary</h2>
      <p className="muted small">How the QuickAgent works and what it needs. Save it, install it here, or submit it when this is what you want; every other choice is in the form.</p>

      <Card>
        <strong>How it works</strong>
        <p style={{ overflowWrap: "anywhere" }}>
          <span className="mono selectable">{s.name ? <RevealText text={s.name} /> : "no name yet"}</span>:{" "}
          {s.summary ? <RevealText text={s.summary} /> : "no summary yet"}
        </p>
        {s.overview.trim() !== "" && <AgentMarkdown text={s.overview} showHosts />}
      </Card>

      <Card className="mt">
        <strong>What it needs</strong>
        {s.anyUnnamed && (
          <p className="small build-summary-unnamed">
            It reaches hosts your description did not name, marked below. Check each one before you submit: a host nobody
            asked for is what an injected page would add.
          </p>
        )}

        <h4 className="small">Keys</h4>
        {s.keys.length === 0 ? <p className="muted small">None.</p> : (
          <ul className="small build-summary-list" aria-label="Keys">
            {s.keys.map((k) => (
              <li key={k.name}>
                <span className="mono selectable"><RevealText text={k.name} /></span>
                <span className="muted">, for <RevealText text={k.usedBy.join(" and ")} />
                  {k.optional ? "; optional: a run starts without it" : "; required: a run cannot start without it"}</span>
                {k.help ? <AuthorHelp name={k.name} help={k.help} /> : <div className="muted small">No help is written for this key.</div>}
              </li>
            ))}
          </ul>
        )}
        <p className="muted small">A key is never packed: whoever runs the QuickAgent supplies it.</p>

        <h4 className="small">Websites</h4>
        {s.websites.length === 0 ? <p className="muted small">None.</p> : <Hosts hosts={s.websites} label="Websites" />}

        <h4 className="small">MCP servers</h4>
        {s.servers.length === 0 ? <p className="muted small">None.</p> : (
          <ul className="small build-summary-list" aria-label="MCP servers">
            {s.servers.map((sv, i) => (
              <li key={i}>
                <span className="mono selectable"><RevealText text={sv.ref} /></span>
                <div>{sv.tools.length === 0 ? "May call all of its tools." : <RevealText text={`Tools: ${sv.tools.join(", ")}.`} />}</div>
                {sv.keys.length > 0 && <div><RevealText text={`Keys: ${sv.keys.join(", ")}.`} /></div>}
                {sv.config.length > 0 && <div><RevealText text={`Configuration: ${sv.config.join(", ")}.`} /></div>}
                {(sv.signIn ?? []).map((line, k) => <div key={k}><RevealText text={line} /></div>)}
                {sv.hosts.length === 0 ? <div className="muted">Reaches no website.</div> : <Hosts hosts={sv.hosts} label={`Websites ${sv.ref} reaches`} />}
              </li>
            ))}
          </ul>
        )}

        {s.grants.length > 0 && (
          <>
            <h4 className="small">Files, commands and data</h4>
            <ul className="small build-summary-list" aria-label="Files, commands and data">
              {s.grants.map((g) => <li key={g}><RevealText text={g} /></li>)}
            </ul>
          </>
        )}

        {(s.knowledge ?? []).length > 0 && (
          <>
            <h4 className="small">Knowledge</h4>
            <ul className="small build-summary-list" aria-label="Knowledge">
              {s.knowledge.map((k) => <li key={k}><RevealText text={k} /></li>)}
            </ul>
          </>
        )}
      </Card>

      {check && <p className={`small ${check.tone === "review" ? "build-summary-unnamed" : "muted"}`}>{check.text}</p>}

      {problem && (
        <div className="small step-problem" role="alert">
          <p>It cannot be submitted yet. {problem.label}: <RevealText text={problem.text} />.</p>
          <Button onClick={problem.fix}>Fix it in {problem.label}</Button>
        </div>
      )}

      {!problem && publicProblem && (
        <div className="small step-problem" role="alert">
          <p>It can be saved and installed, but not submitted yet: a submission is public. {publicProblem.label}: <RevealText text={publicProblem.text} />.</p>
          <Button onClick={publicProblem.fix}>Fix it in {publicProblem.label}</Button>
        </div>
      )}

      <div className="row" style={{ marginTop: 12 }}>
        <Button disabled={!ready} onClick={onSave}>{busy === "save" ? "Saving…" : "Save"}</Button>
        <Button disabled={!ready || !!problem} onClick={onInstall}>{busy === "install" ? "Installing…" : "Install"}</Button>
        <Button variant="primary" disabled={!ready || !!problem || !!publicProblem} onClick={onSubmit}>{busy === "submit" ? "Submitting…" : "Submit"}</Button>
      </div>
      <ul className="muted small build-summary-actions">
        <li>
          <strong>Save</strong> writes it to {dir ? <span className="mono selectable">{dir}</span> : <>a new folder named for it in your output folder (Settings → Run defaults, or <span className="mono">agentry-out</span> in your Downloads folder)</>}.
        </li>
        <li>
          <strong>Install</strong> saves it there too, then puts it on this computer only, in your Library: private, unsigned, with no
          registry and no credit. Every run asks you to accept that it is unsigned.
        </li>
        <li>
          <strong>Submit</strong> saves it there too, then sends it to <RevealText text={registry.replace(/^https?:\/\//, "")} /> for the security pipeline and
          makes it <strong>public</strong> (one that already exists private stays private). It first says what the submission
          costs, and sends nothing until you agree.
        </li>
      </ul>
      <ReviewAction onReview={onReview} busy={busy === "review"} disabled={!ready} onOpenSettings={onOpenReviewKey} />
      {outcome}
      <p className="small">
        See it in the form: <Button variant="link" onClick={() => onForm("steps")}>step by step</Button> or{" "}
        <Button variant="link" onClick={() => onForm("advanced")}>Advanced</Button>.
      </p>
    </section>
  );
}
