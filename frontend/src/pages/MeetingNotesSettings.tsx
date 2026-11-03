// Meeting notes' settings, drawn in two places: on the Meeting notes page,
// above the list of meetings, and as Settings → Meeting notes. Both read and
// change the same settings through MeetingAPI, and each re-reads on
// meeting:changed, so a change in one shows in the other. Turning it on, the
// automatic list and giving transcripts to the Assistant stay confirmed in
// Go's native dialog, whichever place asks.
import { useCallback, useEffect, useState } from "react";
import * as MeetingAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/meetingapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { EventsOn } from "../host";
import { Button, Card, ErrorBox, Field, Notice, Select, Spinner } from "../ui";
import { AUTO_CHOICES, DEFAULT_MIC, capitalised, gpuHint, hoursLabel, isNo, possessive } from "../meetingNotes";

type View = app.MeetingNotesView;
type Act = (f: () => Promise<View | void>) => void;

/** The settings card: every Meeting notes setting, for a view that is on. */
export function MeetingNotesSettingsCard({ view, busy, act }: { view: View; busy: boolean; act: Act }) {
  const assistant = view.assistant;
  return (
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
          <label className="check-row">
            <input type="checkbox" role="switch" data-meeting-call-speakers checked={view.tellCallSpeakersApart} disabled={busy}
              onChange={(e) => act(() => MeetingAPI.SetTellCallSpeakersApart(e.target.checked))} />
            Tell speakers apart on calls
          </label>
          <p className="small muted">
            On, a call's transcript keeps Me for you and says Speaker 1, Speaker 2 and so on for the others on the call, told apart by voice
            on this computer after the transcription. It adds a few minutes to each transcription, and the first time Agentry downloads
            about 46 MB of models. Off, everyone else on the call is Them.
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
  );
}

/**
 * Settings → Meeting notes: the same card, read here, with a way to the
 * page. Off, it says so and leaves turning it on to the page, which
 * explains what that means first.
 */
export function MeetingNotesSettings({ onOpenPage }: { onOpenPage?: () => void }) {
  const [view, setView] = useState<View | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

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

  const act: Act = async (f) => {
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

  if (!view) return error ? <ErrorBox error={error} /> : <Spinner label="Reading Meeting notes" />;
  if (!view.available) return <Notice tone="warn">{view.unavailable || "Meeting notes is not available on this computer."}</Notice>;
  const open = onOpenPage && <Button variant="ghost" data-meeting-open-page onClick={onOpenPage}>Open Meeting notes</Button>;
  if (!view.on) {
    return (
      <Card data-meeting-settings-off>
        <p style={{ marginTop: 0 }}>
          Meeting notes is off. Turn it on on the Meeting notes page, which says what it records first; its settings are here once it is on.
        </p>
        {open && <div className="row">{open}</div>}
      </Card>
    );
  }
  return (
    <>
      <ErrorBox error={error} />
      <MeetingNotesSettingsCard view={view} busy={busy} act={act} />
      {open && (
        <p className="small muted">
          These are also on the Meeting notes page, above your meetings. {open}
        </p>
      )}
    </>
  );
}
