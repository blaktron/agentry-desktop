// Managed settings as Settings words them: the landing names each
// organization and its version, and Unsafe Mode's locked switch says who
// manages it and why.
import { test } from "node:test";
import assert from "node:assert/strict";
import { managedLine, unsafeLockBadge, unsafeLockLines, unsafeLockStatus } from "../src/managed.ts";

test("the landing names each organization with its version, and nothing when none apply", () => {
  assert.equal(managedLine([]), null);
  assert.equal(managedLine(null), null);
  assert.equal(managedLine([{ summary: "managed by acme (version 3)" }]), "Some settings are managed by acme (version 3).");
  assert.equal(
    managedLine([{ summary: "managed by acme (version 3)" }, { summary: "managed by globex (version 1)" }]),
    "Some settings are managed by acme (version 3) and managed by globex (version 1).",
  );
});

test("the locked switch names the organization and the reason", () => {
  const lock = { managedBy: "acme", reason: "Unsafe Mode is forbidden by acme's managed settings (run.unsafeMode)", savedOn: false };
  assert.equal(unsafeLockBadge(lock), "Managed by acme");
  assert.deepEqual(unsafeLockLines(lock), ["Unsafe Mode is forbidden by acme's managed settings (run.unsafeMode)."]);
  assert.equal(unsafeLockStatus(lock), "Unsafe Mode is off, managed by acme.");
});

test("a switch that was on says it is held off, and comes back unless kept off", () => {
  const lines = unsafeLockLines({ managedBy: "acme", reason: "Unsafe Mode is forbidden by acme's managed settings (run.unsafeMode)", savedOn: true });
  assert.equal(lines.length, 2);
  assert.match(lines[1], /stays off while these managed settings forbid it/);
  assert.match(lines[1], /no run on this computer uses it/);
});

test("managed settings that cannot be verified lock the switch with no organization", () => {
  const lock = { reason: "Unsafe Mode is refused while the managed settings held for https://r.example cannot be verified (bad signature).", savedOn: false };
  assert.equal(unsafeLockBadge(lock), "Locked");
  assert.equal(unsafeLockStatus(lock), "Unsafe Mode is off and locked.");
  assert.equal(unsafeLockLines(lock)[0], lock.reason);
});
