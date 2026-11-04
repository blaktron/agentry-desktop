import * as app from "../../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { CardFrame, CouldNotRead } from "../../cards";

// Home's Runhost card: the runhost runs go to, whether it answers, its
// model sign-ins and what is running there. The card opens Settings →
// Runhosts. Its reads are the ones that page makes: the list this computer
// keeps, and one status call to the runhost.
export function RunhostCard({ runhost, named, status, error, statusError, onRetry, onOpen }: {
  // The runhost Where to run names, or the default one; null while the list
  // is read, or when there is none.
  runhost: app.RunhostView | null;
  // The name Where to run gives, when it names one this computer lacks.
  named: string;
  status: app.RunhostStatusView | null;
  error: string | null;
  statusError: string | null;
  onRetry?: () => void;
  onOpen: () => void;
}) {
  const line = error
    ? <CouldNotRead what="Your runhosts" error={error} onRetry={onRetry} />
    : runhost
      ? statusError
        ? <CouldNotRead what={`${runhost.name}'s status`} error={statusError} onRetry={onRetry} />
        : status
          ? status.reachable
            ? `${runhost.name} answers${status.running ? `, ${status.running} running` : ""}${status.waiting ? `, ${status.waiting} waiting` : ""}`
            : `${runhost.name} does not answer${status.problem ? `: ${status.problem}` : ""}`
          : `Asking ${runhost.name}…`
      : named
        ? `Your runs go to ${named}, which is not set up on this computer.`
        : "No runhost is set up. Settings → Runhosts sets one up on a server of yours.";
  return (
    <CardFrame title="Runhost" onOpen={onOpen} className="home-card" status={line}>
      {runhost && (
        <dl className="home-card-facts">
          <dt>Server</dt>
          <dd className="mono">{runhost.host}{runhost.port && runhost.port !== 22 ? `:${runhost.port}` : ""}</dd>
          <dt>Models</dt>
          <dd>{runhost.signIns.length > 0 ? runhost.signIns.map((s) => s.vendor).join(", ") : "None signed in: each run brings an API key"}</dd>
          {runhost.cliVersion && (
            <>
              <dt>Agentry</dt>
              <dd className="mono">{runhost.cliVersion}</dd>
            </>
          )}
        </dl>
      )}
    </CardFrame>
  );
}
