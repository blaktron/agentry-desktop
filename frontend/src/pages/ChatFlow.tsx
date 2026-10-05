// A chat with a QuickAgent, on this computer or In Agentry, followed above
// the pages (agentry-desktop#467; plan desktop-review-key-and-run-cards.md
// §5.3; D4), as a run is (RunFlow, HostedRunFlow): App mounts this, the run
// setup hands the chat over and closes, and App attaches a chat In Agentry
// already under way when it starts, as its card. The dialog is the chat;
// closing it, or Run in the background, shrinks it to a card in the corner
// that says whether the QuickAgent is working or waiting for you, with its
// turns, Open and Finish. Once the chat is over, the dialog is its result
// (the result first, Run details beneath, D-035's rules), and Dismiss makes
// the app forget it.
//
// Every string the chat carries (the conversation, the log, the error, the
// result) is drawn as text. One chat per app (D-017).
import { useEffect, useRef, useState } from "react";
import { headlineOf } from "../localRun";
import { hostedHeadline, hostedTone, lostTouch } from "../hostedRun";
import { CornerCard, LogPane } from "../progress";
import {
  type RunSetup, backToChatSetup, dismissChat, endChat, followHostedRunAgain, showChat, useChat,
} from "../runs";
import { working, type ChatRecord } from "../sessionChat";
import { Button, ErrorBox, Modal } from "../ui";
import { HostedResult, StarAsk } from "./HostedRun";
import { LocalRunDetails, RunResultView } from "./RunResult";
import SessionChat from "./SessionChat";

export default function ChatFlow({ onBackToSetup, initialMinimized }: {
  /** Opens the chat's setup again, with the answers as they were. */
  onBackToSetup: (s: RunSetup) => void;
  /** Harness-only: open as the corner card. */
  initialMinimized?: boolean;
}) {
  const s = useChat();
  const c = s.chat;
  // The card is shown for the ask it was minimized under, or when the ask
  // itself was for the card (an attach at start); Open is a new ask.
  const [cardFor, setCardFor] = useState<number | null>(null);
  const minimized = s.asCard || cardFor === s.raised;
  const minimize = () => setCardFor(s.raised);
  // Harness-only: the first ask to open is shown as the card.
  const firstAsCard = useRef(!!initialMinimized);
  useEffect(() => {
    if (firstAsCard.current && s.raised > 0) {
      firstAsCard.current = false;
      setCardFor(s.raised);
    }
  }, [s.raised]);
  const [following, setFollowing] = useState(false);
  if (!s.shown || !c) return null;

  const where = c.kind === "hosted" ? " In Agentry" : "";
  const title = c.over ? `Chat with ${c.ref}${where}: it is over` : `Chat with ${c.ref}${where}`;

  if (minimized) {
    const { line, tone } = c.over ? outcome(c) : { line: stateLine(c), tone: "" as const };
    return (
      <CornerCard
        label={`Chat with ${c.ref}${where}`}
        running={!c.over}
        title={title}
        line={line}
        tone={tone}
        actions={<>
          {!c.over && !c.starting && (
            <Button onClick={() => void endChat(true)} disabled={c.ending || working(c) || queued(c) || !!c.chat.ended} title="Ask the agent for its final result, then end">
              {c.ending ? "Ending…" : "Finish"}
            </Button>
          )}
          {c.over && <Button onClick={dismissChat}>Dismiss</Button>}
          <Button variant="primary" onClick={showChat}>Open</Button>
        </>}
      />
    );
  }

  if (!c.over) {
    return (
      <Modal title={title} onClose={minimize} wide>
        <SessionChat c={c} />
        <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
          <Button variant="primary" onClick={minimize}>Chat in the background</Button>
        </div>
        <p className="muted small">
          In the background, the chat goes on in a card in the corner and the rest of the app stays usable. Open brings
          the conversation back; Finish asks the agent for its final result, then ends.
        </p>
      </Modal>
    );
  }

  const back = () => {
    const setup = backToChatSetup();
    if (setup) onBackToSetup(setup);
  };
  const lost = !!c.run && lostTouch(c.run);
  const again = () => {
    setFollowing(true);
    void followHostedRunAgain(c.registry).finally(() => setFollowing(false));
  };
  // Closing the dialog of a chat that is over keeps its card: only Dismiss
  // makes the app forget it.
  return (
    <Modal title={title} onClose={minimize} wide>
      <ChatResult c={c} />
      <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
        {lost && <Button onClick={again} disabled={following}>{following ? "Following…" : "Follow it again"}</Button>}
        {s.setup && <Button onClick={back}>Back to the setup</Button>}
        <Button variant="primary" onClick={dismissChat}>Dismiss</Button>
      </div>
    </Modal>
  );
}

// stateLine is the card's line while the chat goes: what the QuickAgent is
// doing, and the person's turns.
function stateLine(c: ChatRecord): string {
  const turns = c.chat.turns === 1 ? "1 turn" : `${c.chat.turns} turns`;
  if (c.starting) return "Starting the sandbox and the agent…";
  if (c.ending) return `Ending the chat… · ${turns}`;
  if (c.chat.ended) return `The chat has ended; its output is on its way · ${turns}`;
  if (queued(c)) return "Waiting for a sandbox host…";
  return `${working(c) ? "The QuickAgent is working" : "Waiting for you"} · ${turns}`;
}

// queued: a chat In Agentry no sandbox host has taken yet.
function queued(c: ChatRecord): boolean {
  return c.kind === "hosted" && !c.chat.ended && c.hosted?.status !== "running";
}

// outcome is how the chat ended, in the card's line and tone.
function outcome(c: ChatRecord): { line: string; tone: "ok" | "warn" | "bad" } {
  if (c.kind === "hosted" && c.run) {
    return { line: hostedHeadline(c.run), tone: hostedTone(c.run) };
  }
  if (c.result) return headlineOf(c.result);
  return { line: c.chat.turns === 0 && !c.chat.ended ? "The chat did not start." : "The chat ended with no result.", tone: "bad" };
}

// ChatResult is a chat that is over: its result first, from its envelope
// (on this computer) or its sealed output (In Agentry), with Run details
// beneath; or why there is none.
function ChatResult({ c }: { c: ChatRecord }) {
  if (c.kind === "hosted" && c.run && !lostTouch(c.run)) {
    const run = c.run;
    return (
      <RunResultView
        runId={run.id}
        headline={hostedHeadline(run)}
        tone={hostedTone(run)}
        error={run.downloadError || undefined}
        resultJson={run.resultJson ?? ""}
        knowledge={run.knowledgeLine || undefined}
        ask={<StarAsk registry={c.registry} run={run} />}
      >
        <HostedResult run={run} />
      </RunResultView>
    );
  }
  if (c.kind === "local" && c.result) {
    const r = c.result;
    return (
      <RunResultView
        runId={r.runId}
        headline={headlineOf(r).line}
        tone={headlineOf(r).tone}
        error={r.error || undefined}
        resultJson={r.resultJson}
        knowledge={r.knowledge}
        unsafeExec={r.unsafeExec}
      >
        <LocalRunDetails result={r} log={c.log} />
      </RunResultView>
    );
  }
  const { line, tone } = outcome(c);
  return (
    <>
      <h3 className={`run-outcome run-outcome-${tone}`}>{line}</h3>
      <ErrorBox error={c.error ?? c.run?.downloadError ?? null} />
      {c.log.length > 0 && <LogPane label="Chat log" lines={c.log} />}
    </>
  );
}
