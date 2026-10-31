// Review Your QuickAgents locally, the action: offered on the
// Build tab's summary beside Submit and on Validate for a folder. It never
// runs by itself, and holds nothing else on the page: Submit, Save, Install
// and Validate are the same with or without it. A click goes to Go, which
// asks in a native dialog first; the review is then followed in its own
// dialog and card (ReviewFlow). Without the person's own TypeSafe key it
// says to add one and opens Provider settings at the key, and runs nothing.
import { useEffect, useState } from "react";
import * as BuildAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/buildapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { Button } from "../ui";

export function ReviewAction({ onReview, busy, disabled, onOpenSettings, what = "it" }: {
  onReview: () => void;
  /** The review is waiting for its dialog or for Go to begin it. */
  busy: boolean;
  disabled?: boolean;
  /** Provider settings, at the AI Review key. */
  onOpenSettings?: () => void;
  /** What the sentence calls the QuickAgent. */
  what?: string;
}) {
  const [status, setStatus] = useState<app.ReviewStatusView | null>(null);
  useEffect(() => {
    BuildAPI.ReviewStatus().then(setStatus).catch(() => setStatus(null));
  }, []);

  if (status && !status.key.ready) {
    return (
      <div className="small review-action" role="note">
        <p>
          <strong>Review Your QuickAgents locally</strong> runs the registry&apos;s rules and AI Review over {what} on this computer
          before you submit, and AI Review needs your own TypeSafe key, which this computer does not have yet.
        </p>
        {onOpenSettings && <Button onClick={onOpenSettings}>Add your AI Review key</Button>}
      </div>
    );
  }
  return (
    <div className="review-action">
      <Button disabled={disabled || busy} onClick={onReview}>{busy ? "Starting the review…" : "Review Your QuickAgents locally"}</Button>
      <p className="muted small">
        See what Agentry&apos;s pipeline would be expected to say before you submit: the Examiner runs the registry&apos;s rules
        here, asks AI Review with your key and reads the files with your model ({status?.modelName ?? "your runner policy's"}), for no
        credits. It asks first, submits nothing, and the registry scans again at Submit.
      </p>
    </div>
  );
}
