// The Voice Writer's run, on this computer or In Agentry, followed above the
// pages in its run window (RunWindow.tsx), as a QuickAgent's run is: App
// mounts this, and Edit with AI on the Local or the Cloud Voice page hands
// the run over once it has begun. On this computer the window shows the time
// used against the Voice Writer's limit, its turns and tokens, and its Run
// log, and Stop stops only the Voice Writer (VoiceAPI.StopWrite); In Agentry
// it shows where the run stands in the registry's words, and Cancel run asks
// the registry. Once the run is over the window says what became of the
// voice, and Open your voice opens the page that shows it.
//
// Every string the run, a page or the registry wrote is drawn as text.
import type * as voice from "../../bindings/github.com/blaktron/agentry-desktop/internal/voice/models";
import type { Tone } from "../jobProgress";
import { ProgressView } from "../progress";
import { RevealText } from "../reveal";
import { logLines } from "../runLog";
import { VOICE_ON, hostedFailure } from "../voice";
import { cancelVoiceRun, dismissVoiceRun, showVoiceRun, stopVoiceRun, useVoiceRun } from "../voiceRun";
import { RunWindow, useFollowed } from "./RunWindow";

export default function VoiceFlow({ onOpenVoice, initialMinimized }: {
  /** Opens the Voice page that shows what the run wrote. */
  onOpenVoice: (page: "local" | "cloud") => void;
  /** Harness-only: open as the corner card. */
  initialMinimized?: boolean;
}) {
  const s = useVoiceRun();
  const followed = useFollowed({ raised: s.raised, asCard: s.asCard, initialMinimized });
  const j = s.job;
  if (!s.shown || !j) return null;

  const here = j.where === "here";
  const h = j.hosted;
  const running = !j.over;
  const { hasVoice, samples, urls } = j.meta;
  const reads = here
    ? `It reads ${hasVoice ? "your voice and what should change" : "your samples"}${hasVoice && samples > 0 ? ", your samples" : ""}${urls > 0 ? ", and the pages at your web addresses once AI Review has read them" : ""}, then writes VOICE.md.`
    : "";

  let line = "";
  let tone: Tone = "";
  let made = false;
  if (!running && here) {
    made = !!j.result;
    if (j.result) {
      line = "The Voice Writer kept your new voice on this computer.";
      tone = "ok";
    } else if (j.stopped) {
      line = "You stopped the Voice Writer. Your voice on this computer is as it was.";
      tone = "warn";
    } else {
      line = "The Voice Writer did not finish. Your voice on this computer is as it was.";
      tone = "bad";
    }
  } else if (!running && h) {
    made = h.applied;
    line = h.applied ? h.outcomeLine || "The Voice Writer wrote a new version of your voice. AI Review reads it before it is live." : hostedFailure(h);
    tone = h.applied ? "ok" : h.status === "cancelled" ? "warn" : "bad";
  } else if (running && !here && h) {
    line = h.cancelRequested ? "Cancel asked for." : "";
  }

  return (
    <RunWindow
      {...followed}
      onOpen={showVoiceRun}
      onDismiss={dismissVoiceRun}
      label={here ? "The Voice Writer on this computer" : "The Voice Writer In Agentry"}
      title={hasVoice ? "Edit with AI" : "Write with AI"}
      cardTitle={running ? `The Voice Writer is running${here ? "" : " In Agentry"}` : "The Voice Writer: the run is over"}
      running={running}
      spinner={here
        ? (j.stopping ? "Stopping the Voice Writer…" : "The Voice Writer is running on this computer…")
        : h?.cancelRequested ? "Cancel asked for: Agentry is stopping the run…" : h?.statusLine || h?.state || "The Voice Writer is running In Agentry…"}
      about={here
        ? <>{reads} When it ends, the voice here is replaced with what it wrote.</>
        : <>When it ends, what it wrote becomes your account&apos;s new version and waits for AI Review; your live voice stays as it is until it passes.</>}
      status={!here && h?.pagesLine && <p className="muted small">{h.pagesLine}</p>}
      progress={here ? (compact) => (
        <ProgressView progress={j.progress} job="run" compact={compact} fact={(p) => (p.turns === 1 ? "1 turn" : `${p.turns} turns`)} />
      ) : undefined}
      background={here ? <>
        In the background, the Voice Writer goes on in a card in the corner and the rest of the app stays usable. Stop ends
        it, and your voice stays as it was.
      </> : <>
        In the background, the Voice Writer goes on In Agentry and this app follows it in a card in the corner. Cancel run
        asks Agentry to stop it and settle what it used.
      </>}
      stop={here
        ? { label: "Stop the Voice Writer", cardLabel: "Stop", stopping: j.stopping, onStop: () => void stopVoiceRun() }
        : h && !h.cancelRequested ? { label: "Cancel run", stopping: false, onStop: () => void cancelVoiceRun() } : null}
      log={here ? logLines(j.log.lines, j.log.dropped) : null}
      line={line}
      tone={tone}
      error={here ? j.error : null}
      outcome={j.result ? <VoiceWritten r={j.result} line={line} /> : undefined}
      primary={made ? { label: "Open your voice", onClick: () => { dismissVoiceRun(); onOpenVoice(here ? "local" : "cloud"); } } : null}
    />
  );
}

// VoiceWritten is what a run on this computer did: the voice kept, and what
// became of each sample and web address.
function VoiceWritten({ r, line }: { r: voice.BuildResult; line: string }) {
  const pages = r.pages ?? [];
  const skipped = r.skipped ?? [];
  return (
    <>
      <h3 className="run-outcome run-outcome-ok"><RevealText text={line} /></h3>
      {r.created && <p>{VOICE_ON}</p>}
      {pages.length > 0 && (
        <ul className="small">
          {pages.map((p) => (
            <li key={p.url}>
              <span className="mono selectable"><RevealText text={p.url} /></span>:{" "}
              {p.withheld ? <>withheld by AI Review (<RevealText text={p.withheld} />)</> : p.rendered ? "read, rendered first" : "read"}
            </li>
          ))}
        </ul>
      )}
      {skipped.length > 0 && (
        <p className="muted small">Not read: <span className="mono selectable"><RevealText text={skipped.join(", ")} /></span></p>
      )}
    </>
  );
}
