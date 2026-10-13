// The window's drop guard (agentry-desktop#644). A drop that no page takes
// would get the webview's default, which in a browser opens a dropped file
// or link in place of the app's document; the guard cancels it. Text
// dragged into a field or the voice editor still lands there, and a file
// lands nowhere.
import { window } from "./dom.ts";
import { afterEach, beforeEach, describe, test } from "node:test";
import assert from "node:assert/strict";

const { installDropGuard } = await import("../src/dropGuard.ts");

// jsdom has no DragEvent or DataTransfer: a plain event carrying a
// dataTransfer stand-in with the two fields the guard reads.
function drag(type: string, types: string[]) {
  const event = new window.Event(type, { bubbles: true, cancelable: true }) as Event & { dataTransfer: { types: string[]; dropEffect: string } };
  Object.defineProperty(event, "dataTransfer", { value: { types, dropEffect: "copy" } });
  return event;
}

let remove: () => void = () => {};
beforeEach(() => {
  document.body.innerHTML = `
    <main><p id="text">Plain page text</p><a id="link" href="#x">a link</a></main>
    <input id="field" /><textarea id="area"></textarea>
    <input id="readonly" readonly /><input id="checkbox" type="checkbox" />
    <div id="editor" contenteditable="true"><p id="inside">voice</p></div>`;
  remove = installDropGuard(window as unknown as Window);
});
afterEach(() => remove());

const FILE = ["Files"];
const TEXT = ["text/plain"];
const LINK = ["text/uri-list", "text/plain"];

describe("the window's drop guard", () => {
  for (const type of ["dragenter", "dragover", "drop"]) {
    test(`cancels a file's ${type} anywhere, a field and the editor included`, () => {
      for (const id of ["text", "link", "field", "area", "editor", "inside"]) {
        const event = drag(type, FILE);
        document.getElementById(id)!.dispatchEvent(event);
        assert.equal(event.defaultPrevented, true, `${type} on #${id}`);
        if (type !== "drop") assert.equal(event.dataTransfer.dropEffect, "none", `${type} on #${id} shows no drop`);
      }
    });

    test(`cancels text's or a link's ${type} where nothing edits`, () => {
      for (const types of [TEXT, LINK]) {
        for (const id of ["text", "link", "readonly", "checkbox"]) {
          const event = drag(type, types);
          document.getElementById(id)!.dispatchEvent(event);
          assert.equal(event.defaultPrevented, true, `${types.join("+")} ${type} on #${id}`);
        }
      }
    });

    test(`lets text's or a link's ${type} reach a field and the voice editor`, () => {
      for (const types of [TEXT, LINK]) {
        for (const id of ["field", "area", "editor", "inside"]) {
          const event = drag(type, types);
          document.getElementById(id)!.dispatchEvent(event);
          assert.equal(event.defaultPrevented, false, `${types.join("+")} ${type} on #${id}`);
          assert.equal(event.dataTransfer.dropEffect, "copy");
        }
      }
    });
  }

  test("cancels a drop with no data transfer at all, and one onto the window itself", () => {
    const bare = new window.Event("drop", { bubbles: true, cancelable: true });
    document.getElementById("field")!.dispatchEvent(bare);
    assert.equal(bare.defaultPrevented, true);
    const onWindow = drag("drop", TEXT);
    window.dispatchEvent(onWindow);
    assert.equal(onWindow.defaultPrevented, true);
  });

  test("runs before a page's own handlers, so none can let a file through", () => {
    let seen: boolean | null = null;
    document.getElementById("field")!.addEventListener("drop", (e) => (seen = e.defaultPrevented));
    document.getElementById("field")!.dispatchEvent(drag("drop", FILE));
    assert.equal(seen, true);
  });

  test("stops guarding once removed", () => {
    remove();
    const event = drag("drop", FILE);
    document.getElementById("text")!.dispatchEvent(event);
    assert.equal(event.defaultPrevented, false);
    remove = () => {};
  });
});

describe("the guard's place in the app", () => {
  test("is installed by main.tsx before the app renders", async () => {
    const { readFileSync } = await import("node:fs");
    const main = readFileSync(new URL("../src/main.tsx", import.meta.url), "utf8");
    const install = main.indexOf("installDropGuard(window)");
    assert.ok(install > 0, "main.tsx calls installDropGuard(window)");
    assert.ok(install < main.indexOf("root.render("), "before the first render");
  });
});
