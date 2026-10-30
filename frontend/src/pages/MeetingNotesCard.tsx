// The Assistant's Meeting notes card, drawn for its "meetings" roster entry:
// whether Meeting notes gives it each transcript to summarise and file
// (turned on in Go's native dialog, which says its model provider reads
// them), whether the person is asked first, and the entry's own switch,
// "Can set up meeting recordings", which Go confirms like any roster switch
// and which needs the heartbeat and Meeting notes on.
import { useCallback, useEffect, useRef, useState } from "react";
import * as MeetingAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/meetingapi";
import * as PersistentAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/persistentapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { EventsOn } from "../host";
import { cardIcon, cardState } from "../configCards";
import { ConfigIcon } from "../configGlyphs";
import { capitalised, isNo, possessive } from "../meetingNotes";
import type { Act } from "./ConfigCards";

export function MeetingNotesCard({ agent, entry: e, busy, act, onError }: {
  agent: app.PersistentAgentView;
  entry: app.PersistentRosterEntry;
  busy: boolean;
  act: Act;
  onError: (e: string | null) => void;
}) {
  const [view, setView] = useState<app.MeetingNotesView | null>(null);
  const [working, setWorking] = useState(false);
  // The page's error box, held so a new one does not read the state again.
  const said = useRef(onError);
  said.current = onError;
  const read = useCallback(() => {
    MeetingAPI.Status().then(setView).catch((err) => said.current(String(err)));
  }, []);
  useEffect(() => {
    read();
    try {
      return EventsOn("meeting:changed", read);
    } catch {
      return undefined;
    }
  }, [read]);
  const meetingAct = async (f: () => Promise<app.MeetingNotesView>) => {
    setWorking(true);
    onError(null);
    try {
      setView(await f());
    } catch (err) {
      if (!isNo(err)) onError(String(err));
    } finally {
      setWorking(false);
    }
  };
  const a = view?.assistant;
  const name = a?.name ?? "the Assistant";
  const sending = !!a?.sendToAssistant;
  const heartbeat = agent.roster.find((r) => r.id === "heartbeat");
  const state = cardState(agent, e);
  const off = view !== null && (!view.available || !view.on);
  const flipArming = (on: boolean) => act(() => PersistentAPI.Configure(agent.name, { switch: { id: e.id, on } } as app.PersistentChange));
  return (
    <div className={`config-card config-card-meetings tone-${sending ? "on" : "off"}${sending ? " on" : " off"}`} data-meeting-card>
      <button type="button" className="config-card-main" role="switch" aria-checked={sending}
        aria-label={`Meeting notes: ${sending ? "transcripts are given to it" : "off"}`}
        disabled={busy || working || !view || off} onClick={() => void meetingAct(() => MeetingAPI.SetSendToAssistant(!sending))}>
        <ConfigIcon icon={cardIcon(e)} />
        <span className="config-card-text">
          <span className="config-card-name">Meeting notes</span>
          <span className="config-card-line">Your meetings' transcripts, summarised and filed in its memory</span>
          <span className="config-card-state">{off ? "Meeting notes is off" : sending ? "On" : "Off"}</span>
        </span>
      </button>
      {off && (
        <p className="small muted config-card-note" data-meeting-card-off>
          {view?.available ? "Turn on Meeting notes on its page first." : view?.unavailable}
        </p>
      )}
      {!off && sending && (
        <label className="check-row config-card-share">
          <input type="checkbox" role="switch" aria-label={`Ask me first: ${a?.askFirst ? "on" : "off"}`} data-meeting-card-ask-first
            checked={!!a?.askFirst} disabled={busy || working} onChange={(ev) => void meetingAct(() => MeetingAPI.SetAskFirst(ev.target.checked))} />
          <span className="small">
            Ask me first
            <br />
            <span className="muted">Each transcript waits in {possessive(name)} share folder until you ask {name} to summarise it.</span>
          </span>
        </label>
      )}
      <label className="check-row config-card-share">
        <input type="checkbox" role="switch" aria-label={`${e.name}: ${state.words}`} data-meeting-card-arming
          checked={e.on} disabled={busy || working || (!e.on && off)} onChange={(ev) => void flipArming(ev.target.checked)} />
        <span className="small">
          {e.name}
          <br />
          <span className="muted">
            {capitalised(name)} arms a recording for a meeting on your calendar that you asked it to record, within the hour before it starts.
            Each one is shown on Home and in Meeting notes, where you can cancel it.
            {e.on && heartbeat && !heartbeat.on && " It needs its heartbeat on too."}
          </span>
        </span>
      </label>
    </div>
  );
}
