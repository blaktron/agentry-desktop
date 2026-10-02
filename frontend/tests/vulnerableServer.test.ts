// A declared MCP server the registry's gate rates high (agentry-desktop#459,
// D-055): on this computer it runs only in Unsafe Mode, with the person's
// consent; In Agentry it has no way through; a server rated medium or low
// carries no refusal and asks for nothing.
import { test } from "node:test";
import assert from "node:assert/strict";
import { vulnerableServerStep } from "../src/runSetup.ts";

test("a server with no refusal asks for nothing", () => {
  for (const inAgentry of [false, true]) {
    for (const unsafeOn of [false, true]) {
      assert.deepEqual(vulnerableServerStep({ refused: false, inAgentry, unsafeOn, consented: false }), { offerConsent: false, blocker: null, send: false, showRefusal: false });
    }
  }
});

test("with Unsafe Mode off the consent is not offered, and Run is held with the way through", () => {
  const s = vulnerableServerStep({ refused: true, inAgentry: false, unsafeOn: false, consented: true });
  assert.equal(s.offerConsent, false);
  assert.equal(s.showRefusal, true, "the registry's refusal, with its way through, is shown");
  assert.equal(s.send, false, "a consent left over from an earlier setup is never sent with the switch off");
  assert.match(s.blocker ?? "", /only in Unsafe Mode/);
  assert.match(s.blocker ?? "", /Settings → Run defaults/);
});

test("in Unsafe Mode the consent is offered, holds Run until ticked, and is sent once ticked", () => {
  const unticked = vulnerableServerStep({ refused: true, inAgentry: false, unsafeOn: true, consented: false });
  assert.deepEqual(unticked, { offerConsent: true, send: false, showRefusal: false, blocker: "Tick “Allow vulnerable MCP servers for this run”." });
  const ticked = vulnerableServerStep({ refused: true, inAgentry: false, unsafeOn: true, consented: true });
  assert.deepEqual(ticked, { offerConsent: true, send: true, showRefusal: false, blocker: null });
});

test("a run In Agentry has no way through, Unsafe Mode or not", () => {
  for (const unsafeOn of [false, true]) {
    const s = vulnerableServerStep({ refused: true, inAgentry: true, unsafeOn, consented: true });
    assert.equal(s.offerConsent, false);
    assert.equal(s.send, false);
    assert.equal(s.showRefusal, false);
    assert.match(s.blocker ?? "", /In Agentry refuses/);
  }
});
