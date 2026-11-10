// The zoom keys on Windows and Linux (Ctrl with =, + or -, and Ctrl+0),
// which change the text size Settings → Home → Accessibility keeps, and a
// line that tells a screen reader the new size, from the keys, the macOS
// View menu or Settings. Mounted once, in the app's shell.
import { useEffect, useState } from "react";
import * as HomeAPI from "../bindings/github.com/blaktron/agentry-desktop/internal/app/homeapi";
import { EventsOn } from "./host";
import { onMac, onTextSize, textSizeChanged, zoomStep } from "./accessibility";

export function TextSizeKeys() {
  const [said, setSaid] = useState("");
  useEffect(() => onTextSize((percent) => setSaid(`Text size ${percent}%`)), []);
  // The macOS View menu changes it in Go, which tells the page.
  useEffect(() => EventsOn("accessibility:text-size", (percent: number) => textSizeChanged(percent)), []);
  useEffect(() => {
    const mac = onMac(navigator);
    const onKey = (e: KeyboardEvent) => {
      const step = zoomStep(e, mac);
      if (step === null) return;
      e.preventDefault();
      HomeAPI.StepTextSize(step).then(textSizeChanged).catch(() => undefined);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  return <p className="sr-only" role="status" data-text-size-said>{said}</p>;
}
