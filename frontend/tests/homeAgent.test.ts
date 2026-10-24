// Home's agent card and the Welcome's Agents step: the agent the card
// draws, its hint, the tiles and the choices.
import { test } from "node:test";
import assert from "node:assert/strict";
import { AGENT_TILES, agentCardHint, homeAgentDrawn, homeChoices, isOn, tilesFor } from "../src/homeAgent.ts";

const agents = [
  { name: "main", state: "disabled", displayName: "Agentry Helper", share: "/home/p/agentry/mainshare" },
  { name: "assistant", state: "stopped", displayName: "Sandra", share: "/home/p/agentry/assistantshare" },
  { name: "coder", state: "disabled", displayName: "Coding agent", share: "/home/p/agentry/codingshare" },
];

test("the card draws the agent picked while it is on, else Agentry Helper", () => {
  assert.equal(homeAgentDrawn("main", agents), "main");
  assert.equal(homeAgentDrawn("assistant", agents), "assistant");
  assert.equal(homeAgentDrawn("coder", agents), "main", "an agent that is off");
  assert.equal(homeAgentDrawn("butler", agents), "main", "an agent this app lacks");
  assert.equal(homeAgentDrawn("assistant", []), "main", "before the list is read");
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

test("the card's choices are Agentry Helper and the agents that are on", () => {
  assert.deepEqual(homeChoices(agents), ["main", "assistant"]);
  assert.equal(isOn(agents[0]), true, "Agentry Helper is always on");
  assert.equal(isOn(agents[2]), false);
});
