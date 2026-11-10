// A card window: one of the app's questions or notices, shown as an Agentry
// card in a small window of its own. Go opens the window for the card,
// hands the card only to this window and takes the answer only from it, so
// the main page can neither read nor answer it. Closing the window or
// Escape answers the card's cancel, which also has the focus when the card
// opens, so Enter never accepts. The other answers take a click only once
// the card has been up for ARM_MS, so a card that opens under the pointer
// is not answered by a click meant for the window beneath it.
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import * as CardWindowAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/cardwindowapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { Button, Mark } from "../ui";

const ARM_MS = 600;

export function CardWindow() {
  const [card, setCard] = useState<app.CardView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);
  const [armed, setArmed] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const message = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    CardWindowAPI.Card()
      .then((c) => setCard(c ?? null))
      .catch((e) => setError(String(e)));
  }, []);

  useEffect(() => {
    if (!card) return;
    const t = window.setTimeout(() => setArmed(true), ARM_MS);
    return () => window.clearTimeout(t);
  }, [card]);

  // The window fits the card: as tall as its parts need, the message at its
  // full length. Go caps the height (the message scrolls beyond it) and
  // keeps the window centred over the app's own window.
  useLayoutEffect(() => {
    const el = box.current;
    const msg = message.current;
    if (!el || !card) return;
    CardWindowAPI.FitCard(Math.ceil(naturalHeight(el, msg))).catch(() => {});
  }, [card, error]);

  function answer(i: number) {
    if (!card || sent || (!armed && i !== card.cancel)) return;
    setSent(true);
    CardWindowAPI.AnswerCard(card.id, i).catch((e) => {
      setSent(false);
      setError(String(e));
    });
  }

  useEffect(() => {
    if (!card) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        answer(card.cancel);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  return (
    <div
      ref={box}
      className={`card-window card-window-${card?.kind ?? "question"}`}
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="card-window-title"
      aria-describedby="card-window-message"
      data-card-window
    >
      <header className="card-window-head">
        <Mark size={20} />
        <h1 id="card-window-title">{card?.title ?? "Agentry Desktop"}</h1>
      </header>
      {card && (
        <p ref={message} id="card-window-message" className="card-window-message selectable">
          {card.message}
        </p>
      )}
      {error && <p className="card-window-error small" role="alert">{error}</p>}
      {card && (
        <div className="card-window-actions">
          {card.buttons.map((b, i) => (
            <Button
              key={i}
              type="button"
              variant={b.tone === "primary" ? "primary" : "default"}
              autoFocus={i === card.cancel}
              disabled={sent || (!armed && i !== card.cancel)}
              onClick={() => answer(i)}
            >
              {b.label}
            </Button>
          ))}
        </div>
      )}
    </div>
  );
}

/** The card's height with every part at its own size: the message at its
 *  full length however much of it shows. */
function naturalHeight(el: HTMLElement, msg: HTMLElement | null): number {
  const cs = getComputedStyle(el);
  const px = (v: string) => parseFloat(v) || 0;
  const kids = Array.from(el.children) as HTMLElement[];
  let h = px(cs.paddingTop) + px(cs.paddingBottom) + px(cs.borderTopWidth) + px(cs.borderBottomWidth);
  h += px(cs.rowGap) * Math.max(kids.length - 1, 0);
  for (const k of kids) h += k === msg ? k.scrollHeight : k.offsetHeight;
  return h;
}
