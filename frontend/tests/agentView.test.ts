// The views a persistent agent's page offers, and the header's usage line.
import { test } from "node:test";
import assert from "node:assert/strict";
import { AGENT_VIEWS, agentView, DEFAULT_AGENT_VIEW, isAgentView, usageLine } from "../src/agentView.ts";

test("the View menu lists Terminal first, then Compact, Chat, Focus and Workspace", () => {
  assert.deepEqual(AGENT_VIEWS.map((v) => v.title), ["Terminal", "Compact", "Chat", "Focus", "Workspace"]);
  assert.equal(DEFAULT_AGENT_VIEW, "terminal");
  for (const v of AGENT_VIEWS) assert.ok(v.line.endsWith("."), v.id);
});

test("only Terminal and Compact draw the chat in a terminal's manner, and only Focus hides the panes", () => {
  assert.deepEqual(AGENT_VIEWS.filter((v) => v.term).map((v) => v.id), ["terminal", "compact"]);
  assert.deepEqual(AGENT_VIEWS.filter((v) => !v.panes).map((v) => v.id), ["focus"]);
});

test("a view this app lacks is Terminal", () => {
  assert.equal(agentView("hologram").id, "terminal");
  assert.equal(agentView("focus").id, "focus");
  assert.equal(isAgentView("hologram"), false);
  assert.equal(isAgentView("compact"), true);
});

test("the usage line keeps what says something, in the order asked", () => {
  const rows = [
    { label: "This segment", value: "1,200 in · 300 out" },
    { label: "Turns", value: "—" },
    { label: "Tool calls", value: "4" },
    { label: "Cost", value: "not reported" },
  ];
  assert.equal(usageLine(rows, ["Tool calls", "This segment", "Turns", "Missing"]), "tool calls 4 · this segment 1,200 in · 300 out");
  assert.equal(usageLine([{ label: "This segment", value: "not running" }, { label: "Last turn", value: "not reported" }, { label: "Context", value: "unknown" }], ["This segment", "Last turn", "Context"]), "");
});
