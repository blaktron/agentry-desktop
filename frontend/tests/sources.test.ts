// Input sources in the run setup (plan input-sources.md §5, D3, D5, D13;
// agentry-desktop#794): a repository or url question is asked in the run's
// own flow with an optional token beside it; a saved key for the host is
// used unless another is typed; a schedule reads only the saved key; a run
// In Agentry does not take a source yet; and a token typed for a run goes
// with that run alone.
import { test } from "node:test";
import assert from "node:assert/strict";
import { agentryKeyHosts, bindTypedTokens, isSourceType, joinRepository, shownToken, sourceBlockers, sourceNeed, sourceRecap, splitRepository, tokenFieldLabel, tokenLine, tokensForRun, typedTokens, type SourceKey } from "../src/sources.ts";
import { needList } from "../src/runSetup.ts";

const repo = { id: "codebase", prompt: "The repository to audit", type: "repository", required: true };
const file = { id: "report", prompt: "The report to read", type: "url", required: false };
const text = { id: "focus", prompt: "What to look at", type: "text", required: false };
const ok = (host: string, saved = false): SourceKey => ({ host, saved, keychainOff: false });
const here = { inAgentry: false, scheduling: false };

test("the source types are repository and url, and nothing else", () => {
  assert.deepEqual(["repository", "url", "text", "file", "secret", "choice"].map(isSourceType), [true, true, false, false, false, false]);
});

test("a repository answer is its address with the ref after #, and round-trips", () => {
  assert.deepEqual(splitRepository("https://github.com/acme/app#v1.2.0"), { url: "https://github.com/acme/app", ref: "v1.2.0" });
  assert.deepEqual(splitRepository("https://github.com/acme/app"), { url: "https://github.com/acme/app", ref: "" });
  assert.equal(joinRepository("https://github.com/acme/app", " main "), "https://github.com/acme/app#main");
  assert.equal(joinRepository("https://github.com/acme/app", "  "), "https://github.com/acme/app");
  // A ref typed before the address is kept while the address is typed.
  assert.equal(joinRepository("", "main"), "#main");
  assert.deepEqual(splitRepository("#main"), { url: "", ref: "main" });
});

test("what to have ready names the address and the token, required or optional", () => {
  assert.match(sourceNeed(repo), /^The https address of the repository for “The repository to audit”, and the branch, tag or commit/);
  assert.match(sourceNeed(repo), /private repository needs an access token/);
  assert.match(sourceNeed(file), /“The report to read” \(optional\)/);
  const list = needList({ intake: [repo, file, text], secrets: [] });
  assert.ok(list.includes(sourceNeed(repo)) && list.includes(sourceNeed(file)));
  assert.ok(list.includes("Your answer to “What to look at” (optional)."));
  assert.ok(!list.some((l) => l.startsWith("Your answer to “The repository")), "a source is not asked for as a plain answer");
});

