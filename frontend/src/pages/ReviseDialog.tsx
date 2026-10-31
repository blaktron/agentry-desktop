// Edit with AI on the QuickAgent page: Additional instructions and Problems
// with this draft, then where the Drafter revises the current release.
// In Agentry is for a payer above Standard, at a hosted draft's terms; a
// Standard account is told so and offered this computer, which is for
// anyone, with their own AI Review key, for no credits. Starting it opens
// the draft's progress dialog (DraftFlow), and the revision lands in the
// Build tab with its diff (BuildRevision).
import { useEffect, useState } from "react";
import * as BuildAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/buildapi";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import * as SettingsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/settingsapi";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { startRevision, useDraft } from "../drafts";
import { MAX_REVISION_TEXT_BYTES, canStart, firstWhere, revisionBytes, textsProblem, type ReviseWhere, type ReviseWays } from "../revise";
import { Button, Card, ErrorBox, Modal, Notice, Textarea } from "../ui";
import { GetAKeyLink } from "./AIReviewKey";

export default function ReviseDialog({
  registry, ns, name, version, onClose, onOpenSettings,
}: {
  registry: string;
  ns: string;
  name: string;
  /** The current release the revision starts from. */
  version: string;
  onClose: () => void;
  /** Settings at the AI Review key, for the on-this-computer way. */
  onOpenSettings?: () => void;
}) {
  const [instructions, setInstructions] = useState("");
  const [problems, setProblems] = useState("");
  const [where, setWhere] = useState<ReviseWhere | null>(null);
  const [agentry, setAgentry] = useState<app.DescribeHostedView | null>(null);
  const [status, setStatus] = useState<app.DescribeStatusView | null>(null);
  const [setup, setSetup] = useState<app.SetupStatusView | null>(null);
  const [error, setError] = useState<string | null>(null);
  // One draft at a time, as Describe it allows (drafts.ts).
  const d = useDraft();

  useEffect(() => {
    BuildAPI.DescribeInAgentryStatus(registry).then(setAgentry).catch(() => setAgentry(null));
    BuildAPI.DescribeStatus().then(setStatus).catch((e) => setError(String(e)));
    RunAPI.SetupStatus().then(setSetup).catch(() => {});
  }, [registry]);

  const ways: ReviseWays = {
    // A Standard payer is not offered In Agentry only a line says why.
    agentryOffered: !!agentry?.reviseOffered && !agentry.reviseUpgrade,
    agentryAvailable: !!agentry?.reviseAvailable,
    keyReady: !!status?.key?.ready,
    dockerOk: setup?.dockerOk ?? false,
  };
  useEffect(() => {
    if (where === null && agentry !== null && status !== null) setWhere(firstWhere(ways));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentry, status, where]);

  const problem = textsProblem(instructions, problems);
  const ready = !d.inFlight && where !== null && canStart(where, ways, instructions, problems);

  function start() {
    if (!where) return;
    startRevision(registry, where, ns, name, instructions, problems);
    onClose();
  }

  const count = (t: string) => `${revisionBytes(t)} of ${MAX_REVISION_TEXT_BYTES} bytes`;
  return (
    <Modal title={`Edit ${ns}/${name} with AI`} onClose={onClose}>
      <p className="small" style={{ marginTop: 0 }}>
        The Drafter reads version {version} and drafts its next version from what you write here. You see what changed before
        anything is published.
      </p>
      <label className="small" style={{ display: "block" }}>Additional instructions
        <Textarea rows={4} style={{ width: "100%" }} value={instructions} onChange={(e) => setInstructions(e.target.value)}
          placeholder="e.g. Also give each action item a due date when the notes name one." />
        <span className="muted">{count(instructions)}</span>
      </label>
      <label className="small" style={{ display: "block", marginTop: 8 }}>Problems with this draft
        <Textarea rows={4} style={{ width: "100%" }} value={problems} onChange={(e) => setProblems(e.target.value)}
          placeholder="e.g. It misses dates written as 3 March." />
        <span className="muted">{count(problems)}</span>
      </label>

      <h3 className="small">Where it drafts</h3>
      {agentry?.reviseOffered && !agentry.reviseUpgrade && (
        <Card className={where === "agentry" ? "selected" : ""}>
          <label className="small" style={{ display: "block" }}>
            <input type="radio" name="revise-where" checked={where === "agentry"} disabled={!agentry.reviseAvailable}
              onChange={() => setWhere("agentry")} />{" "}
            <strong>In Agentry</strong>{" "}
            <span className="muted">— against the platform&apos;s model, for credits. AI Review reads your texts and the release through the registry.</span>
          </label>
          {agentry.reviseAvailable && agentry.holdSentence && <p className="small" style={{ marginBottom: 0 }}>{agentry.holdSentence}</p>}
          {!agentry.reviseAvailable && agentry.reviseReason && (
            <Notice tone="warn">
              {agentry.reviseReason}{agentry.reviseRemedy ? ` ${agentry.reviseRemedy}` : ""}
            </Notice>
          )}
        </Card>
      )}
      <Card className={where === "here" ? "selected mt" : "mt"}>
        <label className="small" style={{ display: "block" }}>
          <input type="radio" name="revise-where" checked={where === "here"} onChange={() => setWhere("here")} />{" "}
          <strong>On this computer</strong>{" "}
          <span className="muted">
            — against the model your runner policy names ({status?.modelName ?? "…"}), for no credits. Your own AI Review key reads
            your texts and the release first. It leaves no record in Agentry, so it is scanned as any new version is.
          </span>
        </label>
        {status?.key && !status.key.ready && (
          <Notice tone="warn">
            Editing with AI on this computer needs your own TypeSafe Jev key, because AI Review reads your texts and the release
            before the model does.{" "}
            {onOpenSettings && <Button variant="link" onClick={onOpenSettings}>Add one in Settings</Button>}{" "}
            <GetAKeyLink />
          </Notice>
        )}
        {agentry?.reviseUpgrade && (
          <p className="muted small" style={{ marginBottom: 0 }}>
            {agentry.reviseReason}{" "}
            <Button variant="link" onClick={() => SettingsAPI.OpenUpgrade(registry).catch((e) => setError(String(e)))}>See Pro</Button>
          </p>
        )}
        {setup && !ways.dockerOk && (
          <Notice tone="warn">The Drafter runs in containers, and the container engine is not ready on this computer{setup.dockerErr ? `: ${setup.dockerErr}` : ""}. Settings → Run defaults sets it up.</Notice>
        )}
      </Card>

      {(instructions !== "" || problems !== "") && problem && <p className="small step-problem">{problem}</p>}
      {d.inFlight && <Notice tone="warn">A draft is already in progress. Wait for it to finish, or stop it, before you start another.</Notice>}
      <ErrorBox error={error} />
      <div className="row mt" style={{ justifyContent: "flex-end" }}>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="primary" disabled={!ready} onClick={start}>Draft the next version</Button>
      </div>
    </Modal>
  );
}
