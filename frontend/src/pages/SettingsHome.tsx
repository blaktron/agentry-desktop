import { useEffect, useState } from "react";
import * as HomeAPI from "../../wailsjs/go/main/HomeAPI";
import * as InstallAPI from "../../wailsjs/go/main/InstallAPI";
import { install, main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Modal, Notice } from "../ui";
import { cardSwitches, HOME_DEFAULTS } from "../homeCards";
import { pinRecord, pinsOf, type Pin } from "../pins";

// Settings → Home (plan desktop-home-and-library.md §4.5, D6, D-067;
// agentry-desktop#661): which default cards Home shows, the QuickAgents
// pinned to it, and Reset Home. Go owns the layout (HomeAPI): each switch,
// Unpin and Reset is one change Go makes and answers with the layout as it
// now is, so the page never writes it whole. Hiding a card keeps its place,
// Unpin follows the Pinned cards' rules (an emptied card goes, no pin moves
// to another card), and Reset drops every pin, which the page says before
// it asks. Reordering and sizes are later (D6, D12).

// What each default card holds, under its switch.
const CARD_LINES: Record<string, string> = {
  "recent-runs": "The last five runs, chats and built-in jobs, on this computer and In Agentry.",
  installed: "How many QuickAgents are on this computer, how many have an update, and the newest five.",
  profile: "Your name, plan and credits on the registry in use, or Sign in.",
  "run-defaults": "Where runs go, their limits, the output folder, and Unsafe Mode while it is on.",
};

type HomePart = "cards" | "pins" | "reset";

// What a change in this part answered: Go's refusal, or what was done.
function Said({ said, part }: { said: { part: HomePart; text: string; error?: boolean } | null; part: HomePart }) {
  if (!said || said.part !== part) return null;
  return said.error ? <ErrorBox error={said.text} /> : <p className="small home-said" role="status">{said.text}</p>;
}

