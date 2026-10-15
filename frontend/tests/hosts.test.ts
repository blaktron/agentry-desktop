// Target hosts in the run setup: a host question is asked as user@host[:port]
// with a sign-in beside it (the saved one, a key file or a password, and a
// sudo password); a sign-in typed for one account never goes to another;
// the run is held until each server has an address the CLI reads and a way
// to sign in; and a run In Agentry never reaches one.
import { test } from "node:test";
import assert from "node:assert/strict";
import { emptyHostForm, formFor, givesSignIn, hostBlockers, hostNeed, hostRecap, isHostType, recentOffers, signInWords, signInsForRun, type HostForm, type TargetHost } from "../src/hosts.ts";
import { needList, whereFallback } from "../src/runSetup.ts";

const server = { id: "server", prompt: "The server to set up", type: "host", required: true };
const backup = { id: "backup", prompt: "The backup server", type: "host", required: false };
const text = { id: "focus", prompt: "What to look at", type: "text", required: false };
const ok = (answer: string, saved = false): TargetHost => {
  const [user, rest] = answer.split("@");
  const [host, port] = rest.split(":");
  return { answer, user, host, port: Number(port ?? 22), account: `host:${user}@${host}:${port ?? 22}`, saved, keychainOff: false };
};
const here = { inAgentry: false, scheduling: false };

test("only the host type asks for a target host", () => {
  assert.deepEqual(["host", "repository", "url", "text", "file", "secret", "choice"].map(isHostType), [true, false, false, false, false, false, false]);
});

