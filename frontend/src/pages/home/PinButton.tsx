import { useCallback, useEffect, useState } from "react";
import * as HomeAPI from "../../../wailsjs/go/main/HomeAPI";
import { main } from "../../../wailsjs/go/models";
import { Button } from "../../ui";
import { homeFullSentence, pinOffer } from "../../pins";

// Pin to Home (plan desktop-home-and-library.md D5, D11; agentry-desktop#660),
// on a Library row and on an installed QuickAgent's page. Go keeps the pins
// and places each one; the page reads the layout and asks for one change.
// A pin is kept on this computer only and is not a star: no registry is
// asked.

/** Home's layout as Go keeps it, the setter a pin's answer goes to, and a
 * re-read for after an uninstall, which removes the QuickAgent's pin in Go.
 * null until read, or when it cannot be (no Pin is offered then). */
export function useHomeLayout(): [main.HomeLayoutView | null, (v: main.HomeLayoutView) => void, () => void] {
  const [layout, setLayout] = useState<main.HomeLayoutView | null>(null);
  const reread = useCallback(() => { HomeAPI.Layout().then(setLayout).catch(() => {}); }, []);
  useEffect(() => {
    let live = true;
    HomeAPI.Layout().then((v) => live && setLayout(v)).catch(() => {});
    return () => { live = false; };
  }, []);
  return [layout, setLayout, reread];
}

/** The sentence a page shows while Home holds every pin it can (D5), so a
 * held Pin button is explained, not silent. */
export function homeFullLine(layout: main.HomeLayoutView | null): string | null {
  if (!layout || layout.newer || layout.unreadable) return null;
  return homeFullSentence(layout.cards, layout);
}

export function PinButton({ agentRef, registry, layout, onLayout }: {
  // namespace/name.
  agentRef: string;
  // The record's origin, "" for a folder install.
  registry: string;
  layout: main.HomeLayoutView | null;
  onLayout: (v: main.HomeLayoutView) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  // A layout this app keeps rather than uses takes no pin (D-067).
  if (!layout || layout.newer || layout.unreadable) return null;
  const offer = pinOffer(layout.cards, layout, agentRef, registry);
  async function toggle() {
    setBusy(true);
    setErr(null);
    try {
      onLayout(await (offer.pinned ? HomeAPI.Unpin(agentRef, registry) : HomeAPI.Pin(agentRef, registry)));
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <Button onClick={toggle} busy={busy} disabled={offer.full} title={offer.why} data-pin-button={offer.pinned ? "unpin" : offer.full ? "full" : "pin"}>
        {offer.pinned ? "Unpin from Home" : "Pin to Home"}
      </Button>
      {err && <span className="small step-problem" role="alert">{err}</span>}
    </>
  );
}
