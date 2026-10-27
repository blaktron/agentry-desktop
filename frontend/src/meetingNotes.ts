// Meeting notes' words and numbers, apart from the page so the node tests
// can import them.

/** The line the person can paste into the meeting's chat. */
export const CHAT_LINE = "I'm recording this meeting to keep notes. They are transcribed on my computer.";

/** A length of time as the page says it: "45 s", "12 min", "1 h 05 min". */
export function duration(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  if (s < 60) return `${s} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min`;
  return `${Math.floor(m / 60)} h ${String(m % 60).padStart(2, "0")} min`;
}

/** A running recording's clock, "mm:ss" or "h:mm:ss", from its start. */
export function elapsed(startedISO: string, now: number): string {
  const s = Math.max(0, Math.floor((now - Date.parse(startedISO)) / 1000));
  const h = Math.floor(s / 3600);
  const mm = String(Math.floor(s / 60) % 60).padStart(2, "0");
  const ss = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

export type MeetingRow = {
  state: string;
  progress: number;
  error?: string;
  hasAudio: boolean;
  hasTranscript: boolean;
  transcriptName?: string;
};

/** What a meeting in the list says about where it stands. */
export function meetingState(m: MeetingRow): { words: string; tone: "ok" | "warn" | "info" } {
  switch (m.state) {
    case "recording":
      return { words: "Recording", tone: "warn" };
    case "pending":
      return { words: m.hasAudio ? "Waiting to be transcribed" : "Not transcribed; the audio is gone", tone: "info" };
    case "transcribing":
      return { words: m.progress >= 0 ? `Transcribing, ${m.progress}%` : "Transcribing", tone: "info" };
    case "done":
      return m.hasTranscript ? { words: "Transcribed", tone: "ok" } : { words: "Transcribed; the file was moved or deleted", tone: "warn" };
    case "failed":
      return { words: `Not transcribed: ${m.error || "the transcriber failed"}`, tone: "warn" };
    default:
      return { words: m.state, tone: "info" };
  }
}

/** Whether a meeting can be transcribed again from the list. */
export function canTranscribe(m: MeetingRow): boolean {
  return (m.state === "failed" || m.state === "pending") && m.hasAudio;
}

/** What a recording records, in its own words. */
export function recordingWhat(app: string, wholeDevice: boolean, inRoom = false): string {
  if (inRoom) return "your microphone in this room";
  if (wholeDevice) return "your microphone and everything your computer plays";
  return `your microphone and ${app || "the meeting app"}`;
}

/** What a meeting in the list says it recorded: the app, a meeting in the
 *  room, or the whole device. */
export function meetingSource(m: { app: string; inRoom: boolean; wholeDevice: boolean }): string {
  if (m.inRoom) return "In the room";
  return m.app || (m.wholeDevice ? "Whole device" : "");
}

/** The microphone choice's label for the default. */
export const DEFAULT_MIC = "The system's default";

/** A download's progress, "120 of 190 MB". */
export function downloadWords(done: number, total: number): string {
  if (!total) return "";
  const mb = (n: number) => Math.round(n / 1_000_000);
  return `${mb(done)} of ${mb(total)} MB`;
}

/** A refusal from Go's native dialog ("not confirmed: …") is the person's
 *  No, said as nothing rather than as an error. */
export function isNo(err: unknown): boolean {
  return String(err).includes("not confirmed");
}

/** The offer of a detected call, as its card's title says it. */
export function offerTitle(app: string): string {
  return `Record this meeting? (${app || "a meeting app"})`;
}

/** A longest-recording choice, "1 hour" or "4 hours". */
export function hoursLabel(h: number): string {
  return h === 1 ? "1 hour" : `${h} hours`;
}

/** How a meeting app's calls are handled, as its choice reads. */
export const AUTO_CHOICES = [
  { value: "ask", label: "Ask each time" },
  { value: "auto", label: "Record automatically" },
] as const;

/** A meeting's hand-over to the Assistant, as its row says it: nothing
 *  until it is handed on. */
export function deliveryWords(m: { delivery?: string; deliveryError?: string }, name: string): string {
  switch (m.delivery) {
    case "waiting":
      return `In ${possessive(name)} share folder, waiting for you to ask ${name} to summarise it`;
    case "sending":
      return `Being given to ${name}`;
    case "sent":
      return `Given to ${name} to summarise`;
    case "failed":
      return `Not given to ${name}: ${m.deliveryError || "it could not be copied or sent"}`;
    default:
      return "";
  }
}

/** Whether a meeting's row offers "Ask … to summarise": a transcript the
 *  Assistant was not given, while the person gives it transcripts. */
export function canAskAssistant(m: { state: string; hasTranscript: boolean; delivery?: string }, a: { enabled: boolean; sendToAssistant: boolean }): boolean {
  return a.enabled && a.sendToAssistant && m.state === "done" && m.hasTranscript && m.delivery !== "sent" && m.delivery !== "sending";
}

/** A name with its possessive ending: "Ada's", "the Assistant's". */
export function possessive(name: string): string {
  return name.endsWith("s") ? `${name}'` : `${name}'s`;
}

/** A sentence's start: "the Assistant" begins one as "The Assistant". */
export function capitalised(s: string): string {
  return s.startsWith("the ") ? `The ${s.slice(4)}` : s;
}

/** When an armed window records, "14:55 to 16:15", with its day when it is
 *  not today. */
export function windowWhen(opens: string, closes: string, now: Date): string {
  const o = new Date(opens);
  const c = new Date(closes);
  const t = (d: Date) => d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  const day = o.toDateString() === now.toDateString() ? "" : `${o.toLocaleDateString([], { weekday: "short", day: "numeric", month: "short" })}, `;
  return `${day}${t(o)} to ${t(c)}`;
}

/** Where an armed window stands, in words. */
export function windowStateWords(w: { state: string; recorded: number }): string {
  switch (w.state) {
    case "waiting":
      return "Waiting to open";
    case "open":
      return w.recorded > 0 ? "Open, recorded" : "Open: a call Agentry notices now is recorded";
    case "recording":
      return "Recording";
    default:
      return w.recorded > 0 ? "Recorded" : "Closed: no call was noticed";
  }
}

/** The offer of the graphics card, as its card's title says it. */
export function gpuOfferTitle(sizeBytes: number): string {
  return `Use your graphics card with the better model (${Math.round(sizeBytes / 1_000_000)} MB)`;
}

/** What the Settings choice of where to transcribe says about the card
 *  measured here. */
export function gpuHint(g: { name: string; fast: boolean; modelName: string }): string {
  return g.fast
    ? `The graphics card runs ${g.modelName}, the more accurate model. Agentry measured ${g.name} fast enough to keep up with a meeting.`
    : `The graphics card runs ${g.modelName}, the more accurate model, but Agentry measured ${g.name} too slow to keep up with a meeting: transcripts may take longer than the meeting did.`;
}
