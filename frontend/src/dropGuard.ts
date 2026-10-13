// The window's drop guard (agentry-desktop#644). No page of the app takes a
// dropped file, and a drop that no element takes gets the webview's default,
// which in a browser opens the dropped file or link in place of the app's
// document. That page would be outside the bundle the window's
// Content-Security-Policy was written for (internal/webview), while the
// window still carries every Go binding. So the window cancels:
//
// - every drag that carries a file, wherever it is dropped, the voice
//   editor and the text fields included (ProseMirror inserts only a drop's
//   text or HTML, and filters those through the schema, agentry-desktop#612);
// - a drag of text or a link anywhere but a text field or the voice editor,
//   where it would otherwise navigate or open.
//
// Text dragged into a field or the editor still lands there, the browser's
// own behaviour. Wails' DisableWebViewDrop was not used: it refuses those
// drops too. The listeners are on the window in the capture phase, so they
// run before any page's handler, and they never stop the event, so a field
// still sees it.

const DRAG_EVENTS = ["dragenter", "dragover", "drop"] as const;

type DragLike = Event & { dataTransfer?: { types?: ArrayLike<string> | null; dropEffect?: string } | null };

// carriesFiles is whether the drag holds a file: "Files" among its types,
// which every engine the desktop runs on (WebKitGTK, WebView2, WKWebView)
// sets for a drag from the file manager.
function carriesFiles(event: DragLike): boolean {
  const types = event.dataTransfer?.types;
  return !!types && Array.from(types).includes("Files");
}

// takesText is whether a drop of text on this target is the person typing
// into a field: a text input or a textarea that may be edited, or the
// inside of a contenteditable (the voice editor).
function takesText(target: EventTarget | null): boolean {
  const el = target as (Element & { isContentEditable?: boolean }) | null;
  if (!el || typeof el.closest !== "function") return false;
  if (el.isContentEditable || el.closest('[contenteditable="true"], [contenteditable=""]')) return true;
  if (el instanceof (el.ownerDocument?.defaultView ?? globalThis).HTMLTextAreaElement) return !el.readOnly && !el.disabled;
  if (el instanceof (el.ownerDocument?.defaultView ?? globalThis).HTMLInputElement) {
    const textual = ["", "text", "search", "url", "email", "tel", "password", "number"].includes(el.getAttribute("type")?.toLowerCase() ?? "");
    return textual && !el.readOnly && !el.disabled;
  }
  return false;
}

// refuses is the guard's one rule, kept apart so it reads as the comment
// above: a file never; text only into a field or the editor.
function refuses(event: DragLike): boolean {
  if (!event.dataTransfer) return true;
  return carriesFiles(event) || !takesText(event.target);
}

// installDropGuard puts the guard on a window and returns what removes it.
export function installDropGuard(win: Window): () => void {
  const guard = (event: Event) => {
    const drag = event as DragLike;
    if (!refuses(drag)) return;
    event.preventDefault();
    // On dragenter and dragover, a cancelled event with "none" shows the
    // no-drop cursor and keeps the drop from firing at all.
    if (event.type !== "drop" && drag.dataTransfer) drag.dataTransfer.dropEffect = "none";
  };
  for (const type of DRAG_EVENTS) win.addEventListener(type, guard, true);
  return () => {
    for (const type of DRAG_EVENTS) win.removeEventListener(type, guard, true);
  };
}
