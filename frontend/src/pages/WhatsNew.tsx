import { useEffect, useState } from "react";
import { Button, Modal } from "../ui";
import { whatsNewKey, whatsNewStep, type WhatsNewData, type WhatsNewMode } from "../whatsNew";

// WhatsNew is the What's new screen: one card per version, newest first,
// with Back and Next between them and Dismiss on every card. Done, Dismiss
// and Escape call onClose, and the caller records the version as seen; a
// click beside the card does nothing, so a stray one cannot skip the cards.
// Each highlight is shown as plain text.
export default function WhatsNew({ data, mode, onClose, onOpenReleases }: {
  data: WhatsNewData;
  mode: WhatsNewMode;
  onClose: () => void;
  onOpenReleases: (url: string) => void;
}) {
  const [at, setAt] = useState(0);
  const step = whatsNewStep(data, at, mode);
  const total = data.cards.length;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const move = whatsNewKey(e.key, at, total);
      if (move === null) return;
      e.preventDefault();
      if (move === "close") onClose();
      else setAt(move.at);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [at, total, onClose]);

  if (!step) return null;
  return (
    <Modal title={step.title} onClose={() => {}}>
      <div className="whats-new" aria-live="polite">
        <p className="muted">{step.lead}</p>
        <ul className="whats-new-lines">
          {step.card.highlights.map((line, i) => <li key={i}>{line}</li>)}
        </ul>
        {step.earlier && (
          <p className="muted small">
            {step.earlier}{" "}
            <Button variant="link" onClick={() => onOpenReleases(data.releasesUrl)}>Every version's notes</Button>
          </p>
        )}
        <div className="row whats-new-foot">
          <Button variant="ghost" onClick={onClose}>Dismiss</Button>
          <span className="grow" />
          {step.counter && <span className="muted small" aria-label={`Card ${step.counter}`}>{step.counter}</span>}
          <Button disabled={step.backDisabled} onClick={() => setAt(at - 1)}>Back</Button>
          <Button variant="primary" autoFocus onClick={() => (step.last ? onClose() : setAt(at + 1))}>{step.nextLabel}</Button>
        </div>
      </div>
    </Modal>
  );
}
