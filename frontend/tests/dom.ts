// A browser document for the tests that need one (the voice editor's,
// agentry-desktop#612): jsdom's window, its globals copied onto Node's, as a
// browser has them. Import it before anything that reads `document` at load.
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><head></head><body></body></html>", { url: "http://wails.localhost/", pretendToBeVisual: true });
const win = dom.window as unknown as Record<string, unknown>;
const g = globalThis as Record<string, unknown>;
for (const key of Object.getOwnPropertyNames(win)) {
  if (key in g) continue;
  g[key] = win[key];
}
g.window = win;
g.document = dom.window.document;
// Node has a navigator of its own; ProseMirror reads the browser's.
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });

export const window = dom.window;
