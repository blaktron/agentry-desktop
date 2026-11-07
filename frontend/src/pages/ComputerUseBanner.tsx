// The computer-use banner: a small window of its own that shows while a
// run uses the computer, naming who uses what, with Pause (or Resume, which
// Go asks about again natively) and Stop. Every state comes from Go.
import { useEffect, useState } from "react";
import * as YourComputerAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/yourcomputerapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { EventsOn } from "../host";

export function ComputerUseBanner() {
  const [view, setView] = useState<app.BannerView | null>(null);
  const read = () => {
    YourComputerAPI.ComputerBanner().then(setView).catch(() => setView(null));
  };
  useEffect(() => {
    read();
    const off = EventsOn("yourcomputer:changed", read);
    const t = window.setInterval(read, 2000);
    return () => {
      off();
      window.clearInterval(t);
    };
  }, []);
  if (!view?.active) {
    return <div className="computer-use-banner" data-computer-banner>No agent is using your computer.</div>;
  }
  return (
    <div className="computer-use-banner" data-computer-banner role="status">
      <span className="computer-use-banner-dot" aria-hidden="true" />
      <span className="computer-use-banner-what selectable">
        {view.paused ? "Paused: " : "Using your computer: "}
        {view.what}
      </span>
      {view.paused ? (
        <button type="button" onClick={() => YourComputerAPI.ResumeComputerUse().finally(read)}>Resume</button>
      ) : (
        <button type="button" onClick={() => YourComputerAPI.PauseComputerUse().finally(read)}>Pause</button>
      )}
      <button type="button" className="danger" onClick={() => YourComputerAPI.StopComputerUse().finally(read)}>
        Stop ({view.stopKey})
      </button>
    </div>
  );
}
