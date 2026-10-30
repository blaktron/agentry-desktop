// Meeting notes: record a meeting, by hand or on the offer of a call the
// app noticed (at once for an app the person put on the automatic list),
// or a meeting held in the room through the microphone alone (by hand
// only), transcribe it on this computer, and keep the transcript in a
// folder. Turning it on, every Record from this page and every app put on the
// automatic list are confirmed in Go's native dialog: this page asking is
// not the person deciding. While it is off nothing records. The page names
// a meeting by its id and never a path, and the output folder comes from
// the native picker alone. Giving transcripts to the Assistant is turned on
// in Go's native dialog too; the windows the Assistant armed are listed
// with Cancel.
import { useCallback, useEffect, useRef, useState } from "react";
import * as MeetingAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/meetingapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { EventsOn } from "../host";
import { Button, Card, ErrorBox, Field, Notice, PageHeader, Select, Spinner } from "../ui";
import {
  AUTO_CHOICES, CHAT_LINE, DEFAULT_MIC, canAskAssistant, canTranscribe, capitalised, deliveryWords, downloadWords, duration, elapsed, gpuHint,
  gpuOfferTitle, hoursLabel, isNo, meetingSource, meetingState, micLostText, offerTitle, parsePeople, possessive, recordingWhat, windowStateWords,
  windowWhen,
} from "../meetingNotes";

type View = app.MeetingNotesView;

