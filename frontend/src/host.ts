// The page's side of the window toolkit's runtime: the events Go sends (run
// lines, state changes, the tray's requests), the clipboard and the system
// browser. The bound calls are the generated bindings'.
import { Browser, Call, Clipboard, Events } from "@wailsio/runtime";

// A bound method that returns an error rejects its call with a RuntimeError
// whose message is the Go error's text. The pages show a failure as
// String(e) or `${e}`, which is that text alone; the class's own spelling
// ("RuntimeError: ...") would put its name in front of every message.
Call.RuntimeError.prototype.toString = function toString(this: Error): string {
  return this.message;
};

/**
 * Calls cb with the data of each event named name, and returns the cancel.
 * Go sends each event's payload as one JSON value; the caller states its
 * shape in cb's parameter.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function EventsOn(name: string, cb: (data: any) => void): () => void {
  return Events.On(name, (event) => cb(event.data));
}

/** Puts text on the clipboard, and says whether it got there. */
export function ClipboardSetText(text: string): Promise<boolean> {
  return Clipboard.SetText(text).then(() => true, () => false);
}

/** Opens an address in the system browser. */
export function BrowserOpenURL(url: string): void {
  void Browser.OpenURL(url).catch(() => undefined);
}
