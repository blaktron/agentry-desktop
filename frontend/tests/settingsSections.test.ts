// Settings' eight sections, and where a link naming a section opens: today's
// sections as named, a former section's name at what holds it now.
import { test } from "node:test";
import assert from "node:assert/strict";
import { partsShown, SECTIONS, SETTINGS_FOR, settingsPlace } from "../src/settingsSections.ts";

test("the landing's eight sections, in order", () => {
  assert.deepEqual(SECTIONS.map((s) => s.title), ["Account", "Report design", "Models and keys", "Runs", "Connections", "Updates and installs", "Home", "Licences"]);
});

test("no part is named twice across the sections", () => {
  const ids = SECTIONS.flatMap((s) => s.parts.map((p) => p.id));
  assert.deepEqual(ids, [...new Set(ids)]);
});

test("Account links the profile's parts only while signed in", () => {
  const account = SECTIONS.find((s) => s.id === "account")!;
  assert.deepEqual(partsShown(account, false).map((p) => p.id), ["registries", "add-registry"]);
  assert.deepEqual(partsShown(account, true).map((p) => p.id), ["profile", "subscription", "account-voice", "registries", "add-registry"]);
});

test("a section of today's opens as named, with its part", () => {
  assert.deepEqual(settingsPlace("provider"), { page: "settings", section: "provider" });
  assert.deepEqual(settingsPlace("updates", "desktop"), { page: "settings", section: "updates", part: "desktop" });
  assert.deepEqual(settingsPlace("run-defaults", "unsafe"), { page: "settings", section: "run-defaults", part: "unsafe" });
});

test("a former section opens what holds it now", () => {
  assert.deepEqual(settingsPlace("profile"), { page: "settings", section: "account", part: "profile" });
  assert.deepEqual(settingsPlace("profile", "subscription"), { page: "settings", section: "account", part: "subscription" });
  assert.deepEqual(settingsPlace("registry"), { page: "settings", section: "account", part: "registries" });
  assert.deepEqual(settingsPlace("registry", "add-registry"), { page: "settings", section: "account", part: "add-registry" });
  assert.deepEqual(settingsPlace("coding-agents"), { page: "settings", section: "updates", part: "coding-tools" });
  assert.deepEqual(settingsPlace("runhosts", "runhosts"), { page: "settings", section: "run-defaults", part: "runhosts" });
  assert.deepEqual(settingsPlace("destinations"), { page: "settings", section: "connections", part: "destinations" });
  assert.deepEqual(settingsPlace("persistent-agents"), { page: "agents" });
  assert.deepEqual(settingsPlace("meeting-notes"), { page: "meetings" });
  assert.deepEqual(settingsPlace("your-computer", "using"), { page: "computer" });
});

test("a name Settings never had opens nothing", () => {
  assert.equal(settingsPlace("the keychain"), null);
  assert.equal(settingsPlace(undefined), null);
  assert.equal(settingsPlace(""), null);
});

test("every place that opens Settings names a section and a part it has", () => {
  for (const [name, at] of Object.entries(SETTINGS_FOR)) {
    const meta = SECTIONS.find((s) => s.id === at.section);
    assert.ok(meta, `${name}: ${at.section}`);
    const part = (at as { part?: string }).part;
    if (part) assert.ok(meta.parts.some((p) => p.id === part), `${name}: ${at.section} has no part ${part}`);
  }
});
