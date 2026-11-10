// Settings → Home → Accessibility: how large Agentry Desktop draws its text
// and layout, and whether it moves. Each choice shows at once and Go keeps
// it (accessibility.ts).
import { useEffect, useState } from "react";
import * as HomeAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/homeapi";
import { Card, ErrorBox } from "../ui";
import { applyMotion, motionOn, onMac, onTextSize, sizeLabel, textSizeChanged, type Motion } from "../accessibility";

export function AccessibilitySettings() {
  const [sizes, setSizes] = useState<number[]>([]);
  const [size, setSize] = useState<number | null>(null);
  const [motion, setMotion] = useState<Motion>(() => motionOn(document.documentElement));
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    Promise.all([HomeAPI.TextSizes(), HomeAPI.TextSize()])
      .then(([all, now]) => { if (live) { setSizes(all ?? []); setSize(now); } })
      .catch((e) => { if (live) setErr(`The text size could not be read: ${e}`); });
    return () => { live = false; };
  }, []);
  // The keys and the macOS View menu change it too.
  useEffect(() => onTextSize(setSize), []);

  const chooseSize = async (percent: number) => {
    setErr(null);
    try {
      textSizeChanged(await HomeAPI.SetTextSize(percent));
    } catch (e) {
      setErr(String(e));
    }
  };
  const chooseMotion = async (choice: Motion) => {
    const before = motion;
    setErr(null);
    setMotion(applyMotion(choice, document.documentElement));
    try {
      await HomeAPI.SetMotion(choice);
    } catch (e) {
      setMotion(applyMotion(before, document.documentElement));
      setErr(String(e));
    }
  };
  const key = onMac(navigator) ? "⌘" : "Ctrl+";

  return (
    <section id="settings-part-accessibility" className="settings-part">
      <h2>Accessibility</h2>
      <Card>
        <fieldset className="a11y-choices" data-text-size>
          <legend><strong>Text size</strong></legend>
          <p className="muted small" style={{ marginTop: 0 }}>
            How large Agentry Desktop draws its text and everything around it, on this computer. {key}= makes it larger,{" "}
            {key}− smaller and {key}0 puts it back to 100%.
          </p>
          {size !== null && (
            <div className="row" role="radiogroup" aria-label="Text size">
              {sizes.map((p) => (
                <label key={p} className={`a11y-choice${size === p ? " is-chosen" : ""}`}>
                  <input type="radio" name="text-size" value={p} checked={size === p} onChange={() => void chooseSize(p)} />
                  <span>{sizeLabel(p)}</span>
                </label>
              ))}
            </div>
          )}
        </fieldset>
        <fieldset className="a11y-choices" data-motion-choice>
          <legend><strong>Motion</strong></legend>
          <div role="radiogroup" aria-label="Motion">
            <label className={`check-row${motion === "system" ? " is-chosen" : ""}`}>
              <input type="radio" name="motion" value="system" checked={motion === "system"} onChange={() => void chooseMotion("system")} />
              <span>As your computer is set: reduced when it asks apps to reduce motion.</span>
            </label>
            <label className={`check-row${motion === "reduce" ? " is-chosen" : ""}`}>
              <input type="radio" name="motion" value="reduce" checked={motion === "reduce"} onChange={() => void chooseMotion("reduce")} />
              <span>Reduce motion: no spinning, pulsing or sliding, whatever your computer says.</span>
            </label>
          </div>
        </fieldset>
        <p className="muted small">
          For stronger lines and text, choose the High contrast theme under Theme. Screen readers, the keyboard and your
          computer&apos;s own zoom and colour settings work throughout.
        </p>
        <ErrorBox error={err} />
      </Card>
    </section>
  );
}
