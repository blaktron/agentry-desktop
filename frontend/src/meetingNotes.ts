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

/** What the far side records, in the recording's own words. */
export function recordingWhat(app: string, wholeDevice: boolean): string {
  if (wholeDevice) return "your microphone and everything your computer plays";
  return `your microphone and ${app || "the meeting app"}`;
}

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
