// The QuickAgents an organization gives its members: the Library's section,
// the required badge and the refused Uninstall, the heading over the first
// run's contract, and the run setup's preset (a default, or fixed when
// locked).
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  answerLocked, assignmentAction, assignmentLine, contractAsk, contractHeading, fromOrganization, groupByOrganization,
  placeLockedOut, presetAnswers, presetLine, presetNumber, requiredBadge, requiredFor, uninstallExplanation, type Assignment,
} from "../src/assignments.ts";

const a = (over: Partial<Assignment>): Assignment => ({
  ref: "acme/one", version: "latest", selector: "acme/one", mode: "offered", required: false, organization: "acme",
  registry: "https://r.example", installed: false, ...over,
});

test("the section is headed by the organization, required ones first", () => {
  const groups = groupByOrganization([
    a({ ref: "acme/menu" }),
    a({ ref: "acme/one", mode: "required", required: true }),
    a({ ref: "globex/two", organization: "globex" }),
  ]);
  assert.deepEqual(groups.map((g) => g.organization), ["acme", "globex"]);
  assert.deepEqual(groups[0].items.map((x) => x.ref), ["acme/one", "acme/menu"]);
  assert.equal(fromOrganization("acme"), "From acme");
  assert.deepEqual(groupByOrganization(null), []);
});

test("an offered QuickAgent that is not installed offers Install; an installed one opens", () => {
  assert.equal(assignmentAction(a({})), "install");
  assert.equal(assignmentAction(a({ installed: true })), "open");
  // A required one that is not installed yet is tried again, never installed
  // as the member's own choice.
  assert.equal(assignmentAction(a({ required: true })), "retry");
});

test("the row says where each stands, and why a required install did not happen", () => {
  assert.equal(assignmentLine(a({})), "Offered at the newest version.");
  assert.equal(assignmentLine(a({ version: "1.2.0", installed: true, installedVersion: "1.2.0" })), "Offered at version 1.2.0 (1.2.0 is installed).");
  assert.equal(assignmentLine(a({ required: true, installed: true, installedVersion: "1.4.0" })), "Required, kept at the newest version (1.4.0 is installed).");
  assert.match(assignmentLine(a({ required: true })), /installs the next time the settings are read/);
  assert.equal(assignmentLine(a({ required: true, problem: "acme/one, required by acme, was not installed: digest mismatch" })), "acme/one, required by acme, was not installed: digest mismatch");
});

test("a pinned version that moved reads as the pin, with what is installed", () => {
  assert.equal(
    assignmentLine(a({ version: "1.3.0", required: true, installed: true, installedVersion: "1.2.0" })),
    "Required, kept at version 1.3.0 (1.2.0 is installed).",
  );
});

test("a required QuickAgent carries the badge and Uninstall says why it is off", () => {
  const list = [a({ required: true, installed: true }), a({ ref: "acme/menu" })];
  const req = requiredFor(list, "acme", "one");
  assert.ok(req);
  assert.equal(requiredBadge(req.organization), "Required by acme");
  assert.match(uninstallExplanation(req), /^Required by acme: it stays installed while their managed settings require it\.$/);
  assert.equal(requiredFor(list, "acme", "menu"), null);
  assert.equal(requiredFor(list, "other", "one"), null);
  assert.equal(requiredFor(undefined, "acme", "one"), null);
});

test("the first run of a required install heads its contract with the organization", () => {
  assert.equal(contractHeading("acme"), "Required by acme");
  assert.equal(contractHeading(undefined), "What it may reach");
  assert.equal(contractHeading(""), "What it may reach");
  assert.match(contractAsk("acme"), /acme installed this for you/);
  assert.match(contractAsk("acme"), /asks you to accept what it may do/);
  assert.equal(contractAsk(undefined), "");
});

test("a preset's numbers are defaults the member may change, or fixed when locked", () => {
  assert.deepEqual(presetNumber("", 300, false), { value: "300", disabled: false });
  assert.deepEqual(presetNumber("60", 300, false), { value: "60", disabled: false });
  assert.deepEqual(presetNumber("60", 300, true), { value: "300", disabled: true });
  assert.deepEqual(presetNumber("", 300, true), { value: "300", disabled: true });
  assert.deepEqual(presetNumber("", undefined, true), { value: "", disabled: false });
  assert.deepEqual(presetNumber("7", undefined, false), { value: "7", disabled: false });
});

test("a preset's answers fill what is blank, and a locked one fixes them", () => {
  const open = { organization: "acme", locked: false, inputs: { region: "eu" } };
  assert.deepEqual(presetAnswers({}, open), { region: "eu" });
  assert.deepEqual(presetAnswers({ region: "  " }, open), { region: "eu" });
  assert.deepEqual(presetAnswers({ region: "us", other: "x" }, open), { region: "us", other: "x" });
  const locked = { ...open, locked: true };
  assert.deepEqual(presetAnswers({ region: "us", other: "x" }, locked), { region: "eu", other: "x" });
  assert.equal(answerLocked(locked, "region"), true);
  assert.equal(answerLocked(locked, "other"), false);
  assert.equal(answerLocked(open, "region"), false);
  assert.equal(answerLocked(null, "region"), false);
  const typed = { x: "1" };
  assert.equal(presetAnswers(typed, null), typed);
});

