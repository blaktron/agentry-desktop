// A message waiting for the agent's next turn, drawn just above the chat's
// composer: the text, dimmed, as the person's line, and under it a short
// line of what can still be done with it. Send now stops the turn in flight
// and sends it; Edit takes it back into the composer; Add to this turn, where
// the turn takes one, hands it to the turn in flight. The QuickAgent chat,
// a persistent agent's page and Home's agent card draw it alike. A long
// message is cut to its first lines here (Edit shows the whole of it), so
// the composer under it stays in view.
import { Button } from "../ui";
import { QUEUED_SAID, QUEUED_WORDS } from "../queued";

export function QueuedMessage({ text, bubble, flush, note, onSendNow, sendingNow, onEdit, onAddToTurn, adding }: {
  text: string;
  /** Without the side padding, where the chat around it has its own. */
  flush?: boolean;
  /** Drawn as the app's chat bubble under "You"; else in a terminal's
   *  manner, after a ">". */
  bubble?: boolean;
  /** Why it still waits, when the person asked for more. */
  note?: string | null;
  /** Absent where the chat cannot stop a turn (a chat In Agentry). */
  onSendNow?: () => void;
  sendingNow?: boolean;
  onEdit: () => void;
  /** Present while the turn in flight may take a message. */
  onAddToTurn?: () => void;
  adding?: boolean;
}) {
  return (
    <div className={`queued-message${flush ? " flush" : ""}`} data-queued>
      {bubble ? (
        <div className="chat-msg user">
          <div className="chat-bubble">
            <div className="chat-who">You<span className="muted small"> · {QUEUED_WORDS}</span></div>
            <div className="queued-text">{text}</div>
          </div>
        </div>
      ) : (
        <div className="agent-term-person"><div className="queued-text"><span aria-hidden="true">&gt; </span>{text}</div></div>
      )}
      <div className="queued-actions small" role="group" aria-label="Queued message">
        {!bubble && <span className="muted">{QUEUED_WORDS}</span>}
        {onSendNow && (
          <Button variant="link" busy={sendingNow} busyLabel="Stopping the turn…" onClick={onSendNow}
            title="Stop the agent's turn and send this message now">
            Send now
          </Button>
        )}
        <Button variant="link" onClick={onEdit} title="Take this message back to change it before it is sent">Edit</Button>
        {onAddToTurn && (
          <Button variant="link" busy={adding} busyLabel="Adding…" onClick={onAddToTurn}
            title="Give this message to the agent now, inside the turn it is on">
            Add to this turn
          </Button>
        )}
      </div>
      {note && <p className="muted small queued-note">{note}</p>}
      <span className="sr-only" role="status">{QUEUED_SAID}</span>
    </div>
  );
}
