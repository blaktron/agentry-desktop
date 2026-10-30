// The window's drop guard. No page of the app takes a
// dropped file, and a drop that no element takes gets the webview's default,
// which in a browser opens the dropped file or link in place of the app's
// document. That page would be outside the bundle the window's
// Content-Security-Policy was written for (internal/webview), while the
// window still carries every Go binding. So the window cancels:
//
// - every drag that carries a file, wherever it is dropped, the voice
//   editor and the text fields included (ProseMirror inserts only a drop's
//   text or HTML, and filters those through the schema).
//   A drag reports a file as "Files" among its types, except in WebKitGTK,
//   which reports a file manager's drag as text/uri-list and text/html and
//   withholds the URI list from the page (it reads as ""), and a field would
//   take the file's address as text. Data is readable only on drop, so a
//   drop whose URI list is listed but withheld, or names a file: URI, is
//   cancelled there (observed in WebKitGTK 4.1);
// - a drag of text or a link anywhere but a text field or the voice editor,
//   where it would otherwise navigate or open.
//
// Text dragged into a field or the editor still lands there, the browser's
// own behaviour. The window library's option to refuse every drop was not
// used: it refuses those too. The listeners are on the window in the capture phase, so they
// run before any page's handler, and they never stop the event, so a field
// still sees it.

const DRAG_EVENTS = ["dragenter", "dragover", "drop"] as const;

type DragLike = Event & { dataTransfer?: { types?: ArrayLike<string> | null; dropEffect?: string; getData?: (format: string) => string } | null };

// carriesFiles is whether the drag holds a file: "Files" among its types,
// as a browser reports a dragged file, or, on drop, a URI list that is
// listed but withheld or that names a file: URI, as WebKitGTK reports one
// (above). A link's URI list is readable, so a link is not a file.
function carriesFiles(event: DragLike): boolean {
  const types = event.dataTransfer?.types;
  if (types && Array.from(types).includes("Files")) return true;
  if (event.type !== "drop" || !types || !Array.from(types).includes("text/uri-list")) return false;
  let uris = "";
  try {
    uris = event.dataTransfer?.getData?.("text/uri-list") ?? "";
  } catch {
    return true; // listed, and not readable: withheld
  }
  // A URI list's comment lines start "#".
  const entries = uris.split(/\r?\n/).map((line) => line.trim()).filter((line) => line && !line.startsWith("#"));
  return entries.length === 0 || entries.some((line) => /^file:/i.test(line));
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