test("a locked preset keeps runs to its place", () => {
  const p = { organization: "acme", locked: true, where: "hosted" };
  assert.equal(placeLockedOut(p, "local"), true);
  assert.equal(placeLockedOut(p, "runhost"), true);
  assert.equal(placeLockedOut(p, "hosted"), false);
  assert.equal(placeLockedOut({ ...p, locked: false }, "local"), false);
  assert.equal(placeLockedOut({ organization: "acme", locked: true }, "local"), false);
  assert.equal(placeLockedOut(null, "local"), false);
});

test("the preset's note names the organization and what it fixes or suggests", () => {
  const p = { organization: "acme", locked: true, timeoutSeconds: 300, maxTurns: 12, where: "hosted", inputs: { region: "eu" } };
  assert.equal(presetLine(p), "acme fixes a timeout of 300 seconds, 12 turns, runs on In Agentry and an answer for its runs.");
  assert.equal(presetLine({ organization: "acme", locked: false, timeoutSeconds: 60 }), "acme suggests a timeout of 60 seconds for its runs.");
  assert.equal(presetLine({ organization: "acme", locked: false }), "");
  assert.equal(presetLine(null), "");
});

test("a preset model is the Model step's pick, managed when locked and suggested when not", async () => {
  const { presetModelRecap, presetModelStep } = await import("../src/assignments.ts");
  const sonnet = { provider: "anthropic", model: "claude-sonnet-5-5", savedId: "m2", savedName: "Sonnet" };
  const p = (over: Partial<NonNullable<Parameters<typeof presetModelStep>[0]>>) => ({ organization: "acme", locked: false, ...over });

  assert.equal(presetModelStep(null, ""), null);
  assert.equal(presetModelStep(p({}), ""), null);

  const locked = presetModelStep(p({ locked: true, model: sonnet }), "");
  assert.deepEqual({ pick: locked?.pick, locked: locked?.locked, blocker: locked?.blocker }, { pick: "m2", locked: true, blocker: "" });
  assert.match(locked!.line, /^Managed by acme: .*Sonnet \(anthropic\/claude-sonnet-5-5\)/);

  const missing = presetModelStep(p({ locked: true, model: { provider: "anthropic", model: "claude-sonnet-5-5" } }), "");
  assert.equal(missing?.locked, true);
  assert.match(missing!.line, /no saved model on this computer is that model\. Save it in Settings → Models and keys first\./);
  assert.match(missing!.blocker, /^No saved model is anthropic\/claude-sonnet-5-5, the model acme fixes/);

  const suggested = presetModelStep(p({ model: sonnet }), "");
  assert.deepEqual({ pick: suggested?.pick, locked: suggested?.locked, back: suggested?.back }, { pick: "m2", locked: false, back: false });
  assert.match(suggested!.line, /^Set by acme for this QuickAgent: Sonnet/);
  const own = presetModelStep(p({ model: sonnet }), "m1");
  assert.deepEqual({ pick: own?.pick, back: own?.back, blocker: own?.blocker }, { pick: "m1", back: true, blocker: "" });

  assert.deepEqual(presetModelRecap(p({ locked: true, model: sonnet }), locked, "m2"), { model: "", by: "managed by acme" });
  assert.deepEqual(presetModelRecap(p({ model: sonnet }), suggested, "m2"), { model: "", by: "set by acme" });
  assert.deepEqual(presetModelRecap(p({ model: sonnet }), own, "m1"), { model: "", by: "" });
  const unsaved = { provider: "anthropic", model: "claude-sonnet-5-5" };
  assert.deepEqual(presetModelRecap(p({ locked: true, model: unsaved }), missing, ""),
    { model: "anthropic/claude-sonnet-5-5", by: "managed by acme, and no saved model is it" });

  const none = presetModelStep(p({ model: { provider: "anthropic", model: "claude-sonnet-5-5" } }), "");
  assert.deepEqual({ pick: none?.pick, blocker: none?.blocker }, { pick: "", blocker: "" });
  assert.match(none!.line, /your own choice/);

  const refused = { ...sonnet, savedId: "", refusal: "Running on anthropic is not allowed by acme's managed settings" };
  assert.match(presetModelStep(p({ model: refused }), "")!.blocker, /not allowed/);
  assert.equal(presetModelStep(p({ model: refused }), "m1")!.blocker, "");
  assert.match(presetModelStep(p({ locked: true, model: refused }), "m1")!.blocker, /not allowed/);
});

test("an assignment's row names the preset model", () => {
  assert.match(presetLine({ organization: "acme", locked: true, model: { provider: "anthropic", model: "claude-sonnet-5-5" } }),
    /acme fixes anthropic\/claude-sonnet-5-5 as the model for its runs/);
});