test("what to have ready names the address, the sign-in and sudo, and says it runs here only", () => {
  assert.match(hostNeed(server), /^The server for “The server to set up”, as user@host/);
  assert.match(hostNeed(server), /private key file, or the account's password/);
  assert.match(hostNeed(server), /sudo password/);
  assert.match(hostNeed(server), /on this computer only/);
  assert.match(hostNeed(backup), /“The backup server” \(optional\)/);
  const list = needList({ intake: [server, text], secrets: [] });
  assert.ok(list.includes(hostNeed(server)));
  assert.ok(!list.some((l) => l.startsWith("Your answer to “The server")), "a host is not asked for as a plain answer");
});

test("a fresh form signs in with the saved sign-in where there is one, else a key", () => {
  assert.equal(emptyHostForm("host:a@h:22", true).way, "saved");
  assert.equal(emptyHostForm("host:a@h:22", false).way, "key");
});

test("a sign-in typed for one account is left behind when the address names another", () => {
  const typed: HostForm = { ...emptyHostForm("host:admin@a.example:22", false), way: "password", password: "pw" };
  const forms = { server: typed };
  assert.equal(formFor(forms, "server", ok("admin@a.example"))?.password, "pw");
  const other = formFor(forms, "server", ok("admin@b.example"));
  assert.equal(other?.password, "", "the password typed for a.example is not offered for b.example");
  assert.equal(formFor(forms, "server", { problem: "no", saved: false, keychainOff: false }), null);
  assert.equal(formFor(forms, "server", null), null);
});

test("a way to sign in is the saved one only when it is there, a chosen key, or a typed password", () => {
  const key = ok("admin@a.example");
  assert.equal(givesSignIn({ ...emptyHostForm(key.account!, false), way: "saved" }, key), false);
  assert.equal(givesSignIn({ ...emptyHostForm(key.account!, true), way: "saved" }, { ...key, saved: true }), true);
  assert.equal(givesSignIn({ ...emptyHostForm(key.account!, false), way: "key" }, key), false);
  assert.equal(givesSignIn({ ...emptyHostForm(key.account!, false), way: "key", keyFile: "/home/me/.ssh/id_ed25519" }, key), true);
  assert.equal(givesSignIn({ ...emptyHostForm(key.account!, false), way: "password", password: "pw" }, key), true);
});

test("Run is held for an address being checked, one the CLI refuses, and a host with no sign-in", () => {
  const values = { server: "admin@a.example" };
  assert.deepEqual(hostBlockers([server], values, { server: null }, {}, here), ["Checking the address for “The server to set up”."]);
  assert.deepEqual(hostBlockers([server], values, { server: { problem: "\"a b\" is not a host", saved: false, keychainOff: false } }, {}, here),
    ["Fix the address for “The server to set up”: \"a b\" is not a host."]);
  assert.match(hostBlockers([server], values, { server: ok("admin@a.example") }, {}, here)[0], /^Give a way to sign in to admin@a.example/);
  assert.deepEqual(hostBlockers([server], values, { server: ok("admin@a.example", true) }, {}, here), [], "the saved sign-in is a way in");
  const typed = { server: { ...emptyHostForm("host:admin@a.example:22", false), keyFile: "/k" } };
  assert.deepEqual(hostBlockers([server], values, { server: ok("admin@a.example") }, typed, here), []);
});

test("an optional host left empty still holds Run: the run reaches every server it declares", () => {
  assert.match(hostBlockers([backup], {}, {}, {}, here)[0], /^Answer: The backup server\. A run reaches every server/);
  assert.deepEqual(hostBlockers([server], {}, {}, {}, here), [], "a required one is the intake check's to name");
});

test("a run In Agentry never reaches a server; a schedule needs the keychain", () => {
  assert.match(hostBlockers([server], { server: "admin@a.example" }, { server: ok("admin@a.example", true) }, {}, { inAgentry: true, scheduling: false })[0], /never reaches a server you name/);
  const off = { ...ok("admin@a.example", false), keychainOff: true };
  const typed = { server: { ...emptyHostForm(off.account!, false), way: "password" as const, password: "pw" } };
  assert.deepEqual(hostBlockers([server], { server: "admin@a.example" }, { server: off }, typed, here), []);
  assert.match(hostBlockers([server], { server: "admin@a.example" }, { server: off }, typed, { inAgentry: false, scheduling: true })[0], /keychain, which is off/);
});

test("the run is sent each answered host's sign-in, and nothing for the saved one but sudo and the save", () => {
  const a = ok("admin@a.example"), b = ok("root@b.example:2222", true);
  const forms = {
    server: { ...emptyHostForm(a.account!, false), way: "key" as const, keyFile: " /home/me/.ssh/id_ed25519 ", passphrase: "pp", password: "left over", save: true },
    backup: { ...emptyHostForm(b.account!, true), sudo: "s" },
  };
  assert.deepEqual(signInsForRun([server, backup, text], { server: "admin@a.example", backup: "root@b.example:2222" }, { server: a, backup: b }, forms), {
    server: { keyFile: "/home/me/.ssh/id_ed25519", passphrase: "pp", save: true },
    backup: { sudo: "s" },
  });
  // A form typed for another account sends nothing; an unanswered question
  // sends nothing; a saved sign-in with nothing typed sends nothing.
  assert.deepEqual(signInsForRun([server], { server: "admin@c.example" }, { server: ok("admin@c.example") }, forms), {});
  assert.deepEqual(signInsForRun([server], {}, { server: a }, forms), {});
  assert.deepEqual(signInsForRun([backup], { backup: "root@b.example:2222" }, { backup: b }, { backup: emptyHostForm(b.account!, true) }), {});
  // The keychain off: no save is asked for.
  const off = { ...a, keychainOff: true };
  assert.deepEqual(signInsForRun([server], { server: "admin@a.example" }, { server: off }, { server: { ...forms.server } }).server.save, undefined);
});

test("Review says where the sign-in comes from, and never a secret", () => {
  const a = ok("admin@a.example");
  const pw: HostForm = { ...emptyHostForm(a.account!, false), way: "password", password: "hunter22", sudo: "s3cret" };
  const said = hostRecap("admin@a.example", a, pw);
  assert.equal(said, "admin@a.example, with the password typed here; a sudo password is given");
  assert.ok(!said.includes("hunter22") && !said.includes("s3cret"));
  assert.equal(hostRecap("admin@a.example", a, { ...pw, save: true }), "admin@a.example, with the password typed here, saved on this computer when the run starts; a sudo password is given");
  assert.equal(hostRecap("admin@a.example", a, pw, true), "admin@a.example, with the password typed here, kept in the system keychain for the schedule's runs; a sudo password is given");
  const key: HostForm = { ...emptyHostForm(a.account!, false), keyFile: "/k", passphrase: "x" };
  assert.equal(signInWords(a, key), "with the key file /k and its passphrase; no sudo password");
  assert.equal(signInWords({ ...a, saved: true }, emptyHostForm(a.account!, true)), "with the sign-in saved on this computer");
  assert.equal(hostRecap("", a, pw), "no answer");
  assert.equal(hostRecap("admin@a.example", null, pw), "admin@a.example (being checked)");
});

test("recent hosts offered are the others, newest first, at most five", () => {
  const recent = ["a@1", "b@2", "c@3", "d@4", "e@5", "f@6"];
  assert.deepEqual(recentOffers(recent, "b@2"), ["a@1", "c@3", "d@4", "e@5", "f@6"]);
  assert.deepEqual(recentOffers(recent, ""), ["a@1", "b@2", "c@3", "d@4", "e@5"]);
});

test("Run defaults' In Agentry says why a QuickAgent with a target host runs here", () => {
  assert.equal(
    whereFallback("agentry", { offered: false, available: false, signedIn: true, targetHosts: true }, false),
    "Your Run defaults say In Agentry, but it works a server you name over SSH, which only a run on this computer does, so this run starts on this computer.",
  );
});