const hostOf = (url: string) => url.replace(/^https?:\/\//, "").replace(/\/+$/, "");

export function HomeSettings() {
  const [view, setView] = useState<main.HomeLayoutView | null>(null);
  const [records, setRecords] = useState<install.Item[] | null>(null);
  const [recordsFailed, setRecordsFailed] = useState(false);
  // The read's error; a change's answer or refusal is said in the part
  // where it was asked, next to what was clicked.
  const [err, setErr] = useState<string | null>(null);
  const [said, setSaid] = useState<{ part: HomePart; text: string; error?: boolean } | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmReset, setConfirmReset] = useState(false);

  useEffect(() => {
    let live = true;
    HomeAPI.Layout().then((v) => { if (live) setView(v); }).catch((e) => { if (live) setErr(String(e)); });
    // Only to say each pin's version, or that it is gone: a failed read
    // leaves the pins listed without either.
    InstallAPI.ListInstalled().then((v) => { if (live) setRecords(v ?? []); }).catch(() => { if (live) setRecordsFailed(true); });
    return () => { live = false; };
  }, []);

  async function act(part: HomePart, fn: () => Promise<main.HomeLayoutView>, done?: string) {
    setBusy(true);
    setSaid(null);
    try {
      setView(await fn());
      if (done) setSaid({ part, text: done });
    } catch (e) {
      setSaid({ part, text: String(e), error: true });
    } finally {
      setBusy(false);
    }
  }

  if (!view) return err ? <ErrorBox error={`Home's layout could not be read: ${err}`} /> : <div className="empty">Reading Home's layout…</div>;

  // A layout a newer app arranged, or one this app cannot decode, is drawn
  // as the defaults and kept: Go refuses a change to it, so the switches and
  // Unpin are held, and Reset is the way to start again.
  const kept = view.newer || view.unreadable;
  const switches = cardSwitches(view.cards);
  const pinCards = kept ? [] : view.cards.filter((c) => c.kind === "pinned" && (c.pins?.length ?? 0) > 0);
  const pins = kept ? [] : pinsOf(view.cards);

  return (
    <>
      {view.newer && (
        <Notice tone="info">
          Home was arranged by a newer Agentry Desktop. This one shows the default cards, leaves that arrangement as it
          is, and changes nothing in it. Reset Home starts again from the defaults.
        </Notice>
      )}
      {view.unreadable && (
        <Notice tone="warn">
          The settings file&apos;s Home arrangement cannot be read by this Agentry Desktop, so the default cards are shown
          and the file is left as it is. Reset Home starts again from the defaults.
        </Notice>
      )}

      <section id="settings-part-home-cards" className="settings-part">
        <h2>Cards</h2>
        <Card>
          <p className="muted small" style={{ marginTop: 0 }}>
            A hidden card keeps its place on Home, and shows there again when you turn it back on.
          </p>
          {switches.map((s) => (
            <div key={s.kind} className="home-card-switch" data-home-switch={s.kind}>
              <label className="check-row" style={{ margin: 0 }}>
                <input type="checkbox" role="switch" checked={!kept && s.shown} disabled={busy || kept}
                  onChange={(e) => void act("cards", () => HomeAPI.SetShown(s.kind, e.target.checked))} />
                <span><strong>{s.title}</strong></span>
              </label>
              <p className="muted small">{CARD_LINES[s.kind]}</p>
            </div>
          ))}
          <Said said={said} part="cards" />
        </Card>
      </section>

      <section id="settings-part-pins" className="settings-part">
        <h2>Pinned</h2>
        <Card>
          {kept ? (
            <p className="muted small" style={{ margin: 0 }}>The pins of an arrangement this app keeps rather than uses are not listed here.</p>
          ) : pins.length === 0 ? (
            <p className="muted small" style={{ margin: 0 }} data-pins-none>
              Nothing is pinned. Pin to Home, on a QuickAgent&apos;s row in the Library or on its page, puts it on a
              Pinned card on Home, with its latest run and Run.
            </p>
          ) : (
            <>
              <p className="small" style={{ marginTop: 0 }} data-pins-count>
                {pins.length} of {view.maxPins} pinned, {view.pinsPerCard} to a Pinned card. Unpin leaves the other pins
                where they are; a card it empties goes from Home.
              </p>
              {pinCards.map((c, i) => (
                <div key={i} className="home-pins-card" data-pins-card={i}>
                  {pinCards.length > 1 && <p className="muted small home-pins-card-title">Pinned card {i + 1}</p>}
                  <ul className="home-card-list">
                    {(c.pins ?? []).map((pin: Pin) => {
                      const rec = records ? pinRecord(records, pin) : undefined;
                      return (
                        <li key={`${pin.registry ?? ""} ${pin.ref}`} data-pin={pin.ref}>
                          <div>
                            <span className="mono">{pin.ref}</span>
                            <span className="muted small"> {pin.registry ? `from ${hostOf(pin.registry)}` : "from a folder"}</span>
                            {rec && <span className="muted small"> · v{rec.version}</span>}
                          </div>
                          {records && !rec && <div className="muted small">No longer installed on this computer.</div>}
                          <div className="small">
                            <Button variant="link" disabled={busy}
                              onClick={() => void act("pins", () => HomeAPI.Unpin(pin.ref, pin.registry ?? ""), `Unpinned ${pin.ref}.`)}>Unpin</Button>
                          </div>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ))}
              {recordsFailed && <p className="muted small">What is installed on this computer could not be read, so no version is shown.</p>}
            </>
          )}
          <Said said={said} part="pins" />
        </Card>
      </section>

      <section id="settings-part-reset-home" className="settings-part">
        <h2>Reset Home</h2>
        <Card>
          <p className="small" style={{ marginTop: 0 }}>
            Shows the {HOME_DEFAULTS.length} default cards again in their first order
            {pins.length > 0 ? <>, and removes every pin ({pins.length})</> : null}. It uninstalls nothing.
          </p>
          <Button disabled={busy} data-reset-home onClick={() => setConfirmReset(true)}>Reset Home…</Button>
          <Said said={said} part="reset" />
        </Card>
      </section>

      {confirmReset && (
        <Modal title="Reset Home?" onClose={() => setConfirmReset(false)}>
          <p>
            Home shows the {HOME_DEFAULTS.length} default cards again, in their first order
            {pins.length > 0
              ? <>, and <strong>every pin is removed</strong>: {pins.length === 1 ? "the 1 QuickAgent" : `the ${pins.length} QuickAgents`} pinned
                there stay installed, and Pin to Home puts them back</>
              : null}
            {kept ? <>. The arrangement the settings file holds now is replaced</> : null}.
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setConfirmReset(false)}>Cancel</Button>
            <Button variant="danger" data-reset-home-confirm
              onClick={() => { setConfirmReset(false); void act("reset", () => HomeAPI.Reset(), "Home is back to its default cards."); }}>
              Reset Home
            </Button>
          </div>
        </Modal>
      )}
    </>
  );
}
