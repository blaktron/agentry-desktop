// A deep link's part is followed while its page fills in, and the person
// ends the follow.
import { window } from "./dom.ts";
import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { followPart, FOLLOW_OWN_SCROLL_MS, PERSON_TAKES_OVER } from "../src/followPart.ts";

// jsdom lays nothing out, so a scroll is counted, not measured.
let scrolls: string[] = [];
window.HTMLElement.prototype.scrollIntoView = function (this: HTMLElement) { scrolls.push(this.id); };

const tick = () => new Promise((r) => setTimeout(r, 0));
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
const add = (id: string, before?: Element | null) => {
  const el = document.createElement("section");
  el.id = id;
  document.body.insertBefore(el, before ?? null);
  return el;
};

beforeEach(() => {
  document.body.replaceChildren();
  scrolls = [];
});

test("a part already there is scrolled to at once", () => {
  add("settings-part-desktop");
  const stop = followPart("settings-part-desktop");
  stop();
  assert.deepEqual(scrolls, ["settings-part-desktop"]);
});

test("a part a section's own read renders later is scrolled to when it appears", async () => {
  const stop = followPart("settings-part-history");
  assert.deepEqual(scrolls, []);
  await sleep(20);
  add("settings-part-budgets");
  add("settings-part-history");
  await tick();
  stop();
  assert.ok(scrolls.includes("settings-part-history"));
});

test("a part pushed down by parts rendered above it is scrolled to again", async () => {
  const voice = add("settings-part-voice");
  const stop = followPart("settings-part-voice");
  assert.equal(scrolls.length, 1);
  add("settings-part-budgets", voice);
  await tick();
  stop();
  assert.equal(scrolls.length, 2);
});

for (const kind of PERSON_TAKES_OVER) {
  test(`the person's ${kind} ends the follow`, async () => {
    const stop = followPart("settings-part-history");
    window.dispatchEvent(new window.Event(kind));
    add("settings-part-history");
    await tick();
    stop();
    assert.deepEqual(scrolls, []);
  });
}

test("a scroll the follow did not just make, a scrollbar drag, ends the follow", async () => {
  const voice = add("settings-part-voice");
  const stop = followPart("settings-part-voice");
  await sleep(FOLLOW_OWN_SCROLL_MS + 50);
  document.body.dispatchEvent(new window.Event("scroll"));
  add("settings-part-budgets", voice);
  await tick();
  stop();
  assert.equal(scrolls.length, 1);
});

test("the follow's own scroll does not end it", async () => {
  const voice = add("settings-part-voice");
  const stop = followPart("settings-part-voice");
  document.body.dispatchEvent(new window.Event("scroll"));
  add("settings-part-budgets", voice);
  await tick();
  stop();
  assert.equal(scrolls.length, 2);
});

test("the follow ends when its time is up", async () => {
  const stop = followPart("settings-part-history", 10);
  await sleep(30);
  add("settings-part-history");
  await tick();
  stop();
  assert.deepEqual(scrolls, []);
});

test("stopped, it scrolls no more", async () => {
  const stop = followPart("settings-part-history");
  stop();
  add("settings-part-history");
  await tick();
  assert.deepEqual(scrolls, []);
});
