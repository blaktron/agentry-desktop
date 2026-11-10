// Managed settings as Settings words them: the landing names each
// organization and its version, and Unsafe Mode's locked switch says who
// manages it and why.
import { test } from "node:test";
import assert from "node:assert/strict";
import { MANAGED_REFUSAL, managedLine, managedRefusalHeading, unsafeLockBadge, unsafeLockLines, unsafeLockStatus, whereLeaves } from "../src/managed.ts";
import { inAgentryByDefault, whereFallback } from "../src/runSetup.ts";
import { hostedGapHeading } from "../src/scheduled.ts";

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

// Where runs may go, under a lock: the run setup disables a place the lock
// leaves out, as Settings does, rather than offering it and refusing the run.
const whereRow = (places: string[], state: "locked" | "default" = "locked") =>
  [{ id: "run.where", state, value: JSON.stringify(places), organization: "acme", applied: true }];

test("a lock on where runs may go leaves only its places", () => {
  const local = whereRow(["local"]);
  assert.equal(whereLeaves(local, "local"), true);
  assert.equal(whereLeaves(local, "hosted"), false);
  assert.equal(whereLeaves(local, "runhost"), false);
  assert.equal(whereLeaves(whereRow(["local", "hosted"]), "hosted"), true);
});

test("with no lock on where runs may go, every place is left", () => {
  assert.equal(whereLeaves(null, "hosted"), true);
  assert.equal(whereLeaves([], "runhost"), true);
  assert.equal(whereLeaves([{ id: "run.timeout", state: "locked", value: "900", organization: "acme", applied: true }], "hosted"), true);
});

test("the run setup does not pick In Agentry by default when a lock leaves it out, and says why", () => {
  const offer = { offered: true, available: true, signedIn: true };
  assert.equal(inAgentryByDefault("agentry", offer, false), true);
  assert.equal(inAgentryByDefault("agentry", offer, false, true), false);
  assert.equal(
    whereFallback("agentry", offer, false, true),
    "Runs are set to go In Agentry (Settings → Runs), but your organization's managed settings do not allow runs In Agentry, so this run starts on this computer.",
  );
  assert.equal(whereFallback("here", offer, false, true), null);
});

test("a refusal the managed settings gave is headed as theirs, not the registry's", () => {
  assert.equal(MANAGED_REFUSAL, "managed-settings");
  assert.equal(managedRefusalHeading("run"), "Your organization's managed settings do not allow this run.");
  assert.equal(managedRefusalHeading("session"), "Your organization's managed settings do not allow this session.");
  assert.equal(hostedGapHeading("managed-settings"), "Your organization's managed settings do not allow this schedule.");
  assert.doesNotMatch(hostedGapHeading("managed-settings"), /registry/);
});