export default function MeetingNotes() {
  const [view, setView] = useState<View | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [now, setNow] = useState(Date.now());
  const [copied, setCopied] = useState(false);
  // The Record in the room form: open, and its People in the room box.
  const [roomForm, setRoomForm] = useState(false);
  const [people, setPeople] = useState("");
  // justOn: turned on in this visit, when starting at sign-in is offered.
  const [justOn, setJustOn] = useState(false);
  // A notification's click opens this page at its top, where an offer or
  // the recording is, however far down it was left.
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    try {
      return EventsOn("meeting:open", () => root.current?.closest(".content")?.scrollTo({ top: 0 }));
    } catch {
      return undefined;
    }
  }, []);

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

  const assistant = view.assistant;
  const header = (
    <PageHeader
      title="Meeting notes"
      sub={assistant.sendToAssistant
        ? `Record a meeting, transcribe it on this computer, and keep the transcript in a folder. No audio leaves this computer; each transcript is given to ${assistant.name}, whose model provider reads it.`
        : "Record a meeting, transcribe it on this computer, and keep the transcript in a folder. No audio or transcript leaves this computer."}
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
            <Button
              variant="primary"
              data-meeting-turn-on
              disabled={busy}
              onClick={() =>
                act(async () => {
                  const v = await MeetingAPI.TurnOn();
                  setJustOn(true);
                  return v;
                })
              }
            >
              Turn on Meeting notes…
            </Button>
          </div>
        </Card>
      </div>
    );
  }

  const offer = view.offer;
  const prepareLine = view.preparing
    ? `${view.prepareStep || "Getting ready"}${view.prepareTotal ? `: ${downloadWords(view.prepareDone ?? 0, view.prepareTotal)}` : ""}`
    : null;

  return (
    <div className="meeting-notes" ref={root}>
      {header}
      <ErrorBox error={error} />

      {justOn && view.detects && !view.startsAtSignIn && (
        <Card className="meeting-signin" data-meeting-signin-offer>
          <p>
            Agentry notices calls only while it runs. Closing its window keeps it running in the background; to notice calls
            after you sign in to this computer, start Agentry when you sign in.
          </p>
          <div className="row">
            <Button variant="primary" disabled={busy} onClick={() => act(async () => { const v = await MeetingAPI.SetStartsAtSignIn(true); setJustOn(false); return v; })}>
              Start Agentry when I sign in
            </Button>
            <Button variant="ghost" onClick={() => setJustOn(false)}>Not now</Button>
          </div>
        </Card>
      )}

      {offer && !recording && (
        <Card className="meeting-offer" data-meeting-offer>
          <h3>{offerTitle(offer.app)}</h3>
          <p>
            Agentry noticed a call in {offer.app || "a meeting app"}. Record it to transcribe it on this computer. Recording others may
            need their consent; tell them you are recording.
          </p>
          <div className="row">
            <Button variant="primary" data-meeting-offer-record disabled={busy} onClick={() => act(() => MeetingAPI.RecordOffer(offer.id))}>Record…</Button>
            <Button variant="ghost" data-meeting-offer-not-now disabled={busy} onClick={() => act(() => MeetingAPI.NotNow(offer.id))}>Not now</Button>
          </div>
          <p className="small muted">The offer lapses at {new Date(offer.expires).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} and nothing is recorded.</p>
        </Card>
      )}

      {view.windows.length > 0 && (
        <Card className="meeting-windows" data-meeting-windows>
          <h3>Set up by {assistant.name}</h3>
          <p className="small muted">
            {capitalised(assistant.name)} set these up from your calendar. A call Agentry notices while one is open is recorded at once, and shown
            as it records; nothing is recorded if no call is noticed.
          </p>
          <ul className="meeting-window-list">
            {view.windows.map((w) => (
              <li key={w.id} className="meeting-window" data-meeting-window={w.state}>
                <div>
                  <strong>{w.title}</strong>
                  <div className="small muted">{windowWhen(w.opens, w.closes, new Date(now))}{w.app ? ` · ${w.app}` : ""}{w.invitees.length ? ` · ${w.invitees.length} invited` : ""}</div>
                  <div className="small">{windowStateWords(w)}</div>
                </div>
                {(w.state === "waiting" || w.state === "open") && (
                  <Button variant="ghost" data-meeting-window-cancel disabled={busy} onClick={() => act(() => MeetingAPI.CancelWindow(w.id))}>Cancel</Button>
                )}
              </li>
            ))}
          </ul>
        </Card>
      )}

      <Card className="meeting-recorder">
        {recording ? (
          <>
            <div className="meeting-live" data-meeting-recording>
              <span className={`badge ${recording.paused ? "" : "warn"}`}>{recording.paused ? "Paused" : "Recording"}</span>
              <strong className="meeting-clock">{elapsed(recording.started, now)}</strong>
              <span>Recording {recordingWhat(recording.app, recording.wholeDevice, recording.inRoom)}.</span>
            </div>
            {recording.inRoom && (
              <p className="small muted" data-meeting-in-room>
                A meeting in the room{recording.microphone ? `, through ${recording.microphone}` : ""}. Nothing your computer plays is recorded.
                If nobody speaks for 15 minutes, Agentry asks whether the meeting is still on.
              </p>
            )}
            {recording.micLost && (
              <Notice tone="warn">
                <span data-meeting-mic-lost>{micLostText(recording.micLost, recording.micNone, recording.microphone)}</span>
              </Notice>
            )}
            {recording.micMissing && (
              <Notice tone="warn">
                <span data-meeting-mic-missing>The microphone chosen in the settings was not found, so the default microphone is recorded.</span>
              </Notice>
            )}
            {recording.armed && (
              <p className="small muted" data-meeting-armed>
                It started by itself: {recording.title ? `"${recording.title}" is` : "this meeting is"} one {assistant.name} set up to record from your calendar.
              </p>
            )}
            {recording.automatic && (
              <p className="small muted" data-meeting-automatic>
                It started by itself: {recording.app || "this app"} is on your list of apps recorded automatically.
              </p>
            )}
            {recording.wholeDevice && (
              <p className="small muted">
                No meeting app was found to record on its own, so every sound your computer plays is in this recording, and the
                transcript says so.
              </p>
            )}
            {recording.warning && (
              <Notice tone="warn">
                <span data-meeting-warning>{recording.warning}</span>
                {recording.warningSettings && (
                  <>
                    {" "}
                    <Button data-meeting-privacy onClick={() => act(() => MeetingAPI.OpenPrivacySettings(recording.warningSettings || ""))}>
                      Open System Settings
                    </Button>
                  </>
                )}
              </Notice>
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
              {view.detects
                ? "When a meeting app starts a call, Agentry offers to record it. You can also press Record once your meeting has started. "
                : "Press Record when your meeting has started. "}
              Agentry records the meeting app's sound and your microphone as two tracks, and stops when you press Stop or the meeting
              app lets go of the microphone.
            </p>
            <div className="row">
              <Button variant="primary" data-meeting-record disabled={busy} onClick={() => act(MeetingAPI.Record)}>Record…</Button>
            </div>
            {view.recordsRooms && (
              <>
                <p data-meeting-room-hint>
                  For a meeting held here in the room, press Record in the room: Agentry records your microphone alone, and nothing your
                  computer plays, until you press Stop or it reaches its longest length.
                </p>
                {!roomForm && (
                  <div className="row">
                    <Button
                      data-meeting-record-room
                      disabled={busy}
                      onClick={() => (view.separatesSpeakers && view.tellSpeakersApart ? setRoomForm(true) : act(() => MeetingAPI.RecordRoom(0)))}
                    >
                      Record in the room…
                    </Button>
                  </div>
                )}
                {roomForm && (() => {
                  const parsed = parsePeople(people, view.minPeople, view.maxPeople);
                  return (
                    <div className="meeting-room-form" data-meeting-room-form>
                      <Field
                        label="People in the room (optional)"
                        htmlFor="meeting-people"
                        hint={parsed.error ?? "Agentry tells the speakers apart after the meeting. Say how many people are here, and it splits the voices into that many; leave it empty, and it estimates, so one person may appear under two numbers."}
                      >
                        <input id="meeting-people" data-meeting-people inputMode="numeric" value={people} disabled={busy}
                          onChange={(e) => setPeople(e.target.value)} aria-invalid={parsed.error ? true : undefined} />
                      </Field>
                      <div className="row">
                        <Button variant="primary" data-meeting-room-start disabled={busy || !!parsed.error}
                          onClick={() => act(async () => {
                            const v = await MeetingAPI.RecordRoom(parsed.people);
                            setRoomForm(false);
                            setPeople("");
                            return v;
                          })}>
                          Start…
                        </Button>
                        <Button variant="ghost" data-meeting-room-cancel disabled={busy} onClick={() => { setRoomForm(false); setPeople(""); }}>Cancel</Button>
                      </div>
                    </div>
                  );
                })()}
              </>
            )}
          </>
        )}
        {!recording?.inRoom && (
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
        )}
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
          <p className="small muted" data-meeting-model>
            Transcribing on this computer with {view.modelName}{view.gpu?.on ? ", on the graphics card" : ""}.
          </p>
        ) : (
          <p className="small muted">The {view.modelName} model is downloaded when a meeting is first transcribed.</p>
        )}
        {view.gpu?.offer && !view.preparing && (
          <div className="meeting-gpu-offer" data-meeting-gpu-offer>
            <h3>{gpuOfferTitle(view.gpu.sizeBytes)}</h3>
            <p>
              Agentry measured your graphics card, {view.gpu.name}: it can transcribe with {view.gpu.modelName}, a more accurate model,
              as fast as a meeting is spoken. The model is downloaded once. You can change this in Settings.
            </p>
            <div className="row">
              <Button variant="primary" data-meeting-gpu-use disabled={busy} onClick={() => act(() => MeetingAPI.UseGPU(true))}>Use the graphics card</Button>
              <Button variant="ghost" data-meeting-gpu-decline disabled={busy} onClick={() => act(() => MeetingAPI.UseGPU(false))}>Keep the processor</Button>
            </div>
          </div>
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
                    {[meetingSource(m), m.durationSeconds ? duration(m.durationSeconds) : ""].filter(Boolean).join(" · ")}
                  </span>
                </div>
                <div className={`small meeting-state tone-${st.tone}`} data-meeting-state={m.state}>{st.words}</div>
                {m.transcriptName && <div className="small muted">{m.transcriptName}</div>}
                {m.fromCalendar && <div className="small muted">Recorded in a window {assistant.name} set up from your calendar</div>}
                {m.delivery && <div className={`small${m.delivery === "failed" ? " tone-warn" : " muted"}`} data-meeting-delivery={m.delivery}>{deliveryWords(m, assistant.name)}</div>}
                {m.hasAudio && m.audioUntil && !working && (
                  <div className="small muted">The recording is kept until {new Date(m.audioUntil).toLocaleDateString()}.</div>
                )}
                <div className="row">
                  {m.hasTranscript && <Button variant="ghost" onClick={() => act(() => MeetingAPI.OpenTranscript(m.id))}>Open transcript</Button>}
                  {m.hasTranscript && <Button variant="ghost" onClick={() => act(() => MeetingAPI.ShowTranscript(m.id))}>Show in folder</Button>}
                  {canAskAssistant(m, assistant) && (
                    <Button variant="ghost" data-meeting-ask disabled={busy || !assistant.running}
                      title={assistant.running ? undefined : `Start ${assistant.name} first`}
                      onClick={() => act(() => MeetingAPI.AskAssistant(m.id))}>
                      Ask {assistant.name} to summarise
                    </Button>
                  )}
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
        {view.checksGPU && (
          <Field label="Transcribe on" htmlFor="meeting-backend" hint={view.gpu ? gpuHint(view.gpu) : undefined}>
            {view.gpu ? (
              <Select id="meeting-backend" data-meeting-backend value={view.gpu.on ? "gpu" : "cpu"} disabled={busy}
                onChange={(e) => act(() => MeetingAPI.UseGPU(e.target.value === "gpu"))}>
                <option value="cpu">The processor</option>
                <option value="gpu">The graphics card ({view.gpu.name})</option>
              </Select>
            ) : (
              <p className="small muted" data-meeting-backend-none>
                {view.gpuChecked
                  ? "The processor: no graphics card here ran a transcription when Agentry measured it."
                  : "The processor. Agentry measures your graphics card once the speech model is ready."}
              </p>
            )}
            {view.gpuChecked && (
              <div className="row">
                <Button variant="ghost" data-meeting-gpu-check disabled={busy || view.preparing} onClick={() => act(MeetingAPI.CheckGPU)}>
                  Measure the graphics card again
                </Button>
              </div>
            )}
          </Field>
        )}
        {view.recordsRooms && (
          <Field label="Microphone" htmlFor="meeting-mic"
            hint={view.micHint}>
            <Select id="meeting-mic" data-meeting-mic value={view.mic} disabled={busy} onChange={(e) => act(() => MeetingAPI.SetMic(e.target.value))}>
              <option value="">{DEFAULT_MIC}</option>
              {view.mic && !view.microphones.some((m) => m.id === view.mic) && <option value={view.mic}>Not connected now</option>}
              {view.microphones.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
            </Select>
          </Field>
        )}
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
        {view.detects && (
          <Field label="Meeting apps" hint="An app's calls are offered for recording unless you choose to record them automatically. A browser's calls come from any site.">
            {view.apps.length === 0 ? (
              <p className="small muted" data-meeting-apps-empty>Apps appear here once Agentry has noticed a call in them.</p>
            ) : (
              <div className="meeting-apps" data-meeting-apps>
                {view.apps.map((a) => (
                  <div className="row meeting-app" key={a.key}>
                    <span>{a.name}</span>
                    <Select
                      aria-label={`${a.name}'s calls`}
                      value={a.automatic ? "auto" : "ask"}
                      disabled={busy}
                      onChange={(e) => act(() => MeetingAPI.SetAutomatic(a.key, e.target.value === "auto"))}
                    >
                      {AUTO_CHOICES.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}
                    </Select>
                  </div>
                ))}
              </div>
            )}
          </Field>
        )}
        <Field label="Stop a recording after" htmlFor="meeting-max-hours" hint="A recording that runs this long is stopped and transcribed.">
          <Select id="meeting-max-hours" value={String(view.maxHours)} disabled={busy} onChange={(e) => act(() => MeetingAPI.SetMaxHours(Number(e.target.value)))}>
            {view.maxHoursChoices.map((h) => <option key={h} value={String(h)}>{hoursLabel(h)}</option>)}
          </Select>
        </Field>
        {view.detects && (
          <label className="check-row">
            <input
              type="checkbox"
              role="switch"
              data-meeting-signin
              checked={view.startsAtSignIn}
              disabled={busy}
              onChange={(e) => act(() => MeetingAPI.SetStartsAtSignIn(e.target.checked))}
            />
            Start Agentry when I sign in, so calls are noticed from then on
          </label>
        )}
        {assistant.enabled && (
          <div className="meeting-assistant" data-meeting-assistant>
            <p className="meeting-assistant-title">{capitalised(assistant.name)}</p>
            <label className="check-row">
              <input type="checkbox" role="switch" data-meeting-send checked={assistant.sendToAssistant} disabled={busy}
                onChange={(e) => act(() => MeetingAPI.SetSendToAssistant(e.target.checked))} />
              Give my meeting transcripts to {assistant.name}
            </label>
            {assistant.sendToAssistant && (
              <label className="check-row">
                <input type="checkbox" role="switch" data-meeting-ask-first checked={assistant.askFirst} disabled={busy}
                  onChange={(e) => act(() => MeetingAPI.SetAskFirst(e.target.checked))} />
                Ask me first: each transcript waits in {possessive(assistant.name)} share folder until I ask {assistant.name} to summarise it
              </label>
            )}
            <p className="small muted">
              {capitalised(assistant.name)} summarises each transcript with its meeting-notes skill and files the meeting in its memory. It is
              never started for this.
            </p>
          </div>
        )}
        {view.separatesSpeakers && (
          <>
            <label className="check-row">
              <input type="checkbox" role="switch" data-meeting-speakers checked={view.tellSpeakersApart} disabled={busy}
                onChange={(e) => act(() => MeetingAPI.SetTellSpeakersApart(e.target.checked))} />
              Tell speakers apart in meetings in the room
            </label>
            <p className="small muted">
              On, a meeting in the room's transcript says Speaker 1, Speaker 2 and so on, told apart by voice on this computer after the
              transcription. The first time, Agentry downloads about 46 MB of models. Off, every line says Room.
            </p>
          </>
        )}
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
