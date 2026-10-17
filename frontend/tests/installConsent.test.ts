// The first install's consent names what the release declares in the
// CLI's words, a server's reach included.
import { test } from "node:test";
import assert from "node:assert/strict";
import { acceptSentence, consentWords } from "../src/installConsent.ts";

test("one, two and several things are joined as the CLI joins them", () => {
  assert.equal(consentWords(["a remote MCP server"]), "a remote MCP server");
  assert.equal(consentWords(["network egress", "exec"]), "network egress and exec");
  assert.equal(
    consentWords(["network egress", "UNRESTRICTED network egress for MCP server npm:@acme/osv-mcp@1.2.3", "a web browser"]),
    "network egress, UNRESTRICTED network egress for MCP server npm:@acme/osv-mcp@1.2.3 and a web browser",
  );
  assert.equal(consentWords(null), "");
});

test("the sentence names them, and falls back when the plan names none", () => {
  assert.equal(acceptSentence(["a remote MCP server"]), "Accept what this release declares: a remote MCP server.");
  assert.equal(acceptSentence([]), "Accept the declared capabilities.");
});
