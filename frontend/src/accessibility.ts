// Settings → Home → Accessibility: the text size, the zoom keys that change
// it, and the motion choice. Pure but for the root element and the
// listeners it is handed, so the node tests can import it.
//
// Go keeps both choices (HomeAPI.TextSize, SetTextSize, StepTextSize,
// Motion, SetMotion; desktop.yaml's textSize and motion keys). The text size
// is the window's own zoom, which Go sets, so text and layout grow together;
// the motion choice is a mark on the root element, which /theme.js puts
// there before the first paint.

export type Motion = "system" | "reduce";

/** The zoom step a key press asks for: 1 larger, -1 smaller, 0 back to the
 *  default, null for any other key. Ctrl with =, + or the keypad's +; Ctrl
 *  with - or the keypad's -; Ctrl with 0. On a Mac the View menu's own
 *  items (Cmd) take these keys, so the page leaves them alone there. */
export function zoomStep(e: { key: string; code?: string; ctrlKey: boolean; metaKey: boolean; altKey: boolean }, mac: boolean): 1 | -1 | 0 | null {
  if (mac || !e.ctrlKey || e.metaKey || e.altKey) return null;
  if (e.key === "=" || e.key === "+" || e.code === "NumpadAdd") return 1;
  if (e.key === "-" || e.key === "_" || e.code === "NumpadSubtract") return -1;
  if (e.key === "0" || e.code === "Numpad0") return 0;
  return null;
}

/** Whether this window runs on a Mac, where the menu bar has the zoom keys. */
export const onMac = (nav: { platform?: string; userAgent?: string }): boolean =>
  /Mac/i.test(nav.platform ?? "") || /Macintosh/i.test(nav.userAgent ?? "");

type Root = { dataset: Record<string, string | undefined> };

/** Puts the motion choice on the window: data-motion="reduce", or none to
 *  follow the computer. A value this app lacks follows the computer. */
export function applyMotion(choice: string, root: Root): Motion {
  const motion: Motion = choice === "reduce" ? "reduce" : "system";
  if (motion === "reduce") root.dataset.motion = "reduce";
  else delete root.dataset.motion;
  return motion;
}

/** The motion choice on the window now. */
export const motionOn = (root: Root): Motion => (root.dataset.motion === "reduce" ? "reduce" : "system");

/** How a size reads: "125%", with the default named. */
export const sizeLabel = (percent: number): string => (percent === 100 ? "100% (default)" : `${percent}%`);

// Who hears of a new text size: the Settings part, when it is open, and the
// line that announces it.
type Listener = (percent: number) => void;
const listeners = new Set<Listener>();

/** Follows the text size as it changes, from the keys, the menu or
 *  Settings; the returned function stops following. */
export function onTextSize(cb: Listener): () => void {
  listeners.add(cb);
  return () => { listeners.delete(cb); };
}

/** Tells every follower the text size is now `percent`. */
export function textSizeChanged(percent: number): void {
  for (const cb of listeners) cb(percent);
}
