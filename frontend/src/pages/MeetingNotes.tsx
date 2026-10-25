// Meeting notes: record a meeting by hand, transcribe it on this computer,
// and keep the transcript in a folder. Turning it on, and every Record, is
// confirmed in Go's native dialog: this page asking is not the person
// deciding. While it is off nothing records. The page names a meeting by
// its id and never a path, and the output folder comes from the native
// picker alone.
import { useCallback, useEffect, useState } from "react";
import * as MeetingAPI from "../../wailsjs/go/app/MeetingAPI";
import type { app } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { Button, Card, ErrorBox, Field, Notice, PageHeader, Select, Spinner } from "../ui";
import { CHAT_LINE, canTranscribe, downloadWords, duration, elapsed, isNo, meetingState, recordingWhat } from "../meetingNotes";

type View = app.MeetingNotesView;

export default function MeetingNotes() {
  const [view, setView] = useState<View | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [now, setNow] = useState(Date.now());
  const [copied, setCopied] = useState(false);

  const read = useCallback(() => {
    MeetingAPI.Status().then(setView).catch((e) => setError(String(e)));
  }, []);
  useEffect(() => {
    read();
    try {
      return EventsOn("meeting:changed", read);
    } catch {
      return undefined;
    }
  }, [read]);
  // The recording's clock.
  const recording = view?.recording;
  useEffect(() => {
    if (!recording) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [recording]);

  const act = async (f: () => Promise<View | void>) => {
    setBusy(true);
    setError(null);
    try {
      const v = await f();
      if (v) setView(v);
      else read();
    } catch (e) {
      if (!isNo(e)) setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  if (!view) {
    return (
      <div className="meeting-notes">
        <PageHeader title="Meeting notes" />
        {error ? <ErrorBox error={error} /> : <Spinner label="Reading Meeting notes" />}
      </div>
    );
  }

  const header = (
    <PageHeader
      title="Meeting notes"
      sub="Record a meeting, transcribe it on this computer, and keep the transcript in a folder. No audio or transcript leaves this computer."
    />
  );

  if (!view.available) {
    return (
      <div className="meeting-notes">
        {header}
        <Notice tone="warn">{view.unavailable}</Notice>
      </div>
    );
  }

  if (!view.on) {
    return (
      <div className="meeting-notes">
        {header}
        <ErrorBox error={error} />
        <Card>
          <h3>Before you turn it on</h3>
          <ul className="meeting-warning" data-meeting-warning>
            {view.warnings.map((w) => <li key={w}>{w}</li>)}
          </ul>
          <div className="row">
            <Button variant="primary" data-meeting-turn-on disabled={busy} onClick={() => act(MeetingAPI.TurnOn)}>Turn on Meeting notes…</Button>
          </div>
        </Card>
      </div>
    );
  }

  const prepareLine = view.preparing
    ? `${view.prepareStep || "Getting ready"}${view.prepareTotal ? `: ${downloadWords(view.prepareDone ?? 0, view.prepareTotal)}` : ""}`
    : null;

  return (
    <div className="meeting-notes">
      {header}
      <ErrorBox error={error} />

      <Card className="meeting-recorder">
        {recording ? (
          <>
            <div className="meeting-live" data-meeting-recording>
              <span className={`badge ${recording.paused ? "" : "warn"}`}>{recording.paused ? "Paused" : "Recording"}</span>
              <strong className="meeting-clock">{elapsed(recording.started, now)}</strong>
              <span>Recording {recordingWhat(recording.app, recording.wholeDevice)}.</span>
            </div>
            {recording.wholeDevice && (
              <p className="small muted">
                No meeting app was found to record on its own, so every sound your computer plays is in this recording, and the
                transcript says so.
              </p>
            )}
            <div className="row">
              {recording.paused
                ? <Button data-meeting-resume disabled={busy} onClick={() => act(MeetingAPI.Resume)}>Resume</Button>
                : <Button data-meeting-pause disabled={busy} onClick={() => act(MeetingAPI.Pause)}>Pause</Button>}
              <Button variant="primary" data-meeting-stop disabled={busy} onClick={() => act(MeetingAPI.Stop)}>Stop and transcribe</Button>
            </div>
          </>
        ) : (
          <>
            <p>
              Press Record when your meeting has started. Agentry records the meeting app's sound and your microphone as two
              tracks, and stops when you press Stop or the meeting app stops.
            </p>
            <div className="row">
              <Button variant="primary" data-meeting-record disabled={busy} onClick={() => act(MeetingAPI.Record)}>Record…</Button>
            </div>
          </>
        )}
        <div className="meeting-chat-line">
          <span className="small muted">To tell the others, paste this into the meeting's chat:</span>
          <code>{CHAT_LINE}</code>
          <Button
            variant="ghost"
            onClick={() => {
              navigator.clipboard?.writeText(CHAT_LINE).then(() => setCopied(true)).catch(() => setCopied(false));
            }}
          >
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
      </Card>

      <Card className="meeting-model">
        {prepareLine ? (
          <div data-meeting-preparing><Spinner label={prepareLine} /></div>
        ) : view.prepareError ? (
          <Notice tone="warn">
            The speech model is not ready: {view.prepareError}
            <div className="row">
              <Button onClick={() => act(MeetingAPI.Prepare)} disabled={busy}>Try again</Button>
            </div>
          </Notice>
        ) : view.modelReady ? (
          <p className="small muted" data-meeting-model>Transcribing on this computer with {view.modelName}.</p>
        ) : (
          <p className="small muted">The {view.modelName} model is downloaded when a meeting is first transcribed.</p>
        )}
      </Card>

      <h3>Meetings</h3>
      {view.meetings.length === 0 ? (
        <p className="muted">No meetings recorded yet.</p>
      ) : (
        <div className="meeting-list" data-meeting-list>
          {view.meetings.map((m) => {
            const st = meetingState(m);
            const working = m.state === "recording" || m.state === "transcribing";
            return (
              <Card key={m.id} className="meeting-row">
                <div className="meeting-row-head">
                  <strong>{m.title}</strong>
                  <span className="small muted">
                    {[m.app || (m.wholeDevice ? "Whole device" : ""), m.durationSeconds ? duration(m.durationSeconds) : ""].filter(Boolean).join(" · ")}
                  </span>
                </div>
                <div className={`small meeting-state tone-${st.tone}`} data-meeting-state={m.state}>{st.words}</div>
                {m.transcriptName && <div className="small muted">{m.transcriptName}</div>}
                {m.hasAudio && m.audioUntil && !working && (
                  <div className="small muted">The recording is kept until {new Date(m.audioUntil).toLocaleDateString()}.</div>
                )}
                <div className="row">
                  {m.hasTranscript && <Button variant="ghost" onClick={() => act(() => MeetingAPI.OpenTranscript(m.id))}>Open transcript</Button>}
                  {m.hasTranscript && <Button variant="ghost" onClick={() => act(() => MeetingAPI.ShowTranscript(m.id))}>Show in folder</Button>}
                  {canTranscribe(m) && <Button variant="ghost" disabled={busy} onClick={() => act(() => MeetingAPI.Transcribe(m.id))}>Transcribe</Button>}
                  {m.hasAudio && !working && <Button variant="ghost" disabled={busy} onClick={() => act(() => MeetingAPI.DeleteAudio(m.id))}>Delete recording</Button>}
                  {!working && <Button variant="ghost" disabled={busy} onClick={() => act(() => MeetingAPI.Forget(m.id))}>Remove from list</Button>}
                </div>
              </Card>
            );
          })}
        </div>
      )}

      <h3>Settings</h3>
      <Card className="meeting-settings">
        <Field label="Language" htmlFor="meeting-profile" hint="English is more accurate for English; Multilingual detects the language spoken.">
          <Select id="meeting-profile" value={view.profile} disabled={busy} onChange={(e) => act(() => MeetingAPI.SetProfile(e.target.value))}>
            {view.profiles.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </Select>
        </Field>
        <Field label="Model" htmlFor="meeting-model" hint="Automatic measures this computer and picks the most accurate model that keeps up.">
          <Select id="meeting-model" value={view.modelChoice} disabled={busy} onChange={(e) => act(() => MeetingAPI.SetModel(e.target.value))}>
            <option value="">Automatic ({view.modelName})</option>
            {view.models.map((m) => <option key={m.id} value={m.id}>{m.name} ({Math.round((m.sizeBytes ?? 0) / 1_000_000)} MB)</option>)}
          </Select>
        </Field>
        <Field label="Transcripts folder">
          <div className="row">
            <code className="nowrap" title={view.outputDir}>{view.outputDir}</code>
          </div>
          <div className="row">
            <Button variant="ghost" disabled={busy} onClick={() => act(MeetingAPI.ChooseOutputDir)}>Choose…</Button>
            {view.outputDirChosen && <Button variant="ghost" disabled={busy} onClick={() => act(MeetingAPI.ResetOutputDir)}>Use the default</Button>}
            <Button variant="ghost" onClick={() => act(MeetingAPI.OpenOutputFolder)}>Open folder</Button>
          </div>
        </Field>
        <label className="check-row">
          <input type="checkbox" role="switch" data-meeting-keep checked={view.keepAudio} disabled={busy} onChange={(e) => act(() => MeetingAPI.SetKeepAudio(e.target.checked))} />
          Keep recordings for 7 days
        </label>
        <p className="small muted">
          Off, each recording is deleted once its transcript is written. On, it stays in Agentry's own folder for seven days,
          never in the transcripts folder.
        </p>
        <div className="row">
          <Button variant="ghost" data-meeting-turn-off disabled={busy} onClick={() => act(MeetingAPI.TurnOff)}>Turn off Meeting notes</Button>
        </div>
      </Card>
    </div>
  );
}
