// Plugins: the optional features of Agentry Desktop, a card each, drawn
// like Settings' landing: the whole card opens the feature's page, with one
// sentence and a status line. Meeting notes says why it is not offered where
// this copy cannot record.
import { useEffect, useState } from "react";
import * as MeetingAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/meetingapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { PageHeader } from "../ui";
import { meetingNotesStatus } from "../meetingNotes";

export type PluginPage = "meetings" | "computer";

export default function Plugins({ onOpen }: { onOpen: (page: PluginPage) => void }) {
  // Unread, the Meeting notes card shows without a line; a failed read
  // leaves it the same.
  const [meetings, setMeetings] = useState<app.MeetingNotesView | null>(null);
  useEffect(() => {
    let live = true;
    Promise.resolve().then(() => MeetingAPI.Status()).then((v) => { if (live) setMeetings(v); }).catch(() => undefined);
    return () => { live = false; };
  }, []);

  const meetingsOff = meetings !== null && !meetings.available;
  const cards: { page: PluginPage; title: string; sentence: string; status: string | null; unavailable?: string }[] = [
    {
      page: "meetings", title: "Meeting notes",
      sentence: "Records your meetings on this computer, transcribes them here, and gives the Assistant what was said: the language and model, telling speakers apart in the room and on calls, the transcripts folder and the meeting apps.",
      status: meetings?.available ? meetingNotesStatus(meetings) : null,
      unavailable: meetingsOff ? (meetings?.unavailable || "Not offered on this computer.") : undefined,
    },
    {
      page: "computer", title: "Computer use",
      sentence: "The applications on this computer a QuickAgent may use, their script screening and computer use, and the runs using your computer now or that used it.",
      status: null,
    },
  ];

  return (
    <div data-page="plugins">
      <PageHeader title="Plugins" sub="Features you turn on when you want them. Each card opens one." />
      <div className="settings-cards">
        {cards.map((c) => (
          <div key={c.page} className={`card settings-card${c.unavailable ? " is-unavailable" : ""}`} data-plugin={c.page}>
            {c.unavailable ? (
              <strong>{c.title}</strong>
            ) : (
              // The title's button is stretched over the whole card, so the
              // card is one link.
              <button type="button" className="settings-card-link" onClick={() => onOpen(c.page)}>
                <strong>{c.title}</strong>
              </button>
            )}
            <p className="muted small">{c.sentence}</p>
            {c.unavailable && <p className="small settings-card-status" role="status">{c.unavailable}</p>}
            {c.status && <p className="small settings-card-status" role="status">{c.status}</p>}
          </div>
        ))}
      </div>
    </div>
  );
}
