// Page-wide rules in style.css. The webview's context menu stays hidden over
// a link and an image: the window toolkit's runtime hides it wherever
// --default-contextmenu is "hide", and style.css says so for both, so the
// menu that copies and pastes in a text field never offers to open a link
// or an image elsewhere. Reduce motion stills every animation.
import { window } from "./dom.ts";
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const css = readFileSync(new URL("../src/style.css", import.meta.url), "utf8");
const style = window.document.createElement("style");
style.textContent = css;
window.document.head.appendChild(style);
window.document.body.innerHTML = `
  <p>Text <a id="link" href="https://example.com/"><span id="inner">a link</span></a> <img id="image" alt="" src="data:,"></p>
  <input id="field"><textarea id="area"></textarea>`;

const menu = (id: string) => window.getComputedStyle(window.document.getElementById(id)!).getPropertyValue("--default-contextmenu").trim();

test("the context menu is hidden over a link, anything inside one, and an image", () => {
  assert.equal(menu("link"), "hide");
  assert.equal(menu("inner"), "hide");
  assert.equal(menu("image"), "hide");
});

test("the context menu is left to the runtime's own rule over a text field", () => {
  assert.equal(menu("field"), "");
  assert.equal(menu("area"), "");
});

test("Reduce motion stills animations and transitions on every element of the window", () => {
  const rules = Array.from(window.document.styleSheets).flatMap((sheet) => Array.from(sheet.cssRules)) as CSSStyleRule[];
  const rule = rules.find((r) => r.selectorText?.includes(':root[data-motion="reduce"] *'));
  assert.ok(rule, "no rule for Reduce motion");
  for (const sel of [':root[data-motion="reduce"] *', ':root[data-motion="reduce"] *::before', ':root[data-motion="reduce"] *::after']) {
    assert.ok(rule.selectorText.includes(sel), `the rule does not cover ${sel}`);
  }
  for (const prop of ["animation-duration", "transition-duration"]) {
    assert.equal(rule.style.getPropertyValue(prop), "0.01ms", prop);
    assert.equal(rule.style.getPropertyPriority(prop), "important", `${prop} is not important`);
  }
  assert.equal(rule.style.getPropertyValue("animation-iteration-count"), "1");
});
