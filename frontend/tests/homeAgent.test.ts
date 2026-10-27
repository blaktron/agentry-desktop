// Home's agent card and the Welcome's Agents step: the agent the card
// draws, its hint, the tiles and the choices.
import { test } from "node:test";
import assert from "node:assert/strict";
import { AGENT_TILES, agentCardHint, helperSuperseded, homeAgentDrawn, homeChoices, isOn, tilesFor } from "../src/homeAgent.ts";

const agents = [
  { name: "main", state: "disabled", displayName: "Agentry Helper", share: "/home/p/agentry/mainshare" },
  { name: "assistant", state: "stopped", displayName: "Sandra", share: "/home/p/agentry/assistantshare" },
  { name: "coder", state: "disabled", displayName: "Coding agent", share: "/home/p/agentry/codingshare" },
];

test("the card draws the agent picked while it is on, else the Assistant, else Agentry Helper", () => {
  assert.equal(homeAgentDrawn("main", agents), "assistant", "Agentry Helper is off");
  assert.equal(homeAgentDrawn("assistant", agents), "assistant");
  assert.equal(homeAgentDrawn("coder", agents), "assistant", "an agent that is off");
  assert.equal(homeAgentDrawn("butler", agents), "assistant", "an agent this app lacks");
  const helperOnly = [{ ...agents[0], state: "stopped" }, { ...agents[1], state: "disabled" }, agents[2]];
  assert.equal(homeAgentDrawn("main", helperOnly), "main");
  assert.equal(homeAgentDrawn("assistant", helperOnly), "main", "the Assistant is off");
  const coderOnly = [agents[0], { ...agents[1], state: "disabled" }, { ...agents[2], state: "running" }];
  assert.equal(homeAgentDrawn("coder", coderOnly), "coder");
  assert.equal(homeAgentDrawn("main", coderOnly), null, "the Coding agent is drawn only when picked");
  assert.equal(homeAgentDrawn("main", agents.map((a) => ({ ...a, state: "disabled" }))), null, "no agent is on");
  assert.equal(homeAgentDrawn("assistant", []), null, "before the list is read");
});

test("the hint names the agent and its share folder", () => {
  assert.equal(agentCardHint(agents[0]), "Ask Agentry Helper to find a QuickAgent, run one on a file in your mainshare folder, or schedule one.");
  assert.equal(agentCardHint(agents[1]), "Ask Sandra anything, or to find, run or schedule a QuickAgent on a file in your assistantshare folder.");
  assert.equal(agentCardHint(agents[2]), "Ask Coding agent to build, run or fix code in your codingshare folder.");
  assert.equal(agentCardHint({ name: "assistant", displayName: "Sandra", share: "" }), "Ask Sandra anything, or to find, run or schedule a QuickAgent on a file.");
  assert.equal(agentCardHint({ name: "coder", displayName: "Coding agent", share: "C:\\Users\\p\\code\\" }), "Ask Coding agent to build, run or fix code in your code folder.");
});

test("the tiles follow the app's agents, by the names the person gave them", () => {
  assert.deepEqual(AGENT_TILES.map((t) => t.name), ["main", "assistant", "coder"]);
  const tiles = tilesFor(agents);
  assert.deepEqual(tiles.map((t) => t.title), ["Agentry Helper", "Sandra", "Coding agent"]);
  assert.deepEqual(tilesFor([agents[0]]).map((t) => t.name), ["main"], "an agent the app does not list has no tile");
});

test("the card's choices are the agents that are on", () => {
  assert.deepEqual(homeChoices(agents), ["assistant"]);
  assert.equal(isOn(agents[0]), false, "Agentry Helper is off until turned on");
  assert.equal(isOn(agents[1]), true);
  assert.equal(isOn(agents[2]), false);
});

test("Agentry Helper is held off while the Assistant is on", () => {
  assert.equal(helperSuperseded(agents), true);
  assert.equal(helperSuperseded([agents[0], { ...agents[1], state: "disabled" }]), false);
  assert.equal(helperSuperseded([agents[0], { ...agents[1], state: "running" }]), true);
  assert.equal(helperSuperseded([]), false);
});

test("the Assistant's tile alone is recommended, and says it turns the Helper off", () => {
  assert.deepEqual(AGENT_TILES.filter((t) => t.recommended).map((t) => t.name), ["assistant"]);
  assert.match(AGENT_TILES[0].line, /^Lite personal agent that works only with Agentry/);
  assert.match(AGENT_TILES[1].line, /everything the Helper does/);
  assert.match(AGENT_TILES[1].line, /Turns the Helper off\.$/);
});
