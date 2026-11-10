// Settings → Home → Accessibility and the zoom keys: Ctrl with =, + or -
// and Ctrl+0 step the kept text size on Windows and Linux (on a Mac the
// View menu has them); a step, a choice in Settings and the macOS menu's
// step all reach the Settings part and are said to a screen reader; and
// Reduce motion marks the window at once.
import { window } from "./dom.ts";
import { afterEach, test } from "node:test";
import assert from "node:assert/strict";
import { applyMotion, motionOn, onMac, sizeLabel, zoomStep } from "../src/accessibility.ts";
import { emit } from "./wails-mock.ts";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = globalThis as any;
g.IS_REACT_ACT_ENVIRONMENT = true;

const React = await import("react");
const { createRoot } = await import("react-dom/client");
const { AccessibilitySettings } = await import("../src/pages/SettingsAccessibility.tsx");
const { TextSizeKeys } = await import("../src/TextSizeKeys.tsx");

let root: ReturnType<typeof createRoot> | null = null;
let el: HTMLElement | null = null;
afterEach(async () => {
  if (root) await React.act(async () => root?.unmount());
  el?.remove();
  root = null;
  el = null;
  delete g.go;
  delete window.document.documentElement.dataset.motion;
});

async function render(node: React.ReactElement) {
  el = window.document.createElement("div");
  window.document.body.appendChild(el);
  root = createRoot(el);
  await React.act(async () => { root!.render(node); });
  await React.act(async () => {});
  return el;
}

const SIZES = [100, 110, 125, 150, 175, 200];

/** Go's HomeAPI for the text size and motion, keeping what it is told. */
function mockHome(start = 100) {
  const kept = { size: start, motion: "system", calls: [] as string[] };
  g.go = {
    app: {
      HomeAPI: {
        TextSizes: () => SIZES,
        TextSize: () => kept.size,
        SetTextSize: (p: number) => { kept.calls.push(`set ${p}`); kept.size = p; return p; },
        StepTextSize: (step: number) => {
          kept.calls.push(`step ${step}`);
          const i = SIZES.indexOf(kept.size);
          kept.size = step === 0 ? 100 : SIZES[Math.max(0, Math.min(SIZES.length - 1, i + step))];
          return kept.size;
        },
        Motion: () => kept.motion,
        SetMotion: (m: string) => { kept.calls.push(`motion ${m}`); kept.motion = m; },
      },
    },
  };
  return kept;
}

const key = (k: string, mods: Partial<KeyboardEventInit> = {}) => ({ key: k, code: "", ctrlKey: false, metaKey: false, altKey: false, ...mods });

test("the zoom keys are Ctrl with =, +, - or 0, never with Alt or the Mac's Cmd, and none on a Mac", () => {
  assert.equal(zoomStep(key("=", { ctrlKey: true }), false), 1);
  assert.equal(zoomStep(key("+", { ctrlKey: true }), false), 1);
  assert.equal(zoomStep({ ...key("", { ctrlKey: true }), code: "NumpadAdd" }, false), 1);
  assert.equal(zoomStep(key("-", { ctrlKey: true }), false), -1);
  assert.equal(zoomStep({ ...key("", { ctrlKey: true }), code: "NumpadSubtract" }, false), -1);
  assert.equal(zoomStep(key("0", { ctrlKey: true }), false), 0);
  assert.equal(zoomStep(key("="), false), null);
  assert.equal(zoomStep(key("=", { ctrlKey: true, altKey: true }), false), null);
  assert.equal(zoomStep(key("=", { ctrlKey: true, metaKey: true }), false), null);
  assert.equal(zoomStep(key("c", { ctrlKey: true }), false), null);
  assert.equal(zoomStep(key("=", { ctrlKey: true }), true), null);
  assert.equal(onMac({ platform: "MacIntel" }), true);
  assert.equal(onMac({ platform: "Win32", userAgent: "Mozilla/5.0 (Windows NT 10.0)" }), false);
  assert.equal(sizeLabel(100), "100% (default)");
  assert.equal(sizeLabel(150), "150%");
});

test("Reduce motion marks the window, and following the computer removes the mark", () => {
  const rootEl = window.document.documentElement;
  assert.equal(applyMotion("reduce", rootEl), "reduce");
  assert.equal(rootEl.dataset.motion, "reduce");
  assert.equal(motionOn(rootEl), "reduce");
  assert.equal(applyMotion("anything else", rootEl), "system");
  assert.equal(rootEl.dataset.motion, undefined);
});

test("Ctrl+= and Ctrl+0 step the kept size and say it; other keys are left alone", async () => {
  const kept = mockHome(125);
  const node = await render(React.createElement(TextSizeKeys));
  const press = async (k: string, ctrlKey = true) => {
    const e = new window.KeyboardEvent("keydown", { key: k, ctrlKey, bubbles: true, cancelable: true });
    await React.act(async () => { window.dispatchEvent(e); });
    await React.act(async () => {});
    return e;
  };
  const e = await press("=");
  assert.ok(e.defaultPrevented);
  assert.equal(node.querySelector("[data-text-size-said]")?.textContent, "Text size 150%");
  await press("0");
  assert.equal(node.querySelector("[data-text-size-said]")?.textContent, "Text size 100%");
  assert.equal((await press("c")).defaultPrevented, false);
  assert.equal((await press("=", false)).defaultPrevented, false);
  assert.deepEqual(kept.calls, ["step 1", "step 0"]);
});

test("Settings → Home → Accessibility offers the sizes, keeps a choice, follows the menu and reduces motion", async () => {
  const kept = mockHome(110);
  const node = await render(React.createElement(React.Fragment, null,
    React.createElement(AccessibilitySettings), React.createElement(TextSizeKeys)));
  const radios = Array.from(node.querySelectorAll<HTMLInputElement>('input[name="text-size"]'));
  assert.deepEqual(radios.map((r) => Number(r.value)), SIZES);
  assert.equal(radios.find((r) => r.checked)?.value, "110");
  assert.ok(node.querySelector('[role="radiogroup"][aria-label="Text size"]'));

  await React.act(async () => { radios.find((r) => r.value === "175")!.click(); });
  await React.act(async () => {});
  assert.equal(kept.size, 175);
  assert.equal(node.querySelector<HTMLInputElement>('input[name="text-size"]:checked')?.value, "175");
  assert.equal(node.querySelector("[data-text-size-said]")?.textContent, "Text size 175%");

  // The macOS View menu steps it in Go, which tells the page.
  await React.act(async () => { emit("accessibility:text-size", 200); });
  assert.equal(node.querySelector<HTMLInputElement>('input[name="text-size"]:checked')?.value, "200");

  const reduce = node.querySelector<HTMLInputElement>('input[name="motion"][value="reduce"]')!;
  await React.act(async () => { reduce.click(); });
  await React.act(async () => {});
  assert.equal(window.document.documentElement.dataset.motion, "reduce");
  assert.ok(kept.calls.includes("motion reduce"));
});
