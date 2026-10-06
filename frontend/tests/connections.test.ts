// The run setup's sign-ins (agentry-desktop#666): what the page says of a
// connection, and what it holds Run for.
import { test } from "node:test";
import assert from "node:assert/strict";
import { connectionBlockers, connectionLine, pastedProblem, signInRecap, type ServerConn } from "../src/connections.ts";
import { needList, signInNeed } from "../src/runSetup.ts";

const gmail = "https://gmailmcp.googleapis.com/mcp/v1";
const none: ServerConn = { server: gmail, what: gmail, scopes: ["https://www.googleapis.com/auth/gmail.readonly"], state: "none" };
const connected: ServerConn = { ...none, state: "connected", issuer: "https://accounts.google.com", client: "own", createdAt: "2026-10-06T12:00:00Z" };

test("a server not connected holds Run on this computer, by name, until it is", () => {
  assert.deepEqual(connectionBlockers([none], false), [`Connect ${gmail} on Connect tools.`]);
  assert.deepEqual(connectionBlockers([connected], false), []);
  assert.deepEqual(connectionBlockers([{ ...connected, state: "broken", broken: "invalid_grant" }], false), [`Connect ${gmail} on Connect tools.`]);
  assert.deepEqual(connectionBlockers(null, false), [], "a read that did not answer holds nothing: the engine still refuses by name");
});

test("In Agentry any server that signs in holds Run, connected here or not", () => {
  for (const c of [none, connected]) {
    const b = connectionBlockers([c], true);
    assert.equal(b.length, 1);
    assert.match(b[0], /In Agentry cannot sign in/);
    assert.match(b[0], /this computer/);
  }
});

test("a package on the unsupported list is said with its alternative", () => {
  const bad: ServerConn = { server: "pypi:mcp-gsuite", what: "pypi:mcp-gsuite (signing in at https://accounts.google.com)", scopes: [], state: "none", unsupported: "pypi:workspace-mcp" };
  assert.match(connectionLine(bad), /workspace-mcp/);
  assert.match(connectionBlockers([bad], false)[0], /workspace-mcp/);
});

test("connected says where, as whom where the vendor said, and never claims an account it was not told", () => {
  const line = connectionLine(connected);
  assert.match(line, /^Connected at accounts\.google\.com, as your own OAuth client/);
  assert.match(line, /did not say which account/);
  assert.equal(connectionLine({ ...connected, label: "me@example.com" }), "Connected as me@example.com at accounts.google.com.");
  assert.match(connectionLine({ ...connected, state: "broken", broken: "invalid_grant" }), /no longer works \(invalid_grant\)\. Connect again\./);
  assert.match(connectionLine(none), /consent screen in your browser/);
});

test("the recap counts what is connected", () => {
  assert.equal(signInRecap([connected]), "connected");
  assert.equal(signInRecap([connected, none]), "1 of 2 connected");
  assert.equal(signInRecap([connected, connected]), "all 2 connected");
});

test("what to have ready names the sign-in only when there is one", () => {
  assert.equal(signInNeed(0), null);
  const one = needList({ intake: [], secrets: [], servers: 1, signIns: 1 });
  assert.ok(one.some((l) => /sign-in to the MCP server/.test(l)), one.join("\n"));
  const plain = needList({ intake: [], secrets: [], servers: 1 });
  assert.ok(!plain.some((l) => /sign-in/.test(l)));
});

test("a pasted address must be the redirect, whole", () => {
  assert.equal(pastedProblem("http://127.0.0.1:41234/callback?code=abc&state=xyz"), null);
  assert.match(pastedProblem("") ?? "", /whole address/);
  assert.match(pastedProblem("abc") ?? "", /not an address/);
  assert.match(pastedProblem("http://127.0.0.1:41234/callback") ?? "", /carries a code/);
  assert.match(pastedProblem("javascript:alert(1)?x") ?? "", /not the address/);
});