test("the token line says which token the run uses", () => {
  // Before the address is checked, or when it is refused, the line still
  // says what a token is for (agentry-desktop#869): the field is there.
  assert.match(tokenLine(null, "", true), /A public repository needs no token; a private one needs an access token/);
  assert.match(tokenLine({ problem: "the repository address must be https", saved: false, keychainOff: false }, "", true), /a private one needs an access token/);
  assert.match(tokenLine(null, "", false), /A public file needs no token/);
  assert.match(tokenLine(null, "tok", true), /used for the repository's host once its address is checked/);
  assert.match(tokenLine(null, "", true, true), /In Agentry/);
  assert.match(tokenLine(ok("github.com"), "", true), /No key is saved for github\.com\. A public repository needs none/);
  assert.match(tokenLine(ok("github.com", true), "", true), /uses the key saved on this computer for github\.com/);
  assert.match(tokenLine(ok("github.com", true), "tok", true), /uses the token typed here, not the key saved for github\.com/);
  assert.match(tokenLine(ok("github.com"), "tok", true), /not saved unless you save it for github\.com/);
  assert.match(tokenLine({ host: "github.com", saved: false, keychainOff: true }, "", true), /keychain is off/);
});

test("Review and start shows the address, the ref and where the token comes from, never the token", () => {
  const typed = "ghp-secret-typed-token";
  assert.equal(sourceRecap(repo, "https://github.com/acme/app#v2", ok("github.com", true), ""), "https://github.com/acme/app, at v2, with the key saved for github.com");
  assert.equal(sourceRecap(repo, "https://github.com/acme/app", ok("github.com"), ""), "https://github.com/acme/app, its default branch, with no access token");
  const r = sourceRecap(repo, "https://github.com/acme/app", ok("github.com", true), typed);
  assert.equal(r, "https://github.com/acme/app, its default branch, with the access token typed for this run");
  assert.ok(!r.includes(typed));
  // A url's query is shown as Go shows it.
  assert.equal(sourceRecap(file, "https://files.example.com/r.pdf?sig=abc", { host: "files.example.com", shown: "https://files.example.com/r.pdf?…", saved: false, keychainOff: false }, ""), "https://files.example.com/r.pdf?…, with no access token");
  assert.equal(sourceRecap(file, " ", null, ""), "no answer");
});

test("an unanswered source holds nothing here; an unchecked or refused address holds Run", () => {
  assert.deepEqual(sourceBlockers([repo, file], {}, {}, {}, here), []);
  assert.deepEqual(sourceBlockers([repo], { codebase: "https://github.com/acme/app" }, { codebase: null }, {}, here), ["Checking the address for “The repository to audit”."]);
  assert.deepEqual(
    sourceBlockers([repo], { codebase: "http://github.com/acme/app" }, { codebase: { problem: 'the repository address must be https, not "http"', saved: false, keychainOff: false } }, {}, here),
    ['Fix the address for “The repository to audit”: the repository address must be https, not "http".'],
  );
  assert.deepEqual(sourceBlockers([repo], { codebase: "https://github.com/acme/app" }, { codebase: ok("github.com") }, { codebase: "tok" }, here), []);
});

test("a run In Agentry takes a source, its token sent with the start (agentry-desktop#833)", () => {
  const v = { codebase: "https://github.com/acme/app" };
  const there = { ...here, inAgentry: true };
  assert.deepEqual(sourceBlockers([repo], v, { codebase: ok("github.com") }, {}, there), []);
  assert.deepEqual(sourceBlockers([repo], v, { codebase: ok("github.com", true) }, { codebase: "tok" }, there), []);
  // An address the CLI refuses still holds it.
  assert.deepEqual(
    sourceBlockers([repo], { codebase: "http://github.com/acme/app" }, { codebase: { problem: "must be https", saved: false, keychainOff: false } }, {}, there),
    ["Fix the address for “The repository to audit”: must be https."],
  );
  assert.match(tokenLine(ok("github.com"), "tok", true, true), /sent the token typed here: Agentry holds it for this run alone/);
  assert.match(tokenLine(ok("github.com", true), "", true, true), /sent the key saved on this computer for github.com/);
  assert.match(tokenLine(ok("github.com"), "", true, true), /uses the key saved in Agentry for github.com, if there is one/);
  assert.equal(sourceRecap(repo, "https://github.com/acme/app", ok("github.com", true), "", true),
    "https://github.com/acme/app, its default branch, with the key saved on this computer for github.com, sent to Agentry for this run alone");
});

test("a schedule In Agentry saves a source's token there, with leave (agentry-enterprise#532)", () => {
  const sched = { ...here, inAgentry: true, scheduling: true };
  const v = { codebase: "https://github.com/acme/app" };
  const hold = ["A schedule In Agentry fetches with the key Agentry keeps for github.com: tick \u201cSave in Agentry\u201d to keep the token there, or schedule it on this computer."];
  assert.deepEqual(sourceBlockers([repo], v, { codebase: ok("github.com") }, { codebase: "tok" }, sched), hold);
  assert.deepEqual(sourceBlockers([repo], v, { codebase: ok("github.com", true) }, {}, sched), hold);
  assert.deepEqual(sourceBlockers([repo], v, { codebase: ok("github.com", true) }, {}, { ...sched, saveInAgentry: true }), []);
  assert.deepEqual(sourceBlockers([repo], v, { codebase: ok("github.com") }, {}, sched), [], "a public repository, or one Agentry already keeps a key for");
  assert.deepEqual(agentryKeyHosts([repo, file], { ...v, report: "https://files.example.com/r.pdf" }, { codebase: ok("github.com", true), report: ok("files.example.com") }, {}), ["github.com"]);
});

test("a schedule reads the saved key: a typed token must be saved, signed out or not", () => {
  const sched = { ...here, scheduling: true };
  const v = { codebase: "https://github.com/acme/app" };
  assert.deepEqual(sourceBlockers([repo], v, { codebase: ok("github.com") }, { codebase: "tok" }, sched), ["A schedule uses the key saved for github.com: save the token for github.com, or clear it."]);
  // Signed out too: the saved key is sealed beside the schedule (agentry-cli#854).
  assert.deepEqual(sourceBlockers([repo], v, { codebase: ok("github.com", true) }, {}, sched), []);
  assert.deepEqual(sourceBlockers([repo], v, { codebase: ok("github.com") }, {}, sched), [], "a public repository schedules with no key");
});

test("a run is given only the tokens typed for its answered source questions", () => {
  const values = { codebase: "https://github.com/acme/app", report: "", focus: "x" };
  assert.deepEqual(tokensForRun([repo, file, text], values, { codebase: " tok ", report: "unused", focus: "never" }), { codebase: "tok" });
  assert.deepEqual(tokensForRun([repo], values, { codebase: "  " }), {});
});

test("a token typed for one host is neither shown nor sent once the address names another", () => {
  const typed = { codebase: { host: "github.com", token: "tok" } };
  assert.deepEqual(typedTokens(typed, { codebase: ok("github.com") }), { codebase: "tok" });
  assert.deepEqual(typedTokens(typed, { codebase: ok("gitlab.example.org") }), {}, "edited to another host");
  assert.deepEqual(typedTokens(typed, { codebase: null }), {}, "while the new address is checked");
  assert.deepEqual(typedTokens(typed, { codebase: { problem: "bad", saved: false, keychainOff: false } }), {});
  // Back on the host it was typed for, it applies again.
  assert.deepEqual(typedTokens(typed, { codebase: ok("github.com", true) }), { codebase: "tok" });
});

test("the token field is labelled before the host is known, and names it after (agentry-desktop#869)", () => {
  assert.equal(tokenFieldLabel(null, true), "Access token, for a private repository");
  assert.equal(tokenFieldLabel({ problem: "the repository address must be https", saved: false, keychainOff: false }, true), "Access token, for a private repository");
  assert.equal(tokenFieldLabel(null, false), "Access token, for a file behind a sign-in");
  assert.equal(tokenFieldLabel(ok("github.com"), true), "Access token for github.com");
});

test("a token typed before the host is known binds to the first host, and is shown until then (agentry-desktop#869)", () => {
  const early = { repo: { host: "", token: "tok" } };
  // Unchecked: shown in the field, sent nowhere.
  assert.equal(shownToken(early.repo, null), "tok");
  assert.deepEqual(typedTokens(early, { repo: null }), {});
  assert.equal(bindTypedTokens(early, { repo: null }), early, "nothing to bind returns the same object");
  // Refused: still unbound.
  assert.equal(bindTypedTokens(early, { repo: { problem: "the repository address must be https", saved: false, keychainOff: false } }), early);
  // The first host the address resolves to takes it.
  const bound = bindTypedTokens(early, { repo: ok("github.com") });
  assert.deepEqual(bound, { repo: { host: "github.com", token: "tok" } });
  assert.deepEqual(typedTokens(bound, { repo: ok("github.com") }), { repo: "tok" });
  assert.equal(shownToken(bound.repo, ok("github.com")), "tok");
  // Then edited to another host: left behind, neither shown nor sent (D-079).
  assert.equal(bindTypedTokens(bound, { repo: ok("gitlab.com") }), bound, "a bound token never moves");
  assert.deepEqual(typedTokens(bound, { repo: ok("gitlab.com") }), {});
  assert.equal(shownToken(bound.repo, ok("gitlab.com")), "");
  assert.equal(shownToken(undefined, ok("github.com")), "");
});
