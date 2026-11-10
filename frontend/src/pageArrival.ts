// What happens when another page opens in the content area: the keyboard
// focus moves to the page's heading (or the content area itself when it has
// none), so a screen reader says where the person now is and the next Tab
// starts on the page rather than in the sidebar; and the window's title
// names the page. Nothing moves on the first page the app opens with, a
// page that put the focus on one of its own controls keeps it there, and an
// open dialog keeps the focus it holds.
import { useEffect, useRef, type RefObject } from "react";

const APP = "Agentry Desktop";

/** The window's title for a page whose heading reads `heading`. */
export function pageTitle(heading: string | null | undefined): string {
  const h = (heading ?? "").replace(/\s+/g, " ").trim();
  return h ? `${h} — ${APP}` : APP;
}

/** Moves the focus to the page's heading and names the window after it,
 *  whenever `key` (the open page) changes. */
export function usePageArrival(key: string, content: RefObject<HTMLElement | null>): void {
  const first = useRef(true);
  useEffect(() => {
    const arrive = first.current ? null : focusHeading;
    first.current = false;
    let retry: ReturnType<typeof setTimeout> | undefined;
    const frame = requestAnimationFrame(() => {
      const box = content.current;
      if (!box) return;
      arrive?.(box);
      const h1 = box.querySelector("h1");
      document.title = pageTitle(h1?.textContent);
      // A page that draws its heading once its first read is answered.
      if (!h1) retry = setTimeout(() => { document.title = pageTitle(box.querySelector("h1")?.textContent); }, 400);
    });
    return () => { cancelAnimationFrame(frame); if (retry) clearTimeout(retry); };
  }, [key, content]);
}

function focusHeading(box: HTMLElement): void {
  const at = document.activeElement;
  // A control the page focused itself, or a dialog that holds the focus.
  if (at && at !== document.body && (box.contains(at) || at.closest('[role="dialog"],[role="alertdialog"]'))) return;
  const target = box.querySelector<HTMLElement>("h1") ?? box;
  if (!target.hasAttribute("tabindex")) target.setAttribute("tabindex", "-1");
  target.focus({ preventScroll: true });
}
