import { useCallback, useEffect, useState } from "react";
import * as MeetingAPI from "../../../wailsjs/go/app/MeetingAPI";
import type { app } from "../../../wailsjs/go/models";
import { EventsOn } from "../../../wailsjs/runtime/runtime";
import { CardFrame } from "../../cards";
import { Button, ErrorBox } from "../../ui";
import { canAskAssistant, deliveryWords, elapsed, isNo, meetingState, recordingWhat, windowStateWords, windowWhen } from "../../meetingNotes";

// How many meetings and armed windows the card lists.
const MEETING_ROWS = 3;
const WINDOW_ROWS = 3;

// Home's Meeting notes card: Record, or the recording under way with Stop;
// the windows the Assistant armed from the calendar, each with Cancel; and
// the last meetings, each with Ask to summarise while its transcript waits
// for the Assistant. Record asks in Go's native dialog, as the page's does.
// The card opens Meeting notes, where everything else is.
export function MeetingsCard({ onOpen }: { onOpen: () => void }) {
  const [view, setView] = useState<app.MeetingNotesView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [now, setNow] = useState(Date.now());
  const read = useCallback(() => {
    MeetingAPI.Status().then(setView).catch((e) => setError(String(e)));
  }, []);
  useEffect(() => {
    read();
    // The harness has no runtime.
    try {
      return EventsOn("meeting:changed", read);
    } catch {
      return undefined;
    }
  }, [read]);
  const recording = view?.recording;
  useEffect(() => {
    if (!recording) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [recording]);
  const act = async (f: () => Promise<app.MeetingNotesView>) => {
    setBusy(true);
    setError(null);
    try {
      setView(await f());
    } catch (e) {
      if (!isNo(e)) setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const status = !view
    ? error ? "Not read" : "Reading…"
    : !view.on
      ? "Off"
      : recording
        ? `${recording.paused ? "Paused" : "Recording"} ${recording.title ? `"${recording.title}"` : recordingWhat(recording.app, recording.wholeDevice, recording.inRoom)} · ${elapsed(recording.started, now)}`
        : view.offer
          ? `A call in ${view.offer.app || "a meeting app"} is offered on the Meeting notes page`
          : "Ready to record";
  const a = view?.assistant;
  const windows = (view?.windows ?? []).filter((w) => w.state !== "closed").slice(0, WINDOW_ROWS);
  const meetings = (view?.meetings ?? []).slice(0, MEETING_ROWS);
  return (
    <CardFrame title="Meeting notes" onOpen={onOpen} className="home-card home-meetings" status={status}
      foot={view?.on && a?.sendToAssistant ? `Transcripts are given to ${a.name}.` : undefined}>
      <ErrorBox error={error} />
      {view && !view.on && <p className="muted small">Turn it on on its page to record meetings and transcribe them on this computer.</p>}
      {view?.on && (
        <div className="row build-way-part">
          {recording ? (
            <Button variant="primary" data-home-meeting-stop disabled={busy} onClick={() => act(MeetingAPI.Stop)}>Stop and transcribe</Button>
          ) : (
            <>
              <Button variant="primary" data-home-meeting-record disabled={busy} onClick={() => act(MeetingAPI.Record)}>Record…</Button>
              {view.recordsRooms && (
                <Button data-home-meeting-record-room disabled={busy} onClick={() => act(MeetingAPI.RecordRoom)}>Record in the room…</Button>
              )}
            </>
          )}
        </div>
      )}
      {windows.length > 0 && a && (
        <>
          <p className="muted small home-meetings-title">Set up by {a.name}</p>
          <ul className="home-card-list" data-home-meeting-windows>
            {windows.map((w) => (
              <li key={w.id} data-home-meeting-window={w.state}>
                <span className="small">{w.title}</span>
                <div className="muted small">{windowWhen(w.opens, w.closes, new Date(now))} · {windowStateWords(w)}</div>
                {w.state !== "recording" && (
                  <button type="button" className="link small build-way-part" data-home-meeting-window-cancel disabled={busy}
                    onClick={() => void act(() => MeetingAPI.CancelWindow(w.id))}>Cancel</button>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
      {meetings.length > 0 && a && (
        <>
          <p className="muted small home-meetings-title">Last meetings</p>
          <ul className="home-card-list" data-home-meetings>
            {meetings.map((m) => (
              <li key={m.id}>
                <span className="small">{m.title}</span>
                <div className="muted small">{deliveryWords(m, a.name) || meetingState(m).words}</div>
                {canAskAssistant(m, a) && (
                  <button type="button" className="link small build-way-part" data-home-meeting-ask disabled={busy || !a.running}
                    title={a.running ? undefined : `Start ${a.name} first`}
                    onClick={() => void act(() => MeetingAPI.AskAssistant(m.id))}>Ask {a.name} to summarise</button>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
    </CardFrame>
  );
}
