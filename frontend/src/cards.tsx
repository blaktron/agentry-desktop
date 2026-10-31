// One card frame for every start page's cards and Home's: the Build, Voice and
// Knowledge pages' light-line card, the whole of it one button (the
// title's, stretched over the card by .build-way-main::after in style.css).
// A start page's card has a figure and a sentence; a Home card has a
// status line and a body of live content instead, and no figure. Anything
// pressable inside a card carries .build-way-part, which lifts it above the
// stretched button.
import { Children, type ReactNode } from "react";
import { causeOf } from "./errorText";

export function CardFrame({ title, onOpen, aside, figure, blurb, status, children, foot, className = "" }: {
  title: string;
  onOpen: () => void;
  // A control beside the title, above the card's stretched button.
  aside?: ReactNode;
  figure?: ReactNode;
  blurb?: ReactNode;
  // One line of what the card stands for now.
  status?: ReactNode;
  // The card's own content, below its title and status.
  children?: ReactNode;
  // A muted line at the card's foot.
  foot?: ReactNode;
  className?: string;
}) {
  return (
    <div className={`card build-way ${className}`.trim()}>
      {figure && <div className="build-way-figure">{figure}</div>}
      {aside ? (
        <div className="build-way-head">
          <h2 className="build-way-title">
            <button type="button" className="build-way-main" onClick={onOpen}>{title}</button>
          </h2>
          <div className="build-way-part build-way-aside">{aside}</div>
        </div>
      ) : (
        <h2 className="build-way-title">
          <button type="button" className="build-way-main" onClick={onOpen}>{title}</button>
        </h2>
      )}
      {blurb && <p className="build-way-blurb">{blurb}</p>}
      {status && <p className="small build-way-status" role="status">{status}</p>}
      {/* Children.toArray drops what renders nothing, so a card whose
          content is all conditional has no empty body. */}
      {Children.toArray(children).length > 0 && <div className="build-way-body">{children}</div>}
      {foot && <p className="muted small build-way-foot">{foot}</p>}
    </div>
  );
}

/** A card's status when its read failed: what could not be read, the
 *  cause in a sentence, and Try again where the card can read again. */
export function CouldNotRead({ what, error, onRetry }: { what: string; error: string; onRetry?: () => void }) {
  return (
    <span data-could-not-read>
      {what} could not be read: {causeOf(error)}
      {onRetry && <> <button type="button" className="link build-way-part" onClick={onRetry}>Try again</button></>}
    </span>
  );
}
