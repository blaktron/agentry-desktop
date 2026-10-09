// A remote MCP server and a package server's OAuth sign-in in the Builder
// (OAuth connections M11, agentry-desktop#668): the page's readings of the
// Go model's rules (internal/build MCPServerRow.Declared, Hosts), in the
// site's words (agentry-web lib/builder-form.ts networkReach).
import { test } from "node:test";
import assert from "node:assert/strict";
import { isRemote, reachRow, remoteScopes, serverHosts, serverKeys, serverRef, signInPhrase } from "../src/remoteServers.ts";
import { declaredKeys, optionalKeys } from "../src/requiredKeys.ts";

const linear = { source: "url", url: "https://mcp.linear.app/mcp", auth: "oauth", scopes: ["read"], tools: ["list_issues"], secrets: ["HIDDEN"], egress: ["hidden.example"] };
const github = { source: "url", url: "https://api.githubcopilot.com/mcp/", auth: "secret", secrets: ["GITHUB_TOKEN"], scopes: ["hidden"] };
const workspace = {
  source: "pypi", package: "workspace-mcp", version: "2.0.1", egress: ["gmail.googleapis.com"], env: [{ name: "MCP_SINGLE_USER_MODE" }],
  oauth: { issuer: "https://accounts.google.com", scopes: ["https://www.googleapis.com/auth/gmail.readonly"] },
};

test("a remote server is named by its address and reaches its own host only", () => {
  assert.equal(isRemote(linear), true);
  assert.equal(serverRef(linear), "https://mcp.linear.app/mcp");
  assert.equal(serverRef(workspace), "pypi:workspace-mcp@2.0.1");
  assert.deepEqual(serverHosts(linear), ["mcp.linear.app"]);
  assert.deepEqual(serverHosts({ source: "url", url: "not an address" }), []);
  assert.deepEqual(serverHosts(workspace), ["gmail.googleapis.com"]);
});

test("a remote server sends its key only with auth secret, and asks for scopes only with auth oauth", () => {
  assert.deepEqual(serverKeys(linear), []);
  assert.deepEqual(remoteScopes(linear), ["read"]);
  assert.deepEqual(serverKeys(github), ["GITHUB_TOKEN"]);
  assert.deepEqual(remoteScopes(github), []);
  assert.deepEqual(declaredKeys({ secrets: ["OWN"], mcpServers: [linear, github] }), ["OWN", "GITHUB_TOKEN"]);
  assert.deepEqual(optionalKeys({ mcpServers: [github], requiredSecrets: [] }), ["GITHUB_TOKEN"]);
});

test("the reach rows say where each server runs and how it signs in, in the site's words", () => {
  assert.deepEqual(reachRow(linear), {
    what: "https://mcp.linear.app/mcp",
    value: "runs at its vendor, reached at mcp.linear.app; tools list_issues; signs in with OAuth, asking for scopes read",
  });
  assert.deepEqual(reachRow(github), {
    what: "https://api.githubcopilot.com/mcp/",
    value: "runs at its vendor, reached at api.githubcopilot.com; all its tools; keys GITHUB_TOKEN; signs in with the key GITHUB_TOKEN",
  });
  assert.deepEqual(reachRow(workspace), {
    what: "pypi:workspace-mcp@2.0.1",
    value: "reaches gmail.googleapis.com; all its tools; configuration MCP_SINGLE_USER_MODE; signs in with OAuth at https://accounts.google.com, asking for scopes https://www.googleapis.com/auth/gmail.readonly",
  });
  assert.equal(signInPhrase({ source: "url", url: "https://x.example/mcp", auth: "none" }), "signs in with nothing");
  assert.equal(signInPhrase({ source: "url", url: "https://x.example/mcp", auth: "oauth" }), "signs in with OAuth, asking for the vendor's default scopes");
  assert.equal(signInPhrase({ source: "npm", package: "x", version: "1.0.0" }), "");
  assert.equal(reachRow({ source: "url", url: "" }).what, "a remote server, no address yet");
});
