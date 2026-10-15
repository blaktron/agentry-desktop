// A deep link's part, followed while its page fills in.
// A Settings section's cards make their own reads (Run defaults and its Docker
// parts, Profile, the provider form), so the part a link names, or what sits
// above it, can render after the route changes. followPart scrolls to the
// element now and again on every change to the document, until the person
// takes over or the time runs out. It only scrolls: it focuses, clicks and
// changes nothing.

// Long enough for the Docker reads under Run defaults on a slow engine.
export const FOLLOW_PART_MS = 10_000;
// What the person does that ends the follow at once.
export const PERSON_TAKES_OVER = ["wheel", "touchstart", "keydown", "pointerdown"] as const;
// A scroll event this soon after the follow scrolled is the follow's own.
export const FOLLOW_OWN_SCROLL_MS = 150;

// followPart returns the function that stops it, for an effect's cleanup.
export function followPart(id: string, ms = FOLLOW_PART_MS): () => void {
  let followedAt = -Infinity;
  const follow = () => {
    const el = document.getElementById(id);
    if (!el) return;
    followedAt = performance.now();
    el.scrollIntoView({ block: "start" });
  };
  // A scroll the follow did not just make is the person's: dragging the
  // scrollbar fires none of PERSON_TAKES_OVER.
  const scrolled = () => { if (performance.now() - followedAt > FOLLOW_OWN_SCROLL_MS) stop(); };
  const observer = new MutationObserver(follow);
  let timer: ReturnType<typeof setTimeout> | undefined;
  const stop = () => {
    observer.disconnect();
    clearTimeout(timer);
    PERSON_TAKES_OVER.forEach((e) => window.removeEventListener(e, stop, true));
    window.removeEventListener("scroll", scrolled, true);
  };
  follow();
  observer.observe(document.body, { childList: true, subtree: true });
  PERSON_TAKES_OVER.forEach((e) => window.addEventListener(e, stop, true));
  window.addEventListener("scroll", scrolled, true);
  timer = setTimeout(stop, ms);
  return stop;
}
