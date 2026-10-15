import { main } from "../../../wailsjs/go/models";
import { CardFrame } from "../../cards";
import { fmtDuration } from "../../usage";

// Home's Run defaults card: where runs go, their time
// and turn limits, the output folder, and Unsafe Mode while it is on, as
// Settings → Run defaults keeps them. It edits nothing; the card opens that
// page.
export function RunDefaultsCard({ defaults, error, onOpen }: {
  defaults: main.RunDefaultsView | null;
  error: string | null;
  onOpen: () => void;
}) {
  return (
    <CardFrame
      title="Run defaults"
      onOpen={onOpen}
      className="home-card"
      status={error ? "Run defaults could not be read." : defaults ? null : "Reading run defaults…"}
    >
      {defaults && (
        <>
          <dl className="home-card-facts">
            <dt>Runs go</dt>
            <dd>{defaults.where === "agentry" ? "In Agentry, where it is offered" : "On this computer"}</dd>
            <dt>Time limit</dt>
            <dd>{fmtDuration(defaults.timeoutSeconds)}</dd>
            <dt>Turns</dt>
            <dd>{defaults.maxTurns}</dd>
            <dt>Output folder</dt>
            <dd className="mono">{defaults.outputDir || "~/Downloads/agentry-out"}</dd>
          </dl>
          {defaults.unsafeMode && <p className="small"><span className="badge warn">Unsafe Mode is on</span></p>}
        </>
      )}
    </CardFrame>
  );
}
